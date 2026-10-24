// Package adapters connects the staff shares module to the io engine
// (TEC-976): exports of the calculated shares and of the share payouts,
// with the list filters. The organization always comes from the context
// (orgctx.OrganizationID); without one every call fails. Shares are
// written with their sale lines and payouts post cash movements, so
// neither is ever imported.
package adapters

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/google/uuid"
)

// Engine slugs of the two lists.
const (
	LinesResource   = model.LinesSlug
	PayoutsResource = model.PayoutsSlug
)

// exportLimit caps one export file.
const exportLimit = 100000

var (
	errNoOrganization = errors.New("staffshares: organization scope is required")
	errNoImport       = errors.New("staffshares: import is not supported")
)

// query helpers shared by both filters.
func uuids(raw string) []uuid.UUID {
	var out []uuid.UUID
	for _, part := range apiquery.SplitCSV(raw) {
		if id, err := uuid.Parse(part); err == nil {
			out = append(out, id)
		}
	}
	return out
}

func enums(raw string, allowed []string) []string {
	var out []string
	for _, v := range apiquery.SplitCSV(raw) {
		if slices.Contains(allowed, v) {
			out = append(out, v)
		}
	}
	return out
}

func date(raw string) *time.Time {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(raw))
	if err != nil {
		return nil
	}
	return &t
}

func sortOf(query map[string]string) (string, bool) {
	sort := strings.TrimSpace(query["sort"])
	return strings.TrimPrefix(sort, "-"), strings.HasPrefix(sort, "-")
}

// LineFilterFromQuery maps the export query (the list filters, sort and q)
// onto the share list filter.
func LineFilterFromQuery(query map[string]string) model.LineFilter {
	f := model.LineFilter{
		Branches: uuids(query["branch"]), Staff: uuids(query["staff"]),
		Kinds: enums(query["kind"], model.LineKinds), ItemTypes: enums(query["item_type"], model.ItemTypes),
		DateFrom: date(query["date_from"]), DateTo: date(query["date_to"]), Q: strings.TrimSpace(query["q"]),
		Limit: exportLimit,
	}
	if st := enums(query["status"], model.Statuses); len(st) == 1 {
		f.Status = st[0]
	}
	f.SortField, f.SortDesc = sortOf(query)
	return f
}

// PayoutFilterFromQuery maps the export query onto the payout list filter.
func PayoutFilterFromQuery(query map[string]string) model.PayoutFilter {
	f := model.PayoutFilter{
		Branches: uuids(query["branch"]), Staff: uuids(query["staff"]), Kinds: enums(query["kind"], model.PayoutKinds),
		DateFrom: date(query["date_from"]), DateTo: date(query["date_to"]), Q: strings.TrimSpace(query["q"]),
		Limit: exportLimit,
	}
	f.SortField, f.SortDesc = sortOf(query)
	return f
}

// ------------------------------------------------------------------ shares

// LinesIO exports the calculated shares.
type LinesIO struct {
	svc *usecase.Service
}

// NewLinesIO builds the share export adapter.
func NewLinesIO(svc *usecase.Service) *LinesIO { return &LinesIO{svc: svc} }

func (a *LinesIO) Resource() string { return LinesResource }

func (a *LinesIO) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "share_date", LabelKey: "staff_share.share_date", Type: ioengine.ColumnTypeString},
		{Key: "staff", LabelKey: "staff_share.staff", Type: ioengine.ColumnTypeString},
		{Key: "kind", LabelKey: "staff_share.kind", Type: ioengine.ColumnTypeEnum},
		{Key: "item_type", LabelKey: "staff_share.item_type", Type: ioengine.ColumnTypeEnum},
		{Key: "description", LabelKey: "staff_share.description", Type: ioengine.ColumnTypeString},
		{Key: "quantity", LabelKey: "staff_share.quantity", Type: ioengine.ColumnTypeString},
		{Key: "base_amount", LabelKey: "staff_share.base_amount", Type: ioengine.ColumnTypeString},
		{Key: "rule_scope", LabelKey: "staff_share.rule_scope", Type: ioengine.ColumnTypeEnum},
		{Key: "basis", LabelKey: "staff_share.basis", Type: ioengine.ColumnTypeEnum},
		{Key: "rate", LabelKey: "staff_share.rate", Type: ioengine.ColumnTypeString},
		{Key: "fixed_amount", LabelKey: "staff_share.fixed_amount", Type: ioengine.ColumnTypeString},
		{Key: "share_amount", LabelKey: "staff_share.share_amount", Type: ioengine.ColumnTypeString},
		{Key: "business_amount", LabelKey: "staff_share.business_amount", Type: ioengine.ColumnTypeString},
		{Key: "currency", LabelKey: "staff_share.currency", Type: ioengine.ColumnTypeString},
		{Key: "status", LabelKey: "staff_share.status", Type: ioengine.ColumnTypeEnum},
		{Key: "branch", LabelKey: "staff_share.branch", Type: ioengine.ColumnTypeString},
		{Key: "customer", LabelKey: "staff_share.customer", Type: ioengine.ColumnTypeString},
		{Key: "reservation_number", LabelKey: "staff_share.reservation_number", Type: ioengine.ColumnTypeString},
		{Key: "uuid", LabelKey: "staff_share.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "created_at", LabelKey: "staff_share.created_at", Type: ioengine.ColumnTypeDatetime},
	}
}

