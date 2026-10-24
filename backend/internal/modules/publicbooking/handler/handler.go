// Package handler is the HTTP layer of the public booking API (TEC-906).
// The routes are unauthenticated: the business comes from the {slug} path
// segment, never from a header or the body, and every answer carries only
// customer-facing fields. Rate limits are per client IP (and per phone for
// bookings), like the other public routes; they fail open without Redis.
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
	paymentshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicbooking/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicbooking/usecase"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// ReservationTokenHeader carries the access token of a reservation on the
// status route.
const ReservationTokenHeader = "X-Reservation-Token"

// CodeBookingUnavailable answers a booking call when the business does not
// take public bookings on the channel (detail channel / reason).
const CodeBookingUnavailable = "BOOKING_UNAVAILABLE"

// Limit is one fixed window.
type Limit struct {
	Max    int
	Window time.Duration
}

// Limits are the rate limits of the public routes.
type Limits struct {
	Read         Limit // profile, catalog, checkout and status reads (per IP)
	Availability Limit // slot searches (per IP)
	Checkout     Limit // checkout holds and releases (per IP)
	Booking      Limit // bookings (per IP)
	BookingPhone Limit // bookings (per business and phone)
}

// DefaultLimits are the production limits.
var DefaultLimits = Limits{
	Read:         Limit{Max: 300, Window: time.Minute},
	Availability: Limit{Max: 60, Window: time.Minute},
	Checkout:     Limit{Max: 20, Window: 10 * time.Minute},
	Booking:      Limit{Max: 10, Window: 10 * time.Minute},
	BookingPhone: Limit{Max: 5, Window: time.Hour},
}

// RateLimiter is a fixed-window limiter (ratelimit.Limiter).
type RateLimiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// Handler serves /v1/public/businesses/{slug}.
type Handler struct {
	svc      *usecase.Service
	storage  storage.Driver
	activity *activity.Recorder
	events   resourceevents.Publisher
	limiter  RateLimiter
	limits   Limits
}

// New builds the handler. store, rec and the limiter may be nil.
func New(svc *usecase.Service, store storage.Driver, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, storage: store, activity: rec, limits: DefaultLimits}
}

// SetRateLimiter plugs in the limiter (nil = no limits).
func (h *Handler) SetRateLimiter(l RateLimiter) {
	h.limiter = l
}

// SetLimits replaces the limits (tests).
func (h *Handler) SetLimits(l Limits) {
	h.limits = l
}

// SetResourceEvents publishes live list changes of the panel (optional).
func (h *Handler) SetResourceEvents(pub resourceevents.Publisher) {
	h.events = pub
}

// allow applies one limit; false = answered 429 with Retry-After.
func (h *Handler) allow(w http.ResponseWriter, r *http.Request, action, subject string, l Limit) bool {
	if h.limiter == nil || subject == "" {
		return true
	}
	ok, retry := h.limiter.Allow(r.Context(), "public_booking:"+action, subject, l.Max, l.Window)
	if ok {
		return true
	}
	if retry > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
	}
	response.TooManyRequests(w, r, "Too many requests. Try again later.")
	return false
}

// business applies the IP limit of action and resolves {slug}.
func (h *Handler) business(w http.ResponseWriter, r *http.Request, action string, l Limit) (usecase.Org, bool) {
	if !h.allow(w, r, action, httpkit.ClientIP(r), l) {
		return usecase.Org{}, false
	}
	o, err := h.svc.Resolve(r.Context(), r.PathValue("slug"))
	if err != nil {
		writeError(w, r, err)
		return usecase.Org{}, false
	}
	return o, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

// optionalUUID reads an optional uuid query parameter; false = answered 400.
func optionalUUID(w http.ResponseWriter, r *http.Request, key string) (*uuid.UUID, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: key, Message: key + " must be a uuid", Code: "invalid"}})
		return nil, false
	}
	return &id, true
}

