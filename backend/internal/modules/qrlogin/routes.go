package qrlogin

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the QR sign-in endpoints.
func RegisterRoutes(mux *http.ServeMux, h *Handler, tokens *jwt.Manager, loader middleware.IdentityLoader) {
	authn := middleware.Authenticate(tokens, loader)
	requireSession := middleware.RequirePermission(rbac.PermAuthSession)

	// Web tab (anonymous; the browser secret proves tab ownership).
	mux.HandleFunc("POST /v1/auth/qr/sessions", h.Create)
	mux.HandleFunc("POST /v1/auth/qr/sessions/{id}/state", h.State)
	mux.HandleFunc("POST /v1/auth/qr/exchange", h.Exchange)

	// Signed-in phone.
	mux.Handle("GET /v1/auth/qr/sessions/{id}", middleware.Chain(http.HandlerFunc(h.Get), authn, requireSession))
	mux.Handle("POST /v1/auth/qr/sessions/{id}/approve", middleware.Chain(http.HandlerFunc(h.Approve), authn, requireSession))
	mux.Handle("POST /v1/auth/qr/sessions/{id}/reject", middleware.Chain(http.HandlerFunc(h.Reject), authn, requireSession))
}
