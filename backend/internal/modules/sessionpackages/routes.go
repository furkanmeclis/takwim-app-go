// Package sessionpackages is the multi-session packages of a business
// (/v1/tenant/session-packages, /v1/tenant/customer-packages, TEC-920):
// the package definitions (N sessions of a service for a price, branches,
// validity), the packages sold to customers with their balance, refunds
// and the package report. The sessions of a visit live in the
// reservations module (/v1/tenant/reservations/{uuid}/package-sessions).
package sessionpackages

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	sessionpackageshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sessionpackages/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sessionpackages/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// RegisterRoutes mounts the package routes. A nil resolver (tenancy off)
// mounts nothing. Reading takes tenant.session_packages.read; managing the
// definitions .write; selling .sell with tenant.payment.collect; a refund
// tenant.payment.refund. Selling and refunding move money, so they also
// need the cash module.
func RegisterRoutes(
	mux *http.ServeMux,
	h *sessionpackageshandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	module := middleware.RequireModule(sector.ModulePackages)
	cash := middleware.RequireModule(sector.ModuleCash)
	perm := middleware.RequireTenantPermission
	read := perm(rbac.PermTenantSessionPackagesRead)
	write := perm(rbac.PermTenantSessionPackagesWrite)
	sell := perm(rbac.PermTenantSessionPackagesSell)
	collect := perm(rbac.PermTenantPaymentCollect)
	refund := perm(rbac.PermTenantPaymentRefund)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org, module}, guards...)...))
	}

	// Static segments before /{uuid}.
	route("GET /v1/tenant/session-packages/meta", h.PackagesMeta, read)
	route("GET /v1/tenant/session-packages", h.ListPackages, read)
	route("POST /v1/tenant/session-packages", h.CreatePackage, read, write)
	route("GET /v1/tenant/session-packages/{uuid}", h.GetPackage, read)
	route("PATCH /v1/tenant/session-packages/{uuid}", h.PatchPackage, read, write)
	route("DELETE /v1/tenant/session-packages/{uuid}", h.DeletePackage, read, write)

	route("GET /v1/tenant/customer-packages/meta", h.CustomerPackagesMeta, read)
	route("GET /v1/tenant/customer-packages/facets", h.CustomerPackagesFacets, read)
	route("GET /v1/tenant/customer-packages/summary", h.Summary, read)
	route("GET /v1/tenant/customer-packages", h.ListCustomerPackages, read)
	route("POST /v1/tenant/customer-packages", h.Sell, cash, read, sell, collect)
	route("GET /v1/tenant/customer-packages/{uuid}", h.GetCustomerPackage, read)
	route("POST /v1/tenant/customer-packages/{uuid}/refund", h.Refund, cash, read, refund)
}

// Engine slugs.
const (
	PackagesResource         = model.PackagesSlug
	CustomerPackagesResource = model.CustomerPackagesSlug
)

// RegisterLiveResources declares the package list channels.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(PackagesResource, rbac.PermTenantSessionPackagesRead)
	reg.Register(CustomerPackagesResource, rbac.PermTenantSessionPackagesRead)
}
