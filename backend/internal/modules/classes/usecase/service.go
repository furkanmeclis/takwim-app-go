// Package usecase holds the rules of the class timetable (TEC-970). Every
// method takes the organization id of the caller explicitly.
//
//   - A class is a group service (services.group_capacity) taught every
//     week on one weekday at one wall-clock time in a branch, with an
//     optional fixed trainer and area and its own seats.
//   - Every such weekday between valid_from and valid_until is a session.
//     The availability engine offers a group class only at its sessions
//     and a session holds its trainer and area for other bookings.
//   - One session can be cancelled or get other seats. A session with
//     bookings is not cancelled (cancel the bookings first) and never gets
//     fewer seats than are booked; a class whose change would leave booked
//     future sessions outside the timetable is refused the same way.
//   - Classes of one trainer or one area never overlap on a weekday; one
//     service has one class per branch and start.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/timetable"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

const defaultTimezone = "Europe/Istanbul"

var (
	// ErrNotFound: the class is missing, deleted, of another business or
	// outside the caller's branch scope.
	ErrNotFound = errors.New("class not found")
	// ErrSessionNotFound: the class has no session on that date.
	ErrSessionNotFound = errors.New("class session not found")
	// ErrHasBookings: the change would leave booked sessions without a
	// class, or cancel a session with bookings.
	ErrHasBookings = errors.New("sessions of the class have bookings; cancel or move them first")
	// ErrBelowBooked: the seats would be fewer than the seats booked.
	ErrBelowBooked = errors.New("the session has more bookings than these seats")
	// ErrPast: a session that already started is not changed.
	ErrPast = errors.New("the session has already started")
)

// HasBookingsError is ErrHasBookings with the local dates of the booked
// sessions the change would leave behind, so the panel can offer to move
// their bookings (TEC-994).
type HasBookingsError struct {
	Dates []string
}

func (e *HasBookingsError) Error() string { return ErrHasBookings.Error() }

// Is makes errors.Is(err, ErrHasBookings) true.
func (e *HasBookingsError) Is(target error) bool { return target == ErrHasBookings }

// ConflictError: another class of the trainer or the area (or of the same
// service at the same start) runs at that time.
type ConflictError struct {
	Field string
	Code  string
	With  uuid.UUID
}

func (e *ConflictError) Error() string {
	switch e.Code {
	case "staff_busy":
		return "the trainer teaches another class at that time"
	case "resource_busy":
		return "the area hosts another class at that time"
	}
	return "the class already runs at that time"
}

// ScheduleSortColumns are the sortable timetable fields.
var ScheduleSortColumns = map[string]string{
	"weekday": "weekday", "service": "service", "created_at": "created_at",
}

// SessionSortColumns are the sortable session list fields.
var SessionSortColumns = map[string]string{
	"starts_at": "starts_at", "service": "service", "booked": "booked",
}

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

// Runner runs fn in one capacity transaction of the organization (the
// lock every booking writer takes), so a session change and a booking of
// the same session never cross.
type Runner interface {
	Run(ctx context.Context, orgID int64, fn func(tx *availabilityusecase.Tx) error) error
}

// Service is the class timetable.
type Service struct {
	q      *db.Queries
	runner Runner
	now    func() time.Time
}

// New builds the service.
func New(q *db.Queries, runner Runner) *Service {
	return &Service{q: q, runner: runner, now: time.Now}
}

// SetClock replaces time.Now (tests).
func (s *Service) SetClock(now func() time.Time) {
	s.now = now
}

// ------------------------------------------------------------------ helpers

func pgDate(s string) pgtype.Date {
	t, _ := time.Parse(timetable.DateLayout, s)
	return pgtype.Date{Time: t, Valid: true}
}

func dateString(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format(timetable.DateLayout)
}

func location(name string) *time.Location {
	if name != "" {
		if loc, err := time.LoadLocation(name); err == nil {
			return loc
		}
	}
	loc, _ := time.LoadLocation(defaultTimezone)
	return loc
}

