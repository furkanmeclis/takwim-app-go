package usecase

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	productsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/model"
	salesmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/model"
	salesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Money of walk-in product sales (TEC-992, docs/design/payments.md
// "Product sales"):
//
//   - A stock sale charges what its live revenue debits say (the sale line
//     the products hook posted, corrected or reversed on the books). Money
//     taken for it is a ledger collection with the movement as its source,
//     at most the remaining amount, with the sale (the products hook, in
//     the sale's transaction) or later (CollectStockSale). Collections of
//     one sale serialize on its sale debits.
//   - A refund pays back what is left of the sale's collections, oldest
//     first, by hand out of a till, bank or online account: with the return
//     (the products hook, in the return's transaction) or later
//     (RefundStockSale). Every refund entry goes through RecordRefundTx, so
//     the refunds of a collection never exceed it, also under concurrency.
//   - A stock sale has no customer and no reservation: no refund notice is
//     sent and no payment status follows.

// StockSaleParty is the sale a collection or refund belongs to.
type StockSaleParty struct {
	MovementUUID uuid.UUID
	BranchID     int64
	BranchUUID   uuid.UUID
}

// stockSale loads a sale movement the caller may see (any other movement,
// or one outside the branch scope, is not found).
func (s *Service) stockSale(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (db.GetStockMovementRow, error) {
	row, err := q.GetStockMovement(ctx, db.GetStockMovementParams{OrganizationID: orgID, Uuid: id})
	if isNoRows(err) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, err
	}
	if row.Kind != productsmodel.KindSale || orgctx.CheckBranch(ctx, row.BranchID) != nil {
		return row, ErrNotFound
	}
	return row, nil
}

type stockSaleMoney struct {
	charged, collected, refunded money.Money
	currency                     string
}

func (m stockSaleMoney) paid() money.Money      { return m.collected.Sub(m.refunded) }
func (m stockSaleMoney) remaining() money.Money { return m.charged.Sub(m.paid()).ClampZero() }

// stockSaleMoney reads the ledger figures of a sale; the currency is the
// entries' one, else the branch's.
func (s *Service) stockSaleMoney(ctx context.Context, q *db.Queries, orgID, branchID int64, movement uuid.UUID) (stockSaleMoney, error) {
	t, err := q.StockSaleLedgerTotals(ctx, db.StockSaleLedgerTotalsParams{OrganizationID: orgID, MovementUuid: movement})
	if err != nil {
		return stockSaleMoney{}, err
	}
	out := stockSaleMoney{charged: amountOf(t.Charged), collected: amountOf(t.Collected), refunded: amountOf(t.Refunded), currency: t.Currency}
	if out.currency == "" {
		c, err := q.GetSalesClock(ctx, db.GetSalesClockParams{OrganizationID: orgID, BranchID: branchID})
		if err != nil {
			return out, err
		}
		out.currency = c.Currency
	}
	return out, nil
}

// StockSalePayments is the payment view of a stock sale.
func (s *Service) StockSalePayments(ctx context.Context, orgID int64, id uuid.UUID, opts ViewOptions) (model.StockSalePayments, error) {
	row, err := s.stockSale(ctx, s.q, orgID, id)
	if err != nil {
		return model.StockSalePayments{}, err
	}
	return s.stockSaleView(ctx, s.q, orgID, row, opts)
}

func (s *Service) stockSaleView(ctx context.Context, q *db.Queries, orgID int64, row db.GetStockMovementRow, opts ViewOptions) (model.StockSalePayments, error) {
	m, err := s.stockSaleMoney(ctx, q, orgID, row.BranchID, row.Uuid)
	if err != nil {
		return model.StockSalePayments{}, err
	}
	out := model.StockSalePayments{
		Movement: model.StockSaleRef{
			UUID: row.Uuid, Product: row.ProductName, Variant: row.VariantName.String,
			Branch: model.Ref{UUID: row.BranchUuid, Name: row.BranchName}, Quantity: -row.Quantity,
			Returned: row.Returned, OccurredAt: row.OccurredAt.Time,
		},
		Currency: m.currency, Expected: m.charged, Collected: m.collected, Refunded: m.refunded, Paid: m.paid(),
		Remaining: m.remaining(), Refundable: m.paid().ClampZero(),
		Entries: []salesmodel.LedgerEntry{}, Collections: []model.StockSaleCollection{},
		CollectAccounts: []model.CollectAccount{}, RefundAccounts: []model.CollectAccount{},
	}
	entries, err := q.ListStockSaleEntries(ctx, db.ListStockSaleEntriesParams{OrganizationID: orgID, MovementUuid: row.Uuid})
	if err != nil {
		return out, err
	}
	for _, r := range entries {
		e := salesusecase.MapEntry(db.GetLedgerEntryRow(r))
		// Money only: the revenue stays in the finance views.
		if e.Kind != salesmodel.EntrySale && e.EffectiveKind != salesmodel.EntrySale {
			out.Entries = append(out.Entries, e)
		}
	}
	cols, err := q.ListStockSaleCollections(ctx, db.ListStockSaleCollectionsParams{OrganizationID: orgID, MovementUuid: row.Uuid})
	if err != nil {
		return out, err
	}
	for _, c := range cols {
		out.Collections = append(out.Collections, model.StockSaleCollection{
			UUID: c.Uuid, Method: c.Method, EntryDate: c.EntryDate.Time.Format("2006-01-02"), Amount: amountOf(c.Amount),
			Refunded: amountOf(c.Refunded), Refundable: amountOf(c.Amount).Sub(amountOf(c.Refunded)).ClampZero(),
			CashAccount: model.Ref{UUID: c.CashAccountUuid, Name: c.CashAccountName},
		})
	}
	branch := pgtype.Int8{Int64: row.BranchID, Valid: true}
	if opts.Collect && !row.Returned {
		accounts, err := q.ListCollectAccounts(ctx, db.ListCollectAccountsParams{OrganizationID: orgID, Currency: m.currency, BranchID: branch})
		if err != nil {
			return out, err
		}
		out.CollectAccounts = collectAccounts(accounts)
		out.CanCollect = out.Remaining.IsPositive() && len(out.CollectAccounts) > 0
	}
	if opts.Refund {
		accounts, err := q.ListRefundAccounts(ctx, db.ListRefundAccountsParams{OrganizationID: orgID, Currency: m.currency, BranchID: branch})
		if err != nil {
			return out, err
		}
		for _, a := range accounts {
			out.RefundAccounts = append(out.RefundAccounts, model.CollectAccount{
				UUID: a.Uuid, Name: a.Name, Kind: a.Kind, Currency: a.Currency, Branch: a.BranchID.Valid,
			})
		}
		out.CanRefund = out.Refundable.IsPositive() && len(out.RefundAccounts) > 0
	}
	return out, nil
}

func collectAccounts(rows []db.ListCollectAccountsRow) []model.CollectAccount {
	out := make([]model.CollectAccount, 0, len(rows))
	for _, a := range rows {
		out = append(out, model.CollectAccount{UUID: a.Uuid, Name: a.Name, Kind: a.Kind, Currency: a.Currency, Branch: a.BranchID.Valid})
	}
	return out
}

// StockPaymentAccounts lists the accounts counter sales of a branch take
// money into and pay it back from, in the branch's currency.
func (s *Service) StockPaymentAccounts(ctx context.Context, orgID int64, branch uuid.UUID, opts ViewOptions) (model.StockPaymentAccounts, error) {
	out := model.StockPaymentAccounts{CollectAccounts: []model.CollectAccount{}, RefundAccounts: []model.CollectAccount{}}
	rows, err := s.q.ListSalesBranchesByUUID(ctx, db.ListSalesBranchesByUUIDParams{OrganizationID: orgID, Uuids: []uuid.UUID{branch}})
	if err != nil {
		return out, err
	}
	if len(rows) == 0 || orgctx.CheckBranch(ctx, rows[0].ID) != nil {
		return out, invalid("branch_uuid", "not_found", "branch not found")
	}
	c, err := s.q.GetSalesClock(ctx, db.GetSalesClockParams{OrganizationID: orgID, BranchID: rows[0].ID})
	if err != nil {
		return out, err
	}
	out.Currency = c.Currency
	id := pgtype.Int8{Int64: rows[0].ID, Valid: true}
	if opts.Collect {
		accounts, err := s.q.ListCollectAccounts(ctx, db.ListCollectAccountsParams{OrganizationID: orgID, Currency: c.Currency, BranchID: id})
		if err != nil {
			return out, err
		}
		out.CollectAccounts = collectAccounts(accounts)
	}
	if opts.Refund {
		accounts, err := s.q.ListRefundAccounts(ctx, db.ListRefundAccountsParams{OrganizationID: orgID, Currency: c.Currency, BranchID: id})
		if err != nil {
			return out, err
		}
		for _, a := range accounts {
			out.RefundAccounts = append(out.RefundAccounts, model.CollectAccount{
				UUID: a.Uuid, Name: a.Name, Kind: a.Kind, Currency: a.Currency, Branch: a.BranchID.Valid,
			})
		}
	}
	return out, nil
}

// CollectStockSale records money taken for a stock sale after the sale
// (cash, card or transfer; a part or the rest).
func (s *Service) CollectStockSale(ctx context.Context, orgID int64, id uuid.UUID, actorID int64, opts ViewOptions, in model.CollectInput) (model.StockSalePayments, error) {
	row, err := s.stockSale(ctx, s.q, orgID, id)
	if err != nil {
		return model.StockSalePayments{}, err
	}
	if row.Returned {
		return model.StockSalePayments{}, ErrNotCollectable
	}
	if !in.Amount.IsPositive() {
		return model.StockSalePayments{}, invalid("amount", "invalid", "amount must be greater than zero")
	}
	user := actorID
	party := StockSaleParty{MovementUUID: row.Uuid, BranchID: row.BranchID, BranchUUID: row.BranchUuid}
	err = s.sales.InTx(ctx, func(q *db.Queries) error {
		return s.CollectStockSaleTx(ctx, q, orgID, party, in, "", &user)
	})
	if err != nil {
		return model.StockSalePayments{}, err
	}
	s.stockChanged(ctx, orgID, row.ProductUuid)
	return s.StockSalePayments(ctx, orgID, id, opts)
}

// CollectStockSaleTx posts a collection for a stock sale inside q's
// transaction (the products hook calls it in the sale's own): at most the
// remaining amount (a zero amount takes all of it), into an active till or
// bank account of the sale's branch or a business-wide one. key is the
// ledger idempotency key (empty: none).
func (s *Service) CollectStockSaleTx(ctx context.Context, q *db.Queries, orgID int64, sale StockSaleParty, in model.CollectInput, key string, userID *int64) error {
	ve := &ValidationError{}
	if in.Amount.IsNegative() {
		ve.add("amount", "invalid", "amount must be greater than zero")
	}
	if !validMethod(in.Method) {
		ve.add("method", "invalid", "method must be one of cash, card, transfer")
	}
	var account db.GetCollectAccountRow
	if in.CashAccountUUID == uuid.Nil {
		ve.add("cash_account_uuid", "required", "cash_account_uuid is required")
	} else {
		var err error
		account, err = q.GetCollectAccount(ctx, db.GetCollectAccountParams{OrganizationID: orgID, Uuid: in.CashAccountUUID})
		if err != nil && !isNoRows(err) {
			return err
		}
		if isNoRows(err) || (account.BranchID.Valid && account.BranchID.Int64 != sale.BranchID) {
			ve.add("cash_account_uuid", "not_found", "cash account not found")
		}
	}
	if len(ve.Details) > 0 {
		return ve
	}
	if account.Status != "active" || (account.Kind != "cash" && account.Kind != "bank") {
		return invalid("cash_account_uuid", "invalid", "the account must be an active till or bank account")
	}
	// Collections of one sale serialize on its sale debits.
	if _, err := q.LockStockSaleDebits(ctx, db.LockStockSaleDebitsParams{OrganizationID: orgID, MovementUuid: sale.MovementUUID}); err != nil {
		return err
	}
	m, err := s.stockSaleMoney(ctx, q, orgID, sale.BranchID, sale.MovementUUID)
	if err != nil {
		return err
	}
	if account.Currency != m.currency {
		return invalid("cash_account_uuid", "currency", "the account has another currency")
	}
	amount := in.Amount
	if amount.IsZero() {
		amount = m.remaining()
	}
	switch {
	case !amount.IsPositive():
		return invalid("amount", "exceeds_remaining", "nothing is left to collect for this sale")
	case amount.Cmp(m.remaining()) > 0:
		return invalid("amount", "exceeds_remaining", "amount is more than what is left to pay")
	}
	desc := clip(in.Note, maxNoteLength)
	if desc == "" {
		desc = "product sale"
	}
	branch, movement := sale.BranchUUID, sale.MovementUUID
	_, err = s.sales.RecordCollectionTx(ctx, q, orgID, salesmodel.CollectionInput{
		BranchUUID: &branch, CashAccountUUID: account.Uuid, Amount: amount, Method: in.Method, Description: desc,
		SourceType: salesmodel.SourceStockMovement, SourceUUID: &movement, IdempotencyKey: key, CreatedBy: userID,
	})
	return err
}

// stockRefundKey prefixes the idempotency keys of refunds asked for after
// the return.
const stockRefundKey = "stock_refund:"

// RefundStockSale pays back (part of) the money taken for a stock sale, by
// hand, after the return (or without one). The same key answers the same
// refund again; the same key with another amount is refused.
func (s *Service) RefundStockSale(ctx context.Context, orgID int64, id uuid.UUID, actorID int64, opts ViewOptions, in model.StockSaleRefundInput) (model.StockSaleRefundResult, error) {
	row, err := s.stockSale(ctx, s.q, orgID, id)
	if err != nil {
		return model.StockSaleRefundResult{}, err
	}
	key := strings.TrimSpace(in.IdempotencyKey)
	switch {
	case key == "":
		return model.StockSaleRefundResult{}, invalid("idempotency_key", "required", "idempotency_key is required")
	case len(key) > maxKeyLength || !keyRE.MatchString(key):
		return model.StockSaleRefundResult{}, invalid("idempotency_key", "invalid", "idempotency_key must be at most 100 letters, digits or :_-.")
	}
	prev, err := s.q.ListLedgerEntriesByIdempotencyPrefix(ctx, db.ListLedgerEntriesByIdempotencyPrefixParams{OrganizationID: orgID, Key: stockRefundKey + key})
	if err != nil {
		return model.StockSaleRefundResult{}, err
	}
	if len(prev) > 0 {
		sum := money.Zero()
		for _, e := range prev {
			sum = sum.Add(amountOf(e.Amount))
		}
		if in.Amount.IsPositive() && !in.Amount.Equal(sum) {
			return model.StockSaleRefundResult{}, ErrIdempotencyReused
		}
		view, err := s.stockSaleView(ctx, s.q, orgID, row, opts)
		return model.StockSaleRefundResult{Payments: view, Replayed: true}, err
	}
	user := actorID
	party := StockSaleParty{MovementUUID: row.Uuid, BranchID: row.BranchID, BranchUUID: row.BranchUuid}
	err = s.sales.InTx(ctx, func(q *db.Queries) error {
		return s.RefundStockSaleTx(ctx, q, orgID, party, in, stockRefundKey+key, &user)
	})
	if err != nil {
		return model.StockSaleRefundResult{}, err
	}
	s.stockChanged(ctx, orgID, row.ProductUuid)
	view, err := s.StockSalePayments(ctx, orgID, id, opts)
	return model.StockSaleRefundResult{Payments: view}, err
}

// RefundStockSaleTx pays back (part of) the collections of a stock sale
// inside q's transaction (the products hook calls it in the return's own),
// oldest collection first, out of an active till, bank or online account
// of the sale's branch or a business-wide one. A zero amount pays back all
// that is left. The first refund entry takes key, the next ones key:2,
// key:3, ... (empty: no keys).
func (s *Service) RefundStockSaleTx(ctx context.Context, q *db.Queries, orgID int64, sale StockSaleParty, in model.StockSaleRefundInput, key string, userID *int64) error {
	ve := &ValidationError{}
	if in.Amount.IsNegative() {
		ve.add("amount", "invalid", "amount must be greater than zero")
	}
	if !slices.Contains(model.RefundMethods, in.Method) {
		ve.add("method", "invalid", "method must be one of cash, card, transfer, online")
	}
	if in.CashAccountUUID == uuid.Nil {
		ve.add("cash_account_uuid", "required", "cash_account_uuid is required")
	}
	if len(ve.Details) > 0 {
		return ve
	}
	acc, err := q.GetRefundAccount(ctx, db.GetRefundAccountParams{OrganizationID: orgID, Uuid: in.CashAccountUUID})
	if isNoRows(err) || (err == nil && acc.BranchID.Valid && acc.BranchID.Int64 != sale.BranchID) {
		return invalid("cash_account_uuid", "not_found", "cash account not found")
	}
	if err != nil {
		return err
	}
	if acc.Status != "active" || (acc.Kind != "cash" && acc.Kind != "bank" && acc.Kind != "online") {
		return invalid("cash_account_uuid", "invalid", "the account must be an active till, bank or online account")
	}
	cols, err := q.ListStockSaleCollections(ctx, db.ListStockSaleCollectionsParams{OrganizationID: orgID, MovementUuid: sale.MovementUUID})
	if err != nil {
		return err
	}
	left := money.Zero()
	for _, c := range cols {
		if acc.Currency != c.Currency {
			return invalid("cash_account_uuid", "currency", "the account has another currency")
		}
		left = left.Add(amountOf(c.Amount).Sub(amountOf(c.Refunded)).ClampZero())
	}
	amount := in.Amount
	if amount.IsZero() {
		amount = left
	}
	switch {
	case !amount.IsPositive():
		return invalid("amount", "exceeds_refundable", "nothing is left to pay back for this sale")
	case amount.Cmp(left) > 0:
		return salesusecase.ErrRefundTooLarge
	}
	desc := clip(in.Reason, maxReasonLength)
	if desc == "" {
		desc = "product refund"
	}
	movement := sale.MovementUUID
	rest, n := amount, 0
	for _, c := range cols {
		avail := amountOf(c.Amount).Sub(amountOf(c.Refunded)).ClampZero()
		if !avail.IsPositive() {
			continue
		}
		part := avail.Min(rest)
		n++
		entryKey := key
		if key != "" && n > 1 {
			entryKey = key + ":" + strconv.Itoa(n)
		}
		if _, err := s.sales.RecordRefundTx(ctx, q, orgID, salesmodel.RefundInput{
			CollectionUUID: c.Uuid, CashAccountUUID: acc.Uuid, Amount: part, Method: in.Method, Description: desc,
			SourceType: salesmodel.SourceStockMovement, SourceUUID: &movement, IdempotencyKey: entryKey, CreatedBy: userID,
		}); err != nil {
			return err
		}
		rest = rest.Sub(part)
		if rest.IsZero() {
			break
		}
	}
	if rest.IsPositive() {
		return salesusecase.ErrRefundTooLarge
	}
	return nil
}

// stockChanged refreshes the product (its movement list) and the ledger
// lists after money moved for a sale.
func (s *Service) stockChanged(ctx context.Context, orgID int64, product uuid.UUID) {
	if s.events == nil {
		return
	}
	ctx = orgctx.WithOrganizationID(ctx, orgID)
	s.events.Publish(ctx, productsmodel.Slug, "updated", product.String())
	s.events.Publish(ctx, salesmodel.EntriesSlug, "updated")
}
