// Package usecase holds the service catalog rules. Every method takes the
// organization id of the caller explicitly (from orgctx.Scope in handlers,
// from the job in workers).
//
// The catalog (categories, services, staff links, resource kinds) is
// business-level: every member with tenant.services.read sees all of it.
// Branch offers are branch-level: a member limited to some branches only
// changes the offers of those branches and cannot name another branch.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// SearchSpec is the Cmd+K spec id of services.
const SearchSpec = "services"

// Limits of the service fields.
const (
	minDuration       = 5
	maxDuration       = 1440
	maxBuffer         = 480
	maxSortOrder      = 100000
	maxNameLength     = 160
	maxCategoryName   = 120
	maxDescriptionLen = 2000
	maxLinks          = 500
)

var (
	ErrNotFound          = errors.New("service not found")
	ErrCategoryNotFound  = errors.New("service category not found")
	ErrNameTaken         = errors.New("the catalog already has a service with this name")
	ErrCategoryNameTaken = errors.New("a category with this name already exists")
)

// Detail is one field-level validation problem.
type Detail struct {
	Field   string
	Message string
	Code    string
}

// ValidationError carries field details; handlers answer 400 VALIDATION_ERROR.
type ValidationError struct {
	Details []Detail
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Details) == 0 {
		return "validation failed"
	}
	return e.Details[0].Field + ": " + e.Details[0].Message
}

func (e *ValidationError) add(field, code, msg string) {
	e.Details = append(e.Details, Detail{Field: field, Code: code, Message: msg})
}

func (e *ValidationError) orNil() error {
	if len(e.Details) == 0 {
		return nil
	}
	return e
}

// IsValidation reports whether err is a field validation error.
func IsValidation(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}

// SearchIndexer schedules Cmd+K index updates (fail-soft).
type SearchIndexer interface {
	EnqueueUpsert(ctx context.Context, spec, id string)
	EnqueueDelete(ctx context.Context, spec, id string)
}

// Service implements the service catalog use cases.
type Service struct {
	repo    *repository.Postgres
	indexer SearchIndexer
	storage storage.Driver
}

// New builds the service. indexer may be nil.
func New(repo *repository.Postgres, indexer SearchIndexer) *Service {
	return &Service{repo: repo, indexer: indexer}
}

// SetStorage attaches the object store of service images. Without it
// image uploads answer ErrStorageUnavailable.
func (s *Service) SetStorage(store storage.Driver) {
	s.storage = store
}

// SortColumns are the sortable list fields.
var SortColumns = map[string]string{
	"name": "name", "duration": "duration", "price": "price", "sort_order": "sort_order",
	"is_active": "is_active", "created_at": "created_at", "updated_at": "updated_at",
}

// listArgs are the shared filter arguments of the list queries.
type listArgs struct {
	categories, branches []int64
	uncategorized        pgtype.Bool
	paymentModes         []string
	isActive, showOnline pgtype.Bool
	q                    pgtype.Text
}

func (s *Service) listArgs(ctx context.Context, q *db.Queries, orgID int64, f model.ListFilter) (listArgs, error) {
	a := listArgs{
		paymentModes: nilIfEmpty(f.PaymentModes), isActive: boolArg(f.IsActive),
		showOnline: boolArg(f.ShowOnline), q: textArg(f.Q),
	}
	if f.Uncategorized {
		a.uncategorized = pgtype.Bool{Bool: true, Valid: true}
	}
	if len(f.Categories) > 0 {
		// Unknown categories match nothing (an empty, non-nil filter).
		a.categories = []int64{}
		rows, err := q.ListServiceCategories(ctx, orgID)
		if err != nil {
			return a, err
		}
		for _, row := range rows {
			if slices.Contains(f.Categories, row.Uuid) {
				a.categories = append(a.categories, row.ID)
			}
		}
	}
	if len(f.Branches) > 0 {
		rows, err := q.ListBranchIDsByUUIDs(ctx, db.ListBranchIDsByUUIDsParams{OrganizationID: orgID, Uuids: f.Branches})
		if err != nil {
			return a, err
		}
		a.branches = []int64{}
		for _, row := range rows {
			a.branches = append(a.branches, row.ID)
		}
	}
	return a, nil
}

