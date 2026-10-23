// Package usecase holds the staff business rules. Every method takes the
// organization id of the caller explicitly (from orgctx.Scope in handlers,
// from the job in workers).
//
// Branch scope: a member limited to some branches sees the staff assigned
// to one of them, manages only those assignments and the shifts there, and
// deletes a person only when every assignment is inside the scope.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// SearchSpec is the Cmd+K spec id of staff.
const SearchSpec = "staff"

// DefaultColor is the calendar colour of a new staff member.
const DefaultColor = "#2563EB"

var (
	ErrNotFound           = errors.New("staff member not found")
	ErrTimeOffNotFound    = errors.New("time off not found")
	ErrAssignmentNotFound = errors.New("the staff member does not work in this branch")
	ErrAccountTaken       = errors.New("this account already has a staff record")
	// ErrOutOfScope is returned when a branch-limited member deletes a
	// person who also works in branches outside the member's scope.
	ErrOutOfScope = errors.New("the staff member also works in branches outside your access")
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

// SearchIndexer schedules Cmd+K index updates (fail-soft).
type SearchIndexer interface {
	EnqueueUpsert(ctx context.Context, spec, id string)
	EnqueueDelete(ctx context.Context, spec, id string)
}

// StaffLimiter enforces the staff seat limit of the plan (TEC-883). Staff
// without an account take a seat; a staff record linked to a member uses
// the member's seat.
type StaffLimiter interface {
	CheckStaffAdd(ctx context.Context, q *db.Queries, scope orgctx.Scope) error
}

// Service implements the staff use cases.
type Service struct {
	repo    *repository.Postgres
	indexer SearchIndexer
	limits  StaffLimiter
}

// New builds the service. indexer may be nil.
func New(repo *repository.Postgres, indexer SearchIndexer) *Service {
	return &Service{repo: repo, indexer: indexer}
}

// SetLimiter attaches the plan seat check.
func (s *Service) SetLimiter(l StaffLimiter) {
	s.limits = l
}

// SortColumns are the sortable list fields.
var SortColumns = map[string]string{
	"name": "name", "title": "title", "is_active": "is_active",
	"created_at": "created_at", "updated_at": "updated_at",
}

// List returns one page of the organization's staff.
func (s *Service) List(ctx context.Context, orgID int64, f model.ListFilter) ([]model.Staff, int64, error) {
	if _, ok := SortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "created_at", true
	}
	q := s.repo.Q()
	filterIDs, err := s.filterBranchIDs(ctx, q, orgID, f.Branches)
	if err != nil {
		return nil, 0, err
	}
	scope := orgctx.BranchScope(ctx)
	rows, err := q.ListStaff(ctx, db.ListStaffParams{
		OrganizationID: orgID, BranchIds: scope, FilterBranchIds: filterIDs,
		IsActive: boolArg(f.IsActive), ShowOnline: boolArg(f.ShowOnline), HasAccount: boolArg(f.HasAccount),
		Q: textArg(f.Q), SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountStaff(ctx, db.CountStaffParams{
		OrganizationID: orgID, BranchIds: scope, FilterBranchIds: filterIDs,
		IsActive: boolArg(f.IsActive), ShowOnline: boolArg(f.ShowOnline), HasAccount: boolArg(f.HasAccount),
		Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Staff, 0, len(rows))
	for _, row := range rows {
		out = append(out, MapStaff(db.GetStaffRow(row)))
	}
	return out, total, nil
}

// Facets returns the branches in use with their staff count.
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	rows, err := s.repo.Q().ListStaffBranchFacets(ctx, db.ListStaffBranchFacetsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx),
	})
	if err != nil {
		return model.Facets{}, err
	}
	out := model.Facets{Branch: make([]model.BranchFacet, 0, len(rows))}
	for _, row := range rows {
		out.Branch = append(out.Branch, model.BranchFacet{Value: row.Uuid.String(), Label: row.Name, Count: row.Count})
	}
	return out, nil
}

// Get returns one staff member with the branch assignments and shifts.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Staff, error) {
	row, assignments, err := s.load(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return model.Staff{}, err
	}
	st := MapStaff(row)
	st.Assignments = mapAssignments(ctx, assignments)
	return st, nil
}

// LinkableMembers lists the members of the business without a staff record.
func (s *Service) LinkableMembers(ctx context.Context, orgID int64) ([]model.LinkableMember, error) {
	rows, err := s.repo.Q().ListLinkableMembers(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]model.LinkableMember, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.LinkableMember{UUID: row.Uuid, Name: row.Name, Surname: row.Surname, Email: row.Email})
	}
	return out, nil
}

