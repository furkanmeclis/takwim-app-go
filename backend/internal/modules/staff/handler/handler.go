// Package handler is the HTTP layer of the staff tenant resource. Every
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

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves /v1/tenant/staff.
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

// Meta serves GET /v1/tenant/staff/meta.
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, Meta())
}

// Facets serves GET /v1/tenant/staff/facets.
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	out, err := h.svc.Facets(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// LinkableMembers serves GET /v1/tenant/staff/linkable-members.
func (h *Handler) LinkableMembers(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	out, err := h.svc.LinkableMembers(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": out})
}

// List serves GET /v1/tenant/staff.
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
	f := model.ListFilter{Limit: q.Limit, Offset: q.Offset, Q: q.Q, SortField: "created_at", SortDesc: true}
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
	for _, b := range []struct {
		key string
		dst **bool
	}{{"is_active", &f.IsActive}, {"show_online", &f.ShowOnline}, {"has_account", &f.HasAccount}} {
		if raw := strings.TrimSpace(values.Get(b.key)); raw != "" {
			v, err := strconv.ParseBool(raw)
			if err != nil {
				details = append(details, response.Detail{Field: b.key, Message: b.key + " must be true or false", Code: "invalid"})
				continue
			}
			*b.dst = &v
		}
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
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

// Create serves POST /v1/tenant/staff.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in model.StaffInput
	if !decodeJSON(w, r, &in) {
		return
	}
	st, err := h.svc.Create(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "staff.created", st, map[string]any{"name": st.Name}, nil)
	response.JSON(w, r, http.StatusCreated, st)
}

// Get serves GET /v1/tenant/staff/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	st, err := h.svc.Get(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

// staffDiffSkip are the response fields left out of the before / after of
// a profile edit (derived, or recorded by the shift events).
var staffDiffSkip = []string{"created_at", "updated_at", "assignments"}

// Patch serves PATCH /v1/tenant/staff/{uuid}.
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.StaffInput
	if !decodeJSON(w, r, &in) {
		return
	}
	before, beforeErr := h.svc.Get(r.Context(), scope.ID, id)
	st, err := h.svc.Patch(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	var changes map[string]activity.Change
	if beforeErr == nil {
		changes = activity.Diff(profileView(before), profileView(st), staffDiffSkip...)
	}
	h.record(r, "staff.updated", st, map[string]any{"name": st.Name}, changes)
	response.JSON(w, r, http.StatusOK, st)
}

// Delete serves DELETE /v1/tenant/staff/{uuid} (soft delete).
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	before, _ := h.svc.Get(r.Context(), scope.ID, id)
	if err := h.svc.Delete(r.Context(), scope.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	before.UUID = id
	h.record(r, "staff.deleted", before, map[string]any{"name": before.Name}, nil)
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// Activity serves GET /v1/tenant/staff/{uuid}/activity.
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

// PutShifts serves PUT /v1/tenant/staff/{uuid}/branches/{branchUuid}/shifts.
func (h *Handler) PutShifts(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	branchID, ok := pathUUID(w, r, "branchUuid")
	if !ok {
		return
	}
	var in model.WeeklyShifts
	if !decodeJSON(w, r, &in) {
		return
	}
	before, beforeErr := h.svc.Get(r.Context(), scope.ID, id)
	st, err := h.svc.SetShifts(r.Context(), scope.ID, id, branchID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	var changes map[string]activity.Change
	if beforeErr == nil {
		changes = activity.Diff(shiftsOf(before, branchID), shiftsOf(st, branchID))
	}
	branchName := ""
	for _, a := range st.Assignments {
		if a.BranchUUID == branchID {
			branchName = a.BranchName
		}
	}
	h.recordEntry(r, "staff.shifts_updated", st.UUID, &branchID,
		map[string]any{"name": st.Name, "branch": branchName}, changes)
	response.JSON(w, r, http.StatusOK, st)
}

// ListTimeOff serves GET /v1/tenant/staff/{uuid}/time-off.
func (h *Handler) ListTimeOff(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	values := r.URL.Query()
	q := apiquery.Parse(values)
	f := model.TimeOffFilter{Limit: q.Limit, Offset: q.Offset}
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
	items, total, err := h.svc.ListTimeOff(r.Context(), scope.ID, id, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateTimeOff serves POST /v1/tenant/staff/{uuid}/time-off.
func (h *Handler) CreateTimeOff(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.TimeOffInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	off, err := h.svc.CreateTimeOff(r.Context(), scope.ID, id, &p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordEntry(r, "staff.time_off_created", id, nil, timeOffPayload(off), nil)
	response.JSON(w, r, http.StatusCreated, off)
}

// PatchTimeOff serves PATCH /v1/tenant/staff/{uuid}/time-off/{timeOffUuid}.
func (h *Handler) PatchTimeOff(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	offID, ok := pathUUID(w, r, "timeOffUuid")
	if !ok {
		return
	}
	var in model.TimeOffInput
	if !decodeJSON(w, r, &in) {
		return
	}
	before, off, err := h.svc.PatchTimeOff(r.Context(), scope.ID, id, offID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordEntry(r, "staff.time_off_updated", id, nil, timeOffPayload(off),
		activity.Diff(before, off, "uuid", "created_at"))
	response.JSON(w, r, http.StatusOK, off)
}

// DeleteTimeOff serves DELETE /v1/tenant/staff/{uuid}/time-off/{timeOffUuid}.
func (h *Handler) DeleteTimeOff(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	offID, ok := pathUUID(w, r, "timeOffUuid")
	if !ok {
		return
	}
	off, err := h.svc.DeleteTimeOff(r.Context(), scope.ID, id, offID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordEntry(r, "staff.time_off_deleted", id, nil, timeOffPayload(off), nil)
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

func timeOffPayload(off model.TimeOff) map[string]any {
	return map[string]any{
		"time_off_uuid": off.UUID.String(), "kind": off.Kind,
		"starts_at": off.StartsAt, "ends_at": off.EndsAt,
	}
}

// profileView is the part of a staff member a profile edit changes; the
// account is reduced to its e-mail and the branches to their names.
func profileView(st model.Staff) map[string]any {
	branches := make([]string, 0, len(st.Branches))
	for _, b := range st.Branches {
		branches = append(branches, b.Name)
	}
	account := ""
	if st.Account != nil {
		account = st.Account.Email
	}
	return map[string]any{
		"name": st.Name, "title": st.Title, "bio": st.Bio, "phone": st.Phone, "email": st.Email,
		"color": st.Color, "show_online": st.ShowOnline, "is_active": st.IsActive,
		"account": account, "branches": branches,
	}
}

func shiftsOf(st model.Staff, branchID uuid.UUID) model.WeeklyShifts {
	for _, a := range st.Assignments {
		if a.BranchUUID == branchID {
			return a.Shifts
		}
	}
	return model.WeeklyShifts{}
}

// record writes the activity event of a staff member and publishes the
// list change. The event belongs to the person's branch when there is one.
func (h *Handler) record(r *http.Request, action string, st model.Staff, payload map[string]any, changes map[string]activity.Change) {
	var branch *uuid.UUID
	if len(st.Branches) == 1 {
		branch = &st.Branches[0].UUID
	}
	h.recordEntry(r, action, st.UUID, branch, payload, changes)
}

func (h *Handler) recordEntry(r *http.Request, action string, id uuid.UUID, branch *uuid.UUID, payload map[string]any, changes map[string]activity.Change) {
	h.publish(r, action, id)
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.RecordEntry(r.Context(), activity.Entry{
		ActorID: &p.UserInternal, Action: action, Resource: model.Resource, ResourceUUID: &id,
		BranchUUID: branch, Payload: payload, Changes: changes, Request: r,
	})
}

// publish tells open staff lists of the organization what changed. Shift
// and time off changes count as an update of the person.
func (h *Handler) publish(r *http.Request, action string, id uuid.UUID) {
	if h.events == nil {
		return
	}
	ev := resourceevents.ActionUpdated
	switch action {
	case "staff.created":
		ev = resourceevents.ActionCreated
	case "staff.deleted":
		ev = resourceevents.ActionDeleted
	}
	h.events.Publish(r.Context(), model.Resource, ev, id.String())
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
	if entitlement.WriteLimitError(w, r, err) {
		return
	}
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
		response.NotFound(w, r, "Staff member was not found")
	case errors.Is(err, usecase.ErrAssignmentNotFound):
		response.NotFound(w, r, err.Error())
	case errors.Is(err, usecase.ErrTimeOffNotFound):
		response.NotFound(w, r, "Time off was not found")
	case errors.Is(err, usecase.ErrOutOfScope):
		response.Forbidden(w, r, err.Error())
	case errors.Is(err, usecase.ErrAccountTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "account_uuid", Message: err.Error(), Code: "taken",
		}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}