// List returns one page of the catalog with each service's offers. The
// default order is category, sort order, name.
func (s *Service) List(ctx context.Context, orgID int64, f model.ListFilter) ([]model.Service, int64, error) {
	if _, ok := SortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "", false
	}
	q := s.repo.Q()
	a, err := s.listArgs(ctx, q, orgID, f)
	if err != nil {
		return nil, 0, err
	}
	rows, err := q.ListServices(ctx, db.ListServicesParams{
		OrganizationID: orgID, CategoryIds: a.categories, Uncategorized: a.uncategorized,
		FilterBranchIds: a.branches, PaymentModes: a.paymentModes, IsActive: a.isActive,
		ShowOnline: a.showOnline, Q: a.q, SortField: f.SortField, SortDesc: f.SortDesc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountServices(ctx, db.CountServicesParams{
		OrganizationID: orgID, CategoryIds: a.categories, Uncategorized: a.uncategorized,
		FilterBranchIds: a.branches, PaymentModes: a.paymentModes, IsActive: a.isActive,
		ShowOnline: a.showOnline, Q: a.q,
	})
	if err != nil {
		return nil, 0, err
	}
	conv := make([]db.GetServiceRow, 0, len(rows))
	for _, row := range rows {
		conv = append(conv, db.GetServiceRow(row))
	}
	out, err := s.mapRows(ctx, q, orgID, conv, false)
	return out, total, err
}

// Facets returns the categories, offering branches and payment modes in use.
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	q := s.repo.Q()
	out := model.Facets{Category: []model.Facet{}, Branch: []model.Facet{}, PaymentMode: []model.Facet{}}
	cats, err := q.ListServiceCategoryFacets(ctx, orgID)
	if err != nil {
		return out, err
	}
	for _, row := range cats {
		out.Category = append(out.Category, model.Facet{Value: row.Uuid.String(), Label: row.Name, Count: row.Count})
	}
	none, err := q.CountUncategorizedServices(ctx, orgID)
	if err != nil {
		return out, err
	}
	if none > 0 {
		out.Category = append(out.Category, model.Facet{Value: model.UncategorizedFacet, Count: none})
	}
	branches, err := q.ListServiceBranchFacets(ctx, db.ListServiceBranchFacetsParams{OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx)})
	if err != nil {
		return out, err
	}
	for _, row := range branches {
		out.Branch = append(out.Branch, model.Facet{Value: row.Uuid.String(), Label: row.Name, Count: row.Count})
	}
	modes, err := q.ListServicePaymentModeFacets(ctx, orgID)
	if err != nil {
		return out, err
	}
	for _, row := range modes {
		out.PaymentMode = append(out.PaymentMode, model.Facet{Value: row.Value, Count: row.Count})
	}
	return out, nil
}

// Get returns one service with its offers and staff links.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Service, error) {
	q := s.repo.Q()
	row, err := s.load(ctx, q, orgID, id)
	if err != nil {
		return model.Service{}, err
	}
	out, err := s.mapRows(ctx, q, orgID, []db.GetServiceRow{row}, true)
	if err != nil {
		return model.Service{}, err
	}
	return out[0], nil
}

