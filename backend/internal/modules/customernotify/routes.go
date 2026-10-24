// Package customernotify is the customer notification engine (TEC-911,
// docs/design/customer-notifications.md): reservation notices with the
// panel link, request received, accepted, rejected, moved, cancelled and a
// reminder before the visit, in every business (no add-on needed).
package customernotify

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// RegisterRoutes mounts /v1/tenant/customer-notifications. The rules and
// texts are business settings (tenant.settings.read / .write). A nil
// resolver (tenancy off) mounts nothing.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
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

	route("GET /v1/tenant/customer-notifications/settings", h.GetSettings, read)
	route("PUT /v1/tenant/customer-notifications/settings", h.PutSettings, read, write)
	route("POST /v1/tenant/customer-notifications/settings/reset", h.Reset, read, write)
	route("POST /v1/tenant/customer-notifications/preview", h.Preview, read)
	route("GET /v1/tenant/customer-notifications/deliveries/meta", h.DeliveriesMeta, read)
	route("GET /v1/tenant/customer-notifications/deliveries", h.Deliveries, read)
}
