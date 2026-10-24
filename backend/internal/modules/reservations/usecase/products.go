package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Products of a visit (TEC-919, requirements §12, A18):
//
//   - Adding a product reserves its units in the visit's branch
//     (stock_levels.reserved) in the same transaction as the line, after
//     locking the level row: available = on hand - reserved, so the last
//     unit is never reserved for two visits.
//   - A request keeps its products for as long as its timed hold (review
//     window, payment time); when the hold lapses the request is cancelled
//     and its products are released with it.
//   - Reject, cancel, expiry and no-show release the products that were
//     not handed over; removing a line releases it.
//   - Completing the visit hands the held products over: the reserved
//     units leave the stock as one sale movement each (source
//     reservation), and the completion hook (sales) turns each into one
//     product sale line of the visit. A payment never hands a product over.

// SourceReservation marks the stock movements written by a visit.
const SourceReservation = "reservation"

var (
	// ErrProductNotFound: the product line is not part of the visit.
	ErrProductNotFound = errors.New("reservation product not found")
	// ErrPaymentPending: the visit waits for its online payment; its
	// amount cannot change until the payment ends.
	ErrPaymentPending = errors.New("the reservation waits for its online payment")
)

// Statuses whose products may still change.
func productsOpen(status string) bool {
	return status == model.StatusRequested || status == model.StatusActive || status == model.StatusInService
}

// pickedProduct is a validated product input.
type pickedProduct struct {
	index       int
	productID   int64
	variantID   pgtype.Int8
	name        string
	variantName pgtype.Text
	sku         string
	quantity    int32
	price       money.Money
	cost        money.Money
	staffID     pgtype.Int8
}

// pickProducts validates the product inputs of a visit in its branch:
// active, sold in the branch (and shown online for the customer screens),
// a variant when the product has them, a quantity inside the limits.
// Prices follow the stock sale order: the variant's own price, else the
// branch price, else the catalog price.
func pickProducts(ctx context.Context, q *db.Queries, orgID int64, row db.Reservation, items []model.ProductInput, onlineOnly bool, field string) ([]pickedProduct, error) {
	ve := &ValidationError{}
	if len(items) > model.MaxProductLines {
		ve.add(field, "too_many", fmt.Sprintf("at most %d products", model.MaxProductLines))
		return nil, ve
	}
	defaultStaff := pgtype.Int8{}
	if len(items) > 0 {
		id, err := q.GetReservationFirstStaff(ctx, db.GetReservationFirstStaffParams{OrganizationID: orgID, ReservationID: row.ID})
		switch {
		case err == nil:
			defaultStaff = pgtype.Int8{Int64: id, Valid: true}
		case !errors.Is(err, pgx.ErrNoRows):
			return nil, err
		}
	}
	out := make([]pickedProduct, 0, len(items))
	for i, in := range items {
		f := func(name string) string {
			if field == "" {
				return name
			}
			return fmt.Sprintf("%s[%d].%s", field, i, name)
		}
		if in.Quantity < 1 || in.Quantity > model.MaxProductQuantity {
			ve.add(f("quantity"), "invalid", fmt.Sprintf("quantity must be between 1 and %d", model.MaxProductQuantity))
			continue
		}
		pick, err := q.GetReservationProductPick(ctx, db.GetReservationProductPickParams{
			OrganizationID: orgID, BranchID: row.BranchID, ProductUuid: in.ProductUUID, VariantUuid: optUUID(in.VariantUUID),
		})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && onlineOnly && !pick.ShowOnline) {
			ve.add(f("product_uuid"), "not_found", "product not found")
			continue
		}
		if err != nil {
			return nil, err
		}
		switch {
		case !pick.IsActive:
			ve.add(f("product_uuid"), "inactive", "the product is not active")
			continue
		case !pick.Offered.Valid || !pick.Offered.Bool:
			ve.add(f("product_uuid"), "not_offered", "the product is not sold in this branch")
			continue
		case in.VariantUUID != nil && !pick.VariantID.Valid:
			ve.add(f("variant_uuid"), "not_found", "variant not found")
			continue
		case in.VariantUUID == nil && pick.VariantsCount > 0:
			ve.add(f("variant_uuid"), "required", "the product has variants; name one")
			continue
		case pick.VariantID.Valid && !pick.VariantActive.Bool:
			ve.add(f("variant_uuid"), "inactive", "the variant is not active")
			continue
		}
		p := pickedProduct{
			index: i, productID: pick.ID, variantID: pick.VariantID, name: pick.Name, sku: pick.Sku,
			quantity: int32(in.Quantity), price: money.FromNumericOrZero(pick.Price), cost: money.FromNumericOrZero(pick.Cost),
			staffID: defaultStaff,
		}
		if pick.VariantID.Valid {
			p.variantName, p.sku = pick.VariantName, pick.VariantSku.String
			if pick.VariantCost.Valid {
				p.cost = money.FromNumericOrZero(pick.VariantCost)
			}
		}
		switch {
		case pick.VariantID.Valid && pick.VariantPrice.Valid:
			p.price = money.FromNumericOrZero(pick.VariantPrice)
		case pick.BranchPrice.Valid:
			p.price = money.FromNumericOrZero(pick.BranchPrice)
		}
		if in.StaffUUID != nil {
			ids, err := q.GetStaffIDsByUUIDs(ctx, db.GetStaffIDsByUUIDsParams{OrganizationID: orgID, Uuids: []uuid.UUID{*in.StaffUUID}})
			if err != nil {
				return nil, err
			}
			if len(ids) == 0 {
				ve.add(f("staff_uuid"), "not_found", "staff member not found")
				continue
			}
			p.staffID = pgtype.Int8{Int64: ids[0].ID, Valid: true}
		}
		out = append(out, p)
	}
	return out, ve.orNil()
}

