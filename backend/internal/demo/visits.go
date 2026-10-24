package demo

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	availabilitymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	branchmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/model"
	paymentsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	paymentsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
	productsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/model"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
)

// Visits are booked the way the panel books them: the slot engine offers
// the free starts of the day, one is picked and the reservation use case
// books it under the business's capacity lock.

var anyone = availabilityusecase.Caller{}

// Shares of outcomes, in percent.
const (
	cancelPercent = 6
	noShowPercent = 5
	cashPercent   = 60
)

type visit struct {
	id     uuid.UUID
	branch int
}

func closedOn(h branchmodel.WeeklyHours, d time.Time) bool {
	switch d.Weekday() {
	case time.Monday:
		return h.Mon.Closed
	case time.Tuesday:
		return h.Tue.Closed
	case time.Wednesday:
		return h.Wed.Closed
	case time.Thursday:
		return h.Thu.Closed
	case time.Friday:
		return h.Fri.Closed
	case time.Saturday:
		return h.Sat.Closed
	default:
		return h.Sun.Closed
	}
}

// seedHistory books the visits of every day before today in the morning of
// that day and closes them in the evening: completed and paid (cash into
// the branch till or card into the bank account), a few cancelled or not
// attended.
func (s *Seeder) seedHistory(b *biz, from, today time.Time) error {
	n := 0
	for d := from; d.Before(today); d = d.AddDate(0, 0, 1) {
		if closedOn(b.spec.Hours, d) {
			continue
		}
		s.now = d.Add(5 * time.Minute)
		var day []visit
		for bi := range b.branches {
			for range b.spec.VisitsPerDay + b.rng.IntN(2) {
				r, ok, err := s.bookOne(b, bi, d, n)
				n++
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				if b.rng.IntN(100) < cancelPercent {
					if _, err := s.reservations.Cancel(b.ctx, b.org.ID, r.UUID, anyone, b.reception.ID, reservationsusecase.ChangeInput{Reason: "customer_cancelled"}); err != nil {
						return fmt.Errorf("cancel: %w", err)
					}
					continue
				}
				day = append(day, visit{id: r.UUID, branch: bi})
			}
		}
		s.now = d.Add(23*time.Hour + 30*time.Minute)
		for _, v := range day {
			if err := s.closeVisit(b, v); err != nil {
				return err
			}
		}
		if err := s.walkInSale(b, d); err != nil {
			return err
		}
	}
	return nil
}

// closeVisit completes and collects a past visit (or marks a no-show).
func (s *Seeder) closeVisit(b *biz, v visit) error {
	ctx, org, actor := b.ctx, b.org.ID, b.reception.ID
	if b.rng.IntN(100) < noShowPercent {
		if _, err := s.reservations.NoShow(ctx, org, v.id, anyone, actor, reservationsusecase.ChangeInput{}); err != nil {
			return fmt.Errorf("no-show: %w", err)
		}
		return nil
	}
	if _, err := s.reservations.Start(ctx, org, v.id, anyone, actor, reservationsusecase.ChangeInput{}); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	if _, err := s.reservations.Complete(ctx, org, v.id, anyone, actor, reservationsusecase.ChangeInput{}); err != nil {
		return fmt.Errorf("complete: %w", err)
	}
	b.stats.Completed++
	view, err := s.payments.ReservationPayments(ctx, org, v.id, anyone, paymentsusecase.ViewOptions{})
	if err != nil {
		return err
	}
	if !view.Remaining.IsPositive() {
		return nil
	}
	method, account := "card", b.bank
	if b.rng.IntN(100) < cashPercent {
		method, account = "cash", b.tills[v.branch]
	}
	if _, err := s.payments.Collect(ctx, org, v.id, anyone, actor, paymentsusecase.ViewOptions{}, paymentsmodel.CollectInput{
		Amount: view.Remaining, Method: method, CashAccountUUID: account,
	}); err != nil {
		return fmt.Errorf("collect: %w", err)
	}
	return nil
}

// walkInSale sells a product over the counter on some days.
func (s *Seeder) walkInSale(b *biz, d time.Time) error {
	if len(b.spec.Products) == 0 || b.rng.IntN(3) != 0 {
		return nil
	}
	sp := b.spec.Products[b.rng.IntN(len(b.spec.Products))]
	bi := b.rng.IntN(len(b.branches))
	in := productsmodel.MovementInput{
		Kind: productsmodel.KindSale, BranchUUID: b.branches[bi].UUID, ProductUUID: b.products[sp.Name],
		Quantity: ptr(int32(1)), UnitPrice: num(sp.Price), OccurredAt: ptr(d.Add(16 * time.Hour)),
	}
	if st := s.staffOf(b, bi); st != nil {
		in.StaffUUID = st
	}
	if _, err := s.products.RecordMovement(b.ctx, b.org.ID, b.reception.ID, in); err != nil {
		return fmt.Errorf("walk-in sale: %w", err)
	}
	return nil
}

