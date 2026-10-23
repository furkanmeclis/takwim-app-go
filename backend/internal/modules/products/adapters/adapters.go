// Package adapters connects the products resource to the shared engines:
// ioengine (export / import), bulkengine (activate / deactivate) and
// searchengine (Cmd+K). The organization always comes from the context
// (orgctx.OrganizationID); without one every adapter call fails.
package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/google/uuid"
)

// Resource is the engine slug of products.
const Resource = model.Slug

// MaxBulkTargets caps the "all matching" bulk scope.
const MaxBulkTargets = 10000

// MaxExportRows caps one export.
const MaxExportRows = 50000

var errNoOrganization = errors.New("products: organization scope is required")

func organizationID(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return 0, errNoOrganization
	}
	return id, nil
}

// FilterFromQuery maps list query params (q, branch, stock_status,
// is_active, show_online) to a filter.
func FilterFromQuery(query map[string]string) model.ListFilter {
	f := model.ListFilter{Q: strings.TrimSpace(query["q"])}
	for _, part := range apiquery.SplitCSV(query["branch"]) {
		if id, err := uuid.Parse(part); err == nil {
			f.Branches = append(f.Branches, id)
		}
	}
	f.StockStatus = apiquery.SplitCSV(query["stock_status"])
	for _, b := range []struct {
		key string
		dst **bool
	}{{"is_active", &f.IsActive}, {"show_online", &f.ShowOnline}} {
		if v, err := strconv.ParseBool(strings.TrimSpace(query[b.key])); err == nil {
			*b.dst = &v
		}
	}
	return f
}

// ---------------------------------------------------------------- io engine

// IO exports and imports products (catalog fields; stock moves through
// the ledger only).
type IO struct {
	svc *usecase.Service
}

// NewIO builds the io adapter.
func NewIO(svc *usecase.Service) *IO {
	return &IO{svc: svc}
}

func (a *IO) Resource() string { return Resource }

func (a *IO) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "sku", LabelKey: "product.sku", Type: ioengine.ColumnTypeString},
		{Key: "name", LabelKey: "product.name", Type: ioengine.ColumnTypeString},
		{Key: "barcode", LabelKey: "product.barcode", Type: ioengine.ColumnTypeString},
		{Key: "unit", LabelKey: "product.unit", Type: ioengine.ColumnTypeEnum},
		{Key: "price", LabelKey: "product.price", Type: ioengine.ColumnTypeString},
		{Key: "cost", LabelKey: "product.cost", Type: ioengine.ColumnTypeString},
		{Key: "currency", LabelKey: "product.currency", Type: ioengine.ColumnTypeString},
		{Key: "low_stock_threshold", LabelKey: "product.low_stock_threshold", Type: ioengine.ColumnTypeString},
		{Key: "stock_on_hand", LabelKey: "product.stock_on_hand", Type: ioengine.ColumnTypeString},
		{Key: "stock_available", LabelKey: "product.stock_available", Type: ioengine.ColumnTypeString},
		{Key: "variants_count", LabelKey: "product.variants_count", Type: ioengine.ColumnTypeString},
		{Key: "branch_codes", LabelKey: "product.branch_codes", Type: ioengine.ColumnTypeString},
		{Key: "show_online", LabelKey: "product.show_online", Type: ioengine.ColumnTypeBoolean},
		{Key: "is_active", LabelKey: "product.is_active", Type: ioengine.ColumnTypeBoolean},
		{Key: "description", LabelKey: "product.description", Type: ioengine.ColumnTypeString},
		{Key: "uuid", LabelKey: "product.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "created_at", LabelKey: "product.created_at", Type: ioengine.ColumnTypeDatetime},
	}
}

func (a *IO) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, err := a.svc.ExportRows(ctx, orgID, FilterFromQuery(query), MaxExportRows)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, p := range rows {
		codes := make([]string, 0, len(p.Offers))
		for _, o := range p.Offers {
			codes = append(codes, o.BranchCode)
		}
		out = append(out, map[string]any{
			"uuid": p.UUID.String(), "sku": p.SKU, "name": p.Name, "barcode": p.Barcode, "unit": p.Unit,
			"price": p.Price.String(), "cost": p.Cost.String(), "currency": p.Currency,
			"low_stock_threshold": p.LowStockThreshold, "stock_on_hand": p.Stock.OnHand,
			"stock_available": p.Stock.Available, "variants_count": p.VariantsCount,
			"branch_codes": strings.Join(codes, ","), "show_online": p.ShowOnline, "is_active": p.IsActive,
			"description": p.Description, "created_at": p.CreatedAt,
		})
	}
	return ioengine.Dataset{Resource: Resource, Columns: a.ExportColumns(), Rows: out}, nil
}

