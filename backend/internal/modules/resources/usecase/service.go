// Package usecase holds the resource business rules. Every method takes the
// organization id of the caller explicitly (from orgctx.Scope in handlers,
// from the job in workers).
//
// Branch scope: a resource belongs to one branch. A member limited to some
// branches only sees and manages the resources of those branches; any other
// resource reads as not found, and a branch outside the scope is the same
// validation error as an unknown branch.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// SearchSpec is the Cmd+K spec id of resources.
const SearchSpec = "resources"

// Limits of the resource fields.
const (
	maxCapacity       = 1000
	maxSortOrder      = 100000
	maxClosureReason  = 1000
	maxClosureSpan    = 366 * 24 * time.Hour
	closurePageLimit  = 200
	maxNameLength     = 120
	maxZoneLength     = 64
	maxDescriptionLen = 2000
)

var (
	ErrNotFound        = errors.New("resource not found")
	ErrClosureNotFound = errors.New("closed period not found")
	ErrNameTaken       = errors.New("the branch already has a resource with this name")
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

// Service implements the resource use cases.
type Service struct {
	repo    *repository.Postgres
	indexer SearchIndexer
}

// New builds the service. indexer may be nil.
func New(repo *repository.Postgres, indexer SearchIndexer) *Service {
	return &Service{repo: repo, indexer: indexer}
}

// SortColumns are the sortable list fields.
var SortColumns = map[string]string{
	"name": "name", "kind": "kind", "zone": "zone", "capacity": "capacity", "sort_order": "sort_order",
	"is_active": "is_active", "created_at": "created_at", "updated_at": "updated_at",
}

// listArgs are the shared filter arguments of the list queries.
type listArgs struct {
	scope, branches []int64
	kinds, zones    []string
	isActive        pgtype.Bool
	showOnline      pgtype.Bool
	q               pgtype.Text
}

func (s *Service) listArgs(ctx context.Context, q *db.Queries, orgID int64, f model.ListFilter) (listArgs, error) {
	branches, err := s.filterBranchIDs(ctx, q, orgID, f.Branches)
	if err != nil {
		return listArgs{}, err
	}
	return listArgs{
		scope: orgctx.BranchScope(ctx), branches: branches, kinds: nilIfEmpty(f.Kinds), zones: nilIfEmpty(f.Zones),
		isActive: boolArg(f.IsActive), showOnline: boolArg(f.ShowOnline), q: textArg(f.Q),
	}, nil
}

// List returns one page of the organization's resources. The default order
// is branch, sort order, name.
func (s *Service) List(ctx context.Context, orgID int64, f model.ListFilter) ([]model.Resource, int64, error) {
	if _, ok := SortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "", false
	}
	q := s.repo.Q()
	a, err := s.listArgs(ctx, q, orgID, f)
	if err != nil {
		return nil, 0, err
	}
	rows, err := q.ListResources(ctx, db.ListResourcesParams{
		OrganizationID: orgID, BranchIds: a.scope, FilterBranchIds: a.branches, Kinds: a.kinds, Zones: a.zones,
		IsActive: a.isActive, ShowOnline: a.showOnline, Q: a.q,
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountResources(ctx, db.CountResourcesParams{
		OrganizationID: orgID, BranchIds: a.scope, FilterBranchIds: a.branches, Kinds: a.kinds, Zones: a.zones,
		IsActive: a.isActive, ShowOnline: a.showOnline, Q: a.q,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Resource, 0, len(rows))
	for _, row := range rows {
		out = append(out, MapResource(db.GetResourceRow(row)))
	}
	return out, total, nil
}

// Facets returns the branches, kinds and zones in use with their counts.
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	q := s.repo.Q()
	scope := orgctx.BranchScope(ctx)
	out := model.Facets{Branch: []model.Facet{}, Kind: []model.Facet{}, Zone: []model.Facet{}}
	branches, err := q.ListResourceBranchFacets(ctx, db.ListResourceBranchFacetsParams{OrganizationID: orgID, BranchIds: scope})
	if err != nil {
		return out, err
	}
	for _, row := range branches {
		out.Branch = append(out.Branch, model.Facet{Value: row.Uuid.String(), Label: row.Name, Count: row.Count})
	}
	kinds, err := q.ListResourceKindFacets(ctx, db.ListResourceKindFacetsParams{OrganizationID: orgID, BranchIds: scope})
	if err != nil {
		return out, err
	}
	for _, row := range kinds {
		out.Kind = append(out.Kind, model.Facet{Value: row.Value, Count: row.Count})
	}
	zones, err := q.ListResourceZoneFacets(ctx, db.ListResourceZoneFacetsParams{OrganizationID: orgID, BranchIds: scope})
	if err != nil {
		return out, err
	}
	for _, row := range zones {
		out.Zone = append(out.Zone, model.Facet{Value: row.Value, Count: row.Count})
	}
	return out, nil
}

// Get returns one resource with its branch's hours.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Resource, error) {
	row, err := s.load(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return model.Resource{}, err
	}
	r := MapResource(row)
	branch, err := s.repo.Q().GetBranchByUUID(ctx, db.GetBranchByUUIDParams{OrganizationID: orgID, Uuid: row.BranchUuid})
	if err == nil {
		r.BranchHours = json.RawMessage(branch.WorkingHours)
	}
	return r, nil
}

// Create validates and stores a new resource.
func (s *Service) Create(ctx context.Context, orgID int64, in model.Input) (model.Resource, error) {
	f := fields{MinCapacity: 1, Capacity: 1, ShowOnline: true, IsActive: true}
	ve := &ValidationError{}
	q := s.repo.Q()
	if in.BranchUUID == nil {
		ve.add("branch_uuid", "required", "branch_uuid is required")
	} else if err := s.resolveBranch(ctx, q, orgID, *in.BranchUUID, &f, ve); err != nil {
		return model.Resource{}, err
	}
	f.apply(ve, in, true)
	if err := ve.orNil(); err != nil {
		return model.Resource{}, err
	}
	if err := s.checkName(ctx, q, orgID, f, nil); err != nil {
		return model.Resource{}, err
	}
	row, err := q.CreateResource(ctx, db.CreateResourceParams{
		OrganizationID: orgID, BranchID: f.BranchID, Kind: f.Kind, Name: f.Name, Zone: f.Zone,
		Description: f.Description, MinCapacity: f.MinCapacity, Capacity: f.Capacity, Combinable: f.Combinable,
		ShowOnline: f.ShowOnline, IsActive: f.IsActive, SortOrder: f.SortOrder, WorkingHours: f.hoursJSON(),
	})
	if err != nil {
		return model.Resource{}, mapNameTaken(err)
	}
	s.index(ctx, orgID, row.Uuid)
	return s.Get(ctx, orgID, row.Uuid)
}

// Patch updates the given fields. Moving a resource to another branch needs
// both branches inside the caller's scope.
func (s *Service) Patch(ctx context.Context, orgID int64, id uuid.UUID, in model.Input) (model.Resource, error) {
	q := s.repo.Q()
	row, err := s.load(ctx, q, orgID, id)
	if err != nil {
		return model.Resource{}, err
	}
	f := fieldsFromRow(row)
	ve := &ValidationError{}
	if in.BranchUUID != nil && *in.BranchUUID != row.BranchUuid {
		if err := s.resolveBranch(ctx, q, orgID, *in.BranchUUID, &f, ve); err != nil {
			return model.Resource{}, err
		}
	}
	f.apply(ve, in, false)
	if err := ve.orNil(); err != nil {
		return model.Resource{}, err
	}
	if err := s.checkName(ctx, q, orgID, f, &row.ID); err != nil {
		return model.Resource{}, err
	}
	if _, err := q.UpdateResource(ctx, db.UpdateResourceParams{
		OrganizationID: orgID, ID: row.ID, BranchID: f.BranchID, Kind: f.Kind, Name: f.Name, Zone: f.Zone,
		Description: f.Description, MinCapacity: f.MinCapacity, Capacity: f.Capacity, Combinable: f.Combinable,
		ShowOnline: f.ShowOnline, IsActive: f.IsActive, SortOrder: f.SortOrder, WorkingHours: f.hoursJSON(),
	}); err != nil {
		return model.Resource{}, mapNotFound(mapNameTaken(err), ErrNotFound)
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
		if err := s.repo.Q().SetResourceActive(ctx, db.SetResourceActiveParams{OrganizationID: orgID, ID: row.ID, IsActive: active}); err != nil {
			return row.IsActive, err
		}
		s.index(ctx, orgID, id)
	}
	return row.IsActive, nil
}

// Delete soft-deletes a resource. Its name becomes free in the branch.
func (s *Service) Delete(ctx context.Context, orgID int64, id uuid.UUID) (model.Resource, error) {
	row, err := s.load(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return model.Resource{}, err
	}
	n, err := s.repo.Q().SoftDeleteResource(ctx, db.SoftDeleteResourceParams{OrganizationID: orgID, ID: row.ID})
	if err != nil {
		return model.Resource{}, err
	}
	if n == 0 {
		return model.Resource{}, ErrNotFound
	}
	if s.indexer != nil {
		s.indexer.EnqueueDelete(ctx, SearchSpec, searchengine.TenantDocumentID(orgID, id.String()))
	}
	return MapResource(row), nil
}

// TargetUUIDs resolves the resources matching a list filter (bulk "all
// matching" scope), capped at limit.
func (s *Service) TargetUUIDs(ctx context.Context, orgID int64, f model.ListFilter, limit int32) ([]uuid.UUID, error) {
	q := s.repo.Q()
	a, err := s.listArgs(ctx, q, orgID, f)
	if err != nil {
		return nil, err
	}
	return q.ListResourceUUIDs(ctx, db.ListResourceUUIDsParams{
		OrganizationID: orgID, BranchIds: a.scope, FilterBranchIds: a.branches, Kinds: a.kinds, Zones: a.zones,
		IsActive: a.isActive, ShowOnline: a.showOnline, Q: a.q, LimitCount: limit,
	})
}

// ExportRows returns every resource matching a list filter.
func (s *Service) ExportRows(ctx context.Context, orgID int64, f model.ListFilter) ([]model.Resource, error) {
	q := s.repo.Q()
	a, err := s.listArgs(ctx, q, orgID, f)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListResourcesForExport(ctx, db.ListResourcesForExportParams{
		OrganizationID: orgID, BranchIds: a.scope, FilterBranchIds: a.branches, Kinds: a.kinds, Zones: a.zones,
		IsActive: a.isActive, ShowOnline: a.showOnline, Q: a.q,
	})
	if err != nil {
		return nil, err
	}
	out := make([]model.Resource, 0, len(rows))
	for _, row := range rows {
		out = append(out, MapResource(db.GetResourceRow(row)))
	}
	return out, nil
}

// ImportRow updates the resource of the branch with the same name, or
// creates one. branchCode names the branch. It returns the previous state
// of an update.
func (s *Service) ImportRow(ctx context.Context, orgID int64, branchCode string, in model.Input) (uuid.UUID, bool, *model.Resource, error) {
	q := s.repo.Q()
	branch, err := q.GetBranchByCode(ctx, db.GetBranchByCodeParams{OrganizationID: orgID, Code: strings.ToUpper(strings.TrimSpace(branchCode))})
	if err != nil {
		if errors.Is(repository.MapErr(err), repository.ErrNotFound) {
			return uuid.Nil, false, nil, &ValidationError{Details: []Detail{{Field: "branch_code", Code: "not_found", Message: "unknown branch code " + branchCode}}}
		}
		return uuid.Nil, false, nil, err
	}
	in.BranchUUID = &branch.Uuid
	if in.Name != nil {
		existing, err := q.GetResourceByBranchName(ctx, db.GetResourceByBranchNameParams{
			OrganizationID: orgID, BranchID: branch.ID, Name: strings.TrimSpace(*in.Name),
		})
		if err == nil {
			prev, err := s.Get(ctx, orgID, existing.Uuid)
			if err != nil {
				return uuid.Nil, false, nil, err
			}
			r, err := s.Patch(ctx, orgID, existing.Uuid, in)
			if err != nil {
				return uuid.Nil, false, nil, err
			}
			return r.UUID, false, &prev, nil
		}
		if !errors.Is(repository.MapErr(err), repository.ErrNotFound) {
			return uuid.Nil, false, nil, err
		}
	}
	r, err := s.Create(ctx, orgID, in)
	if err != nil {
		return uuid.Nil, false, nil, err
	}
	return r.UUID, true, nil, nil
}

// Activity lists the audit events of a resource.
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

// load reads one resource; resources outside the caller's branch scope read
// as not found.
func (s *Service) load(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (db.GetResourceRow, error) {
	row, err := q.GetResource(ctx, db.GetResourceParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return db.GetResourceRow{}, mapNotFound(repository.MapErr(err), ErrNotFound)
	}
	if orgctx.CheckBranch(ctx, row.BranchID) != nil {
		return db.GetResourceRow{}, ErrNotFound
	}
	return row, nil
}

// resolveBranch sets f.BranchID from a branch uuid of the organization.
// Unknown branches and branches outside the caller's scope are the same
// validation error.
func (s *Service) resolveBranch(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID, f *fields, ve *ValidationError) error {
	rows, err := q.ListBranchIDsByUUIDs(ctx, db.ListBranchIDsByUUIDsParams{OrganizationID: orgID, Uuids: []uuid.UUID{id}})
	if err != nil {
		return err
	}
	if len(rows) == 0 || orgctx.CheckBranch(ctx, rows[0].ID) != nil {
		ve.add("branch_uuid", "not_found", "branch not found")
		return nil
	}
	f.BranchID = rows[0].ID
	return nil
}

// checkName answers ErrNameTaken when the branch has another live resource
// with the name (the unique index is the backstop).
func (s *Service) checkName(ctx context.Context, q *db.Queries, orgID int64, f fields, exclude *int64) error {
	excl := pgtype.Int8{}
	if exclude != nil {
		excl = pgtype.Int8{Int64: *exclude, Valid: true}
	}
	taken, err := q.ResourceNameTaken(ctx, db.ResourceNameTakenParams{OrganizationID: orgID, BranchID: f.BranchID, Name: f.Name, ExcludeID: excl})
	if err != nil {
		return err
	}
	if taken {
		return ErrNameTaken
	}
	return nil
}

// filterBranchIDs resolves the branch filter of a list; nil = no filter,
// unknown branches match nothing.
func (s *Service) filterBranchIDs(ctx context.Context, q *db.Queries, orgID int64, ids []uuid.UUID) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := q.ListBranchIDsByUUIDs(ctx, db.ListBranchIDsByUUIDsParams{OrganizationID: orgID, Uuids: ids})
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ID)
	}
	return out, nil
}

