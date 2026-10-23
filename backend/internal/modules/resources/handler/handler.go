// Package handler is the HTTP layer of the resources tenant resource. Every
// handler runs behind RequireOrganization and reads the organization id from
// orgctx.Scope; the client never sends it.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves /v1/tenant/resources.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
	events   resourceevents.Publisher
}

// New builds the handler. rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// SetResourceEvents publishes live list changes (optional).
func (h *Handler) SetResourceEvents(pub resourceevents.Publisher) {
	h.events = pub
}

// Meta serves GET /v1/tenant/resources/meta.
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, Meta())
}

// Facets serves GET /v1/tenant/resources/facets.
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	out, err := h.svc.Facets(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ParseListFilter reads the list filters shared by the list, export and
// bulk "all matching" scope.
func ParseListFilter(values map[string][]string) (model.ListFilter, []response.Detail) {
	var f model.ListFilter
	var details []response.Detail
	for _, raw := range values["branch"] {
		for _, part := range apiquery.SplitCSV(raw) {
			id, err := uuid.Parse(part)
			if err != nil {
				details = append(details, response.Detail{Field: "branch", Message: "branch must list branch uuids", Code: "invalid"})
				break
			}
			f.Branches = append(f.Branches, id)
		}
	}
	for _, raw := range values["kind"] {
		f.Kinds = append(f.Kinds, apiquery.SplitCSV(raw)...)
	}
	for _, raw := range values["zone"] {
		f.Zones = append(f.Zones, apiquery.SplitCSV(raw)...)
	}
	for _, b := range []struct {
		key string
		dst **bool
	}{{"is_active", &f.IsActive}, {"show_online", &f.ShowOnline}} {
		if list := values[b.key]; len(list) > 0 && strings.TrimSpace(list[0]) != "" {
			v, err := strconv.ParseBool(strings.TrimSpace(list[0]))
			if err != nil {
				details = append(details, response.Detail{Field: b.key, Message: b.key + " must be true or false", Code: "invalid"})
				continue
			}
			*b.dst = &v
		}
	}
	return f, details
}

// List serves GET /v1/tenant/resources.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return
	}
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns(usecase.SortColumns)); err != nil {
		writeError(w, r, err)
		return
	}
	f, details := ParseListFilter(values)
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	f.Limit, f.Offset, f.Q = q.Limit, q.Offset, q.Q
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.List(r.Context(), scope.ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Create serves POST /v1/tenant/resources.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in model.Input
	if !decodeJSON(w, r, &in) {
		return
	}
	res, err := h.svc.Create(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "resources.created", res, nil)
	response.JSON(w, r, http.StatusCreated, res)
}

// Get serves GET /v1/tenant/resources/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	res, err := h.svc.Get(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}

// Patch serves PATCH /v1/tenant/resources/{uuid}.
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.Input
	if !decodeJSON(w, r, &in) {
		return
	}
	before, beforeErr := h.svc.Get(r.Context(), scope.ID, id)
	res, err := h.svc.Patch(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	var changes map[string]activity.Change
	if beforeErr == nil {
		changes = activity.Diff(diffView(before), diffView(res))
	}
	h.record(r, "resources.updated", res, changes)
	response.JSON(w, r, http.StatusOK, res)
}

