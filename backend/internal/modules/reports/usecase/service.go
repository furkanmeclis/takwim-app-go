// Package usecase builds the reports of a business (TEC-925) from the
// records of the other modules: reservations and confirmation tasks, sale
// lines (revenue), the customer ledger (collections), the cash book
// (transfers, other income and expenses), staff shares and payouts, and
// the stock. Nothing is stored; every figure comes from its own record,
// so revenue, collections, transfers and staff payouts never mix (A20).
package usecase

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// MaxDays is the longest range of one report.
const MaxDays = 366

const dateLayout = "2006-01-02"

// Detail is one field problem of a ValidationError.
type Detail struct {
	Field   string
	Code    string
	Message string
}

// ValidationError is a bad filter.
type ValidationError struct {
	Details []Detail
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Details) == 0 {
		return "validation failed"
	}
	return e.Details[0].Field + ": " + e.Details[0].Message
}

func invalid(field, code, msg string) error {
	return &ValidationError{Details: []Detail{{Field: field, Code: code, Message: msg}}}
}

// IsValidation reports whether err is a filter error.
func IsValidation(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}

// Queries are the report reads (db.Queries).
type Queries interface {
	ListReportBranches(ctx context.Context, organizationID int64) ([]db.ListReportBranchesRow, error)
	GetStaffIDsByUUIDs(ctx context.Context, arg db.GetStaffIDsByUUIDsParams) ([]db.GetStaffIDsByUUIDsRow, error)
	ReportRevenue(ctx context.Context, arg db.ReportRevenueParams) ([]db.ReportRevenueRow, error)
	ReportCollections(ctx context.Context, arg db.ReportCollectionsParams) ([]db.ReportCollectionsRow, error)
	ReportCashBook(ctx context.Context, arg db.ReportCashBookParams) ([]db.ReportCashBookRow, error)
	ReportStaffShares(ctx context.Context, arg db.ReportStaffSharesParams) ([]db.ReportStaffSharesRow, error)
	ReportStaffPayouts(ctx context.Context, arg db.ReportStaffPayoutsParams) ([]db.ReportStaffPayoutsRow, error)
	ReportReservations(ctx context.Context, arg db.ReportReservationsParams) ([]db.ReportReservationsRow, error)
	ReportConfirmations(ctx context.Context, arg db.ReportConfirmationsParams) ([]db.ReportConfirmationsRow, error)
	ReportStockMovements(ctx context.Context, arg db.ReportStockMovementsParams) ([]db.ReportStockMovementsRow, error)
	ReportStockLevels(ctx context.Context, arg db.ReportStockLevelsParams) ([]db.ReportStockLevelsRow, error)
	// Daily summary (TEC-926).
	GetAgendaStaff(ctx context.Context, arg db.GetAgendaStaffParams) (db.GetAgendaStaffRow, error)
	ListDailyStaffLines(ctx context.Context, arg db.ListDailyStaffLinesParams) ([]db.ListDailyStaffLinesRow, error)
	CountPendingApprovals(ctx context.Context, arg db.CountPendingApprovalsParams) (int64, error)
}

// Service builds reports.
type Service struct {
	q   Queries
	now func() time.Time
}

