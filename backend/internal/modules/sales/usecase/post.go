package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ReservationKey is the accrual key of one line of a reservation.
func ReservationKey(reservationID int64, lineIndex int32) string {
	return fmt.Sprintf("reservation:%d:line:%d", reservationID, lineIndex)
}

// StockMovementKey is the accrual key of a stock sale.
func StockMovementKey(movement uuid.UUID) string {
	return "stock_movement:" + movement.String()
}

// RecordReservationTx writes the sale lines of a completed visit inside the
// caller's transaction (the reservation's completion): one line per
// reservation line at the price booked, with its staff member, plus the
// customer's ledger debit. The business day is the day the visit ended,
// never later than today (a visit closed early counts today). Lines that
// already have a live sale line are skipped, so running it again changes
// nothing. Products handed over with the visit (TEC-919: one stock sale
// movement each) become product lines of the visit, keyed by their
// movement like a counter sale, so a product is never revenue twice.
func (s *Service) RecordReservationTx(ctx context.Context, q *db.Queries, orgID, reservationID, actorID int64) error {
	r, err := q.GetSalesReservation(ctx, db.GetSalesReservationParams{OrganizationID: orgID, ID: reservationID})
	if err != nil {
		if isNotFound(err) {
			return ErrReservationMissing
		}
		return err
	}
	c, err := s.clockOf(ctx, q, orgID, r.BranchID)
	if err != nil {
		return err
	}
	when := s.now()
	if r.EndsAt.Valid && r.EndsAt.Time.Before(when) {
		when = r.EndsAt.Time
	}
	day := c.day(when)
	lines, err := q.ListSalesReservationLines(ctx, db.ListSalesReservationLinesParams{OrganizationID: orgID, ReservationID: r.ID})
	if err != nil {
		return err
	}
	actor := int8Of(actorID)
	if actorID == 0 {
		actor = pgtype.Int8{}
	}
	for _, l := range lines {
		key := ReservationKey(r.ID, l.LineIndex)
		if _, err := q.GetLiveSaleLineByKey(ctx, db.GetLiveSaleLineByKeyParams{OrganizationID: orgID, AccrualKey: key}); err == nil {
			continue
		} else if !isNotFound(err) {
			return err
		}
		price := amountOf(l.UnitPrice)
		idx := pgtype.Int4{Int32: l.LineIndex, Valid: true}
		row, err := q.InsertSaleLine(ctx, db.InsertSaleLineParams{
			OrganizationID: orgID, BranchID: r.BranchID, CustomerID: int8Of(r.CustomerID), Kind: model.LineSale,
			ItemType: model.ItemService, SourceType: model.SourceReservation, SourceUuid: pgtype.UUID{Bytes: r.Uuid, Valid: true},
			ReservationID: int8Of(r.ID), LineIndex: idx, ServiceID: l.ServiceID, StaffID: l.StaffID,
			Description: clip(l.ServiceName, maxDescriptionLength), Quantity: 1, UnitPrice: price.Numeric(),
			GrossAmount: price.Numeric(), DiscountAmount: money.Zero().Numeric(), NetAmount: price.Numeric(),
			Currency: r.Currency, SaleDate: dateArg(day), AccrualKey: key, CreatedBy: actor,
		})
		if err != nil {
			return mapRepoErr(err)
		}
		if err := s.postSaleEntry(ctx, q, orgID, row.ID); err != nil {
			return err
		}
	}
	products, err := q.ListSalesReservationProducts(ctx, db.ListSalesReservationProductsParams{OrganizationID: orgID, ReservationID: r.ID})
	if err != nil {
		return err
	}
	customer, reservation := r.CustomerID, r.ID
	for _, p := range products {
		name := p.ProductName
		if p.VariantName.Valid && p.VariantName.String != "" {
			name += " / " + p.VariantName.String
		}
		sale := ProductSale{
			MovementUUID: p.StockMovementUuid, BranchID: r.BranchID, ProductID: p.ProductID,
			CustomerID: &customer, ReservationID: &reservation, Description: name, Quantity: p.Quantity,
			UnitPrice: amountOf(p.UnitPrice), OccurredAt: when,
		}
		if p.VariantID.Valid {
			v := p.VariantID.Int64
			sale.VariantID = &v
		}
		if p.StaffID.Valid {
			st := p.StaffID.Int64
			sale.StaffID = &st
		}
		if actor.Valid {
			a := actor.Int64
			sale.CreatedBy = &a
		}
		if _, err := s.RecordProductSaleTx(ctx, q, orgID, sale); err != nil {
			return err
		}
	}
	return nil
}

