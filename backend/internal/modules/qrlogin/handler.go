package qrlogin

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	authusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ratelimit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

const (
	rateWindow       = 15 * time.Minute
	createIPLimit    = 60 // a tab refreshes its QR every 2 min
	exchangeIPLimit  = 30
	stateIPLimit     = 120
	approveUserLimit = 30
)

// Handler serves /v1/auth/qr/*.
type Handler struct {
	svc     *Service
	geo     *Locator
	limiter *ratelimit.Limiter
}

// NewHandler builds the HTTP handler. geo and limiter may be nil.
func NewHandler(svc *Service, geo *Locator, limiter *ratelimit.Limiter) *Handler {
	return &Handler{svc: svc, geo: geo, limiter: limiter}
}

// Create is POST /v1/auth/qr/sessions (public).
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !h.allow(w, r, "qr_create_ip", ip, createIPLimit) {
		return
	}
	out, err := h.svc.Create(r.Context(), r.UserAgent(), ip, h.geo.Locate(r, ip))
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, http.StatusCreated, out)
}

// State is POST /v1/auth/qr/sessions/{id}/state (public, needs browser secret).
func (h *Handler) State(w http.ResponseWriter, r *http.Request) {
	var in struct {
		BrowserSecret string `json:"browser_secret"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !h.allow(w, r, "qr_state_ip", clientIP(r), stateIPLimit) {
		return
	}
	out, err := h.svc.State(r.Context(), r.PathValue("id"), in.BrowserSecret)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, http.StatusOK, out)
}

// Exchange is POST /v1/auth/qr/exchange (public; called by the web server).
func (h *Handler) Exchange(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SessionID     string `json:"session_id"`
		BrowserSecret string `json:"browser_secret"`
		ExchangeToken string `json:"exchange_token"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !h.allow(w, r, "qr_exchange_ip", clientIP(r), exchangeIPLimit) {
		return
	}
	out, err := h.svc.Exchange(r.Context(), in.SessionID, in.BrowserSecret, in.ExchangeToken)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, http.StatusOK, out)
}

// Get is GET /v1/auth/qr/sessions/{id} (Bearer, the phone).
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	viewer := approverFrom(r)
	if !h.allow(w, r, "qr_view_user", viewer.UserUUID.String(), approveUserLimit) {
		return
	}
	out, err := h.svc.Get(r.Context(), r.PathValue("id"), viewer)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Approve is POST /v1/auth/qr/sessions/{id}/approve (Bearer).
func (h *Handler) Approve(w http.ResponseWriter, r *http.Request) {
	viewer := approverFrom(r)
	if !h.allow(w, r, "qr_resolve_user", viewer.UserUUID.String(), approveUserLimit) {
		return
	}
	if err := h.svc.Approve(r.Context(), r.PathValue("id"), viewer); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": StatusApproved})
}

// Reject is POST /v1/auth/qr/sessions/{id}/reject (Bearer).
func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) {
	viewer := approverFrom(r)
	if !h.allow(w, r, "qr_resolve_user", viewer.UserUUID.String(), approveUserLimit) {
		return
	}
	if err := h.svc.Reject(r.Context(), r.PathValue("id"), viewer); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": StatusRejected})
}

func approverFrom(r *http.Request) Approver {
	p := authctx.MustPrincipal(r.Context())
	return Approver{
		UserUUID:      p.UserID,
		Impersonating: p.ImpersonatorUserID != nil,
	}
}

func (h *Handler) allow(w http.ResponseWriter, r *http.Request, action, subject string, limit int) bool {
	if h.limiter == nil || strings.TrimSpace(subject) == "" {
		return true
	}
	ok, retry := h.limiter.Allow(r.Context(), action, subject, limit, rateWindow)
	if ok {
		return true
	}
	if retry > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
	}
	response.TooManyRequests(w, r, "")
	return false
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body")
		return false
	}
	return true
}

// clientIP mirrors the auth handlers: the BFF / edge sets X-Forwarded-For.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := strings.TrimSpace(strings.Split(xff, ",")[0]); ip != "" {
			return ip
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, ErrNotFound):
		response.Error(w, r, http.StatusNotFound, response.CodeQRSessionNotFound, "This QR code has expired or was already used")
	case errors.Is(err, ErrResolved):
		response.Conflict(w, r, response.CodeQRSessionResolved, "This sign-in request was already answered")
	case errors.Is(err, ErrClaimed):
		response.Conflict(w, r, response.CodeQRSessionClaimed, "This QR code was scanned by another account")
	case errors.Is(err, ErrInvalidSecret):
		response.BadRequest(w, r, response.CodeQRLoginInvalid, "QR sign-in could not be verified")
	case errors.Is(err, ErrImpersonating):
		response.Forbidden(w, r, "Stop impersonating before approving a sign-in")
	case errors.Is(err, ErrRealtimeDisabled):
		response.ServiceUnavailable(w, r, response.CodeRealtimeDisabled, "Realtime is disabled")
	case errors.Is(err, authusecase.ErrAccountDeactivated):
		response.Error(w, r, http.StatusForbidden, response.CodeAccountDeactivated, "This account has been deleted")
	case errors.Is(err, authusecase.ErrUserDisabled):
		response.Forbidden(w, r, "User is disabled")
	case errors.Is(err, authusecase.ErrInvalidCredentials):
		response.Unauthorized(w, r, "Account is no longer available")
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
