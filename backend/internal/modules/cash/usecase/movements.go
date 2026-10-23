package usecase

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// MovementSortColumns are the sortable movement list fields.
var MovementSortColumns = map[string]string{
	"movement_date": "movement_date", "amount": "amount", "kind": "kind", "created_at": "created_at",
}

// PostInput is one movement written by another module (a payment, a sale,
// an expense, a staff payout) into the cash book. Kind must be one of the
// module kinds (collection, refund, income, expense, staff_payment); the
// direction follows from it. A posting with an IdempotencyKey that was
// written before returns the first movement instead of a second one.
type PostInput struct {
	AccountUUID    uuid.UUID
	Kind           string
	Amount         money.Money
	Date           time.Time // zero = today
	Description    string
	SourceType     string
	SourceUUID     *uuid.UUID
	IdempotencyKey string
	CreatedBy      *int64
}

// moduleKinds maps the kinds other modules post to their direction.
var moduleKinds = map[string]string{
	model.MovementCollection:   model.DirectionIn,
	model.MovementIncome:       model.DirectionIn,
	model.MovementRefund:       model.DirectionOut,
	model.MovementExpense:      model.DirectionOut,
	model.MovementStaffPayment: model.DirectionOut,
}

// lockedAccount is an account row locked for the current transaction.
type lockedAccount = db.LockCashAccountsRow

// Post writes one module movement in its own transaction.
func (s *Service) Post(ctx context.Context, orgID int64, in PostInput) (uuid.UUID, error) {
	var out uuid.UUID
	err := s.repo.InTx(ctx, func(q *db.Queries) error {
		var err error
		out, err = s.PostTx(ctx, q, orgID, in)
		return err
	})
	return out, err
}

// PostTx writes one module movement inside the caller's transaction (q),
// so the module record and its money movement commit together.
func (s *Service) PostTx(ctx context.Context, q *db.Queries, orgID int64, in PostInput) (uuid.UUID, error) {
	direction, ok := moduleKinds[in.Kind]
	if !ok {
		return uuid.Nil, invalid("kind", "invalid", "kind is not a module posting kind")
	}
	if in.IdempotencyKey != "" {
		prev, err := q.GetCashMovementByIdempotencyKey(ctx, db.GetCashMovementByIdempotencyKeyParams{
			OrganizationID: orgID, IdempotencyKey: pgtype.Text{String: in.IdempotencyKey, Valid: true},
		})
		if err == nil {
			if prev.Kind != in.Kind || !money.FromNumericOrZero(prev.Amount).Equal(in.Amount) {
				return uuid.Nil, ErrIdempotencyReused
			}
			return prev.Uuid, nil
		}
		if !errors.Is(repository.MapErr(err), repository.ErrNotFound) {
			return uuid.Nil, err
		}
	}
	row, err := s.loadAccount(ctx, q, orgID, in.AccountUUID)
	if err != nil {
		return uuid.Nil, err
	}
	date := in.Date
	if date.IsZero() {
		date = s.Today(ctx)
	}
	id, err := s.writeMovement(ctx, q, orgID, row.ID, movementRow{
		Kind: in.Kind, Direction: direction, Amount: in.Amount, Date: date, Description: in.Description,
		SourceType: in.SourceType, SourceUUID: in.SourceUUID, IdempotencyKey: in.IdempotencyKey, CreatedBy: in.CreatedBy,
	})
	if errors.Is(err, repository.ErrIdempotencyKey) {
		return uuid.Nil, ErrIdempotencyReused
	}
	return id, err
}

