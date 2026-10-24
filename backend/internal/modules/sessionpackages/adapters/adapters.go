// Package adapters connects the session packages to the shared engines
// (TEC-984): ioengine (export of the sold packages; nothing is imported, a
// package is sold with its payment), bulkengine (activate / deactivate of
// the definitions) and searchengine (the sold packages in Cmd+K). The
// organization and the branch scope always come from the context
// (orgctx); without an organization every adapter call fails.
package adapters

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sessionpackages/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sessionpackages/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/phone"
)

// Engine slugs.
const (
	PackagesResource         = model.PackagesSlug
	CustomerPackagesResource = model.CustomerPackagesSlug
)

// MaxBulkTargets caps the "all matching" bulk scope.
const MaxBulkTargets = 10000

// MaxExportRows caps one export.
const MaxExportRows = 50000

// ExportQueryKeys are the list query keys an export keeps.
var ExportQueryKeys = []string{"q", "sort", "state", "package", "branch", "customer", "date_from", "date_to"}

var (
	errNoOrganization = errors.New("session packages: organization scope is required")
	errImport         = errors.New("sold packages are not imported")
)

func organizationID(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return 0, errNoOrganization
	}
	return id, nil
}

func uuids(raw string) []uuid.UUID {
	var out []uuid.UUID
	for _, part := range apiquery.SplitCSV(raw) {
		if id, err := uuid.Parse(part); err == nil {
			out = append(out, id)
		}
	}
	return out
}

func date(raw string) *time.Time {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(raw))
	if err != nil {
		return nil
	}
	return &t
}

// CustomerPackageFilter maps the sold package list query (q, sort, state,
// package, branch, customer, date_from, date_to) to a filter; unknown
// values are dropped.
func CustomerPackageFilter(query map[string]string) model.CustomerPackageFilter {
	f := model.CustomerPackageFilter{
		Q: strings.TrimSpace(query["q"]), Branches: uuids(query["branch"]), Packages: uuids(query["package"]),
		DateFrom: date(query["date_from"]), DateTo: date(query["date_to"]),
	}
	for _, s := range apiquery.SplitCSV(query["state"]) {
		if slices.Contains(model.States, s) && !slices.Contains(f.States, s) {
			f.States = append(f.States, s)
		}
	}
	if c := uuids(query["customer"]); len(c) > 0 {
		f.Customer = &c[0]
	}
	if sort := strings.TrimSpace(query["sort"]); sort != "" {
		f.SortField, f.SortDesc = strings.TrimPrefix(sort, "-"), strings.HasPrefix(sort, "-")
	}
	return f
}

// PackageFilter maps the definition list query (q, is_active, service,
// branch) to a filter.
func PackageFilter(query map[string]string) model.PackageFilter {
	f := model.PackageFilter{Q: strings.TrimSpace(query["q"]), Services: uuids(query["service"])}
	if b := uuids(query["branch"]); len(b) > 0 {
		f.Branch = &b[0]
	}
	if v, err := strconv.ParseBool(strings.TrimSpace(query["is_active"])); err == nil {
		f.IsActive = &v
	}
	return f
}

// ---------------------------------------------------------------- io engine

// IO exports the sold packages with their balance (export only).
type IO struct {
	svc *usecase.Service
}

// NewIO builds the io adapter.
func NewIO(svc *usecase.Service) *IO {
	return &IO{svc: svc}
}

func (a *IO) Resource() string { return CustomerPackagesResource }

