// Package usecase holds the rules of the combined service bundles
// (TEC-895). Every method takes the organization id of the caller
// explicitly. Bundles are business-level catalog data like services: every
// member with tenant.services.read sees all of them. The branches of a
// bundle are branch-level: a member limited to some branches names and
// removes only those; the other branches of the bundle stay.
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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/servicebundles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

var (
	// ErrNotFound: missing, deleted or of another organization.
	ErrNotFound = errors.New("service bundle not found")
	// ErrNameTaken: the catalog already has a bundle with the name.
	ErrNameTaken = errors.New("the catalog already has a bundle with this name")
)

// Detail is one field-level validation problem.
type Detail struct {
	Field   string
	Message string
	Code    string
}

// ValidationError carries field details; handlers answer 400.
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

// Store runs the queries, alone or in one transaction.
type Store interface {
	Q() *db.Queries
	InTx(ctx context.Context, fn func(q *db.Queries) error) error
}

// Service implements the bundle use cases.
type Service struct {
	store Store
}

// New builds the service.
func New(store Store) *Service {
	return &Service{store: store}
}

// SortColumns are the sortable list fields.
var SortColumns = map[string]string{
	"name": "name", "price": "price", "sort_order": "sort_order", "is_active": "is_active",
	"created_at": "created_at", "updated_at": "updated_at",
}

type listArgs struct {
	branches             []int64
	isActive, showOnline pgtype.Bool
	q                    pgtype.Text
}

func (s *Service) listArgs(ctx context.Context, q *db.Queries, orgID int64, f model.ListFilter) (listArgs, error) {
	a := listArgs{isActive: boolArg(f.IsActive), showOnline: boolArg(f.ShowOnline)}
	if v := strings.TrimSpace(f.Q); v != "" {
		a.q = pgtype.Text{String: v, Valid: true}
	}
	if len(f.Branches) > 0 {
		rows, err := q.ListBranchIDsByUUIDs(ctx, db.ListBranchIDsByUUIDsParams{OrganizationID: orgID, Uuids: f.Branches})
		if err != nil {
			return a, err
		}
		// Unknown branches match nothing (an empty, non-nil filter).
		a.branches = []int64{}
		for _, row := range rows {
			a.branches = append(a.branches, row.ID)
		}
	}
	return a, nil
}

// List returns one page of bundles with their contents and branches.
func (s *Service) List(ctx context.Context, orgID int64, f model.ListFilter) ([]model.Bundle, int64, error) {
	if _, ok := SortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "", false
	}
	q := s.store.Q()
	a, err := s.listArgs(ctx, q, orgID, f)
	if err != nil {
		return nil, 0, err
	}
	rows, err := q.ListServiceBundles(ctx, db.ListServiceBundlesParams{
		OrganizationID: orgID, FilterBranchIds: a.branches, IsActive: a.isActive, ShowOnline: a.showOnline, Q: a.q,
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountServiceBundles(ctx, db.CountServiceBundlesParams{
		OrganizationID: orgID, FilterBranchIds: a.branches, IsActive: a.isActive, ShowOnline: a.showOnline, Q: a.q,
	})
	if err != nil {
		return nil, 0, err
	}
	out, err := s.mapRows(ctx, q, orgID, rows)
	return out, total, err
}

// Facets returns the branches offering bundles (inside the branch scope).
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	out := model.Facets{Branch: []model.Facet{}}
	rows, err := s.store.Q().ListServiceBundleBranchFacets(ctx, db.ListServiceBundleBranchFacetsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx),
	})
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		out.Branch = append(out.Branch, model.Facet{Value: r.Uuid.String(), Label: r.Name, Count: r.Count})
	}
	return out, nil
}

// Get returns one bundle.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Bundle, error) {
	q := s.store.Q()
	row, err := s.load(ctx, q, orgID, id)
	if err != nil {
		return model.Bundle{}, err
	}
	out, err := s.mapRows(ctx, q, orgID, []db.ServiceBundle{row})
	if err != nil {
		return model.Bundle{}, err
	}
	return out[0], nil
}

// fields are the scalar values of a bundle.
type fields struct {
	Name, Description    string
	Price                money.Money
	ShowOnline, IsActive bool
	SortOrder            int32
}

