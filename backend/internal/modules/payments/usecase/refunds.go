package usecase

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	salesmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Refund rules (TEC-913, docs/design/payments.md "Refunds"):
//
//   - A refund pays back (part of) one live collection of a visit, never
//     more than what is left of it: the collection less its refunds and
//     the refunds under way (checked under a lock of the collection).
//   - The request, the provider's answer and the posting to the books are
//     kept apart (refunds row). A manual refund (money handed back, or a
//     refund made in the provider's own panel) is all three at once. A
//     provider refund is asked from the provider after the request is
//     committed; only its confirmation posts the refund ledger entry.
//   - A provider answer that is not known (a timeout, a 5xx) is never sent
//     again: the refund stays requested and appears on the follow-up list
//     until a person settles it after checking the provider's panel.
//   - The same idempotency key answers the same refund and sends nothing.
//   - After the posting the visit's payment state follows
//     (reservations.RecordRefund): refunded or partially_refunded. A refund
//     never reopens or cancels the visit.

var (
	// ErrIdempotencyReused: the key names another refund request.
	ErrIdempotencyReused = errors.New("the idempotency key was used for another refund")
	// ErrRefundNotOpen: the refund is settled already.
	ErrRefundNotOpen = errors.New("the refund is settled already")

	errRefundApplied = errors.New("refund already posted")
)

const (
	maxReasonLength = 240
	maxKeyLength    = 100
)

var keyRE = regexp.MustCompile(`^[A-Za-z0-9:_\-.]+$`)

// refundable is what is left of a collection to pay back.
func refundable(c db.ListRefundableCollectionsRow) money.Money {
	return amountOf(c.Amount).Sub(amountOf(c.Refunded)).Sub(amountOf(c.Pending)).ClampZero()
}

// providerRefunder is the refund call that can pay back a collection's
// online payment (nil when none: no payment, an unknown provider or one
// without a refund API).
func (s *Service) providerRefunder(c db.ListRefundableCollectionsRow) payments.Refunder {
	if !c.PaymentID.Valid || !c.PaymentProvider.Valid || !c.PaymentAccountID.Valid {
		return nil
	}
	p, ok := s.provider(c.PaymentProvider.String)
	if !ok {
		return nil
	}
	r, ok := payments.RefunderOf(p)
	if !ok {
		return nil
	}
	return r
}

func (s *Service) refundCollections(ctx context.Context, q *db.Queries, orgID, reservationID int64) ([]db.ListRefundableCollectionsRow, error) {
	return q.ListRefundableCollections(ctx, db.ListRefundableCollectionsParams{
		OrganizationID: orgID, ReservationID: pgtype.Int8{Int64: reservationID, Valid: true},
	})
}

// refundView fills the refund part of the payment view.
func (s *Service) refundView(ctx context.Context, orgID int64, row db.Reservation, out *model.ReservationPayments, allowed bool) error {
	out.Refundable = money.Zero()
	out.Refunds = []model.Refund{}
	out.RefundCollections = []model.RefundCollection{}
	out.RefundAccounts = []model.CollectAccount{}
	refunds, err := s.q.ListReservationRefunds(ctx, db.ListReservationRefundsParams{OrganizationID: orgID, ReservationID: row.ID})
	if err != nil {
		return err
	}
	for _, f := range refunds {
		r := mapRefund(f)
		r.AutoCheck = r.Status == model.RefundRequested && r.ProviderStatus == model.ProviderUncertain && s.canCheck(r.Provider)
		out.Refunds = append(out.Refunds, r)
	}
	if err := s.keptView(ctx, orgID, row, out, allowed); err != nil {
		return err
	}
	cols, err := s.refundCollections(ctx, s.q, orgID, row.ID)
	if err != nil {
		return err
	}
	for _, c := range cols {
		left := refundable(c)
		out.Refundable = out.Refundable.Add(left)
		rc := model.RefundCollection{
			UUID: c.Uuid, Method: c.Method, EntryDate: c.EntryDate.Time.Format("2006-01-02"), Amount: amountOf(c.Amount),
			Refunded: amountOf(c.Refunded), Pending: amountOf(c.Pending), Refundable: left,
			CashAccount: model.Ref{UUID: c.CashAccountUuid, Name: c.CashAccountName}, ProviderRefund: s.providerRefunder(c) != nil,
		}
		if c.PaymentProvider.Valid {
			rc.Provider = c.PaymentProvider.String
		}
		out.RefundCollections = append(out.RefundCollections, rc)
	}
	if !allowed {
		out.RefundCollections = []model.RefundCollection{}
		return nil
	}
	accounts, err := s.q.ListRefundAccounts(ctx, db.ListRefundAccountsParams{
		OrganizationID: orgID, Currency: out.Currency, BranchID: pgtype.Int8{Int64: row.BranchID, Valid: true},
	})
	if err != nil {
		return err
	}
	for _, a := range accounts {
		out.RefundAccounts = append(out.RefundAccounts, model.CollectAccount{
			UUID: a.Uuid, Name: a.Name, Kind: a.Kind, Currency: a.Currency, Branch: a.BranchID.Valid,
		})
	}
	out.CanRefund = out.Refundable.IsPositive()
	return nil
}

