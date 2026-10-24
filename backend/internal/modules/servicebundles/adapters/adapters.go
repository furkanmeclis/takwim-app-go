// Package adapters connects the service bundles to the shared engines:
// ioengine (export / import), bulkengine (activate / deactivate) and
// searchengine (Cmd+K). Bundles belong to the service catalog, so the
// engines use the catalog's permissions (tenant.services.export / import /
// bulk.*). The organization always comes from the context
// (orgctx.OrganizationID); without one every adapter call fails.
package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/servicebundles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/servicebundles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
)

// Resource is the engine slug of service bundles.
const Resource = model.Slug

// ExportQueryKeys are the list filters an export honours.
var ExportQueryKeys = []string{"branch", "is_active", "show_online"}

// MaxBulkTargets caps the "all matching" bulk scope.
const MaxBulkTargets = 10000

// MaxExportRows caps one export.
const MaxExportRows = 50000

// ServicesSeparator joins the service names of a bundle in a file cell.
const ServicesSeparator = " + "

var errNoOrganization = errors.New("service bundles: organization scope is required")

func organizationID(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return 0, errNoOrganization
	}
	return id, nil
}

// FilterFromQuery maps list query params (q, branch, is_active,
// show_online) to a filter. branch lists branch uuids.
func FilterFromQuery(query map[string]string) model.ListFilter {
	f := model.ListFilter{Q: strings.TrimSpace(query["q"])}
	for _, part := range apiquery.SplitCSV(query["branch"]) {
		if id, err := uuid.Parse(part); err == nil {
			f.Branches = append(f.Branches, id)
		}
	}
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

// IO exports and imports bundles.
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
		{Key: "name", LabelKey: "service_bundle.name", Type: ioengine.ColumnTypeString},
		{Key: "services", LabelKey: "service_bundle.services", Type: ioengine.ColumnTypeString},
		{Key: "price", LabelKey: "service_bundle.price", Type: ioengine.ColumnTypeString},
		{Key: "list_price", LabelKey: "service_bundle.list_price", Type: ioengine.ColumnTypeString},
		{Key: "discount", LabelKey: "service_bundle.discount", Type: ioengine.ColumnTypeString},
		{Key: "currency", LabelKey: "service_bundle.currency", Type: ioengine.ColumnTypeString},
		{Key: "duration_minutes", LabelKey: "service_bundle.duration_minutes", Type: ioengine.ColumnTypeString},
		{Key: "branch_codes", LabelKey: "service_bundle.branch_codes", Type: ioengine.ColumnTypeString},
		{Key: "show_online", LabelKey: "service_bundle.show_online", Type: ioengine.ColumnTypeBoolean},
		{Key: "is_active", LabelKey: "service_bundle.is_active", Type: ioengine.ColumnTypeBoolean},
		{Key: "sort_order", LabelKey: "service_bundle.sort_order", Type: ioengine.ColumnTypeString},
		{Key: "description", LabelKey: "service_bundle.description", Type: ioengine.ColumnTypeString},
		{Key: "uuid", LabelKey: "service_bundle.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "created_at", LabelKey: "service_bundle.created_at", Type: ioengine.ColumnTypeDatetime},
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
	for _, b := range rows {
		names := make([]string, 0, len(b.Items))
		for _, it := range b.Items {
			names = append(names, it.ServiceName)
		}
		codes := make([]string, 0, len(b.Branches))
		for _, br := range b.Branches {
			codes = append(codes, br.Code)
		}
		out = append(out, map[string]any{
			"uuid": b.UUID.String(), "name": b.Name, "services": strings.Join(names, ServicesSeparator),
			"price": b.Price.String(), "list_price": b.ListPrice.String(), "discount": b.Discount.String(),
			"currency": b.Currency, "duration_minutes": b.DurationMinutes, "branch_codes": strings.Join(codes, ","),
			"show_online": b.ShowOnline, "is_active": b.IsActive, "sort_order": b.SortOrder,
			"description": b.Description, "created_at": b.CreatedAt,
		})
	}
	return ioengine.Dataset{Resource: Resource, Columns: a.ExportColumns(), Rows: out}, nil
}

// ImportSchema: a row updates the bundle with the same name, otherwise a
// new bundle is created. services names the contents in serving order,
// separated by " + " (or ";" / "|"); branch_codes the branches that offer it.
func (a *IO) ImportSchema() []ioengine.ImportField {
	return []ioengine.ImportField{
		{Key: "name", LabelKey: "service_bundle.name", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "services", LabelKey: "service_bundle.services", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "price", LabelKey: "service_bundle.price", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "branch_codes", LabelKey: "service_bundle.branch_codes", Type: ioengine.ColumnTypeString},
		{Key: "show_online", LabelKey: "service_bundle.show_online", Type: ioengine.ColumnTypeBoolean, DefaultHint: "true"},
		{Key: "is_active", LabelKey: "service_bundle.is_active", Type: ioengine.ColumnTypeBoolean, DefaultHint: "true"},
		{Key: "sort_order", LabelKey: "service_bundle.sort_order", Type: ioengine.ColumnTypeString},
		{Key: "description", LabelKey: "service_bundle.description", Type: ioengine.ColumnTypeString},
	}
}

// SampleRows implements ioengine.SampleProvider.
func (a *IO) SampleRows() []map[string]any {
	return []map[string]any{{
		"name": "İlkbahar paketi", "services": "Boya + Fön", "price": "1000.00", "branch_codes": "MRK",
		"show_online": "true", "is_active": "true", "sort_order": "1", "description": "Boya ve fön birlikte",
	}}
}

// ApplyRow creates a bundle, or updates the one with the same name.
func (a *IO) ApplyRow(ctx context.Context, row map[string]any, defaults map[string]any) (ioengine.RowResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.RowResult{}, err
	}
	in, services, codes, err := inputFromRow(row, defaults)
	if err != nil {
		return ioengine.RowResult{OK: false, Error: err.Error()}, nil
	}
	got, created, prev, err := a.svc.ImportRow(ctx, orgID, services, codes, in)
	if err != nil {
		if usecase.IsValidation(err) || errors.Is(err, usecase.ErrNotFound) || errors.Is(err, usecase.ErrNameTaken) {
			return ioengine.RowResult{OK: false, Error: err.Error()}, nil
		}
		return ioengine.RowResult{}, err
	}
	res := ioengine.RowResult{OK: true, EntityType: "service_bundle", EntityUUID: got.String(), Op: "create"}
	if !created && prev != nil {
		res.Op = "update"
		res.Previous = snapshot(*prev)
	}
	return res, nil
}

