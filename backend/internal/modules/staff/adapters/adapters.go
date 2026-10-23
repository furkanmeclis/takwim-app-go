// Package adapters connects the staff resource to the shared engines:
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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/google/uuid"
)

// Resource is the engine slug of staff.
const Resource = model.Resource

// MaxBulkTargets caps the "all matching" bulk scope.
const MaxBulkTargets = 10000

var errNoOrganization = errors.New("staff: organization scope is required")

func organizationID(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return 0, errNoOrganization
	}
	return id, nil
}

// FilterFromQuery maps list query params (q, branch, is_active,
// show_online, has_account) to a filter. branch lists branch uuids.
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
	}{{"is_active", &f.IsActive}, {"show_online", &f.ShowOnline}, {"has_account", &f.HasAccount}} {
		if v, err := strconv.ParseBool(strings.TrimSpace(query[b.key])); err == nil {
			*b.dst = &v
		}
	}
	return f
}

// ---------------------------------------------------------------- io engine

// IO exports and imports staff.
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
		{Key: "name", LabelKey: "staff.name", Type: ioengine.ColumnTypeString},
		{Key: "title", LabelKey: "staff.title", Type: ioengine.ColumnTypeString},
		{Key: "phone", LabelKey: "staff.phone", Type: ioengine.ColumnTypeString},
		{Key: "email", LabelKey: "staff.email", Type: ioengine.ColumnTypeString},
		{Key: "branches", LabelKey: "staff.branches", Type: ioengine.ColumnTypeString},
		{Key: "account_email", LabelKey: "staff.account_email", Type: ioengine.ColumnTypeString},
		{Key: "color", LabelKey: "staff.color", Type: ioengine.ColumnTypeString},
		{Key: "show_online", LabelKey: "staff.show_online", Type: ioengine.ColumnTypeBoolean},
		{Key: "is_active", LabelKey: "staff.is_active", Type: ioengine.ColumnTypeBoolean},
		{Key: "uuid", LabelKey: "staff.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "created_at", LabelKey: "staff.created_at", Type: ioengine.ColumnTypeDatetime},
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
	for _, st := range rows {
		names := make([]string, 0, len(st.Branches))
		for _, b := range st.Branches {
			names = append(names, b.Name)
		}
		account := ""
		if st.Account != nil {
			account = st.Account.Email
		}
		out = append(out, map[string]any{
			"uuid": st.UUID.String(), "name": st.Name, "title": st.Title, "phone": st.Phone, "email": st.Email,
			"branches": strings.Join(names, ", "), "account_email": account, "color": st.Color,
			"show_online": st.ShowOnline, "is_active": st.IsActive, "created_at": st.CreatedAt,
		})
	}
	return ioengine.Dataset{Resource: Resource, Columns: a.ExportColumns(), Rows: out}, nil
}

// ImportSchema: a row with the uuid of an existing person updates it,
// otherwise a new person is created. branch_codes lists branch codes.
func (a *IO) ImportSchema() []ioengine.ImportField {
	return []ioengine.ImportField{
		{Key: "name", LabelKey: "staff.name", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "title", LabelKey: "staff.title", Type: ioengine.ColumnTypeString},
		{Key: "phone", LabelKey: "staff.phone", Type: ioengine.ColumnTypeString},
		{Key: "email", LabelKey: "staff.email", Type: ioengine.ColumnTypeString},
		{Key: "branch_codes", LabelKey: "staff.branch_codes", Type: ioengine.ColumnTypeString},
		{Key: "color", LabelKey: "staff.color", Type: ioengine.ColumnTypeString},
		{Key: "show_online", LabelKey: "staff.show_online", Type: ioengine.ColumnTypeBoolean, DefaultHint: "true"},
		{Key: "is_active", LabelKey: "staff.is_active", Type: ioengine.ColumnTypeBoolean, DefaultHint: "true"},
		{Key: "uuid", LabelKey: "staff.uuid", Type: ioengine.ColumnTypeUUID},
	}
}

// SampleRows implements ioengine.SampleProvider.
func (a *IO) SampleRows() []map[string]any {
	return []map[string]any{{
		"name": "Ayşe Yılmaz", "title": "Kuaför", "phone": "+90 532 000 00 00", "email": "ayse@example.com",
		"branch_codes": "MRK", "color": "#2563EB", "show_online": "true", "is_active": "true",
	}}
}

