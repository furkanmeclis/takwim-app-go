// Package products is the organization-scoped product catalog and stock
// (/v1/tenant/products, /v1/tenant/stock-movements, TEC-918): product
// cards with variants, the branches that sell them (with branch price),
// branch stock levels and the append-only stock ledger (receipt, sale,
// return, count adjustment, transfer between branches).
package products

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/adapters"
	productshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/handler"
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

// RegisterRoutes mounts /v1/tenant/products and /v1/tenant/stock-movements.
// The public cover route is always mounted; a nil resolver (tenancy off)
// mounts nothing else.
func RegisterRoutes(
	mux *http.ServeMux,
	h *productshandler.Handler,
	engines EngineHandlers,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	// Product covers are public media (booking pages, the mobile app).
	mux.HandleFunc("GET /v1/public/media/products/{orgUuid}/images/{imageUuid}", h.PublicImage)
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	module := middleware.RequireModule(sector.ModuleProducts)
	perm := middleware.RequireTenantPermission
	read := perm(rbac.PermTenantProductsRead)
	write := perm(rbac.PermTenantProductsWrite)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org, module}, guards...)...))
	}

	// Static segments before /{uuid}.
	route("GET /v1/tenant/products/meta", h.Meta, read)
	route("GET /v1/tenant/products/facets", h.Facets, read)
	route("GET /v1/tenant/products/lookup", h.Lookup, read)
	route("GET /v1/tenant/products", h.List, read)
	route("POST /v1/tenant/products", h.Create, write)
	if engines.Export != nil {
		route("POST /v1/tenant/products/export", engines.Export, read, perm(rbac.PermTenantProductsExport))
	}
	if engines.Import != nil {
		route("POST /v1/tenant/products/import", engines.Import, write, perm(rbac.PermTenantProductsImport))
	}
	if engines.Sample != nil {
		route("GET /v1/tenant/products/import/sample", engines.Sample, perm(rbac.PermTenantProductsImport))
	}
	if engines.Bulk != nil {
		// The bulk handler checks the per-action permission in the scope.
		route("POST /v1/tenant/products/bulk", engines.Bulk, read)
	}

	route("GET /v1/tenant/products/{uuid}", h.Get, read)
	route("PATCH /v1/tenant/products/{uuid}", h.Patch, write)
	route("DELETE /v1/tenant/products/{uuid}", h.Delete, write)
	route("GET /v1/tenant/products/{uuid}/activity", h.Activity, read)
	route("PUT /v1/tenant/products/{uuid}/offers", h.PutOffers, read, perm(rbac.PermTenantProductsOffer))
	route("PUT /v1/tenant/products/{uuid}/image", h.UploadImage, write)
	route("DELETE /v1/tenant/products/{uuid}/image", h.DeleteImage, write)
	route("POST /v1/tenant/products/{uuid}/variants", h.CreateVariant, write)
	route("PATCH /v1/tenant/products/{uuid}/variants/{variantUuid}", h.PatchVariant, write)
	route("DELETE /v1/tenant/products/{uuid}/variants/{variantUuid}", h.DeleteVariant, write)

	// Stock ledger. POST checks tenant.products.sell (sale) or
	// tenant.products.stock (everything else) in the handler.
	route("GET /v1/tenant/stock-movements/meta", h.MovementMeta, read)
	route("GET /v1/tenant/stock-movements/facets", h.MovementFacets, read)
	route("GET /v1/tenant/stock-movements", h.ListMovements, read)
	route("POST /v1/tenant/stock-movements", h.CreateMovement, read)
}

// ExportQueryKeys are the list filters an export honours.
var ExportQueryKeys = []string{"branch", "stock_status", "is_active", "show_online"}

// Resource is the engine slug of products.
const Resource = adapters.Resource

// RegisterLiveResources declares the product list channel
// (org:{org_uuid}:resource:tenant.products) and its tenant read
// permission. Stock movements publish an update of their product, so the
// movement list listens on the same channel.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(Resource, rbac.PermTenantProductsRead)
}
