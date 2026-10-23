// Package handler is the HTTP layer of the shared customer panel (TEC-909):
// /v1/customer/*. Sign-in routes are public and rate limited; every other
// route needs a customer token (kind customer) in Authorization: Bearer.
// Staff tokens are refused here, and customer tokens are refused on every
// staff route (jwt.ParseAccess), so the two never cross.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/usecase"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Error codes of the customer panel.
const (
	CodeInvalidCode      = "OTP_INVALID"
	CodeActionNotAllowed = "ACTION_NOT_ALLOWED"
)

// Limit is one fixed window.
type Limit struct {
	Max    int
	Window time.Duration
}

// Limits are the rate limits of the customer routes.
type Limits struct {
	StartIP     Limit // code requests per IP
	StartPhone  Limit // code requests per phone
	VerifyIP    Limit // code checks per IP
	VerifyPhone Limit // code checks per phone
	Read        Limit // signed-in reads per session
	Search      Limit // reschedule searches per session
	Write       Limit // cancel / reschedule per session
}

// DefaultLimits are the production limits.
var DefaultLimits = Limits{
	StartIP:     Limit{Max: 10, Window: 10 * time.Minute},
	StartPhone:  Limit{Max: 5, Window: 10 * time.Minute},
	VerifyIP:    Limit{Max: 30, Window: 10 * time.Minute},
	VerifyPhone: Limit{Max: 10, Window: 10 * time.Minute},
	Read:        Limit{Max: 240, Window: time.Minute},
	Search:      Limit{Max: 60, Window: time.Minute},
	Write:       Limit{Max: 20, Window: 10 * time.Minute},
}

// RateLimiter is a fixed-window limiter (ratelimit.Limiter).
type RateLimiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// Handler serves /v1/customer/*.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
	events   resourceevents.Publisher
	limiter  RateLimiter
	limits   Limits
}

// New builds the handler. rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec, limits: DefaultLimits}
}

// SetRateLimiter plugs in the limiter (nil = no limits).
func (h *Handler) SetRateLimiter(l RateLimiter) {
	h.limiter = l
}

// SetLimits replaces the limits (tests).
func (h *Handler) SetLimits(l Limits) {
	h.limits = l
}

// SetResourceEvents publishes changes to the business panel's live lists.
func (h *Handler) SetResourceEvents(pub resourceevents.Publisher) {
	h.events = pub
}

type principalKey struct{}

func principalFrom(ctx context.Context) usecase.Principal {
	p, _ := ctx.Value(principalKey{}).(usecase.Principal)
	return p
}

// Authenticate requires a live customer session.
func (h *Handler) Authenticate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := bearerToken(r.Header.Get("Authorization"))
		if raw == "" {
			response.Unauthorized(w, r, "Customer session is required")
			return
		}
		p, err := h.svc.Authenticate(r.Context(), raw)
		if err != nil {
			if !errors.Is(err, usecase.ErrUnauthorized) {
				response.InternalErr(w, r, err, "Unexpected server error")
				return
			}
			response.Unauthorized(w, r, "Customer session is not valid")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	}
}

func bearerToken(header string) string {
	const prefix = "bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

// allow applies one limit; false = answered 429 with Retry-After.
func (h *Handler) allow(w http.ResponseWriter, r *http.Request, action, subject string, l Limit) bool {
	if h.limiter == nil || subject == "" {
		return true
	}
	ok, retry := h.limiter.Allow(r.Context(), "customer_panel:"+action, subject, l.Max, l.Window)
	if ok {
		return true
	}
	if retry > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
	}
	response.TooManyRequests(w, r, "Too many requests. Try again later.")
	return false
}

// session applies a per-session limit.
func (h *Handler) session(w http.ResponseWriter, r *http.Request, action string, l Limit) (usecase.Principal, bool) {
	p := principalFrom(r.Context())
	return p, h.allow(w, r, action, p.SessionUUID.String(), l)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func meta(r *http.Request) usecase.Meta {
	return usecase.Meta{IP: httpkit.ClientIP(r), UserAgent: r.UserAgent()}
}

// Start serves POST /v1/customer/auth/start.
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, "start_ip", httpkit.ClientIP(r), h.limits.StartIP) {
		return
	}
	var in model.StartInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if p := usecase.NormalizePhone(in.Phone); p != "" && !h.allow(w, r, "start_phone", p, h.limits.StartPhone) {
		return
	}
	out, err := h.svc.Start(r.Context(), in, meta(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, out)
}

