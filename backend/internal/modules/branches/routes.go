// Package branches is the first organization-scoped list resource
// (/v1/tenant/branches): branches and their service points. It is the worked
// example of .agents/skills/add-tenant-resource.
package branches

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/adapters"
	branchhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// EngineHandlers are the shared engine handlers the resource mounts its
// export / import / bulk routes on. Nil members skip those routes.
type EngineHandlers struct {
	Export http.HandlerFunc
	Import http.HandlerFunc
	Sample http.HandlerFunc
	Bulk   http.HandlerFunc
}

// RegisterRoutes mounts /v1/tenant/branches. A nil resolver (tenancy off)
// mounts nothing.
func RegisterRoutes(
	mux *http.ServeMux,
	h *branchhandler.Handler,
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
	perm := middleware.RequireTenantPermission
	read := perm(rbac.PermTenantBranchesRead)
	write := perm(rbac.PermTenantBranchesWrite)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}

	// Static segments before /{uuid}.
	route("GET /v1/tenant/branches/meta", h.Meta, read)
	route("GET /v1/tenant/branches/facets", h.Facets, read)
	route("GET /v1/tenant/branches", h.List, read)
	route("POST /v1/tenant/branches", h.Create, write)
	if engines.Export != nil {
		route("POST /v1/tenant/branches/export", engines.Export, read, perm(rbac.PermTenantBranchesExport))
	}
	if engines.Import != nil {
		route("POST /v1/tenant/branches/import", engines.Import, write, perm(rbac.PermTenantBranchesImport))
	}
	if engines.Sample != nil {
		route("GET /v1/tenant/branches/import/sample", engines.Sample, perm(rbac.PermTenantBranchesImport))
	}
	if engines.Bulk != nil {
		// The bulk handler checks the per-action permission in the scope.
		route("POST /v1/tenant/branches/bulk", engines.Bulk, read)
	}

	route("GET /v1/tenant/branches/{uuid}", h.Get, read)
	route("PATCH /v1/tenant/branches/{uuid}", h.Patch, write)
	route("DELETE /v1/tenant/branches/{uuid}", h.Delete, write)
	route("GET /v1/tenant/branches/{uuid}/activity", h.Activity, read)
	route("GET /v1/tenant/branches/{uuid}/service-points", h.ListServicePoints, read)
	route("POST /v1/tenant/branches/{uuid}/service-points", h.CreateServicePoint, write)
	route("PATCH /v1/tenant/branches/{uuid}/service-points/{pointUuid}", h.PatchServicePoint, write)
	route("DELETE /v1/tenant/branches/{uuid}/service-points/{pointUuid}", h.DeleteServicePoint, write)
}

// ExportQueryKeys are the list filters an export honours.
var ExportQueryKeys = []string{"city", "is_active"}

// Resource is the engine slug of branches.
const Resource = adapters.Resource