// Create validates and stores a new staff member with its branches.
func (s *Service) Create(ctx context.Context, orgID int64, in model.StaffInput) (model.Staff, error) {
	f := staffFields{Color: DefaultColor, ShowOnline: true, IsActive: true}
	ve := &ValidationError{}
	f.apply(ve, in, true)
	var created uuid.UUID
	err := s.repo.InTx(ctx, func(q *db.Queries) error {
		if in.AccountUUID.Set && in.AccountUUID.Value != nil {
			userID, err := s.resolveAccount(ctx, q, orgID, *in.AccountUUID.Value, nil, ve)
			if err != nil {
				return err
			}
			f.UserID = userID
		}
		var requested []uuid.UUID
		if in.BranchUUIDs != nil {
			requested = *in.BranchUUIDs
		}
		branchIDs, err := s.resolveBranches(ctx, q, orgID, requested, ve)
		if err != nil {
			return err
		}
		if len(branchIDs) == 0 && !allBranches(ctx) {
			ve.add("branch_uuids", "required", "pick at least one branch")
		}
		if err := ve.orNil(); err != nil {
			return err
		}
		if !f.UserID.Valid {
			if err := s.checkSeat(ctx, q); err != nil {
				return err
			}
		}
		row, err := q.CreateStaff(ctx, db.CreateStaffParams{
			OrganizationID: orgID, UserID: f.UserID, Name: f.Name, Title: f.Title, Bio: f.Bio,
			Phone: f.Phone, Email: f.Email, Color: f.Color, ShowOnline: f.ShowOnline, IsActive: f.IsActive,
		})
		if err != nil {
			return mapAccountTaken(err)
		}
		for _, branchID := range branchIDs {
			if err := q.AddStaffBranch(ctx, db.AddStaffBranchParams{OrganizationID: orgID, StaffID: row.ID, BranchID: branchID}); err != nil {
				return err
			}
		}
		created = row.Uuid
		return nil
	})
	if err != nil {
		return model.Staff{}, err
	}
	s.index(ctx, orgID, created)
	return s.Get(ctx, orgID, created)
}

