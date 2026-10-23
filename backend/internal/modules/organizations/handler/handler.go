package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

type Handler struct {
	svc      *orgusecase.Service
	activity *activity.Recorder
	events   resourceevents.Publisher
}

func New(svc *orgusecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// SetResourceEvents makes member and role changes refresh the open member
// and role lists of the organization (tenant.members / tenant.roles on
// org:{org_uuid}:resource:*). nil disables it.
func (h *Handler) SetResourceEvents(pub resourceevents.Publisher) {
	h.events = pub
}

func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, resourcemeta.PlatformOrganizations())
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q, ok := parseListQuery(w, r, apiquery.TenantsSort)
	if !ok {
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	items, total, err := h.svc.ListOrganizations(r.Context(), orgmodel.ListFilter{
		Limit: q.Limit, Offset: q.Offset, Q: q.Q, Status: status,
		SortField: firstSortField(q, "created_at"), SortDesc: firstSortDesc(q, true),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name          string    `json:"name"`
		Slug          string    `json:"slug"`
		OwnerUserUUID uuid.UUID `json:"owner_user_uuid"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	// Owner defaults to the caller; platform admins usually pass the
	// customer's account so they do not become a member themselves.
	p := authctx.MustPrincipal(r.Context())
	org, err := h.svc.CreateOrganization(r.Context(), orgmodel.CreateOrganizationInput{
		Name: in.Name, Slug: in.Slug, OwnerUserUUID: in.OwnerUserUUID, OwnerUserID: p.UserInternal,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "organizations.created", &org.UUID, map[string]any{"slug": org.Slug, "name": org.Name})
	response.JSON(w, r, http.StatusCreated, org)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid", "organization uuid is invalid")
	if !ok {
		return
	}
	org, err := h.svc.GetOrganization(r.Context(), id)
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
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	var settings []byte
	if in.Settings != nil {
		settings = []byte(*in.Settings)
	}
	org, err := h.svc.PatchOrganization(r.Context(), id, orgmodel.PatchOrganizationInput{
		Name: in.Name, Slug: in.Slug, Settings: settings,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "organizations.updated", &org.UUID, map[string]any{"slug": org.Slug})
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
	org, err := h.svc.SetAccessWindow(r.Context(), id, in.AccessStartsAt, in.AccessEndsAt)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "organizations.access_updated", &org.UUID, map[string]any{})
	response.JSON(w, r, http.StatusOK, org)
}

func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid", "organization uuid is invalid")
	if !ok {
		return
	}
	q, ok := parseListQuery(w, r, apiquery.TenantMembersSort)
	if !ok {
		return
	}
	items, total, err := h.svc.ListMembers(r.Context(), id, orgmodel.MemberListFilter{
		Limit: q.Limit, Offset: q.Offset, Q: q.Q,
		SortField: firstSortField(q, "created_at"), SortDesc: firstSortDesc(q, false),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) AddMember(w http.ResponseWriter, r *http.Request) {
	orgID, ok := parsePathUUID(w, r, "uuid", "organization uuid is invalid")
	if !ok {
		return
	}
	var in struct {
		UserUUID string `json:"user_uuid"`
		RoleKey  string `json:"role_key"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	userID, err := uuid.Parse(strings.TrimSpace(in.UserUUID))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "user_uuid is invalid")
		return
	}
	member, err := h.svc.AddMember(r.Context(), orgID, userID, in.RoleKey)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "organizations.member_added", &orgID, map[string]any{"user_uuid": userID.String(), "role": member.Role.Key})
	h.publishPlatformMember(r, orgID, resourceevents.ActionCreated, userID)
	response.JSON(w, r, http.StatusCreated, member)
}

func (h *Handler) PatchMember(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := parseOrgMemberPath(w, r)
	if !ok {
		return
	}
	var in struct {
		RoleKey string `json:"role_key"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	member, err := h.svc.ChangeMemberRole(r.Context(), orgID, userID, in.RoleKey)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "organizations.member_role_updated", &orgID, map[string]any{"user_uuid": userID.String(), "role": member.Role.Key})
	h.publishPlatformMember(r, orgID, resourceevents.ActionUpdated, userID)
	response.JSON(w, r, http.StatusOK, member)
}

func (h *Handler) DeleteMember(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := parseOrgMemberPath(w, r)
	if !ok {
		return
	}
	if err := h.svc.RemoveMember(r.Context(), orgID, userID); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "organizations.member_removed", &orgID, map[string]any{"user_uuid": userID.String()})
	h.publishPlatformMember(r, orgID, resourceevents.ActionDeleted, userID)
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "removed"})
}

func (h *Handler) record(r *http.Request, action string, resourceUUID *uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, "platform.organizations", resourceUUID, payload, r)
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
	case errors.Is(err, orgusecase.ErrNotFound), errors.Is(err, orgusecase.ErrMemberNotFound), errors.Is(err, orgusecase.ErrUserNotFound), errors.Is(err, orgusecase.ErrRoleNotFound):
		response.NotFound(w, r, "Resource was not found")
	case errors.Is(err, orgusecase.ErrLastOrganizationOwner):
		response.Conflict(w, r, response.CodeLastOrganizationOwner, "Cannot remove or demote the last organization owner")
	case errors.Is(err, orgusecase.ErrConflict):
		response.Conflict(w, r, response.CodeConflict, err.Error())
	case errors.Is(err, orgusecase.ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
