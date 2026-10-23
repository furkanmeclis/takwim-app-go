package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// ResourcePlatformOrganizations is the engine / live list slug of the
// platform organizations list (resource:platform.organizations).
const ResourcePlatformOrganizations = "platform.organizations"

// OrganizationSortColumns are the sortable platform list fields.
var OrganizationSortColumns = apiquery.SortColumns{
	"name": "name", "slug": "slug", "status": "status", "member_count": "member_count",
	"branch_count": "branch_count", "access_ends_at": "access_ends_at",
	"created_at": "created_at", "updated_at": "updated_at",
}

// RegistrationLimiter limits public invitation registrations per IP.
type RegistrationLimiter interface {
	AllowRegister(ctx context.Context, ip string) (bool, time.Duration)
}

type Handler struct {
	svc      *orgusecase.Service
	mgmt     *orgusecase.Management
	activity *activity.Recorder
	events   resourceevents.Publisher
	storage  storage.Driver
	limiter  RegistrationLimiter
}

func New(svc *orgusecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// SetManagement attaches invitations, logo and activity (TEC-670/671).
func (h *Handler) SetManagement(m *orgusecase.Management) { h.mgmt = m }

// SetStorage attaches object storage for organization logos.
func (h *Handler) SetStorage(d storage.Driver) { h.storage = d }

// SetRegistrationLimiter limits POST /v1/invitations/{token}/register per IP.
func (h *Handler) SetRegistrationLimiter(l RegistrationLimiter) { h.limiter = l }

// SetResourceEvents makes member and role changes refresh the open member
// and role lists of the organization (tenant.members / tenant.roles on
// org:{org_uuid}:resource:*) and organization changes the platform list
// (resource:platform.organizations). nil disables it.
func (h *Handler) SetResourceEvents(pub resourceevents.Publisher) {
	h.events = pub
}

// PlatformOrganizationsMeta is the table contract of the platform list.
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, PlatformOrganizationsMeta())
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q, ok := parseListQuery(w, r, OrganizationSortColumns)
	if !ok {
		return
	}
	f, ok := organizationListFilter(w, r, q)
	if !ok {
		return
	}
	items, total, err := h.svc.ListOrganizations(r.Context(), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// organizationListFilter reads the list filters (also used by export / bulk
// "all matching" through OrganizationFilterFromQuery).
func organizationListFilter(w http.ResponseWriter, r *http.Request, q apiquery.Query) (orgmodel.ListFilter, bool) {
	f, field, err := orgmodel.OrganizationFilterFromValues(func(k string) string { return r.URL.Query().Get(k) })
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: field, Message: err.Error(), Code: "invalid"}})
		return orgmodel.ListFilter{}, false
	}
	f.Limit, f.Offset, f.Q = q.Limit, q.Offset, q.Q
	f.SortField, f.SortDesc = firstSortField(q, "created_at"), firstSortDesc(q, true)
	return f, true
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name          string    `json:"name"`
		Slug          string    `json:"slug"`
		OwnerUserUUID uuid.UUID `json:"owner_user_uuid"`
		OwnerEmail    string    `json:"owner_email"`
		Sector        string    `json:"sector"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	input := orgmodel.CreateOrganizationInput{Name: in.Name, Slug: in.Slug, Sector: in.Sector, OwnerUserUUID: in.OwnerUserUUID, OwnerUserID: p.UserInternal}
	var (
		org        orgmodel.Organization
		invitation *orgmodel.Invitation
		err        error
	)
	if strings.TrimSpace(in.OwnerEmail) != "" && h.mgmt != nil {
		if in.OwnerUserUUID != uuid.Nil {
			response.ValidationError(w, r, []response.Detail{{Field: "owner_email", Message: "send owner_user_uuid or owner_email, not both", Code: "invalid"}})
			return
		}
		var inv orgmodel.Invitation
		org, inv, err = h.mgmt.CreateOrganizationWithOwnerInvitation(r.Context(), input, in.OwnerEmail, h.actor(r))
		invitation = &inv
	} else {
		// Owner defaults to the caller; platform admins usually pass the
		// customer's account so they do not become a member themselves.
		org, err = h.svc.CreateOrganization(r.Context(), input)
	}
	if err != nil {
		writeTenantError(w, r, err)
		return
	}
	payload := map[string]any{"slug": org.Slug, "name": org.Name}
	if invitation != nil {
		payload["owner_invitation"] = invitation.Email
	}
	h.record(r, "organizations.created", &org.UUID, payload)
	h.publishPlatform(r.Context(), resourceevents.ActionCreated, org.UUID)
	summary, err := h.svc.GetOrganizationSummary(r.Context(), org.UUID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, map[string]any{"organization": summary, "owner_invitation": invitation})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid", "organization uuid is invalid")
	if !ok {
		return
	}
	org, err := h.svc.GetOrganizationSummary(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, org)
}

func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid", "organization uuid is invalid")
	if !ok {
		return
	}
	var in struct {
		Name     *string          `json:"name"`
		Slug     *string          `json:"slug"`
		Settings *json.RawMessage `json:"settings"`
		Timezone *string          `json:"timezone"`
		Currency *string          `json:"currency"`
		Sector   *string          `json:"sector"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	var settings []byte
	if in.Settings != nil {
		settings = []byte(*in.Settings)
	}
	org, err := h.svc.PatchOrganization(r.Context(), id, orgmodel.PatchOrganizationInput{
		Name: in.Name, Slug: in.Slug, Settings: settings, Timezone: in.Timezone, Currency: in.Currency, Sector: in.Sector,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "organizations.updated", &org.UUID, map[string]any{"slug": org.Slug, "name": org.Name})
	h.publishPlatform(r.Context(), resourceevents.ActionUpdated, org.UUID)
	response.JSON(w, r, http.StatusOK, org)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid", "organization uuid is invalid")
	if !ok {
		return
	}
	if err := h.svc.DeleteOrganization(r.Context(), id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "organizations.deleted", &id, map[string]any{})
	h.publishPlatform(r.Context(), resourceevents.ActionDeleted, id)
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) SetStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid", "organization uuid is invalid")
	if !ok {
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	org, err := h.svc.SetStatus(r.Context(), id, in.Status)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "organizations.status_updated", &org.UUID, map[string]any{"status": org.Status})
	h.publishPlatform(r.Context(), resourceevents.ActionUpdated, org.UUID)
	response.JSON(w, r, http.StatusOK, org)
}

