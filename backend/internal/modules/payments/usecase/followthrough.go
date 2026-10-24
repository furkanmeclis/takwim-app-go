package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Refund follow-through (TEC-978, docs/design/payments.md "Refunds"):
//
//   - Close without a refund: a refund to make (a paid visit cancelled, a
//     late online payment, a last refund that failed) is closed with a
//     reason when the money stays with the business (the customer left it,
//     or it counts for the next visit). The payment state becomes kept and
//     the reason goes into the visit's history; the money stays on the
//     customer's ledger as a credit and may still be paid back later. A
//     refund still under way is settled first.
//   - Customer notices: a refund paid back sends "refund done"; one the
//     provider refused or whose result is unknown sends "refund being
//     processed" (once per refund each, through the customer notification
//     engine and the business's rule for the event).
//   - Status queries: a provider refund whose result is unknown is asked
//     about again by the worker through the provider's refund status query
//     (payments.RefundChecker), never sent again. Found posts it; not done
//     ends it as failed; anything else asks again later, further apart. A
//     provider without a status query leaves the refund to a person
//     (resolve), as before.

var (
	// ErrNothingToClose: the visit has no refund to make (nothing is held,
	// or no refund is waiting or failed).
	ErrNothingToClose = errors.New("the visit has no refund to close")
	// ErrRefundOpen: a refund of the visit is still under way; settle it
	// first.
	ErrRefundOpen = errors.New("a refund of the visit is still under way")
)

// Refund notices.
const (
	NoticeRefundCompleted  = "completed"
	NoticeRefundProcessing = "processing"
)

// RefundNotifier writes a refund notice for the customer of a reservation
// (once per refund and notice).
type RefundNotifier func(ctx context.Context, orgID, reservationID int64, refund uuid.UUID, notice string) error

// SetRefundNotifier installs the refund notices (nil: none are sent).
func (s *Service) SetRefundNotifier(fn RefundNotifier) {
	s.notify = fn
}

// noticeRefund sends a refund notice; a failure is logged, never returned
// (the refund itself is done).
func (s *Service) noticeRefund(ctx context.Context, orgID int64, reservation, refund uuid.UUID, notice string) {
	if s.notify == nil {
		return
	}
	row, err := s.q.GetReservationRecord(ctx, db.GetReservationRecordParams{OrganizationID: orgID, Uuid: reservation})
	if err == nil {
		err = s.notify(ctx, orgID, row.ID, refund, notice)
	}
	if err != nil {
		s.log.Warn("refund_notice_failed", "organization_id", orgID, "refund", refund, "notice", notice, "error", err)
	}
}

