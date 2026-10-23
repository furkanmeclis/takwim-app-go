// Package adapters connects reservations to the shared engines (TEC-963):
// ioengine (export only: a booking is never written without the capacity
// engine), bulkengine (accept / reject / cancel, each record through its own
// transition) and searchengine (Cmd+K by number, customer name and phone).
//
// Every call works inside the caller's reach, exactly like the panel list:
// the organization and branch scope come from the context (orgctx), and a
// member holding only tenant.reservations.own reaches the bookings it
// created or serves. The acting user is the request principal or, in a
// worker, the job's requester (orgctx.JobActor).
package adapters

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/phone"
)

// Resource is the engine slug of reservations.
const Resource = model.Slug

// MaxBulkTargets caps the "all matching" bulk scope.
const MaxBulkTargets = 10000

// MaxExportRows caps one export.
const MaxExportRows = 50000

// MaxCancelReason is the longest cancel reason (the record's column).
const MaxCancelReason = 32

// DefaultTimezone is used when the scope carries no business timezone.
const DefaultTimezone = "Europe/Istanbul"

var (
	errNoOrganization = errors.New("reservations: organization scope is required")
	errForbidden      = errors.New("reservations: missing permission")
)

// reach resolves the organization, the caller and the acting user for a
// full / own slug pair, like the panel handlers do.
func reach(ctx context.Context, full string) (int64, availabilityusecase.Caller, int64, error) {
	scope, ok := orgctx.From(ctx)
	if !ok || scope.ID <= 0 {
		return 0, availabilityusecase.Caller{}, 0, errNoOrganization
	}
	var actor int64
	if p, ok := authctx.PrincipalFrom(ctx); ok {
		actor = p.UserInternal
	} else if id, ok := orgctx.JobActor(ctx); ok {
		actor = id
	}
	switch scope.Reach(full, rbac.PermTenantReservationsOwn) {
	case orgctx.ReachAll:
		return scope.ID, availabilityusecase.Caller{}, actor, nil
	case orgctx.ReachOwn:
		if actor <= 0 {
			return 0, availabilityusecase.Caller{}, 0, errForbidden
		}
		return scope.ID, availabilityusecase.Caller{OwnUserID: actor}, actor, nil
	}
	return 0, availabilityusecase.Caller{}, 0, errForbidden
}

// FilterFromQuery maps the list query (q, sort and the list filters) to a
// list filter; an invalid filter is an error.
func FilterFromQuery(query map[string]string) (model.ListFilter, error) {
	values := url.Values{}
	for k, v := range query {
		if strings.TrimSpace(v) != "" {
			values.Set(k, v)
		}
	}
	f, details := handler.ParseListFilter(values)
	if len(details) > 0 {
		return f, fmt.Errorf("%s: %s", details[0].Field, details[0].Message)
	}
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns(usecase.SortColumns)); err != nil {
		return f, err
	}
	f.Q = q.Q
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	return f, nil
}

func location(ctx context.Context) *time.Location {
	name := DefaultTimezone
	if s, ok := orgctx.From(ctx); ok && s.Timezone != "" {
		name = s.Timezone
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

// ---------------------------------------------------------------- io engine

// IO exports reservations; import is not supported.
type IO struct {
	svc *usecase.Service
}

// NewIO builds the io adapter.
func NewIO(svc *usecase.Service) *IO {
	return &IO{svc: svc}
}

func (a *IO) Resource() string { return Resource }

func (a *IO) ExportColumns() []ioengine.Column {
	col := func(key string, typ ioengine.ColumnType) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "reservation." + key, Type: typ}
	}
	return []ioengine.Column{
		col("number", ioengine.ColumnTypeString),
		col("date", ioengine.ColumnTypeString),
		col("starts_at", ioengine.ColumnTypeString),
		col("ends_at", ioengine.ColumnTypeString),
		col("customer", ioengine.ColumnTypeString),
		col("phone", ioengine.ColumnTypeString),
		col("services", ioengine.ColumnTypeString),
		col("staff", ioengine.ColumnTypeString),
		col("branch", ioengine.ColumnTypeString),
		col("status", ioengine.ColumnTypeEnum),
		col("approval_status", ioengine.ColumnTypeEnum),
		col("payment_status", ioengine.ColumnTypeEnum),
		col("source", ioengine.ColumnTypeEnum),
		col("total_price", ioengine.ColumnTypeString),
		col("currency", ioengine.ColumnTypeString),
		col("created_at", ioengine.ColumnTypeString),
		col("uuid", ioengine.ColumnTypeUUID),
	}
}

