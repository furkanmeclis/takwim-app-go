package usecase

import (
	"context"
	"fmt"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// LineSortColumns are the sortable sale line list fields.
var LineSortColumns = map[string]string{
	"sale_date": "sale_date", "net_amount": "net_amount", "kind": "kind", "created_at": "created_at",
}

// GetLine returns one sale line inside the caller's branch scope.
func (s *Service) GetLine(ctx context.Context, orgID int64, id uuid.UUID) (model.SaleLine, error) {
	row, err := s.repo.Q().GetSaleLine(ctx, db.GetSaleLineParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if isNotFound(err) {
			return model.SaleLine{}, ErrLineNotFound
		}
		return model.SaleLine{}, err
	}
	if orgctx.CheckBranch(ctx, row.BranchID) != nil {
		return model.SaleLine{}, ErrLineNotFound
	}
	return MapLine(row), nil
}

// lineArgs resolves the uuids of a filter inside the organization. An
// unknown uuid matches nothing.
type lineArgs struct {
	branches, staff       []int64
	reservation, customer pgtype.Int8
	empty                 bool
}

func (s *Service) resolveFilter(ctx context.Context, q *db.Queries, orgID int64, branches []uuid.UUID, reservation, customer *uuid.UUID, staff []uuid.UUID) (lineArgs, error) {
	var a lineArgs
	if len(branches) > 0 {
		rows, err := q.ListSalesBranchesByUUID(ctx, db.ListSalesBranchesByUUIDParams{OrganizationID: orgID, Uuids: branches})
		if err != nil {
			return a, err
		}
		a.branches = []int64{}
		for _, r := range rows {
			a.branches = append(a.branches, r.ID)
		}
	}
	if reservation != nil {
		r, err := q.GetSalesReservationByUUID(ctx, db.GetSalesReservationByUUIDParams{OrganizationID: orgID, Uuid: *reservation})
		switch {
		case err == nil:
			a.reservation = int8Of(r.ID)
		case isNotFound(err):
			a.empty = true
		default:
			return a, err
		}
	}
	if customer != nil {
		c, err := q.GetSalesCustomerByUUID(ctx, db.GetSalesCustomerByUUIDParams{OrganizationID: orgID, Uuid: *customer})
		switch {
		case err == nil:
			a.customer = int8Of(c.ID)
		case isNotFound(err):
			a.empty = true
		default:
			return a, err
		}
	}
	if len(staff) > 0 {
		rows, err := q.GetStaffIDsByUUIDs(ctx, db.GetStaffIDsByUUIDsParams{OrganizationID: orgID, Uuids: staff})
		if err != nil {
			return a, err
		}
		a.staff = []int64{}
		for _, r := range rows {
			a.staff = append(a.staff, r.ID)
		}
	}
	return a, nil
}

// ListLines returns one page of the sale lines of the caller's scope.
func (s *Service) ListLines(ctx context.Context, orgID int64, f model.LineFilter) ([]model.SaleLine, int64, error) {
	if _, ok := LineSortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "", false
	}
	q := s.repo.Q()
	a, err := s.resolveFilter(ctx, q, orgID, f.Branches, f.ReservationUUID, f.CustomerUUID, f.Staff)
	if err != nil {
		return nil, 0, err
	}
	if a.empty {
		return []model.SaleLine{}, 0, nil
	}
	scope := orgctx.BranchScope(ctx)
	rows, err := q.ListSaleLines(ctx, db.ListSaleLinesParams{
		OrganizationID: orgID, BranchIds: scope, FilterBranchIds: a.branches, Kinds: nilIfEmpty(f.Kinds),
		ItemTypes: nilIfEmpty(f.ItemTypes), ReservationID: a.reservation, CustomerID: a.customer, StaffIds: a.staff,
		DateFrom: datePtrArg(f.DateFrom), DateTo: datePtrArg(f.DateTo), Q: textArg(f.Q),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountSaleLines(ctx, db.CountSaleLinesParams{
		OrganizationID: orgID, BranchIds: scope, FilterBranchIds: a.branches, Kinds: nilIfEmpty(f.Kinds),
		ItemTypes: nilIfEmpty(f.ItemTypes), ReservationID: a.reservation, CustomerID: a.customer, StaffIds: a.staff,
		DateFrom: datePtrArg(f.DateFrom), DateTo: datePtrArg(f.DateTo), Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.SaleLine, 0, len(rows))
	for _, r := range rows {
		out = append(out, MapLine(db.GetSaleLineRow(r)))
	}
	return out, total, nil
}

// loadLine reads a stored sale line inside the caller's scope.
func (s *Service) loadLine(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (lineRow, error) {
	row, err := q.GetSaleLineRow(ctx, db.GetSaleLineRowParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if isNotFound(err) {
			return row, ErrLineNotFound
		}
		return row, err
	}
	if orgctx.CheckBranch(ctx, row.BranchID) != nil {
		return lineRow{}, ErrLineNotFound
	}
	return row, nil
}

// ReverseLine reverses a live sale line (the service was not delivered,
// the product not handed over): the exact negative dated today, and the
// reversal of its ledger debit. The old line stays.
func (s *Service) ReverseLine(ctx context.Context, orgID int64, id uuid.UUID, userID *int64, in model.ReverseInput) (model.SaleLine, error) {
	line, err := s.loadLine(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return model.SaleLine{}, err
	}
	reason, err := checkReason(in.Reason)
	if err != nil {
		return model.SaleLine{}, err
	}
	var revID uuid.UUID
	err = s.repo.InTx(ctx, func(q *db.Queries) error {
		c, err := s.clockOf(ctx, q, orgID, line.BranchID)
		if err != nil {
			return err
		}
		rev, err := s.reverseLineTx(ctx, q, orgID, line, reason, c.day(s.now()), userID)
		revID = rev.Uuid
		return err
	})
	if err != nil {
		return model.SaleLine{}, err
	}
	return s.GetLine(ctx, orgID, revID)
}

// CorrectLine corrects the quantity, unit price or discount of a live sale
// line: the old line is reversed and a replacement for the same item is
// written, both dated today, with their ledger postings. Earlier days keep
// their figures; the difference shows on the day of the correction.
func (s *Service) CorrectLine(ctx context.Context, orgID int64, id uuid.UUID, userID *int64, in model.CorrectInput) (model.Correction, error) {
	line, err := s.loadLine(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return model.Correction{}, err
	}
	reason, err := checkReason(in.Reason)
	if err != nil {
		return model.Correction{}, err
	}
	if line.Kind != model.LineSale {
		return model.Correction{}, ErrNotReversible
	}
	if line.Reversed {
		return model.Correction{}, ErrAlreadyReversed
	}
	ve := &ValidationError{}
	qty := line.Quantity
	if in.Quantity != nil {
		if *in.Quantity < 1 || *in.Quantity > maxQuantity {
			ve.add("quantity", "invalid", fmt.Sprintf("quantity must be between 1 and %d", maxQuantity))
		} else {
			qty = int32(*in.Quantity)
		}
	}
	unit := amountOf(line.UnitPrice)
	if in.UnitPrice != nil {
		if in.UnitPrice.IsNegative() {
			ve.add("unit_price", "invalid", "unit_price must not be negative")
		} else {
			unit = *in.UnitPrice
		}
	}
	discount := amountOf(line.DiscountAmount)
	if in.DiscountAmount != nil {
		discount = *in.DiscountAmount
	}
	gross := unit.Mul(int64(qty))
	if discount.IsNegative() || discount.Cmp(gross) > 0 {
		ve.add("discount_amount", "invalid", "discount_amount must be between 0 and the line total")
	}
	if gross.Minor() > money.MaxMinor {
		ve.add("unit_price", "too_large", "the line total is too large")
	}
	if err := ve.orNil(); err != nil {
		return model.Correction{}, err
	}
	if qty == line.Quantity && unit.Equal(amountOf(line.UnitPrice)) && discount.Equal(amountOf(line.DiscountAmount)) {
		return model.Correction{}, ErrNothingChanged
	}
	var revID, newID uuid.UUID
	err = s.repo.InTx(ctx, func(q *db.Queries) error {
		c, err := s.clockOf(ctx, q, orgID, line.BranchID)
		if err != nil {
			return err
		}
		day := c.day(s.now())
		rev, err := s.reverseLineTx(ctx, q, orgID, line, reason, day, userID)
		if err != nil {
			return err
		}
		revID = rev.Uuid
		row, err := q.InsertSaleLine(ctx, db.InsertSaleLineParams{
			OrganizationID: orgID, BranchID: line.BranchID, CustomerID: line.CustomerID, Kind: model.LineSale,
			ItemType: line.ItemType, SourceType: line.SourceType, SourceUuid: line.SourceUuid,
			ReservationID: line.ReservationID, LineIndex: line.LineIndex, ServiceID: line.ServiceID,
			ProductID: line.ProductID, VariantID: line.VariantID, StaffID: line.StaffID, Description: line.Description,
			Quantity: qty, UnitPrice: unit.Numeric(), GrossAmount: gross.Numeric(), DiscountAmount: discount.Numeric(),
			NetAmount: gross.Sub(discount).Numeric(), Currency: line.Currency, SaleDate: dateArg(day),
			AccrualKey: line.AccrualKey, ReplacesLineID: int8Of(line.ID), Reason: reason, CreatedBy: int8Ptr(userID),
		})
		if err != nil {
			return mapRepoErr(err)
		}
		newID = row.Uuid
		return s.postSaleEntry(ctx, q, orgID, row.ID)
	})
	if err != nil {
		return model.Correction{}, err
	}
	rev, err := s.GetLine(ctx, orgID, revID)
	if err != nil {
		return model.Correction{}, err
	}
	repl, err := s.GetLine(ctx, orgID, newID)
	if err != nil {
		return model.Correction{}, err
	}
	return model.Correction{Reversal: rev, Replacement: repl}, nil
}

// MapLine maps a sale line row (detail and list).
func MapLine(r db.GetSaleLineRow) model.SaleLine {
	l := model.SaleLine{
		UUID: r.Uuid, Kind: r.Kind, ItemType: r.ItemType, SourceType: r.SourceType, SourceUUID: uuidPtr(r.SourceUuid),
		Branch: model.Ref{UUID: r.BranchUuid, Name: r.BranchName}, ServiceUUID: uuidPtr(r.ServiceUuid),
		ProductUUID: uuidPtr(r.ProductUuid), VariantUUID: uuidPtr(r.VariantUuid), Description: r.Description,
		Quantity: int(r.Quantity), UnitPrice: amountOf(r.UnitPrice), GrossAmount: amountOf(r.GrossAmount),
		DiscountAmount: amountOf(r.DiscountAmount), NetAmount: amountOf(r.NetAmount), Currency: r.Currency,
		SaleDate: formatDate(r.SaleDate.Time), Generation: int(r.AccrualGeneration),
		ReversesUUID: uuidPtr(r.ReversesUuid), ReversedByUUID: uuidPtr(r.ReversedByUuid),
		ReplacesUUID: uuidPtr(r.ReplacesUuid), ReplacedByUUID: uuidPtr(r.ReplacedByUuid), Reason: r.Reason,
		CreatedAt: r.CreatedAt.Time,
	}
	if r.CustomerUuid.Valid {
		l.Customer = &model.Ref{UUID: uuid.UUID(r.CustomerUuid.Bytes), Name: name(r.CustomerFirstName, r.CustomerLastName)}
	}
	if r.ReservationUuid.Valid {
		l.Reservation = &model.ReservationRef{UUID: uuid.UUID(r.ReservationUuid.Bytes), Number: r.ReservationNumber.Int64}
	}
	if r.LineIndex.Valid {
		v := int(r.LineIndex.Int32)
		l.LineIndex = &v
	}
	if r.StaffUuid.Valid {
		l.Staff = &model.Ref{UUID: uuid.UUID(r.StaffUuid.Bytes), Name: r.StaffName.String}
	}
	l.CreatedBy = userRef(r.CreatorUuid, r.CreatorName, r.CreatorSurname)
	return l
}

func userRef(id pgtype.UUID, first, last pgtype.Text) *model.UserRef {
	if !id.Valid {
		return nil
	}
	return &model.UserRef{UUID: uuid.UUID(id.Bytes), Name: first.String, Surname: last.String}
}