// closable tells whether the visit's money may be kept (nil) or why not,
// inside q's transaction.
func (s *Service) closable(ctx context.Context, q *db.Queries, orgID int64, row db.Reservation) error {
	refunds, err := q.ListReservationRefunds(ctx, db.ListReservationRefundsParams{OrganizationID: orgID, ReservationID: row.ID})
	if err != nil {
		return err
	}
	for _, f := range refunds {
		if f.Status == model.RefundRequested {
			return ErrRefundOpen
		}
	}
	cols, err := s.refundCollections(ctx, q, orgID, row.ID)
	if err != nil {
		return err
	}
	held := money.Zero()
	for _, c := range cols {
		held = held.Add(refundable(c))
	}
	if !held.IsPositive() {
		return ErrNothingToClose
	}
	if row.PaymentStatus == reservationsmodel.PaymentRefundPending {
		return nil
	}
	// The newest refund failed: closable unless it was closed after it.
	if len(refunds) > 0 && refunds[0].Status == model.RefundFailed {
		if row.PaymentStatus != reservationsmodel.PaymentKept {
			return nil
		}
		kept, err := q.LastPaymentKept(ctx, db.LastPaymentKeptParams{OrganizationID: orgID, ReservationID: row.ID})
		if isNoRows(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if refunds[0].CreatedAt.Time.After(kept.CreatedAt.Time) {
			return nil
		}
	}
	return ErrNothingToClose
}

// keptView fills what the payment view shows of closing without a refund.
func (s *Service) keptView(ctx context.Context, orgID int64, row db.Reservation, out *model.ReservationPayments, allowed bool) error {
	if row.PaymentStatus == reservationsmodel.PaymentKept {
		k, err := s.q.LastPaymentKept(ctx, db.LastPaymentKeptParams{OrganizationID: orgID, ReservationID: row.ID})
		switch {
		case err == nil:
			out.Kept = &model.Kept{At: k.CreatedAt.Time, Reason: k.Note}
			if k.ActorUuid.Valid {
				out.Kept.By = &model.Ref{
					UUID: uuid.UUID(k.ActorUuid.Bytes), Name: strings.TrimSpace(k.ActorName.String + " " + k.ActorSurname.String),
				}
			}
		case !isNoRows(err):
			return err
		}
	}
	if !allowed {
		return nil
	}
	err := s.closable(ctx, s.q, orgID, row)
	switch {
	case err == nil:
		out.CanClose = true
	case errors.Is(err, ErrNothingToClose), errors.Is(err, ErrRefundOpen):
	default:
		return err
	}
	return nil
}

// CloseWithoutRefund closes the refund to make of a visit the caller may
// see without paying the money back (see the rules above).
func (s *Service) CloseWithoutRefund(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, actorID int64,
	opts ViewOptions, in model.CloseInput,
) (model.ReservationPayments, error) {
	if _, err := s.ReservationPayments(ctx, orgID, id, caller, ViewOptions{}); err != nil {
		return model.ReservationPayments{}, err
	}
	reason := clip(in.Reason, maxReasonLength)
	if reason == "" {
		return model.ReservationPayments{}, invalid("reason", "required", "reason is required")
	}
	_, err := s.reservations.KeepPayment(ctx, orgID, id, actorID, reason, func(ctx context.Context, q *db.Queries, row db.Reservation) error {
		return s.closable(ctx, q, orgID, row)
	})
	if err != nil {
		return model.ReservationPayments{}, mapRefundErr(err)
	}
	s.changed(ctx, orgID, id)
	return s.ReservationPayments(ctx, orgID, id, caller, opts)
}

// Refund status queries.
const (
	// refundCheckBatch bounds the refunds one sweep asks about.
	refundCheckBatch = 50
	// refundCheckLease keeps a refund off other workers while it is asked.
	refundCheckLease = 2 * time.Minute
	// DefaultRefundCheckInterval is how often the worker sweeps.
	DefaultRefundCheckInterval = time.Minute
)

// refundCheckDelays are the waits before the next status query after the
// first, second, ... query; the last one repeats.
var refundCheckDelays = []time.Duration{5 * time.Minute, 15 * time.Minute, time.Hour, 6 * time.Hour, 24 * time.Hour}

func refundCheckDelay(checks int32) time.Duration {
	i := int(checks) - 1
	if i < 0 {
		i = 0
	}
	if i >= len(refundCheckDelays) {
		i = len(refundCheckDelays) - 1
	}
	return refundCheckDelays[i]
}

// CheckUncertainRefunds asks the providers about the refunds whose result
// is unknown and whose next query is due, every business; it returns how
// many refunds were asked about.
func (s *Service) CheckUncertainRefunds(ctx context.Context) (int, error) {
	now := s.now().UTC()
	due, err := s.q.ListUncertainRefundsDue(ctx, db.ListUncertainRefundsDueParams{
		NowAt: pgtype.Timestamptz{Time: now, Valid: true}, LimitCount: refundCheckBatch,
	})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, d := range due {
		asked, err := s.checkRefund(ctx, d.OrganizationID, d.ID, now)
		if err != nil {
			s.log.Error("refund_check_failed", "organization_id", d.OrganizationID, "refund_id", d.ID, "error", err)
		}
		if asked {
			n++
		}
	}
	return n, nil
}

// RunRefundChecks sweeps every interval until ctx ends (the worker).
func (s *Service) RunRefundChecks(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultRefundCheckInterval
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if _, err := s.CheckUncertainRefunds(ctx); err != nil && ctx.Err() == nil {
			s.log.Error("refund_checks_failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// checkRefund asks the provider about one refund (asked is false when the
// refund was not due any more or nothing could be asked).
func (s *Service) checkRefund(ctx context.Context, orgID, id int64, now time.Time) (bool, error) {
	f, err := s.q.ClaimRefundCheck(ctx, db.ClaimRefundCheckParams{
		OrganizationID: orgID, ID: id, NowAt: pgtype.Timestamptz{Time: now, Valid: true},
		LeaseUntil: pgtype.Timestamptz{Time: now.Add(refundCheckLease), Valid: true},
	})
	if isNoRows(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	later := func() error {
		return s.q.ScheduleRefundCheck(ctx, db.ScheduleRefundCheckParams{
			OrganizationID: orgID, ID: f.ID, NextCheckAt: pgtype.Timestamptz{Time: now.Add(refundCheckDelay(f.ProviderChecks)), Valid: true},
		})
	}
	var checker payments.RefundChecker
	if p, ok := s.provider(f.Provider); ok {
		checker, _ = payments.RefundCheckerOf(p)
	}
	if checker == nil || !f.IntegrationAccountID.Valid || !f.PaymentID.Valid {
		// Nothing to ask (no status query, the account is gone): a person
		// settles it; looked at again much later.
		return false, s.q.ScheduleRefundCheck(ctx, db.ScheduleRefundCheckParams{
			OrganizationID: orgID, ID: f.ID, NextCheckAt: pgtype.Timestamptz{Time: now.Add(refundCheckDelays[len(refundCheckDelays)-1]), Valid: true},
		})
	}
	account, err := s.accounts.AccountByID(ctx, orgID, f.IntegrationAccountID.Int64)
	if err != nil {
		return false, errors.Join(err, later())
	}
	target, err := s.q.GetRefundCheckTarget(ctx, db.GetRefundCheckTargetParams{OrganizationID: orgID, ID: f.ID})
	if err != nil {
		return false, errors.Join(err, later())
	}
	res, err := checker.CheckRefund(ctx, account, payments.RefundRequest{
		OrderRef: target.OrderRef, ProviderPaymentID: target.ProviderPaymentID, Amount: amountOf(f.Amount),
		Currency: f.Currency, RefundRef: strings.ReplaceAll(f.Uuid.String(), "-", ""),
	})
	if err != nil {
		s.log.Warn("refund_check_unknown", "organization_id", orgID, "refund", f.Uuid, "checks", f.ProviderChecks, "error", err)
		return true, later()
	}
	switch res.Status {
	case payments.RefundFound:
		n, err := s.q.SetRefundProviderAnswer(ctx, db.SetRefundProviderAnswerParams{
			OrganizationID: orgID, ID: f.ID, ProviderStatus: model.ProviderSucceeded, ProviderRefundID: clip(res.ProviderRefundID, 200),
		})
		if err != nil || n == 0 {
			return true, err
		}
		if err := s.postProviderRefund(ctx, orgID, target.ReservationUuid, f, nil); err != nil && !errors.Is(err, errRefundApplied) {
			// Confirmed but not posted: it waits for a person (resolve).
			s.changed(ctx, orgID, target.ReservationUuid)
			return true, mapRefundErr(err)
		}
	case payments.RefundNotDone:
		reason := "not refunded at the provider (automatic check)"
		if msg := strings.TrimSpace(res.Message); msg != "" {
			reason += ": " + msg
		}
		if _, err := s.q.ResolveRefundFailed(ctx, db.ResolveRefundFailedParams{
			OrganizationID: orgID, ID: f.ID, FailureReason: clip(reason, 500),
		}); err != nil {
			return true, err
		}
	default:
		return true, later()
	}
	s.changed(ctx, orgID, target.ReservationUuid)
	return true, nil
}
