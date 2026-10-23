// Package adapters connects the cash module to the shared engines:
// ioengine (account and movement export), bulkengine (archive / activate
// accounts) and searchengine (Cmd+K accounts). The organization always
// comes from the context (orgctx.OrganizationID); without one every adapter
// call fails. Money is never imported: balances come from movements only.
package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/google/uuid"
)

// Engine slugs.
const (
	AccountsResource  = model.AccountsSlug
	MovementsResource = model.MovementsSlug
)

// MaxBulkTargets caps the "all matching" bulk scope.
const MaxBulkTargets = 10000

var (
	errNoOrganization = errors.New("cash: organization scope is required")
	errNoImport       = errors.New("cash: import is not supported")
)

func organizationID(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return 0, errNoOrganization
	}
	return id, nil
}

// AccountFilterFromQuery maps list query params (q, branch, kind, status).
func AccountFilterFromQuery(query map[string]string) model.AccountFilter {
	f := model.AccountFilter{Q: strings.TrimSpace(query["q"])}
	for _, part := range apiquery.SplitCSV(query["branch"]) {
		if part == model.HeadOffice {
			f.HeadOffice = true
			continue
		}
		if id, err := uuid.Parse(part); err == nil {
			f.Branches = append(f.Branches, id)
		}
	}
	f.Kinds = apiquery.SplitCSV(query["kind"])
	f.Statuses = apiquery.SplitCSV(query["status"])
	return f
}

// MovementFilterFromQuery maps list query params (q, account, kind,
// direction, date_from, date_to).
func MovementFilterFromQuery(query map[string]string) model.MovementFilter {
	f := model.MovementFilter{Q: strings.TrimSpace(query["q"])}
	for _, part := range apiquery.SplitCSV(query["account"]) {
		if id, err := uuid.Parse(part); err == nil {
			f.Accounts = append(f.Accounts, id)
		}
	}
	f.Kinds = apiquery.SplitCSV(query["kind"])
	if d := strings.TrimSpace(query["direction"]); d == model.DirectionIn || d == model.DirectionOut {
		f.Direction = d
	}
	for _, p := range []struct {
		key string
		dst **time.Time
	}{{"date_from", &f.DateFrom}, {"date_to", &f.DateTo}} {
		if t, err := time.Parse("2006-01-02", strings.TrimSpace(query[p.key])); err == nil {
			*p.dst = &t
		}
	}
	return f
}

// ---------------------------------------------------------------- io engine

// AccountsIO exports accounts with their balances.
type AccountsIO struct {
	svc *usecase.Service
}

// NewAccountsIO builds the account io adapter.
func NewAccountsIO(svc *usecase.Service) *AccountsIO {
	return &AccountsIO{svc: svc}
}

func (a *AccountsIO) Resource() string { return AccountsResource }

func (a *AccountsIO) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "branch_code", LabelKey: "cash_account.branch_code", Type: ioengine.ColumnTypeString},
		{Key: "branch", LabelKey: "cash_account.branch", Type: ioengine.ColumnTypeString},
		{Key: "kind", LabelKey: "cash_account.kind", Type: ioengine.ColumnTypeEnum},
		{Key: "name", LabelKey: "cash_account.name", Type: ioengine.ColumnTypeString},
		{Key: "currency", LabelKey: "cash_account.currency", Type: ioengine.ColumnTypeString},
		{Key: "opening_balance", LabelKey: "cash_account.opening_balance", Type: ioengine.ColumnTypeString},
		{Key: "opening_date", LabelKey: "cash_account.opening_date", Type: ioengine.ColumnTypeString},
		{Key: "balance", LabelKey: "cash_account.balance", Type: ioengine.ColumnTypeString},
		{Key: "last_closed_date", LabelKey: "cash_account.last_closed_date", Type: ioengine.ColumnTypeString},
		{Key: "bank_name", LabelKey: "cash_account.bank_name", Type: ioengine.ColumnTypeString},
		{Key: "iban", LabelKey: "cash_account.iban", Type: ioengine.ColumnTypeString},
		{Key: "status", LabelKey: "cash_account.status", Type: ioengine.ColumnTypeEnum},
		{Key: "uuid", LabelKey: "cash_account.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "created_at", LabelKey: "cash_account.created_at", Type: ioengine.ColumnTypeDatetime},
	}
}

