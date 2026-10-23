// Package reservations is the reservation record of Takwim (TEC-899) and
// its panel API (/v1/tenant/reservations, TEC-900): list, detail, booking
// at the counter, accepting / rejecting requests, moving, cancelling and
// closing a visit. The capacity of a booking lives in the availability
// holds; the record and its capacity always change together.
package reservations

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	reservationshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// RegisterRoutes mounts /v1/tenant/reservations. A nil resolver (tenancy
// off) mounts nothing. Every route takes the full slug or
// tenant.reservations.own (the member's own calendar).
func RegisterRoutes(
	mux *http.ServeMux,
	h *reservationshandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	if resolver == nil {
		return
	}
	route := func(pattern string, fn http.HandlerFunc, full string) {
		mux.Handle(pattern, middleware.Chain(fn,
			middleware.Authenticate(tokens, loader),
			middleware.RequireOrganization(resolver),
			middleware.RequireModule(sector.ModuleReservations),
			middleware.RequireAnyTenantPermission(full, rbac.PermTenantReservationsOwn),
		))
	}
	read, write := rbac.PermTenantReservationsRead, rbac.PermTenantReservationsWrite
	// Static segments before /{uuid}.
	route("GET /v1/tenant/reservations/meta", h.Meta, read)
	route("GET /v1/tenant/reservations/facets", h.Facets, read)
	route("GET /v1/tenant/reservations", h.List, read)
	route("POST /v1/tenant/reservations", h.Create, write)
	route("GET /v1/tenant/reservations/{uuid}", h.Get, read)
	route("PATCH /v1/tenant/reservations/{uuid}", h.Notes, write)
	route("POST /v1/tenant/reservations/{uuid}/reschedule", h.Reschedule, write)
	route("POST /v1/tenant/reservations/{uuid}/accept", h.Action("accept"), write)
	route("POST /v1/tenant/reservations/{uuid}/reject", h.Action("reject"), write)
	route("POST /v1/tenant/reservations/{uuid}/cancel", h.Action("cancel"), write)
	route("POST /v1/tenant/reservations/{uuid}/start", h.Action("start"), write)
	route("POST /v1/tenant/reservations/{uuid}/complete", h.Action("complete"), write)
	route("POST /v1/tenant/reservations/{uuid}/no-show", h.Action("no-show"), write)
}

// Resource is the resource slug of reservations.
const Resource = model.Slug

// RegisterLiveResources declares the list channel
// (org:{org_uuid}:resource:tenant.reservations). Own calendar members
// subscribe too; they refetch and the API filters what they see.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(Resource, rbac.PermTenantReservationsRead)
}