// RevertRow deletes an imported bundle or restores an updated one.
func (a *IO) RevertRow(ctx context.Context, entityType, entityUUID string, previous map[string]any) error {
	if entityType != "service_bundle" {
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
	var prev model.Bundle
	if err := json.Unmarshal(raw, &prev); err != nil {
		return err
	}
	in := model.Input{
		Name: &prev.Name, Description: &prev.Description, ShowOnline: &prev.ShowOnline, IsActive: &prev.IsActive,
		SortOrder: &prev.SortOrder, ServiceUUIDs: []uuid.UUID{}, BranchUUIDs: []uuid.UUID{},
	}
	price := json.Number(prev.Price.String())
	in.Price = &price
	for _, it := range prev.Items {
		in.ServiceUUIDs = append(in.ServiceUUIDs, it.ServiceUUID)
	}
	for _, br := range prev.Branches {
		in.BranchUUIDs = append(in.BranchUUIDs, br.UUID)
	}
	_, err = a.svc.Patch(ctx, orgID, id, in)
	return err
}

// splitServices splits a services cell on " + ", ";" or "|".
func splitServices(v string) []string {
	v = strings.NewReplacer(" + ", ";", "+", ";", "|", ";").Replace(v)
	parts := strings.Split(v, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func inputFromRow(row, defaults map[string]any) (model.Input, []string, []string, error) {
	get := func(key string) (string, bool) {
		if v := cell(row, key); v != "" {
			return v, true
		}
		if v := cell(defaults, key); v != "" {
			return v, true
		}
		return "", false
	}
	var in model.Input
	name := cell(row, "name")
	in.Name = &name
	if v, ok := get("description"); ok {
		in.Description = &v
	}
	if v, ok := get("price"); ok {
		n := json.Number(strings.ReplaceAll(v, ",", "."))
		in.Price = &n
	}
	if v, ok := get("sort_order"); ok {
		parsed, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			return in, nil, nil, fmt.Errorf("sort_order must be a whole number")
		}
		p := int32(parsed)
		in.SortOrder = &p
	}
	for _, b := range []struct {
		key string
		dst **bool
	}{{"show_online", &in.ShowOnline}, {"is_active", &in.IsActive}} {
		if v, ok := get(b.key); ok {
			parsed, err := parseBool(b.key, v)
			if err != nil {
				return in, nil, nil, err
			}
			*b.dst = &parsed
		}
	}
	var services, codes []string
	if v, ok := get("services"); ok {
		services = splitServices(v)
	}
	if v, ok := get("branch_codes"); ok {
		codes = apiquery.SplitCSV(v)
	}
	return in, services, codes, nil
}

func snapshot(b model.Bundle) map[string]any {
	raw, err := json.Marshal(b)
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

// Bulk activates / deactivates bundles.
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
			ID: "activate", LabelKey: "bulk.actions.service_bundles.activate",
			Permission: rbac.PermTenantServicesBulkActivate, Reversible: true,
		},
		{
			ID: "deactivate", LabelKey: "bulk.actions.service_bundles.deactivate",
			Permission: rbac.PermTenantServicesBulkDeactivate, Reversible: true,
			ConfirmKey: "bulk.confirm.service_bundles.deactivate",
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
		EntityUUID: entityUUID, EntityType: "service_bundle", OK: true, Op: "update",
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

// Search indexes bundles for the tenant Cmd+K palette.
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
		LabelKey:   "search.specs_service_bundles",
		Permission: rbac.PermTenantServicesRead,
		Icon:       "bundles",
		Searchable: []string{"title", "subtitle", "keywords"},
		Scope:      searchengine.ScopeTenant,
		// The catalog is business level: every reader sees every bundle.
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
		rows, err := a.q.ListServiceBundleSearchDocuments(ctx, orgID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			out = append(out, searchDocument(db.GetServiceBundleSearchDocumentRow(row)))
		}
	}
	return out, nil
}

// Document loads one bundle by its tenant document id.
func (a *Search) Document(ctx context.Context, docID string) (searchengine.Document, error) {
	orgID, raw, err := searchengine.ParseTenantDocumentID(docID)
	if err != nil {
		return searchengine.Document{}, err
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("service bundles search: invalid uuid")
	}
	row, err := a.q.GetServiceBundleSearchDocument(ctx, db.GetServiceBundleSearchDocumentParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return searchengine.Document{}, err
	}
	return searchDocument(row), nil
}

func searchDocument(row db.GetServiceBundleSearchDocumentRow) searchengine.Document {
	return searchengine.Document{
		ID:             searchengine.TenantDocumentID(row.OrganizationID, row.Uuid.String()),
		Spec:           usecase.SearchSpec,
		Title:          row.Name,
		Subtitle:       row.Items,
		Keywords:       []string{row.Name, row.Items, row.Description},
		Href:           "/t/" + row.OrganizationSlug + "/service-bundles/" + row.Uuid.String(),
		Icon:           "bundles",
		OrganizationID: row.OrganizationID,
	}
}

var (
	_ ioengine.ResourceAdapter = (*IO)(nil)
	_ ioengine.SampleProvider  = (*IO)(nil)
	_ bulkengine.BulkAdapter   = (*Bulk)(nil)
	_ searchengine.Adapter     = (*Search)(nil)
)