func (a *AccountsIO) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, err := a.svc.ExportAccounts(ctx, orgID, AccountFilterFromQuery(query))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		row := map[string]any{
			"uuid": r.UUID.String(), "kind": r.Kind, "name": r.Name, "currency": r.Currency,
			"opening_balance": r.OpeningBalance.String(), "opening_date": r.OpeningDate, "balance": r.Balance.String(),
			"last_closed_date": "", "bank_name": r.BankName, "iban": r.IBAN, "status": r.Status,
			"created_at": r.CreatedAt, "branch_code": "", "branch": "",
		}
		if r.Branch != nil {
			row["branch_code"], row["branch"] = r.Branch.Code, r.Branch.Name
		}
		if r.LastClosedDate != nil {
			row["last_closed_date"] = *r.LastClosedDate
		}
		out = append(out, row)
	}
	return ioengine.Dataset{Resource: AccountsResource, Columns: a.ExportColumns(), Rows: out}, nil
}

func (a *AccountsIO) ImportSchema() []ioengine.ImportField { return nil }

func (a *AccountsIO) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errNoImport
}

func (a *AccountsIO) RevertRow(context.Context, string, string, map[string]any) error {
	return errNoImport
}

// MovementsIO exports the cash book.
type MovementsIO struct {
	svc *usecase.Service
}

// NewMovementsIO builds the movement io adapter.
func NewMovementsIO(svc *usecase.Service) *MovementsIO {
	return &MovementsIO{svc: svc}
}

func (a *MovementsIO) Resource() string { return MovementsResource }

func (a *MovementsIO) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "movement_date", LabelKey: "cash_movement.movement_date", Type: ioengine.ColumnTypeString},
		{Key: "account", LabelKey: "cash_movement.account", Type: ioengine.ColumnTypeString},
		{Key: "kind", LabelKey: "cash_movement.kind", Type: ioengine.ColumnTypeEnum},
		{Key: "direction", LabelKey: "cash_movement.direction", Type: ioengine.ColumnTypeEnum},
		{Key: "amount", LabelKey: "cash_movement.amount", Type: ioengine.ColumnTypeString},
		{Key: "currency", LabelKey: "cash_movement.currency", Type: ioengine.ColumnTypeString},
		{Key: "counter_account", LabelKey: "cash_movement.counter_account", Type: ioengine.ColumnTypeString},
		{Key: "description", LabelKey: "cash_movement.description", Type: ioengine.ColumnTypeString},
		{Key: "created_by", LabelKey: "cash_movement.created_by", Type: ioengine.ColumnTypeString},
		{Key: "uuid", LabelKey: "cash_movement.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "created_at", LabelKey: "cash_movement.created_at", Type: ioengine.ColumnTypeDatetime},
	}
}

func (a *MovementsIO) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, err := a.svc.ExportMovements(ctx, orgID, MovementFilterFromQuery(query))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, m := range rows {
		row := map[string]any{
			"uuid": m.UUID.String(), "movement_date": m.MovementDate, "account": m.Account.Name, "kind": m.Kind,
			"direction": m.Direction, "amount": m.Amount.String(), "currency": m.Currency, "counter_account": "",
			"description": m.Description, "created_by": "", "created_at": m.CreatedAt,
		}
		if m.CounterAccount != nil {
			row["counter_account"] = m.CounterAccount.Name
		}
		if m.CreatedBy != nil {
			row["created_by"] = strings.TrimSpace(m.CreatedBy.Name + " " + m.CreatedBy.Surname)
		}
		out = append(out, row)
	}
	return ioengine.Dataset{Resource: MovementsResource, Columns: a.ExportColumns(), Rows: out}, nil
}

func (a *MovementsIO) ImportSchema() []ioengine.ImportField { return nil }

func (a *MovementsIO) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errNoImport
}

func (a *MovementsIO) RevertRow(context.Context, string, string, map[string]any) error {
	return errNoImport
}

// -------------------------------------------------------------- bulk engine

// Bulk archives / activates accounts. Archiving needs a zero balance.
type Bulk struct {
	svc *usecase.Service
}

// NewBulk builds the bulk adapter. svc may be nil for metadata only.
func NewBulk(svc *usecase.Service) *Bulk {
	return &Bulk{svc: svc}
}

func (a *Bulk) Resource() string { return AccountsResource }

func (a *Bulk) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{
		{
			ID: "archive", LabelKey: "bulk.actions.cash_accounts.archive",
			Permission: rbac.PermTenantCashBulkArchive, Reversible: true,
			ConfirmKey: "bulk.confirm.cash_accounts.archive",
		},
		{
			ID: "activate", LabelKey: "bulk.actions.cash_accounts.activate",
			Permission: rbac.PermTenantCashBulkActivate, Reversible: true,
		},
	}
}