func (s *Service) index(ctx context.Context, orgID int64, id uuid.UUID) {
	if s.indexer != nil {
		s.indexer.EnqueueUpsert(ctx, SearchSpec, searchengine.TenantDocumentID(orgID, id.String()))
	}
}

// fields are the writable columns of a resource.
type fields struct {
	BranchID                      int64
	Kind, Name, Zone, Description string
	MinCapacity, Capacity         int32
	Combinable, ShowOnline        bool
	IsActive                      bool
	SortOrder                     int32
	Hours                         *model.WeeklyHours
}

func fieldsFromRow(row db.GetResourceRow) fields {
	return fields{
		BranchID: row.BranchID, Kind: row.Kind, Name: row.Name, Zone: row.Zone, Description: row.Description,
		MinCapacity: row.MinCapacity, Capacity: row.Capacity, Combinable: row.Combinable,
		ShowOnline: row.ShowOnline, IsActive: row.IsActive, SortOrder: row.SortOrder, Hours: decodeHours(row.WorkingHours),
	}
}

func (f *fields) hoursJSON() []byte {
	if f.Hours == nil {
		return nil
	}
	b, _ := json.Marshal(f.Hours)
	return b
}

var timeRE = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

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
	if in.Kind != nil {
		if !slices.Contains(model.Kinds, *in.Kind) {
			ve.add("kind", "invalid", "kind must be one of "+strings.Join(model.Kinds, ", "))
		} else {
			f.Kind = *in.Kind
		}
	} else if create {
		ve.add("kind", "required", "kind is required")
	}
	text("name", in.Name, &f.Name, maxNameLength, true)
	text("zone", in.Zone, &f.Zone, maxZoneLength, false)
	text("description", in.Description, &f.Description, maxDescriptionLen, false)
	if in.MinCapacity != nil {
		f.MinCapacity = *in.MinCapacity
	}
	if in.Capacity != nil {
		f.Capacity = *in.Capacity
		// A capacity alone lowers the minimum with it.
		if in.MinCapacity == nil && f.MinCapacity > f.Capacity && f.Capacity >= 1 {
			f.MinCapacity = f.Capacity
		}
	}
	switch {
	case f.MinCapacity < 1 || f.MinCapacity > maxCapacity:
		ve.add("min_capacity", "invalid", fmt.Sprintf("min_capacity must be between 1 and %d", maxCapacity))
	case f.Capacity < 1 || f.Capacity > maxCapacity:
		ve.add("capacity", "invalid", fmt.Sprintf("capacity must be between 1 and %d", maxCapacity))
	case f.Capacity < f.MinCapacity:
		ve.add("capacity", "below_min", "capacity must be at least min_capacity")
	}
	if in.Combinable != nil {
		f.Combinable = *in.Combinable
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
	if in.WorkingHours.Set {
		if in.WorkingHours.Value == nil {
			f.Hours = nil
		} else {
			hours := *in.WorkingHours.Value
			for _, day := range hours.Days() {
				validateDay(ve, "working_hours."+day.Key, day.Hours)
			}
			f.Hours = &hours
		}
	}
}