// ApplyRow creates a staff member, or updates the one with the row's uuid.
func (a *IO) ApplyRow(ctx context.Context, row map[string]any, defaults map[string]any) (ioengine.RowResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.RowResult{}, err
	}
	in, id, codes, err := inputFromRow(row, defaults)
	if err != nil {
		return ioengine.RowResult{OK: false, Error: err.Error()}, nil
	}
	if codes != nil {
		branches, err := a.svc.BranchUUIDsByCodes(ctx, orgID, codes)
		if err != nil {
			if usecase.IsValidation(err) {
				return ioengine.RowResult{OK: false, Error: err.Error()}, nil
			}
			return ioengine.RowResult{}, err
		}
		in.BranchUUIDs = &branches
	}
	got, created, prev, err := a.svc.ImportRow(ctx, orgID, id, in)
	if err != nil {
		if usecase.IsValidation(err) || errors.Is(err, usecase.ErrNotFound) || errors.Is(err, usecase.ErrAccountTaken) {
			return ioengine.RowResult{OK: false, Error: err.Error()}, nil
		}
		if errors.Is(err, entitlement.ErrLimitReached) {
			return ioengine.RowResult{OK: false, Error: err.Error()}, nil
		}
		return ioengine.RowResult{}, err
	}
	res := ioengine.RowResult{OK: true, EntityType: "staff", EntityUUID: got.String(), Op: "create"}
	if !created && prev != nil {
		res.Op = "update"
		res.Previous = snapshot(*prev)
	}
	return res, nil
}

// RevertRow deletes an imported staff member or restores an updated one.
func (a *IO) RevertRow(ctx context.Context, entityType, entityUUID string, previous map[string]any) error {
	if entityType != "staff" {
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
	var prev model.Staff
	if err := json.Unmarshal(raw, &prev); err != nil {
		return err
	}
	branches := make([]uuid.UUID, 0, len(prev.Branches))
	for _, b := range prev.Branches {
		branches = append(branches, b.UUID)
	}
	_, err = a.svc.Patch(ctx, orgID, id, model.StaffInput{
		Name: &prev.Name, Title: &prev.Title, Phone: &prev.Phone, Email: &prev.Email, Color: &prev.Color,
		ShowOnline: &prev.ShowOnline, IsActive: &prev.IsActive, BranchUUIDs: &branches,
	})
	return err
}

func inputFromRow(row, defaults map[string]any) (model.StaffInput, *uuid.UUID, []string, error) {
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
	var in model.StaffInput
	name := cell(row, "name")
	in.Name = &name
	in.Title, in.Phone, in.Email, in.Color = str("title"), str("phone"), str("email"), str("color")
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
	var id *uuid.UUID
	if v := cell(row, "uuid"); v != "" {
		parsed, err := uuid.Parse(v)
		if err != nil {
			return in, nil, nil, fmt.Errorf("uuid is invalid")
		}
		id = &parsed
	}
	var codes []string
	if v, ok := get("branch_codes"); ok {
		codes = apiquery.SplitCSV(v)
	}
	return in, id, codes, nil
}

func snapshot(st model.Staff) map[string]any {
	st.Assignments = nil
	raw, err := json.Marshal(st)
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

// Bulk activates / deactivates staff.
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
			ID: "activate", LabelKey: "bulk.actions.staff.activate",
			Permission: rbac.PermTenantStaffBulkActivate, Reversible: true,
		},
		{
			ID: "deactivate", LabelKey: "bulk.actions.staff.deactivate",
			Permission: rbac.PermTenantStaffBulkDeactivate, Reversible: true,
			ConfirmKey: "bulk.confirm.staff.deactivate",
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
		EntityUUID: entityUUID, EntityType: "staff", OK: true, Op: "update",
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

// Search indexes staff for the tenant Cmd+K palette.
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
		LabelKey:   "search.specs_staff",
		Permission: rbac.PermTenantStaffRead,
		Icon:       "staff",
		Searchable: []string{"title", "subtitle", "keywords"},
		Scope:      searchengine.ScopeTenant,
		// Documents carry the person's branches; branch-limited members
		// find the staff of their branches only.
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
		rows, err := a.q.ListStaffSearchDocuments(ctx, orgID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			out = append(out, searchDocument(db.GetStaffSearchDocumentRow(row)))
		}
	}
	return out, nil
}

// Document loads one staff member by its tenant document id.
func (a *Search) Document(ctx context.Context, docID string) (searchengine.Document, error) {
	orgID, raw, err := searchengine.ParseTenantDocumentID(docID)
	if err != nil {
		return searchengine.Document{}, err
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("staff search: invalid uuid")
	}
	row, err := a.q.GetStaffSearchDocument(ctx, db.GetStaffSearchDocumentParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return searchengine.Document{}, err
	}
	return searchDocument(row), nil
}

func searchDocument(row db.GetStaffSearchDocumentRow) searchengine.Document {
	return searchengine.Document{
		ID:             searchengine.TenantDocumentID(row.OrganizationID, row.Uuid.String()),
		Spec:           usecase.SearchSpec,
		Title:          row.Name,
		Subtitle:       row.Title,
		Keywords:       []string{row.Name, row.Title, row.Email, row.Phone},
		Href:           "/t/" + row.OrganizationSlug + "/staff/" + row.Uuid.String(),
		Icon:           "staff",
		OrganizationID: row.OrganizationID,
		BranchIDs:      row.BranchIds,
	}
}

var (
	_ ioengine.ResourceAdapter = (*IO)(nil)
	_ ioengine.SampleProvider  = (*IO)(nil)
	_ bulkengine.BulkAdapter   = (*Bulk)(nil)
	_ searchengine.Adapter     = (*Search)(nil)
)