// Patch updates the given fields and, with branch_uuids, the assignments
// the caller may manage.
func (s *Service) Patch(ctx context.Context, orgID int64, id uuid.UUID, in model.StaffInput) (model.Staff, error) {
	err := s.repo.InTx(ctx, func(q *db.Queries) error {
		row, assignments, err := s.load(ctx, q, orgID, id)
		if err != nil {
			return err
		}
		f := fieldsFromRow(row)
		ve := &ValidationError{}
		f.apply(ve, in, false)
		if in.AccountUUID.Set {
			if in.AccountUUID.Value == nil {
				// Unlinking turns the record into staff without an account,
				// which takes a seat of its own.
				if row.AccountUuid.Valid {
					if err := s.checkSeat(ctx, q); err != nil {
						return err
					}
				}
				f.UserID = pgtype.Int8{}
			} else {
				userID, err := s.resolveAccount(ctx, q, orgID, *in.AccountUUID.Value, &id, ve)
				if err != nil {
					return err
				}
				f.UserID = userID
			}
		}
		current := make([]int64, 0, len(assignments))
		for _, a := range assignments {
			current = append(current, a.BranchID)
		}
		final := current
		if in.BranchUUIDs != nil {
			requested, err := s.resolveBranches(ctx, q, orgID, *in.BranchUUIDs, ve)
			if err != nil {
				return err
			}
			// Assignments outside a limited caller's scope stay as they are.
			final = requested
			for _, branchID := range current {
				if !allowsBranch(ctx, branchID) && !slices.Contains(final, branchID) {
					final = append(final, branchID)
				}
			}
			if len(final) == 0 && !allBranches(ctx) {
				ve.add("branch_uuids", "required", "pick at least one branch")
			}
		}
		if err := ve.orNil(); err != nil {
			return err
		}
		if _, err := q.UpdateStaff(ctx, db.UpdateStaffParams{
			OrganizationID: orgID, Uuid: id, UserID: f.UserID, Name: f.Name, Title: f.Title, Bio: f.Bio,
			Phone: f.Phone, Email: f.Email, Color: f.Color, ShowOnline: f.ShowOnline, IsActive: f.IsActive,
		}); err != nil {
			return mapNotFound(mapAccountTaken(err), ErrNotFound)
		}
		for _, branchID := range current {
			if !slices.Contains(final, branchID) {
				if err := q.RemoveStaffBranch(ctx, db.RemoveStaffBranchParams{OrganizationID: orgID, StaffID: row.ID, BranchID: branchID}); err != nil {
					return err
				}
			}
		}
		for _, branchID := range final {
			if !slices.Contains(current, branchID) {
				if err := q.AddStaffBranch(ctx, db.AddStaffBranchParams{OrganizationID: orgID, StaffID: row.ID, BranchID: branchID}); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return model.Staff{}, err
	}
	s.index(ctx, orgID, id)
	return s.Get(ctx, orgID, id)
}

// SetActive switches is_active and returns the previous value (bulk engine).
func (s *Service) SetActive(ctx context.Context, orgID int64, id uuid.UUID, active bool) (bool, error) {
	row, _, err := s.load(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return false, err
	}
	if row.IsActive != active {
		if err := s.repo.Q().SetStaffActive(ctx, db.SetStaffActiveParams{OrganizationID: orgID, ID: row.ID, IsActive: active}); err != nil {
			return row.IsActive, err
		}
		s.index(ctx, orgID, id)
	}
	return row.IsActive, nil
}

// Delete soft-deletes a staff member. A branch-limited caller deletes only
// people whose every branch is inside the scope.
func (s *Service) Delete(ctx context.Context, orgID int64, id uuid.UUID) error {
	row, assignments, err := s.load(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return err
	}
	for _, a := range assignments {
		if !allowsBranch(ctx, a.BranchID) {
			return ErrOutOfScope
		}
	}
	n, err := s.repo.Q().SoftDeleteStaff(ctx, db.SoftDeleteStaffParams{OrganizationID: orgID, ID: row.ID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	if s.indexer != nil {
		s.indexer.EnqueueDelete(ctx, SearchSpec, searchengine.TenantDocumentID(orgID, id.String()))
	}
	return nil
}

// TargetUUIDs resolves the staff matching a list filter (bulk "all
// matching" scope), capped at limit.
func (s *Service) TargetUUIDs(ctx context.Context, orgID int64, f model.ListFilter, limit int32) ([]uuid.UUID, error) {
	q := s.repo.Q()
	filterIDs, err := s.filterBranchIDs(ctx, q, orgID, f.Branches)
	if err != nil {
		return nil, err
	}
	return q.ListStaffUUIDs(ctx, db.ListStaffUUIDsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), FilterBranchIds: filterIDs,
		IsActive: boolArg(f.IsActive), ShowOnline: boolArg(f.ShowOnline), HasAccount: boolArg(f.HasAccount),
		Q: textArg(f.Q), LimitCount: limit,
	})
}

// ExportRows returns every staff member matching a list filter.
func (s *Service) ExportRows(ctx context.Context, orgID int64, f model.ListFilter) ([]model.Staff, error) {
	q := s.repo.Q()
	filterIDs, err := s.filterBranchIDs(ctx, q, orgID, f.Branches)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListStaffForExport(ctx, db.ListStaffForExportParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), FilterBranchIds: filterIDs,
		IsActive: boolArg(f.IsActive), ShowOnline: boolArg(f.ShowOnline), HasAccount: boolArg(f.HasAccount),
		Q: textArg(f.Q),
	})
	if err != nil {
		return nil, err
	}
	out := make([]model.Staff, 0, len(rows))
	for _, row := range rows {
		out = append(out, MapStaff(db.GetStaffRow(row)))
	}
	return out, nil
}

// ImportRow updates the staff member with the given uuid, or creates one
// when id is nil or unknown. It returns the previous state of an update.
func (s *Service) ImportRow(ctx context.Context, orgID int64, id *uuid.UUID, in model.StaffInput) (uuid.UUID, bool, *model.Staff, error) {
	if id != nil {
		prev, err := s.Get(ctx, orgID, *id)
		if err == nil {
			st, err := s.Patch(ctx, orgID, *id, in)
			if err != nil {
				return uuid.Nil, false, nil, err
			}
			return st.UUID, false, &prev, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return uuid.Nil, false, nil, err
		}
	}
	st, err := s.Create(ctx, orgID, in)
	if err != nil {
		return uuid.Nil, false, nil, err
	}
	return st.UUID, true, nil, nil
}

// BranchUUIDsByCodes resolves branch codes of the organization (import).
func (s *Service) BranchUUIDsByCodes(ctx context.Context, orgID int64, codes []string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(codes))
	for _, code := range codes {
		b, err := s.repo.Q().GetBranchByCode(ctx, db.GetBranchByCodeParams{OrganizationID: orgID, Code: strings.ToUpper(code)})
		if err != nil {
			if errors.Is(repository.MapErr(err), repository.ErrNotFound) {
				return nil, &ValidationError{Details: []Detail{{Field: "branches", Code: "not_found", Message: "unknown branch code " + code}}}
			}
			return nil, err
		}
		out = append(out, b.Uuid)
	}
	return out, nil
}