// Verify serves POST /v1/customer/auth/verify.
func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, "verify_ip", httpkit.ClientIP(r), h.limits.VerifyIP) {
		return
	}
	var in model.VerifyInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if p := usecase.NormalizePhone(in.Phone); p != "" && !h.allow(w, r, "verify_phone", p, h.limits.VerifyPhone) {
		return
	}
	out, err := h.svc.Verify(r.Context(), in, meta(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// LogoutInput ends one or every session.
type LogoutInput struct {
	Everywhere bool `json:"everywhere"`
}

// Logout serves POST /v1/customer/auth/logout.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var in LogoutInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := h.svc.Logout(r.Context(), principalFrom(r.Context()), in.Everywhere); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Me serves GET /v1/customer/me.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	p, ok := h.session(w, r, "read", h.limits.Read)
	if !ok {
		return
	}
	out, err := h.svc.Profile(r.Context(), p)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// listSort are the sortable fields of the customer lists.
var listSort = apiquery.SortColumns{"starts_at": "starts_at"}

func (h *Handler) list(w http.ResponseWriter, r *http.Request, withMoney bool) {
	p, ok := h.session(w, r, "read", h.limits.Read)
	if !ok {
		return
	}
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{Field: banned[0], Message: "use limit/offset instead of " + banned[0], Code: "forbidden"}})
		return
	}
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, listSort); err != nil {
		var ve *apiquery.ValidationError
		if errors.As(err, &ve) {
			response.ValidationError(w, r, []response.Detail{{Field: "sort", Message: err.Error(), Code: "invalid"}})
			return
		}
		writeError(w, r, err)
		return
	}
	f := model.ListFilter{Limit: q.Limit, Offset: q.Offset, WithMoney: withMoney}
	switch scope := strings.TrimSpace(values.Get("scope")); scope {
	case "", "all":
	case model.ScopeUpcoming, model.ScopePast:
		f.Scope = scope
	default:
		response.ValidationError(w, r, []response.Detail{{Field: "scope", Message: "scope must be upcoming, past or all", Code: "invalid"}})
		return
	}
	// Upcoming visits read soonest first; history and payments newest first.
	f.SortDesc = f.Scope != model.ScopeUpcoming
	if len(q.Sort) > 0 {
		f.SortDesc = q.Sort[0].Desc
	}
	if raw := strings.TrimSpace(values.Get("business")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "business", Message: "business must be a uuid", Code: "invalid"}})
			return
		}
		f.BusinessUUID = &id
	}
	for _, s := range apiquery.SplitCSV(values.Get("status")) {
		if !validStatus(s) {
			response.ValidationError(w, r, []response.Detail{{Field: "status", Message: "unknown status " + s, Code: "invalid"}})
			return
		}
		f.Statuses = append(f.Statuses, s)
	}
	items, total, err := h.svc.List(r.Context(), p, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func validStatus(s string) bool {
	switch s {
	case reservationsmodel.StatusRequested, reservationsmodel.StatusActive, reservationsmodel.StatusInService,
		reservationsmodel.StatusCompleted, reservationsmodel.StatusCancelled, reservationsmodel.StatusRejected,
		reservationsmodel.StatusNoShow:
		return true
	}
	return false
}

// Reservations serves GET /v1/customer/reservations.
func (h *Handler) Reservations(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, false)
}

// Payments serves GET /v1/customer/payments.
func (h *Handler) Payments(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, true)
}

// PaymentSummary serves GET /v1/customer/payments/summary.
func (h *Handler) PaymentSummary(w http.ResponseWriter, r *http.Request) {
	p, ok := h.session(w, r, "read", h.limits.Read)
	if !ok {
		return
	}
	out, err := h.svc.PaymentSummary(r.Context(), p)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Reservation was not found")
		return uuid.Nil, false
	}
	return id, true
}

