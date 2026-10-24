package usecase

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// LineSortColumns are the sortable share line list fields.
var LineSortColumns = map[string]string{
	"share_date": "share_date", "share_amount": "share_amount", "base_amount": "base_amount", "created_at": "created_at",
}

// ListLines returns one page of the shares inside the caller's branch
// scope (an own-only viewer sees its own).
func (s *Service) ListLines(ctx context.Context, orgID int64, v Viewer, f model.LineFilter) ([]model.ShareLine, int64, error) {
	if _, ok := LineSortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "", false
	}
	q := s.repo.Q()
	own, err := s.ownStaff(ctx, q, orgID, v)
	if err != nil {
		if errors.Is(err, ErrNoStaffRecord) {
			return []model.ShareLine{}, 0, nil
		}
		return nil, 0, err
	}
	staff, err := s.staffIDs(ctx, q, orgID, f.Staff, own)
	if err != nil {
		return nil, 0, err
	}
	branches, err := s.branchIDs(ctx, q, orgID, f.Branches)
	if err != nil {
		return nil, 0, err
	}
	var collected pgtype.Bool
	switch f.Status {
	case model.StatusPending:
		collected = pgtype.Bool{Bool: false, Valid: true}
	case model.StatusPayable:
		collected = pgtype.Bool{Bool: true, Valid: true}
	}
	scope := orgctx.BranchScope(ctx)
	rows, err := q.ListShareLines(ctx, db.ListShareLinesParams{
		OrganizationID: orgID, BranchIds: scope, FilterBranchIds: branches, StaffIds: staff, Kinds: nilIfEmpty(f.Kinds),
		ItemTypes: nilIfEmpty(f.ItemTypes), Collected: collected, DateFrom: datePtrArg(f.DateFrom), DateTo: datePtrArg(f.DateTo),
		Q: textArg(f.Q), SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountShareLines(ctx, db.CountShareLinesParams{
		OrganizationID: orgID, BranchIds: scope, FilterBranchIds: branches, StaffIds: staff, Kinds: nilIfEmpty(f.Kinds),
		ItemTypes: nilIfEmpty(f.ItemTypes), Collected: collected, DateFrom: datePtrArg(f.DateFrom), DateTo: datePtrArg(f.DateTo),
		Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.ShareLine, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapLine(r))
	}
	return out, total, nil
}

func mapLine(r db.ListShareLinesRow) model.ShareLine {
	l := model.ShareLine{
		UUID: r.Uuid, Kind: r.Kind, ItemType: r.ItemType, Status: model.StatusPending, RuleScope: r.RuleScope,
		RuleUUID: uuidPtr(r.RuleUuid), Basis: r.Basis, Rate: ratePtr(r.Rate), FixedAmount: amountPtr(r.FixedAmount),
		BaseAmount: amountOf(r.BaseAmount), ShareAmount: amountOf(r.ShareAmount), Currency: r.Currency,
		ShareDate: formatDate(r.ShareDate.Time), Branch: model.Ref{UUID: r.BranchUuid, Name: r.BranchName},
		Staff: model.Ref{UUID: r.StaffUuid, Name: r.StaffName}, SaleLineUUID: r.SaleLineUuid, Description: r.Description,
		Quantity: int(r.Quantity), ReversesUUID: uuidPtr(r.ReversesUuid), ReversedByUUID: uuidPtr(r.ReversedByUuid),
		CreatedAt: r.CreatedAt.Time,
	}
	if r.Collected {
		l.Status = model.StatusPayable
	}
	if r.ReservationUuid.Valid {
		l.Reservation = &model.ReservationRef{UUID: uuid.UUID(r.ReservationUuid.Bytes), Number: r.ReservationNumber.Int64}
	}
	if r.CustomerUuid.Valid {
		l.Customer = &model.Ref{
			UUID: uuid.UUID(r.CustomerUuid.Bytes),
			Name: strings.TrimSpace(r.CustomerFirstName.String + " " + r.CustomerLastName.String),
		}
	}
	return l
}

// ------------------------------------------------------------------ summary

// summaryRange applies the defaults (first day of the month to today) and
// the length limit.
func (s *Service) summaryRange(ctx context.Context, from, to *pgtype.Date) error {
	today := s.today(ctx)
	if !to.Valid {
		*to = dateArg(today)
	}
	if !from.Valid {
		*from = dateArg(time1st(to.Time))
	}
	if to.Time.Before(from.Time) {
		return invalid("date_to", "invalid", "date_to must not be before date_from")
	}
	if to.Time.Sub(from.Time).Hours()/24 >= maxSummaryDays {
		return invalid("date_to", "too_long", "the period is at most 366 days")
	}
	return nil
}

// Summary returns the shares per staff member (and, for a viewer of every
// share, per branch) for a period: the period figures inside the caller's
// branch scope and each person's balance.
func (s *Service) Summary(ctx context.Context, orgID int64, v Viewer, f model.SummaryFilter) (model.Summary, error) {
	from, to := datePtrArg(f.DateFrom), datePtrArg(f.DateTo)
	if err := s.summaryRange(ctx, &from, &to); err != nil {
		return model.Summary{}, err
	}
	out := model.Summary{
		DateFrom: formatDate(from.Time), DateTo: formatDate(to.Time),
		Staff: []model.StaffSummary{}, Branches: []model.BranchSummary{}, Totals: []model.Totals{},
	}
	q := s.repo.Q()
	own, err := s.ownStaff(ctx, q, orgID, v)
	if err != nil {
		if errors.Is(err, ErrNoStaffRecord) {
			return out, nil
		}
		return out, err
	}
	staff, err := s.staffIDs(ctx, q, orgID, f.Staff, own)
	if err != nil {
		return out, err
	}
	branches, err := s.branchIDs(ctx, q, orgID, f.Branches)
	if err != nil {
		return out, err
	}
	rows, err := s.staffSummaries(ctx, q, orgID, staff, branches, from, to)
	if err != nil {
		return out, err
	}
	out.Staff = rows
	if own != 0 {
		return out, nil
	}
	btotals, err := q.BranchShareTotals(ctx, db.BranchShareTotalsParams{
		OrganizationID: orgID, DateFrom: from, DateTo: to, BranchIds: orgctx.BranchScope(ctx), FilterBranchIds: branches,
	})
	if err != nil {
		return out, err
	}
	totals := map[string]*model.Totals{}
	total := func(currency string) *model.Totals {
		t, ok := totals[currency]
		if !ok {
			t = &model.Totals{Currency: currency}
			totals[currency] = t
		}
		return t
	}
	for _, b := range btotals {
		revenue, share := amountOf(b.Revenue), amountOf(b.Share)
		out.Branches = append(out.Branches, model.BranchSummary{
			Branch: model.Ref{UUID: b.BranchUuid, Name: b.BranchName}, Currency: b.Currency,
			Revenue: revenue, Share: share, BusinessShare: revenue.Sub(share),
		})
		t := total(b.Currency)
		t.Revenue, t.Share = t.Revenue.Add(revenue), t.Share.Add(share)
		t.BusinessShare = t.Revenue.Sub(t.Share)
	}
	for _, r := range rows {
		t := total(r.Currency)
		t.Pending, t.PeriodPaid = t.Pending.Add(r.Pending), t.PeriodPaid.Add(r.PeriodPaid)
	}
	keys := make([]string, 0, len(totals))
	for k := range totals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out.Totals = append(out.Totals, *totals[k])
	}
	return out, nil
}