// ImportSchema: a row updates the product with the same SKU, otherwise a
// new product is created. branch_codes add offers to a new product only.
func (a *IO) ImportSchema() []ioengine.ImportField {
	return []ioengine.ImportField{
		{Key: "sku", LabelKey: "product.sku", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "name", LabelKey: "product.name", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "barcode", LabelKey: "product.barcode", Type: ioengine.ColumnTypeString},
		{Key: "unit", LabelKey: "product.unit", Type: ioengine.ColumnTypeEnum, DefaultHint: "piece"},
		{Key: "price", LabelKey: "product.price", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "cost", LabelKey: "product.cost", Type: ioengine.ColumnTypeString, DefaultHint: "0"},
		{Key: "low_stock_threshold", LabelKey: "product.low_stock_threshold", Type: ioengine.ColumnTypeString, DefaultHint: "0"},
		{Key: "branch_codes", LabelKey: "product.branch_codes", Type: ioengine.ColumnTypeString},
		{Key: "show_online", LabelKey: "product.show_online", Type: ioengine.ColumnTypeBoolean, DefaultHint: "true"},
		{Key: "is_active", LabelKey: "product.is_active", Type: ioengine.ColumnTypeBoolean, DefaultHint: "true"},
		{Key: "description", LabelKey: "product.description", Type: ioengine.ColumnTypeString},
	}
}

// SampleRows implements ioengine.SampleProvider.
func (a *IO) SampleRows() []map[string]any {
	return []map[string]any{{
		"sku": "SMP-001", "name": "Onarıcı şampuan 250 ml", "barcode": "8690000000017", "unit": "bottle",
		"price": "320.00", "cost": "180.00", "low_stock_threshold": "3", "branch_codes": "MRK",
		"show_online": "true", "is_active": "true",
	}}
}

// ApplyRow creates a product, or updates the one with the same SKU.
func (a *IO) ApplyRow(ctx context.Context, row map[string]any, defaults map[string]any) (ioengine.RowResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.RowResult{}, err
	}
	in, codes, err := inputFromRow(row, defaults)
	if err != nil {
		return ioengine.RowResult{OK: false, Error: err.Error()}, nil
	}
	got, created, prev, err := a.svc.ImportRow(ctx, orgID, codes, in)
	if err != nil {
		if usecase.IsValidation(err) || errors.Is(err, usecase.ErrNotFound) || errors.Is(err, usecase.ErrCodeTaken) {
			return ioengine.RowResult{OK: false, Error: err.Error()}, nil
		}
		return ioengine.RowResult{}, err
	}
	res := ioengine.RowResult{OK: true, EntityType: "product", EntityUUID: got.String(), Op: "create"}
	if !created && prev != nil {
		res.Op = "update"
		res.Previous = snapshot(*prev)
	}
	return res, nil
}

// RevertRow deletes an imported product or restores an updated one.
func (a *IO) RevertRow(ctx context.Context, entityType, entityUUID string, previous map[string]any) error {
	if entityType != "product" {
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
	if len(previous) == 0 {
		_, err := a.svc.Delete(ctx, orgID, id)
		return err
	}
	raw, err := json.Marshal(previous)
	if err != nil {
		return err
	}
	var prev model.Product
	if err := json.Unmarshal(raw, &prev); err != nil {
		return err
	}
	price, cost := json.Number(prev.Price.String()), json.Number(prev.Cost.String())
	_, err = a.svc.Patch(ctx, orgID, id, model.Input{
		Name: &prev.Name, Description: &prev.Description, SKU: &prev.SKU, Barcode: &prev.Barcode, Unit: &prev.Unit,
		Price: &price, Cost: &cost, LowStockThreshold: &prev.LowStockThreshold, ShowOnline: &prev.ShowOnline,
		IsActive: &prev.IsActive,
	})
	return err
}

func inputFromRow(row, defaults map[string]any) (model.Input, []string, error) {
	get := func(key string) (string, bool) {
		if v := cell(row, key); v != "" {
			return v, true
		}
		if v := cell(defaults, key); v != "" {
			return v, true
		}
		return "", false
	}
	str := func(key string) *string {
		if v, ok := get(key); ok {
			return &v
		}
		return nil
	}
	var in model.Input
	sku, name := cell(row, "sku"), cell(row, "name")
	in.SKU, in.Name = &sku, &name
	in.Barcode = str("barcode")
	in.Description = str("description")
	if v, ok := get("unit"); ok {
		v = strings.ToLower(v)
		in.Unit = &v
	}
	for _, m := range []struct {
		key string
		dst **json.Number
	}{{"price", &in.Price}, {"cost", &in.Cost}} {
		if v, ok := get(m.key); ok {
			n := json.Number(strings.ReplaceAll(v, ",", "."))
			*m.dst = &n
		}
	}
	if v, ok := get("low_stock_threshold"); ok {
		parsed, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			return in, nil, fmt.Errorf("low_stock_threshold must be a whole number")
		}
		t := int32(parsed)
		in.LowStockThreshold = &t
	}
	for _, b := range []struct {
		key string
		dst **bool
	}{{"show_online", &in.ShowOnline}, {"is_active", &in.IsActive}} {
		if v, ok := get(b.key); ok {
			parsed, err := parseBool(b.key, v)
			if err != nil {
				return in, nil, err
			}
			*b.dst = &parsed
		}
	}
	codes, _ := get("branch_codes")
	return in, apiquery.SplitCSV(codes), nil
}

