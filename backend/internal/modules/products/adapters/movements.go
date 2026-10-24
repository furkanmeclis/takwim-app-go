package adapters

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/google/uuid"
)

// MovementsResource is the engine slug of the stock ledger.
const MovementsResource = model.MovementsSlug

// maxCounted mirrors the count limit of the ledger.
const maxCounted = 100000

// MovementFilterFromQuery maps the movement list query (q, sort, kind,
// branch, product, staff, from, to) to a filter. Invalid uuids and times
// are dropped (the list endpoint rejects them before an export starts).
func MovementFilterFromQuery(query map[string]string) model.MovementFilter {
	f := model.MovementFilter{Q: strings.TrimSpace(query["q"]), Kinds: apiquery.SplitCSV(query["kind"])}
	for _, p := range []struct {
		key string
		dst *[]uuid.UUID
	}{{"branch", &f.Branches}, {"product", &f.Products}, {"staff", &f.Staff}} {
		for _, part := range apiquery.SplitCSV(query[p.key]) {
			if id, err := uuid.Parse(part); err == nil {
				*p.dst = append(*p.dst, id)
			}
		}
	}
	for _, t := range []struct {
		key string
		dst **time.Time
	}{{"from", &f.From}, {"to", &f.To}} {
		if v, err := time.Parse(time.RFC3339, strings.TrimSpace(query[t.key])); err == nil {
			*t.dst = &v
		}
	}
	if sort := apiquery.ParseSort(query["sort"]); len(sort) > 0 {
		if _, ok := usecase.MovementSortColumns[sort[0].Field]; ok {
			f.SortField, f.SortDesc = sort[0].Field, sort[0].Desc
		}
	}
	return f
}

// MovementsIO exports the stock ledger and imports stock counts: a row
// (branch code, stock code, counted units) writes one adjustment.
type MovementsIO struct {
	svc *usecase.Service
}

// NewMovementsIO builds the ledger io adapter.
func NewMovementsIO(svc *usecase.Service) *MovementsIO {
	return &MovementsIO{svc: svc}
}

func (a *MovementsIO) Resource() string { return MovementsResource }

func (a *MovementsIO) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "occurred_at", LabelKey: "stock_movement.occurred_at", Type: ioengine.ColumnTypeDatetime},
		{Key: "kind", LabelKey: "stock_movement.kind", Type: ioengine.ColumnTypeEnum},
		{Key: "branch", LabelKey: "stock_movement.branch", Type: ioengine.ColumnTypeString},
		{Key: "product", LabelKey: "stock_movement.product", Type: ioengine.ColumnTypeString},
		{Key: "variant", LabelKey: "stock_movement.variant", Type: ioengine.ColumnTypeString},
		{Key: "stock_code", LabelKey: "stock_movement.stock_code", Type: ioengine.ColumnTypeString},
		{Key: "quantity", LabelKey: "stock_movement.quantity", Type: ioengine.ColumnTypeString},
		{Key: "balance_after", LabelKey: "stock_movement.balance_after", Type: ioengine.ColumnTypeString},
		{Key: "counterpart_branch", LabelKey: "stock_movement.counterpart_branch", Type: ioengine.ColumnTypeString},
		{Key: "unit_price", LabelKey: "stock_movement.unit_price", Type: ioengine.ColumnTypeString},
		{Key: "unit_cost", LabelKey: "stock_movement.unit_cost", Type: ioengine.ColumnTypeString},
		{Key: "currency", LabelKey: "stock_movement.currency", Type: ioengine.ColumnTypeString},
		{Key: "staff", LabelKey: "stock_movement.staff", Type: ioengine.ColumnTypeString},
		{Key: "reason", LabelKey: "stock_movement.reason", Type: ioengine.ColumnTypeString},
		{Key: "note", LabelKey: "stock_movement.note", Type: ioengine.ColumnTypeString},
		{Key: "source", LabelKey: "stock_movement.source", Type: ioengine.ColumnTypeEnum},
		{Key: "created_by", LabelKey: "stock_movement.created_by", Type: ioengine.ColumnTypeString},
		{Key: "uuid", LabelKey: "stock_movement.uuid", Type: ioengine.ColumnTypeUUID},
	}
}

func (a *MovementsIO) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, err := a.svc.ExportMovements(ctx, orgID, MovementFilterFromQuery(query), MaxExportRows)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, m := range rows {
		row := map[string]any{
			"uuid": m.UUID.String(), "occurred_at": m.OccurredAt, "kind": m.Kind, "branch": m.Branch.Name,
			"product": m.Product.Name, "variant": "", "stock_code": m.Product.SKU, "quantity": m.Quantity,
			"balance_after": m.BalanceAfter, "counterpart_branch": "", "unit_price": "", "unit_cost": "",
			"currency": m.Currency, "staff": "", "reason": m.Reason, "note": m.Note, "source": m.Source, "created_by": "",
		}
		if m.Variant != nil {
			row["variant"], row["stock_code"] = m.Variant.Name, m.Variant.SKU
		}
		if m.CounterpartBranch != nil {
			row["counterpart_branch"] = m.CounterpartBranch.Name
		}
		if m.UnitPrice != nil {
			row["unit_price"] = m.UnitPrice.String()
		}
		if m.UnitCost != nil {
			row["unit_cost"] = m.UnitCost.String()
		}
		if m.Staff != nil {
			row["staff"] = m.Staff.Name
		}
		if m.CreatedBy != nil {
			row["created_by"] = m.CreatedBy.Name
		}
		out = append(out, row)
	}
	return ioengine.Dataset{Resource: MovementsResource, Columns: a.ExportColumns(), Rows: out}, nil
}

