package usecase

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// LedgerSortColumns are the sortable list fields.
var LedgerSortColumns = map[string]string{
	"entry_date": "entry_date", "amount": "amount", "created_at": "created_at",
}

type ledgerArgs struct {
	branches   []int64
	headOffice bool
	categories []int64
	itemTypes  []string
	staff      []int64
	accounts   []int64
}

// ledgerArgs resolves the uuid filters inside the organization. Unknown
// uuids match nothing (an empty list), never another organization's row.
func (s *Service) ledgerArgs(ctx context.Context, q *db.Queries, orgID int64, f model.LedgerFilter) (ledgerArgs, error) {
	a := ledgerArgs{headOffice: f.HeadOffice}
	if len(f.Branches) > 0 {
		rows, err := q.ListBranchIDsByUUIDs(ctx, db.ListBranchIDsByUUIDsParams{OrganizationID: orgID, Uuids: f.Branches})
		if err != nil {
			return a, err
		}
		a.branches = []int64{}
		for _, r := range rows {
			a.branches = append(a.branches, r.ID)
		}
	}
	if len(f.Categories) > 0 {
		var ids []uuid.UUID
		for _, c := range f.Categories {
			switch c {
			case model.SaleServiceCategory:
				a.itemTypes = append(a.itemTypes, model.ItemService)
			case model.SaleProductCategory:
				a.itemTypes = append(a.itemTypes, model.ItemProduct)
			default:
				if id, err := uuid.Parse(c); err == nil {
					ids = append(ids, id)
				}
			}
		}
		a.categories = []int64{}
		if len(ids) > 0 {
			rows, err := q.ListFinanceCategoryIDsByUUIDs(ctx, db.ListFinanceCategoryIDsByUUIDsParams{OrganizationID: orgID, Uuids: ids})
			if err != nil {
				return a, err
			}
			for _, r := range rows {
				a.categories = append(a.categories, r.ID)
			}
		}
		if a.itemTypes == nil {
			a.itemTypes = []string{}
		}
	}
	if len(f.Staff) > 0 {
		rows, err := q.ListFinanceStaffIDsByUUIDs(ctx, db.ListFinanceStaffIDsByUUIDsParams{OrganizationID: orgID, Uuids: f.Staff})
		if err != nil {
			return a, err
		}
		a.staff = []int64{}
		for _, r := range rows {
			a.staff = append(a.staff, r.ID)
		}
	}
	if len(f.Accounts) > 0 {
		rows, err := q.ListFinanceAccountIDsByUUIDs(ctx, db.ListFinanceAccountIDsByUUIDsParams{OrganizationID: orgID, Uuids: f.Accounts})
		if err != nil {
			return a, err
		}
		a.accounts = []int64{}
		for _, r := range rows {
			if allowsBranch(ctx, r.BranchID) {
				a.accounts = append(a.accounts, r.ID)
			}
		}
	}
	return a, nil
}

