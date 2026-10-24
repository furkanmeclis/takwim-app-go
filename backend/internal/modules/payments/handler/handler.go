// Package handler is the HTTP layer of payments (TEC-912): the payment
// view and collections of a reservation in the panel, the providers'
// server notifications and the development provider's test page.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	salesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Error codes.
const (
	CodeNotPayable       = "PAYMENT_NOT_AVAILABLE"
	CodeNotReady         = "PAYMENT_NOT_READY"
	CodeProvider         = "PAYMENT_PROVIDER_ERROR"
	CodeNotCollectable   = "PAYMENT_NOT_COLLECTABLE"
	CodeInvalidSignature = "PAYMENT_SIGNATURE_INVALID"
	CodeRefundKeyReused  = "REFUND_IDEMPOTENCY_REUSED"
	CodeRefundNotOpen    = "REFUND_NOT_OPEN"
	CodeRefundReversed   = "REFUND_COLLECTION_REVERSED"
)

// maxNotificationBytes bounds a provider notification body.
const maxNotificationBytes = 64 << 10

// RateLimiter is a fixed-window limiter (ratelimit.Limiter).
type RateLimiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// Handler serves the payment routes.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
	limiter  RateLimiter
}

// New builds the handler. rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// SetRateLimiter limits the public routes per IP (nil = no limits).
func (h *Handler) SetRateLimiter(l RateLimiter) {
	h.limiter = l
}

// allow applies a per-IP limit; false = answered 429.
func (h *Handler) allow(w http.ResponseWriter, r *http.Request, action string, limit int, window time.Duration) bool {
	ip := httpkit.ClientIP(r)
	if h.limiter == nil || ip == "" {
		return true
	}
	ok, retry := h.limiter.Allow(r.Context(), "payments:"+action, ip, limit, window)
	if ok {
		return true
	}
	if retry > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
	}
	response.TooManyRequests(w, r, "Too many requests. Try again later.")
	return false
}

// caller resolves the reservations reach of the member; false = 403.
func caller(w http.ResponseWriter, r *http.Request) (availabilityusecase.Caller, bool) {
	scope := orgctx.Must(r.Context())
	switch scope.Reach(rbac.PermTenantReservationsRead, rbac.PermTenantReservationsOwn) {
	case orgctx.ReachNone:
		response.Forbidden(w, r, "Missing permission: "+rbac.PermTenantReservationsRead)
		return availabilityusecase.Caller{}, false
	case orgctx.ReachOwn:
		return availabilityusecase.Caller{OwnUserID: authctx.MustPrincipal(r.Context()).UserInternal}, true
	}
	return availabilityusecase.Caller{}, true
}

// canCollect: the member may take money and the cash module is on.
func canCollect(r *http.Request) bool {
	scope := orgctx.Must(r.Context())
	return scope.Has(rbac.PermTenantPaymentCollect) && entitlement.ForScope(scope).Allows(sector.ModuleCash)
}

// canRefund: the member may pay money back and the cash module is on.
func canRefund(r *http.Request) bool {
	scope := orgctx.Must(r.Context())
	return scope.Has(rbac.PermTenantPaymentRefund) && entitlement.ForScope(scope).Allows(sector.ModuleCash)
}

// viewOptions are what the member may do on the payment view.
func viewOptions(r *http.Request) usecase.ViewOptions {
	return usecase.ViewOptions{Collect: canCollect(r), Refund: canRefund(r)}
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Reservation was not found")
		return uuid.Nil, false
	}
	return id, true
}

