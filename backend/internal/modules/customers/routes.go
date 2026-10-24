// Package customers is the organization-scoped customer list
// (/v1/tenant/customers, TEC-896): the business's own record of each
// customer (contact, notes, tags, consent). The shared, platform-level
// identity behind a verified phone is never reachable through these routes.
package customers

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/adapters"
	customershandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// EngineHandlers are the shared engine handlers the resource mounts its
// export / import / bulk routes on. Nil members skip those routes.
type EngineHandlers struct {
	Export http.HandlerFunc
	Import http.HandlerFunc
	Sample http.HandlerFunc
	Bulk   http.HandlerFunc
}

// RegisterRoutes mounts /v1/tenant/customers. A nil resolver (tenancy off)
// mounts nothing.
func RegisterRoutes(
	mux *http.ServeMux,
	h *customershandler.Handler,
	engines EngineHandlers,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	module := middleware.RequireModule(sector.ModuleCustomers)
	perm := middleware.RequireTenantPermission
	read := perm(rbac.PermTenantCustomersRead)
	write := perm(rbac.PermTenantCustomersWrite)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org, module}, guards...)...))
	}

	// Static segments before /{uuid}.
	route("GET /v1/tenant/customers/meta", h.Meta, read)
	route("GET /v1/tenant/customers/facets", h.Facets, read)
	route("GET /v1/tenant/customers", h.List, read)
	route("POST /v1/tenant/customers", h.Create, write)
	if engines.Export != nil {
		route("POST /v1/tenant/customers/export", engines.Export, read, perm(rbac.PermTenantCustomersExport))
	}
	if engines.Import != nil {
		route("POST /v1/tenant/customers/import", engines.Import, write, perm(rbac.PermTenantCustomersImport))
	}
	if engines.Sample != nil {
		route("GET /v1/tenant/customers/import/sample", engines.Sample, perm(rbac.PermTenantCustomersImport))
	}
	if engines.Bulk != nil {
		// The bulk handler checks the per-action permission in the scope.
		route("POST /v1/tenant/customers/bulk", engines.Bulk, read)
	}

	route("GET /v1/tenant/customers/{uuid}", h.Get, read)
	route("PATCH /v1/tenant/customers/{uuid}", h.Patch, write)
	route("DELETE /v1/tenant/customers/{uuid}", h.Delete, write)
	route("GET /v1/tenant/customers/{uuid}/activity", h.Activity, read)
	// TEC-991: reopen the panel history closed for the person.
	route("POST /v1/tenant/customers/{uuid}/history/restore", h.RestoreHistory, write)
}

// ExportQueryKeys are the list filters an export honours.
var ExportQueryKeys = []string{"tag", "source", "sms_consent", "email_consent", "verified"}

// Resource is the engine slug of customers.
const Resource = adapters.Resource

// RegisterLiveResources declares the resource list channel
// (org:{org_uuid}:resource:tenant.customers) and its tenant read permission.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(Resource, rbac.PermTenantCustomersRead)
}
