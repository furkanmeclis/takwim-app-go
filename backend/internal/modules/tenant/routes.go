// Package tenant holds /v1/tenant routes: requests scoped to the organization
// named by the X-Organization header (see docs/design/tenant-core.md).
package tenant

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

// RegisterRoutes mounts the tenant routes. A nil resolver (tenancy off)
// registers nothing.
func RegisterRoutes(
	mux *http.ServeMux,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)

	mux.Handle("GET /v1/tenant/context", middleware.Chain(
		http.HandlerFunc(getContext), authn, org,
	))
}