func (a *IO) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "customer", LabelKey: "customer_package.customer", Type: ioengine.ColumnTypeString},
		{Key: "customer_phone", LabelKey: "customer_package.customer_phone", Type: ioengine.ColumnTypeString},
		{Key: "name", LabelKey: "customer_package.name", Type: ioengine.ColumnTypeString},
		{Key: "service", LabelKey: "customer_package.service", Type: ioengine.ColumnTypeString},
		{Key: "branch", LabelKey: "customer_package.branch", Type: ioengine.ColumnTypeString},
		{Key: "total_sessions", LabelKey: "customer_package.total_sessions", Type: ioengine.ColumnTypeString},
		{Key: "used", LabelKey: "customer_package.used", Type: ioengine.ColumnTypeString},
		{Key: "held", LabelKey: "customer_package.held", Type: ioengine.ColumnTypeString},
		{Key: "remaining", LabelKey: "customer_package.remaining", Type: ioengine.ColumnTypeString},
		{Key: "price", LabelKey: "customer_package.price", Type: ioengine.ColumnTypeString},
		{Key: "used_amount", LabelKey: "customer_package.used_amount", Type: ioengine.ColumnTypeString},
		{Key: "remaining_amount", LabelKey: "customer_package.remaining_amount", Type: ioengine.ColumnTypeString},
		{Key: "currency", LabelKey: "customer_package.currency", Type: ioengine.ColumnTypeString},
		{Key: "sold_on", LabelKey: "customer_package.sold_on", Type: ioengine.ColumnTypeString},
		{Key: "expires_on", LabelKey: "customer_package.expires_on", Type: ioengine.ColumnTypeString},
		{Key: "state", LabelKey: "customer_package.state", Type: ioengine.ColumnTypeEnum},
		{Key: "refund_amount", LabelKey: "customer_package.refund_amount", Type: ioengine.ColumnTypeString},
		{Key: "staff", LabelKey: "customer_package.staff", Type: ioengine.ColumnTypeString},
		{Key: "uuid", LabelKey: "customer_package.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "created_at", LabelKey: "customer_package.created_at", Type: ioengine.ColumnTypeDatetime},
	}
}

func (a *IO) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, err := a.svc.ExportCustomerPackages(ctx, orgID, CustomerPackageFilter(query), MaxExportRows)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, p := range rows {
		row := map[string]any{
			"uuid": p.UUID.String(), "customer": p.Customer.Name, "customer_phone": "", "name": p.Name,
			"service": p.ServiceName, "branch": p.Branch.Name, "total_sessions": p.TotalSessions, "used": p.Used,
			"held": p.Held, "remaining": p.Remaining, "price": p.Price.String(), "used_amount": p.UsedAmount.String(),
			"remaining_amount": p.RemainingAmount.String(), "currency": p.Currency, "sold_on": p.SoldOn,
			"expires_on": "", "state": p.State, "refund_amount": "", "staff": "", "created_at": p.CreatedAt,
		}
		if p.Customer.Phone != nil {
			row["customer_phone"] = *p.Customer.Phone
		}
		if p.ExpiresOn != nil {
			row["expires_on"] = *p.ExpiresOn
		}
		if p.Refund != nil {
			row["refund_amount"] = p.Refund.Amount.String()
		}
		if p.Staff != nil {
			row["staff"] = p.Staff.Name
		}
		out = append(out, row)
	}
	return ioengine.Dataset{Resource: CustomerPackagesResource, Columns: a.ExportColumns(), Rows: out}, nil
}

// ImportSchema: nothing is imported (a sale moves money).
func (a *IO) ImportSchema() []ioengine.ImportField { return nil }

func (a *IO) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errImport
}

func (a *IO) RevertRow(context.Context, string, string, map[string]any) error {
	return errImport
}

// -------------------------------------------------------------- bulk engine

// Bulk activates / deactivates package definitions.
type Bulk struct {
	svc *usecase.Service
}

// NewBulk builds the bulk adapter. svc may be nil for metadata only.
func NewBulk(svc *usecase.Service) *Bulk {
	return &Bulk{svc: svc}
}

func (a *Bulk) Resource() string { return PackagesResource }

func (a *Bulk) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{
		{
			ID: "activate", LabelKey: "bulk.actions.session_packages.activate",
			Permission: rbac.PermTenantSessionPackagesBulkActivate, Reversible: true,
		},
		{
			ID: "deactivate", LabelKey: "bulk.actions.session_packages.deactivate",
			Permission: rbac.PermTenantSessionPackagesBulkDeactivate, Reversible: true,
			ConfirmKey: "bulk.confirm.session_packages.deactivate",
		},
	}
}

func (a *Bulk) ResolveTargets(ctx context.Context, _ string, target bulkengine.BulkTarget) ([]string, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return nil, err
	}
	switch target.Scope {
	case "ids":
		seen := map[string]bool{}
		out := make([]string, 0, len(target.IDs))
		for _, raw := range target.IDs {
			id, err := uuid.Parse(strings.TrimSpace(raw))
			if err != nil || seen[id.String()] {
				continue
			}
			seen[id.String()] = true
			out = append(out, id.String())
		}
		if len(out) > MaxBulkTargets {
			return nil, fmt.Errorf("at most %d targets are allowed", MaxBulkTargets)
		}
		return out, nil
	case "query":
		ids, err := a.svc.PackageUUIDs(ctx, orgID, PackageFilter(target.Query), MaxBulkTargets)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(ids))
		for _, id := range ids {
			out = append(out, id.String())
		}
		return out, nil
	default:
		return nil, fmt.Errorf("invalid target scope")
	}
}

