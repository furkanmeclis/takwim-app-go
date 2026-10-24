// Package servicebundles is the organization-scoped catalog of combined
// service bundles (/v1/tenant/service-bundles, TEC-895): several services
// sold as one priced offer, in the branches that offer it. Bundles belong to
// the service catalog and use its permissions (tenant.services.read /
// tenant.services.write).
package servicebundles

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	bundleshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/servicebundles/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/servicebundles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// Resource is the live list slug of bundles.
const Resource = model.Slug

// RegisterRoutes mounts /v1/tenant/service-bundles; a nil resolver
// (tenancy off) mounts nothing.
func RegisterRoutes(
	mux *http.ServeMux,
	h *bundleshandler.Handler,
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
	read := middleware.RequireTenantPermission(rbac.PermTenantServicesRead)
	write := middleware.RequireTenantPermission(rbac.PermTenantServicesWrite)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org, module}, guards...)...))
	}

	// Static segments before /{uuid}.
	route("GET /v1/tenant/service-bundles/meta", h.Meta, read)
	route("GET /v1/tenant/service-bundles/facets", h.Facets, read)
	route("GET /v1/tenant/service-bundles", h.List, read)
	route("POST /v1/tenant/service-bundles", h.Create, write)
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
