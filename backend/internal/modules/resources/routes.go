// Package resources is the organization-scoped resource catalog
// (/v1/tenant/resources, TEC-891): the bookable capacity of a branch besides
// staff time (chair, table, area, room, device, bay) with its capacity range,
// optional own working hours and closures.
package resources

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/adapters"
	resourceshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/handler"
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

// RegisterRoutes mounts /v1/tenant/resources. A nil resolver (tenancy off)
// mounts nothing.
func RegisterRoutes(
	mux *http.ServeMux,
	h *resourceshandler.Handler,
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
	module := middleware.RequireModule(sector.ModuleResources)
	perm := middleware.RequireTenantPermission
	read := perm(rbac.PermTenantResourcesRead)
	write := perm(rbac.PermTenantResourcesWrite)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org, module}, guards...)...))
	}

	// Static segments before /{uuid}.
	route("GET /v1/tenant/resources/meta", h.Meta, read)
	route("GET /v1/tenant/resources/facets", h.Facets, read)
	route("GET /v1/tenant/resources", h.List, read)
	route("POST /v1/tenant/resources", h.Create, write)
	if engines.Export != nil {
		route("POST /v1/tenant/resources/export", engines.Export, read, perm(rbac.PermTenantResourcesExport))
	}
	if engines.Import != nil {
		route("POST /v1/tenant/resources/import", engines.Import, write, perm(rbac.PermTenantResourcesImport))
	}
	if engines.Sample != nil {
		route("GET /v1/tenant/resources/import/sample", engines.Sample, perm(rbac.PermTenantResourcesImport))
	}
	if engines.Bulk != nil {
		// The bulk handler checks the per-action permission in the scope.
		route("POST /v1/tenant/resources/bulk", engines.Bulk, read)
	}

	route("GET /v1/tenant/resources/{uuid}", h.Get, read)
	route("PATCH /v1/tenant/resources/{uuid}", h.Patch, write)
	route("DELETE /v1/tenant/resources/{uuid}", h.Delete, write)
	route("GET /v1/tenant/resources/{uuid}/activity", h.Activity, read)
	route("GET /v1/tenant/resources/{uuid}/closures", h.ListClosures, read)
	route("POST /v1/tenant/resources/{uuid}/closures", h.CreateClosure, write)
	route("PATCH /v1/tenant/resources/{uuid}/closures/{closureUuid}", h.PatchClosure, write)
	route("DELETE /v1/tenant/resources/{uuid}/closures/{closureUuid}", h.DeleteClosure, write)
}

// ExportQueryKeys are the list filters an export honours.
var ExportQueryKeys = []string{"branch", "kind", "zone", "is_active", "show_online"}

// Resource is the engine slug of resources.
const Resource = adapters.Resource

// RegisterLiveResources declares the resource list channel
// (org:{org_uuid}:resource:tenant.resources) and its tenant read permission.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(Resource, rbac.PermTenantResourcesRead)
}
