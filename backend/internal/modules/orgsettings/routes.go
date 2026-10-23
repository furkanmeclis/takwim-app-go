// Package orgsettings is the versioned organization settings resource
// (/v1/tenant/settings, TEC-690): company / billing identity, reservation and
// payment rules, policies, sales channels and tax mapping.
package orgsettings

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// RegisterRoutes mounts /v1/tenant/settings. A nil resolver (tenancy off)
// mounts nothing. Saving does not need step-up.
func RegisterRoutes(
	mux *http.ServeMux,
	h *handler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantSettingsRead)
	write := middleware.RequireTenantPermission(rbac.PermTenantSettingsWrite)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}

	route("GET /v1/tenant/settings", h.Get, read)
	route("PUT /v1/tenant/settings", h.Put, read, write)
	route("GET /v1/tenant/settings/versions/meta", h.VersionsMetaH, read)
	route("GET /v1/tenant/settings/versions", h.ListVersions, read)
	route("GET /v1/tenant/settings/versions/{version}", h.GetVersion, read)
}
