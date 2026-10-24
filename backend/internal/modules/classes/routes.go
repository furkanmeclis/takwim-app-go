// Package classes is the class timetable of a gym (/v1/tenant/classes,
// /v1/tenant/class-sessions, TEC-970): weekly classes of group services
// with their trainer, area and seats, the sessions they make with the
// seats booked, single-session changes (cancel, seats) and the bookings of
// a session. Attendance is the reservation's own step (complete, no-show).
package classes

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	classeshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// RegisterRoutes mounts the timetable routes behind the classes module. A
// nil resolver (tenancy off) mounts nothing. Reading takes
// tenant.classes.read; managing classes and sessions tenant.classes.write.
func RegisterRoutes(
	mux *http.ServeMux,
	h *classeshandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	module := middleware.RequireModule(sector.ModuleClasses)
	read := middleware.RequireTenantPermission(rbac.PermTenantClassesRead)
	write := middleware.RequireTenantPermission(rbac.PermTenantClassesWrite)
	reservationsWrite := middleware.RequireTenantPermission(rbac.PermTenantReservationsWrite)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org, module}, guards...)...))
	}

	// Static segments before /{uuid}.
	route("GET /v1/tenant/classes/meta", h.Meta, read)
	route("GET /v1/tenant/classes/facets", h.Facets, read)
	route("GET /v1/tenant/classes/form", h.Form, read, write)
	route("GET /v1/tenant/classes", h.List, read)
	route("POST /v1/tenant/classes", h.Create, read, write)
	route("GET /v1/tenant/classes/{uuid}", h.Get, read)
	route("PATCH /v1/tenant/classes/{uuid}", h.Patch, read, write)
	route("DELETE /v1/tenant/classes/{uuid}", h.Delete, read, write)
	route("GET /v1/tenant/classes/{uuid}/sessions/{date}", h.Session, read)
	route("PATCH /v1/tenant/classes/{uuid}/sessions/{date}", h.ChangeSession, read, write)
	// Cancelling a session with its bookings also cancels reservations.
	route("POST /v1/tenant/classes/{uuid}/sessions/{date}/cancel", h.CancelSession, read, write, reservationsWrite)

	route("GET /v1/tenant/class-sessions", h.Sessions, read)
}

// Resource is the live / activity slug of the timetable.
const Resource = model.Slug

// RegisterLiveResources declares the timetable list channel.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(Resource, rbac.PermTenantClassesRead)
}
