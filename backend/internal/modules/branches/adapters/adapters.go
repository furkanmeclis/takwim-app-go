// Package adapters connects the branches resource to the shared engines:
// ioengine (export / import), bulkengine (activate / deactivate) and
// searchengine (Cmd+K). The organization always comes from the context
// (orgctx.OrganizationID): the request scope on tenant routes, the job row in
// the worker. Without one every adapter call fails.
package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/google/uuid"
)

// Resource is the engine slug of branches.
const Resource = model.Resource

// MaxBulkTargets caps the "all matching" bulk scope.
const MaxBulkTargets = 10000

var errNoOrganization = errors.New("branches: organization scope is required")

func organizationID(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return 0, errNoOrganization
	}
	return id, nil
}

// FilterFromQuery maps list query params (q, city, is_active) to a filter.
// city may list several comma-separated cities.
func FilterFromQuery(query map[string]string) model.ListFilter {
	f := model.ListFilter{Q: strings.TrimSpace(query["q"]), Cities: SplitCities(query["city"])}
	if v, err := strconv.ParseBool(strings.TrimSpace(query["is_active"])); err == nil {
		f.IsActive = &v
	}
	return f
}

// SplitCities parses a comma-separated city filter.
func SplitCities(raw string) []string {
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// ---------------------------------------------------------------- io engine

// IO exports and imports branches.
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
		{Key: "code", LabelKey: "branches.code", Type: ioengine.ColumnTypeString},
		{Key: "name", LabelKey: "branches.name", Type: ioengine.ColumnTypeString},
		{Key: "city", LabelKey: "branches.city", Type: ioengine.ColumnTypeString},
		{Key: "district", LabelKey: "branches.district", Type: ioengine.ColumnTypeString},
		{Key: "address", LabelKey: "branches.address", Type: ioengine.ColumnTypeString},
		{Key: "phone", LabelKey: "branches.phone", Type: ioengine.ColumnTypeString},
		{Key: "email", LabelKey: "branches.email", Type: ioengine.ColumnTypeString},
		{Key: "latitude", LabelKey: "branches.latitude", Type: ioengine.ColumnTypeString},
		{Key: "longitude", LabelKey: "branches.longitude", Type: ioengine.ColumnTypeString},
		{Key: "prep_minutes", LabelKey: "branches.prep_minutes", Type: ioengine.ColumnTypeString},
		{Key: "is_active", LabelKey: "branches.is_active", Type: ioengine.ColumnTypeBoolean},
		{Key: "uuid", LabelKey: "branches.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "created_at", LabelKey: "branches.created_at", Type: ioengine.ColumnTypeDatetime},
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
	for _, b := range rows {
		out = append(out, map[string]any{
			"uuid": b.UUID.String(), "code": b.Code, "name": b.Name, "city": b.City, "district": b.District,
			"address": b.Address, "phone": b.Phone, "email": b.Email,
			"latitude": floatString(b.Latitude), "longitude": floatString(b.Longitude),
			"prep_minutes": strconv.Itoa(int(b.PrepMinutes)), "is_active": b.IsActive,
			"created_at": b.CreatedAt,
		})
	}
	return ioengine.Dataset{Resource: Resource, Columns: a.ExportColumns(), Rows: out}, nil
}

func (a *IO) ImportSchema() []ioengine.ImportField {
	return []ioengine.ImportField{
		{Key: "code", LabelKey: "branches.code", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "name", LabelKey: "branches.name", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "city", LabelKey: "branches.city", Type: ioengine.ColumnTypeString},
		{Key: "district", LabelKey: "branches.district", Type: ioengine.ColumnTypeString},
		{Key: "address", LabelKey: "branches.address", Type: ioengine.ColumnTypeString},
		{Key: "phone", LabelKey: "branches.phone", Type: ioengine.ColumnTypeString},
		{Key: "email", LabelKey: "branches.email", Type: ioengine.ColumnTypeString},
		{Key: "latitude", LabelKey: "branches.latitude", Type: ioengine.ColumnTypeString},
		{Key: "longitude", LabelKey: "branches.longitude", Type: ioengine.ColumnTypeString},
		{Key: "prep_minutes", LabelKey: "branches.prep_minutes", Type: ioengine.ColumnTypeString, DefaultHint: "60"},
		{Key: "is_active", LabelKey: "branches.is_active", Type: ioengine.ColumnTypeBoolean, DefaultHint: "true"},
	}
}

// SampleRows implements ioengine.SampleProvider.
func (a *IO) SampleRows() []map[string]any {
	return []map[string]any{{
		"code": "IST01", "name": "İstanbul Havalimanı", "city": "İstanbul", "district": "Arnavutköy",
		"address": "Tayakadın, Terminal Cd. No:1", "phone": "+90 212 000 00 00", "email": "ist@example.com",
		"latitude": "41.275278", "longitude": "28.751944", "prep_minutes": "45", "is_active": "true",
	}}
}

// ApplyRow creates a branch, or updates the branch with the same code.
func (a *IO) ApplyRow(ctx context.Context, row map[string]any, defaults map[string]any) (ioengine.RowResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.RowResult{}, err
	}
	in, err := inputFromRow(row, defaults)
	if err != nil {
		return ioengine.RowResult{OK: false, Error: err.Error()}, nil
	}
	id, created, prev, err := a.svc.ImportRow(ctx, orgID, in)
	if err != nil {
		var ve *usecase.ValidationError
		if errors.As(err, &ve) || errors.Is(err, usecase.ErrCodeTaken) ||
			errors.Is(err, usecase.ErrNotFound) || errors.Is(err, usecase.ErrAllBranchesRequired) {
			return ioengine.RowResult{OK: false, Error: err.Error()}, nil
		}
		return ioengine.RowResult{}, err
	}
	res := ioengine.RowResult{OK: true, EntityType: "branch", EntityUUID: id.String(), Op: "create"}
	if !created && prev != nil {
		res.Op = "update"
		res.Previous = branchSnapshot(*prev)
	}
	return res, nil
}