// today is the local date of now in the business's time zone.
func (s *Service) today(ctx context.Context) string {
	tz := ""
	if scope, ok := orgctx.From(ctx); ok {
		tz = scope.Timezone
	}
	return s.now().In(location(tz)).Format(timetable.DateLayout)
}

func optUUID(v pgtype.UUID) (uuid.UUID, bool) {
	if !v.Valid {
		return uuid.Nil, false
	}
	return uuid.UUID(v.Bytes), true
}

// endClock is the wall clock duration minutes after start.
func endClock(start string, duration int) string {
	m := (timetable.Minutes(start) + duration) % (24 * 60)
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

// scheduleRow is the shape shared by GetClassSchedule and ListClassSchedules.
type scheduleRow struct {
	ID, BranchID, ServiceID     int64
	UUID                        uuid.UUID
	Weekday, StartTime, Note    string
	StaffID, ResourceID         pgtype.Int8
	Capacity, ServiceCapacity   pgtype.Int4
	ValidFrom, ValidUntil       pgtype.Date
	IsActive                    bool
	CreatedAt, UpdatedAt        pgtype.Timestamptz
	BranchUUID, ServiceUUID     uuid.UUID
	BranchName, ServiceName, TZ string
	Duration, Before, After     int32
	StaffUUID, ResourceUUID     pgtype.UUID
	StaffName, ResourceName     pgtype.Text
	ResourceKind                pgtype.Text
}

func fromGet(r db.GetClassScheduleRow) scheduleRow {
	return scheduleRow{
		ID: r.ID, BranchID: r.BranchID, ServiceID: r.ServiceID, UUID: r.Uuid, Weekday: r.Weekday, StartTime: r.StartTime,
		Note: r.Note, StaffID: r.StaffID, ResourceID: r.ResourceID, Capacity: r.Capacity, ServiceCapacity: r.ServiceCapacity,
		ValidFrom: r.ValidFrom, ValidUntil: r.ValidUntil, IsActive: r.IsActive, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		BranchUUID: r.BranchUuid, ServiceUUID: r.ServiceUuid, BranchName: r.BranchName, ServiceName: r.ServiceName, TZ: r.Timezone,
		Duration: r.DurationMinutes, Before: r.BufferBeforeMinutes, After: r.BufferAfterMinutes,
		StaffUUID: r.StaffUuid, ResourceUUID: r.ResourceUuid, StaffName: r.StaffName, ResourceName: r.ResourceName, ResourceKind: r.ResourceKind,
	}
}

func fromList(r db.ListClassSchedulesRow) scheduleRow {
	return scheduleRow{
		ID: r.ID, BranchID: r.BranchID, ServiceID: r.ServiceID, UUID: r.Uuid, Weekday: r.Weekday, StartTime: r.StartTime,
		Note: r.Note, StaffID: r.StaffID, ResourceID: r.ResourceID, Capacity: r.Capacity, ServiceCapacity: r.ServiceCapacity,
		ValidFrom: r.ValidFrom, ValidUntil: r.ValidUntil, IsActive: r.IsActive, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		BranchUUID: r.BranchUuid, ServiceUUID: r.ServiceUuid, BranchName: r.BranchName, ServiceName: r.ServiceName, TZ: r.Timezone,
		Duration: r.DurationMinutes, Before: r.BufferBeforeMinutes, After: r.BufferAfterMinutes,
		StaffUUID: r.StaffUuid, ResourceUUID: r.ResourceUuid, StaffName: r.StaffName, ResourceName: r.ResourceName, ResourceKind: r.ResourceKind,
	}
}

func (r scheduleRow) entry() timetable.Entry {
	return timetable.Entry{
		ID: r.ID, Weekday: r.Weekday, Start: r.StartTime, Location: location(r.TZ),
		ValidFrom: dateString(r.ValidFrom), ValidUntil: dateString(r.ValidUntil),
	}
}

func (r scheduleRow) staffRef() *model.Ref {
	if id, ok := optUUID(r.StaffUUID); ok {
		return &model.Ref{UUID: id, Name: r.StaffName.String}
	}
	return nil
}

func (r scheduleRow) resourceRef() *model.ResourceRef {
	if id, ok := optUUID(r.ResourceUUID); ok {
		return &model.ResourceRef{UUID: id, Name: r.ResourceName.String, Kind: r.ResourceKind.String}
	}
	return nil
}

func (r scheduleRow) seats() int {
	return timetable.Seats(nil, int(r.Capacity.Int32), int(r.ServiceCapacity.Int32))
}

func (r scheduleRow) model() model.Schedule {
	out := model.Schedule{
		UUID: r.UUID, Branch: model.Ref{UUID: r.BranchUUID, Name: r.BranchName},
		Service: model.Ref{UUID: r.ServiceUUID, Name: r.ServiceName},
		Weekday: r.Weekday, StartTime: r.StartTime, EndTime: endClock(r.StartTime, int(r.Duration)),
		DurationMinutes: int(r.Duration), Staff: r.staffRef(), Resource: r.resourceRef(), Seats: r.seats(),
		ValidFrom: dateString(r.ValidFrom), IsActive: r.IsActive, Note: r.Note, Timezone: location(r.TZ).String(),
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.Capacity.Valid {
		c := int(r.Capacity.Int32)
		out.Capacity = &c
	}
	if r.ValidUntil.Valid {
		v := dateString(r.ValidUntil)
		out.ValidUntil = &v
	}
	return out
}

// get loads a live class inside the caller's branch scope.
func (s *Service) get(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (scheduleRow, error) {
	row, err := q.GetClassSchedule(ctx, db.GetClassScheduleParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return scheduleRow{}, ErrNotFound
	}
	if err != nil {
		return scheduleRow{}, err
	}
	if orgctx.CheckBranch(ctx, row.BranchID) != nil {
		return scheduleRow{}, ErrNotFound
	}
	return fromGet(row), nil
}

// -------------------------------------------------------------- timetable

// List answers the timetable page.
func (s *Service) List(ctx context.Context, orgID int64, f model.ScheduleFilter) ([]model.Schedule, int64, error) {
	if _, ok := ScheduleSortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "", false
	}
	var active pgtype.Bool
	if f.IsActive != nil {
		active = pgtype.Bool{Bool: *f.IsActive, Valid: true}
	}
	var q pgtype.Text
	if v := strings.TrimSpace(f.Q); v != "" {
		q = pgtype.Text{String: v, Valid: true}
	}
	nilIfEmpty := func(ids []uuid.UUID) []uuid.UUID {
		if len(ids) == 0 {
			return nil
		}
		return ids
	}
	var weekdays []string
	if len(f.Weekdays) > 0 {
		weekdays = f.Weekdays
	}
	scope := orgctx.BranchScope(ctx)
	rows, err := s.q.ListClassSchedules(ctx, db.ListClassSchedulesParams{
		OrganizationID: orgID, BranchIds: scope, BranchUuids: nilIfEmpty(f.Branches), ServiceUuids: nilIfEmpty(f.Services),
		StaffUuids: nilIfEmpty(f.Staff), Weekdays: weekdays, IsActive: active, Q: q,
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: int32(f.Limit), OffsetCount: int32(f.Offset),
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountClassSchedules(ctx, db.CountClassSchedulesParams{
		OrganizationID: orgID, BranchIds: scope, BranchUuids: nilIfEmpty(f.Branches), ServiceUuids: nilIfEmpty(f.Services),
		StaffUuids: nilIfEmpty(f.Staff), Weekdays: weekdays, IsActive: active, Q: q,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Schedule, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromList(r).model())
	}
	return out, total, nil
}

// Get returns one class.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Schedule, error) {
	row, err := s.get(ctx, s.q, orgID, id)
	if err != nil {
		return model.Schedule{}, err
	}
	return row.model(), nil
}

// resolved is a validated class input.
type resolved struct {
	branchID, serviceID int64
	staffID, resourceID pgtype.Int8
	capacity            pgtype.Int4
	weekday, start      string
	validFrom           string
	validUntil          pgtype.Date
	isActive            bool
	note                string
	timezone            string
	duration            int
	before, after       int
}

func (r resolved) entry() timetable.Entry {
	until := ""
	if r.validUntil.Valid {
		until = dateString(r.validUntil)
	}
	return timetable.Entry{Weekday: r.weekday, Start: r.start, Location: location(r.timezone), ValidFrom: r.validFrom, ValidUntil: until}
}

// held is the held minutes of the day of a class.
func held(start string, duration, before, after int) (int, int) {
	m := timetable.Minutes(start)
	return m - before, m + duration + after
}

func (s *Service) resolve(ctx context.Context, q *db.Queries, orgID int64, in model.ScheduleInput, current *scheduleRow) (resolved, error) {
	ve := &ValidationError{}
	out := resolved{weekday: in.Weekday, start: strings.TrimSpace(in.StartTime), isActive: true, note: strings.TrimSpace(in.Note)}
	if in.IsActive != nil {
		out.isActive = *in.IsActive
	}
	if _, ok := timetable.Weekdays[in.Weekday]; !ok {
		ve.add("weekday", "invalid", "weekday must be one of "+strings.Join(timetable.WeekdayKeys, ", "))
	}
	if _, _, ok := timetable.Clock(out.start); !ok {
		ve.add("start_time", "invalid", "start_time must be HH:MM")
	}
	if in.Capacity != nil {
		if *in.Capacity < model.MinCapacity || *in.Capacity > model.MaxCapacity {
			ve.add("capacity", "invalid", fmt.Sprintf("capacity must be between %d and %d", model.MinCapacity, model.MaxCapacity))
		} else {
			out.capacity = pgtype.Int4{Int32: int32(*in.Capacity), Valid: true}
		}
	}
	out.validFrom = strings.TrimSpace(in.ValidFrom)
	switch {
	case out.validFrom == "" && current != nil:
		out.validFrom = dateString(current.ValidFrom)
	case out.validFrom == "":
		out.validFrom = s.today(ctx)
	case !timetable.ValidDate(out.validFrom):
		ve.add("valid_from", "invalid", "valid_from must be a date (yyyy-MM-dd)")
	}
	if in.ValidUntil != nil && strings.TrimSpace(*in.ValidUntil) != "" {
		until := strings.TrimSpace(*in.ValidUntil)
		switch {
		case !timetable.ValidDate(until):
			ve.add("valid_until", "invalid", "valid_until must be a date (yyyy-MM-dd)")
		case until < out.validFrom:
			ve.add("valid_until", "before_from", "valid_until must not be before valid_from")
		default:
			out.validUntil = pgDate(until)
		}
	}
	if utf8.RuneCountInString(out.note) > model.MaxNote {
		ve.add("note", "too_long", fmt.Sprintf("note must be at most %d characters", model.MaxNote))
	}

	if in.BranchUUID == uuid.Nil {
		ve.add("branch_uuid", "required", "branch is required")
		return out, ve
	}
	branch, err := q.GetClassBranch(ctx, db.GetClassBranchParams{OrganizationID: orgID, Uuid: in.BranchUUID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && orgctx.CheckBranch(ctx, branch.ID) != nil) {
		ve.add("branch_uuid", "not_found", "branch not found")
		return out, ve
	}
	if err != nil {
		return out, err
	}
	out.branchID, out.timezone = branch.ID, branch.Timezone

	if in.ServiceUUID == uuid.Nil {
		ve.add("service_uuid", "required", "class is required")
		return out, ve
	}
	svc, err := q.GetClassService(ctx, db.GetClassServiceParams{OrganizationID: orgID, Uuid: in.ServiceUUID, BranchID: branch.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		ve.add("service_uuid", "not_found", "service not found")
		return out, ve
	}
	if err != nil {
		return out, err
	}
	switch {
	case !svc.GroupCapacity.Valid:
		ve.add("service_uuid", "not_group", "the service is not a group class")
		return out, ve
	case !svc.OfferID.Valid || !svc.OfferIsActive.Bool:
		ve.add("service_uuid", "not_offered", "the branch does not offer the class")
		return out, ve
	}
	out.serviceID, out.duration = svc.ID, int(svc.DurationMinutes)
	out.before, out.after = int(svc.BufferBeforeMinutes), int(svc.BufferAfterMinutes)

	if in.StaffUUID != nil && *in.StaffUUID != uuid.Nil {
		trainer, err := q.GetClassTrainer(ctx, db.GetClassTrainerParams{OrganizationID: orgID, Uuid: *in.StaffUUID, ServiceID: svc.ID, BranchID: branch.ID})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			ve.add("staff_uuid", "not_found", "staff member not found")
		case err != nil:
			return out, err
		case svc.StaffMode == "none":
			ve.add("staff_uuid", "not_allowed", "the class runs without a trainer")
		case !trainer.Performs:
			ve.add("staff_uuid", "not_qualified", "this person does not teach the class")
		case !trainer.InBranch:
			ve.add("staff_uuid", "not_in_branch", "this person does not work in the branch")
		default:
			out.staffID = pgtype.Int8{Int64: trainer.ID, Valid: true}
		}
	}
	if in.ResourceUUID != nil && *in.ResourceUUID != uuid.Nil {
		res, err := q.GetClassResource(ctx, db.GetClassResourceParams{OrganizationID: orgID, Uuid: *in.ResourceUUID})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			ve.add("resource_uuid", "not_found", "resource not found")
		case err != nil:
			return out, err
		case res.BranchID != branch.ID:
			ve.add("resource_uuid", "not_in_branch", "the resource belongs to another branch")
		case len(svc.ResourceKinds) > 0 && !slices.Contains(svc.ResourceKinds, res.Kind):
			ve.add("resource_uuid", "kind_mismatch", "the class does not use this kind of resource")
		default:
			out.resourceID = pgtype.Int8{Int64: res.ID, Valid: true}
		}
	}
	if err := ve.orNil(); err != nil {
		return out, err
	}
	return out, nil
}

// conflicts refuses a class that overlaps another class of its trainer or
// area, or repeats a class of the service in the branch at that start.
func (s *Service) conflicts(ctx context.Context, q *db.Queries, orgID int64, r resolved, exclude int64) error {
	if !r.isActive {
		return nil
	}
	rows, err := q.ListClassScheduleNeighbours(ctx, db.ListClassScheduleNeighboursParams{
		OrganizationID: orgID, Weekday: r.weekday, ExcludeID: exclude, BranchID: r.branchID, ServiceID: r.serviceID,
		StaffID: r.staffID, ResourceID: r.resourceID,
	})
	if err != nil {
		return err
	}
	mine := r.entry()
	from, to := held(r.start, r.duration, r.before, r.after)
	for _, n := range rows {
		other := timetable.Entry{Weekday: n.Weekday, Start: n.StartTime, ValidFrom: dateString(n.ValidFrom), ValidUntil: dateString(n.ValidUntil)}
		oFrom, oTo := held(n.StartTime, int(n.DurationMinutes), int(n.BufferBeforeMinutes), int(n.BufferAfterMinutes))
		if !timetable.Overlap(mine, from, to, other, oFrom, oTo) {
			continue
		}
		switch {
		case n.BranchID == r.branchID && n.ServiceID == r.serviceID && n.StartTime == r.start:
			return &ConflictError{Field: "start_time", Code: "duplicate", With: n.Uuid}
		case r.staffID.Valid && n.StaffID.Valid && n.StaffID.Int64 == r.staffID.Int64:
			return &ConflictError{Field: "staff_uuid", Code: "staff_busy", With: n.Uuid}
		case r.resourceID.Valid && n.ResourceID.Valid && n.ResourceID.Int64 == r.resourceID.Int64:
			return &ConflictError{Field: "resource_uuid", Code: "resource_busy", With: n.Uuid}
		}
	}
	return nil
}

// keepsBookings refuses a change of a class (next = nil: removed) that
// would leave a booked future session outside the timetable.
func (s *Service) keepsBookings(ctx context.Context, tx *availabilityusecase.Tx, current scheduleRow, next *resolved) error {
	rows, err := tx.Q.ListGroupSessionHolds(ctx, db.ListGroupSessionHoldsParams{
		OrganizationID: tx.OrgID, BranchID: current.BranchID, ServiceIds: []int64{current.ServiceID}, Now: pgtype.Timestamptz{Time: tx.Now, Valid: true},
		Since: pgtype.Timestamptz{Time: tx.Now, Valid: true}, Until: pgtype.Timestamptz{Time: tx.Now.AddDate(20, 0, 0), Valid: true},
	})
	if err != nil {
		return err
	}
	old := current.entry()
	var dates []string
	for _, h := range rows {
		date, ok := old.At(h.StartsAt.Time)
		if !ok {
			continue
		}
		if next != nil && next.isActive && next.branchID == current.BranchID && next.serviceID == current.ServiceID {
			if _, ok := next.entry().At(h.StartsAt.Time); ok {
				continue
			}
		}
		if !slices.Contains(dates, date) {
			dates = append(dates, date)
		}
	}
	if len(dates) > 0 {
		slices.Sort(dates)
		return &HasBookingsError{Dates: dates}
	}
	return nil
}

// Create adds a class.
func (s *Service) Create(ctx context.Context, orgID int64, actorID *int64, in model.ScheduleInput) (model.Schedule, error) {
	var id uuid.UUID
	err := s.runner.Run(ctx, orgID, func(tx *availabilityusecase.Tx) error {
		r, err := s.resolve(ctx, tx.Q, orgID, in, nil)
		if err != nil {
			return err
		}
		if err := s.conflicts(ctx, tx.Q, orgID, r, 0); err != nil {
			return err
		}
		var by pgtype.Int8
		if actorID != nil {
			by = pgtype.Int8{Int64: *actorID, Valid: true}
		}
		row, err := tx.Q.InsertClassSchedule(ctx, db.InsertClassScheduleParams{
			OrganizationID: orgID, BranchID: r.branchID, ServiceID: r.serviceID, Weekday: r.weekday, StartTime: r.start,
			StaffID: r.staffID, ResourceID: r.resourceID, Capacity: r.capacity, ValidFrom: pgDate(r.validFrom),
			ValidUntil: r.validUntil, IsActive: r.isActive, Note: r.note, CreatedBy: by,
		})
		if err != nil {
			return err
		}
		id = row.Uuid
		return nil
	})
	if err != nil {
		return model.Schedule{}, err
	}
	return s.Get(ctx, orgID, id)
}

// Update replaces a class; it returns the class before and after.
func (s *Service) Update(ctx context.Context, orgID int64, id uuid.UUID, in model.ScheduleInput) (model.Schedule, model.Schedule, error) {
	var before scheduleRow
	err := s.runner.Run(ctx, orgID, func(tx *availabilityusecase.Tx) error {
		current, err := s.get(ctx, tx.Q, orgID, id)
		if err != nil {
			return err
		}
		before = current
		r, err := s.resolve(ctx, tx.Q, orgID, in, &current)
		if err != nil {
			return err
		}
		if err := s.conflicts(ctx, tx.Q, orgID, r, current.ID); err != nil {
			return err
		}
		if err := s.keepsBookings(ctx, tx, current, &r); err != nil {
			return err
		}
		n, err := tx.Q.UpdateClassSchedule(ctx, db.UpdateClassScheduleParams{
			OrganizationID: orgID, ID: current.ID, BranchID: r.branchID, ServiceID: r.serviceID, Weekday: r.weekday,
			StartTime: r.start, StaffID: r.staffID, ResourceID: r.resourceID, Capacity: r.capacity,
			ValidFrom: pgDate(r.validFrom), ValidUntil: r.validUntil, IsActive: r.isActive, Note: r.note,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return model.Schedule{}, model.Schedule{}, err
	}
	after, err := s.Get(ctx, orgID, id)
	return before.model(), after, err
}

// Delete removes a class whose future sessions have no bookings.
func (s *Service) Delete(ctx context.Context, orgID int64, id uuid.UUID) (model.Schedule, error) {
	var current scheduleRow
	err := s.runner.Run(ctx, orgID, func(tx *availabilityusecase.Tx) error {
		var err error
		current, err = s.get(ctx, tx.Q, orgID, id)
		if err != nil {
			return err
		}
		if err := s.keepsBookings(ctx, tx, current, nil); err != nil {
			return err
		}
		n, err := tx.Q.SoftDeleteClassSchedule(ctx, db.SoftDeleteClassScheduleParams{OrganizationID: orgID, ID: current.ID})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return model.Schedule{}, err
	}
	return current.model(), nil
}

// Facets lists the branches, classes and trainers of the timetable.
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	rows, err := s.q.ListClassFacets(ctx, db.ListClassFacetsParams{OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx)})
	if err != nil {
		return model.Facets{}, err
	}
	out := model.Facets{Branches: []model.Facet{}, Services: []model.Facet{}, Staff: []model.Facet{}}
	for _, r := range rows {
		f := model.Facet{UUID: r.Uuid, Name: r.Name, Count: r.Count}
		switch r.Kind {
		case "branch":
			out.Branches = append(out.Branches, f)
		case "service":
			out.Services = append(out.Services, f)
		case "staff":
			out.Staff = append(out.Staff, f)
		}
	}
	return out, nil
}

// Form lists the group services (with the branches that offer them and
// the people who may teach them) and the resources of the caller's
// branches.
func (s *Service) Form(ctx context.Context, orgID int64) (model.FormOptions, error) {
	services, err := s.q.ListClassFormServices(ctx, orgID)
	if err != nil {
		return model.FormOptions{}, err
	}
	trainers, err := s.q.ListClassFormTrainers(ctx, orgID)
	if err != nil {
		return model.FormOptions{}, err
	}
	resources, err := s.q.ListClassFormResources(ctx, db.ListClassFormResourcesParams{OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx)})
	if err != nil {
		return model.FormOptions{}, err
	}
	byService := map[int64][]model.FormTrainer{}
	for _, t := range trainers {
		byService[t.ServiceID] = append(byService[t.ServiceID], model.FormTrainer{
			UUID: t.Uuid, Name: t.Name, IsActive: t.IsActive, BranchUUIDs: t.BranchUuids,
		})
	}
	out := model.FormOptions{Services: make([]model.FormService, 0, len(services)), Resources: make([]model.FormResource, 0, len(resources))}
	for _, svc := range services {
		list := byService[svc.ID]
		if list == nil {
			list = []model.FormTrainer{}
		}
		out.Services = append(out.Services, model.FormService{
			UUID: svc.Uuid, Name: svc.Name, GroupCapacity: int(svc.GroupCapacity.Int32), StaffMode: svc.StaffMode,
			ResourceKinds: svc.ResourceKinds, DurationMinutes: int(svc.DurationMinutes), IsActive: svc.IsActive,
			BranchUUIDs: svc.BranchUuids, Trainers: list,
		})
	}
	for _, r := range resources {
		out.Resources = append(out.Resources, model.FormResource{
			UUID: r.Uuid, Name: r.Name, Kind: r.Kind, IsActive: r.IsActive, BranchUUID: r.BranchUuid,
		})
	}
	return out, nil
}
