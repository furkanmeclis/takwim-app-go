// Package usecase holds the branches business rules. Every method takes the
// organization id of the caller explicitly (from orgctx.Scope in handlers,
// from the job in workers) and passes it to the repository.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/mail"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// SearchSpec is the Cmd+K spec id of branches.
const SearchSpec = "branches"

var (
	ErrNotFound             = errors.New("branch not found")
	ErrServicePointNotFound = errors.New("service point not found")
	ErrCodeTaken            = errors.New("branch code is already used in this organization")
	ErrSlugTaken            = errors.New("branch slug is already used in this organization")
	// ErrAllBranchesRequired is returned when a branch-restricted member
	// tries an action that needs every branch (creating a branch).
	ErrAllBranchesRequired = errors.New("this action needs access to every branch")
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

// Repository is the persistence port. Every method is scoped by orgID.
type Repository interface {
	ListBranches(ctx context.Context, orgID int64, f model.ListFilter) ([]repository.BranchRow, int64, error)
	CityFacets(ctx context.Context, orgID int64, branchIDs []int64) ([]model.Facet, error)
	ListBranchUUIDs(ctx context.Context, orgID int64, f model.ListFilter, limit int32) ([]uuid.UUID, error)
	ListBranchesForExport(ctx context.Context, orgID int64, f model.ListFilter) ([]db.Branch, error)
	GetBranch(ctx context.Context, orgID int64, id uuid.UUID) (db.Branch, error)
	GetBranchByCode(ctx context.Context, orgID int64, code string) (db.Branch, error)
	CodeTaken(ctx context.Context, orgID int64, code string, exclude *uuid.UUID) (bool, error)
	SlugTaken(ctx context.Context, orgID int64, slug string, exclude *uuid.UUID) (bool, error)
	CreateBranch(ctx context.Context, orgID int64, f repository.BranchFields) (db.Branch, error)
	UpdateBranch(ctx context.Context, orgID int64, id uuid.UUID, f repository.BranchFields) (db.Branch, error)
	SetBranchActive(ctx context.Context, orgID int64, id uuid.UUID, active bool) (db.Branch, error)
	DeleteBranch(ctx context.Context, orgID int64, id uuid.UUID) error
	ListServicePoints(ctx context.Context, orgID, branchID int64) ([]db.ServicePoint, error)
	GetServicePoint(ctx context.Context, orgID, branchID int64, id uuid.UUID) (db.ServicePoint, error)
	CreateServicePoint(ctx context.Context, orgID, branchID int64, f repository.ServicePointFields) (db.ServicePoint, error)
	UpdateServicePoint(ctx context.Context, orgID, branchID int64, id uuid.UUID, f repository.ServicePointFields) (db.ServicePoint, error)
	DeleteServicePoint(ctx context.Context, orgID, branchID int64, id uuid.UUID) error
	ListActivity(ctx context.Context, orgID int64, branchUUID uuid.UUID, limit, offset int32) ([]db.ListOrganizationResourceActivityRow, int64, error)
}

// SearchIndexer schedules Cmd+K index updates (fail-soft).
type SearchIndexer interface {
	EnqueueUpsert(ctx context.Context, spec, id string)
	EnqueueDelete(ctx context.Context, spec, id string)
}

// Service implements the branches use cases.
type Service struct {
	repo    Repository
	indexer SearchIndexer
	limits  BranchLimiter
	images  ImageRepository
	storage storage.Driver
}

// BranchLimiter enforces the branch limit of the plan (TEC-883).
type BranchLimiter interface {
	CheckBranchAdd(ctx context.Context, scope orgctx.Scope) error
}

// SetLimiter attaches the plan limit check of new branches.
func (s *Service) SetLimiter(l BranchLimiter) {
	s.limits = l
}

// New builds the service. indexer may be nil.
func New(repo Repository, indexer SearchIndexer) *Service {
	return &Service{repo: repo, indexer: indexer}
}

// SortColumns are the sortable list fields.
var SortColumns = map[string]string{
	"code": "code", "name": "name", "city": "city", "prep_minutes": "prep_minutes",
	"is_active": "is_active", "created_at": "created_at", "updated_at": "updated_at",
}

// List returns one page of the organization's branches.
func (s *Service) List(ctx context.Context, orgID int64, f model.ListFilter) ([]model.Branch, int64, error) {
	if _, ok := SortColumns[f.SortField]; !ok {
		f.SortField = "created_at"
		f.SortDesc = true
	}
	f.BranchIDs = orgctx.BranchScope(ctx)
	rows, total, err := s.repo.ListBranches(ctx, orgID, f)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Branch, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		b := withZone(ctx, MapBranch(row.Branch))
		b.ServicePointsCount = row.ServicePointsCount
		out = append(out, b)
		ids = append(ids, row.ID)
	}
	if err := s.attachCovers(ctx, orgID, ids, out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// Facets returns the dynamic filter options (cities in use).
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	cities, err := s.repo.CityFacets(ctx, orgID, orgctx.BranchScope(ctx))
	if err != nil {
		return model.Facets{}, err
	}
	return model.Facets{City: cities}, nil
}

// Get returns one branch of the organization.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Branch, error) {
	row, err := s.getBranch(ctx, orgID, id)
	if err != nil {
		return model.Branch{}, err
	}
	points, err := s.repo.ListServicePoints(ctx, orgID, row.ID)
	if err != nil {
		return model.Branch{}, err
	}
	b := withZone(ctx, MapBranch(row))
	b.ServicePointsCount = int64(len(points))
	if err := s.attachImages(ctx, orgID, row.ID, &b); err != nil {
		return model.Branch{}, err
	}
	return b, nil
}