func (a *LinesIO) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return ioengine.Dataset{}, errNoOrganization
	}
	// The route needs tenant.earnings.read: every share inside the
	// caller's branch scope.
	items, _, err := a.svc.ListLines(ctx, orgID, usecase.Viewer{}, LineFilterFromQuery(query))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(items))
	for _, l := range items {
		row := map[string]any{
			"uuid": l.UUID.String(), "share_date": l.ShareDate, "staff": l.Staff.Name, "kind": l.Kind,
			"item_type": l.ItemType, "description": l.Description, "quantity": l.Quantity,
			"base_amount": l.BaseAmount.String(), "rule_scope": l.RuleScope, "basis": l.Basis, "rate": "",
			"fixed_amount": "", "share_amount": l.ShareAmount.String(),
			"business_amount": l.BaseAmount.Sub(l.ShareAmount).String(), "currency": l.Currency, "status": l.Status,
			"branch": l.Branch.Name, "customer": "", "reservation_number": "", "created_at": l.CreatedAt,
		}
		if l.Rate != nil {
			row["rate"] = l.Rate.String()
		}
		if l.FixedAmount != nil {
			row["fixed_amount"] = l.FixedAmount.String()
		}
		if l.Customer != nil {
			row["customer"] = l.Customer.Name
		}
		if l.Reservation != nil {
			row["reservation_number"] = l.Reservation.Number
		}
		out = append(out, row)
	}
	return ioengine.Dataset{Resource: LinesResource, Columns: a.ExportColumns(), Rows: out}, nil
}

func (a *LinesIO) ImportSchema() []ioengine.ImportField { return nil }

func (a *LinesIO) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errNoImport
}

func (a *LinesIO) RevertRow(context.Context, string, string, map[string]any) error {
	return errNoImport
}

// ----------------------------------------------------------------- payouts

// PayoutsIO exports the share payouts.
type PayoutsIO struct {
	svc *usecase.Service
}

// NewPayoutsIO builds the payout export adapter.
func NewPayoutsIO(svc *usecase.Service) *PayoutsIO { return &PayoutsIO{svc: svc} }

func (a *PayoutsIO) Resource() string { return PayoutsResource }

func (a *PayoutsIO) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "payout_date", LabelKey: "staff_payout.payout_date", Type: ioengine.ColumnTypeString},
		{Key: "staff", LabelKey: "staff_payout.staff", Type: ioengine.ColumnTypeString},
		{Key: "kind", LabelKey: "staff_payout.kind", Type: ioengine.ColumnTypeEnum},
		{Key: "amount", LabelKey: "staff_payout.amount", Type: ioengine.ColumnTypeString},
		{Key: "currency", LabelKey: "staff_payout.currency", Type: ioengine.ColumnTypeString},
		{Key: "cash_account", LabelKey: "staff_payout.cash_account", Type: ioengine.ColumnTypeString},
		{Key: "branch", LabelKey: "staff_payout.branch", Type: ioengine.ColumnTypeString},
		{Key: "note", LabelKey: "staff_payout.note", Type: ioengine.ColumnTypeString},
		{Key: "reversed", LabelKey: "staff_payout.reversed", Type: ioengine.ColumnTypeBoolean},
		{Key: "created_by", LabelKey: "staff_payout.created_by", Type: ioengine.ColumnTypeString},
		{Key: "uuid", LabelKey: "staff_payout.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "created_at", LabelKey: "staff_payout.created_at", Type: ioengine.ColumnTypeDatetime},
	}
}

func (a *PayoutsIO) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return ioengine.Dataset{}, errNoOrganization
	}
	items, _, err := a.svc.ListPayouts(ctx, orgID, usecase.Viewer{}, PayoutFilterFromQuery(query))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(items))
	for _, p := range items {
		row := map[string]any{
			"uuid": p.UUID.String(), "payout_date": p.PayoutDate, "staff": p.Staff.Name, "kind": p.Kind,
			"amount": p.Amount.String(), "currency": p.Currency, "cash_account": p.CashAccount.Name, "branch": "",
			"note": p.Note, "reversed": p.ReversedByUUID != nil, "created_by": "", "created_at": p.CreatedAt,
		}
		if p.Branch != nil {
			row["branch"] = p.Branch.Name
		}
		if p.CreatedBy != nil {
			row["created_by"] = strings.TrimSpace(p.CreatedBy.Name + " " + p.CreatedBy.Surname)
		}
		out = append(out, row)
	}
	return ioengine.Dataset{Resource: PayoutsResource, Columns: a.ExportColumns(), Rows: out}, nil
}

func (a *PayoutsIO) ImportSchema() []ioengine.ImportField { return nil }

func (a *PayoutsIO) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errNoImport
}

func (a *PayoutsIO) RevertRow(context.Context, string, string, map[string]any) error {
	return errNoImport
}

var (
	_ ioengine.ResourceAdapter = (*LinesIO)(nil)
	_ ioengine.ResourceAdapter = (*PayoutsIO)(nil)
)