// canCheck reports whether a provider can be asked what happened to a
// refund (its refund status query, TEC-978).
func (s *Service) canCheck(provider string) bool {
	p, ok := s.provider(provider)
	if !ok {
		return false
	}
	_, ok = payments.RefundCheckerOf(p)
	return ok
}

func mapRefund(f db.ListReservationRefundsRow) model.Refund {
	out := model.Refund{
		UUID: f.Uuid, Channel: f.Channel, Provider: f.Provider, Method: f.Method, Amount: amountOf(f.Amount), Currency: f.Currency,
		Status: f.Status, ProviderStatus: f.ProviderStatus, ProviderRefundID: f.ProviderRefundID, FailureReason: f.FailureReason,
		Reason: f.Reason, ResolutionNote: f.ResolutionNote, CollectionUUID: f.CollectionUuid,
		CashAccount: model.Ref{UUID: f.CashAccountUuid, Name: f.CashAccountName},
		ProviderAt:  timePtr(f.ProviderAnsweredAt), ReflectedAt: timePtr(f.ReflectedAt), CreatedAt: f.CreatedAt.Time,
		NeedsAction:    f.Status == model.RefundRequested && f.ProviderStatus != "",
		ProviderChecks: f.ProviderChecks, ProviderCheckedAt: timePtr(f.ProviderCheckedAt), NextCheckAt: timePtr(f.NextCheckAt),
	}
	if f.LedgerEntryUuid.Valid {
		v := uuid.UUID(f.LedgerEntryUuid.Bytes)
		out.LedgerEntryUUID = &v
	}
	if f.RequestedByUuid.Valid {
		name := strings.TrimSpace(f.RequestedByName.String + " " + f.RequestedBySurname.String)
		out.RequestedBy = &model.Ref{UUID: uuid.UUID(f.RequestedByUuid.Bytes), Name: name}
	}
	return out
}

// RefundResult is what a refund request answered: the refund and the
// visit's payment view after it.
type RefundResult struct {
	Refund   model.Refund
	Payments model.ReservationPayments
	// Replayed: the idempotency key named an earlier request.
	Replayed bool
}