// Export writes the rows the caller sees in the list with the same filters.
// Times are in the business timezone; states carry their tr / en labels.
func (a *IO) Export(ctx context.Context, query ioengine.ExportQuery, locale i18n.Locale) (ioengine.Dataset, error) {
	orgID, caller, _, err := reach(ctx, rbac.PermTenantReservationsRead)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f, err := FilterFromQuery(query)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f.Limit, f.Offset = MaxExportRows, 0
	items, _, err := a.svc.List(ctx, orgID, f, caller)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	loc := location(ctx)
	label := func(group, value string) string {
		return i18n.Translate(locale, "reservation."+group+"."+value)
	}
	out := make([]map[string]any, 0, len(items))
	for _, r := range items {
		p := ""
		if r.Customer.Phone != nil {
			p = phone.Display(*r.Customer.Phone, "")
		}
		start, end := r.StartsAt.In(loc), r.EndsAt.In(loc)
		out = append(out, map[string]any{
			"number": strconv.FormatInt(r.Number, 10), "date": start.Format("2006-01-02"),
			"starts_at": start.Format("15:04"), "ends_at": end.Format("15:04"),
			"customer": r.Customer.Name, "phone": p, "services": r.Services, "staff": r.Staff, "branch": r.Branch.Name,
			"status": label("status", r.Status), "approval_status": label("approval", r.ApprovalStatus),
			"payment_status": label("payment", r.PaymentStatus), "source": label("source", r.Source),
			"total_price": r.TotalPrice.String(), "currency": r.Currency,
			"created_at": r.CreatedAt.In(loc).Format("2006-01-02 15:04"), "uuid": r.UUID.String(),
		})
	}
	return ioengine.Dataset{Resource: Resource, Columns: a.ExportColumns(), Rows: out}, nil
}

// ImportSchema is empty: reservations are not imported.
func (a *IO) ImportSchema() []ioengine.ImportField { return nil }

func (a *IO) ApplyRow(_ context.Context, _ map[string]any, _ map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{OK: false, Error: "import not supported"}, nil
}

func (a *IO) RevertRow(_ context.Context, _, _ string, _ map[string]any) error {
	return errors.New("import not supported")
}

// -------------------------------------------------------------- bulk engine

// Bulk accepts, rejects and cancels reservations. Each record goes through
// its own transition (capacity confirmed or released in the same
// transaction); records the action does not apply to are skipped and
// reported. There is no rollback: a released time may be booked again.
type Bulk struct {
	svc *usecase.Service
}

// NewBulk builds the bulk adapter. svc may be nil for metadata only.
func NewBulk(svc *usecase.Service) *Bulk {
	return &Bulk{svc: svc}
}

func (a *Bulk) Resource() string { return Resource }

// BulkActions are declared with the table contract (handler.BulkActions).
func (a *Bulk) BulkActions() []bulkengine.BulkActionDef {
	return handler.BulkActions()
}

// ParseVersions reads params["versions"].
func ParseVersions(raw string) (map[uuid.UUID]int, error) {
	out := map[uuid.UUID]int{}
	for _, part := range apiquery.SplitCSV(raw) {
		id, ver, ok := strings.Cut(part, ":")
		if !ok {
			return nil, fmt.Errorf("params.versions must be uuid:version pairs")
		}
		u, err := uuid.Parse(strings.TrimSpace(id))
		if err != nil {
			return nil, fmt.Errorf("params.versions must be uuid:version pairs")
		}
		v, err := strconv.Atoi(strings.TrimSpace(ver))
		if err != nil || v < 0 {
			return nil, fmt.Errorf("params.versions must be uuid:version pairs")
		}
		out[u] = v
	}
	return out, nil
}

func (a *Bulk) ResolveTargets(ctx context.Context, action string, target bulkengine.BulkTarget) ([]string, error) {
	orgID, caller, _, err := reach(ctx, rbac.PermTenantReservationsWrite)
	if err != nil {
		return nil, err
	}
	if action == "cancel" && len(strings.TrimSpace(target.Params["reason"])) > MaxCancelReason {
		return nil, fmt.Errorf("params.reason is at most %d characters", MaxCancelReason)
	}
	if _, err := ParseVersions(target.Params["versions"]); err != nil {
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
		f, err := FilterFromQuery(target.Query)
		if err != nil {
			return nil, err
		}
		f.Limit, f.Offset = MaxBulkTargets, 0
		items, _, err := a.svc.List(ctx, orgID, f, caller)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(items))
		for _, r := range items {
			out = append(out, r.UUID.String())
		}
		return out, nil
	default:
		return nil, fmt.Errorf("invalid target scope")
	}
}