// RevertRow deletes an imported branch or restores an updated one.
func (a *IO) RevertRow(ctx context.Context, entityType, entityUUID string, previous map[string]any) error {
	if entityType != "branch" {
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
		return a.svc.Delete(ctx, orgID, id)
	}
	raw, err := json.Marshal(previous)
	if err != nil {
		return err
	}
	var prev model.Branch
	if err := json.Unmarshal(raw, &prev); err != nil {
		return err
	}
	_, err = a.svc.Patch(ctx, orgID, id, inputFromBranch(prev))
	return err
}

func inputFromRow(row, defaults map[string]any) (model.BranchInput, error) {
	get := func(key string) (string, bool) {
		if v := strings.TrimSpace(cell(row, key)); v != "" {
			return v, true
		}
		if v := strings.TrimSpace(cell(defaults, key)); v != "" {
			return v, true
		}
		return "", false
	}
	var in model.BranchInput
	str := func(key string) *string {
		if v, ok := get(key); ok {
			return &v
		}
		return nil
	}
	code, name := cell(row, "code"), cell(row, "name")
	in.Code, in.Name = &code, &name
	in.City, in.District, in.Address = str("city"), str("district"), str("address")
	in.Phone, in.Email = str("phone"), str("email")
	for _, coord := range []struct {
		key string
		dst *model.OptionalFloat
	}{{"latitude", &in.Latitude}, {"longitude", &in.Longitude}} {
		if v, ok := get(coord.key); ok {
			f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
			if err != nil {
				return in, fmt.Errorf("%s is not a number", coord.key)
			}
			*coord.dst = model.OptionalFloat{Set: true, Value: &f}
		}
	}
	if v, ok := get("prep_minutes"); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return in, fmt.Errorf("prep_minutes is not a whole number")
		}
		m := int32(n)
		in.PrepMinutes = &m
	}
	if v, ok := get("is_active"); ok {
		b, err := parseBool(v)
		if err != nil {
			return in, err
		}
		in.IsActive = &b
	}
	return in, nil
}

