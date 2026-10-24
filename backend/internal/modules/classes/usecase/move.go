package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/timetable"
)

// Moving the bookings of a session to another session of the class
// (TEC-994):
//
//   - MoveTargets checks the target session (another date of the same
//     class, open, not started) and lists the open bookings of the source
//     session. The bulk run then moves each one through the reservation's
//     own reschedule, so seats are held in the target under the capacity
//     lock, package sessions stay where the validity allows and the
//     customer gets the rescheduled notice. A booking that finds no seat
//     (or its trainer or area busy) stays where it was and is reported.
//   - The run can be rolled back in the bulk window: each booking is
//     rescheduled back to the source session, as long as it still exists
//     and is open.

// MoveTargets answers the source and the target session and the open
// bookings of the source. The source is read first, so an unknown class or
// date is ErrSessionNotFound before the body is judged; a bad target is a
// ValidationError on to_date (required, invalid, same_session, not_found,
// cancelled, past, full).
func (s *Service) MoveTargets(ctx context.Context, orgID int64, id uuid.UUID, date string, in model.SessionMoveInput) (model.Session, model.Session, []uuid.UUID, error) {
	if !timetable.ValidDate(date) {
		return model.Session{}, model.Session{}, nil, ErrSessionNotFound
	}
	r, occ, change, err := s.sessionOf(ctx, s.q, orgID, id, date)
	if err != nil {
		return model.Session{}, model.Session{}, nil, err
	}
	booked, err := s.seatsAt(ctx, s.q, orgID, r, occ.Start)
	if err != nil {
		return model.Session{}, model.Session{}, nil, err
	}
	from := s.session(r, occ, change, booked)

	ve := &ValidationError{}
	toDate := strings.TrimSpace(in.ToDate)
	switch {
	case toDate == "":
		ve.add("to_date", "required", "to_date is required")
	case !timetable.ValidDate(toDate):
		ve.add("to_date", "invalid", "to_date must be a date (yyyy-MM-dd)")
	case toDate == date:
		ve.add("to_date", "same_session", "the target is the session itself")
	}
	if err := ve.orNil(); err != nil {
		return model.Session{}, model.Session{}, nil, err
	}
	tOcc, ok := r.entry().On(toDate)
	if !ok {
		ve.add("to_date", "not_found", "the class has no session on that date")
		return model.Session{}, model.Session{}, nil, ve
	}
	tChanges, err := s.changes(ctx, s.q, orgID, []int64{r.ID}, toDate, toDate)
	if err != nil {
		return model.Session{}, model.Session{}, nil, err
	}
	var tChange *db.ListClassSessionChangesRow
	if c, ok := tChanges[changeKey{schedule: r.ID, date: toDate}]; ok {
		tChange = &c
	}
	tBooked, err := s.seatsAt(ctx, s.q, orgID, r, tOcc.Start)
	if err != nil {
		return model.Session{}, model.Session{}, nil, err
	}
	to := s.session(r, tOcc, tChange, tBooked)
	switch to.State {
	case model.StateCancelled:
		ve.add("to_date", "cancelled", "the target session is cancelled")
	case model.StatePast:
		ve.add("to_date", "past", "the target session has already started")
	case model.StateFull:
		ve.add("to_date", "full", "the target session has no seat left")
	}
	if err := ve.orNil(); err != nil {
		return model.Session{}, model.Session{}, nil, err
	}

	rows, err := s.q.ListClassSessionAttendees(ctx, db.ListClassSessionAttendeesParams{
		OrganizationID: orgID, BranchID: r.BranchID, ServiceID: pgtype.Int8{Int64: r.ServiceID, Valid: true}, StartsAt: ts(occ.Start),
	})
	if err != nil {
		return model.Session{}, model.Session{}, nil, err
	}
	var targets []uuid.UUID
	for _, a := range rows {
		if bookable[a.Status] {
			targets = append(targets, a.Uuid)
		}
	}
	return from, to, targets, nil
}

// MoveJob answers the caller's latest run that moved the bookings of a
// session away while it can still be rolled back (nil: none).
func (s *Service) MoveJob(ctx context.Context, orgID, actorID int64, id uuid.UUID, date string) (*model.SessionMove, error) {
	row, err := s.q.GetClassSessionBulkJob(ctx, db.GetClassSessionBulkJobParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, Resource: model.SessionsSlug, Action: model.MoveBookingsAction,
		ActorID: actorID, ClassUuid: id.String(), SessionDate: date,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var target struct {
		Params map[string]string `json:"params"`
	}
	_ = json.Unmarshal(row.TargetJson, &target)
	m := move(row.Uuid, target.Params["to_date"], row.ResultJson, rollbackUntil(row.RollbackUntil), row.CreatedAt.Time)
	return &m, nil
}

// move reads the summary of a bulk run into a session move.
func move(job uuid.UUID, toDate string, result []byte, rollbackUntil *time.Time, createdAt time.Time) model.SessionMove {
	var sum runSummary
	_ = json.Unmarshal(result, &sum)
	out := model.SessionMove{
		JobUUID: job, ToDate: toDate, Total: sum.Total, Succeeded: sum.Succeeded, Failed: sum.Failed, Skipped: sum.Skipped,
		Issues: make([]model.SessionMoveIssue, 0, len(sum.Issues)), RollbackUntil: rollbackUntil, CreatedAt: createdAt.UTC(),
	}
	for _, i := range sum.Issues {
		out.Issues = append(out.Issues, model.SessionMoveIssue{ReservationUUID: i.EntityUUID, Label: i.Label, Error: i.Error, Skipped: i.Skipped})
	}
	return out
}
