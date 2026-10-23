package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

const (
	codeRequestIPLimit     = 10
	codeRequestEmailLimit  = 5
	codeVerifyIPLimit      = 30
	codeVerifyEmailLimit   = 10
	nativeOAuthIPLimit     = 30
	deactivateRequestLimit = 5
	authRateWindow         = 15 * time.Minute
)

type rateCheck struct {
	action  string
	subject string
	limit   int
}

// allowRates applies every check (per-IP, per-email, ...) and writes 429 on the first denial.
func (h *Handler) allowRates(w http.ResponseWriter, r *http.Request, checks ...rateCheck) bool {
	if h.limiter == nil {
		return true
	}
	for _, c := range checks {
		if strings.TrimSpace(c.subject) == "" {
			continue
		}
		ok, retry := h.limiter.Allow(r.Context(), c.action, c.subject, c.limit, authRateWindow)
		if h.writeRateLimited(w, r, ok, retry) {
			return false
		}
	}
	return true
}

func emailSubject(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// RequestEmailCode is POST /v1/auth/email-code/request.
func (h *Handler) RequestEmailCode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	ip := sessionMeta(r).IP
	if !h.allowRates(
		w, r,
		rateCheck{"email_code_request_ip", ip, codeRequestIPLimit},
		rateCheck{"email_code_request_email", emailSubject(in.Email), codeRequestEmailLimit},
	) {
		return
	}
	if err := h.uc.RequestLoginCode(r.Context(), in.Email); err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "accepted"})
}

// VerifyEmailCode is POST /v1/auth/email-code/verify.
func (h *Handler) VerifyEmailCode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Code     string `json:"code"`
		TOTPCode string `json:"totp_code"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	ip := sessionMeta(r).IP
	if !h.allowRates(
		w, r,
		rateCheck{"email_code_verify_ip", ip, codeVerifyIPLimit},
		rateCheck{"email_code_verify_email", emailSubject(in.Email), codeVerifyEmailLimit},
	) {
		return
	}
	tokens, err := h.uc.VerifyLoginCode(r.Context(), in.Email, in.Code, in.TOTPCode, sessionMeta(r))
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, tokens)
}

// RequestAccountDeactivationCode is POST /v1/auth/account/deactivate/request.
func (h *Handler) RequestAccountDeactivationCode(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	if p.ImpersonatorUserID != nil {
		response.Forbidden(w, r, "Stop impersonating before deleting an account")
		return
	}
	if !h.allowRates(w, r, rateCheck{"account_deactivate_request", p.UserID.String(), deactivateRequestLimit}) {
		return
	}
	if err := h.uc.RequestAccountDeactivationCode(r.Context(), p.UserID); err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "accepted"})
}

// DeactivateAccount is POST /v1/auth/account/deactivate.
func (h *Handler) DeactivateAccount(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	if p.ImpersonatorUserID != nil {
		response.Forbidden(w, r, "Stop impersonating before deleting an account")
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if !h.allowRates(w, r, rateCheck{"account_deactivate", p.UserID.String(), codeVerifyEmailLimit}) {
		return
	}
	stepUpOK := false
	if strings.TrimSpace(in.Code) == "" && h.stepUp != nil {
		ok, err := h.stepUp.HasValidGrant(r.Context(), p.UserID)
		if err != nil {
			response.InternalErr(w, r, err, "Failed to verify step-up grant")
			return
		}
		stepUpOK = ok
	}
	if err := h.uc.DeactivateAccount(r.Context(), p.UserID, in.Code, stepUpOK); err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	if h.stepUp != nil {
		_ = h.stepUp.RevokeGrant(r.Context(), p.UserID)
	}
	if h.activity != nil {
		id := p.UserInternal
		method := "step_up"
		if strings.TrimSpace(in.Code) != "" {
			method = "email_code"
		}
		h.activity.Record(r.Context(), &id, "auth.account_deactivated", "auth.account", nil, map[string]any{
			"method": method,
		}, r)
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deactivated"})
}