// ImportSchema is the bulk count sheet.
func (a *MovementsIO) ImportSchema() []ioengine.ImportField {
	return []ioengine.ImportField{
		{Key: "branch_code", LabelKey: "stock_movement.branch_code", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "stock_code", LabelKey: "stock_movement.stock_code", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "counted_quantity", LabelKey: "stock_movement.counted_quantity", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "reason", LabelKey: "stock_movement.reason", Type: ioengine.ColumnTypeString},
		{Key: "note", LabelKey: "stock_movement.note", Type: ioengine.ColumnTypeString},
	}
}

// SampleRows implements ioengine.SampleProvider.
func (a *MovementsIO) SampleRows() []map[string]any {
	return []map[string]any{
		{"branch_code": "MRK", "stock_code": "SMP-001", "counted_quantity": "12", "reason": "Ay sonu sayımı"},
		{"branch_code": "MRK", "stock_code": "8690000000017", "counted_quantity": "0", "reason": "Ay sonu sayımı"},
	}
}

// countRow parses an import row (defaults fill empty optional cells).
func countRow(row, defaults map[string]any) (usecase.CountRow, error) {
	get := func(key string) string {
		if v := cell(row, key); v != "" {
			return v
		}
		return cell(defaults, key)
	}
	out := usecase.CountRow{
		BranchCode: get("branch_code"), StockCode: cell(row, "stock_code"), Reason: get("reason"), Note: get("note"),
	}
	if out.BranchCode == "" {
		return out, errors.New("branch_code is required")
	}
	if out.StockCode == "" {
		return out, errors.New("stock_code is required")
	}
	raw := cell(row, "counted_quantity")
	if raw == "" {
		return out, errors.New("counted_quantity is required")
	}
	// Spreadsheets hand whole numbers as "12" or "12.0".
	raw = strings.TrimSuffix(strings.TrimSuffix(raw, ".0"), ",0")
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || n < 0 || n > maxCounted {
		return out, fmt.Errorf("counted_quantity must be a whole number between 0 and %d", maxCounted)
	}
	out.Counted = int32(n)
	return out, nil
}

// PreviewRow implements ioengine.RowPreviewer: the dry run shows the stock
// on hand changing to the counted units.
func (a *MovementsIO) PreviewRow(ctx context.Context, row map[string]any, defaults map[string]any) (ioengine.RowDiff, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.RowDiff{}, err
	}
	in, err := countRow(row, defaults)
	if err != nil {
		return ioengine.RowDiff{Error: err.Error()}, nil
	}
	target, err := a.svc.ResolveCount(ctx, orgID, in.BranchCode, in.StockCode)
	if err != nil {
		if usecase.IsValidation(err) {
			return ioengine.RowDiff{Error: err.Error()}, nil
		}
		return ioengine.RowDiff{}, err
	}
	return ioengine.RowDiff{Op: "create", Changes: []ioengine.FieldChange{{
		Field: "counted_quantity", From: strconv.Itoa(int(target.OnHand)), To: strconv.Itoa(int(in.Counted)),
	}}}, nil
}

// ApplyRow writes the count of one row as an adjustment movement.
func (a *MovementsIO) ApplyRow(ctx context.Context, row map[string]any, defaults map[string]any) (ioengine.RowResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.RowResult{}, err
	}
	in, err := countRow(row, defaults)
	if err != nil {
		return ioengine.RowResult{OK: false, Error: err.Error()}, nil
	}
	m, err := a.svc.ImportCount(ctx, orgID, actorID(ctx), in)
	if err != nil {
		if usecase.IsValidation(err) || errors.Is(err, usecase.ErrNotFound) {
			return ioengine.RowResult{OK: false, Error: err.Error()}, nil
		}
		return ioengine.RowResult{}, err
	}
	return ioengine.RowResult{
		OK: true, EntityType: "stock_movement", EntityUUID: m.UUID.String(), Op: "create",
		Previous: map[string]any{"on_hand": m.BalanceAfter - m.Quantity, "quantity": m.Quantity},
	}, nil
}

// RevertRow takes an imported count back with a compensating adjustment.
func (a *MovementsIO) RevertRow(ctx context.Context, entityType, entityUUID string, _ map[string]any) error {
	if entityType != "stock_movement" {
		return fmt.Errorf("unsupported entity %q", entityType)
	}
	orgID, err := organizationID(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	return a.svc.RevertImportCount(ctx, orgID, actorID(ctx), id)
}

// actorID is the user the movement is recorded for: the request principal,
// or the requester of the worker's job.
func actorID(ctx context.Context) int64 {
	if p, ok := authctx.PrincipalFrom(ctx); ok {
		return p.UserInternal
	}
	if id, ok := orgctx.JobActor(ctx); ok {
		return id
	}
	return 0
}

var (
	_ ioengine.ResourceAdapter = (*MovementsIO)(nil)
	_ ioengine.SampleProvider  = (*MovementsIO)(nil)
	_ ioengine.RowPreviewer    = (*MovementsIO)(nil)
)
