package usecase

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	cashmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/model"
	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// EntrySortColumns are the sortable ledger list fields.
var EntrySortColumns = map[string]string{
	"entry_date": "entry_date", "amount": "amount", "kind": "kind", "created_at": "created_at",
}

// SourceLedgerEntry names ledger entries on the cash movements they write.
const SourceLedgerEntry = "ledger_entry"

// GetEntry returns one ledger entry inside the caller's branch scope.
func (s *Service) GetEntry(ctx context.Context, orgID int64, id uuid.UUID) (model.LedgerEntry, error) {
	row, err := s.repo.Q().GetLedgerEntry(ctx, db.GetLedgerEntryParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if isNotFound(err) {
			return model.LedgerEntry{}, ErrEntryNotFound
		}
		return model.LedgerEntry{}, err
	}
	if orgctx.CheckBranch(ctx, row.BranchID) != nil {
		return model.LedgerEntry{}, ErrEntryNotFound
	}
	return MapEntry(row), nil
}

// ListEntries returns one page of the ledger of the caller's scope.
func (s *Service) ListEntries(ctx context.Context, orgID int64, f model.EntryFilter) ([]model.LedgerEntry, int64, error) {
	if _, ok := EntrySortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "", false
	}
	q := s.repo.Q()
	a, err := s.resolveFilter(ctx, q, orgID, f.Branches, f.ReservationUUID, f.CustomerUUID, nil)
	if err != nil {
		return nil, 0, err
	}
	if a.empty {
		return []model.LedgerEntry{}, 0, nil
	}
	scope := orgctx.BranchScope(ctx)
	rows, err := q.ListLedgerEntries(ctx, db.ListLedgerEntriesParams{
		OrganizationID: orgID, BranchIds: scope, FilterBranchIds: a.branches, Kinds: nilIfEmpty(f.Kinds),
		ReservationID: a.reservation, CustomerID: a.customer, DateFrom: datePtrArg(f.DateFrom),
		DateTo: datePtrArg(f.DateTo), Q: textArg(f.Q), SortField: f.SortField, SortDesc: f.SortDesc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountLedgerEntries(ctx, db.CountLedgerEntriesParams{
		OrganizationID: orgID, BranchIds: scope, FilterBranchIds: a.branches, Kinds: nilIfEmpty(f.Kinds),
		ReservationID: a.reservation, CustomerID: a.customer, DateFrom: datePtrArg(f.DateFrom),
		DateTo: datePtrArg(f.DateTo), Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.LedgerEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, MapEntry(db.GetLedgerEntryRow(r)))
	}
	return out, total, nil
}

// party is who and where a collection belongs to.
type party struct {
	branchID    int64
	customer    pgtype.Int8
	reservation pgtype.Int8
	currency    string
	clock       clock
}

func (s *Service) partyOf(ctx context.Context, q *db.Queries, orgID int64, in model.CollectionInput) (party, error) {
	var p party
	switch {
	case in.ReservationUUID != nil:
		r, err := q.GetSalesReservationByUUID(ctx, db.GetSalesReservationByUUIDParams{OrganizationID: orgID, Uuid: *in.ReservationUUID})
		if err != nil {
			if isNotFound(err) {
				return p, invalid("reservation_uuid", "not_found", "reservation not found")
			}
			return p, err
		}
		if orgctx.CheckBranch(ctx, r.BranchID) != nil {
			return p, invalid("reservation_uuid", "not_found", "reservation not found")
		}
		p.branchID, p.customer, p.reservation, p.currency = r.BranchID, int8Of(r.CustomerID), int8Of(r.ID), r.Currency
	case in.BranchUUID != nil:
		rows, err := q.ListSalesBranchesByUUID(ctx, db.ListSalesBranchesByUUIDParams{OrganizationID: orgID, Uuids: []uuid.UUID{*in.BranchUUID}})
		if err != nil {
			return p, err
		}
		if len(rows) == 0 || orgctx.CheckBranch(ctx, rows[0].ID) != nil {
			return p, invalid("branch_uuid", "not_found", "branch not found")
		}
		p.branchID = rows[0].ID
		if in.CustomerUUID != nil {
			c, err := q.GetSalesCustomerByUUID(ctx, db.GetSalesCustomerByUUIDParams{OrganizationID: orgID, Uuid: *in.CustomerUUID})
			if err != nil {
				if isNotFound(err) {
					return p, invalid("customer_uuid", "not_found", "customer not found")
				}
				return p, err
			}
			p.customer = int8Of(c.ID)
		}
	default:
		return p, invalid("reservation_uuid", "required", "a reservation or a branch is required")
	}
	c, err := s.clockOf(ctx, q, orgID, p.branchID)
	if err != nil {
		return p, err
	}
	p.clock = c
	if p.currency == "" {
		p.currency = c.currency
	}
	return p, nil
}

// replay returns the entry written before with the same idempotency key
// (uuid.Nil when there is none).
func replay(ctx context.Context, q *db.Queries, orgID int64, key, kind string, amount money.Money) (uuid.UUID, error) {
	if key == "" {
		return uuid.Nil, nil
	}
	prev, err := q.GetLedgerEntryByIdempotencyKey(ctx, db.GetLedgerEntryByIdempotencyKeyParams{OrganizationID: orgID, IdempotencyKey: textArg(key)})
	if err != nil {
		if isNotFound(err) {
			return uuid.Nil, nil
		}
		return uuid.Nil, err
	}
	if prev.Kind != kind || !amountOf(prev.Amount).Equal(amount) {
		return uuid.Nil, ErrIdempotencyReused
	}
	return prev.Uuid, nil
}

func checkMoney(ve *ValidationError, in money.Money, method string, account uuid.UUID) {
	if !in.IsPositive() {
		ve.add("amount", "invalid", "amount must be greater than zero")
	}
	if !slices.Contains(model.Methods, method) {
		ve.add("method", "invalid", "method must be one of cash, card, transfer, online, other")
	}
	if account == uuid.Nil {
		ve.add("cash_account_uuid", "required", "cash_account_uuid is required")
	}
}

// postMoney writes the cash movement of a collection or refund and checks
// that its account holds the entry's currency.
func (s *Service) postMoney(ctx context.Context, q *db.Queries, orgID int64, entry uuid.UUID, kind string, account uuid.UUID,
	amount money.Money, date time.Time, desc, key, currency string, userID *int64,
) (int64, error) {
	cashKey := ""
	if key != "" {
		cashKey = "ledger:" + key
	}
	mvID, err := s.cash.PostTx(ctx, q, orgID, cashusecase.PostInput{
		AccountUUID: account, Kind: kind, Amount: amount, Date: date, Description: desc,
		SourceType: SourceLedgerEntry, SourceUUID: &entry, IdempotencyKey: cashKey, CreatedBy: userID,
	})
	if err != nil {
		if errors.Is(err, cashusecase.ErrNotFound) {
			return 0, invalid("cash_account_uuid", "not_found", "cash account not found")
		}
		return 0, err
	}
	mv, err := q.GetSalesCashMovement(ctx, db.GetSalesCashMovementParams{OrganizationID: orgID, Uuid: mvID})
	if err != nil {
		return 0, err
	}
	if mv.Currency != currency {
		return 0, ErrCurrencyMismatch
	}
	return mv.ID, nil
}

// RecordCollectionTx records money taken (a credit) and its cash movement
// inside the caller's transaction. Payments (TEC-912) call it; a retried
// posting with the same idempotency key returns the first entry.
func (s *Service) RecordCollectionTx(ctx context.Context, q *db.Queries, orgID int64, in model.CollectionInput) (uuid.UUID, error) {
	ve := &ValidationError{}
	checkMoney(ve, in.Amount, in.Method, in.CashAccountUUID)
	if err := ve.orNil(); err != nil {
		return uuid.Nil, err
	}
	if prev, err := replay(ctx, q, orgID, in.IdempotencyKey, model.EntryCollection, in.Amount); err != nil || prev != uuid.Nil {
		return prev, err
	}
	p, err := s.partyOf(ctx, q, orgID, in)
	if err != nil {
		return uuid.Nil, err
	}
	date := in.Date
	if date.IsZero() {
		date = p.clock.day(s.now())
	}
	id := uuid.New()
	desc := clip(in.Description, maxReasonLength)
	mvID, err := s.postMoney(ctx, q, orgID, id, cashmodel.MovementCollection, in.CashAccountUUID, in.Amount, date, desc,
		in.IdempotencyKey, p.currency, in.CreatedBy)
	if err != nil {
		return uuid.Nil, err
	}
	_, err = q.InsertLedgerEntry(ctx, db.InsertLedgerEntryParams{
		Uuid: pgtype.UUID{Bytes: id, Valid: true}, OrganizationID: orgID, BranchID: p.branchID, CustomerID: p.customer,
		ReservationID: p.reservation, Kind: model.EntryCollection, Direction: model.Credit, Amount: in.Amount.Numeric(),
		Currency: p.currency, EntryDate: dateArg(date), Method: in.Method, CashMovementID: int8Of(mvID),
		SourceType: in.SourceType, SourceUuid: uuidArg(in.SourceUUID), IdempotencyKey: textArg(in.IdempotencyKey),
		Description: desc, CreatedBy: int8Ptr(in.CreatedBy),
	})
	if err != nil {
		return uuid.Nil, mapRepoErr(err)
	}
	return id, nil
}

// RecordCollection is RecordCollectionTx in its own transaction.
func (s *Service) RecordCollection(ctx context.Context, orgID int64, in model.CollectionInput) (model.LedgerEntry, error) {
	var id uuid.UUID
	err := s.repo.InTx(ctx, func(q *db.Queries) error {
		var err error
		id, err = s.RecordCollectionTx(ctx, q, orgID, in)
		return err
	})
	if err != nil {
		return model.LedgerEntry{}, err
	}
	return s.GetEntry(ctx, orgID, id)
}

// RecordRefundTx pays back (part of) a collection (a debit) out of a cash
// account inside the caller's transaction. Refunds (TEC-913) call it. The
// refunds of a collection never exceed it.
func (s *Service) RecordRefundTx(ctx context.Context, q *db.Queries, orgID int64, in model.RefundInput) (uuid.UUID, error) {
	ve := &ValidationError{}
	checkMoney(ve, in.Amount, in.Method, in.CashAccountUUID)
	if err := ve.orNil(); err != nil {
		return uuid.Nil, err
	}
	if prev, err := replay(ctx, q, orgID, in.IdempotencyKey, model.EntryRefund, in.Amount); err != nil || prev != uuid.Nil {
		return prev, err
	}
	col, err := q.GetLedgerEntryRow(ctx, db.GetLedgerEntryRowParams{OrganizationID: orgID, Uuid: in.CollectionUUID})
	if err != nil {
		if isNotFound(err) {
			return uuid.Nil, invalid("collection_uuid", "not_found", "collection not found")
		}
		return uuid.Nil, err
	}
	if col.Kind != model.EntryCollection || orgctx.CheckBranch(ctx, col.BranchID) != nil {
		return uuid.Nil, invalid("collection_uuid", "not_found", "collection not found")
	}
	if col.Reversed {
		return uuid.Nil, ErrAlreadyReversed
	}
	// Serialize refunds of one collection.
	if _, err := q.LockLedgerEntry(ctx, db.LockLedgerEntryParams{OrganizationID: orgID, ID: col.ID}); err != nil {
		return uuid.Nil, err
	}
	refunded, err := q.RefundedOfCollection(ctx, db.RefundedOfCollectionParams{OrganizationID: orgID, CollectionID: int8Of(col.ID)})
	if err != nil {
		return uuid.Nil, err
	}
	if amountOf(refunded).Add(in.Amount).Cmp(amountOf(col.Amount)) > 0 {
		return uuid.Nil, ErrRefundTooLarge
	}
	c, err := s.clockOf(ctx, q, orgID, col.BranchID)
	if err != nil {
		return uuid.Nil, err
	}
	date := in.Date
	if date.IsZero() {
		date = c.day(s.now())
	}
	id := uuid.New()
	desc := clip(in.Description, maxReasonLength)
	mvID, err := s.postMoney(ctx, q, orgID, id, cashmodel.MovementRefund, in.CashAccountUUID, in.Amount, date, desc,
		in.IdempotencyKey, col.Currency, in.CreatedBy)
	if err != nil {
		return uuid.Nil, err
	}
	_, err = q.InsertLedgerEntry(ctx, db.InsertLedgerEntryParams{
		Uuid: pgtype.UUID{Bytes: id, Valid: true}, OrganizationID: orgID, BranchID: col.BranchID, CustomerID: col.CustomerID,
		ReservationID: col.ReservationID, Kind: model.EntryRefund, Direction: model.Debit, Amount: in.Amount.Numeric(),
		Currency: col.Currency, EntryDate: dateArg(date), Method: in.Method, CashMovementID: int8Of(mvID),
		RefundsEntryID: int8Of(col.ID), SourceType: in.SourceType, SourceUuid: uuidArg(in.SourceUUID),
		IdempotencyKey: textArg(in.IdempotencyKey), Description: desc, CreatedBy: int8Ptr(in.CreatedBy),
	})
	if err != nil {
		return uuid.Nil, mapRepoErr(err)
	}
	return id, nil
}

// InTx runs fn in one transaction of the ledger's database (the refund
// flow, TEC-913, writes its request rows next to a lock on the collection).
func (s *Service) InTx(ctx context.Context, fn func(q *db.Queries) error) error {
	return s.repo.InTx(ctx, fn)
}

// RecordRefund is RecordRefundTx in its own transaction.
func (s *Service) RecordRefund(ctx context.Context, orgID int64, in model.RefundInput) (model.LedgerEntry, error) {
	var id uuid.UUID
	err := s.repo.InTx(ctx, func(q *db.Queries) error {
		var err error
		id, err = s.RecordRefundTx(ctx, q, orgID, in)
		return err
	})
	if err != nil {
		return model.LedgerEntry{}, err
	}
	return s.GetEntry(ctx, orgID, id)
}

// ReverseEntryTx reverses a collection or a refund (posted by mistake)
// inside the caller's transaction: the opposite ledger entry and the
// reversal of its cash movement, both dated today. A sale entry is
// reversed through its sale line; a collection with live refunds only
// after them.
func (s *Service) ReverseEntryTx(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID, reason string, userID *int64) (uuid.UUID, error) {
	e, err := q.GetLedgerEntryRow(ctx, db.GetLedgerEntryRowParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if isNotFound(err) {
			return uuid.Nil, ErrEntryNotFound
		}
		return uuid.Nil, err
	}
	if orgctx.CheckBranch(ctx, e.BranchID) != nil {
		return uuid.Nil, ErrEntryNotFound
	}
	switch e.Kind {
	case model.EntrySale:
		return uuid.Nil, ErrReverseSaleLine
	case model.EntryReversal:
		return uuid.Nil, ErrNotReversible
	}
	if e.Reversed {
		return uuid.Nil, ErrAlreadyReversed
	}
	if e.Kind == model.EntryCollection {
		if _, err := q.LockLedgerEntry(ctx, db.LockLedgerEntryParams{OrganizationID: orgID, ID: e.ID}); err != nil {
			return uuid.Nil, err
		}
		refunded, err := q.RefundedOfCollection(ctx, db.RefundedOfCollectionParams{OrganizationID: orgID, CollectionID: int8Of(e.ID)})
		if err != nil {
			return uuid.Nil, err
		}
		if amountOf(refunded).IsPositive() {
			return uuid.Nil, ErrHasRefunds
		}
	}
	c, err := s.clockOf(ctx, q, orgID, e.BranchID)
	if err != nil {
		return uuid.Nil, err
	}
	if e.CashMovementID.Valid {
		mv, err := q.GetSalesCashMovementByID(ctx, db.GetSalesCashMovementByIDParams{OrganizationID: orgID, ID: e.CashMovementID.Int64})
		if err != nil {
			return uuid.Nil, err
		}
		if _, err := s.cash.ReverseModuleTx(ctx, q, orgID, mv.Uuid, reason, userID); err != nil {
			return uuid.Nil, err
		}
	}
	dir := model.Debit
	if e.Direction == model.Debit {
		dir = model.Credit
	}
	row, err := q.InsertLedgerEntry(ctx, db.InsertLedgerEntryParams{
		OrganizationID: orgID, BranchID: e.BranchID, CustomerID: e.CustomerID, ReservationID: e.ReservationID,
		Kind: model.EntryReversal, Direction: dir, Amount: e.Amount, Currency: e.Currency,
		EntryDate: dateArg(c.day(s.now())), Method: e.Method, ReversesEntryID: int8Of(e.ID),
		SourceType: e.SourceType, SourceUuid: e.SourceUuid, Description: reason, CreatedBy: int8Ptr(userID),
	})
	if err != nil {
		return uuid.Nil, mapRepoErr(err)
	}
	return row.Uuid, nil
}

// ReverseEntry is ReverseEntryTx in its own transaction (the panel).
func (s *Service) ReverseEntry(ctx context.Context, orgID int64, id uuid.UUID, userID *int64, in model.ReverseInput) (model.LedgerEntry, error) {
	// An unknown or foreign entry answers not found before the body is
	// looked at.
	if _, err := s.GetEntry(ctx, orgID, id); err != nil {
		return model.LedgerEntry{}, err
	}
	reason, err := checkReason(in.Reason)
	if err != nil {
		return model.LedgerEntry{}, err
	}
	var out uuid.UUID
	err = s.repo.InTx(ctx, func(q *db.Queries) error {
		var err error
		out, err = s.ReverseEntryTx(ctx, q, orgID, id, reason, userID)
		return err
	})
	if err != nil {
		return model.LedgerEntry{}, err
	}
	return s.GetEntry(ctx, orgID, out)
}

// MapEntry maps a ledger entry row (detail and list).
func MapEntry(r db.GetLedgerEntryRow) model.LedgerEntry {
	e := model.LedgerEntry{
		UUID: r.Uuid, Kind: r.Kind, EffectiveKind: r.EffectiveKind, Direction: r.Direction, Amount: amountOf(r.Amount),
		Currency: r.Currency, EntryDate: formatDate(r.EntryDate.Time), Method: r.Method,
		Branch: model.Ref{UUID: r.BranchUuid, Name: r.BranchName}, SaleLineUUID: uuidPtr(r.SaleLineUuid),
		CashMovementUUID: uuidPtr(r.CashMovementUuid), RefundsUUID: uuidPtr(r.RefundsUuid),
		ReversesUUID: uuidPtr(r.ReversesUuid), ReversedByUUID: uuidPtr(r.ReversedByUuid), SourceType: r.SourceType,
		SourceUUID: uuidPtr(r.SourceUuid), Description: r.Description, CreatedAt: r.CreatedAt.Time,
	}
	if r.CustomerUuid.Valid {
		e.Customer = &model.Ref{UUID: uuid.UUID(r.CustomerUuid.Bytes), Name: name(r.CustomerFirstName, r.CustomerLastName)}
	}
	if r.ReservationUuid.Valid {
		e.Reservation = &model.ReservationRef{UUID: uuid.UUID(r.ReservationUuid.Bytes), Number: r.ReservationNumber.Int64}
	}
	if r.CashAccountUuid.Valid {
		e.CashAccount = &model.Ref{UUID: uuid.UUID(r.CashAccountUuid.Bytes), Name: r.CashAccountName.String}
	}
	e.CreatedBy = userRef(r.CreatorUuid, r.CreatorName, r.CreatorSurname)
	return e
}
