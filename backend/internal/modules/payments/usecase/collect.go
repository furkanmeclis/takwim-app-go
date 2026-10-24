package usecase

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	salesmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/model"
	salesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// ViewOptions say what the caller may do (the handler reads them from the
// member's permissions and the business's modules).
type ViewOptions struct {
	// Collect: the member holds tenant.payment.collect and the cash module
	// is on.
	Collect bool
}

// collectable reports whether money may be taken for a visit in status.
func collectable(status string) bool {
	return status != reservationsmodel.StatusCancelled && status != reservationsmodel.StatusRejected
}

// ReservationPayments is the payment view of a reservation the caller may
// see (caller: the reservations reach).
func (s *Service) ReservationPayments(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, opts ViewOptions) (model.ReservationPayments, error) {
	res, err := s.reservations.Get(ctx, orgID, id, caller)
	if errors.Is(err, reservationsusecase.ErrNotFound) {
		return model.ReservationPayments{}, ErrNotFound
	}
	if err != nil {
		return model.ReservationPayments{}, err
	}
	bal, err := s.sales.ReservationBalance(ctx, orgID, id)
	if errors.Is(err, salesusecase.ErrReservationMissing) {
		return model.ReservationPayments{}, ErrNotFound
	}
	if err != nil {
		return model.ReservationPayments{}, err
	}
	row, err := s.q.GetReservationRecord(ctx, db.GetReservationRecordParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return model.ReservationPayments{}, err
	}
	out := model.ReservationPayments{
		Reservation: bal.Reservation, Status: res.Status, PaymentStatus: res.PaymentStatus, Currency: bal.Currency,
		Expected: bal.Expected, Paid: bal.Paid, Remaining: bal.Remaining, Credit: bal.Credit,
		Entries: []salesmodel.LedgerEntry{}, Online: []model.Attempt{}, CollectAccounts: []model.CollectAccount{},
	}
	// Money only: the sale lines (revenue, staff) stay in the finance views.
	for _, e := range bal.Entries {
		if e.Kind != salesmodel.EntrySale && e.EffectiveKind != salesmodel.EntrySale {
			out.Entries = append(out.Entries, e)
		}
	}
	attempts, err := s.q.ListReservationPayments(ctx, db.ListReservationPaymentsParams{OrganizationID: orgID, ReservationID: row.ID})
	if err != nil {
		return out, err
	}
	for _, a := range attempts {
		at := model.Attempt{
			UUID: a.Uuid, Provider: a.Provider, Amount: amountOf(a.Amount), Currency: a.Currency, Status: a.Status,
			Outcome: a.Outcome, FailureReason: a.FailureReason, CashAccount: model.Ref{UUID: a.CashAccountUuid, Name: a.CashAccountName},
			ExpiresAt: a.ExpiresAt.Time, CompletedAt: timePtr(a.CompletedAt), CreatedAt: a.CreatedAt.Time,
		}
		if a.PaidAmount.Valid {
			v := amountOf(a.PaidAmount)
			at.PaidAmount = &v
		}
		out.Online = append(out.Online, at)
	}
	if opts.Collect && collectable(res.Status) {
		accounts, err := s.q.ListCollectAccounts(ctx, db.ListCollectAccountsParams{
			OrganizationID: orgID, Currency: res.Currency, BranchID: pgtype.Int8{Int64: row.BranchID, Valid: true},
		})
		if err != nil {
			return out, err
		}
		for _, a := range accounts {
			out.CollectAccounts = append(out.CollectAccounts, model.CollectAccount{
				UUID: a.Uuid, Name: a.Name, Kind: a.Kind, Currency: a.Currency, Branch: a.BranchID.Valid,
			})
		}
		out.CanCollect = out.Remaining.IsPositive() && len(out.CollectAccounts) > 0
	}
	return out, nil
}

// Collect records money taken at the business for a reservation (cash,
// card or transfer; a part or the rest): a ledger collection with its cash
// movement into a till or bank account of the visit's branch (or a
// business-wide one), in the reservation's transaction. A visit paid in
// full becomes paid. At most the remaining amount is taken.
func (s *Service) Collect(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, actorID int64, in model.CollectInput) (model.ReservationPayments, error) {
	view, err := s.ReservationPayments(ctx, orgID, id, caller, ViewOptions{})
	if err != nil {
		return view, err
	}
	if !collectable(view.Status) {
		return view, ErrNotCollectable
	}
	ve := &ValidationError{}
	if !in.Amount.IsPositive() {
		ve.add("amount", "invalid", "amount must be greater than zero")
	} else if in.Amount.Minor() > view.Remaining.Minor() {
		ve.add("amount", "exceeds_remaining", "amount is more than what is left to pay")
	}
	if !validMethod(in.Method) {
		ve.add("method", "invalid", "method must be one of cash, card, transfer")
	}
	note := clip(in.Note, maxNoteLength)
	var account db.GetCollectAccountRow
	if in.CashAccountUUID == uuid.Nil {
		ve.add("cash_account_uuid", "required", "cash_account_uuid is required")
	} else {
		account, err = s.q.GetCollectAccount(ctx, db.GetCollectAccountParams{OrganizationID: orgID, Uuid: in.CashAccountUUID})
		if err != nil && !isNoRows(err) {
			return view, err
		}
		if isNoRows(err) {
			ve.add("cash_account_uuid", "not_found", "cash account not found")
		}
	}
	if len(ve.Details) > 0 {
		return view, ve
	}
	if account.Status != "active" || (account.Kind != "cash" && account.Kind != "bank") {
		return view, invalid("cash_account_uuid", "invalid", "the account must be an active till or bank account")
	}
	if account.Currency != view.Currency {
		return view, invalid("cash_account_uuid", "currency", "the account has another currency")
	}
	user := actorID
	desc := note
	if desc == "" {
		desc = "collection at the business"
	}
	_, _, err = s.reservations.RecordPayment(ctx, orgID, id, caller, actorID, false,
		in.Amount.String()+" "+view.Currency+" · "+in.Method, func(ctx context.Context, q *db.Queries, row db.Reservation) (money.Money, error) {
			if account.BranchID.Valid && account.BranchID.Int64 != row.BranchID {
				return money.Zero(), invalid("cash_account_uuid", "not_found", "cash account not found")
			}
			resUUID := row.Uuid
			if _, err := s.sales.RecordCollectionTx(ctx, q, orgID, salesmodel.CollectionInput{
				ReservationUUID: &resUUID, CashAccountUUID: account.Uuid, Amount: in.Amount, Method: in.Method,
				Description: desc, SourceType: model.SourceReservation, SourceUUID: &resUUID, CreatedBy: &user,
			}); err != nil {
				return money.Zero(), err
			}
			return paidOf(ctx, q, orgID, row.ID, row.Currency)
		})
	if errors.Is(err, reservationsusecase.ErrInvalidTransition) {
		return view, ErrNotCollectable
	}
	if errors.Is(err, reservationsusecase.ErrNotFound) {
		return view, ErrNotFound
	}
	if err != nil {
		return view, err
	}
	s.changed(ctx, orgID, id)
	return s.ReservationPayments(ctx, orgID, id, caller, ViewOptions{Collect: true})
}
