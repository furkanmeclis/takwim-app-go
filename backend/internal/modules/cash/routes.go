// Package cash is the organization-scoped cash book (/v1/tenant/cash,
// TEC-914): cash, bank and online payment accounts per branch (or
// business-wide), opening balances, manual money in / out, transfers
// between accounts, reversals, the daily closing of a till and the daily
// summary. Later modules (payments, sales, income / expense, staff
// payouts) post into the same book through usecase.Service.PostTx.
//
// Bank statements and provider settlement reports (TEC-965) are imported
// as statements of a bank / online account, their lines reconciled with
// the book (matched, posted through PostTx, or ignored) and the period of
// the account closed against the stated balance.
package cash

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/adapters"
	cashhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// EngineHandlers are the shared engine handlers the module mounts its
// export / bulk routes on. Nil members skip those routes.
type EngineHandlers struct {
	ExportAccounts  http.HandlerFunc
	ExportMovements http.HandlerFunc
	BulkAccounts    http.HandlerFunc
	ExportLines     http.HandlerFunc
	ImportLines     http.HandlerFunc
	SampleLines     http.HandlerFunc
}

// RegisterRoutes mounts /v1/tenant/cash. A nil resolver (tenancy off)
// mounts nothing.
func RegisterRoutes(
	mux *http.ServeMux,
	h *cashhandler.Handler,
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
	module := middleware.RequireModule(sector.ModuleCash)
	perm := middleware.RequireTenantPermission
	read := perm(rbac.PermTenantCashRead)
	write := perm(rbac.PermTenantCashWrite)
	manage := perm(rbac.PermTenantCashManage)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org, module}, guards...)...))
	}

	// Accounts: static segments before /{uuid}.
	route("GET /v1/tenant/cash/accounts/meta", h.AccountsMeta, read)
	route("GET /v1/tenant/cash/accounts/facets", h.AccountFacets, read)
	route("GET /v1/tenant/cash/accounts/options", h.AccountOptions, read)
	route("GET /v1/tenant/cash/accounts", h.ListAccounts, read)
	route("POST /v1/tenant/cash/accounts", h.CreateAccount, read, manage)
	if engines.ExportAccounts != nil {
		route("POST /v1/tenant/cash/accounts/export", engines.ExportAccounts, read, perm(rbac.PermTenantCashExport))
	}
	if engines.BulkAccounts != nil {
		// The bulk handler checks the per-action permission in the scope.
		route("POST /v1/tenant/cash/accounts/bulk", engines.BulkAccounts, read)
	}
	route("GET /v1/tenant/cash/accounts/{uuid}", h.GetAccount, read)
	route("PATCH /v1/tenant/cash/accounts/{uuid}", h.PatchAccount, read, manage)
	route("GET /v1/tenant/cash/accounts/{uuid}/activity", h.AccountActivity, read)
	route("GET /v1/tenant/cash/accounts/{uuid}/movements", h.AccountMovements, read)
	route("POST /v1/tenant/cash/accounts/{uuid}/movements", h.CreateMovement, read, write)
	route("GET /v1/tenant/cash/accounts/{uuid}/closings", h.ListClosings, read)
	route("GET /v1/tenant/cash/accounts/{uuid}/closings/preview", h.PreviewClosing, read)
	route("POST /v1/tenant/cash/accounts/{uuid}/closings", h.CloseDay, read, perm(rbac.PermTenantCashClose))
	route("POST /v1/tenant/cash/accounts/{uuid}/closings/{closingUuid}/reopen", h.ReopenDay, read, perm(rbac.PermTenantCashReopen))

	// Movements (the cash book across accounts).
	route("GET /v1/tenant/cash/movements/meta", h.MovementsMeta, read)
	route("GET /v1/tenant/cash/movements/facets", h.MovementFacets, read)
	route("GET /v1/tenant/cash/movements", h.ListMovements, read)
	if engines.ExportMovements != nil {
		route("POST /v1/tenant/cash/movements/export", engines.ExportMovements, read, perm(rbac.PermTenantCashExport))
	}
	route("GET /v1/tenant/cash/movements/{uuid}", h.GetMovement, read)
	route("POST /v1/tenant/cash/movements/{uuid}/reverse", h.ReverseMovement, read, write)

	route("POST /v1/tenant/cash/transfers", h.Transfer, read, write)
	route("GET /v1/tenant/cash/daily-summary", h.DailySummary, read)

	// Statements (bank statement / provider settlement reconciliation).
	// Static segments and the lines sub-tree come before /{uuid}.
	imp := perm(rbac.PermTenantCashImport)
	route("GET /v1/tenant/cash/statements/meta", h.StatementsMeta, read)
	route("GET /v1/tenant/cash/statement-lines/meta", h.StatementLinesMeta, read)
	route("GET /v1/tenant/cash/statement-lines", h.ListLines, read)
	if engines.ExportLines != nil {
		route("POST /v1/tenant/cash/statement-lines/export", engines.ExportLines, read, perm(rbac.PermTenantCashExport))
	}
	if engines.ImportLines != nil {
		route("POST /v1/tenant/cash/statement-lines/import", engines.ImportLines, read, imp)
	}
	if engines.SampleLines != nil {
		route("GET /v1/tenant/cash/statement-lines/import/sample", engines.SampleLines, read, imp)
	}
	route("GET /v1/tenant/cash/statement-lines/{lineUuid}", h.GetLine, read)
	route("GET /v1/tenant/cash/statement-lines/{lineUuid}/candidates", h.LineCandidates, read)
	route("POST /v1/tenant/cash/statement-lines/{lineUuid}/match", h.MatchLine, read, write)
	route("POST /v1/tenant/cash/statement-lines/{lineUuid}/reset", h.ResetLine, read, write)
	route("POST /v1/tenant/cash/statement-lines/{lineUuid}/post", h.PostLine, read, write)
	route("POST /v1/tenant/cash/statement-lines/{lineUuid}/ignore", h.IgnoreLine, read, write)
	route("POST /v1/tenant/cash/statement-lines/{lineUuid}/reverse", h.ReverseLine, read, write)
	route("GET /v1/tenant/cash/statements", h.ListStatements, read)
	route("POST /v1/tenant/cash/statements", h.CreateStatement, read, imp)
	route("GET /v1/tenant/cash/statements/{uuid}", h.GetStatement, read)
	route("PATCH /v1/tenant/cash/statements/{uuid}", h.PatchStatement, read, imp)
	route("DELETE /v1/tenant/cash/statements/{uuid}", h.DeleteStatement, read, imp)
	route("GET /v1/tenant/cash/statements/{uuid}/summary", h.StatementSummary, read)
	route("GET /v1/tenant/cash/statements/{uuid}/lines", h.StatementLines, read)
	route("POST /v1/tenant/cash/statements/{uuid}/lines", h.AddLine, read, imp)
	route("GET /v1/tenant/cash/statements/{uuid}/lines/facets", h.StatementLineFacets, read)
	route("POST /v1/tenant/cash/statements/{uuid}/auto-match", h.AutoMatch, read, write)
}

// ExportAccountQueryKeys / ExportMovementQueryKeys are the list filters an
// export honours.
var (
	ExportAccountQueryKeys  = []string{"branch", "kind", "status"}
	ExportMovementQueryKeys = []string{"account", "kind", "direction", "date_from", "date_to"}
	ExportLineQueryKeys     = []string{"statement", "status", "line_type", "direction"}
)

// Engine slugs.
const (
	AccountsResource       = adapters.AccountsResource
	MovementsResource      = adapters.MovementsResource
	StatementsResource     = adapters.StatementsResource
	StatementLinesResource = adapters.StatementLinesResource
)

// RegisterLiveResources declares the account, movement, statement and
// statement line list channels (org:{org_uuid}:resource:tenant.cash_*).
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(AccountsResource, rbac.PermTenantCashRead)
	reg.Register(MovementsResource, rbac.PermTenantCashRead)
	reg.Register(StatementsResource, rbac.PermTenantCashRead)
	reg.Register(StatementLinesResource, rbac.PermTenantCashRead)
}
