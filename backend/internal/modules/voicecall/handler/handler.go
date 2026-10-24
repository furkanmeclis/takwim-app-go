// Package handler serves the public webhook of a business's voice account
// (TEC-923): the provider posts the result of each confirmation call to
// the address given with the call.
package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/voicecall/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Error codes.
const (
	CodeInvalidToken = "VOICE_WEBHOOK_TOKEN_INVALID"
)

// maxBodyBytes bounds a webhook body (one call result).
const maxBodyBytes = 256 << 10

// RateLimiter is a fixed-window limiter (ratelimit.Limiter).
type RateLimiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// Handler serves the webhook route.
type Handler struct {
	svc     *usecase.Service
	limiter RateLimiter
}

// New builds the handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

// SetRateLimiter limits the route per account (nil = no limits).
func (h *Handler) SetRateLimiter(l RateLimiter) { h.limiter = l }

// Receive serves POST /v1/public/voice/netgsm/{account}/webhook?d=…&s=…:
// the result of one call (d names the attempt, s is its token). 200 once
// it is handled (also a repeated delivery); 500 makes the provider send it
// again.
func (h *Handler) Receive(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("account"))
	if err != nil {
		response.NotFound(w, r, "Webhook was not found")
		return
	}
	if h.limiter != nil {
		ok, retry := h.limiter.Allow(r.Context(), "voice:webhook", id.String(), 600, time.Minute)
		if !ok {
			if retry > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			}
			response.TooManyRequests(w, r, "Too many requests. Try again later.")
			return
		}
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid webhook body")
		return
	}
	q := r.URL.Query()
	if err := h.svc.HandleWebhook(r.Context(), id, q.Get("d"), q.Get("s"), body); err != nil {
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
		response.Error(w, r, http.StatusUnauthorized, CodeInvalidToken, "Invalid webhook token")
	case errors.Is(err, usecase.ErrMalformed):
		response.BadRequest(w, r, response.CodeValidationError, "Invalid webhook body")
	default:
		response.InternalErr(w, r, err, "Webhook could not be handled")
	}
}