// Reservation serves GET /v1/customer/reservations/{uuid}.
func (h *Handler) Reservation(w http.ResponseWriter, r *http.Request) {
	p, ok := h.session(w, r, "read", h.limits.Read)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Reservation(r.Context(), p, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Cancel serves POST /v1/customer/reservations/{uuid}/cancel.
func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	p, ok := h.session(w, r, "write", h.limits.Write)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.CancelInput
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := h.svc.Cancel(r.Context(), p, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "reservations.cancelled", out, map[string]any{"reason": usecase.CancelReason})
	response.JSON(w, r, http.StatusOK, out.Reservation)
}

// Availability serves POST /v1/customer/reservations/{uuid}/availability.
func (h *Handler) Availability(w http.ResponseWriter, r *http.Request) {
	p, ok := h.session(w, r, "search", h.limits.Search)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.AvailabilityInput
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := h.svc.Availability(r.Context(), p, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Reschedule serves POST /v1/customer/reservations/{uuid}/reschedule.
func (h *Handler) Reschedule(w http.ResponseWriter, r *http.Request) {
	p, ok := h.session(w, r, "write", h.limits.Write)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.RescheduleInput
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := h.svc.Reschedule(r.Context(), p, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "reservations.rescheduled", out, map[string]any{"starts_at": out.Reservation.StartsAt})
	response.JSON(w, r, http.StatusOK, out.Reservation)
}

// record publishes the change to the business panel's live list and
// writes the business activity entry (no actor: the customer acted).
func (h *Handler) record(r *http.Request, action string, c usecase.Changed, payload map[string]any) {
	ctx := orgctx.WithScope(r.Context(), c.Scope)
	if h.events != nil {
		h.events.Publish(ctx, reservationsmodel.Slug, resourceevents.ActionUpdated, c.Reservation.UUID.String())
	}
	if h.activity == nil {
		return
	}
	id, branch := c.Reservation.UUID, c.Reservation.Branch.UUID
	if payload == nil {
		payload = map[string]any{}
	}
	payload["number"] = c.Reservation.Number
	payload["source"] = "customer_panel"
	h.activity.RecordEntry(ctx, activity.Entry{
		Action: action, Resource: reservationsmodel.Slug, ResourceUUID: &id, BranchUUID: &branch,
		Payload: payload, Request: r,
	})
}

func conflict(w http.ResponseWriter, r *http.Request, field, code string, err error) {
	response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
		Field: field, Message: err.Error(), Code: code,
	}})
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		out := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			out = append(out, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, out)
		return
	}
	var rve *reservationsusecase.ValidationError
	if errors.As(err, &rve) {
		out := make([]response.Detail, 0, len(rve.Details))
		for _, d := range rve.Details {
			out = append(out, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, out)
		return
	}
	var ave *availabilityusecase.ValidationError
	if errors.As(err, &ave) {
		out := make([]response.Detail, 0, len(ave.Details))
		for _, d := range ave.Details {
			out = append(out, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, out)
		return
	}
	var ae *usecase.ActionError
	if errors.As(err, &ae) {
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeActionNotAllowed, ae.Error(), []response.Detail{{
			Field: ae.Action, Message: ae.Error(), Code: ae.Reason,
		}})
		return
	}
	switch {
	case errors.Is(err, usecase.ErrInvalidCode):
		response.BadRequest(w, r, CodeInvalidCode, "The code is invalid or expired")
	case errors.Is(err, usecase.ErrUnauthorized):
		response.Unauthorized(w, r, "Customer session is not valid")
	case errors.Is(err, usecase.ErrNotFound), errors.Is(err, reservationsusecase.ErrNotFound):
		response.NotFound(w, r, "Reservation was not found")
	case errors.Is(err, availabilityusecase.ErrSlotTaken):
		conflict(w, r, "starts_at", "taken", err)
	case errors.Is(err, reservationsusecase.ErrStaleVersion):
		conflict(w, r, "expected_version", "stale", err)
	case errors.Is(err, reservationsusecase.ErrInvalidTransition):
		conflict(w, r, "status", "invalid_transition", err)
	case errors.Is(err, availabilityusecase.ErrOnlineNotReady), errors.Is(err, reservationsusecase.ErrOnlineNotReady):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeActionNotAllowed, err.Error(), []response.Detail{{
			Field: "reschedule", Message: err.Error(), Code: model.ReasonOnlineUnavailable,
		}})
	case errors.Is(err, availabilityusecase.ErrNotFound):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeActionNotAllowed, err.Error(), []response.Detail{{
			Field: "reschedule", Message: err.Error(), Code: model.ReasonOnlineUnavailable,
		}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