// ProductSale is a product handed over at the counter (a stock sale).
type ProductSale struct {
	MovementUUID uuid.UUID
	BranchID     int64
	ProductID    int64
	VariantID    *int64
	StaffID      *int64
	CustomerID   *int64
	// ReservationID: the visit the product was handed over with (its
	// balance counts the line).
	ReservationID *int64
	Description   string
	Quantity      int32
	UnitPrice     money.Money
	OccurredAt    time.Time
	CreatedBy     *int64
}

// RecordProductSaleTx writes the sale line of a stock sale inside the
// caller's transaction (the stock movement), plus its ledger debit. A
// movement is posted once.
func (s *Service) RecordProductSaleTx(ctx context.Context, q *db.Queries, orgID int64, in ProductSale) (uuid.UUID, error) {
	key := StockMovementKey(in.MovementUUID)
	if live, err := q.GetLiveSaleLineByKey(ctx, db.GetLiveSaleLineByKeyParams{OrganizationID: orgID, AccrualKey: key}); err == nil {
		return live.Uuid, nil
	} else if !isNotFound(err) {
		return uuid.Nil, err
	}
	if in.Quantity < 1 || in.UnitPrice.IsNegative() {
		return uuid.Nil, invalid("quantity", "invalid", "a sale needs a quantity and a price")
	}
	c, err := s.clockOf(ctx, q, orgID, in.BranchID)
	if err != nil {
		return uuid.Nil, err
	}
	at := in.OccurredAt
	if at.IsZero() {
		at = s.now()
	}
	gross := in.UnitPrice.Mul(int64(in.Quantity))
	row, err := q.InsertSaleLine(ctx, db.InsertSaleLineParams{
		OrganizationID: orgID, BranchID: in.BranchID, CustomerID: int8Ptr(in.CustomerID), Kind: model.LineSale,
		ItemType: model.ItemProduct, SourceType: model.SourceStockMovement,
		SourceUuid: pgtype.UUID{Bytes: in.MovementUUID, Valid: true}, ReservationID: int8Ptr(in.ReservationID),
		ProductID: int8Of(in.ProductID),
		VariantID: int8Ptr(in.VariantID), StaffID: int8Ptr(in.StaffID), Description: clip(in.Description, maxDescriptionLength),
		Quantity: in.Quantity, UnitPrice: in.UnitPrice.Numeric(), GrossAmount: gross.Numeric(),
		DiscountAmount: money.Zero().Numeric(), NetAmount: gross.Numeric(), Currency: c.currency,
		SaleDate: dateArg(c.day(at)), AccrualKey: key, CreatedBy: int8Ptr(in.CreatedBy),
	})
	if err != nil {
		return uuid.Nil, mapRepoErr(err)
	}
	return row.Uuid, s.postSaleEntry(ctx, q, orgID, row.ID)
}

// postSaleEntry posts the ledger debit of a sale line (nothing for a line
// of zero) and runs the line hook.
func (s *Service) postSaleEntry(ctx context.Context, q *db.Queries, orgID, lineID int64) error {
	if err := s.afterLine(ctx, q, orgID, lineID); err != nil {
		return err
	}
	line, err := q.GetSaleLineRowByID(ctx, db.GetSaleLineRowByIDParams{OrganizationID: orgID, ID: lineID})
	if err != nil {
		return err
	}
	net := amountOf(line.NetAmount)
	if !net.IsPositive() {
		return nil
	}
	_, err = q.InsertLedgerEntry(ctx, db.InsertLedgerEntryParams{
		OrganizationID: orgID, BranchID: line.BranchID, CustomerID: line.CustomerID, ReservationID: line.ReservationID,
		Kind: model.EntrySale, Direction: model.Debit, Amount: line.NetAmount, Currency: line.Currency,
		EntryDate: line.SaleDate, SaleLineID: int8Of(line.ID), SourceType: line.SourceType, SourceUuid: line.SourceUuid,
		Description: line.Description, CreatedBy: line.CreatedBy,
	})
	return mapRepoErr(err)
}

