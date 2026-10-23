// Package staff is the organization-scoped staff resource (/v1/tenant/staff,
// TEC-890): people of the business with their branch assignments, weekly
// shifts per branch and time off. One person has one calendar: shifts never
// overlap across branches (A04).
package staff

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/adapters"
	staffhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/handler"
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

// RegisterRoutes mounts /v1/tenant/staff. A nil resolver (tenancy off)
// mounts nothing.
func RegisterRoutes(
	mux *http.ServeMux,
	h *staffhandler.Handler,
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
	module := middleware.RequireModule(sector.ModuleStaff)
	perm := middleware.RequireTenantPermission
	read := perm(rbac.PermTenantStaffRead)
	write := perm(rbac.PermTenantStaffWrite)
	schedule := perm(rbac.PermTenantSchedulesWrite)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org, module}, guards...)...))
	}

	// Static segments before /{uuid}.
	route("GET /v1/tenant/staff/meta", h.Meta, read)
	route("GET /v1/tenant/staff/facets", h.Facets, read)
	route("GET /v1/tenant/staff/linkable-members", h.LinkableMembers, write)
	route("GET /v1/tenant/staff", h.List, read)
	route("POST /v1/tenant/staff", h.Create, write)
	if engines.Export != nil {
		route("POST /v1/tenant/staff/export", engines.Export, read, perm(rbac.PermTenantStaffExport))
	}
	if engines.Import != nil {
		route("POST /v1/tenant/staff/import", engines.Import, write, perm(rbac.PermTenantStaffImport))
	}
	if engines.Sample != nil {
		route("GET /v1/tenant/staff/import/sample", engines.Sample, perm(rbac.PermTenantStaffImport))
	}
	if engines.Bulk != nil {
		// The bulk handler checks the per-action permission in the scope.
		route("POST /v1/tenant/staff/bulk", engines.Bulk, read)
	}

	route("GET /v1/tenant/staff/{uuid}", h.Get, read)
	route("PATCH /v1/tenant/staff/{uuid}", h.Patch, write)
	route("DELETE /v1/tenant/staff/{uuid}", h.Delete, write)
	route("GET /v1/tenant/staff/{uuid}/activity", h.Activity, read)
	route("PUT /v1/tenant/staff/{uuid}/branches/{branchUuid}/shifts", h.PutShifts, read, schedule)
	route("GET /v1/tenant/staff/{uuid}/time-off", h.ListTimeOff, read)
	route("POST /v1/tenant/staff/{uuid}/time-off", h.CreateTimeOff, read, schedule)
	route("PATCH /v1/tenant/staff/{uuid}/time-off/{timeOffUuid}", h.PatchTimeOff, read, schedule)
	route("DELETE /v1/tenant/staff/{uuid}/time-off/{timeOffUuid}", h.DeleteTimeOff, read, schedule)
}

// ExportQueryKeys are the list filters an export honours.
var ExportQueryKeys = []string{"branch", "is_active", "show_online", "has_account"}

// Resource is the engine slug of staff.
const Resource = adapters.Resource

// RegisterLiveResources declares the staff list channel
// (org:{org_uuid}:resource:tenant.staff) and its tenant read permission.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(Resource, rbac.PermTenantStaffRead)
}
