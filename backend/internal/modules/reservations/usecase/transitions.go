package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilitymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
)

// ChangeInput is the common part of a transition. ExpectedVersion, when
// given, must be the version the caller read (ErrStaleVersion otherwise).
type ChangeInput struct {
	ExpectedVersion *int   `json:"expected_version"`
	Reason          string `json:"reason"`
	Note            string `json:"note"`
	// Guard, when set, runs on the locked record before the step (the
	// customer panel checks ownership and the cancel cutoff there). Never
	// read from a request body.
	Guard func(model.Locked) error `json:"-"`
}

// lockedView is what a guard sees of the locked row.
func lockedView(r db.Reservation) model.Locked {
	return model.Locked{
		CustomerID: r.CustomerID, Status: r.Status, PaymentStatus: r.PaymentStatus,
		StartsAt: r.StartsAt.Time, Version: int(r.Version),
	}
}

// state is what a transition changes.
type state struct {
	status, approval, payment, cancelReason string
}

func stateOf(r db.Reservation) state {
	s := state{status: r.Status, approval: r.ApprovalStatus, payment: r.PaymentStatus}
	if r.CancelReason.Valid {
		s.cancelReason = r.CancelReason.String
	}
	return s
}

// unpaid ends a payment that never happened (the booking will not be
// paid): online_pending → failed; a paid booking waits for its refund.
func unpaid(payment string) string {
	switch payment {
	case model.PaymentOnlinePending:
		return model.PaymentFailed
	case model.PaymentPaid:
		return model.PaymentRefundPending
	}
	return payment
}

// transition loads the record under the capacity lock, checks visibility
// and version, lets step change capacity and state, then writes the new
// state (version + 1) and the event.
func (s *Service) transition(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, actorID int64, in ChangeInput, typ string,
	step func(tx *availabilityusecase.Tx, row db.Reservation, next *state) error,
) (model.Reservation, error) {
	if len(in.Note) > MaxNoteLength {
		ve := &ValidationError{}
		ve.add("note", "too_long", "note is too long")
		return model.Reservation{}, ve
	}
	err := s.holds.Run(ctx, orgID, func(tx *availabilityusecase.Tx) error {
		row, err := s.visible(ctx, tx.Q, orgID, id, caller, true)
		if err != nil {
			return err
		}
		if in.ExpectedVersion != nil && *in.ExpectedVersion != int(row.Version) {
			return ErrStaleVersion
		}
		if in.Guard != nil {
			if err := in.Guard(lockedView(row)); err != nil {
				return err
			}
		}
		// A status belongs to the whole visit: an own calendar member
		// changes it only when it serves every staffed line.
		if shared, err := s.sharedWithOthers(ctx, tx.Q, orgID, row, caller); err != nil {
			return err
		} else if shared {
			return ErrOtherStaff
		}
		next := stateOf(row)
		if err := step(tx, row, &next); err != nil {
			return err
		}
		updated, err := tx.Q.UpdateReservationState(ctx, db.UpdateReservationStateParams{
			OrganizationID: orgID, ID: row.ID, Status: next.status, ApprovalStatus: next.approval,
			PaymentStatus: next.payment, CancelReason: pgtype.Text{String: next.cancelReason, Valid: next.cancelReason != ""},
		})
		if err != nil {
			return err
		}
		return s.event(ctx, tx.Q, updated, typ, row.Status, updated.Status, actorID, in.Note)
	})
	if err != nil {
		return model.Reservation{}, err
	}
	return s.get(ctx, orgID, id)
}

func holdID(row db.Reservation) int64 {
	if !row.HoldID.Valid {
		return 0
	}
	return row.HoldID.Int64
}

// release frees the capacity of the visit; a commitment already gone is
// fine (nothing left to free).
func (s *Service) release(ctx context.Context, tx *availabilityusecase.Tx, row db.Reservation, reason string) error {
	id := holdID(row)
	if id == 0 {
		return nil
	}
	err := s.holds.ReleaseIn(ctx, tx, id, reason)
	if errors.Is(err, availabilityusecase.ErrNotActive) || errors.Is(err, availabilityusecase.ErrHoldExpired) || errors.Is(err, availabilityusecase.ErrHoldNotFound) {
		return nil
	}
	return err
}

// Accept accepts a request. Paid on site: the hold becomes a booking and
// the reservation is active. Online payment: the hold now waits for the
// payment (payment hold time) and the record stays requested.
func (s *Service) Accept(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, actorID int64, in ChangeInput) (model.Reservation, error) {
	return s.transition(ctx, orgID, id, caller, actorID, in, "accepted", func(tx *availabilityusecase.Tx, row db.Reservation, next *state) error {
		if row.Status != model.StatusRequested || row.ApprovalStatus != model.ApprovalPending {
			return ErrInvalidTransition
		}
		next.approval = model.ApprovalAccepted
		if row.PaymentStatus == model.PaymentOnlinePending {
			return s.holds.RenewIn(ctx, tx, holdID(row), availabilitymodel.ReasonPayment, nil)
		}
		next.status = model.StatusActive
		return s.holds.ConfirmIn(ctx, tx, holdID(row), RefType, &row.Uuid)
	})
}

// Reject rejects a request and frees its capacity.
func (s *Service) Reject(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, actorID int64, in ChangeInput) (model.Reservation, error) {
	return s.transition(ctx, orgID, id, caller, actorID, in, "rejected", func(tx *availabilityusecase.Tx, row db.Reservation, next *state) error {
		if row.Status != model.StatusRequested || row.ApprovalStatus != model.ApprovalPending {
			return ErrInvalidTransition
		}
		next.status, next.approval, next.payment = model.StatusRejected, model.ApprovalRejected, unpaid(row.PaymentStatus)
		return s.release(ctx, tx, row, "rejected")
	})
}