// Create validates and stores a new service, offered in BranchUUIDs at the
// catalog price.
func (s *Service) Create(ctx context.Context, orgID int64, in model.Input) (model.Service, error) {
	f := fields{
		Price: money.Zero(), PaymentMode: "on_site", StaffMode: defaultStaffMode(ctx),
		ResourceKinds: []string{}, ShowOnline: true, IsActive: true,
	}
	ve := &ValidationError{}
	q := s.repo.Q()
	if err := s.resolveCategory(ctx, q, orgID, in.CategoryUUID, &f, ve); err != nil {
		return model.Service{}, err
	}
	f.apply(ve, in, true)
	branchIDs, err := s.resolveOfferBranches(ctx, q, orgID, in.BranchUUIDs, ve)
	if err != nil {
		return model.Service{}, err
	}
	if err := ve.orNil(); err != nil {
		return model.Service{}, err
	}
	if err := s.checkName(ctx, q, orgID, f.Name, nil); err != nil {
		return model.Service{}, err
	}
	var created db.Service
	err = s.repo.InTx(ctx, func(tx *db.Queries) error {
		row, err := tx.CreateService(ctx, db.CreateServiceParams{
			OrganizationID: orgID, CategoryID: f.CategoryID, Name: f.Name, Description: f.Description,
			DurationMinutes: f.Duration, BufferBeforeMinutes: f.BufferBefore, BufferAfterMinutes: f.BufferAfter,
			Price: f.Price.Numeric(), PaymentMode: f.PaymentMode, StaffMode: f.StaffMode,
			ResourceKinds: f.ResourceKinds, ShowOnline: f.ShowOnline, IsActive: f.IsActive, SortOrder: f.SortOrder,
		})
		if err != nil {
			return err
		}
		created = row
		for _, branchID := range branchIDs {
			if err := tx.UpsertServiceBranch(ctx, db.UpsertServiceBranchParams{
				OrganizationID: orgID, ServiceID: row.ID, BranchID: branchID, IsActive: true,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return model.Service{}, mapNameTaken(err)
	}
	s.index(ctx, orgID, created.Uuid)
	return s.Get(ctx, orgID, created.Uuid)
}

// Patch updates the given catalog fields (branch_uuids is ignored; offers
// have their own endpoint).
func (s *Service) Patch(ctx context.Context, orgID int64, id uuid.UUID, in model.Input) (model.Service, error) {
	q := s.repo.Q()
	row, err := s.load(ctx, q, orgID, id)
	if err != nil {
		return model.Service{}, err
	}
	f := fieldsFromRow(row)
	ve := &ValidationError{}
	if in.CategoryUUID.Set {
		if err := s.resolveCategory(ctx, q, orgID, in.CategoryUUID, &f, ve); err != nil {
			return model.Service{}, err
		}
	}
	f.apply(ve, in, false)
	if err := ve.orNil(); err != nil {
		return model.Service{}, err
	}
	if err := s.checkName(ctx, q, orgID, f.Name, &row.ID); err != nil {
		return model.Service{}, err
	}
	if _, err := q.UpdateService(ctx, db.UpdateServiceParams{
		OrganizationID: orgID, ID: row.ID, CategoryID: f.CategoryID, Name: f.Name, Description: f.Description,
		DurationMinutes: f.Duration, BufferBeforeMinutes: f.BufferBefore, BufferAfterMinutes: f.BufferAfter,
		Price: f.Price.Numeric(), PaymentMode: f.PaymentMode, StaffMode: f.StaffMode,
		ResourceKinds: f.ResourceKinds, ShowOnline: f.ShowOnline, IsActive: f.IsActive, SortOrder: f.SortOrder,
	}); err != nil {
		return model.Service{}, mapNotFound(mapNameTaken(err), ErrNotFound)
	}
	s.index(ctx, orgID, id)
	return s.Get(ctx, orgID, id)
}

// SetActive switches is_active and returns the previous value (bulk engine).
func (s *Service) SetActive(ctx context.Context, orgID int64, id uuid.UUID, active bool) (bool, error) {
	row, err := s.load(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return false, err
	}
	if row.IsActive != active {
		if err := s.repo.Q().SetServiceActive(ctx, db.SetServiceActiveParams{OrganizationID: orgID, ID: row.ID, IsActive: active}); err != nil {
			return row.IsActive, err
		}
		s.index(ctx, orgID, id)
	}
	return row.IsActive, nil
}

// Delete soft-deletes a service. Its name becomes free; offers and staff
// links stay with the deleted row (bookings keep their snapshot).
func (s *Service) Delete(ctx context.Context, orgID int64, id uuid.UUID) (model.Service, error) {
	svc, err := s.Get(ctx, orgID, id)
	if err != nil {
		return model.Service{}, err
	}
	row, err := s.load(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return model.Service{}, err
	}
	n, err := s.repo.Q().SoftDeleteService(ctx, db.SoftDeleteServiceParams{OrganizationID: orgID, ID: row.ID})
	if err != nil {
		return model.Service{}, err
	}
	if n == 0 {
		return model.Service{}, ErrNotFound
	}
	if s.indexer != nil {
		s.indexer.EnqueueDelete(ctx, SearchSpec, searchengine.TenantDocumentID(orgID, id.String()))
	}
	return svc, nil
}

// SetOffers replaces the offers of the branches inside the caller's scope.
// Offers of other branches stay as they are. It returns the offers before
// and after (activity diff).
func (s *Service) SetOffers(ctx context.Context, orgID int64, id uuid.UUID, in []model.OfferInput) (model.Service, []model.Offer, error) {
	before, err := s.Get(ctx, orgID, id)
	if err != nil {
		return model.Service{}, nil, err
	}
	q := s.repo.Q()
	row, err := s.load(ctx, q, orgID, id)
	if err != nil {
		return model.Service{}, nil, err
	}
	ve := &ValidationError{}
	if len(in) > maxLinks {
		ve.add("offers", "too_many", fmt.Sprintf("at most %d offers", maxLinks))
		return model.Service{}, nil, ve
	}
	uuids := make([]uuid.UUID, 0, len(in))
	for _, o := range in {
		uuids = append(uuids, o.BranchUUID)
	}
	known := map[uuid.UUID]int64{}
	if len(uuids) > 0 {
		rows, err := q.ListBranchIDsByUUIDs(ctx, db.ListBranchIDsByUUIDsParams{OrganizationID: orgID, Uuids: uuids})
		if err != nil {
			return model.Service{}, nil, err
		}
		for _, r := range rows {
			known[r.Uuid] = r.ID
		}
	}
	type offer struct {
		branchID int64
		price    pgtype.Numeric
		duration pgtype.Int4
		active   bool
	}
	keep := map[int64]offer{}
	for i, o := range in {
		prefix := fmt.Sprintf("offers.%d", i)
		branchID, ok := known[o.BranchUUID]
		if !ok || orgctx.CheckBranch(ctx, branchID) != nil {
			ve.add(prefix+".branch_uuid", "not_found", "branch not found")
			continue
		}
		if _, dup := keep[branchID]; dup {
			ve.add(prefix+".branch_uuid", "duplicate", "branch listed twice")
			continue
		}
		next := offer{branchID: branchID, active: true}
		if o.IsActive != nil {
			next.active = *o.IsActive
		}
		if o.Price != nil {
			if m, ok := parsePrice(ve, prefix+".price", *o.Price); ok {
				next.price = m.Numeric()
			}
		}
		if o.DurationMinutes != nil {
			if d := *o.DurationMinutes; d < minDuration || d > maxDuration {
				ve.add(prefix+".duration_minutes", "invalid", fmt.Sprintf("duration_minutes must be between %d and %d", minDuration, maxDuration))
			} else {
				next.duration = pgtype.Int4{Int32: d, Valid: true}
			}
		}
		keep[branchID] = next
	}
	if err := ve.orNil(); err != nil {
		return model.Service{}, nil, err
	}
	current, err := q.ListServiceBranches(ctx, db.ListServiceBranchesParams{OrganizationID: orgID, ServiceIds: []int64{row.ID}})
	if err != nil {
		return model.Service{}, nil, err
	}
	err = s.repo.InTx(ctx, func(tx *db.Queries) error {
		for _, c := range current {
			if _, stays := keep[c.BranchID]; !stays && orgctx.CheckBranch(ctx, c.BranchID) == nil {
				if err := tx.DeleteServiceBranch(ctx, db.DeleteServiceBranchParams{OrganizationID: orgID, ServiceID: row.ID, BranchID: c.BranchID}); err != nil {
					return err
				}
			}
		}
		for _, o := range keep {
			if err := tx.UpsertServiceBranch(ctx, db.UpsertServiceBranchParams{
				OrganizationID: orgID, ServiceID: row.ID, BranchID: o.branchID, Price: o.price,
				DurationMinutes: o.duration, IsActive: o.active,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return model.Service{}, nil, err
	}
	after, err := s.Get(ctx, orgID, id)
	return after, before.Offers, err
}

// SetStaff replaces the staff who may perform the service. It returns the
// links before (activity diff).
func (s *Service) SetStaff(ctx context.Context, orgID int64, id uuid.UUID, in []model.StaffInput) (model.Service, []model.StaffLink, error) {
	before, err := s.Get(ctx, orgID, id)
	if err != nil {
		return model.Service{}, nil, err
	}
	q := s.repo.Q()
	row, err := s.load(ctx, q, orgID, id)
	if err != nil {
		return model.Service{}, nil, err
	}
	ve := &ValidationError{}
	if len(in) > maxLinks {
		ve.add("staff", "too_many", fmt.Sprintf("at most %d staff", maxLinks))
		return model.Service{}, nil, ve
	}
	uuids := make([]uuid.UUID, 0, len(in))
	for _, l := range in {
		uuids = append(uuids, l.StaffUUID)
	}
	known := map[uuid.UUID]int64{}
	if len(uuids) > 0 {
		rows, err := q.GetStaffIDsByUUIDs(ctx, db.GetStaffIDsByUUIDsParams{OrganizationID: orgID, Uuids: uuids})
		if err != nil {
			return model.Service{}, nil, err
		}
		for _, r := range rows {
			known[r.Uuid] = r.ID
		}
	}
	type link struct {
		staffID  int64
		price    pgtype.Numeric
		duration pgtype.Int4
	}
	links := make([]link, 0, len(in))
	seen := map[int64]bool{}
	for i, l := range in {
		prefix := fmt.Sprintf("staff.%d", i)
		staffID, ok := known[l.StaffUUID]
		if !ok {
			ve.add(prefix+".staff_uuid", "not_found", "staff member not found")
			continue
		}
		if seen[staffID] {
			ve.add(prefix+".staff_uuid", "duplicate", "staff member listed twice")
			continue
		}
		seen[staffID] = true
		next := link{staffID: staffID}
		if l.Price != nil {
			if m, ok := parsePrice(ve, prefix+".price", *l.Price); ok {
				next.price = m.Numeric()
			}
		}
		if l.DurationMinutes != nil {
			if d := *l.DurationMinutes; d < minDuration || d > maxDuration {
				ve.add(prefix+".duration_minutes", "invalid", fmt.Sprintf("duration_minutes must be between %d and %d", minDuration, maxDuration))
			} else {
				next.duration = pgtype.Int4{Int32: d, Valid: true}
			}
		}
		links = append(links, next)
	}
	if err := ve.orNil(); err != nil {
		return model.Service{}, nil, err
	}
	err = s.repo.InTx(ctx, func(tx *db.Queries) error {
		ids := make([]int64, 0, len(links))
		for _, l := range links {
			ids = append(ids, l.staffID)
		}
		if err := tx.DeleteServiceStaffExcept(ctx, db.DeleteServiceStaffExceptParams{
			OrganizationID: orgID, ServiceID: row.ID, KeepStaffIds: ids,
		}); err != nil {
			return err
		}
		for _, l := range links {
			if err := tx.UpsertServiceStaff(ctx, db.UpsertServiceStaffParams{
				OrganizationID: orgID, ServiceID: row.ID, StaffID: l.staffID, Price: l.price, DurationMinutes: l.duration,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return model.Service{}, nil, err
	}
	after, err := s.Get(ctx, orgID, id)
	return after, before.Staff, err
}

// TargetUUIDs resolves the services matching a list filter (bulk "all
// matching" scope), capped at limit.
func (s *Service) TargetUUIDs(ctx context.Context, orgID int64, f model.ListFilter, limit int32) ([]uuid.UUID, error) {
	q := s.repo.Q()
	a, err := s.listArgs(ctx, q, orgID, f)
	if err != nil {
		return nil, err
	}
	return q.ListServiceUUIDs(ctx, db.ListServiceUUIDsParams{
		OrganizationID: orgID, CategoryIds: a.categories, Uncategorized: a.uncategorized,
		FilterBranchIds: a.branches, PaymentModes: a.paymentModes, IsActive: a.isActive,
		ShowOnline: a.showOnline, Q: a.q, LimitCount: limit,
	})
}

// ExportRows returns every service matching a list filter (capped at limit).
func (s *Service) ExportRows(ctx context.Context, orgID int64, f model.ListFilter, limit int32) ([]model.Service, error) {
	q := s.repo.Q()
	a, err := s.listArgs(ctx, q, orgID, f)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListServicesForExport(ctx, db.ListServicesForExportParams{
		OrganizationID: orgID, CategoryIds: a.categories, Uncategorized: a.uncategorized,
		FilterBranchIds: a.branches, PaymentModes: a.paymentModes, IsActive: a.isActive,
		ShowOnline: a.showOnline, Q: a.q, LimitCount: limit,
	})
	if err != nil {
		return nil, err
	}
	conv := make([]db.GetServiceRow, 0, len(rows))
	for _, row := range rows {
		conv = append(conv, db.GetServiceRow(row))
	}
	return s.mapRows(ctx, q, orgID, conv, false)
}

// ImportRow updates the service with the same name, or creates one.
// categoryName names (and creates when missing) the category; branchCodes
// set the offers of a new service. It returns the previous state of an
// update.
func (s *Service) ImportRow(ctx context.Context, orgID int64, categoryName string, branchCodes []string, in model.Input) (uuid.UUID, bool, *model.Service, error) {
	q := s.repo.Q()
	if name := strings.TrimSpace(categoryName); name != "" {
		cat, err := q.GetServiceCategoryByName(ctx, db.GetServiceCategoryByNameParams{OrganizationID: orgID, Name: name})
		if err != nil {
			if !errors.Is(repository.MapErr(err), repository.ErrNotFound) {
				return uuid.Nil, false, nil, err
			}
			created, err := s.CreateCategory(ctx, orgID, model.CategoryInput{Name: &name})
			if err != nil {
				return uuid.Nil, false, nil, err
			}
			in.CategoryUUID = model.OptionalUUID{Set: true, Value: &created.UUID}
		} else {
			in.CategoryUUID = model.OptionalUUID{Set: true, Value: &cat.Uuid}
		}
	}
	for _, code := range branchCodes {
		branch, err := q.GetBranchByCode(ctx, db.GetBranchByCodeParams{OrganizationID: orgID, Code: strings.ToUpper(strings.TrimSpace(code))})
		if err != nil {
			if errors.Is(repository.MapErr(err), repository.ErrNotFound) {
				return uuid.Nil, false, nil, &ValidationError{Details: []Detail{{Field: "branch_codes", Code: "not_found", Message: "unknown branch code " + code}}}
			}
			return uuid.Nil, false, nil, err
		}
		in.BranchUUIDs = append(in.BranchUUIDs, branch.Uuid)
	}
	if in.Name != nil {
		existing, err := q.GetServiceByName(ctx, db.GetServiceByNameParams{OrganizationID: orgID, Name: strings.TrimSpace(*in.Name)})
		if err == nil {
			prev, err := s.Get(ctx, orgID, existing.Uuid)
			if err != nil {
				return uuid.Nil, false, nil, err
			}
			svc, err := s.Patch(ctx, orgID, existing.Uuid, in)
			if err != nil {
				return uuid.Nil, false, nil, err
			}
			return svc.UUID, false, &prev, nil
		}
		if !errors.Is(repository.MapErr(err), repository.ErrNotFound) {
			return uuid.Nil, false, nil, err
		}
	}
	svc, err := s.Create(ctx, orgID, in)
	if err != nil {
		return uuid.Nil, false, nil, err
	}
	return svc.UUID, true, nil, nil
}

// Activity lists the audit events of a service.
func (s *Service) Activity(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]model.ActivityEntry, int64, error) {
	if _, err := s.load(ctx, s.repo.Q(), orgID, id); err != nil {
		return nil, 0, err
	}
	q := s.repo.Q()
	rows, err := q.ListOrganizationResourceActivity(ctx, db.ListOrganizationResourceActivityParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, Resource: model.Slug,
		ResourceUuid: pgtype.UUID{Bytes: id, Valid: true}, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountOrganizationResourceActivity(ctx, db.CountOrganizationResourceActivityParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, Resource: model.Slug,
		ResourceUuid: pgtype.UUID{Bytes: id, Valid: true},
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.ActivityEntry, 0, len(rows))
	for _, row := range rows {
		e := model.ActivityEntry{UUID: row.Uuid, Action: row.Action, Payload: json.RawMessage(row.Payload), CreatedAt: row.CreatedAt.Time}
		if row.ActorUuid.Valid {
			e.Actor = &model.ActivityActor{
				UUID: uuid.UUID(row.ActorUuid.Bytes), Name: row.ActorName.String,
				Surname: row.ActorSurname.String, Email: row.ActorEmail.String,
			}
		}
		out = append(out, e)
	}
	return out, total, nil
}

// load reads one live service of the organization.
func (s *Service) load(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (db.GetServiceRow, error) {
	row, err := q.GetService(ctx, db.GetServiceParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return db.GetServiceRow{}, mapNotFound(repository.MapErr(err), ErrNotFound)
	}
	return row, nil
}

// mapRows maps service rows with their offers (and staff links when
// withStaff) in two queries.
func (s *Service) mapRows(ctx context.Context, q *db.Queries, orgID int64, rows []db.GetServiceRow, withStaff bool) ([]model.Service, error) {
	out := make([]model.Service, 0, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	offers, err := q.ListServiceBranches(ctx, db.ListServiceBranchesParams{OrganizationID: orgID, ServiceIds: ids})
	if err != nil {
		return nil, err
	}
	byService := map[int64][]db.ListServiceBranchesRow{}
	for _, o := range offers {
		byService[o.ServiceID] = append(byService[o.ServiceID], o)
	}
	staffBy := map[int64][]db.ListServiceStaffRow{}
	if withStaff {
		links, err := q.ListServiceStaff(ctx, db.ListServiceStaffParams{OrganizationID: orgID, ServiceIds: ids})
		if err != nil {
			return nil, err
		}
		for _, l := range links {
			staffBy[l.ServiceID] = append(staffBy[l.ServiceID], l)
		}
	}
	currency := currencyOf(ctx)
	orgUUID, _ := scopeUUID(ctx)
	for _, row := range rows {
		svc := MapService(row, orgUUID, currency)
		for _, o := range byService[row.ID] {
			svc.Offers = append(svc.Offers, mapOffer(ctx, o, svc))
		}
		if withStaff {
			svc.Staff = []model.StaffLink{}
			for _, l := range staffBy[row.ID] {
				svc.Staff = append(svc.Staff, mapStaff(l))
			}
		}
		out = append(out, svc)
	}
	return out, nil
}

// resolveCategory sets f.CategoryID from an optional category uuid (null
// clears it).
func (s *Service) resolveCategory(ctx context.Context, q *db.Queries, orgID int64, in model.OptionalUUID, f *fields, ve *ValidationError) error {
	if !in.Set || in.Value == nil {
		if in.Set {
			f.CategoryID = pgtype.Int8{}
		}
		return nil
	}
	cat, err := q.GetServiceCategory(ctx, db.GetServiceCategoryParams{OrganizationID: orgID, Uuid: *in.Value})
	if err != nil {
		if errors.Is(repository.MapErr(err), repository.ErrNotFound) {
			ve.add("category_uuid", "not_found", "category not found")
			return nil
		}
		return err
	}
	f.CategoryID = pgtype.Int8{Int64: cat.ID, Valid: true}
	return nil
}

// resolveOfferBranches maps the branch uuids of a new service; unknown
// branches and branches outside the caller's scope are the same error.
func (s *Service) resolveOfferBranches(ctx context.Context, q *db.Queries, orgID int64, ids []uuid.UUID, ve *ValidationError) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := q.ListBranchIDsByUUIDs(ctx, db.ListBranchIDsByUUIDsParams{OrganizationID: orgID, Uuids: ids})
	if err != nil {
		return nil, err
	}
	known := map[uuid.UUID]int64{}
	for _, r := range rows {
		known[r.Uuid] = r.ID
	}
	out := make([]int64, 0, len(ids))
	for i, id := range ids {
		branchID, ok := known[id]
		if !ok || orgctx.CheckBranch(ctx, branchID) != nil {
			ve.add(fmt.Sprintf("branch_uuids.%d", i), "not_found", "branch not found")
			continue
		}
		if !slices.Contains(out, branchID) {
			out = append(out, branchID)
		}
	}
	return out, nil
}

// checkName answers ErrNameTaken when the catalog has another live service
// with the name (the unique index is the backstop).
func (s *Service) checkName(ctx context.Context, q *db.Queries, orgID int64, name string, exclude *int64) error {
	excl := pgtype.Int8{}
	if exclude != nil {
		excl = pgtype.Int8{Int64: *exclude, Valid: true}
	}
	taken, err := q.ServiceNameTaken(ctx, db.ServiceNameTakenParams{OrganizationID: orgID, Name: name, ExcludeID: excl})
	if err != nil {
		return err
	}
	if taken {
		return ErrNameTaken
	}
	return nil
}

func (s *Service) index(ctx context.Context, orgID int64, id uuid.UUID) {
	if s.indexer != nil {
		s.indexer.EnqueueUpsert(ctx, SearchSpec, searchengine.TenantDocumentID(orgID, id.String()))
	}
}

// fields are the writable columns of a service.
type fields struct {
	CategoryID             pgtype.Int8
	Name, Description      string
	Duration, BufferBefore int32
	BufferAfter            int32
	Price                  money.Money
	PaymentMode, StaffMode string
	ResourceKinds          []string
	ShowOnline, IsActive   bool
	SortOrder              int32
}

func fieldsFromRow(row db.GetServiceRow) fields {
	kinds := row.ResourceKinds
	if kinds == nil {
		kinds = []string{}
	}
	return fields{
		CategoryID: row.CategoryID, Name: row.Name, Description: row.Description,
		Duration: row.DurationMinutes, BufferBefore: row.BufferBeforeMinutes, BufferAfter: row.BufferAfterMinutes,
		Price: money.FromNumericOrZero(row.Price), PaymentMode: row.PaymentMode, StaffMode: row.StaffMode,
		ResourceKinds: kinds, ShowOnline: row.ShowOnline, IsActive: row.IsActive, SortOrder: row.SortOrder,
	}
}

func (f *fields) apply(ve *ValidationError, in model.Input, create bool) {
	text := func(field string, src *string, dst *string, max int, required bool) {
		if src == nil {
			if create && required {
				ve.add(field, "required", field+" is required")
			}
			return
		}
		v := strings.TrimSpace(*src)
		switch {
		case required && v == "":
			ve.add(field, "required", field+" is required")
		case utf8.RuneCountInString(v) > max:
			ve.add(field, "too_long", fmt.Sprintf("%s must be at most %d characters", field, max))
		default:
			*dst = v
		}
	}
	text("name", in.Name, &f.Name, maxNameLength, true)
	text("description", in.Description, &f.Description, maxDescriptionLen, false)
	if in.DurationMinutes != nil {
		f.Duration = *in.DurationMinutes
	} else if create {
		ve.add("duration_minutes", "required", "duration_minutes is required")
	}
	if (in.DurationMinutes != nil || !create) && (f.Duration < minDuration || f.Duration > maxDuration) {
		ve.add("duration_minutes", "invalid", fmt.Sprintf("duration_minutes must be between %d and %d", minDuration, maxDuration))
	}
	buffer := func(field string, src *int32, dst *int32) {
		if src == nil {
			return
		}
		if *src < 0 || *src > maxBuffer {
			ve.add(field, "invalid", fmt.Sprintf("%s must be between 0 and %d", field, maxBuffer))
			return
		}
		*dst = *src
	}
	buffer("buffer_before_minutes", in.BufferBeforeMinutes, &f.BufferBefore)
	buffer("buffer_after_minutes", in.BufferAfterMinutes, &f.BufferAfter)
	if in.Price != nil {
		if m, ok := parsePrice(ve, "price", *in.Price); ok {
			f.Price = m
		}
	}
	choice := func(field string, src *string, dst *string, allowed []string) {
		if src == nil {
			return
		}
		if !slices.Contains(allowed, *src) {
			ve.add(field, "invalid", field+" must be one of "+strings.Join(allowed, ", "))
			return
		}
		*dst = *src
	}
	choice("payment_mode", in.PaymentMode, &f.PaymentMode, model.PaymentModes)
	choice("staff_mode", in.StaffMode, &f.StaffMode, model.StaffModes)
	if in.ResourceKinds != nil {
		kinds := make([]string, 0, len(*in.ResourceKinds))
		for i, k := range *in.ResourceKinds {
			switch {
			case !slices.Contains(model.ResourceKinds, k):
				ve.add(fmt.Sprintf("resource_kinds.%d", i), "invalid", "kind must be one of "+strings.Join(model.ResourceKinds, ", "))
			case slices.Contains(kinds, k):
				ve.add(fmt.Sprintf("resource_kinds.%d", i), "duplicate", "kind listed twice")
			default:
				kinds = append(kinds, k)
			}
		}
		f.ResourceKinds = kinds
	}
	if in.ShowOnline != nil {
		f.ShowOnline = *in.ShowOnline
	}
	if in.IsActive != nil {
		f.IsActive = *in.IsActive
	}
	if in.SortOrder != nil {
		if *in.SortOrder < 0 || *in.SortOrder > maxSortOrder {
			ve.add("sort_order", "invalid", fmt.Sprintf("sort_order must be between 0 and %d", maxSortOrder))
		} else {
			f.SortOrder = *in.SortOrder
		}
	}
}

// parsePrice reads a non-negative amount with at most two decimals.
func parsePrice(ve *ValidationError, field string, raw json.Number) (money.Money, bool) {
	m, err := money.Parse(strings.TrimSpace(raw.String()))
	if err != nil || m.IsNegative() {
		ve.add(field, "invalid", field+" must be a non-negative amount with at most 2 decimals")
		return money.Money{}, false
	}
	return m, true
}

// defaultStaffMode follows the sector: a sector without staff books no
// staff time by default.
func defaultStaffMode(ctx context.Context) string {
	if scope, ok := orgctx.From(ctx); ok && scope.Sector == "restaurant" {
		return "none"
	}
	return "required"
}

func currencyOf(ctx context.Context) string {
	if scope, ok := orgctx.From(ctx); ok && scope.Currency != "" {
		return scope.Currency
	}
	return string(money.TRY)
}

func scopeUUID(ctx context.Context) (uuid.UUID, bool) {
	scope, ok := orgctx.From(ctx)
	if !ok || scope.UUID == uuid.Nil {
		return uuid.Nil, false
	}
	return scope.UUID, true
}

// PublicImagePath is the API-owned public URL of a service image; booking
// pages and the mobile app use it as is.
func PublicImagePath(orgUUID, imageUUID uuid.UUID) string {
	return "/v1/public/media/services/" + orgUUID.String() + "/images/" + imageUUID.String()
}

// MapService maps a service row (list, detail and export share the columns).
func MapService(row db.GetServiceRow, orgUUID uuid.UUID, currency string) model.Service {
	kinds := row.ResourceKinds
	if kinds == nil {
		kinds = []string{}
	}
	svc := model.Service{
		UUID: row.Uuid, Name: row.Name, Description: row.Description,
		DurationMinutes: row.DurationMinutes, BufferBeforeMinutes: row.BufferBeforeMinutes,
		BufferAfterMinutes: row.BufferAfterMinutes, Price: money.FromNumericOrZero(row.Price),
		Currency: currency, PaymentMode: row.PaymentMode, StaffMode: row.StaffMode, ResourceKinds: kinds,
		ShowOnline: row.ShowOnline, IsActive: row.IsActive, SortOrder: row.SortOrder,
		Offers: []model.Offer{}, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if row.CategoryUuid.Valid {
		svc.Category = &model.CategoryRef{UUID: uuid.UUID(row.CategoryUuid.Bytes), Name: row.CategoryName.String}
	}
	if row.ImageUuid.Valid && orgUUID != uuid.Nil {
		url := PublicImagePath(orgUUID, uuid.UUID(row.ImageUuid.Bytes))
		svc.ImageURL = &url
	}
	return svc
}

func mapOffer(ctx context.Context, o db.ListServiceBranchesRow, svc model.Service) model.Offer {
	out := model.Offer{
		BranchUUID: o.BranchUuid, BranchName: o.BranchName, BranchCode: o.BranchCode,
		EffectivePrice: svc.Price, EffectiveDurationMinutes: svc.DurationMinutes, IsActive: o.IsActive,
		Editable: orgctx.CheckBranch(ctx, o.BranchID) == nil,
	}
	if o.Price.Valid {
		p := money.FromNumericOrZero(o.Price)
		out.Price, out.EffectivePrice = &p, p
	}
	if o.DurationMinutes.Valid {
		d := o.DurationMinutes.Int32
		out.DurationMinutes, out.EffectiveDurationMinutes = &d, d
	}
	return out
}

func mapStaff(l db.ListServiceStaffRow) model.StaffLink {
	out := model.StaffLink{
		StaffUUID: l.StaffUuid, Name: l.StaffName, Title: l.StaffTitle, Color: l.StaffColor, IsActive: l.StaffIsActive,
	}
	if l.Price.Valid {
		p := money.FromNumericOrZero(l.Price)
		out.Price = &p
	}
	if l.DurationMinutes.Valid {
		d := l.DurationMinutes.Int32
		out.DurationMinutes = &d
	}
	return out
}

func mapNotFound(err, target error) error {
	if errors.Is(err, repository.ErrNotFound) {
		return target
	}
	return err
}

func mapNameTaken(err error) error {
	switch mapped := repository.MapErr(err); {
	case errors.Is(mapped, repository.ErrNameTaken):
		return ErrNameTaken
	case errors.Is(mapped, repository.ErrCategoryNameTaken):
		return ErrCategoryNameTaken
	}
	return err
}

func nilIfEmpty(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

func textArg(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func boolArg(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}

func pgInt8(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: true} }