func (a *Bulk) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "invalid uuid"}, nil
	}
	var active bool
	switch action {
	case "activate":
		active = true
	case "deactivate":
		active = false
	default:
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "unknown action"}, nil
	}
	prev, err := a.svc.SetPackageActive(ctx, orgID, id, active)
	if err != nil {
		if errors.Is(err, usecase.ErrPackageNotFound) || errors.Is(err, usecase.ErrAllBranches) {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: err.Error()}, nil
		}
		return bulkengine.BulkItemResult{}, err
	}
	return bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: "session_package", OK: true, Op: "update",
		Previous: map[string]any{"is_active": prev},
	}, nil
}

func (a *Bulk) RevertItem(ctx context.Context, _ string, entityUUID string, previous map[string]any) error {
	orgID, err := organizationID(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	prev, ok := previous["is_active"].(bool)
	if !ok {
		return fmt.Errorf("missing previous is_active")
	}
	_, err = a.svc.SetPackageActive(ctx, orgID, id, prev)
	return err
}

// ------------------------------------------------------------ search engine

// Search indexes the sold packages for the tenant Cmd+K palette: the
// customer and the package name, with the customer's phone as keyword.
type Search struct {
	q *db.Queries
}

// NewSearch builds the search adapter.
func NewSearch(q *db.Queries) *Search {
	return &Search{q: q}
}

func (a *Search) Spec() searchengine.Spec {
	return searchengine.Spec{
		ID:           usecase.SearchSpec,
		LabelKey:     "search.specs_customer_packages",
		Permission:   rbac.PermTenantSessionPackagesRead,
		Icon:         "packages",
		Searchable:   []string{"title", "subtitle", "keywords"},
		Scope:        searchengine.ScopeTenant,
		BranchScoped: true,
	}
}

// ListAll walks every live organization (each query is organization-scoped).
func (a *Search) ListAll(ctx context.Context) ([]searchengine.Document, error) {
	orgIDs, err := a.q.ListLiveOrganizationIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]searchengine.Document, 0)
	for _, orgID := range orgIDs {
		rows, err := a.q.ListCustomerPackageSearchDocuments(ctx, orgID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			out = append(out, SearchDocument(db.GetCustomerPackageSearchDocumentRow(row)))
		}
	}
	return out, nil
}

// Document loads one sold package by its tenant document id.
func (a *Search) Document(ctx context.Context, docID string) (searchengine.Document, error) {
	orgID, raw, err := searchengine.ParseTenantDocumentID(docID)
	if err != nil {
		return searchengine.Document{}, err
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("customer packages search: invalid uuid")
	}
	row, err := a.q.GetCustomerPackageSearchDocument(ctx, db.GetCustomerPackageSearchDocumentParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return searchengine.Document{}, err
	}
	return SearchDocument(row), nil
}

// SearchDocument is "customer · package" with the service and the sale
// day; the branch that sold it scopes the document.
func SearchDocument(row db.GetCustomerPackageSearchDocumentRow) searchengine.Document {
	name := strings.TrimSpace(row.CustomerFirstName + " " + row.CustomerLastName)
	keywords := []string{name, row.Name, row.ServiceName}
	if row.CustomerPhone.Valid {
		display := phone.Display(row.CustomerPhone.String, "")
		keywords = append(keywords, row.CustomerPhone.String, strings.ReplaceAll(display, " ", ""))
	}
	return searchengine.Document{
		ID:             searchengine.TenantDocumentID(row.OrganizationID, row.Uuid.String()),
		Spec:           usecase.SearchSpec,
		Title:          name + " · " + row.Name,
		Subtitle:       row.ServiceName + " · " + row.SoldOn.Time.Format("02.01.2006"),
		Keywords:       keywords,
		Href:           "/t/" + row.OrganizationSlug + "/packages/" + row.Uuid.String(),
		Icon:           "packages",
		OrganizationID: row.OrganizationID,
		BranchIDs:      []int64{row.BranchID},
	}
}

var (
	_ ioengine.ResourceAdapter = (*IO)(nil)
	_ bulkengine.BulkAdapter   = (*Bulk)(nil)
	_ searchengine.Adapter     = (*Search)(nil)
)
