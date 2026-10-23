// Package services is the organization-scoped service catalog
// (/v1/tenant/services, TEC-894): what a business sells with its duration,
// buffers and price, the branches that offer it (with branch price /
// duration), the staff who may perform it and the resource kinds it needs.
package services

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/adapters"
	serviceshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/handler"
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

// RegisterRoutes mounts /v1/tenant/services and /v1/tenant/service-categories.
// The public cover route is always mounted; a nil resolver (tenancy off)
// mounts nothing else.
func RegisterRoutes(
	mux *http.ServeMux,
	h *serviceshandler.Handler,
	engines EngineHandlers,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	// Service covers are public media (booking pages, the mobile app).
	mux.HandleFunc("GET /v1/public/media/services/{orgUuid}/images/{imageUuid}", h.PublicImage)
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

	route("GET /v1/tenant/service-categories", h.ListCategories, read)
	route("POST /v1/tenant/service-categories", h.CreateCategory, write)
	route("PATCH /v1/tenant/service-categories/{uuid}", h.PatchCategory, write)
	route("DELETE /v1/tenant/service-categories/{uuid}", h.DeleteCategory, write)

	// Static segments before /{uuid}.
	route("GET /v1/tenant/services/meta", h.Meta, read)
	route("GET /v1/tenant/services/facets", h.Facets, read)
	route("GET /v1/tenant/services", h.List, read)
	route("POST /v1/tenant/services", h.Create, write)
	if engines.Export != nil {
		route("POST /v1/tenant/services/export", engines.Export, read, perm(rbac.PermTenantServicesExport))
	}
	if engines.Import != nil {
		route("POST /v1/tenant/services/import", engines.Import, write, perm(rbac.PermTenantServicesImport))
	}
	if engines.Sample != nil {
		route("GET /v1/tenant/services/import/sample", engines.Sample, perm(rbac.PermTenantServicesImport))
	}
	if engines.Bulk != nil {
		// The bulk handler checks the per-action permission in the scope.
		route("POST /v1/tenant/services/bulk", engines.Bulk, read)
	}

	route("GET /v1/tenant/services/{uuid}", h.Get, read)
	route("PATCH /v1/tenant/services/{uuid}", h.Patch, write)
	route("DELETE /v1/tenant/services/{uuid}", h.Delete, write)
	route("GET /v1/tenant/services/{uuid}/activity", h.Activity, read)
	route("PUT /v1/tenant/services/{uuid}/offers", h.PutOffers, read, perm(rbac.PermTenantServicesOffer))
	route("PUT /v1/tenant/services/{uuid}/staff", h.PutStaff, write)
	route("PUT /v1/tenant/services/{uuid}/image", h.UploadImage, write)
	route("DELETE /v1/tenant/services/{uuid}/image", h.DeleteImage, write)
}

// ExportQueryKeys are the list filters an export honours.
var ExportQueryKeys = []string{"category", "branch", "payment_mode", "is_active", "show_online"}

// Resource is the engine slug of services.
const Resource = adapters.Resource

// RegisterLiveResources declares the service list channel
// (org:{org_uuid}:resource:tenant.services) and its tenant read permission.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(Resource, rbac.PermTenantServicesRead)
}
