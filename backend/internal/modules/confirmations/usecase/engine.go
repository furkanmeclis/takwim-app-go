// Package usecase is the customer confirmation engine (TEC-921,
// docs/design/confirmations.md): the tasks that ask a customer to confirm a
// booking before the visit, and cancel or flag the booking when no answer
// comes by the deadline (K-007).
//
//   - ReservationChanged runs inside the transaction of every reservation
//     change (the reservations Notifier port). It plans the task when a
//     booking becomes active or moves, follows the reservation's version on
//     every other change, and drops the task when the visit is cancelled,
//     started or finished. A task only exists while the Konfirmasyon
//     add-on is on and the branch's rule asks for confirmations.
//   - The worker (Run) sends the due requests and handles the deadlines.
//     Each step locks the reservation, then its task, and acts only when
//     the task tracks the reservation's current version and start, so an
//     old task never touches a moved, cancelled or finished visit (A12).
//   - Requests are customer notification dispatches (TEC-911 outbox): one
//     per attempt with the task's ordered channels and fallback. The
//     dispatch guard drops a queued request whose task was answered or
//     planned again, so a confirmation stops every pending repeat.
//   - The deadline cancels the booking through the reservations Cancel
//     transition (capacity, products and session rights are released
//     there) with the version the task tracks, or flags it. A booking whose
//     requests never left on any channel is flagged as unreachable, never
//     cancelled. A late answer never reopens a cancelled booking.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/confirmations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/catalog"
	customernotifyusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/usecase"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// Event is the customer notification event of a request.
const Event = catalog.ConfirmationRequest

// History event types (reservation_events; from / to are confirmation
// statuses).
const (
	EventScheduled       = "confirmation_scheduled"
	EventRequested       = "confirmation_requested"
	EventConfirmed       = "confirmation_confirmed"
	EventDeclined        = "confirmation_declined"
	EventChangeRequested = "confirmation_change_requested"
	EventExpired         = "confirmation_expired"
	EventDropped         = "confirmation_dropped"
)

var (
	// ErrNotOpen: the booking has no confirmation that takes this answer
	// now (none planned, already answered, the visit is not active).
	ErrNotOpen = errors.New("the confirmation does not take this answer")
	// ErrNotFound: the reservation is missing.
	ErrNotFound = errors.New("reservation not found")
)

// Notices writes request dispatches (the customer notification engine).
type Notices interface {
	EmitReservation(ctx context.Context, q *db.Queries, orgID, reservationID int64, n customernotifyusecase.ReservationNotice) (bool, error)
	Ready(name string) bool
}

// Canceller cancels a booking through its transition (the reservations
// use case).
type Canceller interface {
	Cancel(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, actorID int64, in reservationsusecase.ChangeInput) (reservationsmodel.Reservation, error)
}

// Engine is the confirmation engine.
type Engine struct {
	pool         *pgxpool.Pool
	q            *db.Queries
	notices      Notices
	reservations Canceller
	log          *slog.Logger
	now          func() time.Time
	batch        int32
}

var _ reservationsusecase.Notifier = (*Engine)(nil)

// New builds the engine. reservations may be set later (SetReservations)
// because the reservations use case takes the engine as its notifier.
func New(pool *pgxpool.Pool, q *db.Queries, notices Notices) *Engine {
	return &Engine{pool: pool, q: q, notices: notices, log: slog.Default(), now: time.Now, batch: 100}
}

// SetReservations sets the use case the deadline cancels through.
func (e *Engine) SetReservations(r Canceller) { e.reservations = r }

// SetLogger replaces the logger.
func (e *Engine) SetLogger(l *slog.Logger) {
	if l != nil {
		e.log = l
	}
}

// SetClock replaces the clock (tests).
func (e *Engine) SetClock(now func() time.Time) { e.now = now }

func (e *Engine) clock() time.Time { return e.now().UTC().Truncate(time.Microsecond) }