func (h *Handler) SetAccess(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid", "organization uuid is invalid")
	if !ok {
		return
	}
	var in struct {
		AccessStartsAt *time.Time `json:"access_starts_at"`
		AccessEndsAt   *time.Time `json:"access_ends_at"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if in.AccessStartsAt != nil && in.AccessEndsAt != nil && !in.AccessEndsAt.After(*in.AccessStartsAt) {
		response.ValidationError(w, r, []response.Detail{{Field: "access_ends_at", Message: "access_ends_at must be after access_starts_at", Code: "invalid"}})
		return
	}
	org, err := h.svc.SetAccessWindow(r.Context(), id, in.AccessStartsAt, in.AccessEndsAt)
	if err != nil {
		writeError(w, r, err)
		return
	}
	payload := map[string]any{"access_starts_at": org.AccessStartsAt}
	if org.AccessEndsAt != nil {
		payload["access_ends_at"] = *org.AccessEndsAt
	}
	h.record(r, "organizations.access_updated", &org.UUID, payload)
	h.publishPlatform(r.Context(), resourceevents.ActionUpdated, org.UUID)
	response.JSON(w, r, http.StatusOK, org)
}

func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid", "organization uuid is invalid")
	if !ok {
		return
	}
	f, ok := tenantMemberFilter(w, r)
	if !ok {
		return
	}
	items, total, err := h.svc.PlatformListMembers(r.Context(), id, f)
	if err != nil {
		writeTenantError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

func (h *Handler) AddMember(w http.ResponseWriter, r *http.Request) {
	orgID, ok := parsePathUUID(w, r, "uuid", "organization uuid is invalid")
	if !ok {
		return
	}
	var in orgmodel.PlatformMemberInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if in.UserUUID == uuid.Nil {
		response.ValidationError(w, r, []response.Detail{{Field: "user_uuid", Message: "user_uuid is required", Code: "required"}})
		return
	}
	member, err := h.svc.PlatformAddMember(r.Context(), orgID, in)
	if err != nil {
		writeTenantError(w, r, err)
		return
	}
	h.record(r, "organizations.member_added", &orgID, map[string]any{"user_uuid": in.UserUUID.String(), "roles": roleKeys(member.Roles)})
	h.publishPlatformMember(r, orgID, resourceevents.ActionCreated, in.UserUUID)
	response.JSON(w, r, http.StatusCreated, member)
}

func (h *Handler) PatchMember(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := parseOrgMemberPath(w, r)
	if !ok {
		return
	}
	var in orgmodel.PlatformMemberInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	member, err := h.svc.PlatformPatchMember(r.Context(), orgID, userID, in)
	if err != nil {
		writeTenantError(w, r, err)
		return
	}
	h.record(r, "organizations.member_role_updated", &orgID, map[string]any{
		"user_uuid": userID.String(), "roles": roleKeys(member.Roles), "branch_scope": member.BranchScope,
	})
	h.publishPlatformMember(r, orgID, resourceevents.ActionUpdated, userID)
	response.JSON(w, r, http.StatusOK, member)
}

func (h *Handler) DeleteMember(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := parseOrgMemberPath(w, r)
	if !ok {
		return
	}
	if err := h.svc.RemoveMember(r.Context(), orgID, userID); err != nil {
		writeTenantError(w, r, err)
		return
	}
	h.record(r, "organizations.member_removed", &orgID, map[string]any{"user_uuid": userID.String()})
	h.publishPlatformMember(r, orgID, resourceevents.ActionDeleted, userID)
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "removed"})
}

// ListRoles serves GET /v1/platform/organizations/{uuid}/roles (template and
// organization roles, for member / invitation forms).
func (h *Handler) ListRoles(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid", "organization uuid is invalid")
	if !ok {
		return
	}
	scope, err := h.svc.PlatformScope(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, err := h.svc.ListTenantRoles(r.Context(), scope)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// ListBranches serves GET /v1/platform/organizations/{uuid}/branches.
func (h *Handler) ListBranches(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid", "organization uuid is invalid")
	if !ok {
		return
	}
	scope, err := h.svc.PlatformScope(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, err := h.mgmt.PlatformBranchOptions(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// ListActivity serves GET /v1/platform/organizations/{uuid}/activity.
func (h *Handler) ListActivity(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid", "organization uuid is invalid")
	if !ok {
		return
	}
	q, ok := parseListQuery(w, r, apiquery.SortColumns{})
	if !ok {
		return
	}
	items, total, err := h.mgmt.ListActivity(r.Context(), id, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// ListPlatformInvitations serves GET /v1/platform/organizations/{uuid}/invitations.
func (h *Handler) ListPlatformInvitations(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid", "organization uuid is invalid")
	if !ok {
		return
	}
	scope, err := h.svc.PlatformScope(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.listInvitations(w, r, scope.ID)
}

// CreatePlatformInvitation serves POST /v1/platform/organizations/{uuid}/invitations.
func (h *Handler) CreatePlatformInvitation(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid", "organization uuid is invalid")
	if !ok {
		return
	}
	var in orgmodel.CreateInvitationInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	inv, err := h.mgmt.CreatePlatformInvitation(r.Context(), id, h.actor(r), in)
	if err != nil {
		writeTenantError(w, r, err)
		return
	}
	h.record(r, "organizations.invitation_created", &id, map[string]any{"email": inv.Email, "roles": roleKeys(inv.Roles)})
	h.publishPlatformInvitation(r, id, resourceevents.ActionCreated, inv.UUID)
	response.JSON(w, r, http.StatusCreated, inv)
}

// RevokePlatformInvitation serves POST /v1/platform/organizations/{uuid}/invitations/{invitationUuid}/revoke.
func (h *Handler) RevokePlatformInvitation(w http.ResponseWriter, r *http.Request) {
	h.platformInvitationAction(w, r, "organizations.invitation_revoked", func(ctx context.Context, scope orgctx.Scope, id uuid.UUID) (orgmodel.Invitation, error) {
		return h.mgmt.RevokeInvitation(ctx, scope, h.actor(r), id)
	})
}

// ResendPlatformInvitation serves POST /v1/platform/organizations/{uuid}/invitations/{invitationUuid}/resend.
func (h *Handler) ResendPlatformInvitation(w http.ResponseWriter, r *http.Request) {
	h.platformInvitationAction(w, r, "organizations.invitation_resent", func(ctx context.Context, scope orgctx.Scope, id uuid.UUID) (orgmodel.Invitation, error) {
		return h.mgmt.ResendInvitation(ctx, scope, h.actor(r), id)
	})
}

func (h *Handler) platformInvitationAction(w http.ResponseWriter, r *http.Request, action string, fn func(context.Context, orgctx.Scope, uuid.UUID) (orgmodel.Invitation, error)) {
	orgUUID, ok := parsePathUUID(w, r, "uuid", "organization uuid is invalid")
	if !ok {
		return
	}
	invUUID, ok := parsePathUUID(w, r, "invitationUuid", "invitation uuid is invalid")
	if !ok {
		return
	}
	scope, err := h.svc.PlatformScope(r.Context(), orgUUID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	inv, err := fn(r.Context(), scope, invUUID)
	if err != nil {
		writeTenantError(w, r, err)
		return
	}
	h.record(r, action, &orgUUID, map[string]any{"email": inv.Email, "invitation_uuid": inv.UUID.String()})
	h.publishPlatformInvitation(r, orgUUID, resourceevents.ActionUpdated, inv.UUID)
	response.JSON(w, r, http.StatusOK, inv)
}

// StreamLogo serves GET /v1/organizations/{uuid}/logo to signed-in users
// (sidebar, organization switcher, platform list). The object key never
// leaves the API.
func (h *Handler) StreamLogo(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid", "organization uuid is invalid")
	if !ok {
		return
	}
	if h.mgmt == nil || h.storage == nil {
		response.NotFound(w, r, "logo not found")
		return
	}
	key, err := h.mgmt.LogoObjectKey(r.Context(), id)
	if err != nil {
		response.NotFound(w, r, "logo not found")
		return
	}
	rc, _, err := h.storage.Download(r.Context(), key)
	if err != nil {
		response.NotFound(w, r, "logo not found")
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", storage.MIMEFromLogoKey(key))
	// The URL carries a version derived from the object key.
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = copyBody(w, rc)
}

func (h *Handler) actor(r *http.Request) orgusecase.Actor {
	p := authctx.MustPrincipal(r.Context())
	name := ""
	if u, err := h.svc.UserDisplayName(r.Context(), p.UserInternal); err == nil {
		name = u
	}
	return orgusecase.Actor{UserID: p.UserInternal, Name: name}
}

func (h *Handler) record(r *http.Request, action string, resourceUUID *uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, ResourcePlatformOrganizations, resourceUUID, payload, r)
}

// publishPlatform refreshes the open platform organizations lists.
func (h *Handler) publishPlatform(ctx context.Context, action string, id uuid.UUID) {
	if h.events == nil {
		return
	}
	h.events.Publish(ctx, ResourcePlatformOrganizations, action, id.String())
}

func roleKeys(roles []orgmodel.TenantRoleRef) []string {
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		out = append(out, role.Key)
	}
	return out
}

func parseOrgMemberPath(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	orgID, ok := parsePathUUID(w, r, "uuid", "organization uuid is invalid")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	userID, ok := parsePathUUID(w, r, "userUuid", "user uuid is invalid")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	return orgID, userID, true
}

func parsePathUUID(w http.ResponseWriter, r *http.Request, name, msg string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, msg)
		return uuid.Nil, false
	}
	return id, true
}

func parseListQuery(w http.ResponseWriter, r *http.Request, allowed apiquery.SortColumns) (apiquery.Query, bool) {
	if banned := apiquery.ForbiddenParams(r.URL.Query()); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return apiquery.Query{}, false
	}
	q := apiquery.Parse(r.URL.Query())
	if err := apiquery.ValidateSort(q.Sort, allowed); err != nil {
		writeError(w, r, err)
		return apiquery.Query{}, false
	}
	return q, true
}

func firstSortField(q apiquery.Query, def string) string {
	if len(q.Sort) == 0 {
		return def
	}
	return q.Sort[0].Field
}

func firstSortDesc(q apiquery.Query, def bool) bool {
	if len(q.Sort) == 0 {
		return def
	}
	return q.Sort[0].Desc
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return err
	}
	return nil
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var ve *apiquery.ValidationError
	if errors.As(err, &ve) {
		details := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	switch {
	case errors.Is(err, orgusecase.ErrNotFound), errors.Is(err, orgusecase.ErrMemberNotFound), errors.Is(err, orgusecase.ErrUserNotFound),
		errors.Is(err, orgusecase.ErrRoleNotFound), errors.Is(err, orgusecase.ErrInvitationNotFound):
		response.NotFound(w, r, "Resource was not found")
	case errors.Is(err, orgusecase.ErrLastOrganizationOwner):
		response.Conflict(w, r, response.CodeLastOrganizationOwner, "Cannot remove or demote the last organization owner")
	case errors.Is(err, orgusecase.ErrInvitationExpired):
		response.Error(w, r, http.StatusGone, response.CodeInvitationExpired, err.Error())
	case errors.Is(err, orgusecase.ErrInvitationUsed):
		response.Error(w, r, http.StatusGone, response.CodeInvitationUsed, err.Error())
	case errors.Is(err, orgusecase.ErrInvitationRevoked):
		response.Error(w, r, http.StatusGone, response.CodeInvitationRevoked, err.Error())
	case errors.Is(err, orgusecase.ErrInvitationEmailMismatch):
		response.Error(w, r, http.StatusForbidden, response.CodeInvitationEmailMismatch, err.Error())
	case errors.Is(err, orgusecase.ErrAlreadyMember):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeAlreadyMember, err.Error(), []response.Detail{{
			Field: "email", Message: err.Error(), Code: "already_member",
		}})
	case errors.Is(err, orgusecase.ErrAccountExists):
		response.Conflict(w, r, response.CodeAccountExists, err.Error())
	case errors.Is(err, orgusecase.ErrRateLimited):
		response.TooManyRequests(w, r, err.Error())
	case errors.Is(err, orgusecase.ErrRegistrationUnavailable):
		response.Forbidden(w, r, err.Error())
	case errors.Is(err, orgusecase.ErrConflict):
		response.Conflict(w, r, response.CodeConflict, err.Error())
	case errors.Is(err, orgusecase.ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
