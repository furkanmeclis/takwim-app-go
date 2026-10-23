// Package handler is the HTTP layer of reservations (TEC-900). Every
// handler runs behind RequireOrganization and reads the organization id from
// orgctx.Scope; the client never sends it. A member holding only
// tenant.reservations.own works on its own calendar.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Handler serves /v1/tenant/reservations.
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

// caller resolves the reach of a full / own slug pair; false = answered 403.
func caller(w http.ResponseWriter, r *http.Request, full string) (availabilityusecase.Caller, bool) {
	scope := orgctx.Must(r.Context())
	switch scope.Reach(full, rbac.PermTenantReservationsOwn) {
	case orgctx.ReachNone:
		response.Forbidden(w, r, "Missing permission: "+full)
		return availabilityusecase.Caller{}, false
	case orgctx.ReachOwn:
		return availabilityusecase.Caller{OwnUserID: authctx.MustPrincipal(r.Context()).UserInternal}, true
	}
	return availabilityusecase.Caller{}, true
}

// Meta serves GET /v1/tenant/reservations/meta.
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, Meta())
}

// Facets serves GET /v1/tenant/reservations/facets.
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Facets(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

var listEnums = map[string][]string{
	"status":          {model.StatusRequested, model.StatusActive, model.StatusInService, model.StatusCompleted, model.StatusCancelled, model.StatusRejected, model.StatusNoShow},
	"approval_status": {model.ApprovalPending, model.ApprovalAccepted, model.ApprovalRejected},
	"payment_status":  {model.PaymentOnSite, model.PaymentOnlinePending, model.PaymentPaid, model.PaymentRefundPending, model.PaymentPartiallyRefunded, model.PaymentRefunded, model.PaymentFailed},
	"source":          {model.SourcePanel, model.SourceOnline, model.SourceEmbed},
}

// ParseListFilter reads the list filters.
func ParseListFilter(values map[string][]string) (model.ListFilter, []response.Detail) {
	var f model.ListFilter
	var details []response.Detail
	csv := func(key string) []string {
		var out []string
		for _, raw := range values[key] {
			out = append(out, apiquery.SplitCSV(raw)...)
		}
		return out
	}
	uuids := func(key string) []uuid.UUID {
		var out []uuid.UUID
		for _, v := range csv(key) {
			id, err := uuid.Parse(v)
			if err != nil {
				details = append(details, response.Detail{Field: key, Message: key + " must be uuids", Code: "invalid"})
				return nil
			}
			out = append(out, id)
		}
		return out
	}
	enum := func(key string) []string {
		list := csv(key)
		for _, v := range list {
			ok := false
			for _, allowed := range listEnums[key] {
				ok = ok || v == allowed
			}
			if !ok {
				details = append(details, response.Detail{Field: key, Message: key + " has an unknown value", Code: "invalid"})
				return nil
			}
		}
		return list
	}
	instant := func(key string) *time.Time {
		raw := strings.TrimSpace(firstValue(values, key))
		if raw == "" {
			return nil
		}
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			details = append(details, response.Detail{Field: key, Message: key + " must be an RFC 3339 time", Code: "invalid"})
			return nil
		}
		return &t
	}
	f.Branches, f.Staff = uuids("branch"), uuids("staff")
	if c := uuids("customer_uuid"); len(c) > 0 {
		f.CustomerUUID = &c[0]
	}
	f.Statuses, f.ApprovalStatuses, f.PaymentStatuses, f.Sources = enum("status"), enum("approval_status"), enum("payment_status"), enum("source")
	f.StartsFrom, f.StartsTo = instant("starts_from"), instant("starts_to")
	return f, details
}

func firstValue(values map[string][]string, key string) string {
	if list := values[key]; len(list) > 0 {
		return list[0]
	}
	return ""
}

// List serves GET /v1/tenant/reservations.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return
	}
	c, ok := caller(w, r, rbac.PermTenantReservationsRead)
	if !ok {
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
	items, total, err := h.svc.List(r.Context(), scope.ID, f, c)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Create serves POST /v1/tenant/reservations: a booking made in the panel
// (source panel, paid at the business, active at once).
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in model.CreateInput
	if !decodeJSON(w, r, &in) {
		return
	}
	c, ok := caller(w, r, rbac.PermTenantReservationsWrite)
	if !ok {
		return
	}
	in.Source = model.SourcePanel
	out, err := h.svc.Create(r.Context(), scope.ID, in, c, authctx.MustPrincipal(r.Context()).UserInternal)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "reservations.created", out, nil)
	response.JSON(w, r, http.StatusCreated, out)
}

// Get serves GET /v1/tenant/reservations/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	c, ok := caller(w, r, rbac.PermTenantReservationsRead)
	if !ok {
		return
	}
	out, err := h.svc.Get(r.Context(), scope.ID, id, c)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

type actionFunc func(h *Handler, r *http.Request, orgID int64, id uuid.UUID, c availabilityusecase.Caller, actor int64, in usecase.ChangeInput) (model.Reservation, error)

