package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Stock rules (TEC-918):
//
//   - stock_movements is append-only; stock_levels.on_hand is changed only
//     together with a movement row, inside one transaction, after locking
//     the level row (SELECT … FOR UPDATE), so concurrent movements of the
//     same item serialize and balance_after is exact.
//   - Stock never goes negative: a sale or transfer may take at most the
//     available units (on hand − reserved); reserved units belong to
//     appointments (TEC-919). A count (adjustment) sets the counted value,
//     which is never below zero.
//   - Every movement names its branch, date, reason and the user who
//     recorded it; a sale may name the staff member credited with it.

// Limits of a movement.
const (
	maxQuantity  = 100000
	maxReasonLen = 200
	maxNoteLen   = 2000
	// A movement may be dated up to this much in the future (clock skew).
	futureSlack = 5 * time.Minute
)

// SourceManual marks movements entered on the panel.
const SourceManual = "manual"

// MovementSortColumns are the sortable movement list fields.
var MovementSortColumns = map[string]string{"occurred_at": "occurred_at", "quantity": "quantity", "kind": "kind"}

// MovementResult is what one recorded movement produced.
type MovementResult struct {
	Movements []model.Movement
	// Product is the product the movement belongs to (activity subject).
	Product model.Product
}

// RecordMovement validates and writes one stock movement (two rows for a
// transfer). actorID is the user recording it.
func (s *Service) RecordMovement(ctx context.Context, orgID, actorID int64, in model.MovementInput) (MovementResult, error) {
	q := s.repo.Q()
	ve := &ValidationError{}
	if !slices.Contains(model.InputKinds, in.Kind) {
		ve.add("kind", "invalid", "kind must be one of "+strings.Join(model.InputKinds, ", "))
		return MovementResult{}, ve
	}

	// Branches (unknown and outside the scope are the same answer).
	uuids := []uuid.UUID{in.BranchUUID}
	if in.ToBranchUUID != nil {
		uuids = append(uuids, *in.ToBranchUUID)
	}
	known, err := branchMap(ctx, q, orgID, uuids)
	if err != nil {
		return MovementResult{}, err
	}
	branchID, ok := known[in.BranchUUID]
	if !ok || orgctx.CheckBranch(ctx, branchID) != nil {
		ve.add("branch_uuid", "not_found", "branch not found")
	}
	var toBranchID int64
	if in.Kind == model.KindTransfer {
		if in.ToBranchUUID == nil {
			ve.add("to_branch_uuid", "required", "to_branch_uuid is required")
		} else {
			id, ok := known[*in.ToBranchUUID]
			switch {
			case !ok || orgctx.CheckBranch(ctx, id) != nil:
				ve.add("to_branch_uuid", "not_found", "branch not found")
			case id == branchID:
				ve.add("to_branch_uuid", "same_branch", "the target branch must differ from the source")
			default:
				toBranchID = id
			}
		}
	} else if in.ToBranchUUID != nil {
		ve.add("to_branch_uuid", "forbidden", "to_branch_uuid is for transfers only")
	}

	// Product and stock item.
	product, err := s.load(ctx, q, orgID, in.ProductUUID)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			return MovementResult{}, err
		}
		ve.add("product_uuid", "not_found", "product not found")
		return MovementResult{}, ve
	}
	variantID := pgtype.Int8{}
	var variant *db.ProductVariant
	switch {
	case in.VariantUUID != nil:
		v, err := s.loadVariant(ctx, q, orgID, product.ID, *in.VariantUUID)
		if err != nil {
			if !errors.Is(err, ErrVariantNotFound) {
				return MovementResult{}, err
			}
			ve.add("variant_uuid", "not_found", "variant not found")
		} else {
			variant = &v
			variantID = pgtype.Int8{Int64: v.ID, Valid: true}
		}
	case product.VariantsCount > 0:
		ve.add("variant_uuid", "required", "the product has variants; name one")
	}

	// Quantities.
	var quantity int32
	if in.Kind == model.KindAdjustment {
		switch {
		case in.CountedQuantity == nil:
			ve.add("counted_quantity", "required", "counted_quantity is required")
		case *in.CountedQuantity < 0 || *in.CountedQuantity > maxQuantity:
			ve.add("counted_quantity", "invalid", fmt.Sprintf("counted_quantity must be between 0 and %d", maxQuantity))
		}
		if in.Quantity != nil {
			ve.add("quantity", "forbidden", "a count takes counted_quantity")
		}
	} else {
		switch {
		case in.Quantity == nil:
			ve.add("quantity", "required", "quantity is required")
		case *in.Quantity < 1 || *in.Quantity > maxQuantity:
			ve.add("quantity", "invalid", fmt.Sprintf("quantity must be between 1 and %d", maxQuantity))
		default:
			quantity = *in.Quantity
		}
		if in.CountedQuantity != nil {
			ve.add("counted_quantity", "forbidden", "counted_quantity is for counts only")
		}
	}

	// Prices: a sale / return keeps its unit price (default: the branch
	// price), a receipt its unit cost; sales also keep the cost (margin).
	effPrice, effCost := money.FromNumericOrZero(product.Price), money.FromNumericOrZero(product.Cost)
	if variant != nil {
		if variant.Price.Valid {
			effPrice = money.FromNumericOrZero(variant.Price)
		}
		if variant.Cost.Valid {
			effCost = money.FromNumericOrZero(variant.Cost)
		}
	}
	var unitPrice, unitCost pgtype.Numeric
	switch in.Kind {
	case model.KindSale, model.KindReturn:
		price := effPrice
		if variant == nil || !variant.Price.Valid {
			// The branch override applies to products without their own
			// variant price.
			offers, err := q.ListProductBranches(ctx, db.ListProductBranchesParams{OrganizationID: orgID, ProductIds: []int64{product.ID}})
			if err != nil {
				return MovementResult{}, err
			}
			for _, o := range offers {
				if o.BranchID == branchID && o.Price.Valid {
					price = money.FromNumericOrZero(o.Price)
				}
			}
		}
		if in.UnitPrice != nil {
			if m, ok := parseAmount(ve, "unit_price", *in.UnitPrice); ok {
				price = m
			}
		}
		unitPrice = price.Numeric()
		unitCost = effCost.Numeric()
	case model.KindReceipt:
		cost := effCost
		if in.UnitCost != nil {
			if m, ok := parseAmount(ve, "unit_cost", *in.UnitCost); ok {
				cost = m
			}
		}
		unitCost = cost.Numeric()
	}
	if in.UnitPrice != nil && in.Kind != model.KindSale && in.Kind != model.KindReturn {
		ve.add("unit_price", "forbidden", "unit_price is for sales and returns only")
	}
	if in.UnitCost != nil && in.Kind != model.KindReceipt {
		ve.add("unit_cost", "forbidden", "unit_cost is for receipts only")
	}

	// A sale needs an active product sold in the branch.
	if in.Kind == model.KindSale && branchID != 0 {
		if !product.IsActive || (variant != nil && !variant.IsActive) {
			ve.add("product_uuid", "inactive", "the product is not active")
		} else {
			offers, err := q.ListProductBranches(ctx, db.ListProductBranchesParams{OrganizationID: orgID, ProductIds: []int64{product.ID}})
			if err != nil {
				return MovementResult{}, err
			}
			if !slices.ContainsFunc(offers, func(o db.ListProductBranchesRow) bool { return o.BranchID == branchID && o.IsActive }) {
				ve.add("branch_uuid", "not_offered", "the product is not sold in this branch")
			}
		}
	}

	// Staff credited with a sale or return.
	staffID := pgtype.Int8{}
	if in.StaffUUID != nil {
		if in.Kind != model.KindSale && in.Kind != model.KindReturn {
			ve.add("staff_uuid", "forbidden", "staff_uuid is for sales and returns only")
		} else {
			rows, err := q.GetStaffIDsByUUIDs(ctx, db.GetStaffIDsByUUIDsParams{OrganizationID: orgID, Uuids: []uuid.UUID{*in.StaffUUID}})
			if err != nil {
				return MovementResult{}, err
			}
			if len(rows) == 0 {
				ve.add("staff_uuid", "not_found", "staff member not found")
			} else {
				staffID = pgtype.Int8{Int64: rows[0].ID, Valid: true}
			}
		}
	}

	var reason, note string
	text("reason", in.Reason, &reason, maxReasonLen, false, true, ve)
	text("note", in.Note, &note, maxNoteLen, false, true, ve)
	occurred := time.Now().UTC()
	if in.OccurredAt != nil {
		if in.OccurredAt.After(time.Now().Add(futureSlack)) {
			ve.add("occurred_at", "future", "occurred_at must not be in the future")
		} else {
			occurred = in.OccurredAt.UTC()
		}
	}
	if err := ve.orNil(); err != nil {
		return MovementResult{}, err
	}

	source := SourceManual
	if in.Source != "" {
		source = in.Source
	}
	actor := pgtype.Int8{}
	if actorID > 0 {
		actor = pgtype.Int8{Int64: actorID, Valid: true}
	}
	base := db.InsertStockMovementParams{
		OrganizationID: orgID, ProductID: product.ID, VariantID: variantID, UnitPrice: unitPrice, UnitCost: unitCost,
		StaffID: staffID, Reason: reason, Note: note, Source: source,
		OccurredAt: pgtype.Timestamptz{Time: occurred, Valid: true}, CreatedBy: actor,
	}
	// An undated movement is stamped once its level rows are locked, so
	// concurrent movements of one item get occurred_at in the order they
	// were applied and the ledger (occurred_at, id) matches balance_after.
	stamp := func() {
		if in.OccurredAt == nil {
			occurred = time.Now().UTC()
			base.OccurredAt = pgtype.Timestamptz{Time: occurred, Valid: true}
		}
	}
	var created []uuid.UUID
	err = s.repo.InTx(ctx, func(tx *db.Queries) error {
		switch in.Kind {
		case model.KindTransfer:
			transfer := pgtype.UUID{Bytes: uuid.New(), Valid: true}
			// Lock both levels in branch id order (no deadlock between
			// opposite transfers).
			first, second := branchID, toBranchID
			if second < first {
				first, second = second, first
			}
			levels := map[int64]db.StockLevel{}
			for _, b := range []int64{first, second} {
				l, err := lockLevel(ctx, tx, orgID, b, product.ID, variantID)
				if err != nil {
					return err
				}
				levels[b] = l
			}
			stamp()
			src, dst := levels[branchID], levels[toBranchID]
			if int64(src.OnHand)-int64(src.Reserved) < int64(quantity) {
				return insufficient(src)
			}
			out := base
			out.BranchID, out.Kind, out.Quantity, out.BalanceAfter = branchID, model.KindTransferOut, -quantity, src.OnHand-quantity
			out.CounterpartBranchID, out.TransferUuid = pgtype.Int8{Int64: toBranchID, Valid: true}, transfer
			inLeg := base
			inLeg.BranchID, inLeg.Kind, inLeg.Quantity, inLeg.BalanceAfter = toBranchID, model.KindTransferIn, quantity, dst.OnHand+quantity
			inLeg.CounterpartBranchID, inLeg.TransferUuid = pgtype.Int8{Int64: branchID, Valid: true}, transfer
			for _, m := range []struct {
				level db.StockLevel
				p     db.InsertStockMovementParams
			}{{src, out}, {dst, inLeg}} {
				id, err := apply(ctx, tx, orgID, m.level, m.p)
				if err != nil {
					return err
				}
				created = append(created, id)
			}
			return nil
		default:
			level, err := lockLevel(ctx, tx, orgID, branchID, product.ID, variantID)
			if err != nil {
				return err
			}
			stamp()
			p := base
			p.BranchID, p.Kind = branchID, in.Kind
			switch in.Kind {
			case model.KindReceipt, model.KindReturn:
				p.Quantity = quantity
			case model.KindSale:
				if int64(level.OnHand)-int64(level.Reserved) < int64(quantity) {
					return insufficient(level)
				}
				p.Quantity = -quantity
			case model.KindAdjustment:
				p.Quantity = *in.CountedQuantity - level.OnHand
			}
			p.BalanceAfter = level.OnHand + p.Quantity
			id, err := apply(ctx, tx, orgID, level, p)
			if err != nil {
				return err
			}
			created = append(created, id)
			if in.Kind == model.KindSale && s.sales != nil {
				// The product handed over is revenue (TEC-916), in the same
				// transaction as the stock leaving.
				return s.sales(ctx, tx, orgID, StockSale{
					MovementUUID: id, BranchID: branchID, ProductID: product.ID, VariantID: optional(variantID),
					StaffID: optional(staffID), Description: saleName(product.Name, variant), Quantity: quantity,
					UnitPrice: money.FromNumericOrZero(unitPrice), OccurredAt: occurred, CreatedBy: optional(actor),
				})
			}
			return nil
		}
	})
	if err != nil {
		if errors.Is(repository.MapErr(err), repository.ErrNegativeStock) {
			return MovementResult{}, &ValidationError{Details: []Detail{{Field: "quantity", Code: "insufficient_stock", Message: "not enough stock"}}}
		}
		return MovementResult{}, err
	}
	out := MovementResult{}
	for _, id := range created {
		m, err := s.GetMovement(ctx, orgID, id)
		if err != nil {
			return MovementResult{}, err
		}
		out.Movements = append(out.Movements, m)
	}
	out.Product, err = s.Get(ctx, orgID, product.Uuid)
	return out, err
}

