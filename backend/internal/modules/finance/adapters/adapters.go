// Package adapters connects the income / expense module to the io engine
// (export of the combined list: records of the business and the sale
// lines as automatic income). The organization always comes from the
// context (orgctx.OrganizationID); without one every call fails. Records
// are never imported: each one posts a cash movement.
package adapters

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/google/uuid"
)

// EntriesResource is the engine slug of the list.
const EntriesResource = model.EntriesSlug

var (
	errNoOrganization = errors.New("finance: organization scope is required")
	errNoImport       = errors.New("finance: import is not supported")
)

// LedgerFilterFromQuery maps the export query (the list filters, sort and
// q) onto the list filter.
func LedgerFilterFromQuery(query map[string]string) model.LedgerFilter {
	f := model.LedgerFilter{Q: strings.TrimSpace(query["q"])}
	keep := func(values, allowed []string) []string {
		var out []string
		for _, v := range values {
			for _, a := range allowed {
				if v == a {
					out = append(out, v)
				}
			}
		}
		return out
	}
	uuids := func(raw string) []uuid.UUID {
		var out []uuid.UUID
		for _, part := range apiquery.SplitCSV(raw) {
			if id, err := uuid.Parse(part); err == nil {
				out = append(out, id)
			}
		}
		return out
	}
	f.Sources = keep(apiquery.SplitCSV(query["source"]), model.Sources)
	f.Kinds = keep(apiquery.SplitCSV(query["kind"]), model.Kinds)
	for _, part := range apiquery.SplitCSV(query["branch"]) {
		if part == model.HeadOffice {
			f.HeadOffice = true
		} else if id, err := uuid.Parse(part); err == nil {
			f.Branches = append(f.Branches, id)
		}
	}
	for _, part := range apiquery.SplitCSV(query["category"]) {
		if _, err := uuid.Parse(part); err == nil || part == model.SaleServiceCategory || part == model.SaleProductCategory {
			f.Categories = append(f.Categories, part)
		}
	}
	f.Staff = uuids(query["staff"])
	f.Accounts = uuids(query["account"])
	for _, p := range []struct {
		key string
		dst **time.Time
	}{{"date_from", &f.DateFrom}, {"date_to", &f.DateTo}} {
		if t, err := time.Parse("2006-01-02", strings.TrimSpace(query[p.key])); err == nil {
			*p.dst = &t
		}
	}
	if sort := strings.TrimSpace(query["sort"]); sort != "" {
		f.SortDesc = strings.HasPrefix(sort, "-")
		f.SortField = strings.TrimPrefix(sort, "-")
	}
	return f
}

// EntriesIO exports the income / expense list.
type EntriesIO struct {
	svc *usecase.Service
}

// NewEntriesIO builds the io adapter.
func NewEntriesIO(svc *usecase.Service) *EntriesIO {
	return &EntriesIO{svc: svc}
}

func (a *EntriesIO) Resource() string { return EntriesResource }

func (a *EntriesIO) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "entry_date", LabelKey: "finance_entry.entry_date", Type: ioengine.ColumnTypeString},
		{Key: "kind", LabelKey: "finance_entry.kind", Type: ioengine.ColumnTypeEnum},
		{Key: "source", LabelKey: "finance_entry.source", Type: ioengine.ColumnTypeEnum},
		{Key: "entry_type", LabelKey: "finance_entry.entry_type", Type: ioengine.ColumnTypeEnum},
		{Key: "category", LabelKey: "finance_entry.category", Type: ioengine.ColumnTypeString},
		{Key: "amount", LabelKey: "finance_entry.amount", Type: ioengine.ColumnTypeString},
		{Key: "currency", LabelKey: "finance_entry.currency", Type: ioengine.ColumnTypeString},
		{Key: "branch", LabelKey: "finance_entry.branch", Type: ioengine.ColumnTypeString},
		{Key: "staff", LabelKey: "finance_entry.staff", Type: ioengine.ColumnTypeString},
		{Key: "account", LabelKey: "finance_entry.account", Type: ioengine.ColumnTypeString},
		{Key: "description", LabelKey: "finance_entry.description", Type: ioengine.ColumnTypeString},
		{Key: "reversed", LabelKey: "finance_entry.reversed", Type: ioengine.ColumnTypeBoolean},
		{Key: "has_document", LabelKey: "finance_entry.has_document", Type: ioengine.ColumnTypeBoolean},
		{Key: "uuid", LabelKey: "finance_entry.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "created_at", LabelKey: "finance_entry.created_at", Type: ioengine.ColumnTypeDatetime},
	}
}

func (a *EntriesIO) Export(ctx context.Context, query ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	orgID, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return ioengine.Dataset{}, errNoOrganization
	}
	rows, err := a.svc.ExportLedger(ctx, orgID, LedgerFilterFromQuery(query))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		row := map[string]any{
			"uuid": r.UUID.String(), "entry_date": r.EntryDate, "kind": r.Kind, "source": r.Source,
			"entry_type": r.EntryType, "category": "", "amount": r.Amount.String(), "currency": r.Currency,
			"branch": "", "staff": "", "account": "", "description": r.Description, "reversed": r.Reversed,
			"has_document": r.HasDocument, "created_at": r.CreatedAt,
		}
		switch {
		case r.Category != nil:
			row["category"] = r.Category.Name
		case r.ItemType == model.ItemService:
			row["category"] = i18n.Translate(loc, "finance_entry.status."+model.SaleServiceCategory)
		case r.ItemType == model.ItemProduct:
			row["category"] = i18n.Translate(loc, "finance_entry.status."+model.SaleProductCategory)
		}
		if r.Branch != nil {
			row["branch"] = r.Branch.Name
		}
		if r.Staff != nil {
			row["staff"] = r.Staff.Name
		}
		if r.Account != nil {
			row["account"] = r.Account.Name
		}
		out = append(out, row)
	}
	return ioengine.Dataset{Resource: EntriesResource, Columns: a.ExportColumns(), Rows: out}, nil
}

func (a *EntriesIO) ImportSchema() []ioengine.ImportField { return nil }

func (a *EntriesIO) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errNoImport
}

func (a *EntriesIO) RevertRow(context.Context, string, string, map[string]any) error {
	return errNoImport
}

var _ ioengine.ResourceAdapter = (*EntriesIO)(nil)