// reserveProducts writes the product lines of a visit and reserves their
// units, locking each stock level first (in product order, so two visits
// never wait on each other). Not enough available units is a validation
// error naming the line; the caller's transaction then writes nothing.
func reserveProducts(ctx context.Context, q *db.Queries, orgID int64, row db.Reservation, picked []pickedProduct, source string, actorID int64, field string) error {
	sorted := append([]pickedProduct(nil), picked...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].productID != sorted[j].productID {
			return sorted[i].productID < sorted[j].productID
		}
		return sorted[i].variantID.Int64 < sorted[j].variantID.Int64
	})
	ve := &ValidationError{}
	for _, p := range sorted {
		level, err := lockStock(ctx, q, orgID, row.BranchID, p.productID, p.variantID)
		if err != nil {
			return err
		}
		available := int64(level.OnHand) - int64(level.Reserved)
		if available < int64(p.quantity) {
			name := "quantity"
			if field != "" {
				name = fmt.Sprintf("%s[%d].quantity", field, p.index)
			}
			ve.add(name, "insufficient_stock", fmt.Sprintf("only %d units are available", max(available, 0)))
			continue
		}
		if err := q.SetStockLevelReserved(ctx, db.SetStockLevelReservedParams{
			OrganizationID: orgID, ID: level.ID, Reserved: level.Reserved + p.quantity,
		}); err != nil {
			return err
		}
		if _, err := q.InsertReservationProduct(ctx, db.InsertReservationProductParams{
			OrganizationID: orgID, ReservationID: row.ID, BranchID: row.BranchID, ProductID: p.productID,
			VariantID: p.variantID, ProductName: p.name, VariantName: p.variantName, Sku: p.sku, Quantity: p.quantity,
			UnitPrice: p.price.Numeric(), UnitCost: p.cost.Numeric(), StaffID: p.staffID, Source: source,
			CreatedBy: pgtype.Int8{Int64: actorID, Valid: actorID > 0},
		}); err != nil {
			return err
		}
	}
	return ve.orNil()
}

// lockStock creates the level row of a stock item when missing and locks it.
func lockStock(ctx context.Context, q *db.Queries, orgID, branchID, productID int64, variantID pgtype.Int8) (db.StockLevel, error) {
	if err := q.EnsureStockLevel(ctx, db.EnsureStockLevelParams{
		OrganizationID: orgID, BranchID: branchID, ProductID: productID, VariantID: variantID,
	}); err != nil {
		return db.StockLevel{}, err
	}
	return q.LockStockLevel(ctx, db.LockStockLevelParams{
		OrganizationID: orgID, BranchID: branchID, ProductID: productID, VariantID: variantID,
	})
}

