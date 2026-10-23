package usecase

import (
	"context"
	"sort"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Reservation statuses whose booked total is still expected.
var standing = []string{"requested", "active", "in_service"}

func totalsOf(r db.LedgerTotalsRow) model.Totals {
	t := model.Totals{
		Currency: r.Currency, Charged: amountOf(r.Charged), Collected: amountOf(r.Collected), Refunded: amountOf(r.Refunded),
	}
	t.Paid = t.Collected.Sub(t.Refunded)
	t.Balance = t.Charged.Sub(t.Paid)
	return t
}

// ReservationBalance is what a visit costs and what was paid for it, with
// its sale lines and ledger entries.
func (s *Service) ReservationBalance(ctx context.Context, orgID int64, id uuid.UUID) (model.ReservationBalance, error) {
	q := s.repo.Q()
	r, err := q.GetSalesReservationByUUID(ctx, db.GetSalesReservationByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if isNotFound(err) {
			return model.ReservationBalance{}, ErrReservationMissing
		}
		return model.ReservationBalance{}, err
	}
	if orgctx.CheckBranch(ctx, r.BranchID) != nil {
		return model.ReservationBalance{}, ErrReservationMissing
	}
	out := model.ReservationBalance{
		Reservation: model.ReservationRef{UUID: r.Uuid, Number: r.Number}, Status: r.Status, Currency: r.Currency,
		BookedTotal: amountOf(r.TotalPrice), Lines: []model.SaleLine{}, Entries: []model.LedgerEntry{},
	}
	posted, err := q.CountReservationSaleLines(ctx, db.CountReservationSaleLinesParams{OrganizationID: orgID, ReservationID: int8Of(r.ID)})
	if err != nil {
		return out, err
	}
	out.SalesPosted = posted > 0
	totals, err := q.LedgerTotals(ctx, db.LedgerTotalsParams{OrganizationID: orgID, ReservationID: int8Of(r.ID)})
	if err != nil {
		return out, err
	}
	var t model.Totals
	for _, row := range totals {
		if row.Currency == r.Currency {
			t = totalsOf(row)
		}
	}
	switch {
	case out.SalesPosted:
		out.Expected = t.Charged
	case contains(standing, r.Status):
		out.Expected = out.BookedTotal
	}
	out.Collected, out.Refunded, out.Paid = t.Collected, t.Refunded, t.Paid
	diff := out.Expected.Sub(out.Paid)
	out.Remaining = diff.ClampZero()
	out.Credit = diff.Neg().ClampZero()
	resUUID := r.Uuid
	lines, _, err := s.ListLines(ctx, orgID, model.LineFilter{ReservationUUID: &resUUID, SortField: "sale_date", Limit: 200})
	if err != nil {
		return out, err
	}
	out.Lines = lines
	entries, _, err := s.ListEntries(ctx, orgID, model.EntryFilter{ReservationUUID: &resUUID, SortField: "entry_date", Limit: 200})
	if err != nil {
		return out, err
	}
	out.Entries = entries
	return out, nil
}

// CustomerBalance is the ledger of one customer per currency (inside the
// caller's branch scope).
func (s *Service) CustomerBalance(ctx context.Context, orgID int64, id uuid.UUID) (model.CustomerBalance, error) {
	q := s.repo.Q()
	c, err := q.GetSalesCustomerByUUID(ctx, db.GetSalesCustomerByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if isNotFound(err) {
			return model.CustomerBalance{}, ErrCustomerMissing
		}
		return model.CustomerBalance{}, err
	}
	rows, err := q.LedgerTotals(ctx, db.LedgerTotalsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), CustomerID: int8Of(c.ID),
	})
	if err != nil {
		return model.CustomerBalance{}, err
	}
	out := model.CustomerBalance{Customer: c.Uuid, Totals: []model.Totals{}}
	for _, r := range rows {
		out.Totals = append(out.Totals, totalsOf(r))
	}
	return out, nil
}