// Mine returns the caller's own shares today and in the period (default:
// this month) with the balance; ErrNoStaffRecord when the member has no
// staff record.
func (s *Service) Mine(ctx context.Context, orgID, userID int64, dateFrom, dateTo *pgtype.Date) (model.MyShares, error) {
	q := s.repo.Q()
	st, err := q.GetShareStaffByUser(ctx, db.GetShareStaffByUserParams{OrganizationID: orgID, UserID: int8Of(userID)})
	if err != nil {
		if isNotFound(err) {
			return model.MyShares{}, ErrNoStaffRecord
		}
		return model.MyShares{}, err
	}
	from, to := *dateFrom, *dateTo
	if err := s.summaryRange(ctx, &from, &to); err != nil {
		return model.MyShares{}, err
	}
	today := dateArg(s.today(ctx))
	// The member's own figures over every branch (its own work, wherever
	// it was done).
	all := orgctx.WithScope(ctx, ownScope(ctx))
	daily, err := s.staffSummaries(all, q, orgID, []int64{st.ID}, nil, today, today)
	if err != nil {
		return model.MyShares{}, err
	}
	period, err := s.staffSummaries(all, q, orgID, []int64{st.ID}, nil, from, to)
	if err != nil {
		return model.MyShares{}, err
	}
	return model.MyShares{
		Staff: model.Ref{UUID: st.Uuid, Name: st.Name}, Today: formatDate(today.Time),
		DateFrom: formatDate(from.Time), DateTo: formatDate(to.Time), Daily: daily, Period: period,
	}, nil
}

