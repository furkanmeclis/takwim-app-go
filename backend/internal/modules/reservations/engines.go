package reservations

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// EngineHandlers are the shared engine handlers the resource mounts its
// export and bulk routes on (TEC-963). Nil members skip those routes; there
// is no import.
type EngineHandlers struct {
	Export http.HandlerFunc
	Bulk   http.HandlerFunc
}

// ExportQueryKeys are the list filters an export honours (q and sort too).
var ExportQueryKeys = []string{
	"branch", "staff", "status", "approval_status", "confirmation_status", "payment_status", "source",
	"starts_from", "starts_to", "customer_uuid",
}

// RegisterEngineRoutes mounts POST /v1/tenant/reservations/export and
// /bulk. Both take the read / write slug or tenant.reservations.own: the
// adapters work inside the same reach as the list (branch scope, own
// records). Export also needs tenant.reservations.export; the bulk handler
// checks each action's slug.
func RegisterEngineRoutes(
	mux *http.ServeMux,
	engines EngineHandlers,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	if resolver == nil {
		return
	}
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{
			middleware.Authenticate(tokens, loader),
			middleware.RequireOrganization(resolver),
			middleware.RequireModule(sector.ModuleReservations),
		}, guards...)...))
	}
	if engines.Export != nil {
		route("POST /v1/tenant/reservations/export", engines.Export,
			middleware.RequireAnyTenantPermission(rbac.PermTenantReservationsRead, rbac.PermTenantReservationsOwn),
			middleware.RequireTenantPermission(rbac.PermTenantReservationsExport))
	}
	if engines.Bulk != nil {
		route("POST /v1/tenant/reservations/bulk", engines.Bulk,
			middleware.RequireAnyTenantPermission(rbac.PermTenantReservationsWrite, rbac.PermTenantReservationsOwn))
	}
}
