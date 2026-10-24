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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
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
		ConfirmationStatus: r.ConfirmationStatus, StartsAt: r.StartsAt.Time, Version: int(r.Version),
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

// closedPayment is the payment state of a visit being cancelled or
// rejected: a payment that never happened ends (unpaid), and a visit for
// which money is still held (collections minus refunds on the ledger, on
// site or online) waits for its refund (TEC-913).
func closedPayment(ctx context.Context, q *db.Queries, row db.Reservation) (string, error) {
	next := unpaid(row.PaymentStatus)
	if next == model.PaymentRefundPending {
		return next, nil
	}
	rows, err := q.LedgerTotals(ctx, db.LedgerTotalsParams{
		OrganizationID: row.OrganizationID, ReservationID: pgtype.Int8{Int64: row.ID, Valid: true},
	})
	if err != nil {
		return "", err
	}
	for _, r := range rows {
		if r.Currency != row.Currency {
			continue
		}
		held := money.FromNumericOrZero(r.Collected).Sub(money.FromNumericOrZero(r.Refunded))
		if held.IsPositive() {
			return model.PaymentRefundPending, nil
		}
	}
	return next, nil
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
		payment, err := closedPayment(ctx, tx.Q, row)
		if err != nil {
			return err
		}
		next.status, next.approval, next.payment = model.StatusRejected, model.ApprovalRejected, payment
		if err := releaseProducts(ctx, tx.Q, orgID, row.ID, "rejected"); err != nil {
			return err
		}
		if err := releaseSessions(ctx, tx.Q, orgID, row.ID, "rejected"); err != nil {
			return err
		}
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
		payment, err := closedPayment(ctx, tx.Q, row)
		if err != nil {
			return err
		}
		next.status, next.payment, next.cancelReason = model.StatusCancelled, payment, reason
		if err := releaseProducts(ctx, tx.Q, orgID, row.ID, "cancelled"); err != nil {
			return err
		}
		if err := releaseSessions(ctx, tx.Q, orgID, row.ID, "cancelled"); err != nil {
			return err
		}
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
		// The products held for the visit are handed over now (TEC-919);
		// the completion hook turns them into product sale lines.
		if err := deliverProducts(ctx, tx.Q, orgID, row, actorID, tx.Now); err != nil {
			return err
		}
		// Package sessions are used now (TEC-920); the hook writes their
		// lines at the session's value.
		if err := consumeSessions(ctx, tx.Q, orgID, row.ID, tx.Now); err != nil {
			return err
		}
		if s.onComplete != nil {
			return s.onComplete(ctx, tx.Q, orgID, row.ID, actorID)
		}
		return nil
	})
}