// validateDay checks one weekday of own hours (same rules as the branch).
func validateDay(ve *ValidationError, prefix string, d *branchmodel.DayHours) {
	if d.Closed {
		d.Open, d.Close = "", ""
		return
	}
	d.Open, d.Close = strings.TrimSpace(d.Open), strings.TrimSpace(d.Close)
	okOpen, okClose := timeRE.MatchString(d.Open), timeRE.MatchString(d.Close)
	if !okOpen {
		ve.add(prefix+".open", "invalid", "opening time must be HH:MM")
	}
	if !okClose {
		ve.add(prefix+".close", "invalid", "closing time must be HH:MM")
	}
	if okOpen && okClose && d.Open >= d.Close {
		ve.add(prefix+".close", "before_open", "closing time must be after opening time")
	}
}

// MapResource maps a resource row (list, detail and export share the columns).
func MapResource(row db.GetResourceRow) model.Resource {
	return model.Resource{
		UUID:   row.Uuid,
		Branch: model.BranchRef{UUID: row.BranchUuid, Name: row.BranchName, Code: row.BranchCode},
		Kind:   row.Kind, Name: row.Name, Zone: row.Zone, Description: row.Description,
		MinCapacity: row.MinCapacity, Capacity: row.Capacity, Combinable: row.Combinable,
		ShowOnline: row.ShowOnline, IsActive: row.IsActive, SortOrder: row.SortOrder,
		WorkingHours: decodeHours(row.WorkingHours),
		CreatedAt:    row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func decodeHours(raw []byte) *model.WeeklyHours {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var w model.WeeklyHours
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil
	}
	return &w
}

func mapNotFound(err, target error) error {
	if errors.Is(err, repository.ErrNotFound) {
		return target
	}
	return err
}

func mapNameTaken(err error) error {
	if errors.Is(repository.MapErr(err), repository.ErrNameTaken) {
		return ErrNameTaken
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