func (a *Bulk) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	orgID, caller, actor, err := reach(ctx, rbac.PermTenantReservationsWrite)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "not_found"}, nil
	}
	// The record as the caller sees it: its number for the report, and
	// nothing at all outside the caller's reach.
	current, err := a.svc.Get(ctx, orgID, id, caller)
	if errors.Is(err, usecase.ErrNotFound) {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "not_found"}, nil
	}
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	res := bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: "reservation", Label: "#" + strconv.FormatInt(current.Number, 10), Op: "update",
	}
	params := bulkengine.Params(ctx)
	in := usecase.ChangeInput{}
	if versions, err := ParseVersions(params["versions"]); err == nil {
		if v, ok := versions[id]; ok {
			in.ExpectedVersion = &v
		}
	}
	switch action {
	case "accept":
		_, err = a.svc.Accept(ctx, orgID, id, caller, actor, in)
	case "reject":
		_, err = a.svc.Reject(ctx, orgID, id, caller, actor, in)
	case "cancel":
		in.Reason = strings.TrimSpace(params["reason"])
		_, err = a.svc.Cancel(ctx, orgID, id, caller, actor, in)
	default:
		res.Error = "unknown_action"
		return res, nil
	}
	var ve *usecase.ValidationError
	switch {
	case err == nil:
		res.OK = true
	case errors.Is(err, usecase.ErrInvalidTransition):
		res.Skipped, res.Error = true, "invalid_transition"
	case errors.Is(err, usecase.ErrStaleVersion):
		res.Skipped, res.Error = true, "stale"
	case errors.Is(err, availabilityusecase.ErrHoldExpired):
		res.Skipped, res.Error = true, "expired"
	case errors.Is(err, usecase.ErrNotFound):
		res.Error = "not_found"
	case errors.As(err, &ve):
		res.Error = ve.Error()
	default:
		return bulkengine.BulkItemResult{}, err
	}
	return res, nil
}

// RevertItem: bulk transitions are not reversible.
func (a *Bulk) RevertItem(context.Context, string, string, map[string]any) error {
	return errors.New("reservation bulk actions cannot be rolled back")
}

// ------------------------------------------------------------ search engine

// Search indexes reservations for the tenant Cmd+K palette.
type Search struct {
	q *db.Queries
}

// NewSearch builds the search adapter.
func NewSearch(q *db.Queries) *Search {
	return &Search{q: q}
}

func (a *Search) Spec() searchengine.Spec {
	return searchengine.Spec{
		ID:            usecase.SearchSpec,
		LabelKey:      "search.specs_reservations",
		Permission:    rbac.PermTenantReservationsRead,
		OwnPermission: rbac.PermTenantReservationsOwn,
		Icon:          "reservations",
		Searchable:    []string{"title", "subtitle", "keywords"},
		Scope:         searchengine.ScopeTenant,
		BranchScoped:  true,
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
		rows, err := a.q.ListReservationSearchDocuments(ctx, orgID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			out = append(out, SearchDocument(db.GetReservationSearchDocumentRow(row)))
		}
	}
	return out, nil
}

// Document loads one reservation by its tenant document id.
func (a *Search) Document(ctx context.Context, docID string) (searchengine.Document, error) {
	orgID, raw, err := searchengine.ParseTenantDocumentID(docID)
	if err != nil {
		return searchengine.Document{}, err
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("reservations search: invalid uuid")
	}
	row, err := a.q.GetReservationSearchDocument(ctx, db.GetReservationSearchDocumentParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return searchengine.Document{}, err
	}
	return SearchDocument(row), nil
}

// SearchDocument is the Cmd+K document of a reservation: "#number ·
// customer" with the visit time (business timezone) and branch; keywords
// hold the number and the phone with and without the country code.
func SearchDocument(row db.GetReservationSearchDocumentRow) searchengine.Document {
	number := strconv.FormatInt(row.Number, 10)
	name := strings.TrimSpace(row.CustomerFirstName + " " + row.CustomerLastName)
	loc, err := time.LoadLocation(row.Timezone)
	if err != nil {
		loc = time.UTC
	}
	subtitle := row.StartsAt.Time.In(loc).Format("02.01.2006 15:04") + " · " + row.BranchName
	keywords := []string{number, "#" + number, name}
	if row.CustomerPhone.Valid {
		display := phone.Display(row.CustomerPhone.String, "")
		keywords = append(keywords, row.CustomerPhone.String, strings.ReplaceAll(display, " ", ""))
	}
	return searchengine.Document{
		ID:             searchengine.TenantDocumentID(row.OrganizationID, row.Uuid.String()),
		Spec:           usecase.SearchSpec,
		Title:          "#" + number + " · " + name,
		Subtitle:       subtitle,
		Keywords:       keywords,
		Href:           "/t/" + row.OrganizationSlug + "/reservations/" + row.Uuid.String(),
		Icon:           "reservations",
		OrganizationID: row.OrganizationID,
		BranchIDs:      []int64{row.BranchID},
		OwnerUserIDs:   row.OwnerUserIds,
	}
}

var (
	_ ioengine.ResourceAdapter = (*IO)(nil)
	_ bulkengine.BulkAdapter   = (*Bulk)(nil)
	_ searchengine.Adapter     = (*Search)(nil)
)