// ListLedger returns one page of the income / expense list: the
// business's records and the sale lines (automatic income), inside the
// caller's branch scope.
func (s *Service) ListLedger(ctx context.Context, orgID int64, f model.LedgerFilter) ([]model.LedgerRow, int64, error) {
	if _, ok := LedgerSortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "", false
	}
	q := s.repo.Q()
	a, err := s.ledgerArgs(ctx, q, orgID, f)
	if err != nil {
		return nil, 0, err
	}
	scope := orgctx.BranchScope(ctx)
	rows, err := q.ListFinanceLedger(ctx, db.ListFinanceLedgerParams{
		OrganizationID: orgID, BranchIds: scope, FilterBranchIds: a.branches, FilterHeadOffice: a.headOffice,
		Sources: nilIfEmpty(f.Sources), Kinds: nilIfEmpty(f.Kinds), CategoryIds: a.categories, ItemTypes: a.itemTypes,
		StaffIds: a.staff, AccountIds: a.accounts, DateFrom: datePtrArg(f.DateFrom), DateTo: datePtrArg(f.DateTo),
		Q: textArg(f.Q), SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountFinanceLedger(ctx, db.CountFinanceLedgerParams{
		OrganizationID: orgID, BranchIds: scope, FilterBranchIds: a.branches, FilterHeadOffice: a.headOffice,
		Sources: nilIfEmpty(f.Sources), Kinds: nilIfEmpty(f.Kinds), CategoryIds: a.categories, ItemTypes: a.itemTypes,
		StaffIds: a.staff, AccountIds: a.accounts, DateFrom: datePtrArg(f.DateFrom), DateTo: datePtrArg(f.DateTo),
		Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.LedgerRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapLedgerRow(r))
	}
	return out, total, nil
}

// ExportLedger returns every row matching a filter.
func (s *Service) ExportLedger(ctx context.Context, orgID int64, f model.LedgerFilter) ([]model.LedgerRow, error) {
	f.Limit, f.Offset = 100000, 0
	items, _, err := s.ListLedger(ctx, orgID, f)
	return items, err
}

// Facets returns the categories (with record counts), the branches and the
// staff members in use, inside the caller's scope.
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	out := model.Facets{Category: []model.Facet{}, Branch: []model.Facet{}, Staff: []model.Facet{}}
	q := s.repo.Q()
	scope := orgctx.BranchScope(ctx)
	cats, err := q.ListFinanceCategoryFacets(ctx, db.ListFinanceCategoryFacetsParams{OrganizationID: orgID, BranchIds: scope})
	if err != nil {
		return out, err
	}
	for _, c := range cats {
		out.Category = append(out.Category, model.Facet{Value: c.Uuid.String(), Label: c.Name, Kind: c.Kind, Count: c.Count})
	}
	branches, err := q.ListFinanceBranchFacets(ctx, db.ListFinanceBranchFacetsParams{OrganizationID: orgID, BranchIds: scope})
	if err != nil {
		return out, err
	}
	for _, b := range branches {
		if !b.BranchUuid.Valid {
			out.Branch = append(out.Branch, model.Facet{Value: model.HeadOffice, Count: b.Count})
			continue
		}
		out.Branch = append(out.Branch, model.Facet{Value: uuid.UUID(b.BranchUuid.Bytes).String(), Label: b.BranchName.String, Count: b.Count})
	}
	staff, err := q.ListFinanceStaffFacets(ctx, db.ListFinanceStaffFacetsParams{OrganizationID: orgID, BranchIds: scope})
	if err != nil {
		return out, err
	}
	for _, st := range staff {
		out.Staff = append(out.Staff, model.Facet{Value: st.StaffUuid.String(), Label: st.StaffName, Count: st.Count})
	}
	return out, nil
}