// StockSale is a product handed over at the counter, passed to the sale
// recorder inside the stock movement's transaction.
type StockSale struct {
	MovementUUID uuid.UUID
	BranchID     int64
	ProductID    int64
	VariantID    *int64
	StaffID      *int64
	Description  string
	Quantity     int32
	UnitPrice    money.Money
	OccurredAt   time.Time
	CreatedBy    *int64
}

// SaleRecorder writes the sale line of a stock sale (the sales module).
type SaleRecorder func(ctx context.Context, q *db.Queries, orgID int64, sale StockSale) error

// SetSaleRecorder installs the sale recorder (nil: stock sales are not
// posted as revenue).
func (s *Service) SetSaleRecorder(fn SaleRecorder) {
	s.sales = fn
}

func optional(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	n := v.Int64
	return &n
}

// saleName is the line text of a product sale (product / variant).
func saleName(product string, variant *db.ProductVariant) string {
	if variant == nil || variant.Name == "" {
		return product
	}
	return product + " / " + variant.Name
}

// lockLevel creates the level row of a stock item when missing and locks it.
func lockLevel(ctx context.Context, tx *db.Queries, orgID, branchID, productID int64, variantID pgtype.Int8) (db.StockLevel, error) {
	if err := tx.EnsureStockLevel(ctx, db.EnsureStockLevelParams{
		OrganizationID: orgID, BranchID: branchID, ProductID: productID, VariantID: variantID,
	}); err != nil {
		return db.StockLevel{}, err
	}
	return tx.LockStockLevel(ctx, db.LockStockLevelParams{
		OrganizationID: orgID, BranchID: branchID, ProductID: productID, VariantID: variantID,
	})
}

