// Package adapters connects the resources resource to the shared engines:
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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/google/uuid"
)

// Resource is the engine slug of resources.
const Resource = model.Slug

// MaxBulkTargets caps the "all matching" bulk scope.
const MaxBulkTargets = 10000

var errNoOrganization = errors.New("resources: organization scope is required")

func organizationID(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return 0, errNoOrganization
	}
	return id, nil
}

// FilterFromQuery maps list query params (q, branch, kind, zone, is_active,
// show_online) to a filter. branch lists branch uuids.
func FilterFromQuery(query map[string]string) model.ListFilter {
	f := model.ListFilter{Q: strings.TrimSpace(query["q"])}
	for _, part := range apiquery.SplitCSV(query["branch"]) {
		if id, err := uuid.Parse(part); err == nil {
			f.Branches = append(f.Branches, id)
		}
	}
	f.Kinds = apiquery.SplitCSV(query["kind"])
	f.Zones = apiquery.SplitCSV(query["zone"])
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

// IO exports and imports resources.
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
		{Key: "branch_code", LabelKey: "resource.branch_code", Type: ioengine.ColumnTypeString},
		{Key: "branch", LabelKey: "resource.branch", Type: ioengine.ColumnTypeString},
		{Key: "kind", LabelKey: "resource.kind", Type: ioengine.ColumnTypeEnum},
		{Key: "name", LabelKey: "resource.name", Type: ioengine.ColumnTypeString},
		{Key: "zone", LabelKey: "resource.zone", Type: ioengine.ColumnTypeString},
		{Key: "min_capacity", LabelKey: "resource.min_capacity", Type: ioengine.ColumnTypeString},
		{Key: "capacity", LabelKey: "resource.capacity", Type: ioengine.ColumnTypeString},
		{Key: "combinable", LabelKey: "resource.combinable", Type: ioengine.ColumnTypeBoolean},
		{Key: "show_online", LabelKey: "resource.show_online", Type: ioengine.ColumnTypeBoolean},
		{Key: "is_active", LabelKey: "resource.is_active", Type: ioengine.ColumnTypeBoolean},
		{Key: "sort_order", LabelKey: "resource.sort_order", Type: ioengine.ColumnTypeString},
		{Key: "description", LabelKey: "resource.description", Type: ioengine.ColumnTypeString},
		{Key: "uuid", LabelKey: "resource.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "created_at", LabelKey: "resource.created_at", Type: ioengine.ColumnTypeDatetime},
	}
}

func (a *IO) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, err := a.svc.ExportRows(ctx, orgID, FilterFromQuery(query))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{
			"uuid": r.UUID.String(), "branch_code": r.Branch.Code, "branch": r.Branch.Name, "kind": r.Kind,
			"name": r.Name, "zone": r.Zone, "min_capacity": r.MinCapacity, "capacity": r.Capacity,
			"combinable": r.Combinable, "show_online": r.ShowOnline, "is_active": r.IsActive,
			"sort_order": r.SortOrder, "description": r.Description, "created_at": r.CreatedAt,
		})
	}
	return ioengine.Dataset{Resource: Resource, Columns: a.ExportColumns(), Rows: out}, nil
}

// ImportSchema: a row updates the resource of the branch with the same
// name, otherwise a new resource is created in the branch.
func (a *IO) ImportSchema() []ioengine.ImportField {
	return []ioengine.ImportField{
		{Key: "branch_code", LabelKey: "resource.branch_code", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "kind", LabelKey: "resource.kind", Type: ioengine.ColumnTypeEnum, Required: true},
		{Key: "name", LabelKey: "resource.name", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "zone", LabelKey: "resource.zone", Type: ioengine.ColumnTypeString},
		{Key: "min_capacity", LabelKey: "resource.min_capacity", Type: ioengine.ColumnTypeString, DefaultHint: "1"},
		{Key: "capacity", LabelKey: "resource.capacity", Type: ioengine.ColumnTypeString, DefaultHint: "1"},
		{Key: "combinable", LabelKey: "resource.combinable", Type: ioengine.ColumnTypeBoolean, DefaultHint: "false"},
		{Key: "show_online", LabelKey: "resource.show_online", Type: ioengine.ColumnTypeBoolean, DefaultHint: "true"},
		{Key: "is_active", LabelKey: "resource.is_active", Type: ioengine.ColumnTypeBoolean, DefaultHint: "true"},
		{Key: "sort_order", LabelKey: "resource.sort_order", Type: ioengine.ColumnTypeString},
		{Key: "description", LabelKey: "resource.description", Type: ioengine.ColumnTypeString},
	}
}

// SampleRows implements ioengine.SampleProvider.
func (a *IO) SampleRows() []map[string]any {
	return []map[string]any{{
		"branch_code": "MRK", "kind": "table", "name": "Masa 1", "zone": "Teras", "min_capacity": "2",
		"capacity": "4", "combinable": "true", "show_online": "true", "is_active": "true", "sort_order": "1",
	}}
}

