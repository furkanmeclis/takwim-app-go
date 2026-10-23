// Package adapters connects the services resource to the shared engines:
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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/google/uuid"
)

// Resource is the engine slug of services.
const Resource = model.Slug

// MaxBulkTargets caps the "all matching" bulk scope.
const MaxBulkTargets = 10000

// MaxExportRows caps one export.
const MaxExportRows = 50000

var errNoOrganization = errors.New("services: organization scope is required")

func organizationID(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return 0, errNoOrganization
	}
	return id, nil
}

// FilterFromQuery maps list query params (q, category, branch,
// payment_mode, is_active, show_online) to a filter. category lists
// category uuids and "none"; branch lists branch uuids.
func FilterFromQuery(query map[string]string) model.ListFilter {
	f := model.ListFilter{Q: strings.TrimSpace(query["q"])}
	for _, part := range apiquery.SplitCSV(query["category"]) {
		if part == model.UncategorizedFacet {
			f.Uncategorized = true
		} else if id, err := uuid.Parse(part); err == nil {
			f.Categories = append(f.Categories, id)
		}
	}
	for _, part := range apiquery.SplitCSV(query["branch"]) {
		if id, err := uuid.Parse(part); err == nil {
			f.Branches = append(f.Branches, id)
		}
	}
	f.PaymentModes = apiquery.SplitCSV(query["payment_mode"])
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

// IO exports and imports services.
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
		{Key: "category", LabelKey: "service.category", Type: ioengine.ColumnTypeString},
		{Key: "name", LabelKey: "service.name", Type: ioengine.ColumnTypeString},
		{Key: "duration_minutes", LabelKey: "service.duration_minutes", Type: ioengine.ColumnTypeString},
		{Key: "buffer_before_minutes", LabelKey: "service.buffer_before_minutes", Type: ioengine.ColumnTypeString},
		{Key: "buffer_after_minutes", LabelKey: "service.buffer_after_minutes", Type: ioengine.ColumnTypeString},
		{Key: "price", LabelKey: "service.price", Type: ioengine.ColumnTypeString},
		{Key: "currency", LabelKey: "service.currency", Type: ioengine.ColumnTypeString},
		{Key: "payment_mode", LabelKey: "service.payment_mode", Type: ioengine.ColumnTypeEnum},
		{Key: "staff_mode", LabelKey: "service.staff_mode", Type: ioengine.ColumnTypeEnum},
		{Key: "resource_kinds", LabelKey: "service.resource_kinds", Type: ioengine.ColumnTypeString},
		{Key: "branch_codes", LabelKey: "service.branch_codes", Type: ioengine.ColumnTypeString},
		{Key: "show_online", LabelKey: "service.show_online", Type: ioengine.ColumnTypeBoolean},
		{Key: "is_active", LabelKey: "service.is_active", Type: ioengine.ColumnTypeBoolean},
		{Key: "sort_order", LabelKey: "service.sort_order", Type: ioengine.ColumnTypeString},
		{Key: "description", LabelKey: "service.description", Type: ioengine.ColumnTypeString},
		{Key: "uuid", LabelKey: "service.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "created_at", LabelKey: "service.created_at", Type: ioengine.ColumnTypeDatetime},
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
	for _, s := range rows {
		category := ""
		if s.Category != nil {
			category = s.Category.Name
		}
		codes := make([]string, 0, len(s.Offers))
		for _, o := range s.Offers {
			codes = append(codes, o.BranchCode)
		}
		out = append(out, map[string]any{
			"uuid": s.UUID.String(), "category": category, "name": s.Name,
			"duration_minutes": s.DurationMinutes, "buffer_before_minutes": s.BufferBeforeMinutes,
			"buffer_after_minutes": s.BufferAfterMinutes, "price": s.Price.String(), "currency": s.Currency,
			"payment_mode": s.PaymentMode, "staff_mode": s.StaffMode,
			"resource_kinds": strings.Join(s.ResourceKinds, ","), "branch_codes": strings.Join(codes, ","),
			"show_online": s.ShowOnline, "is_active": s.IsActive, "sort_order": s.SortOrder,
			"description": s.Description, "created_at": s.CreatedAt,
		})
	}
	return ioengine.Dataset{Resource: Resource, Columns: a.ExportColumns(), Rows: out}, nil
}

// ImportSchema: a row updates the service with the same name, otherwise a
// new service is created. branch_codes add offers to a new service only.
func (a *IO) ImportSchema() []ioengine.ImportField {
	return []ioengine.ImportField{
		{Key: "category", LabelKey: "service.category", Type: ioengine.ColumnTypeString},
		{Key: "name", LabelKey: "service.name", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "duration_minutes", LabelKey: "service.duration_minutes", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "buffer_before_minutes", LabelKey: "service.buffer_before_minutes", Type: ioengine.ColumnTypeString, DefaultHint: "0"},
		{Key: "buffer_after_minutes", LabelKey: "service.buffer_after_minutes", Type: ioengine.ColumnTypeString, DefaultHint: "0"},
		{Key: "price", LabelKey: "service.price", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "payment_mode", LabelKey: "service.payment_mode", Type: ioengine.ColumnTypeEnum, DefaultHint: "on_site"},
		{Key: "staff_mode", LabelKey: "service.staff_mode", Type: ioengine.ColumnTypeEnum},
		{Key: "resource_kinds", LabelKey: "service.resource_kinds", Type: ioengine.ColumnTypeString},
		{Key: "branch_codes", LabelKey: "service.branch_codes", Type: ioengine.ColumnTypeString},
		{Key: "show_online", LabelKey: "service.show_online", Type: ioengine.ColumnTypeBoolean, DefaultHint: "true"},
		{Key: "is_active", LabelKey: "service.is_active", Type: ioengine.ColumnTypeBoolean, DefaultHint: "true"},
		{Key: "sort_order", LabelKey: "service.sort_order", Type: ioengine.ColumnTypeString},
		{Key: "description", LabelKey: "service.description", Type: ioengine.ColumnTypeString},
	}
}

// SampleRows implements ioengine.SampleProvider.
func (a *IO) SampleRows() []map[string]any {
	return []map[string]any{{
		"category": "Saç", "name": "Saç kesimi", "duration_minutes": "30", "buffer_before_minutes": "0",
		"buffer_after_minutes": "10", "price": "450.00", "payment_mode": "on_site", "staff_mode": "required",
		"resource_kinds": "chair", "branch_codes": "MRK", "show_online": "true", "is_active": "true", "sort_order": "1",
	}}
}

// ApplyRow creates a service, or updates the one with the same name.
func (a *IO) ApplyRow(ctx context.Context, row map[string]any, defaults map[string]any) (ioengine.RowResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.RowResult{}, err
	}
	in, category, codes, err := inputFromRow(row, defaults)
	if err != nil {
		return ioengine.RowResult{OK: false, Error: err.Error()}, nil
	}
	got, created, prev, err := a.svc.ImportRow(ctx, orgID, category, codes, in)
	if err != nil {
		if usecase.IsValidation(err) || errors.Is(err, usecase.ErrNotFound) ||
			errors.Is(err, usecase.ErrNameTaken) || errors.Is(err, usecase.ErrCategoryNameTaken) {
			return ioengine.RowResult{OK: false, Error: err.Error()}, nil
		}
		return ioengine.RowResult{}, err
	}
	res := ioengine.RowResult{OK: true, EntityType: "service", EntityUUID: got.String(), Op: "create"}
	if !created && prev != nil {
		res.Op = "update"
		res.Previous = snapshot(*prev)
	}
	return res, nil
}