func (f *fields) apply(ve *ValidationError, in model.Input, create bool) {
	if in.Name != nil {
		v := strings.TrimSpace(*in.Name)
		switch {
		case v == "":
			ve.add("name", "required", "name is required")
		case utf8.RuneCountInString(v) > model.MaxNameLength:
			ve.add("name", "too_long", fmt.Sprintf("name must be at most %d characters", model.MaxNameLength))
		default:
			f.Name = v
		}
	} else if create {
		ve.add("name", "required", "name is required")
	}
	if in.Description != nil {
		v := strings.TrimSpace(*in.Description)
		if utf8.RuneCountInString(v) > model.MaxDescriptionLen {
			ve.add("description", "too_long", fmt.Sprintf("description must be at most %d characters", model.MaxDescriptionLen))
		} else {
			f.Description = v
		}
	}
	if in.Price != nil {
		m, err := money.Parse(strings.TrimSpace(in.Price.String()))
		if err != nil || m.IsNegative() {
			ve.add("price", "invalid", "price must be a non-negative amount with at most 2 decimals")
		} else {
			f.Price = m
		}
	} else if create {
		ve.add("price", "required", "price is required")
	}
	if in.ShowOnline != nil {
		f.ShowOnline = *in.ShowOnline
	}
	if in.IsActive != nil {
		f.IsActive = *in.IsActive
	}
	if in.SortOrder != nil {
		if *in.SortOrder < 0 || *in.SortOrder > model.MaxSortOrder {
			ve.add("sort_order", "invalid", fmt.Sprintf("sort_order must be between 0 and %d", model.MaxSortOrder))
		} else {
			f.SortOrder = *in.SortOrder
		}
	}
}

// resolveItems maps the content uuids to live, non-group services of the
// organization (unknown → service_uuids.N not_found).
func (s *Service) resolveItems(ctx context.Context, q *db.Queries, orgID int64, ids []uuid.UUID, ve *ValidationError) ([]int64, error) {
	if len(ids) < model.MinItems {
		ve.add("service_uuids", "too_few", fmt.Sprintf("a bundle holds at least %d services", model.MinItems))
		return nil, nil
	}
	if len(ids) > model.MaxItems {
		ve.add("service_uuids", "too_many", fmt.Sprintf("a bundle holds at most %d services", model.MaxItems))
		return nil, nil
	}
	rows, err := q.ListBundleServicesByUUIDs(ctx, db.ListBundleServicesByUUIDsParams{OrganizationID: orgID, Uuids: ids})
	if err != nil {
		return nil, err
	}
	known := map[uuid.UUID]db.ListBundleServicesByUUIDsRow{}
	for _, r := range rows {
		known[r.Uuid] = r
	}
	out := make([]int64, 0, len(ids))
	for i, id := range ids {
		field := fmt.Sprintf("service_uuids.%d", i)
		r, ok := known[id]
		switch {
		case !ok:
			ve.add(field, "not_found", "service not found")
		case r.GroupCapacity.Valid:
			// A group class books a seat of a session, not a person's time.
			ve.add(field, "group_class", "a group class cannot be part of a bundle")
		default:
			out = append(out, r.ID)
		}
	}
	return out, nil
}

// resolveBranches maps branch uuids inside the caller's scope (others read
// as not found).
func (s *Service) resolveBranches(ctx context.Context, q *db.Queries, orgID int64, ids []uuid.UUID, ve *ValidationError) ([]int64, error) {
	if len(ids) == 0 {
		return []int64{}, nil
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
		field := fmt.Sprintf("branch_uuids.%d", i)
		branchID, ok := known[id]
		if !ok || orgctx.CheckBranch(ctx, branchID) != nil {
			ve.add(field, "not_found", "branch not found")
			continue
		}
		if slices.Contains(out, branchID) {
			ve.add(field, "duplicate", "branch listed twice")
			continue
		}
		out = append(out, branchID)
	}
	return out, nil
}

