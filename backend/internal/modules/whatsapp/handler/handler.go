// Package handler serves the public webhook of a business's WhatsApp
// account (TEC-922): the Cloud API's subscription handshake and its event
// posts (delivery statuses, button replies).
package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Error codes.
const (
	CodeInvalidSignature = "WHATSAPP_SIGNATURE_INVALID"
)

// maxBodyBytes bounds a webhook body (Meta batches up to a few hundred
// events per call).
const maxBodyBytes = 1 << 20

// RateLimiter is a fixed-window limiter (ratelimit.Limiter).
type RateLimiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// Handler serves the webhook routes.
type Handler struct {
	svc     *usecase.Service
	limiter RateLimiter
}

// New builds the handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

// SetRateLimiter limits the routes per account (nil = no limits).
func (h *Handler) SetRateLimiter(l RateLimiter) { h.limiter = l }

func (h *Handler) allow(w http.ResponseWriter, r *http.Request, subject string) bool {
	if h.limiter == nil {
		return true
	}
	ok, retry := h.limiter.Allow(r.Context(), "whatsapp:webhook", subject, 1200, time.Minute)
	if ok {
		return true
	}
	if retry > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
	}
	response.TooManyRequests(w, r, "Too many requests. Try again later.")
	return false
}

func accountOf(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("account"))
	if err != nil {
		response.NotFound(w, r, "Webhook was not found")
		return uuid.Nil, false
	}
	return id, true
}

// Verify serves GET /v1/public/whatsapp/meta/{account}/webhook: the
// subscription handshake (hub.mode, hub.verify_token, hub.challenge). The
// challenge is echoed as plain text.
func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) {
	id, ok := accountOf(w, r)
	if !ok || !h.allow(w, r, id.String()+"|"+httpkit.ClientIP(r)) {
		return
	}
	q := r.URL.Query()
	challenge, err := h.svc.Challenge(r.Context(), id, q.Get("hub.mode"), q.Get("hub.verify_token"), q.Get("hub.challenge"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, challenge)
}

// Receive serves POST /v1/public/whatsapp/meta/{account}/webhook: signed
// delivery statuses and replies. 200 once everything is handled (also a
// repeated delivery); 500 makes the provider send it again.
func (h *Handler) Receive(w http.ResponseWriter, r *http.Request) {
	id, ok := accountOf(w, r)
	if !ok || !h.allow(w, r, id.String()) {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid webhook body")
		return
	}
	if err := h.svc.HandleWebhook(r.Context(), id, r.Header, body); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "ok"})
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, usecase.ErrUnknownAccount):
		response.NotFound(w, r, "Webhook was not found")
	case errors.Is(err, usecase.ErrSignature):
		response.Error(w, r, http.StatusUnauthorized, CodeInvalidSignature, "Invalid webhook signature")
	case errors.Is(err, usecase.ErrMalformed):
		response.BadRequest(w, r, response.CodeValidationError, "Invalid webhook body")
	default:
		response.InternalErr(w, r, err, "Webhook could not be handled")
	}
}