// RevertRow deletes an imported service or restores an updated one.
func (a *IO) RevertRow(ctx context.Context, entityType, entityUUID string, previous map[string]any) error {
	if entityType != "service" {
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
	var prev model.Service
	if err := json.Unmarshal(raw, &prev); err != nil {
		return err
	}
	in := model.Input{
		Name: &prev.Name, Description: &prev.Description, DurationMinutes: &prev.DurationMinutes,
		BufferBeforeMinutes: &prev.BufferBeforeMinutes, BufferAfterMinutes: &prev.BufferAfterMinutes,
		PaymentMode: &prev.PaymentMode, StaffMode: &prev.StaffMode, ResourceKinds: &prev.ResourceKinds,
		ShowOnline: &prev.ShowOnline, IsActive: &prev.IsActive, SortOrder: &prev.SortOrder,
	}
	price := json.Number(prev.Price.String())
	in.Price = &price
	in.CategoryUUID = model.OptionalUUID{Set: true}
	if prev.Category != nil {
		in.CategoryUUID.Value = &prev.Category.UUID
	}
	_, err = a.svc.Patch(ctx, orgID, id, in)
	return err
}

func inputFromRow(row, defaults map[string]any) (model.Input, string, []string, error) {
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
	name := cell(row, "name")
	in.Name = &name
	in.Description = str("description")
	for _, e := range []struct {
		key string
		dst **string
	}{{"payment_mode", &in.PaymentMode}, {"staff_mode", &in.StaffMode}} {
		if v, ok := get(e.key); ok {
			v = strings.ToLower(v)
			*e.dst = &v
		}
	}
	if v, ok := get("price"); ok {
		n := json.Number(strings.ReplaceAll(v, ",", "."))
		in.Price = &n
	}
	if v, ok := get("resource_kinds"); ok {
		kinds := apiquery.SplitCSV(strings.ToLower(v))
		in.ResourceKinds = &kinds
	}
	for _, n := range []struct {
		key string
		dst **int32
	}{
		{"duration_minutes", &in.DurationMinutes}, {"buffer_before_minutes", &in.BufferBeforeMinutes},
		{"buffer_after_minutes", &in.BufferAfterMinutes}, {"sort_order", &in.SortOrder},
	} {
		if v, ok := get(n.key); ok {
			parsed, err := strconv.ParseInt(v, 10, 32)
			if err != nil {
				return in, "", nil, fmt.Errorf("%s must be a whole number", n.key)
			}
			p := int32(parsed)
			*n.dst = &p
		}
	}
	for _, b := range []struct {
		key string
		dst **bool
	}{{"show_online", &in.ShowOnline}, {"is_active", &in.IsActive}} {
		if v, ok := get(b.key); ok {
			parsed, err := parseBool(b.key, v)
			if err != nil {
				return in, "", nil, err
			}
			*b.dst = &parsed
		}
	}
	category, _ := get("category")
	codes, _ := get("branch_codes")
	return in, category, apiquery.SplitCSV(codes), nil
}

func snapshot(s model.Service) map[string]any {
	s.Offers, s.Staff = nil, nil
	raw, err := json.Marshal(s)
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

// Bulk activates / deactivates services.
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
			ID: "activate", LabelKey: "bulk.actions.services.activate",
			Permission: rbac.PermTenantServicesBulkActivate, Reversible: true,
		},
		{
			ID: "deactivate", LabelKey: "bulk.actions.services.deactivate",
			Permission: rbac.PermTenantServicesBulkDeactivate, Reversible: true,
			ConfirmKey: "bulk.confirm.services.deactivate",
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
		EntityUUID: entityUUID, EntityType: "service", OK: true, Op: "update",
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

// Search indexes services for the tenant Cmd+K palette.
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
		LabelKey:   "search.specs_services",
		Permission: rbac.PermTenantServicesRead,
		Icon:       "services",
		Searchable: []string{"title", "subtitle", "keywords"},
		Scope:      searchengine.ScopeTenant,
		// The catalog is business level: every reader sees every service.
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
		rows, err := a.q.ListServiceSearchDocuments(ctx, orgID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			out = append(out, searchDocument(db.GetServiceSearchDocumentRow(row)))
		}
	}
	return out, nil
}