// Summary returns the figures of a period (default: the first of the
// month to today) per currency, kept apart: revenue from the sale lines,
// income and expenses entered here, staff payouts from the cash book, and
// the totals per category.
func (s *Service) Summary(ctx context.Context, orgID int64, from, to *time.Time, branches []uuid.UUID) (model.Summary, error) {
	today := s.today(ctx)
	df := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	dt := today
	if from != nil {
		df = *from
	}
	if to != nil {
		dt = *to
	}
	if dt.Before(df) {
		return model.Summary{}, invalid("date_to", "invalid", "date_to must not be before date_from")
	}
	if dt.Sub(df) > maxSummaryDays*24*time.Hour {
		return model.Summary{}, invalid("date_to", "too_long", "the range is at most 366 days")
	}
	out := model.Summary{DateFrom: formatDate(df), DateTo: formatDate(dt), Totals: []model.SummaryTotals{}, Categories: []model.CategoryTotal{}}
	totals := map[string]*model.SummaryTotals{}
	get := func(currency string) *model.SummaryTotals {
		t, ok := totals[currency]
		if !ok {
			t = &model.SummaryTotals{
				Currency: currency, Revenue: money.Zero(), Income: money.Zero(), Expenses: money.Zero(),
				StaffPayments: money.Zero(), Net: money.Zero(),
			}
			totals[currency] = t
		}
		return t
	}
	if s.sales != nil {
		sum, err := s.sales.Summary(ctx, orgID, &df, &dt, branches)
		if err != nil {
			return model.Summary{}, err
		}
		for _, f := range sum.Totals {
			t := get(f.Currency)
			t.Revenue = f.Revenue.Net
			t.Income = f.Income
			t.Expenses = f.Expenses
			t.StaffPayments = f.StaffPayments
		}
	}
	q := s.repo.Q()
	var filter []int64
	if len(branches) > 0 {
		rows, err := q.ListBranchIDsByUUIDs(ctx, db.ListBranchIDsByUUIDsParams{OrganizationID: orgID, Uuids: branches})
		if err != nil {
			return model.Summary{}, err
		}
		filter = []int64{}
		for _, r := range rows {
			filter = append(filter, r.ID)
		}
	}
	cats, err := q.FinanceTotalsByCategory(ctx, db.FinanceTotalsByCategoryParams{
		OrganizationID: orgID, DateFrom: dateArg(df), DateTo: dateArg(dt), BranchIds: orgctx.BranchScope(ctx),
		FilterBranchIds: filter,
	})
	if err != nil {
		return model.Summary{}, err
	}
	for _, c := range cats {
		out.Categories = append(out.Categories, model.CategoryTotal{
			Kind: c.Kind, Category: model.Ref{UUID: c.CategoryUuid, Name: c.CategoryName}, Currency: c.Currency,
			Total: money.FromNumericOrZero(c.Total), EntryCount: c.EntryCount,
		})
		if s.sales == nil {
			t := get(c.Currency)
			if c.Kind == model.KindIncome {
				t.Income = t.Income.Add(money.FromNumericOrZero(c.Total))
			} else {
				t.Expenses = t.Expenses.Add(money.FromNumericOrZero(c.Total))
			}
		}
	}
	for _, t := range totals {
		t.Net = t.Revenue.Add(t.Income).Sub(t.Expenses)
		out.Totals = append(out.Totals, *t)
	}
	sort.Slice(out.Totals, func(i, j int) bool { return out.Totals[i].Currency < out.Totals[j].Currency })
	return out, nil
}

// Activity lists the audit events of a record.
func (s *Service) Activity(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]model.ActivityEntry, int64, error) {
	q := s.repo.Q()
	if _, err := s.loadEntry(ctx, q, orgID, id); err != nil {
		return nil, 0, err
	}
	org := pgtype.Int8{Int64: orgID, Valid: true}
	res := pgtype.UUID{Bytes: id, Valid: true}
	rows, err := q.ListOrganizationResourceActivity(ctx, db.ListOrganizationResourceActivityParams{
		OrganizationID: org, Resource: model.EntriesSlug, ResourceUuid: res, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountOrganizationResourceActivity(ctx, db.CountOrganizationResourceActivityParams{
		OrganizationID: org, Resource: model.EntriesSlug, ResourceUuid: res,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.ActivityEntry, 0, len(rows))
	for _, row := range rows {
		e := model.ActivityEntry{UUID: row.Uuid, Action: row.Action, Payload: json.RawMessage(row.Payload), CreatedAt: row.CreatedAt.Time}
		if row.ActorUuid.Valid {
			e.Actor = &model.ActivityActor{
				UUID: uuid.UUID(row.ActorUuid.Bytes), Name: row.ActorName.String,
				Surname: row.ActorSurname.String, Email: row.ActorEmail.String,
			}
		}
		out = append(out, e)
	}
	return out, total, nil
}

func mapLedgerRow(r db.ListFinanceLedgerRow) model.LedgerRow {
	row := model.LedgerRow{
		Source: r.Source, UUID: r.Uuid, EntryType: r.EntryType, Kind: r.Kind, EntryDate: formatDate(r.EntryDate.Time),
		Amount: money.FromNumericOrZero(r.Amount), Currency: r.Currency, Branch: ref(r.BranchUuid, r.BranchName),
		Category: ref(r.CategoryUuid, r.CategoryName), ItemType: r.ItemType, Staff: ref(r.StaffUuid, r.StaffName),
		Account: ref(r.AccountUuid, r.AccountName), Description: r.Description, HasDocument: r.HasDocument,
		Reversed: r.Reversed, ReservationUUID: uuidPtr(r.ReservationUuid), CreatedAt: r.CreatedAt.Time,
	}
	return row
}
