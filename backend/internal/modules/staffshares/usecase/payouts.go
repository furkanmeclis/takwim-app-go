package usecase

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	cashmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/model"
	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// PayoutSortColumns are the sortable payout list fields.
var PayoutSortColumns = map[string]string{
	"payout_date": "payout_date", "amount": "amount", "created_at": "created_at",
}

// ListPayouts returns one page of the payouts from the accounts inside the
// caller's branch scope (an own-only viewer sees its own).
func (s *Service) ListPayouts(ctx context.Context, orgID int64, v Viewer, f model.PayoutFilter) ([]model.Payout, int64, error) {
	if _, ok := PayoutSortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "", false
	}
	q := s.repo.Q()
	own, err := s.ownStaff(ctx, q, orgID, v)
	if err != nil {
		if errors.Is(err, ErrNoStaffRecord) {
			return []model.Payout{}, 0, nil
		}
		return nil, 0, err
	}
	staff, err := s.staffIDs(ctx, q, orgID, f.Staff, own)
	if err != nil {
		return nil, 0, err
	}
	branches, err := s.branchIDs(ctx, q, orgID, f.Branches)
	if err != nil {
		return nil, 0, err
	}
	limited, scope := scopeArgs(ctx)
	if own != 0 {
		// A member's own payouts, whatever account they came from.
		limited, scope = false, []int64{}
	}
	rows, err := q.ListSharePayouts(ctx, db.ListSharePayoutsParams{
		OrganizationID: orgID, Limited: limited, BranchIds: scope, FilterBranchIds: branches, StaffIds: staff,
		Kinds: nilIfEmpty(f.Kinds), DateFrom: datePtrArg(f.DateFrom), DateTo: datePtrArg(f.DateTo), Q: textArg(f.Q),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountSharePayouts(ctx, db.CountSharePayoutsParams{
		OrganizationID: orgID, Limited: limited, BranchIds: scope, FilterBranchIds: branches, StaffIds: staff,
		Kinds: nilIfEmpty(f.Kinds), DateFrom: datePtrArg(f.DateFrom), DateTo: datePtrArg(f.DateTo), Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Payout, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapPayout(db.GetSharePayoutRow(r)))
	}
	return out, total, nil
}

// GetPayout returns one payout inside the caller's scope.
func (s *Service) GetPayout(ctx context.Context, orgID int64, v Viewer, id uuid.UUID) (model.Payout, error) {
	q := s.repo.Q()
	row, err := q.GetSharePayout(ctx, db.GetSharePayoutParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if isNotFound(err) {
			return model.Payout{}, ErrPayoutNotFound
		}
		return model.Payout{}, err
	}
	own, err := s.ownStaff(ctx, q, orgID, v)
	switch {
	case err != nil:
		return model.Payout{}, ErrPayoutNotFound
	case own != 0:
		if own != row.StaffID {
			return model.Payout{}, ErrPayoutNotFound
		}
	case !allowsBranch(ctx, row.BranchID):
		return model.Payout{}, ErrPayoutNotFound
	}
	return mapPayout(row), nil
}

// Pay records money paid to a staff member from a cash or bank account:
// a staff_payment movement of the cash book and the payout, in one
// transaction. It is not an expense. The amount may not exceed what is
// payable now (shares of collected sales minus earlier payouts).
func (s *Service) Pay(ctx context.Context, orgID int64, userID *int64, in model.PayoutInput) (model.Payout, error) {
	ve := &ValidationError{}
	if !in.Amount.IsPositive() {
		ve.add("amount", "invalid", "amount must be positive")
	} else if in.Amount.Minor() > money.MaxMinor {
		ve.add("amount", "too_large", "amount is too large")
	}
	note := checkNote(ve, "note", in.Note)
	key := strings.TrimSpace(in.IdempotencyKey)
	if utf8.RuneCountInString(key) > maxIdempotencyKey {
		ve.add("idempotency_key", "too_long", "idempotency_key is too long")
	}
	date := s.today(ctx)
	if in.Date != nil && strings.TrimSpace(*in.Date) != "" {
		d, err := parseDate(*in.Date)
		switch {
		case err != nil:
			ve.add("date", "invalid", "date must be YYYY-MM-DD")
		case d.After(date):
			ve.add("date", "future", "a payout cannot be dated in the future")
		default:
			date = d
		}
	}
	if err := ve.orNil(); err != nil {
		return model.Payout{}, err
	}
	var payoutID uuid.UUID
	err := s.repo.InTx(ctx, func(q *db.Queries) error {
		if key != "" {
			prev, err := q.GetSharePayoutByIdempotencyKey(ctx, db.GetSharePayoutByIdempotencyKeyParams{
				OrganizationID: orgID, IdempotencyKey: textArg(key),
			})
			if err == nil {
				st, serr := q.GetShareStaffByUUID(ctx, db.GetShareStaffByUUIDParams{OrganizationID: orgID, Uuid: in.StaffUUID})
				if serr != nil || st.ID != prev.StaffID || !amountOf(prev.Amount).Equal(in.Amount) {
					return ErrIdempotencyReused
				}
				payoutID = prev.Uuid
				return nil
			}
			if !isNotFound(err) {
				return err
			}
		}
		ve := &ValidationError{}
		st, err := q.GetShareStaffByUUID(ctx, db.GetShareStaffByUUIDParams{OrganizationID: orgID, Uuid: in.StaffUUID})
		if err != nil {
			if !isNotFound(err) {
				return err
			}
			ve.add("staff_uuid", "not_found", "staff member not found")
		}
		acc, err := q.GetCashAccount(ctx, db.GetCashAccountParams{OrganizationID: orgID, Uuid: in.CashAccountUUID})
		if err != nil {
			if !isNotFound(err) {
				return err
			}
			ve.add("cash_account_uuid", "not_found", "cash account not found")
		} else if !allowsBranch(ctx, acc.BranchID) {
			ve.add("cash_account_uuid", "not_found", "cash account not found")
		}
		if err := ve.orNil(); err != nil {
			return err
		}
		if _, err := q.LockShareStaff(ctx, db.LockShareStaffParams{OrganizationID: orgID, ID: st.ID}); err != nil {
			return err
		}
		payable, err := payableNow(ctx, q, orgID, st.ID, acc.Currency)
		if err != nil {
			return err
		}
		if in.Amount.Cmp(payable) > 0 {
			return ErrPayoutTooLarge
		}
		payoutID = uuid.New()
		movementKey := payoutIdempotency + payoutID.String()
		if key != "" {
			movementKey = payoutIdempotency + key
		}
		description := payoutDescription + ": " + st.Name
		if note != "" {
			description += " (" + note + ")"
		}
		mv, err := s.cash.PostTx(ctx, q, orgID, cashusecase.PostInput{
			AccountUUID: acc.Uuid, Kind: cashmodel.MovementStaffPayment, Amount: in.Amount, Date: date,
			Description: description, SourceType: payoutSourceType, SourceUUID: &payoutID,
			IdempotencyKey: movementKey, CreatedBy: userID,
		})
		if err != nil {
			return err
		}
		movement, err := q.GetSalesCashMovement(ctx, db.GetSalesCashMovementParams{OrganizationID: orgID, Uuid: mv})
		if err != nil {
			return err
		}
		_, err = q.InsertSharePayout(ctx, db.InsertSharePayoutParams{
			Uuid: payoutID, OrganizationID: orgID, StaffID: st.ID, BranchID: acc.BranchID, CashAccountID: acc.ID,
			CashMovementID: movement.ID, Kind: model.PayoutPayout, Amount: in.Amount.Numeric(), Currency: acc.Currency,
			PayoutDate: dateArg(date), Note: note, IdempotencyKey: textArg(key), CreatedBy: int8Ptr(userID),
		})
		return mapRepoErr(err)
	})
	if err != nil {
		return model.Payout{}, err
	}
	return s.GetPayout(ctx, orgID, Viewer{}, payoutID)
}

// ReversePayout undoes a payout posted by mistake: the reversal of its cash
// movement (dated today) and a reversal payout; the share becomes payable
// again. The old payout stays.
func (s *Service) ReversePayout(ctx context.Context, orgID int64, id uuid.UUID, userID *int64, in model.ReverseInput) (model.Payout, error) {
	q := s.repo.Q()
	row, err := q.GetSharePayoutRow(ctx, db.GetSharePayoutRowParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if isNotFound(err) {
			return model.Payout{}, ErrPayoutNotFound
		}
		return model.Payout{}, err
	}
	if !allowsBranch(ctx, row.BranchID) {
		return model.Payout{}, ErrPayoutNotFound
	}
	reason := strings.TrimSpace(in.Reason)
	switch n := utf8.RuneCountInString(reason); {
	case n < minReasonLength:
		return model.Payout{}, invalid("reason", "required", "a reason of at least 3 characters is required")
	case n > maxNoteLength:
		return model.Payout{}, invalid("reason", "too_long", "reason is too long")
	}
	if row.Kind != model.PayoutPayout {
		return model.Payout{}, ErrNotReversible
	}
	if row.Reversed {
		return model.Payout{}, ErrAlreadyReversed
	}
	revID := uuid.New()
	err = s.repo.InTx(ctx, func(q *db.Queries) error {
		mv, err := s.cash.ReverseModuleTx(ctx, q, orgID, row.CashMovementUuid, reversalPrefixNote+reason, userID)
		if err != nil {
			return err
		}
		movement, err := q.GetSalesCashMovement(ctx, db.GetSalesCashMovementParams{OrganizationID: orgID, Uuid: mv})
		if err != nil {
			return err
		}
		_, err = q.InsertSharePayout(ctx, db.InsertSharePayoutParams{
			Uuid: revID, OrganizationID: orgID, StaffID: row.StaffID, BranchID: row.BranchID,
			CashAccountID: row.CashAccountID, CashMovementID: movement.ID, Kind: model.PayoutReversal,
			ReversesPayoutID: pgtype.Int8{Int64: row.ID, Valid: true}, Amount: row.Amount, Currency: row.Currency,
			PayoutDate: dateArg(s.today(ctx)), Note: reason, CreatedBy: int8Ptr(userID),
		})
		return mapRepoErr(err)
	})
	if err != nil {
		if errors.Is(err, cashusecase.ErrAlreadyReversed) {
			return model.Payout{}, ErrAlreadyReversed
		}
		return model.Payout{}, err
	}
	return s.GetPayout(ctx, orgID, Viewer{}, revID)
}

func mapPayout(r db.GetSharePayoutRow) model.Payout {
	p := model.Payout{
		UUID: r.Uuid, Kind: r.Kind, Staff: model.Ref{UUID: r.StaffUuid, Name: r.StaffName}, Amount: amountOf(r.Amount),
		Currency: r.Currency, PayoutDate: formatDate(r.PayoutDate.Time), Note: r.Note,
		CashAccount: model.Ref{UUID: r.CashAccountUuid, Name: r.CashAccountName}, CashMovementUUID: r.CashMovementUuid,
		ReversesUUID: uuidPtr(r.ReversesUuid), ReversedByUUID: uuidPtr(r.ReversedByUuid), CreatedAt: r.CreatedAt.Time,
		CreatedBy: userRef(r.CreatorUuid, r.CreatorName, r.CreatorSurname),
	}
	if r.BranchUuid.Valid {
		p.Branch = &model.Ref{UUID: uuid.UUID(r.BranchUuid.Bytes), Name: r.BranchName.String}
	}
	return p
}