func inputFromBranch(b model.Branch) model.BranchInput {
	hours := b.WorkingHours
	holidays := b.HolidayExceptions
	return model.BranchInput{
		Code: &b.Code, Name: &b.Name, Address: &b.Address, City: &b.City, District: &b.District,
		Latitude: model.OptionalFloat{Set: true, Value: b.Latitude}, Longitude: model.OptionalFloat{Set: true, Value: b.Longitude},
		Phone: &b.Phone, Email: &b.Email, WorkingHours: &hours, HolidayExceptions: &holidays,
		PrepMinutes: &b.PrepMinutes, IsActive: &b.IsActive,
	}
}

func branchSnapshot(b model.Branch) map[string]any {
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

func parseBool(v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "evet", "aktif", "active":
		return true, nil
	case "0", "false", "no", "hayır", "hayir", "pasif", "inactive":
		return false, nil
	}
	return false, fmt.Errorf("is_active must be true or false")
}

func floatString(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', -1, 64)
}

// -------------------------------------------------------------- bulk engine

// Bulk activates / deactivates branches.
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
			ID: "activate", LabelKey: "bulk.actions.branches.activate",
			Permission: rbac.PermTenantBranchesBulkActivate, Reversible: true,
		},
		{
			ID: "deactivate", LabelKey: "bulk.actions.branches.deactivate",
			Permission: rbac.PermTenantBranchesBulkDeactivate, Reversible: true,
			ConfirmKey: "bulk.confirm.branches.deactivate",
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
		EntityUUID: entityUUID, EntityType: "branch", OK: true, Op: "update",
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

// Search indexes branches for the tenant Cmd+K palette.
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
		LabelKey:   "search.specs_branches",
		Permission: rbac.PermTenantBranchesRead,
		Icon:       "branches",
		Searchable: []string{"title", "subtitle", "keywords"},
		Scope:      searchengine.ScopeTenant,
		// Documents carry branch_ids; tenant searches apply the caller's
		// branch scope to this spec.
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
		rows, err := a.q.ListBranchSearchDocuments(ctx, orgID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			out = append(out, searchDocument(db.GetBranchSearchDocumentRow(row)))
		}
	}
	return out, nil
}

// Document loads one branch by its tenant document id ("<org>_<uuid>").
func (a *Search) Document(ctx context.Context, docID string) (searchengine.Document, error) {
	orgID, raw, err := searchengine.ParseTenantDocumentID(docID)
	if err != nil {
		return searchengine.Document{}, err
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("branches search: invalid uuid")
	}
	row, err := a.q.GetBranchSearchDocument(ctx, db.GetBranchSearchDocumentParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return searchengine.Document{}, err
	}
	return searchDocument(row), nil
}

func searchDocument(row db.GetBranchSearchDocumentRow) searchengine.Document {
	subtitle := strings.Trim(strings.Join([]string{row.District, row.City}, ", "), ", ")
	return searchengine.Document{
		ID:             searchengine.TenantDocumentID(row.OrganizationID, row.Uuid.String()),
		Spec:           usecase.SearchSpec,
		Title:          row.Code + " · " + row.Name,
		Subtitle:       subtitle,
		Keywords:       []string{row.Code, row.Name, row.City, row.District},
		Href:           "/t/" + row.OrganizationSlug + "/branches/" + row.Uuid.String(),
		Icon:           "branches",
		OrganizationID: row.OrganizationID,
		// Branch-restricted members only find the branches they may see.
		BranchIDs: []int64{row.ID},
	}
}

var (
	_ ioengine.ResourceAdapter = (*IO)(nil)
	_ ioengine.SampleProvider  = (*IO)(nil)
	_ bulkengine.BulkAdapter   = (*Bulk)(nil)
	_ searchengine.Adapter     = (*Search)(nil)
)
