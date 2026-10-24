package usecase

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilitymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/model"
	orgsettings "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// CancelReason is the cancel reason of a visit the customer cancelled in
// the panel (the business sees it in the reservation history).
const CancelReason = reservationsmodel.ReasonCustomerCancelled

// SettingsReader returns the current business settings.
type SettingsReader interface {
	Current(ctx context.Context, orgID int64) (orgsettings.Version, error)
}

// PanelDeps are the business use cases the panel acts through.
type PanelDeps struct {
	Reservations *reservationsusecase.Service
	Availability *availabilityusecase.Service
	Settings     SettingsReader
	// Confirmations records confirmation answers (TEC-921); nil offers
	// none.
	Confirmations Confirmer
}

// ActionError: the business rules do not allow the action now (Reason is
// a model.Reason* code).
type ActionError struct {
	Action string
	Reason string
}

func (e *ActionError) Error() string {
	return e.Action + " is not allowed: " + e.Reason
}

// Changed is the outcome of a customer change, with the business it
// happened in (activity and live lists of the business panel).
type Changed struct {
	Reservation model.Reservation
	Scope       orgctx.Scope
}

// standing reports whether a visit is still ahead (requested, booked or in
// service and not over).
func standing(status string, endsAt, now time.Time) bool {
	switch status {
	case reservationsmodel.StatusRequested, reservationsmodel.StatusActive, reservationsmodel.StatusInService:
		return endsAt.After(now)
	}
	return false
}

// paidStates are payment states in which the customer's money arrived.
var paidStates = []string{
	reservationsmodel.PaymentPaid, reservationsmodel.PaymentRefundPending,
	reservationsmodel.PaymentPartiallyRefunded, reservationsmodel.PaymentRefunded,
}

// moneyOf derives the recorded payment picture of a visit from its total
// and states. Until the payment ledger (TEC-912) records amounts, an online
// payment is the full total (beta: no deposits) and a refund is the full
// total when refunded; a partial refund has no amount yet (null).
func moneyOf(total money.Money, currency, status, payment string) model.Money {
	out := model.Money{Total: total, Paid: money.Zero(), Currency: currency, PaymentStatus: payment, Method: model.MethodOnline}
	if payment == reservationsmodel.PaymentOnSite {
		out.Method = model.MethodOnSite
	}
	if slices.Contains(paidStates, payment) {
		out.Paid = total
	}
	switch payment {
	case reservationsmodel.PaymentRefunded:
		r := total
		out.Refunded = &r
	case reservationsmodel.PaymentRefundPending:
		r := money.Zero()
		out.Refunded = &r
	}
	var balance money.Money
	switch {
	case status == reservationsmodel.StatusCancelled || status == reservationsmodel.StatusRejected:
		balance = money.Zero()
	case payment == reservationsmodel.PaymentOnSite &&
		(status == reservationsmodel.StatusCompleted || status == reservationsmodel.StatusNoShow):
		// Settled at the business; nothing recorded here.
		return out
	default:
		balance = total.Sub(out.Paid).ClampZero()
	}
	out.Balance = &balance
	return out
}

func listArgs(f model.ListFilter) (pgtype.Bool, pgtype.UUID, []string) {
	var upcoming pgtype.Bool
	switch f.Scope {
	case model.ScopeUpcoming:
		upcoming = pgtype.Bool{Bool: true, Valid: true}
	case model.ScopePast:
		upcoming = pgtype.Bool{Bool: false, Valid: true}
	}
	var business pgtype.UUID
	if f.BusinessUUID != nil {
		business = pgtype.UUID{Bytes: *f.BusinessUUID, Valid: true}
	}
	var statuses []string
	if len(f.Statuses) > 0 {
		statuses = f.Statuses
	}
	return upcoming, business, statuses
}