func (e *Engine) ready(ch string) bool { return e.notices != nil && e.notices.Ready(ch) }

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func (e *Engine) tx(ctx context.Context, fn func(q *db.Queries) error) error {
	if e.pool == nil {
		return fn(e.q)
	}
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(e.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// open reports whether a task still waits for something.
func open(status string) bool {
	return status == model.TaskScheduled || status == model.TaskPending
}

// ModuleOn reports whether the business may use the add-on now.
func ModuleOn(ctx context.Context, q *db.Queries, orgID int64) (bool, error) {
	o, err := q.GetConfirmationOrganization(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if o.Deleted || o.Status != orgctx.StatusActive {
		return false, nil
	}
	st := entitlement.Stored{Plan: o.Plan, Addons: o.Addons}
	if o.BranchLimit.Valid {
		st.BranchLimit = &o.BranchLimit.Int32
	}
	if o.StaffLimit.Valid {
		st.StaffLimit = &o.StaffLimit.Int32
	}
	return entitlement.Resolve(o.Sector, st).Allows(sector.ModuleConfirmation), nil
}

func (e *Engine) location(ctx context.Context, q *db.Queries, orgID, branchID int64) *time.Location {
	zone, err := q.GetConfirmationTimezone(ctx, db.GetConfirmationTimezoneParams{OrganizationID: orgID, BranchID: branchID})
	if err == nil {
		if loc, err := time.LoadLocation(zone); err == nil {
			return loc
		}
	}
	if loc, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return loc
	}
	return time.UTC
}

// state is a task being changed.
type state struct {
	row db.ReservationConfirmation
}

func (s *state) save(ctx context.Context, q *db.Queries) error {
	r := s.row
	return q.SaveReservationConfirmation(ctx, db.SaveReservationConfirmationParams{
		OrganizationID: r.OrganizationID, ID: r.ID, ReservationVersion: r.ReservationVersion, Status: r.Status,
		AttemptsSent: r.AttemptsSent, NextAttemptAt: r.NextAttemptAt, Answer: r.Answer, AnswerChannel: r.AnswerChannel,
		AnsweredAt: r.AnsweredAt, ChangeRequestedAt: r.ChangeRequestedAt, ChangeNote: r.ChangeNote, Outcome: r.Outcome,
		LastError: r.LastError, ClosedAt: r.ClosedAt,
	})
}

func (s *state) close(status string, now time.Time) {
	s.row.Status, s.row.NextAttemptAt, s.row.ClosedAt = status, pgtype.Timestamptz{}, ts(now)
}

// setStatus writes the reservation's confirmation status and a history
// row. bump raises the version (a change of the engine's own); otherwise
// the row belongs to the change being written (its version).
func (e *Engine) setStatus(ctx context.Context, q *db.Queries, orgID, reservationID int64, from, to string, bump bool, typ, note string) (int32, error) {
	version, err := q.SetReservationConfirmationStatus(ctx, db.SetReservationConfirmationStatusParams{
		OrganizationID: orgID, ID: reservationID, ConfirmationStatus: to, Bump: bump,
	})
	if err != nil {
		return 0, err
	}
	if typ == "" {
		return version, nil
	}
	return version, q.CreateConfirmationEvent(ctx, db.CreateConfirmationEventParams{
		OrganizationID: orgID, ReservationID: reservationID, Version: version, Type: typ,
		FromStatus: text(from), ToStatus: text(to), Note: note,
	})
}

// ReservationChanged implements the reservations Notifier.
func (e *Engine) ReservationChanged(ctx context.Context, q *db.Queries, c reservationsusecase.Change) error {
	r, err := q.GetConfirmationReservation(ctx, db.GetConfirmationReservationParams{OrganizationID: c.OrganizationID, ID: c.ReservationID})
	if err != nil {
		return fmt.Errorf("confirmation: reservation: %w", err)
	}
	task, err := q.LockReservationConfirmation(ctx, db.LockReservationConfirmationParams{OrganizationID: c.OrganizationID, ReservationID: r.ID})
	has := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	isOpen := has && open(task.Status)
	active := r.Status == reservationsmodel.StatusActive && r.ApprovalStatus == reservationsmodel.ApprovalAccepted
	st := &state{row: task}
	now := e.clock()
	switch c.Type {
	case "cancelled":
		if !isOpen {
			return nil
		}
		st.row.ReservationVersion = c.Version
		switch c.CancelReason {
		case reservationsmodel.ReasonConfirmationTimeout:
			st.close(model.TaskExpired, now)
			st.row.Outcome = text(model.OutcomeCancelled)
			if err := st.save(ctx, q); err != nil {
				return err
			}
			_, err := e.setStatus(ctx, q, c.OrganizationID, r.ID, r.ConfirmationStatus, reservationsmodel.ConfirmationExpired, false, EventExpired, model.OutcomeCancelled)
			return err
		case reservationsmodel.ReasonCustomerCancelled:
			// "Gelmeyeceğim": the customer cancelled while the confirmation
			// was open.
			st.close(model.TaskDeclined, now)
			st.row.Answer, st.row.AnsweredAt = text(model.AnswerCancel), ts(now)
			if err := st.save(ctx, q); err != nil {
				return err
			}
			_, err := e.setStatus(ctx, q, c.OrganizationID, r.ID, r.ConfirmationStatus, reservationsmodel.ConfirmationDeclined, false, EventDeclined, "")
			return err
		}
		st.close(model.TaskDropped, now)
		st.row.LastError = "reservation cancelled"
		return st.save(ctx, q)
	case "created", "accepted", "paid", "rescheduled":
		if active {
			if c.Type != "rescheduled" && isOpen && task.StartsAt.Time.Equal(r.StartsAt.Time) {
				st.row.ReservationVersion = c.Version
				return st.save(ctx, q)
			}
			return e.plan(ctx, q, c.OrganizationID, r, st, isOpen, c.Version, now)
		}
	}
	if !isOpen {
		return nil
	}
	st.row.ReservationVersion = c.Version
	if !active || !task.StartsAt.Time.Equal(r.StartsAt.Time) {
		st.close(model.TaskDropped, now)
		st.row.LastError = "reservation " + c.Type
		return st.save(ctx, q)
	}
	return st.save(ctx, q)
}

// plan plans (or plans again) the task of an active booking in the change's
// transaction. Without the add-on, with the branch's rule off, or too late
// to leave time for an answer, no task is set (and an open one is dropped).
func (e *Engine) plan(ctx context.Context, q *db.Queries, orgID int64, r db.GetConfirmationReservationRow, st *state, isOpen bool, version int32, now time.Time) error {
	on, err := ModuleOn(ctx, q, orgID)
	if err != nil {
		return err
	}
	rule := model.Rule{}
	if on {
		if rule, err = RuleFor(ctx, q, orgID, r.BranchID); err != nil {
			return err
		}
	}
	var sched Schedule
	ok := on && rule.Enabled
	if ok {
		sched, ok = PlanSchedule(rule, r.StartsAt.Time, now, e.location(ctx, q, orgID, r.BranchID))
	}
	if !ok {
		if isOpen {
			st.row.ReservationVersion = version
			st.close(model.TaskDropped, now)
			st.row.LastError = "confirmation not required"
			if err := st.save(ctx, q); err != nil {
				return err
			}
		}
		if r.ConfirmationStatus != reservationsmodel.ConfirmationNotRequired {
			_, err := e.setStatus(ctx, q, orgID, r.ID, r.ConfirmationStatus, reservationsmodel.ConfirmationNotRequired, false, "", "")
			return err
		}
		return nil
	}
	times := make([]pgtype.Timestamptz, 0, len(sched.Attempts))
	for _, t := range sched.Attempts {
		times = append(times, ts(t))
	}
	if _, err := q.PlanReservationConfirmation(ctx, db.PlanReservationConfirmationParams{
		OrganizationID: orgID, ReservationID: r.ID, ReservationVersion: version, StartsAt: r.StartsAt,
		Channels: rule.Channels, Fallback: rule.Fallback, OnNoAnswer: rule.OnNoAnswer, AttemptTimes: times,
		NextAttemptAt: times[0], DeadlineAt: ts(sched.Deadline), PlannedAt: ts(now),
	}); err != nil {
		return err
	}
	_, err = e.setStatus(ctx, q, orgID, r.ID, r.ConfirmationStatus, reservationsmodel.ConfirmationScheduled, false, EventScheduled,
		sched.Deadline.Format(time.RFC3339))
	return err
}

// Guard implements the customer notification dispatch guard: a request
// goes out only while its task is open and of the same round.
func (e *Engine) Guard(ctx context.Context, q *db.Queries, d db.CustomerNotificationDispatch) (string, error) {
	if d.EventCode != Event || !d.ReservationID.Valid {
		return "", nil
	}
	t, err := q.GetReservationConfirmation(ctx, db.GetReservationConfirmationParams{OrganizationID: d.OrganizationID, ReservationID: d.ReservationID.Int64})
	if errors.Is(err, pgx.ErrNoRows) {
		return "confirmation not planned", nil
	}
	if err != nil {
		return "", err
	}
	if !open(t.Status) {
		return "confirmation " + t.Status, nil
	}
	if !strings.Contains(d.IdempotencyKey, "|"+roundKey(t.Round)+":a") {
		return "confirmation planned again", nil
	}
	return "", nil
}

// roundKey names a task round in request keys ("r2").
func roundKey(round int32) string { return "r" + strconv.Itoa(int(round)) }

// cancelJob is a deadline cancel run after the task's transaction.
type cancelJob struct {
	orgID   int64
	id      uuid.UUID
	version int
}

// Process runs the due step of one task: the next request or the
// deadline.
func (e *Engine) Process(ctx context.Context, orgID, reservationID int64) error {
	var job *cancelJob
	err := e.tx(ctx, func(q *db.Queries) error {
		var err error
		job, err = e.step(ctx, q, orgID, reservationID)
		return err
	})
	if err != nil || job == nil {
		return err
	}
	if e.reservations == nil {
		return errors.New("confirmation: no reservations use case to cancel through")
	}
	_, err = e.reservations.Cancel(ctx, job.orgID, job.id, availabilityusecase.Caller{}, 0, reservationsusecase.ChangeInput{
		ExpectedVersion: &job.version, Reason: reservationsmodel.ReasonConfirmationTimeout,
		Guard: func(l reservationsmodel.Locked) error {
			if l.ConfirmationStatus != reservationsmodel.ConfirmationScheduled && l.ConfirmationStatus != reservationsmodel.ConfirmationPending {
				return reservationsusecase.ErrInvalidTransition
			}
			return nil
		},
	})
	// Changed since (answered, moved, started): the next scan sees the
	// new state.
	if errors.Is(err, reservationsusecase.ErrStaleVersion) || errors.Is(err, reservationsusecase.ErrInvalidTransition) ||
		errors.Is(err, reservationsusecase.ErrNotFound) {
		return nil
	}
	return err
}

func (e *Engine) step(ctx context.Context, q *db.Queries, orgID, reservationID int64) (*cancelJob, error) {
	r, err := q.GetConfirmationReservation(ctx, db.GetConfirmationReservationParams{OrganizationID: orgID, ID: reservationID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	task, err := q.LockReservationConfirmation(ctx, db.LockReservationConfirmationParams{OrganizationID: orgID, ReservationID: reservationID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !open(task.Status) {
		return nil, nil
	}
	now := e.clock()
	st := &state{row: task}
	// A task that does not track the reservation any more never acts.
	if task.ReservationVersion != r.Version || !task.StartsAt.Time.Equal(r.StartsAt.Time) ||
		r.Status != reservationsmodel.StatusActive {
		st.close(model.TaskDropped, now)
		st.row.LastError = "reservation changed"
		return nil, st.save(ctx, q)
	}
	on, err := ModuleOn(ctx, q, orgID)
	if err != nil {
		return nil, err
	}
	rule := model.Rule{}
	if on {
		if rule, err = RuleFor(ctx, q, orgID, r.BranchID); err != nil {
			return nil, err
		}
	}
	if !on || !rule.Enabled {
		// The add-on or the branch's rule was turned off: no request and no
		// cancel any more.
		version, err := e.setStatus(ctx, q, orgID, r.ID, r.ConfirmationStatus, reservationsmodel.ConfirmationNotRequired, true, EventDropped, "")
		if err != nil {
			return nil, err
		}
		st.row.ReservationVersion = version
		st.close(model.TaskDropped, now)
		st.row.LastError = "confirmation turned off"
		return nil, st.save(ctx, q)
	}
	if !now.Before(task.DeadlineAt.Time) && !task.ChangeRequestedAt.Valid {
		return e.deadline(ctx, q, r, st, now)
	}
	if !task.NextAttemptAt.Valid || task.NextAttemptAt.Time.After(now) {
		return nil, nil
	}
	return nil, e.attempt(ctx, q, r, st, now)
}

// attempt writes the next request and moves the task on.
func (e *Engine) attempt(ctx context.Context, q *db.Queries, r db.GetConfirmationReservationRow, st *state, now time.Time) error {
	t := st.row
	n := int(t.AttemptsSent) + 1
	loc := e.location(ctx, q, t.OrganizationID, r.BranchID)
	due := r.StartsAt.Time
	if _, err := e.notices.EmitReservation(ctx, q, t.OrganizationID, r.ID, customernotifyusecase.ReservationNotice{
		Event: Event, Key: roundKey(t.Round) + ":a" + strconv.Itoa(n),
		Channels: t.Channels, Fallback: t.Fallback, DueAt: &due,
		Vars: map[string]string{"deadline": t.DeadlineAt.Time.In(loc).Format("02.01.2006 15:04")},
	}); err != nil {
		return err
	}
	st.row.AttemptsSent = int32(n)
	st.row.NextAttemptAt = pgtype.Timestamptz{}
	if n < len(t.AttemptTimes) {
		st.row.NextAttemptAt = t.AttemptTimes[n]
	}
	note := strconv.Itoa(n) + "/" + strconv.Itoa(len(t.AttemptTimes))
	bump := t.Status == model.TaskScheduled
	version, err := e.setStatus(ctx, q, t.OrganizationID, r.ID, r.ConfirmationStatus, reservationsmodel.ConfirmationPending, bump, EventRequested, note)
	if err != nil {
		return err
	}
	st.row.Status, st.row.ReservationVersion = model.TaskPending, version
	return st.save(ctx, q)
}

// deadline ends a task without an answer: cancel (returned as a job, run
// through the reservations transition), or flag. Without a single request
// that left on a channel the booking is flagged as unreachable.
func (e *Engine) deadline(ctx context.Context, q *db.Queries, r db.GetConfirmationReservationRow, st *state, now time.Time) (*cancelJob, error) {
	t := st.row
	sent, err := q.CountConfirmationSends(ctx, db.CountConfirmationSendsParams{
		OrganizationID: t.OrganizationID, ReservationID: pgtype.Int8{Int64: r.ID, Valid: true}, EventCode: Event, Since: t.PlannedAt,
	})
	if err != nil {
		return nil, err
	}
	outcome := model.OutcomeFlagged
	switch {
	case sent == 0:
		outcome = model.OutcomeUnreachable
	case t.OnNoAnswer == model.OnNoAnswerCancel:
		return &cancelJob{orgID: t.OrganizationID, id: r.Uuid, version: int(r.Version)}, nil
	}
	version, err := e.setStatus(ctx, q, t.OrganizationID, r.ID, r.ConfirmationStatus, reservationsmodel.ConfirmationExpired, true, EventExpired, outcome)
	if err != nil {
		return nil, err
	}
	st.row.ReservationVersion = version
	st.close(model.TaskExpired, now)
	st.row.Outcome = text(outcome)
	return nil, st.save(ctx, q)
}

// AnswerInput is a customer's answer (the customer panel, a visit link;
// WhatsApp and voice with TEC-922 / TEC-923).
type AnswerInput struct {
	// Answer is model.AnswerConfirm or model.AnswerChange (a cancel goes
	// through the reservation's cancel under the business's rules).
	Answer string
	Source string
	Note   string
	// Guard runs on the locked reservation (ownership).
	Guard func(reservationsmodel.Locked) error
}

// CanAnswer reports whether a task takes an answer now: open, or expired
// without a cancel (flagged: a late "Geleceğim" still counts while the
// booking stands).
func CanAnswer(c *reservationsmodel.Confirmation) bool {
	if c == nil {
		return false
	}
	if open(c.Status) {
		return true
	}
	return c.Status == model.TaskExpired && (c.Outcome == nil || *c.Outcome != model.OutcomeCancelled)
}

// Answer records a customer's answer. A confirm closes the task (no more
// requests, no cancel); a change request keeps the booking as it is, stops
// the requests and the cancel, and leaves the change to the business. A
// cancelled or finished booking takes no answer (ErrNotOpen).
func (e *Engine) Answer(ctx context.Context, orgID int64, id uuid.UUID, in AnswerInput) error {
	if in.Answer != model.AnswerConfirm && in.Answer != model.AnswerChange {
		return ErrNotOpen
	}
	note := strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(note) > model.MaxChangeNote {
		ve := &ValidationError{}
		ve.add("note", "too_long", "note is at most 500 characters")
		return ve
	}
	return e.tx(ctx, func(q *db.Queries) error {
		r, err := q.GetConfirmationReservationByUUID(ctx, db.GetConfirmationReservationByUUIDParams{OrganizationID: orgID, Uuid: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if in.Guard != nil {
			if err := in.Guard(reservationsmodel.Locked{
				CustomerID: r.CustomerID, Status: r.Status, ConfirmationStatus: r.ConfirmationStatus,
				StartsAt: r.StartsAt.Time, Version: int(r.Version),
			}); err != nil {
				return err
			}
		}
		task, err := q.LockReservationConfirmation(ctx, db.LockReservationConfirmationParams{OrganizationID: orgID, ReservationID: r.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotOpen
		}
		if err != nil {
			return err
		}
		now := e.clock()
		answerable := open(task.Status) || (task.Status == model.TaskExpired && task.Outcome.String != model.OutcomeCancelled)
		if r.Status != reservationsmodel.StatusActive || !now.Before(r.StartsAt.Time) || !answerable {
			return ErrNotOpen
		}
		if on, err := ModuleOn(ctx, q, orgID); err != nil {
			return err
		} else if !on {
			return ErrNotOpen
		}
		st := &state{row: task}
		st.row.Answer, st.row.AnswerChannel, st.row.AnsweredAt = text(in.Answer), text(in.Source), ts(now)
		switch in.Answer {
		case model.AnswerConfirm:
			version, err := e.setStatus(ctx, q, orgID, r.ID, r.ConfirmationStatus, reservationsmodel.ConfirmationConfirmed, true, EventConfirmed, in.Source)
			if err != nil {
				return err
			}
			st.row.ReservationVersion = version
			st.close(model.TaskConfirmed, now)
		default:
			if task.ChangeRequestedAt.Valid {
				return ErrNotOpen
			}
			to := r.ConfirmationStatus
			if to == reservationsmodel.ConfirmationScheduled {
				to = reservationsmodel.ConfirmationPending
			}
			version, err := e.setStatus(ctx, q, orgID, r.ID, r.ConfirmationStatus, to, true, EventChangeRequested, note)
			if err != nil {
				return err
			}
			st.row.ReservationVersion = version
			st.row.ChangeRequestedAt, st.row.ChangeNote, st.row.NextAttemptAt = ts(now), note, pgtype.Timestamptz{}
			if task.Status == model.TaskScheduled {
				st.row.Status = model.TaskPending
			}
		}
		return st.save(ctx, q)
	})
}

// RunDue processes every due task and returns how many it handled.
func (e *Engine) RunDue(ctx context.Context) (int, error) {
	return e.runDue(ctx, pgtype.Int8{})
}

// RunDueOrganization processes one business's due tasks.
func (e *Engine) RunDueOrganization(ctx context.Context, orgID int64) (int, error) {
	return e.runDue(ctx, pgtype.Int8{Int64: orgID, Valid: true})
}

func (e *Engine) runDue(ctx context.Context, orgID pgtype.Int8) (int, error) {
	rows, err := e.q.ListDueReservationConfirmations(ctx, db.ListDueReservationConfirmationsParams{
		NowAt: ts(e.clock()), LimitCount: e.batch, OrganizationID: orgID,
	})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		if err := e.Process(ctx, r.OrganizationID, r.ReservationID); err != nil {
			e.log.WarnContext(ctx, "confirmation_task_failed", "organization", r.OrganizationID, "reservation", r.ReservationID, "error", err)
			continue
		}
		n++
	}
	return n, nil
}

// DefaultInterval is how often the worker looks for due tasks.
const DefaultInterval = 15 * time.Second

// Run processes due tasks every interval until ctx ends (the worker).
func (e *Engine) Run(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = DefaultInterval
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if _, err := e.RunDue(ctx); err != nil && ctx.Err() == nil {
			e.log.WarnContext(ctx, "confirmation_scan_failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
