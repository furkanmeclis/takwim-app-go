// Package handler is the HTTP layer of the class timetable (TEC-970).
// Every handler runs behind RequireOrganization and reads the organization
// id from orgctx.Scope; the client never sends it.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/timetable"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Handler serves /v1/tenant/classes/* and /v1/tenant/class-sessions.
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

// Meta serves GET /v1/tenant/classes/meta.
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, ScheduleMeta())
}

// Facets serves GET /v1/tenant/classes/facets.
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Facets(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Form serves GET /v1/tenant/classes/form.
func (h *Handler) Form(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Form(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// List serves GET /v1/tenant/classes.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, usecase.ScheduleSortColumns)
	if !ok {
		return
	}
	values := r.URL.Query()
	var details []response.Detail
	f := model.ScheduleFilter{
		Q: q.Q, Branches: parseUUIDList(values, "branch", &details), Services: parseUUIDList(values, "service", &details),
		Staff: parseUUIDList(values, "staff", &details), Weekdays: parseEnum(values, "weekday", timetable.WeekdayKeys, &details),
		Limit: int(q.Limit), Offset: int(q.Offset),
	}
	if active := parseEnum(values, "is_active", []string{"true", "false"}, &details); len(active) == 1 {
		v := active[0] == "true"
		f.IsActive = &v
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.List(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Get serves GET /v1/tenant/classes/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Get(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Create serves POST /v1/tenant/classes.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in model.ScheduleInput
	if !decodeJSON(w, r, &in) {
		return
	}
	pr := authctx.MustPrincipal(r.Context())
	out, err := h.svc.Create(r.Context(), orgctx.Must(r.Context()).ID, &pr.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.publish(r, resourceevents.ActionCreated, out.UUID)
	branch := out.Branch
	h.record(r, "classes.created", out.UUID, &branch, schedulePayload(out))
	response.JSON(w, r, http.StatusCreated, out)
}

// Patch serves PATCH /v1/tenant/classes/{uuid} (the whole class).
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.ScheduleInput
	if !decodeJSON(w, r, &in) {
		return
	}
	before, after, err := h.svc.Update(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.publish(r, resourceevents.ActionUpdated, id)
	branch := after.Branch
	h.record(r, "classes.updated", id, &branch, map[string]any{
		"before": schedulePayload(before), "after": schedulePayload(after),
	})
	response.JSON(w, r, http.StatusOK, after)
}

// Delete serves DELETE /v1/tenant/classes/{uuid}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Delete(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.publish(r, resourceevents.ActionDeleted, id)
	branch := out.Branch
	h.record(r, "classes.deleted", id, &branch, schedulePayload(out))
	w.WriteHeader(http.StatusNoContent)
}

// Sessions serves GET /v1/tenant/class-sessions.
func (h *Handler) Sessions(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, usecase.SessionSortColumns)
	if !ok {
		return
	}
	values := r.URL.Query()
	var details []response.Detail
	f := model.SessionFilter{
		From: values.Get("from"), To: values.Get("to"), Q: q.Q,
		Branches: parseUUIDList(values, "branch", &details), Services: parseUUIDList(values, "service", &details),
		Staff: parseUUIDList(values, "staff", &details), States: parseEnum(values, "state", model.States, &details),
		Limit: int(q.Limit), Offset: int(q.Offset),
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.Sessions(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, int64(total), q.Limit, q.Offset))
}

// Session serves GET /v1/tenant/classes/{uuid}/sessions/{date}.
func (h *Handler) Session(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.SessionDetail(r.Context(), orgctx.Must(r.Context()).ID, id, r.PathValue("date"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ChangeSession serves PATCH /v1/tenant/classes/{uuid}/sessions/{date}.
func (h *Handler) ChangeSession(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.SessionChangeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	pr := authctx.MustPrincipal(r.Context())
	date := r.PathValue("date")
	before, after, err := h.svc.ChangeSession(r.Context(), orgctx.Must(r.Context()).ID, id, date, &pr.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.publish(r, resourceevents.ActionUpdated, id)
	action := "classes.session_changed"
	switch {
	case after.Cancelled && !before.Cancelled:
		action = "classes.session_cancelled"
	case !after.Cancelled && before.Cancelled:
		action = "classes.session_restored"
	}
	branch := after.Branch
	h.record(r, action, id, &branch, map[string]any{
		"date": date, "service": after.Service.Name, "starts_at": after.StartsAt,
		"capacity_before": before.Capacity, "capacity": after.Capacity, "booked": after.Booked,
		"reason": after.Reason,
	})
	detail, err := h.svc.SessionDetail(r.Context(), orgctx.Must(r.Context()).ID, id, date)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, detail)
}

func schedulePayload(s model.Schedule) map[string]any {
	out := map[string]any{
		"service": s.Service.Name, "branch": s.Branch.Name, "weekday": s.Weekday, "start_time": s.StartTime,
		"seats": s.Seats, "valid_from": s.ValidFrom, "is_active": s.IsActive,
	}
	if s.Staff != nil {
		out["staff"] = s.Staff.Name
	}
	if s.Resource != nil {
		out["resource"] = s.Resource.Name
	}
	if s.ValidUntil != nil {
		out["valid_until"] = *s.ValidUntil
	}
	return out
}

// ------------------------------------------------------------------ helpers

func parseUUIDList(values map[string][]string, key string, details *[]response.Detail) []uuid.UUID {
	var out []uuid.UUID
	for _, raw := range values[key] {
		for _, part := range apiquery.SplitCSV(raw) {
			id, err := uuid.Parse(part)
			if err != nil {
				*details = append(*details, response.Detail{Field: key, Message: key + " must list uuids", Code: "invalid"})
				return nil
			}
			out = append(out, id)
		}
	}
	return out
}

func parseEnum(values map[string][]string, key string, allowed []string, details *[]response.Detail) []string {
	var out []string
	for _, raw := range values[key] {
		for _, v := range apiquery.SplitCSV(raw) {
			if !slices.Contains(allowed, v) {
				*details = append(*details, response.Detail{Field: key, Message: key + " must be one of " + strings.Join(allowed, ", "), Code: "invalid"})
				return nil
			}
			if !slices.Contains(out, v) {
				out = append(out, v)
			}
		}
	}
	return out
}

// listQuery reads limit / offset / q / sort; false when it answered.
func listQuery(w http.ResponseWriter, r *http.Request, sortable map[string]string) (apiquery.Query, bool) {
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden"}})
		return apiquery.Query{}, false
	}
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns(sortable)); err != nil {
		writeError(w, r, err)
		return q, false
	}
	return q, true
}

func (h *Handler) record(r *http.Request, action string, id uuid.UUID, branch *model.Ref, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	e := activity.Entry{ActorID: &p.UserInternal, Action: action, Resource: model.Slug, ResourceUUID: &id, Payload: payload, Request: r}
	if branch != nil {
		b := branch.UUID
		e.BranchUUID = &b
	}
	h.activity.RecordEntry(r.Context(), e)
}

func (h *Handler) publish(r *http.Request, action string, id uuid.UUID) {
	if h.events == nil {
		return
	}
	h.events.Publish(r.Context(), model.Slug, action, id.String())
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "uuid", Message: "uuid is invalid", Code: "invalid"}})
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

func conflict(w http.ResponseWriter, r *http.Request, field, code string, err error) {
	response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
		Field: field, Message: err.Error(), Code: code,
	}})
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
	var ce *usecase.ConflictError
	if errors.As(err, &ce) {
		conflict(w, r, ce.Field, ce.Code, err)
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Class was not found")
	case errors.Is(err, usecase.ErrSessionNotFound):
		response.NotFound(w, r, "Class session was not found")
	case errors.Is(err, usecase.ErrHasBookings):
		conflict(w, r, "sessions", "has_bookings", err)
	case errors.Is(err, usecase.ErrBelowBooked):
		conflict(w, r, "capacity", "below_booked", err)
	case errors.Is(err, usecase.ErrPast):
		conflict(w, r, "date", "past", err)
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