// List returns the person's reservations across businesses.
func (s *Service) List(ctx context.Context, p Principal, f model.ListFilter) ([]model.ReservationItem, int64, error) {
	s.linkRecords(ctx, p.Phone, p.VerifiedAt, p.source())
	now := s.now()
	upcoming, business, statuses := listArgs(f)
	rows, err := s.q.ListCustomerPanelReservations(ctx, db.ListCustomerPanelReservationsParams{
		IdentityID: p.IdentityID, Upcoming: upcoming, Now: ts(now), BusinessUuid: business, Statuses: statuses,
		WithMoney: f.WithMoney, SortDesc: f.SortDesc, RowLimit: f.Limit, RowOffset: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountCustomerPanelReservations(ctx, db.CountCustomerPanelReservationsParams{
		IdentityID: p.IdentityID, Upcoming: upcoming, Now: ts(now), BusinessUuid: business, Statuses: statuses,
		WithMoney: f.WithMoney,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.ReservationItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, itemOf(r, now))
	}
	return out, total, nil
}

func itemOf(r db.ListCustomerPanelReservationsRow, now time.Time) model.ReservationItem {
	total := money.FromNumericOrZero(r.TotalPrice)
	item := model.ReservationItem{
		UUID: r.Uuid, Number: r.Number,
		Business: model.BusinessRef{UUID: r.BusinessUuid, Slug: r.BusinessSlug, Name: r.BusinessName},
		Branch:   model.Ref{UUID: r.BranchUuid, Name: r.BranchName}, Timezone: r.Timezone,
		StartsAt: r.StartsAt.Time, EndsAt: r.EndsAt.Time, Upcoming: standing(r.Status, r.EndsAt.Time, now),
		Status: r.Status, ApprovalStatus: r.ApprovalStatus, ConfirmationStatus: r.ConfirmationStatus,
		PaymentStatus: r.PaymentStatus, Services: r.ServiceNames, Staff: r.StaffNames,
		Money:     moneyOf(total, r.Currency, r.Status, r.PaymentStatus),
		CreatedAt: r.CreatedAt.Time,
	}
	if r.CancelReason.Valid {
		v := r.CancelReason.String
		item.CancelReason = &v
	}
	if item.Services == nil {
		item.Services = []string{}
	}
	if item.Staff == nil {
		item.Staff = []string{}
	}
	return item
}

// PaymentSummary sums the person's amounts per currency.
func (s *Service) PaymentSummary(ctx context.Context, p Principal) (model.PaymentSummary, error) {
	rows, err := s.q.CustomerPanelPaymentTotals(ctx, p.IdentityID)
	if err != nil {
		return model.PaymentSummary{}, err
	}
	byCurrency := map[string]*model.PaymentTotal{}
	out := model.PaymentSummary{Totals: []model.PaymentTotal{}}
	order := []string{}
	for _, r := range rows {
		t, ok := byCurrency[r.Currency]
		if !ok {
			t = &model.PaymentTotal{Currency: r.Currency, Booked: money.Zero(), Paid: money.Zero(), RefundPending: money.Zero(), Refunded: money.Zero()}
			byCurrency[r.Currency] = t
			order = append(order, r.Currency)
		}
		amount := money.FromNumericOrZero(r.Amount)
		if r.Standing {
			t.Booked = t.Booked.Add(amount)
			t.ReservationCount += r.ReservationCount
		}
		if slices.Contains(paidStates, r.PaymentStatus) {
			t.Paid = t.Paid.Add(amount)
		}
		switch r.PaymentStatus {
		case reservationsmodel.PaymentRefundPending:
			t.RefundPending = t.RefundPending.Add(amount)
		case reservationsmodel.PaymentRefunded:
			t.Refunded = t.Refunded.Add(amount)
		}
	}
	for _, c := range order {
		out.Totals = append(out.Totals, *byCurrency[c])
	}
	return out, nil
}

// owned is a reservation of the person with its business context.
type owned struct {
	row      db.GetCustomerPanelReservationRow
	full     reservationsmodel.Reservation
	settings orgsettings.Version
	live     bool
	modules  entitlement.Effective
}

func (s *Service) load(ctx context.Context, p Principal, id uuid.UUID) (owned, error) {
	row, err := s.q.GetCustomerPanelReservation(ctx, db.GetCustomerPanelReservationParams{IdentityID: p.IdentityID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return owned{}, ErrNotFound
	}
	if err != nil {
		return owned{}, err
	}
	return s.loadRow(ctx, row)
}

// loadRow completes a reservation row (found through the person's links or
// a visit link) with the record, the settings and the business state.
func (s *Service) loadRow(ctx context.Context, row db.GetCustomerPanelReservationRow) (owned, error) {
	full, err := s.deps.Reservations.Get(ctx, row.OrganizationID, row.Uuid, availabilityusecase.Caller{})
	if errors.Is(err, reservationsusecase.ErrNotFound) {
		return owned{}, ErrNotFound
	}
	if err != nil {
		return owned{}, err
	}
	settings, err := s.deps.Settings.Current(ctx, row.OrganizationID)
	if err != nil {
		return owned{}, err
	}
	now := s.now()
	live := row.BusinessStatus == orgctx.StatusActive &&
		(!row.AccessStartsAt.Valid || !row.AccessStartsAt.Time.After(now)) &&
		(!row.AccessEndsAt.Valid || row.AccessEndsAt.Time.After(now))
	st := entitlement.Stored{Plan: row.Plan, Addons: row.Addons}
	if row.BranchLimit.Valid {
		st.BranchLimit = &row.BranchLimit.Int32
	}
	if row.StaffLimit.Valid {
		st.StaffLimit = &row.StaffLimit.Int32
	}
	return owned{row: row, full: full, settings: settings, live: live, modules: entitlement.Resolve(row.Sector, st)}, nil
}

func reason(code string) *string {
	return &code
}

// deadline is the last moment an action is allowed: start − cutoff (nil
// cutoff = until the start).
func deadline(start time.Time, cutoff *int) time.Time {
	if cutoff == nil {
		return start
	}
	return start.Add(-time.Duration(*cutoff) * time.Minute)
}

// actions applies the business rules to a visit at now.
func (o owned) actions(now time.Time) model.Actions {
	res := o.settings.Settings.Reservation
	r := o.full
	open := r.Status == reservationsmodel.StatusRequested || r.Status == reservationsmodel.StatusActive
	decide := func(cutoff *int) model.Action {
		until := deadline(r.StartsAt, cutoff)
		switch {
		case !o.live:
			return model.Action{Reason: reason(model.ReasonBusinessUnavailable)}
		case !open:
			return model.Action{Reason: reason(model.ReasonStatus)}
		case !now.Before(until):
			return model.Action{Reason: reason(model.ReasonCutoff), Until: &until}
		}
		return model.Action{Allowed: true, Until: &until}
	}
	out := model.Actions{Cancel: decide(res.CancelCutoffMinutes), Reschedule: decide(res.ChangeCutoffMinutes)}
	out.Confirm, out.RequestChange = o.confirmActions(now)
	if out.Reschedule.Allowed {
		switch {
		case !o.modules.Allows(sector.ModuleReservations) || !o.modules.Allows(sector.ModuleBookingPage) || !o.settings.ReadyForOnlineSales:
			out.Reschedule = model.Action{Reason: reason(model.ReasonOnlineUnavailable)}
		case slices.ContainsFunc(r.Lines, func(l reservationsmodel.Line) bool { return l.ServiceUUID == nil }):
			out.Reschedule = model.Action{Reason: reason(model.ReasonServicesChanged)}
		}
	}
	return out
}

// view maps the record to the customer's detail.
func (s *Service) view(o owned) model.Reservation {
	now := s.now()
	r := o.full
	set := o.settings.Settings
	item := model.ReservationItem{
		UUID: r.UUID, Number: r.Number,
		Business: model.BusinessRef{UUID: o.row.BusinessUuid, Slug: o.row.BusinessSlug, Name: o.row.BusinessName},
		Branch:   model.Ref{UUID: r.Branch.UUID, Name: r.Branch.Name}, Timezone: o.row.Timezone,
		StartsAt: r.StartsAt, EndsAt: r.EndsAt, Upcoming: standing(r.Status, r.EndsAt, now),
		Status: r.Status, ApprovalStatus: r.ApprovalStatus, ConfirmationStatus: r.ConfirmationStatus,
		PaymentStatus: r.PaymentStatus, CancelReason: r.CancelReason,
		Services: []string{}, Staff: []string{},
		Money:     moneyOf(r.TotalPrice, r.Currency, r.Status, r.PaymentStatus),
		CreatedAt: r.CreatedAt,
	}
	out := model.Reservation{
		ReservationItem: item,
		Branch: model.Branch{
			UUID: r.Branch.UUID, Name: r.Branch.Name, Address: o.row.BranchAddress, City: o.row.BranchCity,
			District: o.row.BranchDistrict, Phone: o.row.BranchPhone,
		},
		Version: r.Version, CustomerNote: r.CustomerNote, HoldExpiresAt: r.HoldExpiresAt,
		Lines: make([]model.Line, 0, len(r.Lines)),
		Policies: model.Policies{
			Cancellation: set.Policies.Cancellation, Change: set.Policies.Change, NoShow: set.Policies.NoShow,
			CancelCutoffMinutes: set.Reservation.CancelCutoffMinutes, ChangeCutoffMinutes: set.Reservation.ChangeCutoffMinutes,
		},
		Actions:      o.actions(now),
		Confirmation: confirmationView(r),
		History:      []model.HistoryEntry{},
		Refunds:      []model.Refund{},
	}
	for _, l := range r.Lines {
		line := model.Line{
			ServiceName: l.ServiceName, StartsAt: l.StartsAt, EndsAt: l.EndsAt,
			DurationMinutes: l.DurationMinutes, UnitPrice: l.UnitPrice,
		}
		out.Services = append(out.Services, l.ServiceName)
		if l.Staff != nil {
			name := l.Staff.Name
			line.StaffName = &name
			if !slices.Contains(out.Staff, name) {
				out.Staff = append(out.Staff, name)
			}
		}
		out.Lines = append(out.Lines, line)
	}
	for _, e := range r.Events {
		switch e.Type {
		case "notes_updated", "confirmation_scheduled", "confirmation_dropped":
			continue
		}
		out.History = append(out.History, model.HistoryEntry{Type: e.Type, ToStatus: e.ToStatus, At: e.CreatedAt})
	}
	return out
}

// Reservation returns one visit of the person.
func (s *Service) Reservation(ctx context.Context, p Principal, id uuid.UUID) (model.Reservation, error) {
	o, err := s.load(ctx, p, id)
	if err != nil {
		return model.Reservation{}, err
	}
	out := s.view(o)
	refunds, err := s.q.ListCustomerRefunds(ctx, db.ListCustomerRefundsParams{OrganizationID: o.row.OrganizationID, ReservationID: o.row.ID})
	if err != nil {
		return model.Reservation{}, err
	}
	for _, f := range refunds {
		r := model.Refund{
			UUID: f.Uuid, Amount: money.FromNumericOrZero(f.Amount), Currency: f.Currency, Status: f.Status, RequestedAt: f.CreatedAt.Time,
		}
		if f.ReflectedAt.Valid {
			t := f.ReflectedAt.Time
			r.RefundedAt = &t
		}
		out.Refunds = append(out.Refunds, r)
	}
	if len(out.Refunds) > 0 {
		// The refunds that are done are the refunded amount of the visit.
		done := money.Zero()
		for _, r := range out.Refunds {
			if r.Status == "succeeded" && r.Currency == out.Money.Currency {
				done = done.Add(r.Amount)
			}
		}
		out.Money.Refunded = &done
	}
	return out, nil
}

// guard re-checks, on the locked record, that it is still the person's
// and that the action is still inside its cutoff.
func guard(o owned, action string, cutoff *int, now func() time.Time) func(reservationsmodel.Locked) error {
	return func(l reservationsmodel.Locked) error {
		if l.CustomerID != o.row.CustomerID {
			return ErrNotFound
		}
		if !now().Before(deadline(l.StartsAt, cutoff)) {
			return &ActionError{Action: action, Reason: model.ReasonCutoff}
		}
		return nil
	}
}

func scopeOf(o owned) orgctx.Scope {
	return orgctx.Scope{
		ID: o.row.OrganizationID, UUID: o.row.BusinessUuid, Slug: o.row.BusinessSlug, Name: o.row.BusinessName,
		Status: o.row.BusinessStatus, Sector: o.row.Sector, Timezone: o.row.Timezone,
	}
}

// Cancel cancels a visit within the business's cancel cutoff; the capacity
// is freed by the reservations use case (reason customer_cancelled).
func (s *Service) Cancel(ctx context.Context, p Principal, id uuid.UUID, in model.CancelInput) (Changed, error) {
	o, err := s.load(ctx, p, id)
	if err != nil {
		return Changed{}, err
	}
	if err := s.cancel(ctx, o, in.ExpectedVersion); err != nil {
		return Changed{}, err
	}
	return s.reloaded(ctx, p, o)
}

// cancel applies the cancel rules and cancels through the reservations use
// case, with the guard on the locked record.
func (s *Service) cancel(ctx context.Context, o owned, version *int) error {
	if a := o.actions(s.now()).Cancel; !a.Allowed {
		return &ActionError{Action: "cancel", Reason: *a.Reason}
	}
	cutoff := o.settings.Settings.Reservation.CancelCutoffMinutes
	_, err := s.deps.Reservations.Cancel(ctx, o.row.OrganizationID, o.row.Uuid, availabilityusecase.Caller{}, 0, reservationsusecase.ChangeInput{
		ExpectedVersion: version, Reason: CancelReason, Guard: guard(o, "cancel", cutoff, s.now),
	})
	return err
}

func (s *Service) reloaded(ctx context.Context, p Principal, o owned) (Changed, error) {
	fresh, err := s.load(ctx, p, o.row.Uuid)
	if err != nil {
		return Changed{}, err
	}
	return Changed{Reservation: s.view(fresh), Scope: scopeOf(fresh)}, nil
}

// lineInputs are the current services and staff of a visit.
func lineInputs(r reservationsmodel.Reservation) []availabilitymodel.LineInput {
	out := make([]availabilitymodel.LineInput, 0, len(r.Lines))
	for _, l := range r.Lines {
		if l.ServiceUUID == nil {
			continue
		}
		line := availabilitymodel.LineInput{ServiceUUID: *l.ServiceUUID}
		if l.Staff != nil && l.Staff.UUID != nil {
			staff := *l.Staff.UUID
			line.StaffUUID = &staff
		}
		out = append(out, line)
	}
	return out
}

// Availability searches new starts for a visit: the same services, staff,
// party and area, on the online channel (the business's online rules apply).
func (s *Service) Availability(ctx context.Context, p Principal, id uuid.UUID, in model.AvailabilityInput) (model.Availability, error) {
	o, err := s.load(ctx, p, id)
	if err != nil {
		return model.Availability{}, err
	}
	if a := o.actions(s.now()).Reschedule; !a.Allowed {
		return model.Availability{}, &ActionError{Action: "reschedule", Reason: *a.Reason}
	}
	res, err := s.deps.Availability.Search(ctx, o.row.OrganizationID, availabilitymodel.SearchInput{
		BranchUUID: o.full.Branch.UUID, Lines: lineInputs(o.full), From: in.From, To: in.To,
		Channel:   availabilitymodel.ChannelOnline,
		PartySize: o.full.PartySize, Zone: o.full.Zone,
	}, availabilityusecase.Caller{})
	if err != nil {
		return model.Availability{}, err
	}
	out := model.Availability{Timezone: res.Timezone, EarliestStart: res.EarliestStart, LatestDate: res.LatestDate, Days: make([]model.Day, 0, len(res.Days))}
	for _, d := range res.Days {
		day := model.Day{Date: d.Date, Status: d.Status, Starts: make([]time.Time, 0, len(d.Slots))}
		for _, slot := range d.Slots {
			if !slot.StartsAt.Equal(o.full.StartsAt) {
				day.Starts = append(day.Starts, slot.StartsAt)
			}
		}
		out.Days = append(out.Days, day)
	}
	return out, nil
}

// Reschedule moves a visit to a new start within the change cutoff. The
// reservations use case plans it again on the online channel with the same
// services and staff; a taken start changes nothing.
func (s *Service) Reschedule(ctx context.Context, p Principal, id uuid.UUID, in model.RescheduleInput) (Changed, error) {
	o, err := s.load(ctx, p, id)
	if err != nil {
		return Changed{}, err
	}
	if a := o.actions(s.now()).Reschedule; !a.Allowed {
		return Changed{}, &ActionError{Action: "reschedule", Reason: *a.Reason}
	}
	if in.StartsAt == nil {
		return Changed{}, invalid("starts_at", "required", "start is required")
	}
	cutoff := o.settings.Settings.Reservation.ChangeCutoffMinutes
	_, err = s.deps.Reservations.Reschedule(ctx, o.row.OrganizationID, o.row.Uuid, availabilityusecase.Caller{}, 0, reservationsmodel.RescheduleInput{
		StartsAt: in.StartsAt, ExpectedVersion: in.ExpectedVersion, Lines: lineInputs(o.full),
		Channel: availabilitymodel.ChannelOnline, Guard: guard(o, "reschedule", cutoff, s.now),
	})
	if err != nil {
		return Changed{}, err
	}
	return s.reloaded(ctx, p, o)
}