// ReservationPayments serves GET /v1/tenant/reservations/{uuid}/payments.
func (h *Handler) ReservationPayments(w http.ResponseWriter, r *http.Request) {
	c, ok := caller(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.ReservationPayments(r.Context(), orgctx.Must(r.Context()).ID, id, c, viewOptions(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Collect serves POST /v1/tenant/reservations/{uuid}/collections.
func (h *Handler) Collect(w http.ResponseWriter, r *http.Request) {
	c, ok := caller(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.CollectInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	p := authctx.MustPrincipal(r.Context())
	out, err := h.svc.Collect(r.Context(), orgctx.Must(r.Context()).ID, id, c, p.UserInternal, viewOptions(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		h.activity.RecordEntry(r.Context(), activity.Entry{
			ActorID: &p.UserInternal, Action: "reservations.payment_collected", Resource: reservationsmodel.Slug, ResourceUUID: &id,
			Payload: map[string]any{
				"number": out.Reservation.Number, "amount": in.Amount.String(), "currency": out.Currency, "method": in.Method,
			},
			Request: r,
		})
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// Notify serves POST /v1/public/payments/{provider}/notify for one
// provider: the server notification of a payment. A verified notification
// is acknowledged the way the provider expects (also when it was handled
// before); anything else is an error so the provider sends it again.
func (h *Handler) Notify(provider string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.allow(w, r, "notify", 600, time.Minute) {
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxNotificationBytes))
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "Invalid notification body")
			return
		}
		n, err := h.svc.HandleNotification(r.Context(), provider, r.Header, body)
		if err != nil {
			writeError(w, r, err)
			return
		}
		n.Provider.Acknowledge(w)
	}
}

func refOf(w http.ResponseWriter, r *http.Request) (string, bool) {
	ref := r.PathValue("ref")
	if ref == "" || len(ref) > 64 {
		response.NotFound(w, r, "Payment was not found")
		return "", false
	}
	return ref, true
}

// TestPage serves GET /v1/public/payments/testpay/{ref}: what the
// development provider's page shows (404 in production).
func (h *Handler) TestPage(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, "test", 120, time.Minute) {
		return
	}
	ref, ok := refOf(w, r)
	if !ok {
		return
	}
	out, err := h.svc.TestPage(r.Context(), ref)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// TestComplete serves POST /v1/public/payments/testpay/{ref}/complete:
// the test page's pay / fail buttons ({"outcome": "succeeded" | "failed"}).
func (h *Handler) TestComplete(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, "test", 120, time.Minute) {
		return
	}
	ref, ok := refOf(w, r)
	if !ok {
		return
	}
	var in struct {
		Outcome string `json:"outcome"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	n, err := h.svc.TestComplete(r.Context(), ref, in.Outcome)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, n.Result)
}

// WriteError maps payment errors to responses (the public booking handler
// uses it for the payment start).
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	writeError(w, r, err)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	if details := validationDetails(err); details != nil {
		response.ValidationError(w, r, details)
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Not found")
	case errors.Is(err, usecase.ErrNotPayable):
		response.Error(w, r, http.StatusConflict, CodeNotPayable, err.Error())
	case errors.Is(err, usecase.ErrNotReady):
		response.Error(w, r, http.StatusConflict, CodeNotReady, err.Error())
	case errors.Is(err, usecase.ErrNotCollectable):
		response.Error(w, r, http.StatusConflict, CodeNotCollectable, err.Error())
	case errors.Is(err, cashusecase.ErrDayClosed), errors.Is(err, cashusecase.ErrAccountArchived):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "cash_account_uuid", Message: err.Error(), Code: "closed",
		}})
	case errors.Is(err, cashusecase.ErrInsufficientFunds):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "cash_account_uuid", Message: err.Error(), Code: "insufficient_funds",
		}})
	case errors.Is(err, usecase.ErrIdempotencyReused):
		response.Error(w, r, http.StatusConflict, CodeRefundKeyReused, err.Error())
	case errors.Is(err, usecase.ErrRefundNotOpen):
		response.Error(w, r, http.StatusConflict, CodeRefundNotOpen, err.Error())
	case errors.Is(err, salesusecase.ErrAlreadyReversed):
		response.Error(w, r, http.StatusConflict, CodeRefundReversed, err.Error())
	case errors.Is(err, salesusecase.ErrRefundTooLarge):
		response.ValidationError(w, r, []response.Detail{{Field: "amount", Code: "exceeds_refundable", Message: err.Error()}})
	case errors.Is(err, salesusecase.ErrCurrencyMismatch):
		response.ValidationError(w, r, []response.Detail{{Field: "cash_account_uuid", Code: "currency", Message: err.Error()}})
	case errors.Is(err, usecase.ErrProvider):
		response.Error(w, r, http.StatusBadGateway, CodeProvider, err.Error())
	case errors.Is(err, usecase.ErrUnknownProvider):
		response.NotFound(w, r, "Unknown payment provider")
	case errors.Is(err, usecase.ErrMalformed):
		response.BadRequest(w, r, response.CodeValidationError, "Malformed notification")
	case errors.Is(err, usecase.ErrSignature):
		response.Error(w, r, http.StatusUnauthorized, CodeInvalidSignature, "Invalid notification signature")
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}

func validationDetails(err error) []response.Detail {
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		out := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			out = append(out, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		return out
	}
	var qve *apiquery.ValidationError
	if errors.As(err, &qve) {
		out := make([]response.Detail, 0, len(qve.Details))
		for _, d := range qve.Details {
			out = append(out, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		return out
	}
	var sve *salesusecase.ValidationError
	if errors.As(err, &sve) {
		out := make([]response.Detail, 0, len(sve.Details))
		for _, d := range sve.Details {
			out = append(out, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		return out
	}
	return nil
}