// ownScope is the caller's scope without a branch restriction.
func ownScope(ctx context.Context) orgctx.Scope {
	scope, _ := orgctx.From(ctx)
	scope.BranchIDs, scope.SelectedBranchID = nil, 0
	return scope
}

// staffSummaries builds the rows of the given staff members (nil = every
// staff member with sales or payouts inside the scope).
func (s *Service) staffSummaries(ctx context.Context, q *db.Queries, orgID int64, staff, branches []int64, from, to pgtype.Date) ([]model.StaffSummary, error) {
	limited, scope := scopeArgs(ctx)
	people, err := q.ListShareSummaryStaff(ctx, db.ListShareSummaryStaffParams{
		OrganizationID: orgID, StaffIds: staff, Limited: limited, BranchIds: scope, FilterBranchIds: branches,
	})
	if err != nil {
		return nil, err
	}
	if len(people) == 0 {
		return []model.StaffSummary{}, nil
	}
	ids := make([]int64, 0, len(people))
	for _, p := range people {
		ids = append(ids, p.ID)
	}
	type key struct {
		staff    int64
		currency string
	}
	rows := map[key]*model.StaffSummary{}
	row := func(staff int64, currency string) *model.StaffSummary {
		k := key{staff, currency}
		r, ok := rows[k]
		if !ok {
			r = &model.StaffSummary{Currency: currency}
			rows[k] = r
		}
		return r
	}
	period, err := q.StaffSharePeriodTotals(ctx, db.StaffSharePeriodTotalsParams{
		OrganizationID: orgID, DateFrom: from, DateTo: to, BranchIds: orgctx.BranchScope(ctx), FilterBranchIds: branches,
		StaffIds: ids,
	})
	if err != nil {
		return nil, err
	}
	for _, p := range period {
		r := row(p.StaffID.Int64, p.Currency)
		r.Sales, r.Share, r.Pending, r.SaleCount = amountOf(p.Sales), amountOf(p.Share), amountOf(p.Pending), p.SaleCount
	}
	balances, err := q.StaffShareBalances(ctx, db.StaffShareBalancesParams{OrganizationID: orgID, StaffIds: ids})
	if err != nil {
		return nil, err
	}
	for _, b := range balances {
		r := row(b.StaffID, b.Currency)
		r.Balance.Earned, r.Balance.Payable = amountOf(b.Earned), amountOf(b.Payable)
	}
	paid, err := q.StaffPayoutTotals(ctx, db.StaffPayoutTotalsParams{
		OrganizationID: orgID, StaffIds: ids, DateFrom: from, DateTo: to, Limited: limited, BranchIds: scope,
		FilterBranchIds: branches,
	})
	if err != nil {
		return nil, err
	}
	for _, p := range paid {
		r := row(p.StaffID, p.Currency)
		r.Balance.Paid, r.PeriodPaid = amountOf(p.Paid), amountOf(p.PeriodPaid)
	}
	out := []model.StaffSummary{}
	for _, p := range people {
		var mine []*model.StaffSummary
		for k, r := range rows {
			if k.staff == p.ID {
				mine = append(mine, r)
			}
		}
		sort.Slice(mine, func(i, j int) bool { return mine[i].Currency < mine[j].Currency })
		for _, r := range mine {
			r.Staff = model.Ref{UUID: p.Uuid, Name: p.Name}
			r.Payable = r.Share.Sub(r.Pending)
			r.BusinessShare = r.Sales.Sub(r.Share)
			r.Balance.Remaining = r.Balance.Earned.Sub(r.Balance.Paid)
			r.Balance.PayableNow = r.Balance.Payable.Sub(r.Balance.Paid)
			out = append(out, *r)
		}
	}
	return out, nil
}

// payableNow is what can be paid to a staff member in a currency now.
func payableNow(ctx context.Context, q *db.Queries, orgID, staffID int64, currency string) (money.Money, error) {
	balances, err := q.StaffShareBalances(ctx, db.StaffShareBalancesParams{OrganizationID: orgID, StaffIds: []int64{staffID}})
	if err != nil {
		return money.Money{}, err
	}
	out := money.Zero()
	for _, b := range balances {
		if b.Currency == currency {
			out = amountOf(b.Payable)
		}
	}
	paid, err := q.StaffPayoutTotals(ctx, db.StaffPayoutTotalsParams{
		OrganizationID: orgID, StaffIds: []int64{staffID}, DateFrom: dateArg(time0), DateTo: dateArg(time0), BranchIds: []int64{},
	})
	if err != nil {
		return money.Money{}, err
	}
	for _, p := range paid {
		if p.Currency == currency {
			out = out.Sub(amountOf(p.Paid))
		}
	}
	return out, nil
}