// ReverseModuleTx posts, inside the caller's transaction, the exact
// opposite of a movement another module wrote through PostTx (a voided
// collection, a refund taken back). It is dated today and follows the same
// rules as any movement (open day, till floor). The posting module has
// authorized the caller already, so the branch scope is not checked here.
func (s *Service) ReverseModuleTx(ctx context.Context, q *db.Queries, orgID int64, movementID uuid.UUID, reason string, userID *int64) (uuid.UUID, error) {
	mv, err := q.GetCashMovement(ctx, db.GetCashMovementParams{OrganizationID: orgID, Uuid: movementID})
	if err != nil {
		if errors.Is(repository.MapErr(err), repository.ErrNotFound) {
			return uuid.Nil, ErrMovementNotFound
		}
		return uuid.Nil, err
	}
	if _, ok := moduleKinds[mv.Kind]; !ok {
		return uuid.Nil, ErrNotReversible
	}
	if mv.ReversedByUuid.Valid {
		return uuid.Nil, ErrAlreadyReversed
	}
	reason = strings.TrimSpace(reason)
	if utf8.RuneCountInString(reason) > maxDescriptionLength {
		reason = string([]rune(reason)[:maxDescriptionLength])
	}
	locked, err := s.lock(ctx, q, orgID, mv.CashAccountID)
	if err != nil {
		return uuid.Nil, err
	}
	acc := locked[mv.CashAccountID]
	date := s.Today(ctx)
	if err := s.checkOpenDay(ctx, q, orgID, acc, date); err != nil {
		return uuid.Nil, err
	}
	dir := opposite(mv.Direction)
	amount := money.FromNumericOrZero(mv.Amount)
	if err := s.checkFunds(ctx, q, orgID, acc, dir, amount); err != nil {
		return uuid.Nil, err
	}
	reverses := mv.ID
	return s.insert(ctx, q, orgID, acc, movementRow{
		Kind: model.MovementReversal, Direction: dir, Amount: amount, Date: date, Description: reason,
		ReversesID: &reverses, SourceType: mv.SourceType, SourceUUID: uuidPtr(mv.SourceUuid), CreatedBy: userID,
	})
}

// CreateMovement records a manual money in (deposit) or out (withdrawal).
func (s *Service) CreateMovement(ctx context.Context, orgID int64, accountID uuid.UUID, userID *int64, in model.MovementInput) (model.Movement, error) {
	row, err := s.loadAccount(ctx, s.repo.Q(), orgID, accountID)
	if err != nil {
		return model.Movement{}, err
	}
	ve := &ValidationError{}
	kind := ""
	switch in.Direction {
	case model.DirectionIn:
		kind = model.MovementDeposit
	case model.DirectionOut:
		kind = model.MovementWithdrawal
	default:
		ve.add("direction", "invalid", "direction must be in or out")
	}
	checkAmount(ve, "amount", in.Amount)
	date := s.movementDate(ve, "movement_date", in.MovementDate, ctx)
	desc := checkDescription(ve, "description", in.Description, true)
	if err := ve.orNil(); err != nil {
		return model.Movement{}, err
	}
	var id uuid.UUID
	err = s.repo.InTx(ctx, func(q *db.Queries) error {
		var err error
		id, err = s.writeMovement(ctx, q, orgID, row.ID, movementRow{
			Kind: kind, Direction: in.Direction, Amount: in.Amount, Date: date, Description: desc, CreatedBy: userID,
		})
		return err
	})
	if err != nil {
		return model.Movement{}, err
	}
	return s.GetMovement(ctx, orgID, id)
}