// Business serves GET /v1/public/businesses/{slug}.
func (h *Handler) Business(w http.ResponseWriter, r *http.Request) {
	o, ok := h.business(w, r, "read", h.limits.Read)
	if !ok {
		return
	}
	out, err := h.svc.Business(r.Context(), o)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Logo serves GET /v1/public/businesses/{slug}/logo.
func (h *Handler) Logo(w http.ResponseWriter, r *http.Request) {
	o, ok := h.business(w, r, "read", h.limits.Read)
	if !ok {
		return
	}
	key, err := h.svc.Logo(o)
	if err != nil || h.storage == nil {
		response.NotFound(w, r, "Logo was not found")
		return
	}
	rc, _, err := h.storage.Download(r.Context(), key)
	if err != nil {
		response.NotFound(w, r, "Logo was not found")
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", storage.MIMEFromLogoKey(key))
	// The URL carries a version derived from the object key.
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

// Branches serves GET /v1/public/businesses/{slug}/branches.
func (h *Handler) Branches(w http.ResponseWriter, r *http.Request) {
	o, ok := h.business(w, r, "read", h.limits.Read)
	if !ok {
		return
	}
	out, err := h.svc.Branches(r.Context(), o)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Services serves GET /v1/public/businesses/{slug}/services.
func (h *Handler) Services(w http.ResponseWriter, r *http.Request) {
	o, ok := h.business(w, r, "read", h.limits.Read)
	if !ok {
		return
	}
	branch, ok := optionalUUID(w, r, "branch_uuid")
	if !ok {
		return
	}
	out, err := h.svc.Services(r.Context(), o, branch)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Products serves GET /v1/public/businesses/{slug}/products?branch_uuid=
// (TEC-919): the products the customer may add in the branch.
func (h *Handler) Products(w http.ResponseWriter, r *http.Request) {
	o, ok := h.business(w, r, "read", h.limits.Read)
	if !ok {
		return
	}
	branch, ok := optionalUUID(w, r, "branch_uuid")
	if !ok {
		return
	}
	out, err := h.svc.Products(r.Context(), o, branch)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Staff serves GET /v1/public/businesses/{slug}/staff.
func (h *Handler) Staff(w http.ResponseWriter, r *http.Request) {
	o, ok := h.business(w, r, "read", h.limits.Read)
	if !ok {
		return
	}
	branch, ok := optionalUUID(w, r, "branch_uuid")
	if !ok {
		return
	}
	service, ok := optionalUUID(w, r, "service_uuid")
	if !ok {
		return
	}
	out, err := h.svc.Staff(r.Context(), o, branch, service)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Availability serves POST /v1/public/businesses/{slug}/availability.
func (h *Handler) Availability(w http.ResponseWriter, r *http.Request) {
	o, ok := h.business(w, r, "availability", h.limits.Availability)
	if !ok {
		return
	}
	var in model.AvailabilityInput
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := h.svc.Availability(r.Context(), o, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// CreateCheckout serves POST /v1/public/businesses/{slug}/checkouts.
func (h *Handler) CreateCheckout(w http.ResponseWriter, r *http.Request) {
	o, ok := h.business(w, r, "checkout", h.limits.Checkout)
	if !ok {
		return
	}
	var in model.CheckoutInput
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := h.svc.CreateCheckout(r.Context(), o, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

func pathToken(w http.ResponseWriter, r *http.Request, key, what string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(key))
	if err != nil {
		response.NotFound(w, r, what+" was not found")
		return uuid.Nil, false
	}
	return id, true
}

// GetCheckout serves GET /v1/public/businesses/{slug}/checkouts/{token}.
func (h *Handler) GetCheckout(w http.ResponseWriter, r *http.Request) {
	o, ok := h.business(w, r, "read", h.limits.Read)
	if !ok {
		return
	}
	token, ok := pathToken(w, r, "token", "Checkout")
	if !ok {
		return
	}
	out, err := h.svc.Checkout(r.Context(), o, token)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ReleaseCheckout serves DELETE /v1/public/businesses/{slug}/checkouts/{token}.
func (h *Handler) ReleaseCheckout(w http.ResponseWriter, r *http.Request) {
	o, ok := h.business(w, r, "checkout", h.limits.Checkout)
	if !ok {
		return
	}
	token, ok := pathToken(w, r, "token", "Checkout")
	if !ok {
		return
	}
	out, err := h.svc.ReleaseCheckout(r.Context(), o, token)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// CreateReservation serves POST /v1/public/businesses/{slug}/reservations.
// A new booking answers 201; a retry of a booked checkout answers 200 with
// the same reservation and Idempotent-Replayed: true.
func (h *Handler) CreateReservation(w http.ResponseWriter, r *http.Request) {
	o, ok := h.business(w, r, "booking", h.limits.Booking)
	if !ok {
		return
	}
	var in model.BookingInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if p := usecase.NormalizePhone(in.Customer.Phone); p != "" {
		if !h.allow(w, r, "booking_phone", o.UUID.String()+"|"+p, h.limits.BookingPhone) {
			return
		}
	}
	out, replayed, err := h.svc.Book(r.Context(), o, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if replayed {
		w.Header().Set("Idempotent-Replayed", "true")
		response.JSON(w, r, http.StatusOK, out)
		return
	}
	h.record(r, o, out)
	response.JSON(w, r, http.StatusCreated, out)
}

// CustomerSession authenticates a customer panel session (the customer
// panel handler) and finds the person's record in a business.
type CustomerSession interface {
	Authenticate(next http.HandlerFunc) http.HandlerFunc
	// CustomerOf returns the business's customer record of the request's
	// session (uuid.Nil when the business does not know the person).
	CustomerOf(r *http.Request, orgID int64) (uuid.UUID, error)
}

// CreateCustomerReservation serves POST
// /v1/customer/businesses/{slug}/reservations (TEC-984): the business
// page books for the signed-in customer panel session, which may use the
// person's package sessions. The route sits under /v1/customer so the
// customer cookie reaches it; the contact form is ignored.
func (h *Handler) CreateCustomerReservation(session CustomerSession) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		o, ok := h.business(w, r, "booking", h.limits.Booking)
		if !ok {
			return
		}
		var in model.BookingInput
		if !decodeJSON(w, r, &in) {
			return
		}
		out, replayed, err := h.svc.BookAs(r.Context(), o, in, func(_ context.Context, orgID int64) (uuid.UUID, error) {
			return session.CustomerOf(r, orgID)
		})
		if err != nil {
			writeError(w, r, err)
			return
		}
		if replayed {
			w.Header().Set("Idempotent-Replayed", "true")
			response.JSON(w, r, http.StatusOK, out)
			return
		}
		h.record(r, o, out)
		h.packagesChanged(r, o, out)
		response.JSON(w, r, http.StatusCreated, out)
	}
}

// packagesChanged refreshes the business's package lists after a booking
// used package sessions.
func (h *Handler) packagesChanged(r *http.Request, o usecase.Org, res model.Reservation) {
	if h.events == nil {
		return
	}
	for _, l := range res.Lines {
		if l.PackageName != nil {
			ctx := orgctx.WithScope(r.Context(), o.Scope())
			h.events.Publish(ctx, customerPackagesResource, resourceevents.ActionUpdated)
			return
		}
	}
}

// customerPackagesResource is the live channel of the sold packages.
const customerPackagesResource = "tenant.customer_packages"

// GetReservation serves GET /v1/public/businesses/{slug}/reservations/{uuid}
// with the X-Reservation-Token header.
func (h *Handler) GetReservation(w http.ResponseWriter, r *http.Request) {
	o, ok := h.business(w, r, "read", h.limits.Read)
	if !ok {
		return
	}
	id, ok := pathToken(w, r, "uuid", "Reservation")
	if !ok {
		return
	}
	out, err := h.svc.Reservation(r.Context(), o, id, r.Header.Get(ReservationTokenHeader))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// StartPayment serves POST /v1/public/businesses/{slug}/reservations/{uuid}/payment
// (TEC-912): the holder of the access token gets the provider's payment
// page of a booking waiting for its online payment ({redirect_url}). The
// page's result is recorded only from the provider's server notification.
func (h *Handler) StartPayment(w http.ResponseWriter, r *http.Request) {
	o, ok := h.business(w, r, "checkout", h.limits.Checkout)
	if !ok {
		return
	}
	id, ok := pathToken(w, r, "uuid", "Reservation")
	if !ok {
		return
	}
	var in struct {
		Locale string `json:"locale"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := h.svc.StartPayment(r.Context(), o, id, r.Header.Get(ReservationTokenHeader), httpkit.ClientIP(r), in.Locale)
	if errors.Is(err, usecase.ErrReservationNotFound) || errors.Is(err, usecase.ErrBusinessNotFound) {
		writeError(w, r, err)
		return
	}
	if err != nil {
		paymentshandler.WriteError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// record publishes the new booking to the panel's live list and writes the
// business activity entry (no actor: the customer booked it).
func (h *Handler) record(r *http.Request, o usecase.Org, res model.Reservation) {
	ctx := orgctx.WithScope(r.Context(), o.Scope())
	if h.events != nil {
		h.events.Publish(ctx, reservationsmodel.Slug, resourceevents.ActionCreated, res.UUID.String())
	}
	if h.activity == nil {
		return
	}
	id, branch := res.UUID, res.Branch.UUID
	h.activity.RecordEntry(ctx, activity.Entry{
		Action: "reservations.created", Resource: reservationsmodel.Slug, ResourceUUID: &id, BranchUUID: &branch,
		Payload: map[string]any{"number": res.Number, "status": res.Status, "source": "public"}, Request: r,
	})
}

func details(in []usecase.Detail) []response.Detail {
	out := make([]response.Detail, 0, len(in))
	for _, d := range in {
		out = append(out, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
	}
	return out
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
		response.ValidationError(w, r, details(ve.Details))
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
	var ue *usecase.UnavailableError
	if errors.As(err, &ue) {
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeBookingUnavailable, "Online booking is not available", []response.Detail{{
			Field: "channel", Message: ue.Error(), Code: ue.Reason,
		}})
		return
	}
	switch {
	case errors.Is(err, usecase.ErrBusinessNotFound):
		response.NotFound(w, r, "Business was not found")
	case errors.Is(err, usecase.ErrCheckoutNotFound):
		response.NotFound(w, r, "Checkout was not found")
	case errors.Is(err, usecase.ErrReservationNotFound):
		response.NotFound(w, r, "Reservation was not found")
	case errors.Is(err, availabilityusecase.ErrNotFound):
		response.ValidationError(w, r, []response.Detail{{Field: "branch_uuid", Message: "branch not found", Code: "not_found"}})
	case errors.Is(err, availabilityusecase.ErrSlotTaken):
		conflict(w, r, "starts_at", "taken", err)
	case errors.Is(err, availabilityusecase.ErrOnlineNotReady), errors.Is(err, reservationsusecase.ErrOnlineNotReady):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeBookingUnavailable, "Online booking is not available", []response.Detail{{
			Field: "channel", Message: err.Error(), Code: model.UnavailableNotReady,
		}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