// unreserve gives the units of one held line back to the available stock.
func unreserve(ctx context.Context, q *db.Queries, orgID int64, p db.ReservationProduct, reason string) error {
	level, err := lockStock(ctx, q, orgID, p.BranchID, p.ProductID, p.VariantID)
	if err != nil {
		return err
	}
	if err := q.SetStockLevelReserved(ctx, db.SetStockLevelReservedParams{
		OrganizationID: orgID, ID: level.ID, Reserved: max(level.Reserved-p.Quantity, 0),
	}); err != nil {
		return err
	}
	return q.ReleaseReservationProduct(ctx, db.ReleaseReservationProductParams{
		OrganizationID: orgID, ID: p.ID, ReleaseReason: pgtype.Text{String: reason, Valid: true},
	})
}

// releaseProducts frees every product of the visit that was not handed
// over (reject, cancel, expiry, no-show).
func releaseProducts(ctx context.Context, q *db.Queries, orgID, reservationID int64, reason string) error {
	held, err := q.ListHeldReservationProducts(ctx, db.ListHeldReservationProductsParams{OrganizationID: orgID, ReservationID: reservationID})
	if err != nil {
		return err
	}
	for _, p := range held {
		if err := unreserve(ctx, q, orgID, p, reason); err != nil {
			return err
		}
	}
	return nil
}

// deliverProducts hands the held products of a completed visit over: the
// reserved units leave the stock as one sale movement each, dated when
// the visit ended (never later than now). A line whose units are no
// longer on hand (a count went below the reservation) stops the
// completion with insufficient_stock; the line can be removed first.
func deliverProducts(ctx context.Context, q *db.Queries, orgID int64, row db.Reservation, actorID int64, now time.Time) error {
	held, err := q.ListHeldReservationProducts(ctx, db.ListHeldReservationProductsParams{OrganizationID: orgID, ReservationID: row.ID})
	if err != nil {
		return err
	}
	when := now
	if row.EndsAt.Valid && row.EndsAt.Time.Before(when) {
		when = row.EndsAt.Time
	}
	actor := pgtype.Int8{Int64: actorID, Valid: actorID > 0}
	for _, p := range held {
		level, err := lockStock(ctx, q, orgID, p.BranchID, p.ProductID, p.VariantID)
		if err != nil {
			return err
		}
		if level.OnHand < p.Quantity {
			ve := &ValidationError{}
			ve.add("products", "insufficient_stock", fmt.Sprintf("%s: only %d units are on hand", p.ProductName, level.OnHand))
			return ve
		}
		onHand := level.OnHand - p.Quantity
		if err := q.SetStockLevelStock(ctx, db.SetStockLevelStockParams{
			OrganizationID: orgID, ID: level.ID, OnHand: onHand, Reserved: max(level.Reserved-p.Quantity, 0),
		}); err != nil {
			return err
		}
		movement, err := q.InsertStockMovement(ctx, db.InsertStockMovementParams{
			OrganizationID: orgID, BranchID: p.BranchID, ProductID: p.ProductID, VariantID: p.VariantID,
			Kind: "sale", Quantity: -p.Quantity, BalanceAfter: onHand, UnitPrice: p.UnitPrice, UnitCost: p.UnitCost,
			StaffID: p.StaffID, Reason: "", Note: fmt.Sprintf("#%d", row.Number), Source: SourceReservation,
			SourceUuid: pgtype.UUID{Bytes: row.Uuid, Valid: true}, OccurredAt: ts(when), CreatedBy: actor,
		})
		if err != nil {
			return err
		}
		if err := q.DeliverReservationProduct(ctx, db.DeliverReservationProductParams{
			OrganizationID: orgID, ID: p.ID, StockMovementUuid: pgtype.UUID{Bytes: movement.Uuid, Valid: true},
			DeliveredAt: ts(now),
		}); err != nil {
			return err
		}
	}
	return nil
}

// visitTotal is the services plus the products held or handed over.
func visitTotal(ctx context.Context, q *db.Queries, orgID, reservationID int64) (money.Money, error) {
	services, err := q.SumReservationLinePrices(ctx, db.SumReservationLinePricesParams{OrganizationID: orgID, ReservationID: reservationID})
	if err != nil {
		return money.Zero(), err
	}
	products, err := q.SumReservationProducts(ctx, db.SumReservationProductsParams{OrganizationID: orgID, ReservationID: reservationID})
	if err != nil {
		return money.Zero(), err
	}
	return money.FromNumericOrZero(services).Add(money.FromNumericOrZero(products)), nil
}

