// Package handler is the HTTP layer of the customers tenant resource. Every
// handler runs behind RequireOrganization and reads the organization id from
// orgctx.Scope; the client never sends it.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves /v1/tenant/customers.
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

// Meta serves GET /v1/tenant/customers/meta.
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, Meta())
}

// Facets serves GET /v1/tenant/customers/facets.
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
	for _, raw := range values["tag"] {
		f.Tags = append(f.Tags, apiquery.SplitCSV(raw)...)
	}
	for _, raw := range values["source"] {
		f.Sources = append(f.Sources, apiquery.SplitCSV(raw)...)
	}
	for _, b := range []struct {
		key string
		dst **bool
	}{{"sms_consent", &f.SMSConsent}, {"email_consent", &f.EmailConsent}, {"verified", &f.Verified}} {
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

// List serves GET /v1/tenant/customers.
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

// Create serves POST /v1/tenant/customers.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in model.Input
	if !decodeJSON(w, r, &in) {
		return
	}
	c, err := h.svc.Create(r.Context(), scope.ID, in, "panel")
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "customers.created", c, nil)
	response.JSON(w, r, http.StatusCreated, c)
}

// Get serves GET /v1/tenant/customers/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	c, err := h.svc.Get(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, c)
}

// Patch serves PATCH /v1/tenant/customers/{uuid}.
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
	c, err := h.svc.Patch(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	var changes map[string]activity.Change
	if beforeErr == nil {
		changes = activity.Diff(diffView(before), diffView(c))
	}
	h.record(r, "customers.updated", c, changes)
	response.JSON(w, r, http.StatusOK, c)
}

// Delete serves DELETE /v1/tenant/customers/{uuid} (soft delete).
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	c, err := h.svc.Delete(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "customers.deleted", c, nil)
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// Activity serves GET /v1/tenant/customers/{uuid}/activity.
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

// diffView is the part of a customer an edit changes.
func diffView(c model.Customer) map[string]any {
	return map[string]any{
		"first_name": c.FirstName, "last_name": c.LastName, "phone": c.Phone, "email": c.Email,
		"birth_date": c.BirthDate, "notes": c.Notes, "tags": c.Tags, "sms_consent": c.SMSConsent,
		"email_consent": c.EmailConsent, "verified": c.Verification != nil,
	}
}

// record writes the activity event of a customer and publishes the list
// change.
func (h *Handler) record(r *http.Request, action string, c model.Customer, changes map[string]activity.Change) {
	h.publish(r, action, c.UUID)
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	id := c.UUID
	h.activity.RecordEntry(r.Context(), activity.Entry{
		ActorID: &p.UserInternal, Action: action, Resource: model.Slug, ResourceUUID: &id,
		Payload: map[string]any{"name": c.FullName}, Changes: changes, Request: r,
	})
}

// publish tells open customer lists of the organization what changed.
func (h *Handler) publish(r *http.Request, action string, id uuid.UUID) {
	if h.events == nil {
		return
	}
	ev := resourceevents.ActionUpdated
	switch action {
	case "customers.created":
		ev = resourceevents.ActionCreated
	case "customers.deleted":
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
		response.NotFound(w, r, "Customer was not found")
	case errors.Is(err, usecase.ErrPhoneTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "phone", Message: err.Error(), Code: "taken",
		}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
