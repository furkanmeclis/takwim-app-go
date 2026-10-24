package usecase

import (
	"context"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	cashmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/model"
	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// DocumentPath is the API path that streams the document of a record.
func DocumentPath(entry uuid.UUID) string {
	return "/v1/tenant/finance/entries/" + entry.String() + "/document"
}

// GetEntry returns one record of the business inside the caller's scope.
func (s *Service) GetEntry(ctx context.Context, orgID int64, id uuid.UUID) (model.Entry, error) {
	row, err := s.loadEntry(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return model.Entry{}, err
	}
	return mapEntry(row), nil
}

func (s *Service) loadEntry(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (db.GetFinanceEntryRow, error) {
	row, err := q.GetFinanceEntry(ctx, db.GetFinanceEntryParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if isNotFound(err) {
			return row, ErrEntryNotFound
		}
		return row, err
	}
	if !allowsBranch(ctx, row.BranchID) {
		return db.GetFinanceEntryRow{}, ErrEntryNotFound
	}
	return row, nil
}

// resolved are the checked references of a record to write.
type resolved struct {
	kind       string
	categoryID int64
	category   string
	account    db.GetCashAccountRow
	branchID   pgtype.Int8
	staffID    pgtype.Int8
	date       time.Time
}

// resolveCategory loads an active category of the kind.
func resolveCategory(ctx context.Context, q *db.Queries, orgID int64, ve *ValidationError, id uuid.UUID, kind string, active bool) (int64, string, error) {
	if id == uuid.Nil {
		ve.add("category_uuid", "required", "category_uuid is required")
		return 0, "", nil
	}
	c, err := q.GetFinanceCategory(ctx, db.GetFinanceCategoryParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if isNotFound(err) {
			ve.add("category_uuid", "not_found", "category not found")
			return 0, "", nil
		}
		return 0, "", err
	}
	switch {
	case kind != "" && c.Kind != kind:
		ve.add("category_uuid", "kind_mismatch", "the category belongs to the other kind")
	case active && c.Status != model.StatusActive:
		ve.add("category_uuid", "archived", "the category is archived")
	}
	return c.ID, c.Name, nil
}

// resolveAccount loads a cash account inside the caller's scope and checks
// that it may pay for the branch: an account of the same branch, or a
// business-wide one. Without a branch the record takes the account's.
func resolveAccount(ctx context.Context, q *db.Queries, orgID int64, ve *ValidationError, id uuid.UUID, branch pgtype.Int8, branchGiven bool) (db.GetCashAccountRow, pgtype.Int8, error) {
	if id == uuid.Nil {
		ve.add("cash_account_uuid", "required", "cash_account_uuid is required")
		return db.GetCashAccountRow{}, branch, nil
	}
	acc, err := q.GetCashAccount(ctx, db.GetCashAccountParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if isNotFound(err) {
			ve.add("cash_account_uuid", "not_found", "cash account not found")
			return acc, branch, nil
		}
		return acc, branch, err
	}
	if !allowsBranch(ctx, acc.BranchID) {
		ve.add("cash_account_uuid", "not_found", "cash account not found")
		return acc, branch, nil
	}
	if !branchGiven {
		return acc, acc.BranchID, nil
	}
	if acc.BranchID.Valid && (!branch.Valid || branch.Int64 != acc.BranchID.Int64) {
		ve.add("cash_account_uuid", "branch_mismatch", "the account belongs to another branch")
	}
	return acc, branch, nil
}

func resolveStaff(ctx context.Context, q *db.Queries, orgID int64, ve *ValidationError, id *uuid.UUID) (pgtype.Int8, error) {
	if id == nil || *id == uuid.Nil {
		return pgtype.Int8{}, nil
	}
	st, err := q.GetFinanceStaffByUUID(ctx, db.GetFinanceStaffByUUIDParams{OrganizationID: orgID, Uuid: *id})
	if err != nil {
		if isNotFound(err) {
			ve.add("staff_uuid", "not_found", "staff member not found")
			return pgtype.Int8{}, nil
		}
		return pgtype.Int8{}, err
	}
	return pgtype.Int8{Int64: st.ID, Valid: true}, nil
}

// resolveBranchField resolves an optional branch_uuid. A missing branch is
// business-wide only for members with every branch; the account's branch
// is taken otherwise.
func resolveBranchField(ctx context.Context, q *db.Queries, orgID int64, ve *ValidationError, id *uuid.UUID) (pgtype.Int8, bool, error) {
	if id == nil || *id == uuid.Nil {
		return pgtype.Int8{}, false, nil
	}
	bid, ok, err := lookupBranch(ctx, q, orgID, *id)
	if err != nil {
		return pgtype.Int8{}, true, err
	}
	if !ok {
		ve.add("branch_uuid", "not_found", "branch not found")
		return pgtype.Int8{}, true, nil
	}
	return pgtype.Int8{Int64: bid, Valid: true}, true, nil
}

func (s *Service) checkDate(ctx context.Context, ve *ValidationError, raw *string, fallback time.Time) time.Time {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return fallback
	}
	d, err := parseDate(*raw)
	switch {
	case err != nil:
		ve.add("entry_date", "invalid", "entry_date must be YYYY-MM-DD")
	case d.After(s.today(ctx)):
		ve.add("entry_date", "future", "a record cannot be dated in the future")
	default:
		return d
	}
	return fallback
}

func checkAmount(ve *ValidationError, m money.Money) {
	switch {
	case !m.IsPositive():
		ve.add("amount", "invalid", "amount must be greater than zero")
	case m.Minor() > money.MaxMinor:
		ve.add("amount", "too_large", "amount is too large")
	}
}

// CreateEntry records money in or out outside sales and posts its cash
// movement in the same transaction. The idempotency key makes a double
// submit write once; the same key with another body is refused.
func (s *Service) CreateEntry(ctx context.Context, orgID int64, userID *int64, in model.EntryInput) (model.Entry, error) {
	ve := &ValidationError{}
	if !slices.Contains(model.Kinds, in.Kind) {
		ve.add("kind", "invalid", "kind must be income or expense")
	}
	checkAmount(ve, in.Amount)
	desc := checkText(ve, "description", in.Description, maxDescription)
	key := strings.TrimSpace(in.IdempotencyKey)
	if utf8.RuneCountInString(key) > maxIdempotencyKey {
		ve.add("idempotency_key", "too_long", "idempotency_key is too long")
	}
	date := s.checkDate(ctx, ve, in.EntryDate, s.today(ctx))
	if err := ve.orNil(); err != nil {
		return model.Entry{}, err
	}
	var entryID uuid.UUID
	err := s.repo.InTx(ctx, func(q *db.Queries) error {
		if key != "" {
			prev, err := q.GetFinanceEntryByIdempotencyKey(ctx, db.GetFinanceEntryByIdempotencyKeyParams{
				OrganizationID: orgID, IdempotencyKey: textArg(key),
			})
			if err == nil {
				acc, aerr := q.GetCashAccount(ctx, db.GetCashAccountParams{OrganizationID: orgID, Uuid: in.CashAccountUUID})
				if aerr != nil || acc.ID != prev.CashAccountID || prev.Kind != in.Kind ||
					!money.FromNumericOrZero(prev.Amount).Equal(in.Amount) {
					return ErrIdempotencyReused
				}
				entryID = prev.Uuid
				return nil
			}
			if !isNotFound(err) {
				return err
			}
		}
		r, err := s.resolveNew(ctx, q, orgID, in.Kind, in.CategoryUUID, in.CashAccountUUID, in.BranchUUID, in.StaffUUID, date, uuid.Nil)
		if err != nil {
			return err
		}
		entryID = uuid.New()
		movementKey := idempotencyPrefix + entryID.String()
		if key != "" {
			movementKey = idempotencyPrefix + key
		}
		_, err = s.insertEntry(ctx, q, orgID, userID, entryID, r, in.Amount, desc, key, movementKey, nil, nil)
		return err
	})
	if err != nil {
		return model.Entry{}, err
	}
	return s.GetEntry(ctx, orgID, entryID)
}

// resolveNew checks the references of a new record (create / correction).
// An archived category is refused unless the record keeps it (keep).
func (s *Service) resolveNew(ctx context.Context, q *db.Queries, orgID int64, kind string, category, account uuid.UUID, branch, staff *uuid.UUID, date time.Time, keep uuid.UUID) (resolved, error) {
	ve := &ValidationError{}
	r := resolved{kind: kind, date: date}
	var err error
	if r.categoryID, r.category, err = resolveCategory(ctx, q, orgID, ve, category, kind, category != keep); err != nil {
		return r, err
	}
	branchID, given, err := resolveBranchField(ctx, q, orgID, ve, branch)
	if err != nil {
		return r, err
	}
	if r.account, r.branchID, err = resolveAccount(ctx, q, orgID, ve, account, branchID, given); err != nil {
		return r, err
	}
	if len(ve.Details) == 0 && !allowsBranch(ctx, r.branchID) {
		ve.add("branch_uuid", "required", "branch_uuid is required")
	}
	if r.staffID, err = resolveStaff(ctx, q, orgID, ve, staff); err != nil {
		return r, err
	}
	return r, ve.orNil()
}

// insertEntry posts the cash movement of a record and writes the record.
func (s *Service) insertEntry(ctx context.Context, q *db.Queries, orgID int64, userID *int64, id uuid.UUID, r resolved,
	amount money.Money, desc, key, movementKey string, replaces *int64, doc *db.GetFinanceEntryRow) (int64, error) {
	movementKind := cashmodel.MovementExpense
	if r.kind == model.KindIncome {
		movementKind = cashmodel.MovementIncome
	}
	description := r.category
	if desc != "" {
		description += ": " + desc
	}
	if utf8.RuneCountInString(description) > maxDescription {
		description = string([]rune(description)[:maxDescription])
	}
	mv, err := s.cash.PostTx(ctx, q, orgID, cashusecase.PostInput{
		AccountUUID: r.account.Uuid, Kind: movementKind, Amount: amount, Date: r.date, Description: description,
		SourceType: movementSourceType, SourceUUID: &id, IdempotencyKey: movementKey, CreatedBy: userID,
	})
	if err != nil {
		return 0, mapCashErr(err)
	}
	movement, err := q.GetFinanceCashMovement(ctx, db.GetFinanceCashMovementParams{OrganizationID: orgID, Uuid: mv})
	if err != nil {
		return 0, err
	}
	p := db.InsertFinanceEntryParams{
		Uuid: id, OrganizationID: orgID, EntryType: model.TypeEntry, Kind: r.kind, CategoryID: r.categoryID,
		BranchID: r.branchID, StaffID: r.staffID, CashAccountID: r.account.ID, CashMovementID: movement.ID,
		Amount: amount.Numeric(), Currency: r.account.Currency, EntryDate: dateArg(r.date), Description: desc,
		IdempotencyKey: textArg(key), CreatedBy: int8Ptr(userID),
	}
	if replaces != nil {
		p.ReplacesEntryID = pgtype.Int8{Int64: *replaces, Valid: true}
	}
	if doc != nil && doc.DocumentKey.Valid {
		// The replacement keeps the document of the record it corrects.
		p.DocumentKey, p.DocumentName = doc.DocumentKey, doc.DocumentName
		p.DocumentContentType, p.DocumentSizeBytes = doc.DocumentContentType, doc.DocumentSizeBytes
	}
	row, err := q.InsertFinanceEntry(ctx, p)
	if err != nil {
		return 0, mapRepoErr(err)
	}
	return row.ID, nil
}

// PatchEntry changes the classification of a live record (category of the
// same kind, description, staff member). It returns the record before and
// after the change for the activity log.
func (s *Service) PatchEntry(ctx context.Context, orgID int64, id uuid.UUID, in model.EntryPatch) (model.Entry, model.Entry, error) {
	q := s.repo.Q()
	row, err := s.loadEntry(ctx, q, orgID, id)
	if err != nil {
		return model.Entry{}, model.Entry{}, err
	}
	if row.EntryType != model.TypeEntry {
		return model.Entry{}, model.Entry{}, ErrNotReversible
	}
	if row.ReversedByUuid.Valid {
		return model.Entry{}, model.Entry{}, ErrAlreadyReversed
	}
	ve := &ValidationError{}
	categoryID, desc, staffID := row.CategoryID, row.Description, row.StaffID
	if in.CategoryUUID != nil && *in.CategoryUUID != row.CategoryUuid {
		if categoryID, _, err = resolveCategory(ctx, q, orgID, ve, *in.CategoryUUID, row.Kind, true); err != nil {
			return model.Entry{}, model.Entry{}, err
		}
	}
	if in.Description != nil {
		desc = checkText(ve, "description", *in.Description, maxDescription)
	}
	if in.StaffUUID.Set {
		if staffID, err = resolveStaff(ctx, q, orgID, ve, in.StaffUUID.Value); err != nil {
			return model.Entry{}, model.Entry{}, err
		}
	}
	if err := ve.orNil(); err != nil {
		return model.Entry{}, model.Entry{}, err
	}
	if _, err := q.UpdateFinanceEntryClassification(ctx, db.UpdateFinanceEntryClassificationParams{
		OrganizationID: orgID, ID: row.ID, CategoryID: categoryID, Description: desc, StaffID: staffID,
	}); err != nil {
		return model.Entry{}, model.Entry{}, mapRepoErr(err)
	}
	after, err := s.GetEntry(ctx, orgID, id)
	return mapEntry(row), after, err
}

// ReverseEntry undoes a record posted by mistake: the reversal of its cash
// movement and a reversal record (the exact negative), both dated today.
// The record stays and shows the reversal.
func (s *Service) ReverseEntry(ctx context.Context, orgID int64, id uuid.UUID, userID *int64, in model.ReverseInput) (model.Entry, error) {
	q := s.repo.Q()
	row, err := s.loadEntry(ctx, q, orgID, id)
	if err != nil {
		return model.Entry{}, err
	}
	reason, err := checkReason(in.Reason)
	if err != nil {
		return model.Entry{}, err
	}
	var revID uuid.UUID
	err = s.repo.InTx(ctx, func(q *db.Queries) error {
		var err error
		revID, err = s.reverseTx(ctx, q, orgID, row, reason, userID)
		return err
	})
	if err != nil {
		return model.Entry{}, err
	}
	return s.GetEntry(ctx, orgID, revID)
}

// reverseTx writes the reversal of one record inside the caller's
// transaction (row is locked and re-checked first).
func (s *Service) reverseTx(ctx context.Context, q *db.Queries, orgID int64, row db.GetFinanceEntryRow, reason string, userID *int64) (uuid.UUID, error) {
	if row.EntryType != model.TypeEntry {
		return uuid.Nil, ErrNotReversible
	}
	if _, err := q.LockFinanceEntry(ctx, db.LockFinanceEntryParams{OrganizationID: orgID, ID: row.ID}); err != nil {
		return uuid.Nil, err
	}
	reversed, err := q.FinanceEntryReversed(ctx, db.FinanceEntryReversedParams{OrganizationID: orgID, ID: row.ID})
	if err != nil {
		return uuid.Nil, err
	}
	if reversed {
		return uuid.Nil, ErrAlreadyReversed
	}
	mv, err := s.cash.ReverseModuleTx(ctx, q, orgID, row.CashMovementUuid, reason, userID)
	if err != nil {
		return uuid.Nil, mapCashErr(err)
	}
	movement, err := q.GetFinanceCashMovement(ctx, db.GetFinanceCashMovementParams{OrganizationID: orgID, Uuid: mv})
	if err != nil {
		return uuid.Nil, err
	}
	revID := uuid.New()
	_, err = q.InsertFinanceEntry(ctx, db.InsertFinanceEntryParams{
		Uuid: revID, OrganizationID: orgID, EntryType: model.TypeReversal, Kind: row.Kind, CategoryID: row.CategoryID,
		BranchID: row.BranchID, StaffID: row.StaffID, CashAccountID: row.CashAccountID, CashMovementID: movement.ID,
		Amount: money.FromNumericOrZero(row.Amount).Neg().Numeric(), Currency: row.Currency,
		EntryDate: dateArg(s.today(ctx)), Description: row.Description,
		ReversesEntryID: pgtype.Int8{Int64: row.ID, Valid: true}, Reason: reason, CreatedBy: int8Ptr(userID),
	})
	if err != nil {
		return uuid.Nil, mapRepoErr(err)
	}
	return revID, nil
}

// CorrectEntry replaces a record: the old one is reversed (today) and a
// new one is written with the changed values, linked to it, in one
// transaction. Missing values are copied, the document moves along.
func (s *Service) CorrectEntry(ctx context.Context, orgID int64, id uuid.UUID, userID *int64, in model.CorrectInput) (model.Entry, error) {
	q := s.repo.Q()
	row, err := s.loadEntry(ctx, q, orgID, id)
	if err != nil {
		return model.Entry{}, err
	}
	if row.EntryType != model.TypeEntry {
		return model.Entry{}, ErrNotReversible
	}
	if row.ReversedByUuid.Valid {
		return model.Entry{}, ErrAlreadyReversed
	}
	reason, err := checkReason(in.Reason)
	if err != nil {
		return model.Entry{}, err
	}
	ve := &ValidationError{}
	amount := money.FromNumericOrZero(row.Amount)
	if in.Amount != nil {
		amount = *in.Amount
		checkAmount(ve, amount)
	}
	date := s.checkDate(ctx, ve, in.EntryDate, row.EntryDate.Time)
	desc := row.Description
	if in.Description != nil {
		desc = checkText(ve, "description", *in.Description, maxDescription)
	}
	if err := ve.orNil(); err != nil {
		return model.Entry{}, err
	}
	category := row.CategoryUuid
	if in.CategoryUUID != nil {
		category = *in.CategoryUUID
	}
	account := row.AccountUuid
	if in.CashAccountUUID != nil {
		account = *in.CashAccountUUID
	}
	branch := uuidPtr(row.BranchUuid)
	if in.BranchUUID.Set {
		branch = in.BranchUUID.Value
	}
	staff := uuidPtr(row.StaffUuid)
	if in.StaffUUID.Set {
		staff = in.StaffUUID.Value
	}
	var newID uuid.UUID
	err = s.repo.InTx(ctx, func(q *db.Queries) error {
		if _, err := s.reverseTx(ctx, q, orgID, row, correctionReasonNote+reason, userID); err != nil {
			return err
		}
		r, err := s.resolveNew(ctx, q, orgID, row.Kind, category, account, branch, staff, date, row.CategoryUuid)
		if err != nil {
			return err
		}
		if branch == nil && !in.BranchUUID.Set {
			// The record keeps its branch (business-wide stays business-wide)
			// unless the account forces another one.
			r.branchID = row.BranchID
			if r.account.BranchID.Valid && (!r.branchID.Valid || r.branchID.Int64 != r.account.BranchID.Int64) {
				return invalid("cash_account_uuid", "branch_mismatch", "the account belongs to another branch")
			}
		}
		newID = uuid.New()
		replaces := row.ID
		_, err = s.insertEntry(ctx, q, orgID, userID, newID, r, amount, desc, "", idempotencyPrefix+newID.String(), &replaces, &row)
		return err
	})
	if err != nil {
		return model.Entry{}, err
	}
	return s.GetEntry(ctx, orgID, newID)
}

func mapEntry(r db.GetFinanceEntryRow) model.Entry {
	e := model.Entry{
		UUID: r.Uuid, EntryType: r.EntryType, Kind: r.Kind,
		Category: model.Ref{UUID: r.CategoryUuid, Name: r.CategoryName},
		Branch:   ref(r.BranchUuid, r.BranchName), Staff: ref(r.StaffUuid, r.StaffName),
		Account:          model.AccountRef{UUID: r.AccountUuid, Name: r.AccountName, Kind: r.AccountKind},
		CashMovementUUID: r.CashMovementUuid, Amount: money.FromNumericOrZero(r.Amount), Currency: r.Currency,
		EntryDate: formatDate(r.EntryDate.Time), Description: r.Description, Reason: r.Reason,
		ReversesUUID: uuidPtr(r.ReversesUuid), ReversedByUUID: uuidPtr(r.ReversedByUuid),
		ReplacesUUID: uuidPtr(r.ReplacesUuid), ReplacedByUUID: uuidPtr(r.ReplacedByUuid),
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.DocumentKey.Valid {
		e.Document = &model.Document{
			Name: r.DocumentName, ContentType: r.DocumentContentType, SizeBytes: r.DocumentSizeBytes,
			URL: DocumentPath(r.Uuid),
		}
	}
	if r.CreatorUuid.Valid {
		e.CreatedBy = &model.UserRef{UUID: uuid.UUID(r.CreatorUuid.Bytes), Name: r.CreatorName.String, Surname: r.CreatorSurname.String}
	}
	return e
}