// productsTotal is the sum of the visit's live products.
func productsTotal(ctx context.Context, q *db.Queries, orgID, reservationID int64) (money.Money, error) {
	n, err := q.SumReservationProducts(ctx, db.SumReservationProductsParams{OrganizationID: orgID, ReservationID: reservationID})
	return money.FromNumericOrZero(n), err
}

// changeProducts runs a product change of a visit under the business's
// lock: the record is locked, the change applied, the total recomputed
// and the history written (version + 1). A visit paid in full that now
// costs more is collected at the business for the rest (on_site).
func (s *Service) changeProducts(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, actorID int64,
	expected *int, typ string, change func(tx *availabilityusecase.Tx, row db.Reservation) (string, error),
) (model.Reservation, error) {
	err := s.holds.Run(ctx, orgID, func(tx *availabilityusecase.Tx) error {
		row, err := s.visible(ctx, tx.Q, orgID, id, caller, true)
		if err != nil {
			return err
		}
		if expected != nil && *expected != int(row.Version) {
			return ErrStaleVersion
		}
		if !productsOpen(row.Status) {
			return ErrInvalidTransition
		}
		if row.PaymentStatus == model.PaymentOnlinePending {
			return ErrPaymentPending
		}
		note, err := change(tx, row)
		if err != nil {
			return err
		}
		total, err := visitTotal(ctx, tx.Q, orgID, row.ID)
		if err != nil {
			return err
		}
		payment := row.PaymentStatus
		if payment == model.PaymentPaid && total.Minor() > money.FromNumericOrZero(row.TotalPrice).Minor() {
			payment = model.PaymentOnSite
		}
		updated, err := tx.Q.UpdateReservationProductsTotal(ctx, db.UpdateReservationProductsTotalParams{
			OrganizationID: orgID, ID: row.ID, TotalPrice: total.Numeric(), PaymentStatus: payment,
		})
		if err != nil {
			return err
		}
		return s.event(ctx, tx.Q, updated, typ, "", "", actorID, note)
	})
	if err != nil {
		return model.Reservation{}, err
	}
	return s.get(ctx, orgID, id)
}

// AddProduct adds a product to a visit that is still open (requested,
// active, in service) and reserves its units.
func (s *Service) AddProduct(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, actorID int64, in model.AddProductInput) (model.Reservation, error) {
	return s.changeProducts(ctx, orgID, id, caller, actorID, in.ExpectedVersion, "product_added", func(tx *availabilityusecase.Tx, row db.Reservation) (string, error) {
		picked, err := pickProducts(ctx, tx.Q, orgID, row, []model.ProductInput{in.ProductInput}, false, "")
		if err != nil {
			return "", err
		}
		if err := reserveProducts(ctx, tx.Q, orgID, row, picked, model.SourcePanel, actorID, ""); err != nil {
			return "", err
		}
		return productNote(picked[0].name, picked[0].variantName, picked[0].quantity), nil
	})
}

// RemoveProduct takes a held product off a visit and frees its units.
func (s *Service) RemoveProduct(ctx context.Context, orgID int64, id, productID uuid.UUID, caller availabilityusecase.Caller, actorID int64, in ChangeInput) (model.Reservation, error) {
	return s.changeProducts(ctx, orgID, id, caller, actorID, in.ExpectedVersion, "product_removed", func(tx *availabilityusecase.Tx, row db.Reservation) (string, error) {
		p, err := tx.Q.GetReservationProductForUpdate(ctx, db.GetReservationProductForUpdateParams{OrganizationID: orgID, ReservationID: row.ID, Uuid: productID})
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrProductNotFound
		}
		if err != nil {
			return "", err
		}
		if p.Status != model.ProductHeld {
			return "", ErrInvalidTransition
		}
		if err := unreserve(ctx, tx.Q, orgID, p, "removed"); err != nil {
			return "", err
		}
		return productNote(p.ProductName, p.VariantName, p.Quantity), nil
	})
}

// ProductOptions lists the stock items that can be added to a visit in
// its branch (q matches name, SKU or barcode), with their available units.
func (s *Service) ProductOptions(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, q string) ([]model.ProductOption, error) {
	row, err := s.visible(ctx, s.q, orgID, id, caller, false)
	if err != nil {
		return nil, err
	}
	return ListProductOptions(ctx, s.q, orgID, row.BranchID, q, false, 50)
}

