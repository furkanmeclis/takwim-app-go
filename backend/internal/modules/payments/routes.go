// Package payments is the payment layer of Takwim (TEC-912,
// docs/design/payments.md): money taken at the business for a reservation
// and online full payment on the hosted page of the business's own
// provider, recorded only from the provider's verified server
// notification. Every payment is a ledger collection (sales module) with
// its cash movement.
package payments

import (
	"net/http"
	"slices"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	paymentshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/paytr"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// RegisterRoutes mounts the panel routes (a nil resolver mounts none) and
// the public provider routes. The panel view needs the reservations reach
// (read or own); a collection also tenant.payment.collect and the cash
// module.
func RegisterRoutes(
	mux *http.ServeMux,
	h *paymentshandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	// Provider notifications and the development test page: public, the
	// business is found from the payment reference.
	mux.HandleFunc("POST /v1/public/payments/paytr/notify", h.Notify(paytr.ProviderCode))
	mux.HandleFunc("GET /v1/public/payments/testpay/{ref}", h.TestPage)
	mux.HandleFunc("POST /v1/public/payments/testpay/{ref}/complete", h.TestComplete)

	if resolver == nil {
		return
	}
	base := []func(http.Handler) http.Handler{
		middleware.Authenticate(tokens, loader),
		middleware.RequireOrganization(resolver),
		middleware.RequireModule(sector.ModuleReservations),
		middleware.RequireAnyTenantPermission(rbac.PermTenantReservationsRead, rbac.PermTenantReservationsOwn),
	}
	mux.Handle("GET /v1/tenant/reservations/{uuid}/payments", middleware.Chain(http.HandlerFunc(h.ReservationPayments), base...))
	mux.Handle("POST /v1/tenant/reservations/{uuid}/collections", middleware.Chain(http.HandlerFunc(h.Collect), append(slices.Clone(base),
		middleware.RequireModule(sector.ModuleCash),
		middleware.RequireTenantPermission(rbac.PermTenantPaymentCollect),
	)...))
	// Refunds (TEC-913): tenant.payment.refund and the cash module.
	refund := append(slices.Clone(base), middleware.RequireModule(sector.ModuleCash), middleware.RequireTenantPermission(rbac.PermTenantPaymentRefund))
	mux.Handle("POST /v1/tenant/reservations/{uuid}/refunds", middleware.Chain(http.HandlerFunc(h.Refund), refund...))
	mux.Handle("POST /v1/tenant/refunds/{uuid}/resolve", middleware.Chain(http.HandlerFunc(h.ResolveRefund), refund...))
	// Closing a refund to make without paying it back (TEC-978).
	mux.Handle("POST /v1/tenant/reservations/{uuid}/close-without-refund", middleware.Chain(http.HandlerFunc(h.CloseWithoutRefund), refund...))

	// The payment follow-up list: members who take or pay back money, or
	// read the books.
	follow := []func(http.Handler) http.Handler{
		middleware.Authenticate(tokens, loader),
		middleware.RequireOrganization(resolver),
		middleware.RequireModule(sector.ModuleReservations),
		middleware.RequireAnyTenantPermission(rbac.PermTenantPaymentRefund, rbac.PermTenantPaymentCollect, rbac.PermTenantFinanceRead),
	}
	mux.Handle("GET /v1/tenant/payments/follow-ups/meta", middleware.Chain(http.HandlerFunc(h.FollowUpsMeta), follow...))
	mux.Handle("GET /v1/tenant/payments/follow-ups/facets", middleware.Chain(http.HandlerFunc(h.FollowUpFacets), follow...))
	mux.Handle("GET /v1/tenant/payments/follow-ups", middleware.Chain(http.HandlerFunc(h.ListFollowUps), follow...))
}