func (a *Bulk) ResolveTargets(ctx context.Context, _ string, target bulkengine.BulkTarget) ([]string, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return nil, err
	}
	switch target.Scope {
	case "ids":
		seen := map[string]bool{}
		out := make([]string, 0, len(target.IDs))
		for _, raw := range target.IDs {
			id, err := uuid.Parse(strings.TrimSpace(raw))
			if err != nil || seen[id.String()] {
				continue
			}
			seen[id.String()] = true
			out = append(out, id.String())
		}
		if len(out) > MaxBulkTargets {
			return nil, fmt.Errorf("at most %d targets are allowed", MaxBulkTargets)
		}
		return out, nil
	case "query":
		ids, err := a.svc.AccountUUIDs(ctx, orgID, AccountFilterFromQuery(target.Query), MaxBulkTargets)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(ids))
		for _, id := range ids {
			out = append(out, id.String())
		}
		return out, nil
	default:
		return nil, fmt.Errorf("invalid target scope")
	}
}

func (a *Bulk) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "invalid uuid"}, nil
	}
	var status string
	switch action {
	case "archive":
		status = model.StatusArchived
	case "activate":
		status = model.StatusActive
	default:
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "unknown action"}, nil
	}
	prev, err := a.svc.SetStatus(ctx, orgID, id, status)
	if err != nil {
		switch {
		case errors.Is(err, usecase.ErrNotFound):
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "not found"}, nil
		case errors.Is(err, usecase.ErrBalanceNotZero):
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: err.Error()}, nil
		}
		return bulkengine.BulkItemResult{}, err
	}
	return bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: "cash_account", OK: true, Op: "update",
		Previous: map[string]any{"status": prev},
	}, nil
}

func (a *Bulk) RevertItem(ctx context.Context, _ string, entityUUID string, previous map[string]any) error {
	orgID, err := organizationID(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	prev, ok := previous["status"].(string)
	if !ok {
		return fmt.Errorf("missing previous status")
	}
	_, err = a.svc.SetStatus(ctx, orgID, id, prev)
	return err
}

// ------------------------------------------------------------ search engine

// Search indexes accounts for the tenant Cmd+K palette.
type Search struct {
	q *db.Queries
}

// NewSearch builds the search adapter.
func NewSearch(q *db.Queries) *Search {
	return &Search{q: q}
}

func (a *Search) Spec() searchengine.Spec {
	return searchengine.Spec{
		ID:         usecase.SearchSpec,
		LabelKey:   "search.specs_cash_accounts",
		Permission: rbac.PermTenantCashRead,
		Icon:       "cash",
		Searchable: []string{"title", "subtitle", "keywords"},
		Scope:      searchengine.ScopeTenant,
		// Branch accounts carry their branch; business-wide accounts carry
		// none, so branch-limited members never find them.
		BranchScoped: true,
	}
}

// ListAll walks every live organization (each query is organization-scoped).
func (a *Search) ListAll(ctx context.Context) ([]searchengine.Document, error) {
	orgIDs, err := a.q.ListLiveOrganizationIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]searchengine.Document, 0)
	for _, orgID := range orgIDs {
		rows, err := a.q.ListCashAccountSearchDocuments(ctx, orgID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			out = append(out, searchDocument(db.GetCashAccountSearchDocumentRow(row)))
		}
	}
	return out, nil
}

// Document loads one account by its tenant document id.
func (a *Search) Document(ctx context.Context, docID string) (searchengine.Document, error) {
	orgID, raw, err := searchengine.ParseTenantDocumentID(docID)
	if err != nil {
		return searchengine.Document{}, err
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("cash search: invalid uuid")
	}
	row, err := a.q.GetCashAccountSearchDocument(ctx, db.GetCashAccountSearchDocumentParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return searchengine.Document{}, err
	}
	return searchDocument(row), nil
}

func searchDocument(row db.GetCashAccountSearchDocumentRow) searchengine.Document {
	subtitle := row.BranchName.String
	if row.BankName != "" {
		subtitle = strings.TrimPrefix(subtitle+" · "+row.BankName, " · ")
	}
	doc := searchengine.Document{
		ID:             searchengine.TenantDocumentID(row.OrganizationID, row.Uuid.String()),
		Spec:           usecase.SearchSpec,
		Title:          row.Name,
		Subtitle:       subtitle,
		Keywords:       []string{row.Name, row.BankName, row.BranchName.String, row.Kind},
		Href:           "/t/" + row.OrganizationSlug + "/cash/" + row.Uuid.String(),
		Icon:           "cash",
		OrganizationID: row.OrganizationID,
		BranchIDs:      []int64{},
	}
	if row.BranchID.Valid {
		doc.BranchIDs = []int64{row.BranchID.Int64}
	}
	return doc
}

var (
	_ ioengine.ResourceAdapter = (*AccountsIO)(nil)
	_ ioengine.ResourceAdapter = (*MovementsIO)(nil)
	_ bulkengine.BulkAdapter   = (*Bulk)(nil)
	_ searchengine.Adapter     = (*Search)(nil)
)
