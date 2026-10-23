package imports

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	importhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/imports/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

func RegisterRoutes(
	mux *http.ServeMux,
	h *importhandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
) {
	authn := middleware.Authenticate(tokens, loader)
	require := func(slug string) func(http.Handler) http.Handler {
		return middleware.RequirePermission(slug)
	}

	mux.Handle("GET /v1/platform/imports", middleware.Chain(
		http.HandlerFunc(h.List), authn, require(rbac.PermPlatformImportsRead),
	))
	mux.Handle("GET /v1/platform/imports/{uuid}", middleware.Chain(
		http.HandlerFunc(h.Get), authn, require(rbac.PermPlatformImportsRead),
	))
	mux.Handle("PATCH /v1/platform/imports/{uuid}/mapping", middleware.Chain(
		http.HandlerFunc(h.UpdateMapping), authn, require(rbac.PermPlatformImportsRead),
	))
	mux.Handle("POST /v1/platform/imports/{uuid}/preview", middleware.Chain(
		http.HandlerFunc(h.Preview), authn, require(rbac.PermPlatformImportsRead),
	))
	mux.Handle("POST /v1/platform/imports/{uuid}/confirm", middleware.Chain(
		http.HandlerFunc(h.Confirm), authn, require(rbac.PermPlatformImportsRead),
	))
	mux.Handle("POST /v1/platform/imports/{uuid}/rollback", middleware.Chain(
		http.HandlerFunc(h.Rollback), authn, require(rbac.PermPlatformImportsRead),
	))

	mux.Handle("POST /v1/platform/users/import", middleware.Chain(
		http.HandlerFunc(h.UploadUsers), authn, require(rbac.PermPlatformUsersImport),
	))
	mux.Handle("GET /v1/platform/users/import/sample", middleware.Chain(
		http.HandlerFunc(h.SampleUsers), authn, require(rbac.PermPlatformUsersImport),
	))
	mux.Handle("POST /v1/platform/roles/import", middleware.Chain(
		http.HandlerFunc(h.UploadRoles), authn, require(rbac.PermPlatformRolesImport),
	))
	mux.Handle("GET /v1/platform/roles/import/sample", middleware.Chain(
		http.HandlerFunc(h.SampleRoles), authn, require(rbac.PermPlatformRolesImport),
	))
}

// RegisterTenantRoutes mounts the organization-scoped import job routes. The
// use case only finds jobs of the caller in the X-Organization organization.
// Resource upload / sample routes are mounted by the resource module.
func RegisterTenantRoutes(
	mux *http.ServeMux,
	h *importhandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)

	mux.Handle("GET /v1/tenant/imports/{uuid}", middleware.Chain(
		http.HandlerFunc(h.TenantGet), authn, org,
	))
	mux.Handle("PATCH /v1/tenant/imports/{uuid}/mapping", middleware.Chain(
		http.HandlerFunc(h.UpdateMapping), authn, org,
	))
	mux.Handle("POST /v1/tenant/imports/{uuid}/preview", middleware.Chain(
		http.HandlerFunc(h.Preview), authn, org,
	))
	mux.Handle("POST /v1/tenant/imports/{uuid}/confirm", middleware.Chain(
		http.HandlerFunc(h.Confirm), authn, org,
	))
	mux.Handle("POST /v1/tenant/imports/{uuid}/rollback", middleware.Chain(
		http.HandlerFunc(h.Rollback), authn, org,
	))
}
