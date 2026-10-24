package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
)

// Cancelling a session with its bookings (TEC-989):
//
//   - CancelWithBookings marks the session cancelled under the capacity
//     lock, so no booking slips in, and lists its open bookings (requests
//     and active bookings). The bulk run then cancels each one through the
//     reservation's own cancel: capacity, package sessions and products
//     are freed, a payment waits for its refund and the customer gets the
//     cancellation notice.
//   - The run can be rolled back in the bulk window: ReopenSessionIn opens
//     the session again inside the transaction that takes each booking
//     back, so the booking finds its start offered again.

// bookable are the booking states a session cancel takes down.
var bookable = map[string]bool{"requested": true, "active": true}

// CancelWithBookings cancels one session that may have bookings and
// answers the session before the change and the open bookings to cancel.
// A session already cancelled (an earlier run that stopped halfway) is
// fine: its open bookings are listed again. A session that started is not
// changed (ErrPast).
func (s *Service) CancelWithBookings(ctx context.Context, orgID int64, id uuid.UUID, date string, actorID *int64, reason string) (model.Session, []uuid.UUID, error) {
	reason = strings.TrimSpace(reason)
	if utf8.RuneCountInString(reason) > model.MaxReason {
		ve := &ValidationError{}
		ve.add("reason", "too_long", fmt.Sprintf("reason must be at most %d characters", model.MaxReason))
		return model.Session{}, nil, ve
	}
	if !timetable.ValidDate(date) {
		return model.Session{}, nil, ErrSessionNotFound
	}
	var before model.Session
	var targets []uuid.UUID
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
		var capacity pgtype.Int4
		if change != nil {
			capacity = change.Capacity
		}
		var by pgtype.Int8
		if actorID != nil {
			by = pgtype.Int8{Int64: *actorID, Valid: true}
		}
		if err := tx.Q.UpsertClassSessionChange(ctx, db.UpsertClassSessionChangeParams{
			OrganizationID: orgID, ScheduleID: r.ID, SessionDate: pgDate(date), Cancelled: true,
			Capacity: capacity, Reason: reason, UpdatedBy: by,
		}); err != nil {
			return err
		}
		rows, err := tx.Q.ListClassSessionAttendees(ctx, db.ListClassSessionAttendeesParams{
			OrganizationID: orgID, BranchID: r.BranchID, ServiceID: pgtype.Int8{Int64: r.ServiceID, Valid: true}, StartsAt: ts(occ.Start),
		})
		if err != nil {
			return err
		}
		for _, a := range rows {
			if bookable[a.Status] {
				targets = append(targets, a.Uuid)
			}
		}
		return nil
	})
	return before, targets, err
}

// SessionStart answers the start of one session of a class (a booking of
// the session starts then).
func (s *Service) SessionStart(ctx context.Context, orgID int64, id uuid.UUID, date string) (time.Time, uuid.UUID, error) {
	if !timetable.ValidDate(date) {
		return time.Time{}, uuid.Nil, ErrSessionNotFound
	}
	r, occ, _, err := s.sessionOf(ctx, s.q, orgID, id, date)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	return occ.Start, r.BranchUUID, nil
}

// ReopenSessionIn opens a cancelled session again on q (the transaction
// of a booking being taken back, under the capacity lock). The session's
// own seats stay; an open session is left as it is. A session that started
// is not changed (ErrPast).
func (s *Service) ReopenSessionIn(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID, date string, now time.Time) error {
	if !timetable.ValidDate(date) {
		return ErrSessionNotFound
	}
	r, occ, change, err := s.sessionOf(ctx, q, orgID, id, date)
	if err != nil {
		return err
	}
	if change == nil || !change.Cancelled {
		return nil
	}
	if !occ.Start.After(now) {
		return ErrPast
	}
	if !change.Capacity.Valid {
		return q.DeleteClassSessionChange(ctx, db.DeleteClassSessionChangeParams{
			OrganizationID: orgID, ScheduleID: r.ID, SessionDate: pgDate(date),
		})
	}
	return q.UpsertClassSessionChange(ctx, db.UpsertClassSessionChangeParams{
		OrganizationID: orgID, ScheduleID: r.ID, SessionDate: pgDate(date), Cancelled: false,
		Capacity: change.Capacity, Reason: "",
	})
}

// CancelJob answers the caller's latest run that cancelled the bookings of
// a session while it can still be rolled back (nil: none).
func (s *Service) CancelJob(ctx context.Context, orgID, actorID int64, id uuid.UUID, date string) (*model.SessionCancellation, error) {
	row, err := s.q.GetClassSessionCancelJob(ctx, db.GetClassSessionCancelJobParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, Resource: model.SessionsSlug, ActorID: actorID,
		ClassUuid: id.String(), SessionDate: date,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var until *time.Time
	if row.RollbackUntil.Valid {
		t := row.RollbackUntil.Time.UTC()
		until = &t
	}
	c := cancellation(row.Uuid, row.ResultJson, until, row.CreatedAt.Time)
	return &c, nil
}

// cancellation reads the summary of a bulk run into a session cancellation.
func cancellation(job uuid.UUID, result []byte, rollbackUntil *time.Time, createdAt time.Time) model.SessionCancellation {
	var sum struct {
		Total     int `json:"total"`
		Succeeded int `json:"succeeded"`
		Failed    int `json:"failed"`
		Skipped   int `json:"skipped"`
	}
	_ = json.Unmarshal(result, &sum)
	return model.SessionCancellation{
		JobUUID: job, Total: sum.Total, Succeeded: sum.Succeeded, Failed: sum.Failed, Skipped: sum.Skipped,
		RollbackUntil: rollbackUntil, CreatedAt: createdAt.UTC(),
	}
}