func snapshot(p model.Product) map[string]any {
	p.Offers, p.Variants, p.Levels = nil, nil, nil
	raw, err := json.Marshal(p)
	if err != nil {
		return nil
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

func cell(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func parseBool(key, v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "evet", "aktif", "active":
		return true, nil
	case "0", "false", "no", "hayır", "hayir", "pasif", "inactive":
		return false, nil
	}
	return false, fmt.Errorf("%s must be true or false", key)
}

// -------------------------------------------------------------- bulk engine

// Bulk activates / deactivates products.
type Bulk struct {
	svc *usecase.Service
}

// NewBulk builds the bulk adapter. svc may be nil for metadata only.
func NewBulk(svc *usecase.Service) *Bulk {
	return &Bulk{svc: svc}
}

func (a *Bulk) Resource() string { return Resource }

func (a *Bulk) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{
		{
			ID: "activate", LabelKey: "bulk.actions.products.activate",
			Permission: rbac.PermTenantProductsBulkActivate, Reversible: true,
		},
		{
			ID: "deactivate", LabelKey: "bulk.actions.products.deactivate",
			Permission: rbac.PermTenantProductsBulkDeactivate, Reversible: true,
			ConfirmKey: "bulk.confirm.products.deactivate",
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
		ids, err := a.svc.TargetUUIDs(ctx, orgID, FilterFromQuery(target.Query), MaxBulkTargets)
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
	var active bool
	switch action {
	case "activate":
		active = true
	case "deactivate":
		active = false
	default:
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "unknown action"}, nil
	}
	prev, err := a.svc.SetActive(ctx, orgID, id, active)
	if err != nil {
		if errors.Is(err, usecase.ErrNotFound) {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "not found"}, nil
		}
		return bulkengine.BulkItemResult{}, err
	}
	return bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: "product", OK: true, Op: "update",
		Previous: map[string]any{"is_active": prev},
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
	prev, ok := previous["is_active"].(bool)
	if !ok {
		return fmt.Errorf("missing previous is_active")
	}
	_, err = a.svc.SetActive(ctx, orgID, id, prev)
	return err
}

// ------------------------------------------------------------ search engine

// Search indexes products for the tenant Cmd+K palette.
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
		LabelKey:   "search.specs_products",
		Permission: rbac.PermTenantProductsRead,
		Icon:       "products",
		Searchable: []string{"title", "subtitle", "keywords"},
		Scope:      searchengine.ScopeTenant,
		// The catalog is business level: every reader sees every product.
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
		rows, err := a.q.ListProductSearchDocuments(ctx, orgID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			out = append(out, searchDocument(db.GetProductSearchDocumentRow(row)))
		}
	}
	return out, nil
}

// Document loads one product by its tenant document id.
func (a *Search) Document(ctx context.Context, docID string) (searchengine.Document, error) {
	orgID, raw, err := searchengine.ParseTenantDocumentID(docID)
	if err != nil {
		return searchengine.Document{}, err
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("products search: invalid uuid")
	}
	row, err := a.q.GetProductSearchDocument(ctx, db.GetProductSearchDocumentParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return searchengine.Document{}, err
	}
	return searchDocument(row), nil
}

func searchDocument(row db.GetProductSearchDocumentRow) searchengine.Document {
	return searchengine.Document{
		ID:             searchengine.TenantDocumentID(row.OrganizationID, row.Uuid.String()),
		Spec:           usecase.SearchSpec,
		Title:          row.Name,
		Subtitle:       row.Sku,
		Keywords:       []string{row.Name, row.Sku, row.Barcode},
		Href:           "/t/" + row.OrganizationSlug + "/products/" + row.Uuid.String(),
		Icon:           "products",
		OrganizationID: row.OrganizationID,
	}
}

var (
	_ ioengine.ResourceAdapter = (*IO)(nil)
	_ ioengine.SampleProvider  = (*IO)(nil)
	_ bulkengine.BulkAdapter   = (*Bulk)(nil)
	_ searchengine.Adapter     = (*Search)(nil)
)