// Transfer moves money between two accounts of the organization: a
// transfer_out on the source and a transfer_in on the target, written in
// one transaction with a shared transfer group. A transfer is not revenue.
func (s *Service) Transfer(ctx context.Context, orgID int64, userID *int64, in model.TransferInput) (model.Transfer, error) {
	q := s.repo.Q()
	ve := &ValidationError{}
	var from, to db.GetCashAccountRow
	for _, side := range []struct {
		field string
		id    uuid.UUID
		dst   *db.GetCashAccountRow
	}{{"from_account_uuid", in.FromAccountUUID, &from}, {"to_account_uuid", in.ToAccountUUID, &to}} {
		if side.id == uuid.Nil {
			ve.add(side.field, "required", side.field+" is required")
			continue
		}
		row, err := s.loadAccount(ctx, q, orgID, side.id)
		if errors.Is(err, ErrNotFound) {
			ve.add(side.field, "not_found", "account not found")
			continue
		}
		if err != nil {
			return model.Transfer{}, err
		}
		*side.dst = row
	}
	if in.FromAccountUUID != uuid.Nil && in.FromAccountUUID == in.ToAccountUUID {
		ve.add("to_account_uuid", "same_account", "the target must be another account")
	}
	checkAmount(ve, "amount", in.Amount)
	date := s.movementDate(ve, "movement_date", in.MovementDate, ctx)
	desc := checkDescription(ve, "description", in.Description, false)
	if err := ve.orNil(); err != nil {
		return model.Transfer{}, err
	}
	if from.Currency != to.Currency {
		return model.Transfer{}, ErrCurrencyMismatch
	}
	group := uuid.New()
	var outID, inID uuid.UUID
	err := s.repo.InTx(ctx, func(q *db.Queries) error {
		locked, err := s.lock(ctx, q, orgID, from.ID, to.ID)
		if err != nil {
			return err
		}
		src, dst := locked[from.ID], locked[to.ID]
		for _, a := range []lockedAccount{src, dst} {
			if err := s.checkWritable(ctx, q, orgID, a, date); err != nil {
				return err
			}
		}
		if err := s.checkFunds(ctx, q, orgID, src, model.DirectionOut, in.Amount); err != nil {
			return err
		}
		counter := pgtype.Int8{Int64: to.ID, Valid: true}
		outID, err = s.insert(ctx, q, orgID, src, movementRow{
			Kind: model.MovementTransferOut, Direction: model.DirectionOut, Amount: in.Amount, Date: date,
			Description: desc, TransferGroup: &group, CounterAccountID: counter, CreatedBy: userID,
		})
		if err != nil {
			return err
		}
		inID, err = s.insert(ctx, q, orgID, dst, movementRow{
			Kind: model.MovementTransferIn, Direction: model.DirectionIn, Amount: in.Amount, Date: date,
			Description: desc, TransferGroup: &group, CounterAccountID: pgtype.Int8{Int64: from.ID, Valid: true},
			CreatedBy: userID,
		})
		return err
	})
	if err != nil {
		return model.Transfer{}, err
	}
	outMv, err := s.GetMovement(ctx, orgID, outID)
	if err != nil {
		return model.Transfer{}, err
	}
	inMv, err := s.GetMovement(ctx, orgID, inID)
	if err != nil {
		return model.Transfer{}, err
	}
	return model.Transfer{TransferGroup: group, Out: outMv, In: inMv}, nil
}