// checkOffers requires every branch of the bundle to offer every service of
// it: a bundle is bookable only where all its lines are.
func (s *Service) checkOffers(ctx context.Context, q *db.Queries, orgID int64, serviceIDs, branchIDs []int64, branchUUIDs []uuid.UUID, ve *ValidationError) error {
	if len(serviceIDs) == 0 || len(branchIDs) == 0 {
		return nil
	}
	rows, err := q.ListServiceBranchOffersFor(ctx, db.ListServiceBranchOffersForParams{
		OrganizationID: orgID, ServiceIds: serviceIDs, BranchIds: branchIDs,
	})
	if err != nil {
		return err
	}
	offered := map[[2]int64]bool{}
	for _, r := range rows {
		offered[[2]int64{r.ServiceID, r.BranchID}] = true
	}
	for i, b := range branchIDs {
		for _, sid := range serviceIDs {
			if !offered[[2]int64{sid, b}] {
				field := "branch_uuids"
				if i < len(branchUUIDs) {
					field = fmt.Sprintf("branch_uuids.%d", i)
				}
				ve.add(field, "services_not_offered", "the branch does not offer every service of the bundle")
				break
			}
		}
	}
	return nil
}

// Create validates and stores a new bundle.
func (s *Service) Create(ctx context.Context, orgID int64, in model.Input) (model.Bundle, error) {
	f := fields{ShowOnline: true, IsActive: true}
	ve := &ValidationError{}
	f.apply(ve, in, true)
	q := s.store.Q()
	items, err := s.resolveItems(ctx, q, orgID, in.ServiceUUIDs, ve)
	if err != nil {
		return model.Bundle{}, err
	}
	branches, err := s.resolveBranches(ctx, q, orgID, in.BranchUUIDs, ve)
	if err != nil {
		return model.Bundle{}, err
	}
	if len(ve.Details) == 0 {
		if err := s.checkOffers(ctx, q, orgID, items, branches, in.BranchUUIDs, ve); err != nil {
			return model.Bundle{}, err
		}
	}
	if err := ve.orNil(); err != nil {
		return model.Bundle{}, err
	}
	var created db.ServiceBundle
	err = s.store.InTx(ctx, func(tx *db.Queries) error {
		row, err := tx.CreateServiceBundle(ctx, db.CreateServiceBundleParams{
			OrganizationID: orgID, Name: f.Name, Description: f.Description, Price: f.Price.Numeric(),
			ShowOnline: f.ShowOnline, IsActive: f.IsActive, SortOrder: f.SortOrder,
		})
		if err != nil {
			return err
		}
		created = row
		if err := writeItems(ctx, tx, orgID, row.ID, items); err != nil {
			return err
		}
		for _, b := range branches {
			if err := tx.InsertServiceBundleBranch(ctx, db.InsertServiceBundleBranchParams{OrganizationID: orgID, BundleID: row.ID, BranchID: b}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return model.Bundle{}, mapErr(err)
	}
	return s.Get(ctx, orgID, created.Uuid)
}

// Patch updates the given fields; service_uuids / branch_uuids replace the
// contents / the branches inside the caller's scope when given.
func (s *Service) Patch(ctx context.Context, orgID int64, id uuid.UUID, in model.Input) (model.Bundle, error) {
	q := s.store.Q()
	row, err := s.load(ctx, q, orgID, id)
	if err != nil {
		return model.Bundle{}, err
	}
	f := fields{
		Name: row.Name, Description: row.Description, Price: money.FromNumericOrZero(row.Price),
		ShowOnline: row.ShowOnline, IsActive: row.IsActive, SortOrder: row.SortOrder,
	}
	ve := &ValidationError{}
	f.apply(ve, in, false)
	currentItems, err := q.ListServiceBundleItems(ctx, db.ListServiceBundleItemsParams{OrganizationID: orgID, BundleIds: []int64{row.ID}})
	if err != nil {
		return model.Bundle{}, err
	}
	items := make([]int64, 0, len(currentItems))
	for _, it := range currentItems {
		items = append(items, it.ServiceID)
	}
	itemsChanged := in.ServiceUUIDs != nil
	if itemsChanged {
		if items, err = s.resolveItems(ctx, q, orgID, in.ServiceUUIDs, ve); err != nil {
			return model.Bundle{}, err
		}
	}
	currentBranches, err := q.ListServiceBundleBranches(ctx, db.ListServiceBundleBranchesParams{OrganizationID: orgID, BundleIds: []int64{row.ID}})
	if err != nil {
		return model.Bundle{}, err
	}
	// The branches after the change: the caller's named ones plus the
	// current branches outside its scope.
	var named []int64
	final := make([]int64, 0, len(currentBranches))
	if in.BranchUUIDs != nil {
		if named, err = s.resolveBranches(ctx, q, orgID, in.BranchUUIDs, ve); err != nil {
			return model.Bundle{}, err
		}
		final = append(final, named...)
		for _, b := range currentBranches {
			if orgctx.CheckBranch(ctx, b.BranchID) != nil && !slices.Contains(final, b.BranchID) {
				final = append(final, b.BranchID)
			}
		}
	} else {
		for _, b := range currentBranches {
			final = append(final, b.BranchID)
		}
	}
	if len(ve.Details) == 0 && (itemsChanged || in.BranchUUIDs != nil) {
		// Named branches are reported by their position in the body; kept
		// ones (outside the scope, or all when only the contents change)
		// are checked when the contents change, on branch_uuids.
		if err := s.checkOffers(ctx, q, orgID, items, named, in.BranchUUIDs, ve); err != nil {
			return model.Bundle{}, err
		}
		if itemsChanged {
			if err := s.checkOffers(ctx, q, orgID, items, outside(final, named), nil, ve); err != nil {
				return model.Bundle{}, err
			}
		}
	}
	if err := ve.orNil(); err != nil {
		return model.Bundle{}, err
	}
	err = s.store.InTx(ctx, func(tx *db.Queries) error {
		if _, err := tx.UpdateServiceBundle(ctx, db.UpdateServiceBundleParams{
			OrganizationID: orgID, ID: row.ID, Name: f.Name, Description: f.Description, Price: f.Price.Numeric(),
			ShowOnline: f.ShowOnline, IsActive: f.IsActive, SortOrder: f.SortOrder,
		}); err != nil {
			return err
		}
		if itemsChanged {
			if err := tx.DeleteServiceBundleItems(ctx, db.DeleteServiceBundleItemsParams{OrganizationID: orgID, BundleID: row.ID}); err != nil {
				return err
			}
			if err := writeItems(ctx, tx, orgID, row.ID, items); err != nil {
				return err
			}
		}
		if in.BranchUUIDs != nil {
			for _, b := range currentBranches {
				if !slices.Contains(final, b.BranchID) {
					if err := tx.DeleteServiceBundleBranch(ctx, db.DeleteServiceBundleBranchParams{
						OrganizationID: orgID, BundleID: row.ID, BranchID: b.BranchID,
					}); err != nil {
						return err
					}
				}
			}
			for _, b := range named {
				if err := tx.InsertServiceBundleBranch(ctx, db.InsertServiceBundleBranchParams{OrganizationID: orgID, BundleID: row.ID, BranchID: b}); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return model.Bundle{}, mapErr(err)
	}
	return s.Get(ctx, orgID, id)
}

// outside returns the members of all that are not in named.
func outside(all, named []int64) []int64 {
	out := []int64{}
	for _, v := range all {
		if !slices.Contains(named, v) {
			out = append(out, v)
		}
	}
	return out
}

// Delete soft-deletes a bundle; booked lines keep its name.
func (s *Service) Delete(ctx context.Context, orgID int64, id uuid.UUID) (model.Bundle, error) {
	b, err := s.Get(ctx, orgID, id)
	if err != nil {
		return model.Bundle{}, err
	}
	row, err := s.load(ctx, s.store.Q(), orgID, id)
	if err != nil {
		return model.Bundle{}, err
	}
	n, err := s.store.Q().SoftDeleteServiceBundle(ctx, db.SoftDeleteServiceBundleParams{OrganizationID: orgID, ID: row.ID})
	if err != nil {
		return model.Bundle{}, err
	}
	if n == 0 {
		return model.Bundle{}, ErrNotFound
	}
	return b, nil
}

// Activity lists the audit events of a bundle.
func (s *Service) Activity(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]model.ActivityEntry, int64, error) {
	q := s.store.Q()
	if _, err := s.load(ctx, q, orgID, id); err != nil {
		return nil, 0, err
	}
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

func (s *Service) load(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (db.ServiceBundle, error) {
	row, err := q.GetServiceBundle(ctx, db.GetServiceBundleParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	return row, err
}

func writeItems(ctx context.Context, q *db.Queries, orgID, bundleID int64, serviceIDs []int64) error {
	for i, sid := range serviceIDs {
		if err := q.InsertServiceBundleItem(ctx, db.InsertServiceBundleItemParams{
			OrganizationID: orgID, BundleID: bundleID, ServiceID: sid, Position: int32(i),
		}); err != nil {
			return err
		}
	}
	return nil
}

// mapRows adds the contents, the branches and the derived values.
func (s *Service) mapRows(ctx context.Context, q *db.Queries, orgID int64, rows []db.ServiceBundle) ([]model.Bundle, error) {
	out := make([]model.Bundle, 0, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	items, err := q.ListServiceBundleItems(ctx, db.ListServiceBundleItemsParams{OrganizationID: orgID, BundleIds: ids})
	if err != nil {
		return nil, err
	}
	branches, err := q.ListServiceBundleBranches(ctx, db.ListServiceBundleBranchesParams{OrganizationID: orgID, BundleIds: ids})
	if err != nil {
		return nil, err
	}
	byBundle := map[int64][]db.ListServiceBundleItemsRow{}
	for _, it := range items {
		byBundle[it.BundleID] = append(byBundle[it.BundleID], it)
	}
	branchesOf := map[int64][]model.BranchRef{}
	for _, b := range branches {
		branchesOf[b.BundleID] = append(branchesOf[b.BundleID], model.BranchRef{
			UUID: b.BranchUuid, Name: b.BranchName, Code: b.BranchCode, Editable: orgctx.CheckBranch(ctx, b.BranchID) == nil,
		})
	}
	currency := string(money.TRY)
	if scope, ok := orgctx.From(ctx); ok && scope.Currency != "" {
		currency = scope.Currency
	}
	for _, r := range rows {
		price := money.FromNumericOrZero(r.Price)
		b := model.Bundle{
			UUID: r.Uuid, Name: r.Name, Description: r.Description, Price: price, ListPrice: money.Zero(),
			Currency: currency, ResourceKinds: []string{}, Items: []model.Item{}, Branches: branchesOf[r.ID],
			ShowOnline: r.ShowOnline, IsActive: r.IsActive, Bookable: r.IsActive, SortOrder: r.SortOrder,
			CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
		}
		if b.Branches == nil {
			b.Branches = []model.BranchRef{}
		}
		lists := []money.Money{}
		for _, it := range byBundle[r.ID] {
			list := money.FromNumericOrZero(it.ServicePrice)
			lists = append(lists, list)
			kinds := it.ResourceKinds
			if kinds == nil {
				kinds = []string{}
			}
			item := model.Item{
				Position: int(it.Position), ServiceUUID: it.ServiceUuid, ServiceName: it.ServiceName,
				DurationMinutes: it.DurationMinutes, BufferBeforeMinutes: it.BufferBeforeMinutes,
				BufferAfterMinutes: it.BufferAfterMinutes, ListPrice: list, StaffMode: it.StaffMode,
				ResourceKinds: kinds, Bookable: it.ServiceIsActive && !it.ServiceDeleted,
			}
			if !item.Bookable {
				b.Bookable = false
			}
			b.ListPrice = b.ListPrice.Add(list)
			b.DurationMinutes += it.DurationMinutes
			if it.StaffMode == "required" {
				b.StaffRequired = true
			}
			for _, k := range kinds {
				if !slices.Contains(b.ResourceKinds, k) {
					b.ResourceKinds = append(b.ResourceKinds, k)
				}
			}
			b.Items = append(b.Items, item)
		}
		for i, share := range model.Distribute(price, lists) {
			b.Items[i].AllocatedPrice = share
		}
		b.Discount = b.ListPrice.Sub(price).ClampZero()
		if len(b.Items) == 0 {
			b.Bookable = false
		}
		out = append(out, b)
	}
	return out, nil
}

func mapErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_service_bundles_name" {
		return ErrNameTaken
	}
	return err
}

func boolArg(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}
