// Package staffshares is the staff shares of a business
// (/v1/tenant/staff-shares, TEC-917): the agreed rates, the share of every
// sale line with a staff member (written by the sales module's line hook
// in the line's transaction), payable / paid / remaining balances and
// share payouts through the cash book.
package staffshares

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	staffshareshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// EngineHandlers are the shared engine handlers the module mounts its
// export routes on. A nil member skips the route.
type EngineHandlers struct {
	ExportLines   http.HandlerFunc
	ExportPayouts http.HandlerFunc
}

// RegisterRoutes mounts /v1/tenant/staff-shares. A nil resolver (tenancy
// off) mounts nothing. Reading takes tenant.earnings.read (every share in
// the branch scope) or tenant.earnings.own (the caller's own); rates need
// tenant.earnings.manage, payouts tenant.earnings.pay (with
// tenant.earnings.read); the exports tenant.earnings.read and
// tenant.report.export.
func RegisterRoutes(
	mux *http.ServeMux,
	h *staffshareshandler.Handler,
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
	module := middleware.RequireModule(sector.ModuleStaffShares)
	read := middleware.RequireAnyTenantPermission(rbac.PermTenantEarningsRead, rbac.PermTenantEarningsOwn)
	readAll := middleware.RequireTenantPermission(rbac.PermTenantEarningsRead)
	manage := middleware.RequireTenantPermission(rbac.PermTenantEarningsManage)
	pay := middleware.RequireTenantPermission(rbac.PermTenantEarningsPay)
	export := middleware.RequireTenantPermission(rbac.PermTenantReportExport)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org, module}, guards...)...))
	}

	route("GET /v1/tenant/staff-shares/me", h.Mine, read)
	route("GET /v1/tenant/staff-shares/summary", h.Summary, read)

	route("GET /v1/tenant/staff-shares/rules", h.ListRules, read)
	route("POST /v1/tenant/staff-shares/rules", h.CreateRule, readAll, manage)
	route("GET /v1/tenant/staff-shares/rules/{uuid}", h.GetRule, read)
	route("PATCH /v1/tenant/staff-shares/rules/{uuid}", h.PatchRule, readAll, manage)
	route("DELETE /v1/tenant/staff-shares/rules/{uuid}", h.DeleteRule, readAll, manage)

	// Static segments before /{uuid}.
	route("GET /v1/tenant/staff-shares/lines/meta", h.LinesMeta, read)
	route("GET /v1/tenant/staff-shares/lines", h.ListLines, read)
	if engines.ExportLines != nil {
		route("POST /v1/tenant/staff-shares/lines/export", engines.ExportLines, readAll, export)
	}

	route("GET /v1/tenant/staff-shares/payouts/meta", h.PayoutsMeta, read)
	route("GET /v1/tenant/staff-shares/payouts", h.ListPayouts, read)
	if engines.ExportPayouts != nil {
		route("POST /v1/tenant/staff-shares/payouts/export", engines.ExportPayouts, readAll, export)
	}
	route("POST /v1/tenant/staff-shares/payouts", h.Pay, readAll, pay)
	route("GET /v1/tenant/staff-shares/payouts/{uuid}", h.GetPayout, read)
	route("POST /v1/tenant/staff-shares/payouts/{uuid}/reverse", h.ReversePayout, readAll, pay)
}

// ExportLineQueryKeys are the share list filters an export honours.
var ExportLineQueryKeys = []string{"branch", "staff", "kind", "item_type", "status", "date_from", "date_to"}

// ExportPayoutQueryKeys are the payout list filters an export honours.
var ExportPayoutQueryKeys = []string{"branch", "staff", "kind", "date_from", "date_to"}

// Engine slugs.
const (
	RulesResource   = model.RulesSlug
	LinesResource   = model.LinesSlug
	PayoutsResource = model.PayoutsSlug
)

// RegisterLiveResources declares the share list channels.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(RulesResource, rbac.PermTenantEarningsRead)
	reg.Register(LinesResource, rbac.PermTenantEarningsRead)
	reg.Register(PayoutsResource, rbac.PermTenantEarningsRead)
}