// Create validates and stores a new branch.
func (s *Service) Create(ctx context.Context, orgID int64, in model.BranchInput) (model.Branch, error) {
	// A new branch is outside every 'selected' scope, so only members with
	// access to every branch create branches.
	if err := orgctx.RequireAllBranches(ctx); err != nil {
		return model.Branch{}, ErrAllBranchesRequired
	}
	if s.limits != nil {
		if scope, ok := orgctx.From(ctx); ok {
			if err := s.limits.CheckBranchAdd(ctx, scope); err != nil {
				return model.Branch{}, err
			}
		}
	}
	fields := repository.BranchFields{
		PrepMinutes: 60, IsActive: true, ShowOnline: true,
		WorkingHours: mustJSON(model.DefaultWeeklyHours()), HolidayExceptions: []byte("[]"),
	}
	if err := applyBranchInput(&fields, in, true); err != nil {
		return model.Branch{}, err
	}
	if taken, err := s.repo.CodeTaken(ctx, orgID, fields.Code, nil); err != nil {
		return model.Branch{}, err
	} else if taken {
		return model.Branch{}, ErrCodeTaken
	}
	if err := s.assignSlug(ctx, orgID, &fields, in.Slug, nil); err != nil {
		return model.Branch{}, err
	}
	row, err := s.repo.CreateBranch(ctx, orgID, fields)
	if err != nil {
		return model.Branch{}, mapCodeTaken(err)
	}
	s.index(ctx, orgID, row.Uuid)
	return withZone(ctx, MapBranch(row)), nil
}

// Patch updates the given fields of a branch.
func (s *Service) Patch(ctx context.Context, orgID int64, id uuid.UUID, in model.BranchInput) (model.Branch, error) {
	current, err := s.getBranch(ctx, orgID, id)
	if err != nil {
		return model.Branch{}, err
	}
	fields := fieldsFromBranch(current)
	if err := applyBranchInput(&fields, in, false); err != nil {
		return model.Branch{}, err
	}
	if fields.Code != current.Code {
		if taken, err := s.repo.CodeTaken(ctx, orgID, fields.Code, &id); err != nil {
			return model.Branch{}, err
		} else if taken {
			return model.Branch{}, ErrCodeTaken
		}
	}
	if in.Slug != nil {
		if err := s.assignSlug(ctx, orgID, &fields, in.Slug, &id); err != nil {
			return model.Branch{}, err
		}
	}
	row, err := s.repo.UpdateBranch(ctx, orgID, id, fields)
	if err != nil {
		return model.Branch{}, mapNotFound(mapCodeTaken(err), ErrNotFound)
	}
	s.index(ctx, orgID, row.Uuid)
	return s.Get(ctx, orgID, row.Uuid)
}