// apply writes the new balance and the ledger row.
func apply(ctx context.Context, tx *db.Queries, orgID int64, level db.StockLevel, p db.InsertStockMovementParams) (uuid.UUID, error) {
	if p.BalanceAfter < 0 {
		return uuid.Nil, repository.ErrNegativeStock
	}
	if err := tx.SetStockLevelOnHand(ctx, db.SetStockLevelOnHandParams{OrganizationID: orgID, ID: level.ID, OnHand: p.BalanceAfter}); err != nil {
		return uuid.Nil, err
	}
	row, err := tx.InsertStockMovement(ctx, p)
	if err != nil {
		return uuid.Nil, err
	}
	return row.Uuid, nil
}

// insufficient is the validation answer of a sale / transfer above the
// available units.
func insufficient(level db.StockLevel) error {
	available := int64(level.OnHand) - int64(level.Reserved)
	if available < 0 {
		available = 0
	}
	return &ValidationError{Details: []Detail{{
		Field: "quantity", Code: "insufficient_stock",
		Message: fmt.Sprintf("only %d units are available", available),
	}}}
}

// GetMovement reads one ledger row of the organization inside the
// caller's branch scope.
func (s *Service) GetMovement(ctx context.Context, orgID int64, id uuid.UUID) (model.Movement, error) {
	row, err := s.repo.Q().GetStockMovement(ctx, db.GetStockMovementParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return model.Movement{}, mapNotFound(repository.MapErr(err), ErrNotFound)
	}
	if orgctx.CheckBranch(ctx, row.BranchID) != nil {
		return model.Movement{}, ErrNotFound
	}
	return mapMovement(db.ListStockMovementsRow(row), currencyOf(ctx)), nil
}

