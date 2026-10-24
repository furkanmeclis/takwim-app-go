// Package sales is the sale lines and customer ledger of a business
// (/v1/tenant/sales, TEC-916). A completed visit and a stock sale become
// sale lines (revenue) in the transaction that completes / sells them;
// money taken and paid back is posted to the ledger by the payment and
// refund flows through usecase.Service (…Tx methods). Revenue,
// collections, transfers and staff payouts are reported apart.
package sales

import (
	"context"
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	productsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/usecase"
	saleshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/model"
	salesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// RegisterRoutes mounts /v1/tenant/sales. A nil resolver (tenancy off)
// mounts nothing. Reading takes tenant.finance.read; reversing and
// correcting tenant.finance.write.
func RegisterRoutes(
	mux *http.ServeMux,
	h *saleshandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	module := middleware.RequireModule(sector.ModuleFinance)
	read := middleware.RequireTenantPermission(rbac.PermTenantFinanceRead)
	write := middleware.RequireTenantPermission(rbac.PermTenantFinanceWrite)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org, module}, guards...)...))
	}

	// Static segments before /{uuid}.
	route("GET /v1/tenant/sales/lines/meta", h.LinesMeta, read)
	route("GET /v1/tenant/sales/lines", h.ListLines, read)
	route("GET /v1/tenant/sales/lines/{uuid}", h.GetLine, read)
	route("POST /v1/tenant/sales/lines/{uuid}/reverse", h.ReverseLine, read, write)
	route("POST /v1/tenant/sales/lines/{uuid}/correct", h.CorrectLine, read, write)

	route("GET /v1/tenant/sales/entries/meta", h.EntriesMeta, read)
	route("GET /v1/tenant/sales/entries", h.ListEntries, read)
	route("GET /v1/tenant/sales/entries/{uuid}", h.GetEntry, read)
	route("POST /v1/tenant/sales/entries/{uuid}/reverse", h.ReverseEntry, read, write)

	route("GET /v1/tenant/sales/summary", h.Summary, read)
	route("GET /v1/tenant/sales/reservations/{uuid}/balance", h.ReservationBalance, read)
	route("GET /v1/tenant/sales/customers/{uuid}/balance", h.CustomerBalance, read)
}

// Engine slugs.
const (
	LinesResource   = model.LinesSlug
	EntriesResource = model.EntriesSlug
)

// RegisterLiveResources declares the sale line and ledger list channels
// (org:{org_uuid}:resource:tenant.sale_lines / tenant.ledger_entries).
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(LinesResource, rbac.PermTenantFinanceRead)
	reg.Register(EntriesResource, rbac.PermTenantFinanceRead)
}

// ProductSaleRecorder adapts the service to the stock flow: every stock
// sale becomes a product sale line in the movement's transaction.
func ProductSaleRecorder(svc *salesusecase.Service) productsusecase.SaleRecorder {
	return func(ctx context.Context, q *db.Queries, orgID int64, sale productsusecase.StockSale) error {
		_, err := svc.RecordProductSaleTx(ctx, q, orgID, salesusecase.ProductSale{
			MovementUUID: sale.MovementUUID, BranchID: sale.BranchID, ProductID: sale.ProductID,
			VariantID: sale.VariantID, StaffID: sale.StaffID, Description: sale.Description,
			Quantity: sale.Quantity, UnitPrice: sale.UnitPrice, OccurredAt: sale.OccurredAt, CreatedBy: sale.CreatedBy,
		})
		return err
	}
}