// Delete serves DELETE /v1/tenant/resources/{uuid} (soft delete).
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	res, err := h.svc.Delete(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "resources.deleted", res, nil)
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// Activity serves GET /v1/tenant/resources/{uuid}/activity.
func (h *Handler) Activity(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.Activity(r.Context(), scope.ID, id, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// ListClosures serves GET /v1/tenant/resources/{uuid}/closures.
func (h *Handler) ListClosures(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	values := r.URL.Query()
	q := apiquery.Parse(values)
	f := model.ClosureFilter{Limit: q.Limit, Offset: q.Offset}
	var details []response.Detail
	for _, p := range []struct {
		key string
		dst **time.Time
	}{{"from", &f.From}, {"to", &f.To}} {
		if raw := strings.TrimSpace(values.Get(p.key)); raw != "" {
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				details = append(details, response.Detail{Field: p.key, Message: "must be an RFC 3339 time", Code: "invalid"})
				continue
			}
			*p.dst = &t
		}
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	items, total, err := h.svc.ListClosures(r.Context(), scope.ID, id, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateClosure serves POST /v1/tenant/resources/{uuid}/closures.
func (h *Handler) CreateClosure(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ClosureInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	res, c, err := h.svc.CreateClosure(r.Context(), scope.ID, id, &p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordPayload(r, "resources.closure_created", res, closurePayload(res, c), nil)
	response.JSON(w, r, http.StatusCreated, c)
}

// PatchClosure serves PATCH /v1/tenant/resources/{uuid}/closures/{closureUuid}.
func (h *Handler) PatchClosure(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	closureID, ok := pathUUID(w, r, "closureUuid")
	if !ok {
		return
	}
	var in model.ClosureInput
	if !decodeJSON(w, r, &in) {
		return
	}
	res, before, c, err := h.svc.PatchClosure(r.Context(), scope.ID, id, closureID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordPayload(r, "resources.closure_updated", res, closurePayload(res, c),
		activity.Diff(before, c, "uuid", "created_at"))
	response.JSON(w, r, http.StatusOK, c)
}

// DeleteClosure serves DELETE /v1/tenant/resources/{uuid}/closures/{closureUuid}.
func (h *Handler) DeleteClosure(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	closureID, ok := pathUUID(w, r, "closureUuid")
	if !ok {
		return
	}
	res, c, err := h.svc.DeleteClosure(r.Context(), scope.ID, id, closureID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordPayload(r, "resources.closure_deleted", res, closurePayload(res, c), nil)
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

func closurePayload(res model.Resource, c model.Closure) map[string]any {
	return map[string]any{
		"name": res.Name, "closure_uuid": c.UUID.String(), "starts_at": c.StartsAt, "ends_at": c.EndsAt,
	}
}

// diffView is the part of a resource an edit changes; the branch is reduced
// to its name and the derived fields are left out.
func diffView(res model.Resource) map[string]any {
	return map[string]any{
		"branch": res.Branch.Name, "kind": res.Kind, "name": res.Name, "zone": res.Zone,
		"description": res.Description, "min_capacity": res.MinCapacity, "capacity": res.Capacity,
		"combinable": res.Combinable, "show_online": res.ShowOnline, "is_active": res.IsActive,
		"sort_order": res.SortOrder, "working_hours": res.WorkingHours,
	}
}

// record writes the activity event of a resource (bound to its branch) and
// publishes the list change.
func (h *Handler) record(r *http.Request, action string, res model.Resource, changes map[string]activity.Change) {
	h.recordPayload(r, action, res, map[string]any{"name": res.Name, "kind": res.Kind}, changes)
}

func (h *Handler) recordPayload(r *http.Request, action string, res model.Resource, payload map[string]any, changes map[string]activity.Change) {
	h.publish(r, action, res.UUID)
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	id, branch := res.UUID, res.Branch.UUID
	h.activity.RecordEntry(r.Context(), activity.Entry{
		ActorID: &p.UserInternal, Action: action, Resource: model.Slug, ResourceUUID: &id,
		BranchUUID: &branch, Payload: payload, Changes: changes, Request: r,
	})
}

// publish tells open resource lists of the organization what changed.
// Closure changes count as an update of the resource.
func (h *Handler) publish(r *http.Request, action string, id uuid.UUID) {
	if h.events == nil {
		return
	}
	ev := resourceevents.ActionUpdated
	switch action {
	case "resources.created":
		ev = resourceevents.ActionCreated
	case "resources.deleted":
		ev = resourceevents.ActionDeleted
	}
	h.events.Publish(r.Context(), model.Slug, ev, id.String())
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: name, Message: name + " is invalid", Code: "invalid"}})
		return uuid.Nil, false
	}
	return id, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var qve *apiquery.ValidationError
	if errors.As(err, &qve) {
		details := make([]response.Detail, 0, len(qve.Details))
		for _, d := range qve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		details := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Resource was not found")
	case errors.Is(err, usecase.ErrClosureNotFound):
		response.NotFound(w, r, "Closed period was not found")
	case errors.Is(err, usecase.ErrNameTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "name", Message: err.Error(), Code: "taken",
		}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