func (s *Seeder) staffOf(b *biz, branch int) *uuid.UUID {
	var ids []uuid.UUID
	for _, n := range b.staffNames {
		for _, i := range b.staffBranches[n] {
			if i == branch {
				ids = append(ids, b.staff[n].UUID)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return &ids[b.rng.IntN(len(ids))]
}

// seedUpcoming books the coming days (from now on) and leaves them open.
func (s *Seeder) seedUpcoming(b *biz, now, today time.Time, days int) error {
	s.now = now
	n := 0
	for d := today; d.Before(today.AddDate(0, 0, days)); d = d.AddDate(0, 0, 1) {
		if closedOn(b.spec.Hours, d) {
			continue
		}
		for bi := range b.branches {
			for range b.spec.VisitsPerDay {
				r, ok, err := s.bookOne(b, bi, d, n)
				n++
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				if b.rng.IntN(100) < cancelPercent {
					if _, err := s.reservations.Cancel(b.ctx, b.org.ID, r.UUID, anyone, b.reception.ID, reservationsusecase.ChangeInput{Reason: "customer_cancelled"}); err != nil {
						return fmt.Errorf("cancel: %w", err)
					}
					continue
				}
				b.stats.Upcoming++
			}
		}
	}
	return nil
}

func (b *biz) pickBooking() bookingSpec {
	total := 0
	for _, k := range b.spec.Bookings {
		total += k.Weight
	}
	n := b.rng.IntN(total)
	for _, k := range b.spec.Bookings {
		if n < k.Weight {
			return k
		}
		n -= k.Weight
	}
	return b.spec.Bookings[0]
}

// bookOne books one visit on day d in branch bi; ok is false when the day
// has no free time left for the kinds tried. The n-th visit goes to the
// panel customer now and then, so her panel has visits in both businesses.
func (s *Seeder) bookOne(b *biz, bi int, d time.Time, n int) (reservationsmodel.Reservation, bool, error) {
	customer := b.customers[b.rng.IntN(len(b.customers))]
	if b.panel != uuid.Nil && n%6 == 0 {
		customer = b.panel
	}
	for range 3 {
		kind := b.pickBooking()
		names := kind.Services
		var bundle *uuid.UUID
		if kind.Bundle != "" {
			names = b.bundleServices[kind.Bundle]
			bundle = ptr(b.bundles[kind.Bundle])
		}
		lines := make([]availabilitymodel.LineInput, 0, len(names))
		for _, name := range names {
			id, ok := b.services[name]
			if !ok {
				return reservationsmodel.Reservation{}, false, fmt.Errorf("service %q not found", name)
			}
			lines = append(lines, availabilitymodel.LineInput{ServiceUUID: id})
		}
		search := availabilitymodel.SearchInput{
			BranchUUID: b.branches[bi].UUID, Lines: lines,
			From: d.Format(time.DateOnly), To: d.Format(time.DateOnly),
		}
		if kind.PartyMax > 0 {
			search.PartySize = ptr(kind.PartyMin + b.rng.IntN(kind.PartyMax-kind.PartyMin+1))
			if zones := b.zones[bi]; len(zones) > 0 && b.rng.IntN(2) == 0 {
				search.Zone = zones[b.rng.IntN(len(zones))]
			}
		}
		found, err := s.search.Search(b.ctx, b.org.ID, search, anyone)
		if err != nil {
			return reservationsmodel.Reservation{}, false, fmt.Errorf("search %v: %w", names, err)
		}
		if len(found.Days) == 0 || len(found.Days[0].Slots) == 0 {
			continue
		}
		slots := found.Days[0].Slots
		start := slots[b.rng.IntN(len(slots))].StartsAt
		in := reservationsmodel.CreateInput{
			BranchUUID: b.branches[bi].UUID, CustomerUUID: customer, StartsAt: &start,
			Source: reservationsmodel.SourcePanel, PartySize: search.PartySize, Zone: search.Zone,
		}
		if bundle != nil {
			in.BundleUUID = bundle
		} else {
			in.Lines = lines
		}
		if kind.Vehicle {
			in.Vehicle = &reservationsmodel.Vehicle{Type: vehicles[b.rng.IntN(len(vehicles))], Plate: plates[b.rng.IntN(len(plates))]}
		}
		if kind.Product != "" && b.rng.IntN(2) == 0 {
			in.Products = []reservationsmodel.ProductInput{{ProductUUID: b.products[kind.Product], Quantity: 1}}
		}
		r, err := s.reservations.Create(b.ctx, b.org.ID, in, anyone, b.reception.ID)
		if errors.Is(err, availabilityusecase.ErrSlotTaken) {
			continue
		}
		if err != nil {
			return r, false, fmt.Errorf("book %v at %s: %w", names, start.In(b.loc).Format(time.DateTime), err)
		}
		if err := s.usePackage(b, r, customer, names); err != nil {
			return r, false, err
		}
		b.stats.Reservations++
		return r, true, nil
	}
	return reservationsmodel.Reservation{}, false, nil
}

// usePackage pays a one-service visit with a session of the customer's
// package for that service.
func (s *Seeder) usePackage(b *biz, r reservationsmodel.Reservation, customer uuid.UUID, names []string) error {
	if len(names) != 1 {
		return nil
	}
	for _, cp := range b.owned[customer] {
		if cp.Service != names[0] {
			continue
		}
		_, err := s.reservations.ApplyPackage(b.ctx, b.org.ID, r.UUID, anyone, b.reception.ID, reservationsmodel.ApplyPackageInput{
			CustomerPackageUUID: cp.UUID, LineIndex: 0,
		})
		if err != nil {
			// A used-up package simply leaves the visit to be paid.
			if isNoSessionsLeft(err) {
				return nil
			}
			return fmt.Errorf("apply package: %w", err)
		}
		return nil
	}
	return nil
}

func isNoSessionsLeft(err error) bool {
	var ve *reservationsusecase.ValidationError
	if !errors.As(err, &ve) {
		return false
	}
	for _, d := range ve.Details {
		if d.Code == "no_sessions_left" || d.Code == "expired" {
			return true
		}
	}
	return false
}