// Reverse posts the exact opposite of a manual movement (both legs of a
// transfer) dated today. Movements of other modules are reversed through
// those modules; a closing difference through reopening the day.
func (s *Service) Reverse(ctx context.Context, orgID int64, id uuid.UUID, userID *int64, in model.ReverseInput) ([]model.Movement, error) {
	q := s.repo.Q()
	mv, err := s.loadMovement(ctx, q, orgID, id)
	if err != nil {
		return nil, err
	}
	reason := strings.TrimSpace(in.Reason)
	if utf8.RuneCountInString(reason) < minReasonLength {
		return nil, invalid("reason", "required", "a reason of at least 3 characters is required")
	}
	if utf8.RuneCountInString(reason) > maxDescriptionLength {
		return nil, invalid("reason", "too_long", "reason is too long")
	}
	if mv.ReversedByUuid.Valid {
		return nil, ErrAlreadyReversed
	}
	if !slices.Contains([]string{model.MovementDeposit, model.MovementWithdrawal, model.MovementTransferIn, model.MovementTransferOut}, mv.Kind) {
		return nil, ErrNotReversible
	}
	type leg struct {
		id, accountID int64
		direction     string
		amount        money.Money
	}
	legs := []leg{{mv.ID, mv.CashAccountID, mv.Direction, money.FromNumericOrZero(mv.Amount)}}
	if mv.TransferGroup.Valid {
		rows, err := q.ListCashTransferLegs(ctx, db.ListCashTransferLegsParams{OrganizationID: orgID, TransferGroup: mv.TransferGroup})
		if err != nil {
			return nil, err
		}
		legs = legs[:0]
		for _, r := range rows {
			// The other leg must be inside the caller's scope too.
			acc, err := q.GetCashAccountByID(ctx, db.GetCashAccountByIDParams{OrganizationID: orgID, ID: r.CashAccountID})
			if err != nil {
				return nil, err
			}
			if !allowsBranch(ctx, acc.BranchID) {
				return nil, ErrNotReversible
			}
			legs = append(legs, leg{r.ID, r.CashAccountID, r.Direction, money.FromNumericOrZero(r.Amount)})
		}
	}
	date := s.Today(ctx)
	var ids []uuid.UUID
	err = s.repo.InTx(ctx, func(q *db.Queries) error {
		accIDs := make([]int64, 0, len(legs))
		for _, l := range legs {
			accIDs = append(accIDs, l.accountID)
		}
		locked, err := s.lock(ctx, q, orgID, accIDs...)
		if err != nil {
			return err
		}
		for _, l := range legs {
			acc := locked[l.accountID]
			if err := s.checkOpenDay(ctx, q, orgID, acc, date); err != nil {
				return err
			}
			dir := opposite(l.direction)
			if err := s.checkFunds(ctx, q, orgID, acc, dir, l.amount); err != nil {
				return err
			}
			reverses := l.id
			newID, err := s.insert(ctx, q, orgID, acc, movementRow{
				Kind: model.MovementReversal, Direction: dir, Amount: l.amount, Date: date, Description: reason,
				ReversesID: &reverses, CreatedBy: userID,
			})
			if err != nil {
				return err
			}
			ids = append(ids, newID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]model.Movement, 0, len(ids))
	for _, id := range ids {
		m, err := s.GetMovement(ctx, orgID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// GetMovement returns one movement.
func (s *Service) GetMovement(ctx context.Context, orgID int64, id uuid.UUID) (model.Movement, error) {
	row, err := s.loadMovement(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return model.Movement{}, err
	}
	return MapMovement(row), nil
}

type movementArgs struct {
	scope, accounts  []int64
	kinds            []string
	direction        pgtype.Text
	dateFrom, dateTo pgtype.Date
	q                pgtype.Text
}

func (s *Service) movementArgs(ctx context.Context, q *db.Queries, orgID int64, f model.MovementFilter) (movementArgs, error) {
	a := movementArgs{
		scope: orgctx.BranchScope(ctx), kinds: nilIfEmpty(f.Kinds), direction: textArg(f.Direction),
		dateFrom: datePtrArg(f.DateFrom), dateTo: datePtrArg(f.DateTo), q: textArg(f.Q),
	}
	if len(f.Accounts) > 0 {
		a.accounts = []int64{}
		for _, id := range f.Accounts {
			row, err := s.loadAccount(ctx, q, orgID, id)
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if err != nil {
				return a, err
			}
			a.accounts = append(a.accounts, row.ID)
		}
	}
	return a, nil
}

// ListMovements returns one page of the organization's movements.
func (s *Service) ListMovements(ctx context.Context, orgID int64, f model.MovementFilter) ([]model.Movement, int64, error) {
	if _, ok := MovementSortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "", false
	}
	q := s.repo.Q()
	a, err := s.movementArgs(ctx, q, orgID, f)
	if err != nil {
		return nil, 0, err
	}
	rows, err := q.ListCashMovements(ctx, db.ListCashMovementsParams{
		OrganizationID: orgID, BranchIds: a.scope, AccountIds: a.accounts, Kinds: a.kinds, Direction: a.direction,
		DateFrom: a.dateFrom, DateTo: a.dateTo, Q: a.q, SortField: f.SortField, SortDesc: f.SortDesc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountCashMovements(ctx, db.CountCashMovementsParams{
		OrganizationID: orgID, BranchIds: a.scope, AccountIds: a.accounts, Kinds: a.kinds, Direction: a.direction,
		DateFrom: a.dateFrom, DateTo: a.dateTo, Q: a.q,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Movement, 0, len(rows))
	for _, row := range rows {
		out = append(out, MapMovement(db.GetCashMovementRow(row)))
	}
	return out, total, nil
}

// ExportMovements returns every movement matching a filter.
func (s *Service) ExportMovements(ctx context.Context, orgID int64, f model.MovementFilter) ([]model.Movement, error) {
	f.Limit, f.Offset = 100000, 0
	items, _, err := s.ListMovements(ctx, orgID, f)
	return items, err
}

// MovementFacets returns the kinds and accounts in use.
func (s *Service) MovementFacets(ctx context.Context, orgID int64) (model.MovementFacets, error) {
	out := model.MovementFacets{Kind: []model.Facet{}, Account: []model.Facet{}}
	q := s.repo.Q()
	kinds, err := q.ListCashMovementKindFacets(ctx, db.ListCashMovementKindFacetsParams{OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx)})
	if err != nil {
		return out, err
	}
	for _, row := range kinds {
		out.Kind = append(out.Kind, model.Facet{Value: row.Value, Count: row.Count})
	}
	opts, err := s.AccountOptions(ctx, orgID)
	if err != nil {
		return out, err
	}
	for _, o := range opts {
		out.Account = append(out.Account, model.Facet{Value: o.UUID.String(), Label: o.Name})
	}
	return out, nil
}

// ListAccountMovements pages the movements of one account with the
// balance after each one.
func (s *Service) ListAccountMovements(ctx context.Context, orgID int64, accountID uuid.UUID, f model.MovementFilter) ([]model.Movement, int64, error) {
	q := s.repo.Q()
	acc, err := s.loadAccount(ctx, q, orgID, accountID)
	if err != nil {
		return nil, 0, err
	}
	rows, err := q.ListCashAccountMovements(ctx, db.ListCashAccountMovementsParams{
		OrganizationID: orgID, CashAccountID: acc.ID, DateFrom: datePtrArg(f.DateFrom), DateTo: datePtrArg(f.DateTo),
		Kinds: nilIfEmpty(f.Kinds), LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountCashAccountMovements(ctx, db.CountCashAccountMovementsParams{
		OrganizationID: orgID, CashAccountID: acc.ID, DateFrom: datePtrArg(f.DateFrom), DateTo: datePtrArg(f.DateTo),
		Kinds: nilIfEmpty(f.Kinds),
	})
	if err != nil {
		return nil, 0, err
	}
	opening := money.FromNumericOrZero(acc.OpeningBalance)
	ref := model.AccountRef{UUID: acc.Uuid, Name: acc.Name, Kind: acc.Kind}
	out := make([]model.Movement, 0, len(rows))
	for _, row := range rows {
		m := model.Movement{
			UUID: row.Uuid, Account: ref, Kind: row.Kind, Direction: row.Direction,
			Amount: money.FromNumericOrZero(row.Amount), Currency: row.Currency,
			MovementDate: formatDate(row.MovementDate.Time), Description: row.Description,
			TransferGroup: uuidPtr(row.TransferGroup), ReversesUUID: uuidPtr(row.ReversesUuid),
			ReversedByUUID: uuidPtr(row.ReversedByUuid), ClosingUUID: uuidPtr(row.ClosingUuid),
			SourceType: row.SourceType, SourceUUID: uuidPtr(row.SourceUuid), CreatedAt: row.CreatedAt.Time,
		}
		if row.CounterAccountUuid.Valid {
			m.CounterAccount = &model.AccountRef{UUID: uuid.UUID(row.CounterAccountUuid.Bytes), Name: row.CounterAccountName.String}
		}
		if row.CreatorUuid.Valid {
			m.CreatedBy = &model.UserRef{UUID: uuid.UUID(row.CreatorUuid.Bytes), Name: row.CreatorName.String, Surname: row.CreatorSurname.String}
		}
		after := opening.Add(money.FromNumericOrZero(row.RunningTotal))
		m.BalanceAfter = &after
		out = append(out, m)
	}
	return out, total, nil
}

// loadMovement reads one movement; movements of an account outside the
// caller's scope read as not found.
func (s *Service) loadMovement(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (db.GetCashMovementRow, error) {
	row, err := q.GetCashMovement(ctx, db.GetCashMovementParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if errors.Is(repository.MapErr(err), repository.ErrNotFound) {
			return row, ErrMovementNotFound
		}
		return row, err
	}
	if !allowsBranch(ctx, row.BranchID) {
		return db.GetCashMovementRow{}, ErrMovementNotFound
	}
	return row, nil
}

// movementRow is one movement to insert.
type movementRow struct {
	Kind, Direction  string
	Amount           money.Money
	Date             time.Time
	Description      string
	TransferGroup    *uuid.UUID
	CounterAccountID pgtype.Int8
	ReversesID       *int64
	ClosingID        *int64
	SourceType       string
	SourceUUID       *uuid.UUID
	IdempotencyKey   string
	CreatedBy        *int64
}

// writeMovement locks the account, checks it and inserts one movement.
func (s *Service) writeMovement(ctx context.Context, q *db.Queries, orgID, accountID int64, m movementRow) (uuid.UUID, error) {
	if !m.Amount.IsPositive() {
		return uuid.Nil, invalid("amount", "invalid", "amount must be greater than zero")
	}
	locked, err := s.lock(ctx, q, orgID, accountID)
	if err != nil {
		return uuid.Nil, err
	}
	acc := locked[accountID]
	if err := s.checkWritable(ctx, q, orgID, acc, m.Date); err != nil {
		return uuid.Nil, err
	}
	if err := s.checkFunds(ctx, q, orgID, acc, m.Direction, m.Amount); err != nil {
		return uuid.Nil, err
	}
	return s.insert(ctx, q, orgID, acc, m)
}

func (s *Service) insert(ctx context.Context, q *db.Queries, orgID int64, acc lockedAccount, m movementRow) (uuid.UUID, error) {
	p := db.InsertCashMovementParams{
		OrganizationID: orgID, CashAccountID: acc.ID, Kind: m.Kind, Direction: m.Direction,
		Amount: m.Amount.Numeric(), Currency: acc.Currency, MovementDate: dateArg(m.Date),
		Description: m.Description, BranchID: acc.BranchID, CounterAccountID: m.CounterAccountID,
		SourceType: m.SourceType, CreatedBy: int8Ptr(m.CreatedBy),
	}
	if m.TransferGroup != nil {
		p.TransferGroup = pgtype.UUID{Bytes: *m.TransferGroup, Valid: true}
	}
	if m.ReversesID != nil {
		p.ReversesMovementID = pgtype.Int8{Int64: *m.ReversesID, Valid: true}
	}
	if m.ClosingID != nil {
		p.ClosingID = pgtype.Int8{Int64: *m.ClosingID, Valid: true}
	}
	if m.SourceUUID != nil {
		p.SourceUuid = pgtype.UUID{Bytes: *m.SourceUUID, Valid: true}
	}
	if m.IdempotencyKey != "" {
		p.IdempotencyKey = pgtype.Text{String: m.IdempotencyKey, Valid: true}
	}
	row, err := q.InsertCashMovement(ctx, p)
	if err != nil {
		mapped := repository.MapErr(err)
		if errors.Is(mapped, repository.ErrIdempotencyKey) {
			return uuid.Nil, mapped
		}
		return uuid.Nil, mapRepoErr(err)
	}
	return row.Uuid, nil
}

// lock locks the accounts (id order) for the transaction.
func (s *Service) lock(ctx context.Context, q *db.Queries, orgID int64, ids ...int64) (map[int64]lockedAccount, error) {
	rows, err := q.LockCashAccounts(ctx, db.LockCashAccountsParams{OrganizationID: orgID, Ids: ids})
	if err != nil {
		return nil, err
	}
	out := make(map[int64]lockedAccount, len(rows))
	for _, r := range rows {
		out[r.ID] = r
	}
	for _, id := range ids {
		if _, ok := out[id]; !ok {
			return nil, ErrNotFound
		}
	}
	return out, nil
}

// checkWritable: the account is active, the date is not before its
// opening, not in the future and not on a closed day.
func (s *Service) checkWritable(ctx context.Context, q *db.Queries, orgID int64, acc lockedAccount, date time.Time) error {
	if acc.Status != model.StatusActive {
		return ErrAccountArchived
	}
	if date.Before(acc.OpeningDate.Time) {
		return invalid("movement_date", "before_opening", "the date is before the opening date of the account")
	}
	if date.After(s.Today(ctx)) {
		return invalid("movement_date", "future", "the date cannot be in the future")
	}
	return s.checkOpenDay(ctx, q, orgID, acc, date)
}

// checkOpenDay refuses a date on or before the latest closed day.
func (s *Service) checkOpenDay(ctx context.Context, q *db.Queries, orgID int64, acc lockedAccount, date time.Time) error {
	last, err := q.LatestCashClosingDate(ctx, db.LatestCashClosingDateParams{OrganizationID: orgID, CashAccountID: acc.ID})
	if err != nil {
		return err
	}
	if last.Valid && !date.After(last.Time) {
		return ErrDayClosed
	}
	return nil
}

// checkFunds refuses money out that would take a till below zero. Bank and
// online accounts may go negative (overdraft, chargeback).
func (s *Service) checkFunds(ctx context.Context, q *db.Queries, orgID int64, acc lockedAccount, direction string, amount money.Money) error {
	if acc.Kind != model.KindCash || direction != model.DirectionOut {
		return nil
	}
	row, err := q.GetCashAccount(ctx, db.GetCashAccountParams{OrganizationID: orgID, Uuid: acc.Uuid})
	if err != nil {
		return err
	}
	if money.FromNumericOrZero(row.Balance).Sub(amount).IsNegative() {
		return ErrInsufficientFunds
	}
	return nil
}

func (s *Service) movementDate(ve *ValidationError, field string, raw *string, ctx context.Context) time.Time {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return s.Today(ctx)
	}
	d, err := parseDate(*raw)
	if err != nil {
		ve.add(field, "invalid", field+" must be YYYY-MM-DD")
		return time.Time{}
	}
	return d
}

func checkAmount(ve *ValidationError, field string, m money.Money) {
	if !m.IsPositive() {
		ve.add(field, "invalid", field+" must be greater than zero")
	}
}

func checkDescription(ve *ValidationError, field, v string, required bool) string {
	v = strings.TrimSpace(v)
	switch {
	case required && v == "":
		ve.add(field, "required", field+" is required")
	case utf8.RuneCountInString(v) > maxDescriptionLength:
		ve.add(field, "too_long", field+" is too long")
	}
	return v
}

func opposite(direction string) string {
	if direction == model.DirectionIn {
		return model.DirectionOut
	}
	return model.DirectionIn
}

func datePtrArg(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

// MapMovement maps a movement row (detail, list and export).
func MapMovement(row db.GetCashMovementRow) model.Movement {
	m := model.Movement{
		UUID:    row.Uuid,
		Account: model.AccountRef{UUID: row.AccountUuid, Name: row.AccountName, Kind: row.AccountKind},
		Kind:    row.Kind, Direction: row.Direction, Amount: money.FromNumericOrZero(row.Amount),
		Currency: row.Currency, MovementDate: formatDate(row.MovementDate.Time), Description: row.Description,
		TransferGroup: uuidPtr(row.TransferGroup), ReversesUUID: uuidPtr(row.ReversesUuid),
		ReversedByUUID: uuidPtr(row.ReversedByUuid), ClosingUUID: uuidPtr(row.ClosingUuid),
		SourceType: row.SourceType, SourceUUID: uuidPtr(row.SourceUuid), CreatedAt: row.CreatedAt.Time,
	}
	if row.CounterAccountUuid.Valid {
		m.CounterAccount = &model.AccountRef{UUID: uuid.UUID(row.CounterAccountUuid.Bytes), Name: row.CounterAccountName.String}
	}
	if row.CreatorUuid.Valid {
		m.CreatedBy = &model.UserRef{UUID: uuid.UUID(row.CreatorUuid.Bytes), Name: row.CreatorName.String, Surname: row.CreatorSurname.String}
	}
	return m
}