// ApplyRow creates a resource, or updates the one with the same name in
// the branch.
func (a *IO) ApplyRow(ctx context.Context, row map[string]any, defaults map[string]any) (ioengine.RowResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.RowResult{}, err
	}
	in, branchCode, err := inputFromRow(row, defaults)
	if err != nil {
		return ioengine.RowResult{OK: false, Error: err.Error()}, nil
	}
	got, created, prev, err := a.svc.ImportRow(ctx, orgID, branchCode, in)
	if err != nil {
		if usecase.IsValidation(err) || errors.Is(err, usecase.ErrNotFound) || errors.Is(err, usecase.ErrNameTaken) {
			return ioengine.RowResult{OK: false, Error: err.Error()}, nil
		}
		return ioengine.RowResult{}, err
	}
	res := ioengine.RowResult{OK: true, EntityType: "resource", EntityUUID: got.String(), Op: "create"}
	if !created && prev != nil {
		res.Op = "update"
		res.Previous = snapshot(*prev)
	}
	return res, nil
}

// RevertRow deletes an imported resource or restores an updated one.
func (a *IO) RevertRow(ctx context.Context, entityType, entityUUID string, previous map[string]any) error {
	if entityType != "resource" {
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
	var prev model.Resource
	if err := json.Unmarshal(raw, &prev); err != nil {
		return err
	}
	_, err = a.svc.Patch(ctx, orgID, id, model.Input{
		BranchUUID: &prev.Branch.UUID, Kind: &prev.Kind, Name: &prev.Name, Zone: &prev.Zone,
		Description: &prev.Description, MinCapacity: &prev.MinCapacity, Capacity: &prev.Capacity,
		Combinable: &prev.Combinable, ShowOnline: &prev.ShowOnline, IsActive: &prev.IsActive, SortOrder: &prev.SortOrder,
	})
	return err
}

func inputFromRow(row, defaults map[string]any) (model.Input, string, error) {
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
	in.Zone, in.Description = str("zone"), str("description")
	if v, ok := get("kind"); ok {
		kind := strings.ToLower(v)
		in.Kind = &kind
	}
	for _, n := range []struct {
		key string
		dst **int32
	}{{"min_capacity", &in.MinCapacity}, {"capacity", &in.Capacity}, {"sort_order", &in.SortOrder}} {
		if v, ok := get(n.key); ok {
			parsed, err := strconv.ParseInt(v, 10, 32)
			if err != nil {
				return in, "", fmt.Errorf("%s must be a whole number", n.key)
			}
			p := int32(parsed)
			*n.dst = &p
		}
	}
	for _, b := range []struct {
		key string
		dst **bool
	}{{"combinable", &in.Combinable}, {"show_online", &in.ShowOnline}, {"is_active", &in.IsActive}} {
		if v, ok := get(b.key); ok {
			parsed, err := parseBool(b.key, v)
			if err != nil {
				return in, "", err
			}
			*b.dst = &parsed
		}
	}
	code, _ := get("branch_code")
	if code == "" {
		return in, "", fmt.Errorf("branch_code is required")
	}
	return in, code, nil
}

func snapshot(r model.Resource) map[string]any {
	r.BranchHours = nil
	raw, err := json.Marshal(r)
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

// Bulk activates / deactivates resources.
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
			ID: "activate", LabelKey: "bulk.actions.resources.activate",
			Permission: rbac.PermTenantResourcesBulkActivate, Reversible: true,
		},
		{
			ID: "deactivate", LabelKey: "bulk.actions.resources.deactivate",
			Permission: rbac.PermTenantResourcesBulkDeactivate, Reversible: true,
			ConfirmKey: "bulk.confirm.resources.deactivate",
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
		EntityUUID: entityUUID, EntityType: "resource", OK: true, Op: "update",
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

// Search indexes resources for the tenant Cmd+K palette.
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
		LabelKey:   "search.specs_resources",
		Permission: rbac.PermTenantResourcesRead,
		Icon:       "resources",
		Searchable: []string{"title", "subtitle", "keywords"},
		Scope:      searchengine.ScopeTenant,
		// Documents carry the resource's branch; branch-limited members
		// find the resources of their branches only.
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
		rows, err := a.q.ListResourceSearchDocuments(ctx, orgID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			out = append(out, searchDocument(db.GetResourceSearchDocumentRow(row)))
		}
	}
	return out, nil
}

// Document loads one resource by its tenant document id.
func (a *Search) Document(ctx context.Context, docID string) (searchengine.Document, error) {
	orgID, raw, err := searchengine.ParseTenantDocumentID(docID)
	if err != nil {
		return searchengine.Document{}, err
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("resources search: invalid uuid")
	}
	row, err := a.q.GetResourceSearchDocument(ctx, db.GetResourceSearchDocumentParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return searchengine.Document{}, err
	}
	return searchDocument(row), nil
}

func searchDocument(row db.GetResourceSearchDocumentRow) searchengine.Document {
	subtitle := row.BranchName
	if row.Zone != "" {
		subtitle += " · " + row.Zone
	}
	return searchengine.Document{
		ID:             searchengine.TenantDocumentID(row.OrganizationID, row.Uuid.String()),
		Spec:           usecase.SearchSpec,
		Title:          row.Name,
		Subtitle:       subtitle,
		Keywords:       []string{row.Name, row.Zone, row.BranchName, row.Kind},
		Href:           "/t/" + row.OrganizationSlug + "/resources/" + row.Uuid.String(),
		Icon:           "resources",
		OrganizationID: row.OrganizationID,
		BranchIDs:      []int64{row.BranchID},
	}
}

var (
	_ ioengine.ResourceAdapter = (*IO)(nil)
	_ ioengine.SampleProvider  = (*IO)(nil)
	_ bulkengine.BulkAdapter   = (*Bulk)(nil)
	_ searchengine.Adapter     = (*Search)(nil)
)