// movementArgs resolves the uuids of a movement filter. Unknown uuids
// match nothing.
func (s *Service) movementArgs(ctx context.Context, q *db.Queries, orgID int64, f model.MovementFilter) (db.ListStockMovementsParams, error) {
	p := db.ListStockMovementsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Kinds: nilIfEmpty(f.Kinds), Q: textArg(f.Q),
	}
	if len(f.Branches) > 0 {
		ids, err := branchIDs(ctx, q, orgID, f.Branches)
		if err != nil {
			return p, err
		}
		p.FilterBranchIds = ids
	}
	if len(f.Products) > 0 {
		rows, err := q.ListProductIDsByUUIDs(ctx, db.ListProductIDsByUUIDsParams{OrganizationID: orgID, Uuids: f.Products})
		if err != nil {
			return p, err
		}
		p.ProductIds = []int64{}
		for _, r := range rows {
			p.ProductIds = append(p.ProductIds, r.ID)
		}
	}
	if len(f.Staff) > 0 {
		rows, err := q.GetStaffIDsByUUIDs(ctx, db.GetStaffIDsByUUIDsParams{OrganizationID: orgID, Uuids: f.Staff})
		if err != nil {
			return p, err
		}
		p.StaffIds = []int64{}
		for _, r := range rows {
			p.StaffIds = append(p.StaffIds, r.ID)
		}
	}
	if f.From != nil {
		p.OccurredFrom = pgtype.Timestamptz{Time: *f.From, Valid: true}
	}
	if f.To != nil {
		p.OccurredTo = pgtype.Timestamptz{Time: *f.To, Valid: true}
	}
	return p, nil
}