// NoShow records that the customer did not come; the time stays booked
// (the staff member kept it free). Products not handed over go back to
// the available stock; payments and refunds are handled apart.
func (s *Service) NoShow(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, actorID int64, in ChangeInput) (model.Reservation, error) {
	return s.transition(ctx, orgID, id, caller, actorID, in, "no_show", func(tx *availabilityusecase.Tx, row db.Reservation, next *state) error {
		if row.Status != model.StatusActive {
			return ErrInvalidTransition
		}
		next.status = model.StatusNoShow
		if err := releaseSessions(ctx, tx.Q, orgID, row.ID, "no_show"); err != nil {
			return err
		}
		return releaseProducts(ctx, tx.Q, orgID, row.ID, "no_show")
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
		payment, err := closedPayment(ctx, q, row)
		if err != nil {
			return err
		}
		updated, err := q.UpdateReservationState(ctx, db.UpdateReservationStateParams{
			OrganizationID: orgID, ID: row.ID, Status: model.StatusCancelled, ApprovalStatus: row.ApprovalStatus,
			PaymentStatus: payment, CancelReason: pgtype.Text{String: reason, Valid: true},
		})
		if err != nil {
			return err
		}
		if err := releaseProducts(ctx, q, orgID, row.ID, "expired"); err != nil {
			return err
		}
		if err := releaseSessions(ctx, q, orgID, row.ID, "expired"); err != nil {
			return err
		}
		if err := recordEvent(ctx, q, n, updated, "expired", row.Status, updated.Status, 0, reason); err != nil {
			return err
		}
	}
	return nil
}

// PaymentPost writes the money of a payment inside the reservation's
// transaction (the payments module posts a ledger collection there) and
// returns what is paid for the visit in total after it.
type PaymentPost func(ctx context.Context, q *db.Queries, row db.Reservation) (money.Money, error)

// What RecordPayment did with the reservation.
const (
	// PaymentConfirmed: the online payment of an accepted request made it
	// a booking (active, paid).
	PaymentConfirmed = "confirmed"
	// PaymentRecorded: the money was recorded; the state changed only when
	// the visit is now paid in full.
	PaymentRecorded = "recorded"
	// PaymentLate: an online payment for a visit cancelled or rejected
	// before it arrived. The visit is not reopened (its time may be taken
	// again); the payment waits for its refund (refund_pending).
	PaymentLate = "late"
)

// OnlinePaymentOutcome is what an online payment does to the locked
// record (see RecordPayment).
func OnlinePaymentOutcome(row db.Reservation) string {
	switch {
	case row.Status == model.StatusRequested && row.ApprovalStatus == model.ApprovalAccepted &&
		row.PaymentStatus == model.PaymentOnlinePending:
		return PaymentConfirmed
	case row.Status == model.StatusCancelled || row.Status == model.StatusRejected:
		return PaymentLate
	}
	return PaymentRecorded
}

// RecordPayment applies money taken for a visit (TEC-912) in one capacity
// transaction with the posting itself: lapsed holds are expired first, the
// record is locked, post writes the money, then the state follows.
//
//   - online, an accepted request waiting for its payment: paid and
//     active, its hold becomes a booking (PaymentConfirmed);
//   - online, a cancelled or rejected visit: refund_pending, the visit is
//     not reopened (PaymentLate);
//   - otherwise the money is recorded, and a visit paid in full is paid.
//
// Money at the business is refused for a cancelled or rejected visit
// (ErrInvalidTransition) before anything is written. A payment never
// confirms the customer's attendance.
func (s *Service) RecordPayment(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, actorID int64,
	online bool, note string, post PaymentPost,
) (model.Reservation, string, error) {
	outcome := ""
	err := s.holds.Run(ctx, orgID, func(tx *availabilityusecase.Tx) error {
		row, err := s.visible(ctx, tx.Q, orgID, id, caller, true)
		if err != nil {
			return err
		}
		closed := row.Status == model.StatusCancelled || row.Status == model.StatusRejected
		if closed && !online {
			return ErrInvalidTransition
		}
		paid, err := post(ctx, tx.Q, row)
		if err != nil {
			return err
		}
		outcome = PaymentRecorded
		if online {
			outcome = OnlinePaymentOutcome(row)
		}
		next, typ := stateOf(row), "payment_recorded"
		switch outcome {
		case PaymentConfirmed:
			next.payment, next.status, typ = model.PaymentPaid, model.StatusActive, "paid"
			if err := s.holds.ConfirmIn(ctx, tx, holdID(row), RefType, &row.Uuid); err != nil {
				return err
			}
		case PaymentLate:
			next.payment, typ = model.PaymentRefundPending, "late_payment"
		default:
			total := money.FromNumericOrZero(row.TotalPrice)
			switch row.PaymentStatus {
			case model.PaymentOnSite, model.PaymentOnlinePending, model.PaymentFailed:
				if paid.Minor() >= total.Minor() {
					next.payment = model.PaymentPaid
				}
			}
		}
		updated, err := tx.Q.UpdateReservationState(ctx, db.UpdateReservationStateParams{
			OrganizationID: orgID, ID: row.ID, Status: next.status, ApprovalStatus: next.approval,
			PaymentStatus: next.payment, CancelReason: pgtype.Text{String: next.cancelReason, Valid: next.cancelReason != ""},
		})
		if err != nil {
			return err
		}
		return s.event(ctx, tx.Q, updated, typ, row.Status, updated.Status, actorID, note)
	})
	if err != nil {
		return model.Reservation{}, "", err
	}
	out, err := s.get(ctx, orgID, id)
	return out, outcome, err
}

// What RecordRefund did with the payment state.
const (
	// RefundAll: nothing is paid for the visit any more (refunded).
	RefundAll = "refunded"
	// RefundPart: part of the money was paid back (partially_refunded).
	RefundPart = "partially_refunded"
	// RefundKept: an open visit paid more than its total got the extra
	// back; the payment state stays.
	RefundKept = "kept"
)

// RecordRefund applies money paid back for a visit (TEC-913) in one
// transaction with the posting itself: the record is locked, post writes
// the refund (and returns what is still paid for the visit after it), then
// the payment state follows:
//
//   - nothing paid any more: refunded;
//   - a cancelled or rejected visit, a refund waiting or one done before,
//     or an open visit now paid less than its total: partially_refunded;
//   - otherwise (an overpayment paid back) the state stays.
//
// The visit itself never changes: a refund does not reopen or cancel it.
func (s *Service) RecordRefund(ctx context.Context, orgID int64, id uuid.UUID, actorID int64, note string, post PaymentPost) (model.Reservation, string, error) {
	outcome := ""
	err := s.holds.Run(ctx, orgID, func(tx *availabilityusecase.Tx) error {
		row, err := s.visible(ctx, tx.Q, orgID, id, availabilityusecase.Caller{}, true)
		if err != nil {
			return err
		}
		paid, err := post(ctx, tx.Q, row)
		if err != nil {
			return err
		}
		next := stateOf(row)
		closed := row.Status == model.StatusCancelled || row.Status == model.StatusRejected
		total := money.FromNumericOrZero(row.TotalPrice)
		switch {
		case !paid.IsPositive():
			next.payment, outcome = model.PaymentRefunded, RefundAll
		case closed, row.PaymentStatus == model.PaymentRefundPending, row.PaymentStatus == model.PaymentRefunded,
			row.PaymentStatus == model.PaymentPartiallyRefunded,
			row.PaymentStatus == model.PaymentPaid && paid.Cmp(total) < 0:
			next.payment, outcome = model.PaymentPartiallyRefunded, RefundPart
		default:
			outcome = RefundKept
		}
		updated, err := tx.Q.UpdateReservationState(ctx, db.UpdateReservationStateParams{
			OrganizationID: orgID, ID: row.ID, Status: next.status, ApprovalStatus: next.approval,
			PaymentStatus: next.payment, CancelReason: pgtype.Text{String: next.cancelReason, Valid: next.cancelReason != ""},
		})
		if err != nil {
			return err
		}
		return s.event(ctx, tx.Q, updated, "refund_recorded", row.Status, updated.Status, actorID, note)
	})
	if err != nil {
		return model.Reservation{}, "", err
	}
	out, err := s.get(ctx, orgID, id)
	return out, outcome, err
}