// SetActive switches is_active and returns the previous value (bulk engine).
func (s *Service) SetActive(ctx context.Context, orgID int64, id uuid.UUID, active bool) (bool, error) {
	current, err := s.getBranch(ctx, orgID, id)
	if err != nil {
		return false, err
	}
	if current.IsActive != active {
		if _, err := s.repo.SetBranchActive(ctx, orgID, id, active); err != nil {
			return current.IsActive, mapNotFound(err, ErrNotFound)
		}
		s.index(ctx, orgID, id)
	}
	return current.IsActive, nil
}

// Delete soft-deletes a branch.
func (s *Service) Delete(ctx context.Context, orgID int64, id uuid.UUID) error {
	if _, err := s.getBranch(ctx, orgID, id); err != nil {
		return err
	}
	if err := s.repo.DeleteBranch(ctx, orgID, id); err != nil {
		return mapNotFound(err, ErrNotFound)
	}
	if s.indexer != nil {
		s.indexer.EnqueueDelete(ctx, SearchSpec, searchengine.TenantDocumentID(orgID, id.String()))
	}
	return nil
}

// TargetUUIDs resolves the branches matching a list filter (bulk "all
// matching" scope), capped at limit.
func (s *Service) TargetUUIDs(ctx context.Context, orgID int64, f model.ListFilter, limit int32) ([]uuid.UUID, error) {
	f.BranchIDs = orgctx.BranchScope(ctx)
	return s.repo.ListBranchUUIDs(ctx, orgID, f, limit)
}

// ExportRows returns every branch matching a list filter (export engine).
func (s *Service) ExportRows(ctx context.Context, orgID int64, f model.ListFilter) ([]model.Branch, error) {
	f.BranchIDs = orgctx.BranchScope(ctx)
	rows, err := s.repo.ListBranchesForExport(ctx, orgID, f)
	if err != nil {
		return nil, err
	}
	out := make([]model.Branch, 0, len(rows))
	for _, row := range rows {
		out = append(out, MapBranch(row))
	}
	return out, nil
}

// ListServicePoints lists the service points of a branch.
func (s *Service) ListServicePoints(ctx context.Context, orgID int64, branchUUID uuid.UUID) ([]model.ServicePoint, error) {
	branch, err := s.getBranch(ctx, orgID, branchUUID)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListServicePoints(ctx, orgID, branch.ID)
	if err != nil {
		return nil, err
	}
	out := make([]model.ServicePoint, 0, len(rows))
	for _, row := range rows {
		out = append(out, MapServicePoint(row, branch.Uuid))
	}
	return out, nil
}

// CreateServicePoint adds a service point to a branch.
func (s *Service) CreateServicePoint(ctx context.Context, orgID int64, branchUUID uuid.UUID, in model.ServicePointInput) (model.ServicePoint, error) {
	branch, err := s.getBranch(ctx, orgID, branchUUID)
	if err != nil {
		return model.ServicePoint{}, err
	}
	fields := repository.ServicePointFields{OnlineSelectable: true, IsActive: true}
	_ = fields.ExtraFee.Scan("0")
	if err := applyServicePointInput(&fields, in, true); err != nil {
		return model.ServicePoint{}, err
	}
	row, err := s.repo.CreateServicePoint(ctx, orgID, branch.ID, fields)
	if err != nil {
		return model.ServicePoint{}, err
	}
	return MapServicePoint(row, branch.Uuid), nil
}

