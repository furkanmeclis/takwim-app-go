// Package finance is the income / expense module of a business
// (/v1/tenant/finance, TEC-915): income and expense categories, records of
// money in or out outside sales (each posting its cash movement), their
// documents, reversals and corrections, the combined list in which sale
// lines appear as automatic income, its export and the period summary.
package finance

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	financehandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// EngineHandlers are the shared engine handlers the module mounts its
// export route on. A nil member skips the route.
type EngineHandlers struct {
	ExportEntries http.HandlerFunc
}

// RegisterRoutes mounts /v1/tenant/finance. A nil resolver (tenancy off)
// mounts nothing. Reading takes tenant.finance.read; writing
// tenant.finance.write; the export tenant.report.export; streaming a
// document tenant.document.download.
func RegisterRoutes(
	mux *http.ServeMux,
	h *financehandler.Handler,
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
	module := middleware.RequireModule(sector.ModuleFinance)
	perm := middleware.RequireTenantPermission
	read := perm(rbac.PermTenantFinanceRead)
	write := perm(rbac.PermTenantFinanceWrite)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org, module}, guards...)...))
	}

	route("GET /v1/tenant/finance/categories", h.ListCategories, read)
	route("POST /v1/tenant/finance/categories", h.CreateCategory, read, write)
	route("GET /v1/tenant/finance/categories/{categoryUuid}", h.GetCategory, read)
	route("PATCH /v1/tenant/finance/categories/{categoryUuid}", h.PatchCategory, read, write)
	route("DELETE /v1/tenant/finance/categories/{categoryUuid}", h.DeleteCategory, read, write)

	// Static segments before /{entryUuid}.
	route("GET /v1/tenant/finance/entries/meta", h.EntriesMeta, read)
	route("GET /v1/tenant/finance/entries/facets", h.Facets, read)
	route("GET /v1/tenant/finance/entries", h.ListEntries, read)
	route("POST /v1/tenant/finance/entries", h.CreateEntry, read, write)
	if engines.ExportEntries != nil {
		route("POST /v1/tenant/finance/entries/export", engines.ExportEntries, read, perm(rbac.PermTenantReportExport))
	}
	route("GET /v1/tenant/finance/entries/{entryUuid}", h.GetEntry, read)
	route("PATCH /v1/tenant/finance/entries/{entryUuid}", h.PatchEntry, read, write)
	route("POST /v1/tenant/finance/entries/{entryUuid}/reverse", h.ReverseEntry, read, write)
	route("POST /v1/tenant/finance/entries/{entryUuid}/correct", h.CorrectEntry, read, write)
	route("GET /v1/tenant/finance/entries/{entryUuid}/activity", h.EntryActivity, read)
	route("PUT /v1/tenant/finance/entries/{entryUuid}/document", h.UploadDocument, read, write)
	route("GET /v1/tenant/finance/entries/{entryUuid}/document", h.DownloadDocument, read, perm(rbac.PermTenantDocumentDownload))
	route("DELETE /v1/tenant/finance/entries/{entryUuid}/document", h.RemoveDocument, read, write)

	route("GET /v1/tenant/finance/summary", h.Summary, read)
}

// ExportQueryKeys are the list filters an export honours.
var ExportQueryKeys = []string{"source", "kind", "category", "branch", "staff", "account", "date_from", "date_to"}

// Engine slugs.
const (
	EntriesResource    = model.EntriesSlug
	CategoriesResource = model.CategoriesSlug
)

// RegisterLiveResources declares the record and category list channels.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(EntriesResource, rbac.PermTenantFinanceRead)
	reg.Register(CategoriesResource, rbac.PermTenantFinanceRead)
}
