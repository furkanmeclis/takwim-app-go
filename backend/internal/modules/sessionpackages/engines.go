package sessionpackages

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sessionpackages/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// EngineHandlers are the shared engine handlers the module mounts its
// export and bulk routes on (TEC-984). Nil members skip those routes.
type EngineHandlers struct {
	// ExportCustomerPackages exports the sold packages.
	ExportCustomerPackages http.HandlerFunc
	// BulkPackages runs bulk actions on the definitions.
	BulkPackages http.HandlerFunc
}

// ExportQueryKeys are the sold package list filters an export honours.
var ExportQueryKeys = adapters.ExportQueryKeys

// RegisterEngineRoutes mounts POST /v1/tenant/customer-packages/export
// (read + export) and POST /v1/tenant/session-packages/bulk (read; the
// bulk handler checks each action's slug). Both need the packages module.
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
			middleware.RequireModule(sector.ModulePackages),
			middleware.RequireTenantPermission(rbac.PermTenantSessionPackagesRead),
		}, guards...)...))
	}
	if engines.ExportCustomerPackages != nil {
		route("POST /v1/tenant/customer-packages/export", engines.ExportCustomerPackages,
			middleware.RequireTenantPermission(rbac.PermTenantSessionPackagesExport))
	}
	if engines.BulkPackages != nil {
		route("POST /v1/tenant/session-packages/bulk", engines.BulkPackages)
	}
}