// PatchServicePoint updates a service point of a branch. A point of another
// branch reads as not found.
func (s *Service) PatchServicePoint(ctx context.Context, orgID int64, branchUUID, pointUUID uuid.UUID, in model.ServicePointInput) (model.ServicePoint, error) {
	branch, err := s.getBranch(ctx, orgID, branchUUID)
	if err != nil {
		return model.ServicePoint{}, err
	}
	current, err := s.repo.GetServicePoint(ctx, orgID, branch.ID, pointUUID)
	if err != nil {
		return model.ServicePoint{}, mapNotFound(err, ErrServicePointNotFound)
	}
	fields := repository.ServicePointFields{
		Type: current.Type, Name: current.Name, Description: current.Description, ExtraFee: current.ExtraFee,
		WorkingHours: current.WorkingHours, OnlineSelectable: current.OnlineSelectable, IsActive: current.IsActive,
	}
	if err := applyServicePointInput(&fields, in, false); err != nil {
		return model.ServicePoint{}, err
	}
	row, err := s.repo.UpdateServicePoint(ctx, orgID, branch.ID, pointUUID, fields)
	if err != nil {
		return model.ServicePoint{}, mapNotFound(err, ErrServicePointNotFound)
	}
	return MapServicePoint(row, branch.Uuid), nil
}

// DeleteServicePoint removes a service point of a branch.
func (s *Service) DeleteServicePoint(ctx context.Context, orgID int64, branchUUID, pointUUID uuid.UUID) error {
	branch, err := s.getBranch(ctx, orgID, branchUUID)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteServicePoint(ctx, orgID, branch.ID, pointUUID); err != nil {
		return mapNotFound(err, ErrServicePointNotFound)
	}
	return nil
}

// Activity lists the audit events of a branch.
func (s *Service) Activity(ctx context.Context, orgID int64, branchUUID uuid.UUID, limit, offset int32) ([]model.ActivityEntry, int64, error) {
	if _, err := s.getBranch(ctx, orgID, branchUUID); err != nil {
		return nil, 0, err
	}
	rows, total, err := s.repo.ListActivity(ctx, orgID, branchUUID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.ActivityEntry, 0, len(rows))
	for _, row := range rows {
		entry := model.ActivityEntry{
			UUID: row.Uuid, Action: row.Action, Payload: json.RawMessage(row.Payload),
			CreatedAt: row.CreatedAt.Time.UTC(),
		}
		if len(entry.Payload) == 0 {
			entry.Payload = json.RawMessage("{}")
		}
		if row.ActorUuid.Valid {
			entry.Actor = &model.ActivityActor{
				UUID: row.ActorUuid.Bytes, Name: row.ActorName.String,
				Surname: row.ActorSurname.String, Email: row.ActorEmail.String,
			}
		}
		out = append(out, entry)
	}
	return out, total, nil
}

// ImportRow creates a branch from an import row, or updates the branch with
// the same code. It returns the branch uuid, whether it was created and the
// previous state of an updated branch (for rollback).
func (s *Service) ImportRow(ctx context.Context, orgID int64, in model.BranchInput) (uuid.UUID, bool, *model.Branch, error) {
	code := ""
	if in.Code != nil {
		code = normalizeCode(*in.Code)
	}
	if code != "" {
		row, err := s.repo.GetBranchByCode(ctx, orgID, code)
		if err == nil && orgctx.CheckBranch(ctx, row.ID) != nil {
			// Same answer as a missing branch: do not leak it.
			return uuid.Nil, false, nil, ErrNotFound
		}
		switch {
		case err == nil:
			prev := MapBranch(row)
			updated, err := s.Patch(ctx, orgID, row.Uuid, in)
			if err != nil {
				return uuid.Nil, false, nil, err
			}
			return updated.UUID, false, &prev, nil
		case !errors.Is(err, repository.ErrNotFound):
			return uuid.Nil, false, nil, err
		}
	}
	created, err := s.Create(ctx, orgID, in)
	if err != nil {
		return uuid.Nil, false, nil, err
	}
	return created.UUID, true, nil, nil
}

