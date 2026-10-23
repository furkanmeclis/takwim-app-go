package exports

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	exporthandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/exports/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/stepup"
)

// RegisterRoutes mounts export routes.
func RegisterRoutes(
	mux *http.ServeMux,
	h *exporthandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	stepUp *stepup.Service,
) {
	authn := middleware.Authenticate(tokens, loader)
	require := func(slug string) func(http.Handler) http.Handler {
		return middleware.RequirePermission(slug)
	}
	var requireStepUp func(http.Handler) http.Handler
	if stepUp != nil {
		requireStepUp = middleware.RequireStepUp(stepUp)
	} else {
		requireStepUp = func(next http.Handler) http.Handler { return next }
	}

	mux.Handle("GET /v1/platform/exports", middleware.Chain(
		http.HandlerFunc(h.List), authn, require(rbac.PermPlatformExportsRead),
	))
	mux.Handle("GET /v1/platform/exports/{uuid}", middleware.Chain(
		http.HandlerFunc(h.Get), authn, require(rbac.PermPlatformExportsRead),
	))
	mux.Handle("GET /v1/platform/exports/{uuid}/download", middleware.Chain(
		http.HandlerFunc(h.Download), authn, require(rbac.PermPlatformExportsRead),
	))

	mux.Handle("POST /v1/platform/users/export", middleware.Chain(
		http.HandlerFunc(h.RequestUsersExport), authn, require(rbac.PermPlatformUsersExport), requireStepUp,
	))
	mux.Handle("POST /v1/platform/roles/export", middleware.Chain(
		http.HandlerFunc(h.RequestRolesExport), authn, require(rbac.PermPlatformRolesExport), requireStepUp,
	))
	mux.Handle("POST /v1/platform/notifications/export", middleware.Chain(
		http.HandlerFunc(h.RequestNotificationsExport), authn, require(rbac.PermPlatformNotificationsExport), requireStepUp,
	))
	mux.Handle("POST /v1/platform/activity/export", middleware.Chain(
		http.HandlerFunc(h.RequestActivityExport), authn, require(rbac.PermPlatformActivityRead), requireStepUp,
	))
}

// RegisterTenantRoutes mounts the organization-scoped export job routes.
// Resource export routes (POST /v1/tenant/<resource>/export) are mounted by
// the resource module with its tenant permission. A nil resolver mounts nothing.
func RegisterTenantRoutes(
	mux *http.ServeMux,
	h *exporthandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)

	mux.Handle("GET /v1/tenant/exports/{uuid}", middleware.Chain(
		http.HandlerFunc(h.TenantGet), authn, org,
	))
	mux.Handle("GET /v1/tenant/exports/{uuid}/download", middleware.Chain(
		http.HandlerFunc(h.TenantDownload), authn, org,
	))
}
