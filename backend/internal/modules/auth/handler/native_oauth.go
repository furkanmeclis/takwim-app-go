package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// NativeOAuth is POST /v1/auth/oauth/{provider}/native (apple | google).
func (h *Handler) NativeOAuth(w http.ResponseWriter, r *http.Request) {
	provider := strings.ToLower(strings.TrimSpace(r.PathValue("provider")))
	if provider != model.OAuthProviderApple && provider != model.OAuthProviderGoogle {
		response.NotFound(w, r, "Unknown OAuth provider")
		return
	}
	var in struct {
		IDToken           string `json:"id_token"`
		Nonce             string `json:"nonce"`
		GivenName         string `json:"given_name"`
		FamilyName        string `json:"family_name"`
		AuthorizationCode string `json:"authorization_code"`
		TOTPCode          string `json:"totp_code"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if !h.allowRates(w, r, rateCheck{"oauth_native_ip", sessionMeta(r).IP, nativeOAuthIPLimit}) {
		return
	}
	tokens, err := h.uc.NativeOAuthLogin(r.Context(), usecase.NativeOAuthInput{
		Provider: provider, IDToken: in.IDToken, Nonce: in.Nonce,
		GivenName: in.GivenName, FamilyName: in.FamilyName,
		AuthorizationCode: in.AuthorizationCode, TOTPCode: in.TOTPCode,
	}, sessionMeta(r))
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, tokens)
}

// LinkNativeIdentity is POST /v1/auth/identities/{provider}/native (apple | google):
// links a mobile SDK id_token to the signed-in user.
func (h *Handler) LinkNativeIdentity(w http.ResponseWriter, r *http.Request) {
	p, ok := authctx.PrincipalFrom(r.Context())
	if !ok {
		response.Unauthorized(w, r, "Authentication is required")
		return
	}
	provider := strings.ToLower(strings.TrimSpace(r.PathValue("provider")))
	if provider != model.OAuthProviderApple && provider != model.OAuthProviderGoogle {
		response.NotFound(w, r, "Unknown OAuth provider")
		return
	}
	var in struct {
		IDToken           string `json:"id_token"`
		Nonce             string `json:"nonce"`
		AuthorizationCode string `json:"authorization_code"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if !h.allowRates(w, r, rateCheck{"oauth_native_ip", sessionMeta(r).IP, nativeOAuthIPLimit}) {
		return
	}
	identity, err := h.uc.LinkNativeIdentity(r.Context(), p.UserID, usecase.NativeLinkInput{
		Provider: provider, IDToken: in.IDToken, Nonce: in.Nonce, AuthorizationCode: in.AuthorizationCode,
	})
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, identity)
}

// OAuthLinkRequest is POST /v1/auth/oauth/link/request.
func (h *Handler) OAuthLinkRequest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		LinkTicket string `json:"link_ticket"`
		Email      string `json:"email"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	ip := sessionMeta(r).IP
	if !h.allowRates(
		w, r,
		rateCheck{"oauth_link_request_ip", ip, codeRequestIPLimit},
		rateCheck{"oauth_link_request_email", emailSubject(in.Email), codeRequestEmailLimit},
	) {
		return
	}
	if err := h.uc.RequestOAuthLinkCode(r.Context(), in.LinkTicket, in.Email); err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "accepted"})
}

// OAuthLinkVerify is POST /v1/auth/oauth/link/verify.
func (h *Handler) OAuthLinkVerify(w http.ResponseWriter, r *http.Request) {
	var in struct {
		LinkTicket string `json:"link_ticket"`
		Email      string `json:"email"`
		Code       string `json:"code"`
		TOTPCode   string `json:"totp_code"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	ip := sessionMeta(r).IP
	if !h.allowRates(
		w, r,
		rateCheck{"oauth_link_verify_ip", ip, codeVerifyIPLimit},
		rateCheck{"oauth_link_verify_email", emailSubject(in.Email), codeVerifyEmailLimit},
	) {
		return
	}
	tokens, err := h.uc.VerifyOAuthLink(r.Context(), in.LinkTicket, in.Email, in.Code, in.TOTPCode, sessionMeta(r))
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, tokens)
}

// OAuthLinkCreate is POST /v1/auth/oauth/link/create.
func (h *Handler) OAuthLinkCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		LinkTicket string `json:"link_ticket"`
		TOTPCode   string `json:"totp_code"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if !h.allowAuthRate(w, r, func() (bool, time.Duration) {
		return h.limiter.AllowRegister(r.Context(), sessionMeta(r).IP)
	}) {
		return
	}
	tokens, err := h.uc.CreateAccountFromLinkTicket(r.Context(), in.LinkTicket, in.TOTPCode, sessionMeta(r))
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, tokens)
}