// Refund pays back (part of) a collection of a visit the caller may see
// (see the rules above).
func (s *Service) Refund(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, actorID int64, opts ViewOptions, in model.RefundInput) (RefundResult, error) {
	if _, err := s.ReservationPayments(ctx, orgID, id, caller, ViewOptions{}); err != nil {
		return RefundResult{}, err
	}
	row, err := s.q.GetReservationRecord(ctx, db.GetReservationRecordParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return RefundResult{}, err
	}
	key := strings.TrimSpace(in.IdempotencyKey)
	ve := &ValidationError{}
	if key == "" {
		ve.add("idempotency_key", "required", "idempotency_key is required")
	} else if len(key) > maxKeyLength || !keyRE.MatchString(key) {
		ve.add("idempotency_key", "invalid", "idempotency_key must be at most 100 letters, digits or :_-.")
	}
	if len(ve.Details) == 0 {
		prev, err := s.q.GetRefundByIdempotencyKey(ctx, db.GetRefundByIdempotencyKeyParams{OrganizationID: orgID, IdempotencyKey: key})
		switch {
		case err == nil:
			return s.replay(ctx, orgID, id, caller, actorID, opts, prev, in)
		case !isNoRows(err):
			return RefundResult{}, err
		}
	}
	if !in.Amount.IsPositive() {
		ve.add("amount", "invalid", "amount must be greater than zero")
	}
	if !slices.Contains(model.RefundChannels, in.Channel) {
		ve.add("channel", "invalid", "channel must be provider or manual")
	}
	reason := clip(in.Reason, maxReasonLength)
	cols, err := s.refundCollections(ctx, s.q, orgID, row.ID)
	if err != nil {
		return RefundResult{}, err
	}
	var col *db.ListRefundableCollectionsRow
	for i := range cols {
		if cols[i].Uuid == in.CollectionUUID {
			col = &cols[i]
		}
	}
	if col == nil {
		ve.add("collection_uuid", "not_found", "collection not found")
	} else if in.Amount.IsPositive() && in.Amount.Cmp(refundable(*col)) > 0 {
		ve.add("amount", "exceeds_refundable", "amount is more than what is left of the collection")
	}
	if len(ve.Details) > 0 {
		return RefundResult{}, ve
	}

	user := actorID
	insert := db.InsertRefundParams{
		OrganizationID: orgID, BranchID: row.BranchID, ReservationID: row.ID, CollectionEntryID: col.ID,
		Channel: in.Channel, Amount: in.Amount.Numeric(), Currency: col.Currency, Reason: reason, IdempotencyKey: key,
		RequestedBy: pgtype.Int8{Int64: actorID, Valid: actorID > 0},
	}
	var account integrations.Account
	var refunder payments.Refunder
	if in.Channel == model.ChannelProvider {
		refunder = s.providerRefunder(*col)
		if refunder == nil {
			return RefundResult{}, invalid("channel", "unsupported", "the payment's provider cannot refund it; record a manual refund")
		}
		account, err = s.accounts.AccountByID(ctx, orgID, col.PaymentAccountID.Int64)
		if err != nil {
			return RefundResult{}, invalid("channel", "unsupported", "the payment's provider account is not connected any more")
		}
		insert.PaymentID, insert.IntegrationAccountID = col.PaymentID, col.PaymentAccountID
		insert.Provider, insert.Method, insert.CashAccountID = col.PaymentProvider.String, salesmodel.MethodOnline, col.CashAccountID
	} else {
		if !slices.Contains(model.RefundMethods, in.Method) {
			ve.add("method", "invalid", "method must be one of cash, card, transfer, online")
		}
		if in.CashAccountUUID == nil || *in.CashAccountUUID == uuid.Nil {
			ve.add("cash_account_uuid", "required", "cash_account_uuid is required")
		}
		if len(ve.Details) > 0 {
			return RefundResult{}, ve
		}
		acc, err := s.q.GetRefundAccount(ctx, db.GetRefundAccountParams{OrganizationID: orgID, Uuid: *in.CashAccountUUID})
		if isNoRows(err) || (err == nil && acc.BranchID.Valid && acc.BranchID.Int64 != row.BranchID) {
			return RefundResult{}, invalid("cash_account_uuid", "not_found", "cash account not found")
		}
		if err != nil {
			return RefundResult{}, err
		}
		if acc.Status != "active" || (acc.Kind != "cash" && acc.Kind != "bank" && acc.Kind != "online") {
			return RefundResult{}, invalid("cash_account_uuid", "invalid", "the account must be an active till, bank or online account")
		}
		if acc.Currency != col.Currency {
			return RefundResult{}, invalid("cash_account_uuid", "currency", "the account has another currency")
		}
		insert.Method, insert.CashAccountID = in.Method, acc.ID
	}

	if in.Channel == model.ChannelManual {
		// Request, answer and posting at once, in the visit's transaction.
		var refundUUID uuid.UUID
		_, _, err := s.reservations.RecordRefund(ctx, orgID, id, actorID, refundNote(in.Amount, col.Currency, in.Method),
			func(ctx context.Context, q *db.Queries, r db.Reservation) (money.Money, error) {
				f, err := s.insertRefund(ctx, q, orgID, insert, in.Amount)
				if err != nil {
					return money.Zero(), err
				}
				refundUUID = f.Uuid
				if err := s.postRefund(ctx, q, orgID, f, &user); err != nil {
					return money.Zero(), err
				}
				return paidOf(ctx, q, orgID, r.ID, r.Currency)
			})
		if err != nil {
			return RefundResult{}, mapRefundErr(err)
		}
		s.changed(ctx, orgID, id)
		s.noticeRefund(ctx, orgID, id, refundUUID, NoticeRefundCompleted)
		return s.result(ctx, orgID, id, caller, opts, refundUUID, false)
	}

	// Provider: commit the request first (the provider is asked once).
	var f db.Refund
	err = s.sales.InTx(ctx, func(q *db.Queries) error {
		var err error
		f, err = s.insertRefund(ctx, q, orgID, insert, in.Amount)
		return err
	})
	if err != nil {
		return RefundResult{}, mapRefundErr(err)
	}
	s.askProvider(ctx, orgID, id, refunder, account, f, col, &user)
	s.changed(ctx, orgID, id)
	return s.result(ctx, orgID, id, caller, opts, f.Uuid, false)
}