// ListMovements returns one page of the ledger (newest first by default)
// in the caller's branches.
func (s *Service) ListMovements(ctx context.Context, orgID int64, f model.MovementFilter) ([]model.Movement, int64, error) {
	if _, ok := MovementSortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "", false
	}
	q := s.repo.Q()
	p, err := s.movementArgs(ctx, q, orgID, f)
	if err != nil {
		return nil, 0, err
	}
	p.SortField, p.SortDesc, p.LimitCount, p.OffsetCount = f.SortField, f.SortDesc, f.Limit, f.Offset
	rows, err := q.ListStockMovements(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountStockMovements(ctx, db.CountStockMovementsParams{
		OrganizationID: orgID, BranchIds: p.BranchIds, FilterBranchIds: p.FilterBranchIds, ProductIds: p.ProductIds,
		StaffIds: p.StaffIds, Kinds: p.Kinds, OccurredFrom: p.OccurredFrom, OccurredTo: p.OccurredTo, Q: p.Q,
	})
	if err != nil {
		return nil, 0, err
	}
	currency := currencyOf(ctx)
	out := make([]model.Movement, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapMovement(row, currency))
	}
	return out, total, nil
}

// MovementFacets returns the kinds, branches and staff in the caller's ledger.
func (s *Service) MovementFacets(ctx context.Context, orgID int64) (model.MovementFacets, error) {
	q := s.repo.Q()
	scope := orgctx.BranchScope(ctx)
	out := model.MovementFacets{Kind: []model.Facet{}, Branch: []model.Facet{}, Staff: []model.Facet{}}
	kinds, err := q.ListStockMovementKindFacets(ctx, db.ListStockMovementKindFacetsParams{OrganizationID: orgID, BranchIds: scope})
	if err != nil {
		return out, err
	}
	for _, r := range kinds {
		out.Kind = append(out.Kind, model.Facet{Value: r.Value, Count: r.Count})
	}
	branches, err := q.ListStockMovementBranchFacets(ctx, db.ListStockMovementBranchFacetsParams{OrganizationID: orgID, BranchIds: scope})
	if err != nil {
		return out, err
	}
	for _, r := range branches {
		out.Branch = append(out.Branch, model.Facet{Value: r.Uuid.String(), Label: r.Name, Count: r.Count})
	}
	staff, err := q.ListStockMovementStaffFacets(ctx, db.ListStockMovementStaffFacetsParams{OrganizationID: orgID, BranchIds: scope})
	if err != nil {
		return out, err
	}
	for _, r := range staff {
		out.Staff = append(out.Staff, model.Facet{Value: r.Uuid.String(), Label: r.Name, Count: r.Count})
	}
	return out, nil
}

