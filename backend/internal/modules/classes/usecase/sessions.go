package usecase

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/timetable"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

type changeKey struct {
	schedule int64
	date     string
}

type seatKey struct {
	service, branch int64
	start           int64
}

func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func (s *Service) changes(ctx context.Context, q *db.Queries, orgID int64, ids []int64, from, to string) (map[changeKey]db.ListClassSessionChangesRow, error) {
	out := map[changeKey]db.ListClassSessionChangesRow{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.ListClassSessionChanges(ctx, db.ListClassSessionChangesParams{
		OrganizationID: orgID, ScheduleIds: ids, FromDate: pgDate(from), ToDate: pgDate(to),
	})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[changeKey{schedule: r.ScheduleID, date: dateString(r.SessionDate)}] = r
	}
	return out, nil
}

// booked reads the seats taken per session of the services in [since, until).
func (s *Service) booked(ctx context.Context, q *db.Queries, orgID int64, services []int64, since, until time.Time) (map[seatKey]int, error) {
	out := map[seatKey]int{}
	if len(services) == 0 {
		return out, nil
	}
	rows, err := q.ListClassSessionSeats(ctx, db.ListClassSessionSeatsParams{
		OrganizationID: orgID, ServiceIds: services, Now: ts(s.now()), Since: ts(since), Until: ts(until),
	})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[seatKey{service: r.ServiceID, branch: r.BranchID, start: r.StartsAt.Time.UnixNano()}] = int(r.Seats)
	}
	return out, nil
}

// session renders one session of a class.
func (s *Service) session(r scheduleRow, occ timetable.Occurrence, change *db.ListClassSessionChangesRow, booked int) model.Session {
	out := model.Session{
		ScheduleUUID: r.UUID, Date: occ.Date, Weekday: r.Weekday, StartsAt: occ.Start.UTC(),
		EndsAt: occ.Start.Add(time.Duration(r.Duration) * time.Minute).UTC(),
		Branch: model.Ref{UUID: r.BranchUUID, Name: r.BranchName}, Service: model.Ref{UUID: r.ServiceUUID, Name: r.ServiceName},
		Staff: r.staffRef(), Resource: r.resourceRef(), Capacity: r.seats(), Booked: booked, Timezone: location(r.TZ).String(),
	}
	if change != nil {
		out.Cancelled, out.Reason = change.Cancelled, change.Reason
		if change.Capacity.Valid {
			out.Capacity, out.CapacityChanged = int(change.Capacity.Int32), true
		}
	}
	out.SeatsLeft = max(out.Capacity-booked, 0)
	switch {
	case out.Cancelled:
		out.State = model.StateCancelled
	case !occ.Start.After(s.now()):
		out.State = model.StatePast
	case out.SeatsLeft == 0:
		out.State = model.StateFull
	default:
		out.State = model.StateOpen
	}
	return out
}

func timetableRow(r db.ListClassTimetableRow) scheduleRow {
	return scheduleRow{
		ID: r.ID, BranchID: r.BranchID, ServiceID: r.ServiceID, UUID: r.Uuid, Weekday: r.Weekday, StartTime: r.StartTime,
		StaffID: r.StaffID, ResourceID: r.ResourceID, Capacity: r.Capacity, ServiceCapacity: r.ServiceCapacity,
		ValidFrom: r.ValidFrom, ValidUntil: r.ValidUntil, IsActive: true,
		BranchUUID: r.BranchUuid, ServiceUUID: r.ServiceUuid, BranchName: r.BranchName, ServiceName: r.ServiceName, TZ: r.Timezone,
		Duration: r.DurationMinutes, Before: r.BufferBeforeMinutes, After: r.BufferAfterMinutes,
		StaffUUID: r.StaffUuid, ResourceUUID: r.ResourceUuid, StaffName: r.StaffName, ResourceName: r.ResourceName, ResourceKind: r.ResourceKind,
	}
}

// period resolves the local dates of the session list.
func (s *Service) period(ctx context.Context, f model.SessionFilter) (string, string, error) {
	ve := &ValidationError{}
	from, to := strings.TrimSpace(f.From), strings.TrimSpace(f.To)
	if from == "" {
		from = s.today(ctx)
	} else if !timetable.ValidDate(from) {
		ve.add("from", "invalid", "from must be a date (yyyy-MM-dd)")
		return "", "", ve
	}
	start, _ := time.Parse(timetable.DateLayout, from)
	if to == "" {
		to = start.AddDate(0, 0, model.DefaultSessionDays-1).Format(timetable.DateLayout)
	} else if !timetable.ValidDate(to) {
		ve.add("to", "invalid", "to must be a date (yyyy-MM-dd)")
		return "", "", ve
	}
	end, _ := time.Parse(timetable.DateLayout, to)
	switch {
	case end.Before(start):
		ve.add("to", "before_from", "to must not be before from")
	case end.After(start.AddDate(0, 0, model.MaxSessionDays-1)):
		ve.add("to", "too_long", fmt.Sprintf("at most %d days", model.MaxSessionDays))
	}
	return from, to, ve.orNil()
}

