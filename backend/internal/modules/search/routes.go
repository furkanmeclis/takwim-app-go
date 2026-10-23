package search

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	searchhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/search/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// RegisterRoutes mounts command-palette search endpoints.
func RegisterRoutes(
	mux *http.ServeMux,
	h *searchhandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
) {
	authn := middleware.Authenticate(tokens, loader)
	session := middleware.RequirePermission(rbac.PermAuthSession)

	mux.Handle("GET /v1/search/specs", middleware.Chain(
		http.HandlerFunc(h.Specs), authn, session,
	))
	mux.Handle("GET /v1/search", middleware.Chain(
		http.HandlerFunc(h.Search), authn, session,
	))
}

// RegisterTenantRoutes mounts organization-scoped search. Specs and hits are
// filtered by the caller's permissions in the X-Organization organization and
// by organization_id in the index. A nil resolver (tenancy off) mounts nothing.
func RegisterTenantRoutes(
	mux *http.ServeMux,
	h *searchhandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)

	mux.Handle("GET /v1/tenant/search/specs", middleware.Chain(
		http.HandlerFunc(h.TenantSpecs), authn, org,
	))
	mux.Handle("GET /v1/tenant/search", middleware.Chain(
		http.HandlerFunc(h.TenantSearch), authn, org,
	))
}