// New returns a Service.
func New(q Queries) *Service {
	return &Service{q: q, now: time.Now}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

// args are the resolved filter of one report.
type args struct {
	orgID      int64
	from, to   pgtype.Date
	scope      []int64
	branches   []int64 // nil = no branch filter
	headOffice bool    // business-wide records (only without a filter, or when asked)
	staff      []int64 // nil = no staff filter
	empty      bool    // the filter names nothing of this business
}

// Range resolves the dates of a filter: both default to today in the
// business zone, a lone date_from is a one-day report.
func (s *Service) Range(ctx context.Context, f model.Filter) (time.Time, time.Time, error) {
	today := s.today(ctx)
	df, dt := today, today
	if f.DateFrom != nil {
		df = *f.DateFrom
	}
	if f.DateTo != nil {
		dt = *f.DateTo
	} else if f.DateFrom != nil {
		dt = df
	}
	if dt.Before(df) {
		return df, dt, invalid("date_to", "invalid", "date_to must not be before date_from")
	}
	if dt.Sub(df) > (MaxDays-1)*24*time.Hour {
		return df, dt, invalid("date_to", "too_long", "the range is at most 366 days")
	}
	return df, dt, nil
}

// Summary builds the report of a filter: totals, one row per day with any
// record, and the branch comparison.
func (s *Service) Summary(ctx context.Context, orgID int64, f model.Filter) (model.Report, error) {
	df, dt, err := s.Range(ctx, f)
	if err != nil {
		return model.Report{}, err
	}
	sc, _ := orgctx.From(ctx)
	currency := sc.Currency
	if currency == "" {
		currency = "TRY"
	}
	tz := sc.Timezone
	if tz == "" {
		tz = "Europe/Istanbul"
	}

	branchRows, err := s.q.ListReportBranches(ctx, orgID)
	if err != nil {
		return model.Report{}, err
	}
	a := args{orgID: orgID, from: dateArg(df), to: dateArg(dt), scope: orgctx.BranchScope(ctx)}
	// Only the filter values of this business are echoed back.
	pickedBranches, pickedStaff := []uuid.UUID{}, []uuid.UUID{}
	allBranches := a.scope == nil
	if len(f.Branches) > 0 || f.HeadOffice {
		a.branches = []int64{}
		for _, b := range branchRows {
			for _, id := range f.Branches {
				if b.Uuid == id {
					a.branches = append(a.branches, b.ID)
					pickedBranches = append(pickedBranches, b.Uuid)
				}
			}
		}
		a.headOffice = f.HeadOffice
		if len(a.branches) == 0 && !a.headOffice {
			a.empty = true
		}
	} else {
		a.headOffice = true
	}
	if len(f.Staff) > 0 {
		rows, err := s.q.GetStaffIDsByUUIDs(ctx, db.GetStaffIDsByUUIDsParams{OrganizationID: orgID, Uuids: f.Staff})
		if err != nil {
			return model.Report{}, err
		}
		a.staff = []int64{}
		for _, r := range rows {
			a.staff = append(a.staff, r.ID)
			pickedStaff = append(pickedStaff, r.Uuid)
		}
		if len(a.staff) == 0 {
			a.empty = true
		}
	}

	out := model.Report{
		DateFrom: df.Format(dateLayout), DateTo: dt.Format(dateLayout),
		Scope: model.Scope{
			Timezone: tz, Currency: currency, Branches: pickedBranches, HeadOffice: a.headOffice && allBranches,
			Staff: pickedStaff, OmittedForStaff: []string{},
		},
		Days: []model.Day{}, Branches: []model.Branch{},
	}
	if a.staff != nil {
		out.Scope.OmittedForStaff = []string{model.FieldCollections, model.FieldTransfers, model.FieldIncome, model.FieldExpenses}
	}

	acc := newAccumulator(currency)
	if !a.empty {
		if err := s.collect(ctx, a, acc); err != nil {
			return model.Report{}, err
		}
	}

	// Branch comparison: every branch of the scope and filter (an inactive
	// one only when it has figures), then the business-wide row when it
	// has any.
	inScope := func(id int64) bool {
		if a.scope != nil && !contains(a.scope, id) {
			return false
		}
		return a.branches == nil || contains(a.branches, id)
	}
	for _, b := range branchRows {
		if !inScope(b.ID) {
			continue
		}
		bucket, has := acc.branches[b.ID]
		if !has && !b.IsActive {
			continue
		}
		if bucket == nil {
			bucket = acc.newBucket()
		}
		out.Branches = append(out.Branches, model.Branch{
			Branch:  &model.BranchRef{UUID: b.Uuid, Code: b.Code, Name: b.Name, IsActive: b.IsActive},
			Figures: bucket.figures(acc.currency),
			Stock:   bucket.stock,
		})
	}
	if bucket, ok := acc.branches[0]; ok {
		out.Branches = append(out.Branches, model.Branch{Figures: bucket.figures(acc.currency), Stock: bucket.stock})
	}

	days := make([]string, 0, len(acc.days))
	for d := range acc.days {
		days = append(days, d)
	}
	sort.Strings(days)
	for _, d := range days {
		out.Days = append(out.Days, model.Day{Date: d, Figures: acc.days[d].figures(acc.currency)})
	}
	out.Totals = model.Totals{Figures: acc.total.figures(acc.currency), Stock: acc.total.stock}
	return out, nil
}

// collect runs every query of the report into acc.
func (s *Service) collect(ctx context.Context, a args, acc *accumulator) error {
	staffFiltered := a.staff != nil
	day := func(d pgtype.Date) string { return d.Time.Format(dateLayout) }
	branchOf := func(b pgtype.Int8) int64 {
		if b.Valid {
			return b.Int64
		}
		return 0
	}

	rev, err := s.q.ReportRevenue(ctx, db.ReportRevenueParams{
		OrganizationID: a.orgID, DateFrom: a.from, DateTo: a.to, BranchIds: a.scope, FilterBranchIds: a.branches, StaffIds: a.staff,
	})
	if err != nil {
		return err
	}
	for _, r := range rev {
		acc.money(day(r.SaleDate), r.BranchID, r.Currency, func(m *model.Money) {
			m.Revenue.Gross = m.Revenue.Gross.Add(amount(r.Gross))
			m.Revenue.Discount = m.Revenue.Discount.Add(amount(r.Discount))
			m.Revenue.Reversed = m.Revenue.Reversed.Add(amount(r.Reversed))
			m.Revenue.Net = m.Revenue.Net.Add(amount(r.Net))
			m.Revenue.ServiceNet = m.Revenue.ServiceNet.Add(amount(r.ServiceNet))
			m.Revenue.ProductNet = m.Revenue.ProductNet.Add(amount(r.ProductNet))
			m.Revenue.SaleCount += r.SaleCount
			m.ProductCost = m.ProductCost.Add(amount(r.ProductCost))
		})
	}

	shares, err := s.q.ReportStaffShares(ctx, db.ReportStaffSharesParams{
		OrganizationID: a.orgID, DateFrom: a.from, DateTo: a.to, BranchIds: a.scope, FilterBranchIds: a.branches, StaffIds: a.staff,
	})
	if err != nil {
		return err
	}
	for _, r := range shares {
		acc.money(day(r.ShareDate), r.BranchID, r.Currency, func(m *model.Money) {
			m.StaffShare = m.StaffShare.Add(amount(r.Share))
		})
	}

	payouts, err := s.q.ReportStaffPayouts(ctx, db.ReportStaffPayoutsParams{
		OrganizationID: a.orgID, DateFrom: a.from, DateTo: a.to, BranchIds: a.scope, FilterBranchIds: a.branches,
		HeadOffice: a.headOffice, StaffIds: a.staff,
	})
	if err != nil {
		return err
	}
	for _, r := range payouts {
		acc.money(day(r.PayoutDate), branchOf(r.BranchID), r.Currency, func(m *model.Money) {
			m.StaffPayments = m.StaffPayments.Add(amount(r.Paid))
		})
	}

	// Money not tied to a staff member stays out of a staff report.
	if !staffFiltered {
		col, err := s.q.ReportCollections(ctx, db.ReportCollectionsParams{
			OrganizationID: a.orgID, DateFrom: a.from, DateTo: a.to, BranchIds: a.scope, FilterBranchIds: a.branches,
		})
		if err != nil {
			return err
		}
		for _, r := range col {
			acc.money(day(r.EntryDate), r.BranchID, r.Currency, func(m *model.Money) {
				m.Collections.Collected = m.Collections.Collected.Add(amount(r.Collected))
				m.Collections.Refunded = m.Collections.Refunded.Add(amount(r.Refunded))
				m.Collections.Reversed = m.Collections.Reversed.Add(amount(r.Reversed))
				m.Collections.Count += r.CollectionCount
			})
		}
		cash, err := s.q.ReportCashBook(ctx, db.ReportCashBookParams{
			OrganizationID: a.orgID, DateFrom: a.from, DateTo: a.to, BranchIds: a.scope, FilterBranchIds: a.branches,
			HeadOffice: a.headOffice,
		})
		if err != nil {
			return err
		}
		for _, r := range cash {
			acc.money(day(r.MovementDate), branchOf(r.BranchID), r.Currency, func(m *model.Money) {
				m.Transfers = m.Transfers.Add(amount(r.Transfers))
				m.Income = m.Income.Add(amount(r.Income))
				m.Expenses = m.Expenses.Add(amount(r.Expenses))
			})
		}
	}

	res, err := s.q.ReportReservations(ctx, db.ReportReservationsParams{
		OrganizationID: a.orgID, DateFrom: a.from, DateTo: a.to, BranchIds: a.scope, FilterBranchIds: a.branches, StaffIds: a.staff,
	})
	if err != nil {
		return err
	}
	for _, r := range res {
		acc.counts(day(r.Day), r.BranchID, func(b *bucket) { addReservation(&b.reservations, r.Status, r.Source, r.Count) })
	}

	conf, err := s.q.ReportConfirmations(ctx, db.ReportConfirmationsParams{
		OrganizationID: a.orgID, DateFrom: a.from, DateTo: a.to, BranchIds: a.scope, FilterBranchIds: a.branches, StaffIds: a.staff,
	})
	if err != nil {
		return err
	}
	for _, r := range conf {
		acc.counts(day(r.Day), r.BranchID, func(b *bucket) { addConfirmation(&b.confirmations, r.State, r.Count) })
	}

	moves, err := s.q.ReportStockMovements(ctx, db.ReportStockMovementsParams{
		OrganizationID: a.orgID, DateFrom: a.from, DateTo: a.to, BranchIds: a.scope, FilterBranchIds: a.branches,
	})
	if err != nil {
		return err
	}
	for _, r := range moves {
		acc.stock(r.BranchID, func(st *model.Stock) { addStockMovement(st, r.Kind, r.Quantity, r.MovementCount) })
	}
	levels, err := s.q.ReportStockLevels(ctx, db.ReportStockLevelsParams{
		OrganizationID: a.orgID, BranchIds: a.scope, FilterBranchIds: a.branches,
	})
	if err != nil {
		return err
	}
	for _, r := range levels {
		acc.stock(r.BranchID, func(st *model.Stock) {
			st.OnHand += r.OnHand
			st.Reserved += r.Reserved
			st.Items += r.ItemCount
			st.LowItems += r.LowCount
			st.OutItems += r.OutCount
			st.Value = st.Value.Add(amount(r.StockValue))
		})
	}
	return nil
}

// ------------------------------------------------------------ accumulator

type bucket struct {
	reservations  model.Reservations
	confirmations model.Confirmations
	money         map[string]*model.Money
	stock         model.Stock
}

type accumulator struct {
	currency string
	total    *bucket
	days     map[string]*bucket
	branches map[int64]*bucket // 0 = business-wide
}

func newAccumulator(currency string) *accumulator {
	acc := &accumulator{currency: currency, days: map[string]*bucket{}, branches: map[int64]*bucket{}}
	acc.total = acc.newBucket()
	return acc
}

func (acc *accumulator) newBucket() *bucket {
	return &bucket{money: map[string]*model.Money{}, stock: model.Stock{Value: money.Zero(), Currency: acc.currency}}
}

func (acc *accumulator) dayBucket(d string) *bucket {
	b := acc.days[d]
	if b == nil {
		b = acc.newBucket()
		acc.days[d] = b
	}
	return b
}

func (acc *accumulator) branchBucket(id int64) *bucket {
	b := acc.branches[id]
	if b == nil {
		b = acc.newBucket()
		acc.branches[id] = b
	}
	return b
}

func (b *bucket) moneyOf(currency string) *model.Money {
	m := b.money[currency]
	if m == nil {
		m = zeroMoney(currency)
		b.money[currency] = m
	}
	return m
}

// money applies fn to the day, the branch and the total.
func (acc *accumulator) money(d string, branch int64, currency string, fn func(*model.Money)) {
	fn(acc.dayBucket(d).moneyOf(currency))
	fn(acc.branchBucket(branch).moneyOf(currency))
	fn(acc.total.moneyOf(currency))
}

func (acc *accumulator) counts(d string, branch int64, fn func(*bucket)) {
	fn(acc.dayBucket(d))
	fn(acc.branchBucket(branch))
	fn(acc.total)
}

// stock is per branch and total only (not per day).
func (acc *accumulator) stock(branch int64, fn func(*model.Stock)) {
	fn(&acc.branchBucket(branch).stock)
	fn(&acc.total.stock)
}

// figures finishes a bucket: the derived amounts and one money entry per
// currency (the business currency first, always present).
func (b *bucket) figures(currency string) model.Figures {
	b.moneyOf(currency)
	keys := make([]string, 0, len(b.money))
	for k := range b.money {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if (keys[i] == currency) != (keys[j] == currency) {
			return keys[i] == currency
		}
		return keys[i] < keys[j]
	})
	out := model.Figures{Reservations: b.reservations, Confirmations: b.confirmations, Money: make([]model.Money, 0, len(keys))}
	for _, k := range keys {
		m := *b.money[k]
		m.Collections.Net = m.Collections.Collected.Sub(m.Collections.Refunded).Add(m.Collections.Reversed)
		m.BusinessShare = m.Revenue.Net.Sub(m.StaffShare)
		m.OperationalResult = m.Revenue.Net.Sub(m.ProductCost).Sub(m.StaffShare).Sub(m.Expenses)
		out.Money = append(out.Money, m)
	}
	return out
}