// Sessions lists the sessions of the timetable on the local dates of the
// filter (default: the next seven days), with the seats booked.
func (s *Service) Sessions(ctx context.Context, orgID int64, f model.SessionFilter) ([]model.Session, int, error) {
	from, to, err := s.period(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.q.ListClassTimetable(ctx, db.ListClassTimetableParams{
		OrganizationID: orgID, FromDate: pgDate(from), ToDate: pgDate(to), BranchIds: orgctx.BranchScope(ctx),
	})
	if err != nil {
		return nil, 0, err
	}
	q := strings.ToLower(strings.TrimSpace(f.Q))
	var keep []scheduleRow
	for _, raw := range rows {
		r := timetableRow(raw)
		staffUUID, _ := optUUID(r.StaffUUID)
		switch {
		case len(f.Branches) > 0 && !slices.Contains(f.Branches, r.BranchUUID),
			len(f.Services) > 0 && !slices.Contains(f.Services, r.ServiceUUID),
			len(f.Staff) > 0 && !slices.Contains(f.Staff, staffUUID):
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(r.ServiceName+" "+r.StaffName.String+" "+r.ResourceName.String+" "+r.BranchName), q) {
			continue
		}
		keep = append(keep, r)
	}
	ids := make([]int64, 0, len(keep))
	var services []int64
	for _, r := range keep {
		ids = append(ids, r.ID)
		if !slices.Contains(services, r.ServiceID) {
			services = append(services, r.ServiceID)
		}
	}
	changes, err := s.changes(ctx, s.q, orgID, ids, from, to)
	if err != nil {
		return nil, 0, err
	}
	first, _ := time.Parse(timetable.DateLayout, from)
	last, _ := time.Parse(timetable.DateLayout, to)
	seats, err := s.booked(ctx, s.q, orgID, services, first.AddDate(0, 0, -1), last.AddDate(0, 0, 2))
	if err != nil {
		return nil, 0, err
	}
	var out []model.Session
	for _, r := range keep {
		for _, occ := range r.entry().Occurrences(from, to) {
			var change *db.ListClassSessionChangesRow
			if c, ok := changes[changeKey{schedule: r.ID, date: occ.Date}]; ok {
				change = &c
			}
			session := s.session(r, occ, change, seats[seatKey{service: r.ServiceID, branch: r.BranchID, start: occ.Start.UnixNano()}])
			if len(f.States) > 0 && !slices.Contains(f.States, session.State) {
				continue
			}
			out = append(out, session)
		}
	}
	sortSessions(out, f.SortField, f.SortDesc)
	total := len(out)
	start := min(f.Offset, total)
	end := total
	if f.Limit > 0 {
		end = min(start+f.Limit, total)
	}
	page := out[start:end]
	if page == nil {
		page = []model.Session{}
	}
	return page, total, nil
}

func sortSessions(list []model.Session, field string, desc bool) {
	less := func(a, b model.Session) int {
		switch field {
		case "service":
			if c := strings.Compare(strings.ToLower(a.Service.Name), strings.ToLower(b.Service.Name)); c != 0 {
				return c
			}
		case "booked":
			if a.Booked != b.Booked {
				return a.Booked - b.Booked
			}
		}
		if c := a.StartsAt.Compare(b.StartsAt); c != 0 {
			return c
		}
		return strings.Compare(strings.ToLower(a.Service.Name), strings.ToLower(b.Service.Name))
	}
	sort.SliceStable(list, func(i, j int) bool {
		c := less(list[i], list[j])
		if desc {
			return c > 0
		}
		return c < 0
	})
}

// sessionOf loads one session of a class.
func (s *Service) sessionOf(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID, date string) (scheduleRow, timetable.Occurrence, *db.ListClassSessionChangesRow, error) {
	r, err := s.get(ctx, q, orgID, id)
	if err != nil {
		return scheduleRow{}, timetable.Occurrence{}, nil, err
	}
	occ, ok := r.entry().On(date)
	if !ok {
		return scheduleRow{}, timetable.Occurrence{}, nil, ErrSessionNotFound
	}
	changes, err := s.changes(ctx, q, orgID, []int64{r.ID}, date, date)
	if err != nil {
		return scheduleRow{}, timetable.Occurrence{}, nil, err
	}
	var change *db.ListClassSessionChangesRow
	if c, ok := changes[changeKey{schedule: r.ID, date: date}]; ok {
		change = &c
	}
	return r, occ, change, nil
}

func (s *Service) seatsAt(ctx context.Context, q *db.Queries, orgID int64, r scheduleRow, start time.Time) (int, error) {
	seats, err := s.booked(ctx, q, orgID, []int64{r.ServiceID}, start, start.Add(time.Second))
	if err != nil {
		return 0, err
	}
	return seats[seatKey{service: r.ServiceID, branch: r.BranchID, start: start.UnixNano()}], nil
}

// SessionDetail returns one session with its bookings.
func (s *Service) SessionDetail(ctx context.Context, orgID int64, id uuid.UUID, date string) (model.SessionDetail, error) {
	r, occ, change, err := s.sessionOf(ctx, s.q, orgID, id, date)
	if err != nil {
		return model.SessionDetail{}, err
	}
	booked, err := s.seatsAt(ctx, s.q, orgID, r, occ.Start)
	if err != nil {
		return model.SessionDetail{}, err
	}
	rows, err := s.q.ListClassSessionAttendees(ctx, db.ListClassSessionAttendeesParams{
		OrganizationID: orgID, BranchID: r.BranchID, ServiceID: pgtype.Int8{Int64: r.ServiceID, Valid: true}, StartsAt: ts(occ.Start),
	})
	if err != nil {
		return model.SessionDetail{}, err
	}
	out := model.SessionDetail{Session: s.session(r, occ, change, booked), Attendees: make([]model.Attendee, 0, len(rows))}
	for _, a := range rows {
		seats := 1
		if a.PartySize.Valid && a.PartySize.Int32 > 0 {
			seats = int(a.PartySize.Int32)
		}
		att := model.Attendee{
			ReservationUUID: a.Uuid, Number: a.Number, Status: a.Status, ApprovalStatus: a.ApprovalStatus,
			PaymentStatus: a.PaymentStatus, ConfirmationStatus: a.ConfirmationStatus, Source: a.Source, Seats: seats,
			Customer:  model.Customer{UUID: a.CustomerUuid, Name: strings.TrimSpace(a.FirstName + " " + a.LastName)},
			CreatedAt: a.CreatedAt.Time,
		}
		if a.PhoneE164.Valid {
			p := a.PhoneE164.String
			att.Customer.Phone = &p
		}
		out.Attendees = append(out.Attendees, att)
	}
	return out, nil
}

// ChangeSession cancels or restores one session or gives it other seats.
// It runs under the capacity lock: no booking slips into a session being
// cancelled.
func (s *Service) ChangeSession(ctx context.Context, orgID int64, id uuid.UUID, date string, actorID *int64, in model.SessionChangeInput) (model.Session, model.Session, error) {
	ve := &ValidationError{}
	reason := strings.TrimSpace(in.Reason)
	if utf8.RuneCountInString(reason) > model.MaxReason {
		ve.add("reason", "too_long", fmt.Sprintf("reason must be at most %d characters", model.MaxReason))
	}
	if in.Capacity != nil && (*in.Capacity < model.MinCapacity || *in.Capacity > model.MaxCapacity) {
		ve.add("capacity", "invalid", fmt.Sprintf("capacity must be between %d and %d", model.MinCapacity, model.MaxCapacity))
	}
	if !timetable.ValidDate(date) {
		return model.Session{}, model.Session{}, ErrSessionNotFound
	}
	if err := ve.orNil(); err != nil {
		return model.Session{}, model.Session{}, err
	}
	var before, after model.Session
	err := s.runner.Run(ctx, orgID, func(tx *availabilityusecase.Tx) error {
		r, occ, change, err := s.sessionOf(ctx, tx.Q, orgID, id, date)
		if err != nil {
			return err
		}
		booked, err := s.seatsAt(ctx, tx.Q, orgID, r, occ.Start)
		if err != nil {
			return err
		}
		before = s.session(r, occ, change, booked)
		if !occ.Start.After(tx.Now) {
			return ErrPast
		}
		if in.Cancelled && !before.Cancelled && booked > 0 {
			return ErrHasBookings
		}
		if in.Capacity != nil && *in.Capacity < booked {
			return ErrBelowBooked
		}
		if !in.Cancelled && in.Capacity == nil {
			if err := tx.Q.DeleteClassSessionChange(ctx, db.DeleteClassSessionChangeParams{
				OrganizationID: orgID, ScheduleID: r.ID, SessionDate: pgDate(date),
			}); err != nil {
				return err
			}
			after = s.session(r, occ, nil, booked)
			return nil
		}
		next := db.ListClassSessionChangesRow{ScheduleID: r.ID, SessionDate: pgDate(date), Cancelled: in.Cancelled, Reason: reason}
		if in.Capacity != nil {
			next.Capacity = pgtype.Int4{Int32: int32(*in.Capacity), Valid: true}
		}
		var by pgtype.Int8
		if actorID != nil {
			by = pgtype.Int8{Int64: *actorID, Valid: true}
		}
		if err := tx.Q.UpsertClassSessionChange(ctx, db.UpsertClassSessionChangeParams{
			OrganizationID: orgID, ScheduleID: r.ID, SessionDate: pgDate(date), Cancelled: in.Cancelled,
			Capacity: next.Capacity, Reason: reason, UpdatedBy: by,
		}); err != nil {
			return err
		}
		after = s.session(r, occ, &next, booked)
		return nil
	})
	return before, after, err
}
