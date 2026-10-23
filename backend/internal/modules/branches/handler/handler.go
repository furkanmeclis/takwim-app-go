// Package handler is the HTTP layer of the branches tenant resource. Every
// handler runs behind RequireOrganization and reads the organization id from
// orgctx.Scope; the client never sends it.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves /v1/tenant/branches.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
	events   resourceevents.Publisher
}

// SetResourceEvents publishes live list changes of the organization's
// branches (optional).
func (h *Handler) SetResourceEvents(pub resourceevents.Publisher) {
	h.events = pub
}

// New builds the handler. rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// Meta serves GET /v1/tenant/branches/meta.
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, Meta())
}

// Facets serves GET /v1/tenant/branches/facets.
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	facets, err := h.svc.Facets(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, facets)
}

// List serves GET /v1/tenant/branches.
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
	f := model.ListFilter{
		Limit: q.Limit, Offset: q.Offset, Q: q.Q, SortField: "created_at", SortDesc: true,
	}
	// city repeats or lists comma-separated values (faceted filter).
	for _, raw := range values["city"] {
		f.Cities = append(f.Cities, adapters.SplitCities(raw)...)
	}
	if raw := strings.TrimSpace(values.Get("is_active")); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "is_active", Message: "is_active must be true or false", Code: "invalid"}})
			return
		}
		f.IsActive = &v
	}
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

// Create serves POST /v1/tenant/branches.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in model.BranchInput
	if !decodeJSON(w, r, &in) {
		return
	}
	b, err := h.svc.Create(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "branches.created", b.UUID, map[string]any{"code": b.Code, "name": b.Name})
	response.JSON(w, r, http.StatusCreated, b)
}

// Get serves GET /v1/tenant/branches/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	b, err := h.svc.Get(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, b)
}

// Patch serves PATCH /v1/tenant/branches/{uuid}.
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.BranchInput
	if !decodeJSON(w, r, &in) {
		return
	}
	b, err := h.svc.Patch(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "branches.updated", b.UUID, map[string]any{"code": b.Code, "fields": changedFields(in)})
	response.JSON(w, r, http.StatusOK, b)
}

// Delete serves DELETE /v1/tenant/branches/{uuid} (soft delete).
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), scope.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "branches.deleted", id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// Activity serves GET /v1/tenant/branches/{uuid}/activity.
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

// ListServicePoints serves GET /v1/tenant/branches/{uuid}/service-points.
func (h *Handler) ListServicePoints(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	items, err := h.svc.ListServicePoints(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// CreateServicePoint serves POST /v1/tenant/branches/{uuid}/service-points.
func (h *Handler) CreateServicePoint(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ServicePointInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p, err := h.svc.CreateServicePoint(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "branches.service_point_created", id, map[string]any{
		"service_point_uuid": p.UUID.String(), "name": p.Name, "type": p.Type,
	})
	response.JSON(w, r, http.StatusCreated, p)
}

// PatchServicePoint serves PATCH /v1/tenant/branches/{uuid}/service-points/{pointUuid}.
func (h *Handler) PatchServicePoint(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	pointID, ok := pathUUID(w, r, "pointUuid")
	if !ok {
		return
	}
	var in model.ServicePointInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p, err := h.svc.PatchServicePoint(r.Context(), scope.ID, id, pointID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "branches.service_point_updated", id, map[string]any{
		"service_point_uuid": p.UUID.String(), "name": p.Name,
	})
	response.JSON(w, r, http.StatusOK, p)
}

// DeleteServicePoint serves DELETE /v1/tenant/branches/{uuid}/service-points/{pointUuid}.
func (h *Handler) DeleteServicePoint(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	pointID, ok := pathUUID(w, r, "pointUuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteServicePoint(r.Context(), scope.ID, id, pointID); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "branches.service_point_deleted", id, map[string]any{"service_point_uuid": pointID.String()})
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// record writes a fail-soft activity event; the recorder stamps the
// organization from the request scope.
func (h *Handler) record(r *http.Request, action string, branchUUID uuid.UUID, payload map[string]any) {
	h.publish(r, action, branchUUID)
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, model.Resource, &branchUUID, payload, r)
}

// publish tells open branch lists of the organization what changed. Service
// point changes count as an update of their branch (points count column).
// Engine jobs (import / bulk) publish their own single event.
func (h *Handler) publish(r *http.Request, action string, branchUUID uuid.UUID) {
	if h.events == nil {
		return
	}
	ev := resourceevents.ActionUpdated
	switch action {
	case "branches.created":
		ev = resourceevents.ActionCreated
	case "branches.deleted":
		ev = resourceevents.ActionDeleted
	}
	h.events.Publish(r.Context(), model.Resource, ev, branchUUID.String())
}

func changedFields(in model.BranchInput) []string {
	out := []string{}
	add := func(set bool, name string) {
		if set {
			out = append(out, name)
		}
	}
	add(in.Code != nil, "code")
	add(in.Name != nil, "name")
	add(in.Address != nil, "address")
	add(in.City != nil, "city")
	add(in.District != nil, "district")
	add(in.Latitude.Set, "latitude")
	add(in.Longitude.Set, "longitude")
	add(in.Phone != nil, "phone")
	add(in.Email != nil, "email")
	add(in.WorkingHours != nil, "working_hours")
	add(in.HolidayExceptions != nil, "holiday_exceptions")
	add(in.PrepMinutes != nil, "prep_minutes")
	add(in.IsActive != nil, "is_active")
	return out
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
		response.NotFound(w, r, "Branch was not found")
	case errors.Is(err, usecase.ErrAllBranchesRequired):
		response.Forbidden(w, r, err.Error())
	case errors.Is(err, usecase.ErrServicePointNotFound):
		response.NotFound(w, r, "Service point was not found")
	case errors.Is(err, usecase.ErrCodeTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "code", Message: err.Error(), Code: "taken",
		}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