// BranchProductOptions lists the stock items of a branch for a booking
// that does not exist yet (the panel's new reservation form, TEC-977). An
// unknown branch, one of another business or outside the caller's branch
// scope is availabilityusecase.ErrNotFound.
func (s *Service) BranchProductOptions(ctx context.Context, orgID int64, branchUUID uuid.UUID, q string) ([]model.ProductOption, error) {
	branch, err := s.q.GetBranchByUUID(ctx, db.GetBranchByUUIDParams{OrganizationID: orgID, Uuid: branchUUID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && orgctx.CheckBranch(ctx, branch.ID) != nil) {
		return nil, availabilityusecase.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return ListProductOptions(ctx, s.q, orgID, branch.ID, q, false, 50)
}

// ListProductOptions lists the stock items sold in a branch. onlineOnly
// keeps the products shown on the customer screens that have units
// available (the public booking page).
func ListProductOptions(ctx context.Context, q *db.Queries, orgID, branchID int64, search string, onlineOnly bool, limit int32) ([]model.ProductOption, error) {
	search = strings.TrimSpace(search)
	rows, err := q.ListReservationProductOptions(ctx, db.ListReservationProductOptionsParams{
		OrganizationID: orgID, BranchID: branchID, OnlineOnly: onlineOnly, InStockOnly: onlineOnly,
		Q: pgtype.Text{String: search, Valid: search != ""}, LimitCount: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]model.ProductOption, 0, len(rows))
	for _, r := range rows {
		o := model.ProductOption{
			ProductUUID: r.ProductUuid, ProductName: r.ProductName, SKU: r.ProductSku,
			Price: money.FromNumericOrZero(r.Price), Available: r.Available,
		}
		if r.VariantUuid.Valid {
			v := uuid.UUID(r.VariantUuid.Bytes)
			name := r.VariantName.String
			o.VariantUUID, o.VariantName, o.SKU = &v, &name, r.VariantSku.String
		}
		if r.ImageUuid.Valid {
			img := uuid.UUID(r.ImageUuid.Bytes)
			o.ImageUUID = &img
		}
		out = append(out, o)
	}
	return out, nil
}

// productNote is the history note of a product change ("Wax / 100 ml × 2").
func productNote(name string, variant pgtype.Text, quantity int32) string {
	if variant.Valid && variant.String != "" {
		name += " / " + variant.String
	}
	return fmt.Sprintf("%s × %d", name, quantity)
}

// listProducts reads the products of a visit.
func listProducts(ctx context.Context, q *db.Queries, orgID, reservationID int64) ([]model.Product, error) {
	rows, err := q.ListReservationProducts(ctx, db.ListReservationProductsParams{OrganizationID: orgID, ReservationID: reservationID})
	if err != nil {
		return nil, err
	}
	out := make([]model.Product, 0, len(rows))
	for _, r := range rows {
		price := money.FromNumericOrZero(r.UnitPrice)
		p := model.Product{
			UUID: r.Uuid, Product: model.Ref{UUID: r.ProductUuid, Name: r.ProductName}, SKU: r.Sku,
			Quantity: int(r.Quantity), UnitPrice: price, Total: price.Mul(int64(r.Quantity)), Status: r.Status,
			Source: r.Source, CreatedAt: r.CreatedAt.Time.UTC(),
		}
		if r.VariantUuid.Valid {
			p.Variant = &model.Ref{UUID: uuid.UUID(r.VariantUuid.Bytes), Name: r.VariantName.String}
		}
		if r.StaffUuid.Valid {
			p.Staff = &model.Ref{UUID: uuid.UUID(r.StaffUuid.Bytes), Name: r.StaffName.String}
		}
		if r.ReleaseReason.Valid {
			reason := r.ReleaseReason.String
			p.ReleaseReason = &reason
		}
		if r.DeliveredAt.Valid {
			t := r.DeliveredAt.Time.UTC()
			p.DeliveredAt = &t
		}
		if r.ReleasedAt.Valid {
			t := r.ReleasedAt.Time.UTC()
			p.ReleasedAt = &t
		}
		out = append(out, p)
	}
	return out, nil
}

func optUUID(v *uuid.UUID) pgtype.UUID {
	if v == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *v, Valid: true}
}
