// Package availability is the slot engine of Takwim (TEC-897): which start
// times a branch can book for a list of services, with the staff member and
// resources of each line. The panel route lives here; the public booking API
// (TEC-906) calls the same usecase with the online channel.
package availability

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	availabilityhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// RegisterRoutes mounts /v1/tenant/availability. A nil resolver (tenancy
// off) mounts nothing.
func RegisterRoutes(
	mux *http.ServeMux,
	h *availabilityhandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	if resolver == nil {
		return
	}
	route := func(pattern string, fn http.HandlerFunc, full, own string) {
		mux.Handle(pattern, middleware.Chain(fn,
			middleware.Authenticate(tokens, loader),
			middleware.RequireOrganization(resolver),
			middleware.RequireModule(sector.ModuleReservations),
			middleware.RequireAnyTenantPermission(full, own),
		))
	}
	read, write, own := rbac.PermTenantReservationsRead, rbac.PermTenantReservationsWrite, rbac.PermTenantReservationsOwn
	route("POST /v1/tenant/availability/search", h.Search, read, own)
	route("POST /v1/tenant/availability/holds", h.CreateHold, write, own)
	route("GET /v1/tenant/availability/holds/{uuid}", h.GetHold, read, own)
	route("POST /v1/tenant/availability/holds/{uuid}/extend", h.ExtendHold, write, own)
	route("DELETE /v1/tenant/availability/holds/{uuid}", h.ReleaseHold, write, own)
}