// insertRefund writes the request after locking the collection and
// checking, under the lock, that the amount is still left of it.
func (s *Service) insertRefund(ctx context.Context, q *db.Queries, orgID int64, in db.InsertRefundParams, amount money.Money) (db.Refund, error) {
	if _, err := q.LockLedgerEntry(ctx, db.LockLedgerEntryParams{OrganizationID: orgID, ID: in.CollectionEntryID}); err != nil {
		return db.Refund{}, err
	}
	cols, err := s.refundCollections(ctx, q, orgID, in.ReservationID)
	if err != nil {
		return db.Refund{}, err
	}
	for _, c := range cols {
		if c.ID != in.CollectionEntryID {
			continue
		}
		if amount.Cmp(refundable(c)) > 0 {
			return db.Refund{}, invalid("amount", "exceeds_refundable", "amount is more than what is left of the collection")
		}
		return q.InsertRefund(ctx, in)
	}
	return db.Refund{}, invalid("collection_uuid", "not_found", "collection not found")
}

// postRefund writes the refund ledger entry (and its cash movement out of
// the account) of a refund and marks it done, inside q's transaction.
func (s *Service) postRefund(ctx context.Context, q *db.Queries, orgID int64, f db.Refund, userID *int64) error {
	locked, err := q.LockRefund(ctx, db.LockRefundParams{OrganizationID: orgID, ID: f.ID})
	if err != nil {
		return err
	}
	if locked.Status != model.RefundRequested {
		return errRefundApplied
	}
	col, err := q.GetLedgerEntryRowByID(ctx, db.GetLedgerEntryRowByIDParams{OrganizationID: orgID, ID: locked.CollectionEntryID})
	if err != nil {
		return err
	}
	acc, err := q.GetCashAccountUUIDByID(ctx, db.GetCashAccountUUIDByIDParams{OrganizationID: orgID, ID: locked.CashAccountID})
	if err != nil {
		return err
	}
	desc := locked.Reason
	if desc == "" {
		desc = "refund"
		if locked.Channel == model.ChannelProvider {
			desc = "online refund (" + locked.Provider + ")"
		}
	}
	refUUID := locked.Uuid
	entry, err := s.sales.RecordRefundTx(ctx, q, orgID, salesmodel.RefundInput{
		CollectionUUID: col.Uuid, CashAccountUUID: acc, Amount: amountOf(locked.Amount), Method: locked.Method,
		Description: desc, SourceType: model.SourceRefund, SourceUUID: &refUUID, IdempotencyKey: "refund:" + locked.Uuid.String(),
		CreatedBy: userID,
	})
	if err != nil {
		return err
	}
	e, err := q.GetLedgerEntryRow(ctx, db.GetLedgerEntryRowParams{OrganizationID: orgID, Uuid: entry})
	if err != nil {
		return err
	}
	n, err := q.MarkRefundPosted(ctx, db.MarkRefundPostedParams{
		OrganizationID: orgID, ID: locked.ID, LedgerEntryID: pgtype.Int8{Int64: e.ID, Valid: true},
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return errRefundApplied
	}
	return nil
}

// askProvider calls the provider's refund and records its answer; a
// confirmed refund is posted at once (a failed posting is retried by
// resolve). Errors are recorded on the refund, never returned: the
// request is committed and shows its state.
func (s *Service) askProvider(ctx context.Context, orgID int64, resID uuid.UUID, refunder payments.Refunder, account integrations.Account,
	f db.Refund, col *db.ListRefundableCollectionsRow, userID *int64,
) {
	res, err := refunder.Refund(ctx, account, payments.RefundRequest{
		OrderRef: col.PaymentOrderRef.String, ProviderPaymentID: col.PaymentProviderRef.String, Amount: amountOf(f.Amount),
		Currency: f.Currency, RefundRef: strings.ReplaceAll(f.Uuid.String(), "-", ""),
	})
	answer := db.SetRefundProviderAnswerParams{OrganizationID: orgID, ID: f.ID, ProviderStatus: model.ProviderSucceeded}
	var pe *payments.ProviderError
	switch {
	case err == nil:
		answer.ProviderRefundID = clip(res.ProviderRefundID, 200)
	case errors.As(err, &pe) && !pe.Temporary:
		answer.ProviderStatus, answer.FailureReason = model.ProviderFailed, clip(strings.TrimSpace(pe.Code+" "+pe.Message), 500)
	default:
		answer.ProviderStatus, answer.FailureReason = model.ProviderUncertain, clip("unknown result: "+err.Error(), 500)
	}
	if _, err := s.q.SetRefundProviderAnswer(ctx, answer); err != nil {
		s.log.Error("refund_answer_not_saved", "organization_id", orgID, "refund", f.Uuid, "provider_status", answer.ProviderStatus, "error", err)
		return
	}
	if answer.ProviderStatus != model.ProviderSucceeded {
		s.log.Warn("refund_not_done", "organization_id", orgID, "refund", f.Uuid, "provider_status", answer.ProviderStatus, "reason", answer.FailureReason)
		// Refused or unknown: the business follows it (TEC-978).
		s.noticeRefund(ctx, orgID, resID, f.Uuid, NoticeRefundProcessing)
		return
	}
	if err := s.postProviderRefund(ctx, orgID, resID, f, userID); err != nil {
		s.log.Error("refund_not_posted", "organization_id", orgID, "refund", f.Uuid, "error", err)
	}
}

// postProviderRefund posts a refund the provider confirmed, in the visit's
// transaction, and tells the customer.
func (s *Service) postProviderRefund(ctx context.Context, orgID int64, resID uuid.UUID, f db.Refund, userID *int64) error {
	actor := int64(0)
	if userID != nil {
		actor = *userID
	}
	_, _, err := s.reservations.RecordRefund(ctx, orgID, resID, actor, refundNote(amountOf(f.Amount), f.Currency, f.Method),
		func(ctx context.Context, q *db.Queries, r db.Reservation) (money.Money, error) {
			if err := s.postRefund(ctx, q, orgID, f, userID); err != nil {
				return money.Zero(), err
			}
			return paidOf(ctx, q, orgID, r.ID, r.Currency)
		})
	if err == nil {
		s.noticeRefund(ctx, orgID, resID, f.Uuid, NoticeRefundCompleted)
	}
	return err
}

func refundNote(amount money.Money, currency, method string) string {
	return fmt.Sprintf("refund %s %s · %s", amount, currency, method)
}

// replay answers a repeated request (same idempotency key): the same
// refund, or ErrIdempotencyReused when the key named another one. A
// confirmed refund whose posting failed is posted now.
func (s *Service) replay(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, actorID int64,
	opts ViewOptions, prev db.GetRefundByIdempotencyKeyRow, in model.RefundInput,
) (RefundResult, error) {
	col, err := s.q.GetLedgerEntryRowByID(ctx, db.GetLedgerEntryRowByIDParams{OrganizationID: orgID, ID: prev.CollectionEntryID})
	if err != nil {
		return RefundResult{}, err
	}
	if prev.ReservationUuid != id || col.Uuid != in.CollectionUUID || !amountOf(prev.Amount).Equal(in.Amount) || prev.Channel != in.Channel {
		return RefundResult{}, ErrIdempotencyReused
	}
	if prev.Status == model.RefundRequested && prev.ProviderStatus == model.ProviderSucceeded {
		user := actorID
		if err := s.postProviderRefund(ctx, orgID, id, refundOf(prev), &user); err != nil && !errors.Is(err, errRefundApplied) {
			s.log.Error("refund_not_posted", "organization_id", orgID, "refund", prev.Uuid, "error", err)
		}
		s.changed(ctx, orgID, id)
	}
	return s.result(ctx, orgID, id, caller, opts, prev.Uuid, true)
}

func refundOf(r db.GetRefundByIdempotencyKeyRow) db.Refund {
	return db.Refund{ID: r.ID, Uuid: r.Uuid, OrganizationID: r.OrganizationID, Amount: r.Amount, Currency: r.Currency, Method: r.Method}
}

// result reads the refund and the payment view after a request.
func (s *Service) result(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, opts ViewOptions, refund uuid.UUID, replayed bool) (RefundResult, error) {
	view, err := s.ReservationPayments(ctx, orgID, id, caller, opts)
	if err != nil {
		return RefundResult{}, err
	}
	out := RefundResult{Payments: view, Replayed: replayed}
	for _, f := range view.Refunds {
		if f.UUID == refund {
			out.Refund = f
		}
	}
	return out, nil
}

// Resolve settles a provider refund that is still open after a person
// checked the provider's panel: succeeded posts it now, failed ends it (a
// refund the provider confirmed can only be posted).
func (s *Service) Resolve(ctx context.Context, orgID int64, refundID uuid.UUID, caller availabilityusecase.Caller, actorID int64, opts ViewOptions, in model.ResolveInput) (RefundResult, error) {
	f, err := s.q.GetRefundByUUID(ctx, db.GetRefundByUUIDParams{OrganizationID: orgID, Uuid: refundID})
	if isNoRows(err) || (err == nil && orgctx.CheckBranch(ctx, f.BranchID) != nil) {
		return RefundResult{}, ErrNotFound
	}
	if err != nil {
		return RefundResult{}, err
	}
	if _, err := s.reservations.Get(ctx, orgID, f.ReservationUuid, caller); err != nil {
		if errors.Is(err, reservationsusecase.ErrNotFound) {
			return RefundResult{}, ErrNotFound
		}
		return RefundResult{}, err
	}
	note := clip(in.Note, maxReasonLength)
	switch in.Outcome {
	case model.ResolveSucceeded, model.ResolveFailed:
	default:
		return RefundResult{}, invalid("outcome", "invalid", "outcome must be succeeded or failed")
	}
	if f.Status != model.RefundRequested {
		return RefundResult{}, ErrRefundNotOpen
	}
	user := actorID
	if in.Outcome == model.ResolveFailed {
		if f.ProviderStatus == model.ProviderSucceeded {
			return RefundResult{}, invalid("outcome", "confirmed", "the provider confirmed this refund; it can only be posted")
		}
		n, err := s.q.ResolveRefundFailed(ctx, db.ResolveRefundFailedParams{
			OrganizationID: orgID, ID: f.ID, FailureReason: clip("not refunded at the provider (checked by staff)", 500),
			ResolvedBy: pgtype.Int8{Int64: actorID, Valid: actorID > 0}, ResolutionNote: note,
		})
		if err != nil {
			return RefundResult{}, err
		}
		if n == 0 {
			return RefundResult{}, ErrRefundNotOpen
		}
	} else {
		if err := s.q.SetRefundResolver(ctx, db.SetRefundResolverParams{
			OrganizationID: orgID, ID: f.ID, ResolvedBy: pgtype.Int8{Int64: actorID, Valid: actorID > 0}, ResolutionNote: note,
		}); err != nil {
			return RefundResult{}, err
		}
		ref := db.Refund{ID: f.ID, Uuid: f.Uuid, OrganizationID: orgID, Amount: f.Amount, Currency: f.Currency, Method: f.Method}
		if err := s.postProviderRefund(ctx, orgID, f.ReservationUuid, ref, &user); err != nil {
			if errors.Is(err, errRefundApplied) {
				return RefundResult{}, ErrRefundNotOpen
			}
			return RefundResult{}, mapRefundErr(err)
		}
	}
	s.changed(ctx, orgID, f.ReservationUuid)
	return s.result(ctx, orgID, f.ReservationUuid, caller, opts, f.Uuid, false)
}

// mapRefundErr turns lower layer answers into the module's.
func mapRefundErr(err error) error {
	if errors.Is(err, reservationsusecase.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