// Cancel cancels a request or an active booking and frees its capacity. A
// visit in service or finished cannot be cancelled.
func (s *Service) Cancel(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, actorID int64, in ChangeInput) (model.Reservation, error) {
	reason := in.Reason
	if reason == "" {
		reason = "cancelled"
	}
	if len(reason) > 32 {
		ve := &ValidationError{}
		ve.add("reason", "too_long", "reason is at most 32 characters")
		return model.Reservation{}, ve
	}
	return s.transition(ctx, orgID, id, caller, actorID, in, "cancelled", func(tx *availabilityusecase.Tx, row db.Reservation, next *state) error {
		if row.Status != model.StatusRequested && row.Status != model.StatusActive {
			return ErrInvalidTransition
		}
		next.status, next.payment, next.cancelReason = model.StatusCancelled, unpaid(row.PaymentStatus), reason
		return s.release(ctx, tx, row, "cancelled")
	})
}

// Start marks the customer as being served.
func (s *Service) Start(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, actorID int64, in ChangeInput) (model.Reservation, error) {
	return s.transition(ctx, orgID, id, caller, actorID, in, "started", func(_ *availabilityusecase.Tx, row db.Reservation, next *state) error {
		if row.Status != model.StatusActive {
			return ErrInvalidTransition
		}
		next.status = model.StatusInService
		return nil
	})
}

// CompletionHook runs inside the transaction that completes a visit (the
// sales module writes its sale lines there, TEC-916), so the status and
// the revenue commit together.
type CompletionHook func(ctx context.Context, q *db.Queries, orgID, reservationID, actorID int64) error

// SetCompletionHook installs the completion hook (nil: none).
func (s *Service) SetCompletionHook(fn CompletionHook) {
	s.onComplete = fn
}

// Complete closes the visit; its capacity stays booked (history). The
// services delivered become sale lines through the completion hook.
func (s *Service) Complete(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, actorID int64, in ChangeInput) (model.Reservation, error) {
	return s.transition(ctx, orgID, id, caller, actorID, in, "completed", func(tx *availabilityusecase.Tx, row db.Reservation, next *state) error {
		if row.Status != model.StatusActive && row.Status != model.StatusInService {
			return ErrInvalidTransition
		}
		next.status = model.StatusCompleted
		if s.onComplete != nil {
			return s.onComplete(ctx, tx.Q, orgID, row.ID, actorID)
		}
		return nil
	})
}

// NoShow records that the customer did not come; the time stays booked
// (the staff member kept it free).
func (s *Service) NoShow(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, actorID int64, in ChangeInput) (model.Reservation, error) {
	return s.transition(ctx, orgID, id, caller, actorID, in, "no_show", func(_ *availabilityusecase.Tx, row db.Reservation, next *state) error {
		if row.Status != model.StatusActive {
			return ErrInvalidTransition
		}
		next.status = model.StatusNoShow
		return nil
	})
}

// MarkPaid records a successful online payment (the payments module calls
// it). Payment is asked for once the business accepted, so the record is
// an accepted request: it becomes active and its hold a booking. A payment
// arriving after the hold lapsed answers ErrInvalidTransition (the request
// was cancelled); the payment flow refunds it.
func (s *Service) MarkPaid(ctx context.Context, orgID int64, id uuid.UUID, actorID int64, in ChangeInput) (model.Reservation, error) {
	return s.transition(ctx, orgID, id, availabilityusecase.Caller{}, actorID, in, "paid", func(tx *availabilityusecase.Tx, row db.Reservation, next *state) error {
		if row.PaymentStatus != model.PaymentOnlinePending || row.Status != model.StatusRequested || row.ApprovalStatus != model.ApprovalAccepted {
			return ErrInvalidTransition
		}
		next.payment, next.status = model.PaymentPaid, model.StatusActive
		return s.holds.ConfirmIn(ctx, tx, holdID(row), RefType, &row.Uuid)
	})
}

// ExpireRequests is the expiry hook of the capacity writer: requests whose
// hold lapsed (no decision within the review window, no payment within the
// payment hold time) are cancelled. It runs in the transaction that expires
// the holds, so the capacity and the record change together.
func ExpireRequests(ctx context.Context, q *db.Queries, orgID int64, holdIDs []int64, now time.Time) error {
	return expireRequests(ctx, q, nil, orgID, holdIDs, now)
}

func expireRequests(ctx context.Context, q *db.Queries, n Notifier, orgID int64, holdIDs []int64, _ time.Time) error {
	rows, err := q.ListReservationsByHolds(ctx, db.ListReservationsByHoldsParams{OrganizationID: orgID, HoldIds: holdIDs})
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.Status != model.StatusRequested {
			continue
		}
		reason := model.ReasonPaymentTimeout
		if row.ApprovalStatus == model.ApprovalPending {
			reason = model.ReasonApprovalTimeout
		}
		updated, err := q.UpdateReservationState(ctx, db.UpdateReservationStateParams{
			OrganizationID: orgID, ID: row.ID, Status: model.StatusCancelled, ApprovalStatus: row.ApprovalStatus,
			PaymentStatus: unpaid(row.PaymentStatus), CancelReason: pgtype.Text{String: reason, Valid: true},
		})
		if err != nil {
			return err
		}
		if err := recordEvent(ctx, q, n, updated, "expired", row.Status, updated.Status, 0, reason); err != nil {
			return err
		}
	}
	return nil
}