// Activity lists the audit events of a staff member.
func (s *Service) Activity(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]model.ActivityEntry, int64, error) {
	if _, _, err := s.load(ctx, s.repo.Q(), orgID, id); err != nil {
		return nil, 0, err
	}
	q := s.repo.Q()
	rows, err := q.ListOrganizationResourceActivity(ctx, db.ListOrganizationResourceActivityParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, Resource: model.Resource,
		ResourceUuid: pgtype.UUID{Bytes: id, Valid: true}, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountOrganizationResourceActivity(ctx, db.CountOrganizationResourceActivityParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, Resource: model.Resource,
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

// load reads one staff member with the assignments; staff outside the
// caller's branch scope read as not found.
func (s *Service) load(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (db.GetStaffRow, []db.ListStaffAssignmentsRow, error) {
	row, err := q.GetStaff(ctx, db.GetStaffParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return db.GetStaffRow{}, nil, mapNotFound(repository.MapErr(err), ErrNotFound)
	}
	assignments, err := q.ListStaffAssignments(ctx, db.ListStaffAssignmentsParams{OrganizationID: orgID, StaffID: row.ID})
	if err != nil {
		return db.GetStaffRow{}, nil, err
	}
	if !allBranches(ctx) {
		visible := false
		for _, a := range assignments {
			if allowsBranch(ctx, a.BranchID) {
				visible = true
				break
			}
		}
		if !visible {
			return db.GetStaffRow{}, nil, ErrNotFound
		}
	}
	return row, assignments, nil
}

// resolveAccount resolves a member account of the organization; an account
// that already has another staff record is ErrAccountTaken.
func (s *Service) resolveAccount(ctx context.Context, q *db.Queries, orgID int64, account uuid.UUID, exclude *uuid.UUID, ve *ValidationError) (pgtype.Int8, error) {
	user, err := q.GetMemberUserByUUID(ctx, db.GetMemberUserByUUIDParams{OrganizationID: orgID, Uuid: account})
	if err != nil {
		if errors.Is(repository.MapErr(err), repository.ErrNotFound) {
			ve.add("account_uuid", "not_found", "account is not a member of this business")
			return pgtype.Int8{}, nil
		}
		return pgtype.Int8{}, err
	}
	excl := pgtype.UUID{}
	if exclude != nil {
		excl = pgtype.UUID{Bytes: *exclude, Valid: true}
	}
	taken, err := q.StaffAccountTaken(ctx, db.StaffAccountTakenParams{OrganizationID: orgID, UserID: pgtype.Int8{Int64: user.ID, Valid: true}, ExcludeUuid: excl})
	if err != nil {
		return pgtype.Int8{}, err
	}
	if taken {
		return pgtype.Int8{}, ErrAccountTaken
	}
	return pgtype.Int8{Int64: user.ID, Valid: true}, nil
}

// resolveBranches maps branch uuids to ids of the organization. Unknown
// branches and branches outside the caller's scope are the same
// validation error.
func (s *Service) resolveBranches(ctx context.Context, q *db.Queries, orgID int64, ids []uuid.UUID, ve *ValidationError) ([]int64, error) {
	if len(ids) == 0 {
		return []int64{}, nil
	}
	rows, err := q.ListBranchIDsByUUIDs(ctx, db.ListBranchIDsByUUIDsParams{OrganizationID: orgID, Uuids: ids})
	if err != nil {
		return nil, err
	}
	byUUID := make(map[uuid.UUID]int64, len(rows))
	for _, row := range rows {
		byUUID[row.Uuid] = row.ID
	}
	out := make([]int64, 0, len(ids))
	for i, id := range ids {
		branchID, ok := byUUID[id]
		if !ok || !allowsBranch(ctx, branchID) {
			ve.add(fmt.Sprintf("branch_uuids.%d", i), "not_found", "branch not found")
			continue
		}
		if !slices.Contains(out, branchID) {
			out = append(out, branchID)
		}
	}
	return out, nil
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

func (s *Service) checkSeat(ctx context.Context, q *db.Queries) error {
	if s.limits == nil {
		return nil
	}
	scope, ok := orgctx.From(ctx)
	if !ok {
		return nil
	}
	return s.limits.CheckStaffAdd(ctx, q, scope)
}

func (s *Service) index(ctx context.Context, orgID int64, id uuid.UUID) {
	if s.indexer != nil {
		s.indexer.EnqueueUpsert(ctx, SearchSpec, searchengine.TenantDocumentID(orgID, id.String()))
	}
}

// staffFields are the writable columns of a staff record.
type staffFields struct {
	UserID                                pgtype.Int8
	Name, Title, Bio, Phone, Email, Color string
	ShowOnline, IsActive                  bool
}

func fieldsFromRow(row db.GetStaffRow) staffFields {
	return staffFields{
		UserID: row.UserID, Name: row.Name, Title: row.Title, Bio: row.Bio, Phone: row.Phone,
		Email: row.Email, Color: row.Color, ShowOnline: row.ShowOnline, IsActive: row.IsActive,
	}
}

var colorRE = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

func (f *staffFields) apply(ve *ValidationError, in model.StaffInput, create bool) {
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
	text("name", in.Name, &f.Name, 120, true)
	text("title", in.Title, &f.Title, 120, false)
	text("bio", in.Bio, &f.Bio, 2000, false)
	text("phone", in.Phone, &f.Phone, 32, false)
	text("email", in.Email, &f.Email, 255, false)
	if in.Email != nil && f.Email != "" {
		if addr, err := mail.ParseAddress(f.Email); err != nil || addr.Address != f.Email {
			ve.add("email", "invalid", "email is invalid")
		}
	}
	if in.Color != nil {
		v := strings.ToUpper(strings.TrimSpace(*in.Color))
		if !colorRE.MatchString(v) {
			ve.add("color", "invalid", "color must be #RRGGBB")
		} else {
			f.Color = v
		}
	}
	if in.ShowOnline != nil {
		f.ShowOnline = *in.ShowOnline
	}
	if in.IsActive != nil {
		f.IsActive = *in.IsActive
	}
}

// MapStaff maps a staff row (list, detail and export share the columns).
func MapStaff(row db.GetStaffRow) model.Staff {
	st := model.Staff{
		UUID: row.Uuid, Name: row.Name, Title: row.Title, Bio: row.Bio, Phone: row.Phone, Email: row.Email,
		Color: row.Color, ShowOnline: row.ShowOnline, IsActive: row.IsActive, Branches: []model.BranchRef{},
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if row.AccountUuid.Valid {
		st.Account = &model.Account{
			UUID: uuid.UUID(row.AccountUuid.Bytes), Name: row.AccountName.String,
			Surname: row.AccountSurname.String, Email: row.AccountEmail.String,
		}
	}
	if len(row.Branches) > 0 {
		_ = json.Unmarshal(row.Branches, &st.Branches)
	}
	return st
}

func mapAssignments(ctx context.Context, rows []db.ListStaffAssignmentsRow) []model.Assignment {
	out := make([]model.Assignment, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.Assignment{
			BranchUUID: row.BranchUuid, BranchName: row.BranchName, BranchCode: row.BranchCode,
			BranchHours: json.RawMessage(row.BranchWorkingHours), Shifts: decodeShifts(row.Shifts),
			Editable: allowsBranch(ctx, row.BranchID),
		})
	}
	return out
}

func decodeShifts(raw []byte) model.WeeklyShifts {
	var w model.WeeklyShifts
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &w)
	}
	w.Normalize()
	return w
}

func allBranches(ctx context.Context) bool {
	scope, ok := orgctx.From(ctx)
	return !ok || scope.AllBranches()
}

func allowsBranch(ctx context.Context, branchID int64) bool {
	return orgctx.CheckBranch(ctx, branchID) == nil
}

func mapNotFound(err, target error) error {
	if errors.Is(err, repository.ErrNotFound) {
		return target
	}
	return err
}

func mapAccountTaken(err error) error {
	if errors.Is(repository.MapErr(err), repository.ErrAccountTaken) {
		return ErrAccountTaken
	}
	return err
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