// Document loads one service by its tenant document id.
func (a *Search) Document(ctx context.Context, docID string) (searchengine.Document, error) {
	orgID, raw, err := searchengine.ParseTenantDocumentID(docID)
	if err != nil {
		return searchengine.Document{}, err
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("services search: invalid uuid")
	}
	row, err := a.q.GetServiceSearchDocument(ctx, db.GetServiceSearchDocumentParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return searchengine.Document{}, err
	}
	return searchDocument(row), nil
}

func searchDocument(row db.GetServiceSearchDocumentRow) searchengine.Document {
	return searchengine.Document{
		ID:             searchengine.TenantDocumentID(row.OrganizationID, row.Uuid.String()),
		Spec:           usecase.SearchSpec,
		Title:          row.Name,
		Subtitle:       row.CategoryName.String,
		Keywords:       []string{row.Name, row.CategoryName.String, row.Description},
		Href:           "/t/" + row.OrganizationSlug + "/services/" + row.Uuid.String(),
		Icon:           "services",
		OrganizationID: row.OrganizationID,
	}
}

var (
	_ ioengine.ResourceAdapter = (*IO)(nil)
	_ ioengine.SampleProvider  = (*IO)(nil)
	_ bulkengine.BulkAdapter   = (*Bulk)(nil)
	_ searchengine.Adapter     = (*Search)(nil)
)
