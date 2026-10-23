package activity

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	activityhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/activity/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

func RegisterRoutes(
	mux *http.ServeMux,
	h *activityhandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
) {
	authn := middleware.Authenticate(tokens, loader)
	require := func(slug string) func(http.Handler) http.Handler {
		return middleware.RequirePermission(slug)
	}

	mux.Handle("GET /v1/platform/activity/meta", middleware.Chain(
		http.HandlerFunc(h.Meta), authn, require(rbac.PermPlatformActivityRead),
	))
	mux.Handle("GET /v1/platform/activity", middleware.Chain(
		http.HandlerFunc(h.List), authn, require(rbac.PermPlatformActivityRead),
	))
}

// RegisterTenantRoutes mounts the organization's activity history
// (TEC-893). A nil resolver (tenancy off) registers nothing.
func RegisterTenantRoutes(
	mux *http.ServeMux,
	h *activityhandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantActivityRead)
	mux.Handle("GET /v1/tenant/activity/facets", middleware.Chain(http.HandlerFunc(h.TenantFacets), authn, org, read))
	mux.Handle("GET /v1/tenant/activity", middleware.Chain(http.HandlerFunc(h.TenantList), authn, org, read))
}
