// Package confirmations is the customer confirmation module of the
// Konfirmasyon add-on (TEC-921, docs/design/confirmations.md): per branch
// settings, the confirmation task of every active booking, the requests
// through the customer notification channels and the automatic cancel or
// flag at the deadline.
package confirmations

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/confirmations/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// RegisterRoutes mounts /v1/tenant/confirmations. The settings are
// business settings (tenant.settings.read / .write) and need the
// confirmation add-on. A nil resolver (tenancy off) mounts nothing.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	module := middleware.RequireModule(sector.ModuleConfirmation)
	read := middleware.RequireTenantPermission(rbac.PermTenantSettingsRead)
	write := middleware.RequireTenantPermission(rbac.PermTenantSettingsWrite)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org, module}, guards...)...))
	}

	route("GET /v1/tenant/confirmations/settings", h.GetSettings, read)
	route("PUT /v1/tenant/confirmations/settings", h.PutSettings, read, write)
	route("POST /v1/tenant/confirmations/settings/reset", h.Reset, read, write)

	// The confirmation list and its follow-up alerts (TEC-924): the
	// reservations' own permissions; members who see only their own
	// calendar do not get the business-wide list.
	resRead := middleware.RequireTenantPermission(rbac.PermTenantReservationsRead)
	resWrite := middleware.RequireTenantPermission(rbac.PermTenantReservationsWrite)
	route("GET /v1/tenant/confirmations/meta", h.Meta, resRead)
	route("GET /v1/tenant/confirmations/facets", h.Facets, resRead)
	route("GET /v1/tenant/confirmations/summary", h.Summary, resRead)
	route("GET /v1/tenant/confirmations", h.List, resRead)
	route("GET /v1/tenant/reservations/{uuid}/confirmation/attempts", h.Attempts, resRead)
	route("POST /v1/tenant/reservations/{uuid}/confirmation/confirm", h.Confirm, resRead, resWrite)
}