func zeroMoney(currency string) *model.Money {
	z := money.Zero()
	return &model.Money{
		Currency:    currency,
		Revenue:     model.Revenue{Gross: z, Discount: z, Reversed: z, Net: z, ServiceNet: z, ProductNet: z},
		Collections: model.Collections{Collected: z, Refunded: z, Reversed: z, Net: z},
		Transfers:   z, StaffShare: z, StaffPayments: z, BusinessShare: z, ProductCost: z,
		Income: z, Expenses: z, OperationalResult: z,
	}
}

func addReservation(r *model.Reservations, status, source string, n int64) {
	r.Total += n
	switch status {
	case "requested":
		r.Requested += n
	case "active":
		r.Active += n
	case "in_service":
		r.InService += n
	case "completed":
		r.Completed += n
	case "cancelled":
		r.Cancelled += n
	case "rejected":
		r.Rejected += n
	case "no_show":
		r.NoShow += n
	}
	switch source {
	case "panel":
		r.FromPanel += n
	case "online":
		r.FromOnline += n
	case "embed":
		r.FromEmbed += n
	}
}

func addConfirmation(c *model.Confirmations, state string, n int64) {
	c.Total += n
	switch state {
	case "scheduled":
		c.Scheduled += n
	case "pending":
		c.Pending += n
	case "confirmed":
		c.Confirmed += n
	case "declined":
		c.Declined += n
	case "change_requested":
		c.ChangeRequested += n
	case "cancelled":
		c.Cancelled += n
	case "flagged":
		c.Flagged += n
	case "unreachable":
		c.Unreachable += n
	case "expired":
		c.Expired += n
	case "dropped":
		c.Dropped += n
	}
}

// addStockMovement adds the units of one kind (sales and transfers out are
// negative in the ledger; reported as positive units).
func addStockMovement(st *model.Stock, kind string, qty, count int64) {
	st.Movements += count
	switch kind {
	case "receipt":
		st.Received += qty
	case "sale":
		st.Sold -= qty
	case "return":
		st.Returned += qty
	case "adjustment":
		st.Adjusted += qty
	case "transfer_in":
		st.TransferredIn += qty
	case "transfer_out":
		st.TransferredOut -= qty
	}
}

// ---------------------------------------------------------------- helpers

func (s *Service) today(ctx context.Context) time.Time {
	loc := time.UTC
	if sc, ok := orgctx.From(ctx); ok && sc.Timezone != "" {
		if l, err := time.LoadLocation(sc.Timezone); err == nil {
			loc = l
		}
	}
	y, m, d := s.now().In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func dateArg(t time.Time) pgtype.Date {
	return pgtype.Date{Time: t, Valid: true}
}

func amount(n pgtype.Numeric) money.Money {
	return money.FromNumericOrZero(n)
}

func contains(list []int64, v int64) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