// getBranch loads a branch of the organization and applies the caller's
// branch scope: a branch outside it reads as not found (404), like another
// organization's uuid.
func (s *Service) getBranch(ctx context.Context, orgID int64, id uuid.UUID) (db.Branch, error) {
	row, err := s.repo.GetBranch(ctx, orgID, id)
	if err != nil {
		return db.Branch{}, mapNotFound(err, ErrNotFound)
	}
	if err := orgctx.CheckBranch(ctx, row.ID); err != nil {
		return db.Branch{}, ErrNotFound
	}
	return row, nil
}

func (s *Service) index(ctx context.Context, orgID int64, id uuid.UUID) {
	if s.indexer != nil {
		s.indexer.EnqueueUpsert(ctx, SearchSpec, searchengine.TenantDocumentID(orgID, id.String()))
	}
}

// DefaultBranchFields is the branch every new organization starts with.
func DefaultBranchFields() repository.BranchFields {
	return repository.BranchFields{
		Code: "MRK", Name: "Merkez", Slug: "merkez", PrepMinutes: 60, IsActive: true, ShowOnline: true,
		WorkingHours: mustJSON(model.DefaultWeeklyHours()), HolidayExceptions: []byte("[]"),
	}
}

// --- validation -----------------------------------------------------------

var (
	codeRE  = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{0,31}$`)
	slugRE  = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	timeRE  = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	moneyRE = regexp.MustCompile(`^[0-9]{1,12}(\.[0-9]{1,2})?$`)
)

func normalizeCode(v string) string {
	return strings.ToUpper(strings.TrimSpace(v))
}

func applyBranchInput(f *repository.BranchFields, in model.BranchInput, create bool) error {
	ve := &ValidationError{}
	if in.Code != nil || create {
		code := ""
		if in.Code != nil {
			code = normalizeCode(*in.Code)
		}
		switch {
		case code == "":
			ve.add("code", "required", "code is required")
		case !codeRE.MatchString(code):
			ve.add("code", "invalid", "code must be 1-32 characters: letters, digits, - or _")
		default:
			f.Code = code
		}
	}
	if in.Name != nil || create {
		name := ""
		if in.Name != nil {
			name = strings.TrimSpace(*in.Name)
		}
		switch {
		case name == "":
			ve.add("name", "required", "name is required")
		case utf8.RuneCountInString(name) > 255:
			ve.add("name", "too_long", "name must be at most 255 characters")
		default:
			f.Name = name
		}
	}
	setText := func(field string, src *string, dst *string, max int) {
		if src == nil {
			return
		}
		v := strings.TrimSpace(*src)
		if utf8.RuneCountInString(v) > max {
			ve.add(field, "too_long", fmt.Sprintf("%s must be at most %d characters", field, max))
			return
		}
		*dst = v
	}
	setText("address", in.Address, &f.Address, 1000)
	setText("city", in.City, &f.City, 128)
	setText("district", in.District, &f.District, 128)
	setText("phone", in.Phone, &f.Phone, 32)
	if in.Email != nil {
		v := strings.TrimSpace(*in.Email)
		if v != "" {
			if addr, err := mail.ParseAddress(v); err != nil || addr.Address != v || len(v) > 255 {
				ve.add("email", "invalid", "email is invalid")
			}
		}
		f.Email = v
	}
	if in.Latitude.Set {
		if in.Latitude.Value != nil && (*in.Latitude.Value < -90 || *in.Latitude.Value > 90) {
			ve.add("latitude", "out_of_range", "latitude must be between -90 and 90")
		} else {
			f.Latitude = floatToNumeric(in.Latitude.Value)
		}
	}
	if in.Longitude.Set {
		if in.Longitude.Value != nil && (*in.Longitude.Value < -180 || *in.Longitude.Value > 180) {
			ve.add("longitude", "out_of_range", "longitude must be between -180 and 180")
		} else {
			f.Longitude = floatToNumeric(in.Longitude.Value)
		}
	}
	if f.Latitude.Valid != f.Longitude.Valid && len(ve.Details) == 0 {
		ve.add("latitude", "pair_required", "latitude and longitude must be set together")
	}
	if in.WorkingHours != nil {
		hours := *in.WorkingHours
		validateWeeklyHours(ve, "working_hours", &hours)
		f.WorkingHours = mustJSON(hours)
	}
	if in.HolidayExceptions != nil {
		list := validateHolidays(ve, *in.HolidayExceptions)
		f.HolidayExceptions = mustJSON(list)
	}
	if in.PrepMinutes != nil {
		if *in.PrepMinutes < 0 || *in.PrepMinutes > 10080 {
			ve.add("prep_minutes", "out_of_range", "prep_minutes must be between 0 and 10080")
		} else {
			f.PrepMinutes = *in.PrepMinutes
		}
	}
	if in.IsActive != nil {
		f.IsActive = *in.IsActive
	}
	setText("description", in.Description, &f.Description, 2000)
	if in.Timezone.Set {
		zone := ""
		if in.Timezone.Value != nil {
			zone = strings.TrimSpace(*in.Timezone.Value)
		}
		switch {
		case zone == "":
			f.Timezone = pgtype.Text{}
		case !validTimezone(zone):
			ve.add("timezone", "invalid", "timezone must be an IANA time zone such as Europe/Istanbul")
		default:
			f.Timezone = pgtype.Text{String: zone, Valid: true}
		}
	}
	if in.ShowOnline != nil {
		f.ShowOnline = *in.ShowOnline
	}
	return ve.orNil()
}

func validateWeeklyHours(ve *ValidationError, prefix string, w *model.WeeklyHours) {
	for _, day := range w.Days() {
		validateDay(ve, prefix+"."+day.Key, day.Hours)
	}
}

func validateDay(ve *ValidationError, prefix string, d *model.DayHours) {
	if d.Closed {
		d.Open, d.Close = "", ""
		return
	}
	d.Open, d.Close = strings.TrimSpace(d.Open), strings.TrimSpace(d.Close)
	okOpen := timeRE.MatchString(d.Open)
	okClose := timeRE.MatchString(d.Close)
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

func validateHolidays(ve *ValidationError, in []model.HolidayException) []model.HolidayException {
	if len(in) > 366 {
		ve.add("holiday_exceptions", "too_many", "at most 366 holiday exceptions are allowed")
		return in
	}
	seen := map[string]bool{}
	out := make([]model.HolidayException, 0, len(in))
	for i, h := range in {
		prefix := "holiday_exceptions." + strconv.Itoa(i)
		h.Date = strings.TrimSpace(h.Date)
		if _, err := time.Parse("2006-01-02", h.Date); err != nil {
			ve.add(prefix+".date", "invalid", "date must be yyyy-MM-dd")
		} else if seen[h.Date] {
			ve.add(prefix+".date", "duplicate", "date is listed twice")
		}
		seen[h.Date] = true
		h.Note = strings.TrimSpace(h.Note)
		if utf8.RuneCountInString(h.Note) > 255 {
			ve.add(prefix+".note", "too_long", "note must be at most 255 characters")
		}
		day := model.DayHours{Closed: h.Closed, Open: h.Open, Close: h.Close}
		validateDay(ve, prefix, &day)
		h.Open, h.Close = day.Open, day.Close
		out = append(out, h)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

func applyServicePointInput(f *repository.ServicePointFields, in model.ServicePointInput, create bool) error {
	ve := &ValidationError{}
	if in.Type != nil || create {
		t := ""
		if in.Type != nil {
			t = strings.TrimSpace(*in.Type)
		}
		valid := false
		for _, allowed := range model.ServicePointTypes {
			if t == allowed {
				valid = true
			}
		}
		if !valid {
			ve.add("type", "invalid", "type must be one of office, airport, address_zone, hotel")
		} else {
			f.Type = t
		}
	}
	if in.Name != nil || create {
		name := ""
		if in.Name != nil {
			name = strings.TrimSpace(*in.Name)
		}
		switch {
		case name == "":
			ve.add("name", "required", "name is required")
		case utf8.RuneCountInString(name) > 255:
			ve.add("name", "too_long", "name must be at most 255 characters")
		default:
			f.Name = name
		}
	}
	if in.Description != nil {
		v := strings.TrimSpace(*in.Description)
		if utf8.RuneCountInString(v) > 2000 {
			ve.add("description", "too_long", "description must be at most 2000 characters")
		} else {
			f.Description = v
		}
	}
	if in.ExtraFee != nil {
		v := strings.TrimSpace(in.ExtraFee.String())
		var n pgtype.Numeric
		if !moneyRE.MatchString(v) || n.Scan(v) != nil {
			ve.add("extra_fee", "invalid", "extra_fee must be a non-negative amount with at most 2 decimals")
		} else {
			f.ExtraFee = n
		}
	}
	if in.WorkingHours.Set {
		if in.WorkingHours.Value == nil {
			f.WorkingHours = nil
		} else {
			hours := *in.WorkingHours.Value
			validateWeeklyHours(ve, "working_hours", &hours)
			f.WorkingHours = mustJSON(hours)
		}
	}
	if in.OnlineSelectable != nil {
		f.OnlineSelectable = *in.OnlineSelectable
	}
	if in.IsActive != nil {
		f.IsActive = *in.IsActive
	}
	return ve.orNil()
}

// --- mapping --------------------------------------------------------------

// MapBranch converts a row to the API shape.
func MapBranch(row db.Branch) model.Branch {
	b := model.Branch{
		UUID: row.Uuid, Code: row.Code, Name: row.Name, Address: row.Address, City: row.City,
		District: row.District, Latitude: numericToFloat(row.Latitude), Longitude: numericToFloat(row.Longitude),
		Phone: row.Phone, Email: row.Email, PrepMinutes: row.PrepMinutes, IsActive: row.IsActive,
		Slug: row.Slug, Description: row.Description, ShowOnline: row.ShowOnline,
		HolidayExceptions: []model.HolidayException{},
		CreatedAt:         row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
	if row.Timezone.Valid && row.Timezone.String != "" {
		zone := row.Timezone.String
		b.Timezone = &zone
		b.EffectiveTimezone = zone
	}
	b.WorkingHours = decodeHours(row.WorkingHours)
	if len(row.HolidayExceptions) > 0 {
		_ = json.Unmarshal(row.HolidayExceptions, &b.HolidayExceptions)
		if b.HolidayExceptions == nil {
			b.HolidayExceptions = []model.HolidayException{}
		}
	}
	return b
}

// MapServicePoint converts a row to the API shape.
func MapServicePoint(row db.ServicePoint, branchUUID uuid.UUID) model.ServicePoint {
	p := model.ServicePoint{
		UUID: row.Uuid, BranchUUID: branchUUID, Type: row.Type, Name: row.Name, Description: row.Description,
		ExtraFee: FormatMoney(row.ExtraFee), OnlineSelectable: row.OnlineSelectable, IsActive: row.IsActive,
		CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
	if len(row.WorkingHours) > 0 && string(row.WorkingHours) != "null" {
		hours := decodeHours(row.WorkingHours)
		p.WorkingHours = &hours
	}
	return p
}

func fieldsFromBranch(row db.Branch) repository.BranchFields {
	return repository.BranchFields{
		Code: row.Code, Name: row.Name, Address: row.Address, City: row.City, District: row.District,
		Latitude: row.Latitude, Longitude: row.Longitude, Phone: row.Phone, Email: row.Email,
		WorkingHours: row.WorkingHours, HolidayExceptions: row.HolidayExceptions,
		PrepMinutes: row.PrepMinutes, IsActive: row.IsActive,
		Slug: row.Slug, Description: row.Description, Timezone: row.Timezone, ShowOnline: row.ShowOnline,
	}
}

func decodeHours(raw []byte) model.WeeklyHours {
	var hours model.WeeklyHours
	if len(raw) == 0 || json.Unmarshal(raw, &hours) != nil {
		return model.DefaultWeeklyHours()
	}
	return hours
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func floatToNumeric(v *float64) pgtype.Numeric {
	if v == nil {
		return pgtype.Numeric{}
	}
	var n pgtype.Numeric
	if err := n.Scan(strconv.FormatFloat(*v, 'f', 6, 64)); err != nil {
		return pgtype.Numeric{}
	}
	return n
}

func numericToFloat(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return nil
	}
	v := f.Float64
	return &v
}

// FormatMoney renders a numeric as a decimal string with two fraction digits.
func FormatMoney(n pgtype.Numeric) string {
	if !n.Valid || n.Int == nil {
		return "0.00"
	}
	cents := new(big.Int).Set(n.Int)
	shift := int(n.Exp) + 2
	ten := big.NewInt(10)
	if shift >= 0 {
		cents.Mul(cents, new(big.Int).Exp(ten, big.NewInt(int64(shift)), nil))
	} else {
		cents.Quo(cents, new(big.Int).Exp(ten, big.NewInt(int64(-shift)), nil))
	}
	neg := cents.Sign() < 0
	cents.Abs(cents)
	s := cents.String()
	for len(s) < 3 {
		s = "0" + s
	}
	out := s[:len(s)-2] + "." + s[len(s)-2:]
	if neg {
		out = "-" + out
	}
	return out
}

func mapNotFound(err, target error) error {
	if errors.Is(err, repository.ErrNotFound) {
		return target
	}
	return err
}

func mapCodeTaken(err error) error {
	switch {
	case errors.Is(err, repository.ErrCodeTaken):
		return ErrCodeTaken
	case errors.Is(err, repository.ErrSlugTaken):
		return ErrSlugTaken
	}
	return err
}

// assignSlug sets f.Slug from the request or, when the request leaves it
// empty, from the name with -2, -3 … on a clash. An explicit slug that is
// taken is an error.
func (s *Service) assignSlug(ctx context.Context, orgID int64, f *repository.BranchFields, requested *string, exclude *uuid.UUID) error {
	explicit := ""
	if requested != nil {
		explicit = strings.ToLower(strings.TrimSpace(*requested))
	}
	if explicit != "" {
		if !slugRE.MatchString(explicit) || len(explicit) > maxSlugLen {
			ve := &ValidationError{}
			ve.add("slug", "invalid", "slug must be 1-64 lowercase letters, digits and single dashes")
			return ve
		}
		taken, err := s.repo.SlugTaken(ctx, orgID, explicit, exclude)
		if err != nil {
			return err
		}
		if taken {
			return ErrSlugTaken
		}
		f.Slug = explicit
		return nil
	}
	base := Slugify(f.Name)
	if base == "" {
		base = Slugify(f.Code)
	}
	if base == "" {
		base = "sube"
	}
	candidate := base
	for n := 2; ; n++ {
		taken, err := s.repo.SlugTaken(ctx, orgID, candidate, exclude)
		if err != nil {
			return err
		}
		if !taken {
			f.Slug = candidate
			return nil
		}
		if n > 1000 {
			return ErrSlugTaken
		}
		candidate = withSuffix(base, n)
	}
}

// withZone fills the effective time zone: the branch's own, else the
// business zone of the request scope.
func withZone(ctx context.Context, b model.Branch) model.Branch {
	if b.Timezone != nil && *b.Timezone != "" {
		b.EffectiveTimezone = *b.Timezone
		return b
	}
	if scope, ok := orgctx.From(ctx); ok && scope.Timezone != "" {
		b.EffectiveTimezone = scope.Timezone
		return b
	}
	b.EffectiveTimezone = defaultTimezone
	return b
}

// defaultTimezone matches the organizations.timezone column default.
const defaultTimezone = "Europe/Istanbul"