// Summary returns the figures of a date range (default: today) per
// currency and per day: revenue from the sale lines, collections and
// refunds from the ledger, transfers and staff payouts from the cash book.
func (s *Service) Summary(ctx context.Context, orgID int64, from, to *time.Time, branches []uuid.UUID) (model.Summary, error) {
	today := s.today(ctx)
	df, dt := today, today
	if from != nil {
		df = *from
	}
	if to != nil {
		dt = *to
	} else if from != nil {
		dt = df
	}
	if dt.Before(df) {
		return model.Summary{}, invalid("date_to", "invalid", "date_to must not be before date_from")
	}
	if dt.Sub(df) > maxSummaryDays*24*time.Hour {
		return model.Summary{}, invalid("date_to", "too_long", "the range is at most 366 days")
	}
	q := s.repo.Q()
	a, err := s.resolveFilter(ctx, q, orgID, branches, nil, nil, nil)
	if err != nil {
		return model.Summary{}, err
	}
	scope := orgctx.BranchScope(ctx)
	out := model.Summary{DateFrom: formatDate(df), DateTo: formatDate(dt), Totals: []model.Figures{}, Days: []model.Day{}}

	type key struct{ day, currency string }
	figs := map[key]*model.Figures{}
	at := func(d pgtype.Date, currency string) *model.Figures {
		k := key{formatDate(d.Time), currency}
		f := figs[k]
		if f == nil {
			f = &model.Figures{Currency: currency}
			zero(f)
			figs[k] = f
		}
		return f
	}
	rev, err := q.SalesRevenueByDay(ctx, db.SalesRevenueByDayParams{
		OrganizationID: orgID, DateFrom: dateArg(df), DateTo: dateArg(dt), BranchIds: scope, FilterBranchIds: a.branches,
	})
	if err != nil {
		return out, err
	}
	for _, r := range rev {
		f := at(r.SaleDate, r.Currency)
		f.Revenue = model.Revenue{
			Gross: amountOf(r.Gross), Discount: amountOf(r.Discount), Reversed: amountOf(r.Reversed), Net: amountOf(r.Net),
			ServiceNet: amountOf(r.ServiceNet), ProductNet: amountOf(r.ProductNet), SaleCount: r.SaleCount,
		}
	}
	col, err := q.SalesCollectionsByDay(ctx, db.SalesCollectionsByDayParams{
		OrganizationID: orgID, DateFrom: dateArg(df), DateTo: dateArg(dt), BranchIds: scope, FilterBranchIds: a.branches,
	})
	if err != nil {
		return out, err
	}
	for _, r := range col {
		f := at(r.EntryDate, r.Currency)
		c := model.Collections{
			Collected: amountOf(r.Collected), Refunded: amountOf(r.Refunded), Reversed: amountOf(r.Reversed),
			Count: r.CollectionCount,
		}
		c.Net = c.Collected.Sub(c.Refunded).Add(c.Reversed)
		f.Collections = c
	}
	cash, err := q.SalesCashByDay(ctx, db.SalesCashByDayParams{
		OrganizationID: orgID, DateFrom: dateArg(df), DateTo: dateArg(dt), BranchIds: scope, FilterBranchIds: a.branches,
	})
	if err != nil {
		return out, err
	}
	for _, r := range cash {
		f := at(r.MovementDate, r.Currency)
		f.Transfers, f.StaffPayments = amountOf(r.Transfers), amountOf(r.StaffPayments)
	}

	keys := make([]key, 0, len(figs))
	for k := range figs {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].day != keys[j].day {
			return keys[i].day < keys[j].day
		}
		return keys[i].currency < keys[j].currency
	})
	totals := map[string]*model.Figures{}
	var currencies []string
	for _, k := range keys {
		f := *figs[k]
		if n := len(out.Days); n == 0 || out.Days[n-1].Date != k.day {
			out.Days = append(out.Days, model.Day{Date: k.day, Figures: []model.Figures{}})
		}
		d := &out.Days[len(out.Days)-1]
		d.Figures = append(d.Figures, f)
		t := totals[k.currency]
		if t == nil {
			t = &model.Figures{Currency: k.currency}
			zero(t)
			totals[k.currency] = t
			currencies = append(currencies, k.currency)
		}
		add(t, f)
	}
	sort.Strings(currencies)
	for _, c := range currencies {
		out.Totals = append(out.Totals, *totals[c])
	}
	if len(out.Totals) == 0 {
		cur := "TRY"
		if sc, ok := orgctx.From(ctx); ok && sc.Currency != "" {
			cur = sc.Currency
		}
		f := model.Figures{Currency: cur}
		zero(&f)
		out.Totals = append(out.Totals, f)
	}
	return out, nil
}

func zero(f *model.Figures) {
	z := money.Zero()
	f.Revenue = model.Revenue{Gross: z, Discount: z, Reversed: z, Net: z, ServiceNet: z, ProductNet: z}
	f.Collections = model.Collections{Collected: z, Refunded: z, Reversed: z, Net: z}
	f.Transfers, f.StaffPayments = z, z
}

func add(t *model.Figures, f model.Figures) {
	t.Revenue.Gross = t.Revenue.Gross.Add(f.Revenue.Gross)
	t.Revenue.Discount = t.Revenue.Discount.Add(f.Revenue.Discount)
	t.Revenue.Reversed = t.Revenue.Reversed.Add(f.Revenue.Reversed)
	t.Revenue.Net = t.Revenue.Net.Add(f.Revenue.Net)
	t.Revenue.ServiceNet = t.Revenue.ServiceNet.Add(f.Revenue.ServiceNet)
	t.Revenue.ProductNet = t.Revenue.ProductNet.Add(f.Revenue.ProductNet)
	t.Revenue.SaleCount += f.Revenue.SaleCount
	t.Collections.Collected = t.Collections.Collected.Add(f.Collections.Collected)
	t.Collections.Refunded = t.Collections.Refunded.Add(f.Collections.Refunded)
	t.Collections.Reversed = t.Collections.Reversed.Add(f.Collections.Reversed)
	t.Collections.Net = t.Collections.Net.Add(f.Collections.Net)
	t.Collections.Count += f.Collections.Count
	t.Transfers = t.Transfers.Add(f.Transfers)
	t.StaffPayments = t.StaffPayments.Add(f.StaffPayments)
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
