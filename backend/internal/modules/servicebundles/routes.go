// Package servicebundles is the organization-scoped catalog of combined
// service bundles (/v1/tenant/service-bundles, TEC-895): several services
// sold as one priced offer, in the branches that offer it. Bundles belong to
// the service catalog and use its permissions (tenant.services.read /
// write / export / import / bulk.*, TEC-985).
package servicebundles

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/servicebundles/adapters"
	bundleshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/servicebundles/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/servicebundles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// Resource is the live list and engine slug of bundles.
const Resource = model.Slug

// ExportQueryKeys are the list filters an export honours.
var ExportQueryKeys = adapters.ExportQueryKeys

// EngineHandlers are the shared engine handlers the resource mounts its
// export / import / bulk routes on. Nil members skip those routes.
type EngineHandlers struct {
	Export http.HandlerFunc
	Import http.HandlerFunc
	Sample http.HandlerFunc
	Bulk   http.HandlerFunc
}

// RegisterRoutes mounts /v1/tenant/service-bundles; a nil resolver
// (tenancy off) mounts nothing. Export, import and bulk use the service
// catalog's permissions (tenant.services.export / import / bulk.*).
func RegisterRoutes(
	mux *http.ServeMux,
	h *bundleshandler.Handler,
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
	module := middleware.RequireModule(sector.ModuleServices)
	perm := middleware.RequireTenantPermission
	read := perm(rbac.PermTenantServicesRead)
	write := perm(rbac.PermTenantServicesWrite)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org, module}, guards...)...))
	}

	// Static segments before /{uuid}.
	route("GET /v1/tenant/service-bundles/meta", h.Meta, read)
	route("GET /v1/tenant/service-bundles/facets", h.Facets, read)
	route("GET /v1/tenant/service-bundles", h.List, read)
	route("POST /v1/tenant/service-bundles", h.Create, write)
	if engines.Export != nil {
		route("POST /v1/tenant/service-bundles/export", engines.Export, read, perm(rbac.PermTenantServicesExport))
	}
	if engines.Import != nil {
		route("POST /v1/tenant/service-bundles/import", engines.Import, write, perm(rbac.PermTenantServicesImport))
	}
	if engines.Sample != nil {
		route("GET /v1/tenant/service-bundles/import/sample", engines.Sample, perm(rbac.PermTenantServicesImport))
	}
	if engines.Bulk != nil {
		// The bulk handler checks the per-action permission in the scope.
		route("POST /v1/tenant/service-bundles/bulk", engines.Bulk, read)
	}
	route("GET /v1/tenant/service-bundles/{uuid}", h.Get, read)
	route("PATCH /v1/tenant/service-bundles/{uuid}", h.Patch, write)
	route("DELETE /v1/tenant/service-bundles/{uuid}", h.Delete, write)
	route("GET /v1/tenant/service-bundles/{uuid}/activity", h.Activity, read)
}

// RegisterLiveResources declares the bundle list channel
// (org:{org_uuid}:resource:tenant.service_bundles) and its read permission.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(Resource, rbac.PermTenantServicesRead)
}