var actions = map[string]actionFunc{
	"accept": func(h *Handler, r *http.Request, org int64, id uuid.UUID, c availabilityusecase.Caller, a int64, in usecase.ChangeInput) (model.Reservation, error) {
		return h.svc.Accept(r.Context(), org, id, c, a, in)
	},
	"reject": func(h *Handler, r *http.Request, org int64, id uuid.UUID, c availabilityusecase.Caller, a int64, in usecase.ChangeInput) (model.Reservation, error) {
		return h.svc.Reject(r.Context(), org, id, c, a, in)
	},
	"cancel": func(h *Handler, r *http.Request, org int64, id uuid.UUID, c availabilityusecase.Caller, a int64, in usecase.ChangeInput) (model.Reservation, error) {
		return h.svc.Cancel(r.Context(), org, id, c, a, in)
	},
	"start": func(h *Handler, r *http.Request, org int64, id uuid.UUID, c availabilityusecase.Caller, a int64, in usecase.ChangeInput) (model.Reservation, error) {
		return h.svc.Start(r.Context(), org, id, c, a, in)
	},
	"complete": func(h *Handler, r *http.Request, org int64, id uuid.UUID, c availabilityusecase.Caller, a int64, in usecase.ChangeInput) (model.Reservation, error) {
		return h.svc.Complete(r.Context(), org, id, c, a, in)
	},
	"no-show": func(h *Handler, r *http.Request, org int64, id uuid.UUID, c availabilityusecase.Caller, a int64, in usecase.ChangeInput) (model.Reservation, error) {
		return h.svc.NoShow(r.Context(), org, id, c, a, in)
	},
}

// actionEvents names the activity entry of each action (past tense, like
// the reservation's own history).
var actionEvents = map[string]string{
	"accept": "accepted", "reject": "rejected", "cancel": "cancelled",
	"start": "started", "complete": "completed", "no-show": "no_show",
}

// Action serves POST /v1/tenant/reservations/{uuid}/{action} for accept,
// reject, cancel, start, complete and no-show.
func (h *Handler) Action(name string) http.HandlerFunc {
	fn := actions[name]
	return func(w http.ResponseWriter, r *http.Request) {
		scope := orgctx.Must(r.Context())
		id, ok := pathUUID(w, r)
		if !ok {
			return
		}
		var in usecase.ChangeInput
		if !decodeOptionalJSON(w, r, &in) {
			return
		}
		c, ok := caller(w, r, rbac.PermTenantReservationsWrite)
		if !ok {
			return
		}
		out, err := fn(h, r, scope.ID, id, c, authctx.MustPrincipal(r.Context()).UserInternal, in)
		if err != nil {
			writeError(w, r, err)
			return
		}
		h.record(r, "reservations."+actionEvents[name], out, map[string]any{"reason": in.Reason, "note": in.Note})
		response.JSON(w, r, http.StatusOK, out)
	}
}

// Reschedule serves POST /v1/tenant/reservations/{uuid}/reschedule.
func (h *Handler) Reschedule(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.RescheduleInput
	if !decodeJSON(w, r, &in) {
		return
	}
	c, ok := caller(w, r, rbac.PermTenantReservationsWrite)
	if !ok {
		return
	}
	out, err := h.svc.Reschedule(r.Context(), scope.ID, id, c, authctx.MustPrincipal(r.Context()).UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "reservations.rescheduled", out, map[string]any{"starts_at": out.StartsAt})
	response.JSON(w, r, http.StatusOK, out)
}

// Notes serves PATCH /v1/tenant/reservations/{uuid}.
func (h *Handler) Notes(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.NotesInput
	if !decodeJSON(w, r, &in) {
		return
	}
	c, ok := caller(w, r, rbac.PermTenantReservationsWrite)
	if !ok {
		return
	}
	out, err := h.svc.UpdateNotes(r.Context(), scope.ID, id, c, authctx.MustPrincipal(r.Context()).UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "reservations.updated", out, nil)
	response.JSON(w, r, http.StatusOK, out)
}

// record writes the activity event and publishes the list change.
func (h *Handler) record(r *http.Request, action string, res model.Reservation, extra map[string]any) {
	if h.events != nil {
		ev := resourceevents.ActionUpdated
		if action == "reservations.created" {
			ev = resourceevents.ActionCreated
		}
		h.events.Publish(r.Context(), model.Slug, ev, res.UUID.String())
	}
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	id := res.UUID
	payload := map[string]any{"number": res.Number, "status": res.Status, "customer": res.Customer.Name}
	for k, v := range extra {
		if s, ok := v.(string); ok && s == "" {
			continue
		}
		payload[k] = v
	}
	h.activity.RecordEntry(r.Context(), activity.Entry{
		ActorID: &p.UserInternal, Action: action, Resource: model.Slug, ResourceUUID: &id, Payload: payload, Request: r,
	})
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Reservation was not found")
		return uuid.Nil, false
	}
	return id, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

// decodeOptionalJSON accepts an empty body (an action without a note).
func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.ContentLength == 0 {
		return true
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil && !errors.Is(err, io.EOF) {
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
	var ave *availabilityusecase.ValidationError
	if errors.As(err, &ave) {
		details := make([]response.Detail, 0, len(ave.Details))
		for _, d := range ave.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Reservation was not found")
	case errors.Is(err, availabilityusecase.ErrNotFound):
		// The branch of the body: unknown, of another business or outside
		// the branch scope.
		response.ValidationError(w, r, []response.Detail{{Field: "branch_uuid", Message: "branch not found", Code: "not_found"}})
	case errors.Is(err, availabilityusecase.ErrForbidden):
		response.Forbidden(w, r, err.Error())
	case errors.Is(err, availabilityusecase.ErrSlotTaken):
		conflict(w, r, "starts_at", "taken", err)
	case errors.Is(err, usecase.ErrStaleVersion):
		conflict(w, r, "expected_version", "stale", err)
	case errors.Is(err, usecase.ErrInvalidTransition):
		conflict(w, r, "status", "invalid_transition", err)
	case errors.Is(err, availabilityusecase.ErrHoldExpired):
		conflict(w, r, "status", "expired", err)
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
