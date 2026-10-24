// Package reports serves the reports of a business (/v1/tenant/reports,
// TEC-925): reservations by status and channel, confirmation results,
// revenue, collections, transfers, other income and expenses, staff share
// and payouts, the business share and the operating result, and the
// stock; per day, per branch (comparison) and in total, with a CSV export.
// Everything is read from the records of the other modules.
package reports

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// RegisterRoutes mounts /v1/tenant/reports and the daily summary of the
// home page. A nil resolver (tenancy off) mounts nothing. The reports
// need the reports module and tenant.finance.read, the CSV also
// tenant.report.export. The daily summary (TEC-926) takes any of the
// reservation, finance or earnings reads and returns only the sections
// they allow (its money also needs the reports module).
func RegisterRoutes(
	mux *http.ServeMux,
	h *handler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	module := middleware.RequireModule(sector.ModuleReports)
	read := middleware.RequireTenantPermission(rbac.PermTenantFinanceRead)
	export := middleware.RequireTenantPermission(rbac.PermTenantReportExport)

	mux.Handle("GET /v1/tenant/reports/summary", middleware.Chain(http.HandlerFunc(h.Summary), authn, org, module, read))
	mux.Handle("GET /v1/tenant/reports/export", middleware.Chain(http.HandlerFunc(h.Export), authn, org, module, read, export))

	daily := middleware.RequireAnyTenantPermission(
		rbac.PermTenantReservationsRead, rbac.PermTenantReservationsOwn, rbac.PermTenantFinanceRead,
		rbac.PermTenantEarningsRead, rbac.PermTenantEarningsOwn,
	)
	mux.Handle("GET /v1/tenant/daily-summary", middleware.Chain(http.HandlerFunc(h.Daily), authn, org, daily))
}
