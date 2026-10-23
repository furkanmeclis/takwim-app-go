package settings

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	settingshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/settings/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

func RegisterRoutes(
	mux *http.ServeMux,
	h *settingshandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
) {
	authn := middleware.Authenticate(tokens, loader)
	require := func(slug string) func(http.Handler) http.Handler {
		return middleware.RequirePermission(slug)
	}

	mux.Handle("GET /v1/platform/settings", middleware.Chain(
		http.HandlerFunc(h.Get), authn, require(rbac.PermPlatformSettingsRead),
	))
	mux.Handle("PATCH /v1/platform/settings", middleware.Chain(
		http.HandlerFunc(h.Patch), authn, require(rbac.PermPlatformSettingsWrite),
	))
	mux.Handle("PUT /v1/platform/settings/logo", middleware.Chain(
		http.HandlerFunc(h.UploadLogo), authn, require(rbac.PermPlatformSettingsWrite),
	))
	mux.Handle("DELETE /v1/platform/settings/logo", middleware.Chain(
		http.HandlerFunc(h.DeleteLogo), authn, require(rbac.PermPlatformSettingsWrite),
	))
	mux.Handle("GET /v1/platform/settings/logo", middleware.Chain(
		http.HandlerFunc(h.StreamLogo), authn,
	))

	mux.Handle("GET /v1/platform/system/info", middleware.Chain(
		http.HandlerFunc(h.GetSystemInfo), authn, require(rbac.PermPlatformSettingsRead),
	))
	mux.Handle("POST /v1/platform/system/ping", middleware.Chain(
		http.HandlerFunc(h.PingSystem), authn, require(rbac.PermPlatformSettingsRead),
	))
}