// lineRow is the stored sale line (with its reversed flag).
type lineRow = db.GetSaleLineRowRow

// reverseLineTx writes the reversal of a live sale line dated day, and the
// reversal of its ledger debit.
func (s *Service) reverseLineTx(ctx context.Context, q *db.Queries, orgID int64, line lineRow, reason string, day time.Time, userID *int64) (db.InsertSaleLineRow, error) {
	if line.Kind != model.LineSale {
		return db.InsertSaleLineRow{}, ErrNotReversible
	}
	if line.Reversed {
		return db.InsertSaleLineRow{}, ErrAlreadyReversed
	}
	neg := func(n pgtype.Numeric) pgtype.Numeric { return amountOf(n).Neg().Numeric() }
	rev, err := q.InsertSaleLine(ctx, db.InsertSaleLineParams{
		OrganizationID: orgID, BranchID: line.BranchID, CustomerID: line.CustomerID, Kind: model.LineReversal,
		ItemType: line.ItemType, SourceType: line.SourceType, SourceUuid: line.SourceUuid, ReservationID: line.ReservationID,
		LineIndex: line.LineIndex, ServiceID: line.ServiceID, ProductID: line.ProductID, VariantID: line.VariantID,
		StaffID: line.StaffID, Description: line.Description, Quantity: line.Quantity, UnitPrice: line.UnitPrice,
		GrossAmount: neg(line.GrossAmount), DiscountAmount: neg(line.DiscountAmount), NetAmount: neg(line.NetAmount),
		Currency: line.Currency, SaleDate: dateArg(day), AccrualKey: line.AccrualKey,
		ReversesLineID: int8Of(line.ID), Reason: reason, CreatedBy: int8Ptr(userID),
	})
	if err != nil {
		return rev, mapRepoErr(err)
	}
	if err := s.afterLine(ctx, q, orgID, rev.ID); err != nil {
		return rev, err
	}
	entry, err := q.GetLedgerEntryBySaleLine(ctx, db.GetLedgerEntryBySaleLineParams{OrganizationID: orgID, SaleLineID: int8Of(line.ID)})
	if err != nil {
		if isNotFound(err) {
			// A line of zero has no debit.
			return rev, nil
		}
		return rev, err
	}
	_, err = q.InsertLedgerEntry(ctx, db.InsertLedgerEntryParams{
		OrganizationID: orgID, BranchID: line.BranchID, CustomerID: line.CustomerID, ReservationID: line.ReservationID,
		Kind: model.EntryReversal, Direction: model.Credit, Amount: line.NetAmount, Currency: line.Currency,
		EntryDate: dateArg(day), SaleLineID: int8Of(rev.ID), ReversesEntryID: int8Of(entry.ID),
		SourceType: line.SourceType, SourceUuid: line.SourceUuid, Description: reason, CreatedBy: int8Ptr(userID),
	})
	return rev, mapRepoErr(err)
}

// ReverseLineTx reverses a live sale line inside the caller's transaction
// (a returned product, a refunded service: TEC-913), dated today in the
// line's branch. The branch scope is not checked.
func (s *Service) ReverseLineTx(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID, reason string, userID *int64) (uuid.UUID, error) {
	line, err := q.GetSaleLineRow(ctx, db.GetSaleLineRowParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if isNotFound(err) {
			return uuid.Nil, ErrLineNotFound
		}
		return uuid.Nil, err
	}
	c, err := s.clockOf(ctx, q, orgID, line.BranchID)
	if err != nil {
		return uuid.Nil, err
	}
	rev, err := s.reverseLineTx(ctx, q, orgID, line, clip(reason, maxReasonLength), c.day(s.now()), userID)
	return rev.Uuid, err
}

// LiveLineOfStockSale returns the live sale line of a stock movement.
func (s *Service) LiveLineOfStockSale(ctx context.Context, q *db.Queries, orgID int64, movement uuid.UUID) (uuid.UUID, error) {
	row, err := q.GetLiveSaleLineByKey(ctx, db.GetLiveSaleLineByKeyParams{OrganizationID: orgID, AccrualKey: StockMovementKey(movement)})
	if err != nil {
		if isNotFound(err) {
			return uuid.Nil, ErrLineNotFound
		}
		return uuid.Nil, err
	}
	return row.Uuid, nil
}
