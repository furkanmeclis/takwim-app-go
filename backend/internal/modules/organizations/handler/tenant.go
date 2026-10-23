package handler

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Tenant member and role management: /v1/tenant/members and /v1/tenant/roles.
// The organization always comes from orgctx.Scope.

const (
	resourceTenantMembers = "tenant.members"
	resourceTenantRoles   = "tenant.roles"
)

// Engine slugs of the tenant member and role lists (live list channels).
const (
	ResourceTenantMembers = resourceTenantMembers
	ResourceTenantRoles   = resourceTenantRoles
)

// TenantMembersMeta is the table contract of GET /v1/tenant/members.
func TenantMembersMeta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      resourceTenantMembers,
		DefaultSort:   "created_at",
		DefaultFields: []string{"name", "email", "roles", "branch_scope", "last_login_at"},
		Capabilities: resourcemeta.Capabilities{
			Read: true, Update: true, Delete: true, Search: true, Filter: true, Sort: true,
		},
		SearchableFields: []string{"name", "surname", "email"},
		SortableFields:   []string{"name", "email", "branch_scope", "last_login_at", "created_at"},
		FilterableFields: []string{"role", "branch_scope"},
		Columns: []resourcemeta.Column{
			{Key: "name", LabelKey: "tenant_members.columns.name", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "email", LabelKey: "tenant_members.columns.email", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "roles", LabelKey: "tenant_members.columns.roles", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "branch_scope", LabelKey: "tenant_members.columns.branch_scope", Type: resourcemeta.ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "last_login_at", LabelKey: "tenant_members.columns.last_login_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: true},
			{Key: "created_at", LabelKey: "tenant_members.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "role", LabelKey: "tenant_members.columns.roles", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "tenant.roles"},
			{Key: "branch_scope", LabelKey: "tenant_members.columns.branch_scope", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "tenant_branch_scope"},
		},
		Includes:    []string{},
		BulkActions: []bulkengine.BulkActionDef{},
	}
}

// TenantRolesMeta is the table contract of GET /v1/tenant/roles.
func TenantRolesMeta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      resourceTenantRoles,
		DefaultSort:   "name",
		DefaultFields: []string{"name", "key", "is_template", "permissions", "member_count"},
		Capabilities: resourcemeta.Capabilities{
			Create: true, Read: true, Update: true, Delete: true, Search: true, Filter: true,
		},
		SearchableFields: []string{"name", "key"},
		SortableFields:   []string{},
		FilterableFields: []string{"is_template"},
		Columns: []resourcemeta.Column{
			{Key: "name", LabelKey: "tenant_roles.columns.name", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "key", LabelKey: "tenant_roles.columns.key", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "is_template", LabelKey: "tenant_roles.columns.is_template", Type: resourcemeta.ColumnTypeBoolean, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "permissions", LabelKey: "tenant_roles.columns.permissions", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "member_count", LabelKey: "tenant_roles.columns.member_count", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
		},
		Filters: []resourcemeta.Filter{
			{Key: "is_template", LabelKey: "tenant_roles.columns.is_template", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "boolean"},
		},
		Includes:    []string{},
		BulkActions: []bulkengine.BulkActionDef{},
	}
}

// TenantMembersMetaH serves GET /v1/tenant/members/meta.
func (h *Handler) TenantMembersMetaH(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, TenantMembersMeta())
}

// TenantRolesMetaH serves GET /v1/tenant/roles/meta.
func (h *Handler) TenantRolesMetaH(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, TenantRolesMeta())
}

