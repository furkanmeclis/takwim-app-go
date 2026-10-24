package usecase

import (
	"context"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// SourceCustomerPackage names the ledger entries of a sold session
// package (TEC-920).
const SourceCustomerPackage = "customer_package"

// PackageCharge charges a customer for a session package sold in a branch
// (TEC-920). It is a "package" debit: what the customer owes for the
// package, never revenue (the sessions become revenue when they are used).
type PackageCharge struct {
	BranchID    int64
	CustomerID  int64
	Amount      money.Money
	Currency    string
	Date        time.Time
	PackageUUID uuid.UUID
	Description string
	CreatedBy   *int64
}

// RecordPackageChargeTx writes the package debit inside the caller's
// transaction and returns its id. A package of zero posts nothing (0).
func (s *Service) RecordPackageChargeTx(ctx context.Context, q *db.Queries, orgID int64, in PackageCharge) (int64, error) {
	if !in.Amount.IsPositive() {
		return 0, nil
	}
	row, err := q.InsertLedgerEntry(ctx, db.InsertLedgerEntryParams{
		OrganizationID: orgID, BranchID: in.BranchID, CustomerID: int8Of(in.CustomerID), Kind: model.EntryPackage,
		Direction: model.Debit, Amount: in.Amount.Numeric(), Currency: in.Currency, EntryDate: dateArg(in.Date),
		SourceType: SourceCustomerPackage, SourceUuid: pgtype.UUID{Bytes: in.PackageUUID, Valid: true},
		Description: clip(in.Description, maxReasonLength), CreatedBy: int8Ptr(in.CreatedBy),
	})
	if err != nil {
		return 0, mapRepoErr(err)
	}
	return row.ID, nil
}

// ReversePackageChargeTx reverses a package debit (a refunded package)
// inside the caller's transaction, dated date.
func (s *Service) ReversePackageChargeTx(ctx context.Context, q *db.Queries, orgID, entryID int64, reason string, date time.Time, userID *int64) error {
	e, err := q.GetLedgerEntryRowByID(ctx, db.GetLedgerEntryRowByIDParams{OrganizationID: orgID, ID: entryID})
	if err != nil {
		if isNotFound(err) {
			return ErrEntryNotFound
		}
		return err
	}
	if e.Kind != model.EntryPackage {
		return ErrNotReversible
	}
	if e.Reversed {
		return ErrAlreadyReversed
	}
	_, err = q.InsertLedgerEntry(ctx, db.InsertLedgerEntryParams{
		OrganizationID: orgID, BranchID: e.BranchID, CustomerID: e.CustomerID, Kind: model.EntryReversal,
		Direction: model.Credit, Amount: e.Amount, Currency: e.Currency, EntryDate: dateArg(date),
		ReversesEntryID: int8Of(e.ID), SourceType: e.SourceType, SourceUuid: e.SourceUuid,
		Description: clip(reason, maxReasonLength), CreatedBy: int8Ptr(userID),
	})
	return mapRepoErr(err)
}

// EntryID returns the internal id of a ledger entry (the packages module
// keeps it on the sold package).
func (s *Service) EntryID(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (int64, error) {
	row, err := q.GetLedgerEntryRow(ctx, db.GetLedgerEntryRowParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if isNotFound(err) {
			return 0, ErrEntryNotFound
		}
		return 0, err
	}
	return row.ID, nil
}