func mapMovement(row db.ListStockMovementsRow, currency string) model.Movement {
	m := model.Movement{
		UUID: row.Uuid, Kind: row.Kind, Quantity: row.Quantity, BalanceAfter: row.BalanceAfter,
		Product:  model.Ref{UUID: row.ProductUuid, Name: row.ProductName, SKU: row.ProductSku},
		Branch:   model.Ref{UUID: row.BranchUuid, Name: row.BranchName},
		Currency: currency, Reason: row.Reason, Note: row.Note, Source: row.Source, Returned: row.Returned,
		OccurredAt: row.OccurredAt.Time, CreatedAt: row.CreatedAt.Time,
	}
	if row.VariantUuid.Valid {
		m.Variant = &model.Ref{UUID: uuid.UUID(row.VariantUuid.Bytes), Name: row.VariantName.String, SKU: row.VariantSku.String}
	}
	if row.CounterpartBranchUuid.Valid {
		m.CounterpartBranch = &model.Ref{UUID: uuid.UUID(row.CounterpartBranchUuid.Bytes), Name: row.CounterpartBranchName.String}
	}
	if row.TransferUuid.Valid {
		id := uuid.UUID(row.TransferUuid.Bytes)
		m.TransferUUID = &id
	}
	if row.UnitPrice.Valid {
		v := money.FromNumericOrZero(row.UnitPrice)
		m.UnitPrice = &v
	}
	if row.UnitCost.Valid {
		v := money.FromNumericOrZero(row.UnitCost)
		m.UnitCost = &v
	}
	if row.StaffUuid.Valid {
		m.Staff = &model.Ref{UUID: uuid.UUID(row.StaffUuid.Bytes), Name: row.StaffName.String}
	}
	if row.CreatedByUuid.Valid {
		m.CreatedBy = &model.Ref{UUID: uuid.UUID(row.CreatedByUuid.Bytes), Name: strings.TrimSpace(row.CreatedByName.String + " " + row.CreatedBySurname.String)}
	}
	return m
}