// TenantListMembers serves GET /v1/tenant/members.
func (h *Handler) TenantListMembers(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	f, ok := tenantMemberFilter(w, r)
	if !ok {
		return
	}
	items, total, err := h.svc.ListTenantMembers(r.Context(), scope, f)
	if err != nil {
		writeTenantError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

// tenantMemberFilter parses the member list query (tenant and platform).
func tenantMemberFilter(w http.ResponseWriter, r *http.Request) (orgmodel.TenantMemberListFilter, bool) {
	q, ok := parseListQuery(w, r, apiquery.SortColumns(orgusecase.MemberSortColumns))
	if !ok {
		return orgmodel.TenantMemberListFilter{}, false
	}
	values := r.URL.Query()
	f := orgmodel.TenantMemberListFilter{
		Limit: q.Limit, Offset: q.Offset, Q: q.Q,
		SortField: firstSortField(q, "created_at"), SortDesc: firstSortDesc(q, false),
	}
	if v := strings.TrimSpace(values.Get("branch_scope")); v != "" {
		// Both values selected in a faceted filter = no filter.
		if v == orgctx.BranchScopeAll || v == orgctx.BranchScopeSelected {
			f.BranchScope = v
		} else if v != "all,selected" && v != "selected,all" {
			response.ValidationError(w, r, []response.Detail{{Field: "branch_scope", Message: "branch_scope must be all or selected", Code: "invalid"}})
			return orgmodel.TenantMemberListFilter{}, false
		}
	}
	for _, raw := range values["role"] {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			id, err := uuid.Parse(part)
			if err != nil {
				response.ValidationError(w, r, []response.Detail{{Field: "role", Message: "role must be a role uuid", Code: "invalid"}})
				return orgmodel.TenantMemberListFilter{}, false
			}
			f.RoleUUIDs = append(f.RoleUUIDs, id)
		}
	}
	return f, true
}

// TenantGetMember serves GET /v1/tenant/members/{userUuid}.
func (h *Handler) TenantGetMember(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := parsePathUUID(w, r, "userUuid", "user uuid is invalid")
	if !ok {
		return
	}
	m, err := h.svc.GetTenantMember(r.Context(), scope, id)
	if err != nil {
		writeTenantError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, m)
}

// TenantPatchMember serves PATCH /v1/tenant/members/{userUuid}.
func (h *Handler) TenantPatchMember(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := parsePathUUID(w, r, "userUuid", "user uuid is invalid")
	if !ok {
		return
	}
	var in orgmodel.PatchTenantMemberInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	before, beforeErr := h.svc.GetTenantMember(r.Context(), scope, id)
	m, err := h.svc.PatchTenantMember(r.Context(), scope, id, in)
	if err != nil {
		writeTenantError(w, r, err)
		return
	}
	after := memberAccess(m)
	var changes map[string]activity.Change
	if beforeErr == nil {
		changes = activity.Diff(memberAccess(before), after)
	}
	h.recordTenantChanges(r, "members.updated", resourceTenantMembers, &m.User.UUID, map[string]any{
		"roles": after["roles"], "branch_scope": m.BranchScope, "branches": len(m.Branches),
	}, changes)
	response.JSON(w, r, http.StatusOK, m)
}

// TenantDeleteMember serves DELETE /v1/tenant/members/{userUuid}.
func (h *Handler) TenantDeleteMember(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := parsePathUUID(w, r, "userUuid", "user uuid is invalid")
	if !ok {
		return
	}
	if err := h.svc.RemoveTenantMember(r.Context(), scope, id); err != nil {
		writeTenantError(w, r, err)
		return
	}
	h.recordTenant(r, "members.removed", resourceTenantMembers, &id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// TenantListRoles serves GET /v1/tenant/roles (template + organization roles).
func (h *Handler) TenantListRoles(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	q, ok := parseListQuery(w, r, apiquery.SortColumns{})
	if !ok {
		return
	}
	items, err := h.svc.ListTenantRoles(r.Context(), scope)
	if err != nil {
		writeTenantError(w, r, err)
		return
	}
	needle := strings.ToLower(strings.TrimSpace(q.Q))
	template := strings.TrimSpace(r.URL.Query().Get("is_template"))
	filtered := make([]orgmodel.TenantRole, 0, len(items))
	for _, role := range items {
		if needle != "" && !strings.Contains(strings.ToLower(role.Name), needle) && !strings.Contains(role.Key, needle) {
			continue
		}
		if (template == "true" && !role.IsTemplate) || (template == "false" && role.IsTemplate) {
			continue
		}
		filtered = append(filtered, role)
	}
	total := int64(len(filtered))
	start := min(int(q.Offset), len(filtered))
	end := min(start+int(q.Limit), len(filtered))
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(filtered[start:end], total, q.Limit, q.Offset))
}

// TenantListPermissions serves GET /v1/tenant/roles/permissions.
func (h *Handler) TenantListPermissions(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListTenantPermissions(r.Context())
	if err != nil {
		writeTenantError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// TenantGetRole serves GET /v1/tenant/roles/{uuid}.
func (h *Handler) TenantGetRole(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := parsePathUUID(w, r, "uuid", "role uuid is invalid")
	if !ok {
		return
	}
	role, err := h.svc.GetTenantRole(r.Context(), scope, id)
	if err != nil {
		writeTenantError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, role)
}

// TenantCreateRole serves POST /v1/tenant/roles.
func (h *Handler) TenantCreateRole(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in orgmodel.TenantRoleInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	role, err := h.svc.CreateTenantRole(r.Context(), scope, in)
	if err != nil {
		writeTenantError(w, r, err)
		return
	}
	h.recordTenant(r, "roles.created", resourceTenantRoles, &role.UUID, map[string]any{
		"key": role.Key, "permissions": role.Permissions,
	})
	response.JSON(w, r, http.StatusCreated, role)
}

// TenantPatchRole serves PATCH /v1/tenant/roles/{uuid}.
func (h *Handler) TenantPatchRole(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := parsePathUUID(w, r, "uuid", "role uuid is invalid")
	if !ok {
		return
	}
	var in orgmodel.TenantRoleInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	before, beforeErr := h.svc.GetTenantRole(r.Context(), scope, id)
	role, err := h.svc.PatchTenantRole(r.Context(), scope, id, in)
	if err != nil {
		writeTenantError(w, r, err)
		return
	}
	var changes map[string]activity.Change
	if beforeErr == nil {
		changes = activity.Diff(before, role, "updated_at", "created_at", "member_count")
	}
	h.recordTenantChanges(r, "roles.updated", resourceTenantRoles, &role.UUID, map[string]any{
		"key": role.Key, "permissions": role.Permissions,
	}, changes)
	response.JSON(w, r, http.StatusOK, role)
}

// TenantDeleteRole serves DELETE /v1/tenant/roles/{uuid}.
func (h *Handler) TenantDeleteRole(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := parsePathUUID(w, r, "uuid", "role uuid is invalid")
	if !ok {
		return
	}
	if err := h.svc.DeleteTenantRole(r.Context(), scope, id); err != nil {
		writeTenantError(w, r, err)
		return
	}
	h.recordTenant(r, "roles.deleted", resourceTenantRoles, &id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) recordTenant(r *http.Request, action, resource string, id *uuid.UUID, payload map[string]any) {
	h.recordTenantChanges(r, action, resource, id, payload, nil)
}

// recordTenantChanges is recordTenant with the before / after of the
// changed fields. Member and role events belong to the whole business.
func (h *Handler) recordTenantChanges(r *http.Request, action, resource string, id *uuid.UUID, payload map[string]any, changes map[string]activity.Change) {
	h.publishTenant(r.Context(), action, id)
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.RecordEntry(r.Context(), activity.Entry{
		ActorID: &p.UserInternal, Action: action, Resource: resource, ResourceUUID: id,
		Payload: payload, Changes: changes, Request: r,
	})
}

// memberAccess is what a member edit changes: role keys, branch scope and
// branch names (sorted, so the before / after compare by content).
func memberAccess(m orgmodel.TenantMember) map[string]any {
	roles := make([]string, 0, len(m.Roles))
	for _, role := range m.Roles {
		roles = append(roles, role.Key)
	}
	branches := make([]string, 0, len(m.Branches))
	for _, b := range m.Branches {
		branches = append(branches, b.Name)
	}
	slices.Sort(roles)
	slices.Sort(branches)
	return map[string]any{"roles": roles, "branch_scope": m.BranchScope, "branches": branches}
}

// publishTenant tells the open member / role lists of the organization on
// ctx what changed: one event per mutation on its own list (uuid = the
// member's user uuid or the role uuid). Member role changes also move the
// member counts of the roles list, and a role update renames it in the
// members list, so the other list gets a bulk event without uuids (refetch).
func (h *Handler) publishTenant(ctx context.Context, action string, id *uuid.UUID) {
	if h.events == nil || id == nil {
		return
	}
	switch action {
	case "members.added":
		h.events.Publish(ctx, resourceTenantMembers, resourceevents.ActionCreated, id.String())
		h.events.Publish(ctx, resourceTenantRoles, resourceevents.ActionBulk)
	case "members.updated":
		h.events.Publish(ctx, resourceTenantMembers, resourceevents.ActionUpdated, id.String())
		h.events.Publish(ctx, resourceTenantRoles, resourceevents.ActionBulk)
	case "members.removed":
		h.events.Publish(ctx, resourceTenantMembers, resourceevents.ActionDeleted, id.String())
		h.events.Publish(ctx, resourceTenantRoles, resourceevents.ActionBulk)
	case "roles.created":
		h.events.Publish(ctx, resourceTenantRoles, resourceevents.ActionCreated, id.String())
	case "roles.updated":
		h.events.Publish(ctx, resourceTenantRoles, resourceevents.ActionUpdated, id.String())
		h.events.Publish(ctx, resourceTenantMembers, resourceevents.ActionBulk)
	case "roles.deleted":
		h.events.Publish(ctx, resourceTenantRoles, resourceevents.ActionDeleted, id.String())
	}
}

// publishPlatformMember refreshes the tenant member list of an organization
// after a platform operator changed its members (/v1/platform/organizations).
// The tenant channel needs the organization on the context.
func (h *Handler) publishPlatformMember(r *http.Request, orgUUID uuid.UUID, action string, userUUID uuid.UUID) {
	if h.events == nil {
		return
	}
	org, err := h.svc.GetOrganization(r.Context(), orgUUID)
	if err != nil {
		return
	}
	ctx := orgctx.WithScope(r.Context(), orgctx.Scope{ID: org.ID, UUID: org.UUID, Slug: org.Slug})
	var tenantAction string
	switch action {
	case resourceevents.ActionCreated:
		tenantAction = "members.added"
	case resourceevents.ActionDeleted:
		tenantAction = "members.removed"
	default:
		tenantAction = "members.updated"
	}
	h.publishTenant(ctx, tenantAction, &userUUID)
	// Member counts and the owner column of the platform list.
	h.publishPlatform(r.Context(), resourceevents.ActionUpdated, orgUUID)
}

func writeTenantError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *orgusecase.ValidationError
	if errors.As(err, &ve) {
		response.RecordFailure(w, err)
		details := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	switch {
	case errors.Is(err, orgusecase.ErrForbidden), errors.Is(err, orgusecase.ErrRoleReadOnly):
		response.RecordFailure(w, err)
		response.Forbidden(w, r, err.Error())
	case errors.Is(err, orgusecase.ErrRateLimited):
		response.RecordFailure(w, err)
		response.Error(w, r, http.StatusTooManyRequests, response.CodeRateLimited, err.Error())
	case errors.Is(err, orgusecase.ErrRoleInUse):
		response.RecordFailure(w, err)
		response.Conflict(w, r, response.CodeConflict, err.Error())
	case errors.Is(err, orgusecase.ErrRoleKeyTaken):
		response.RecordFailure(w, err)
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "key", Message: err.Error(), Code: "taken",
		}})
	default:
		writeError(w, r, err)
	}
}
