package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

var (
	// ErrNotReturnable: only a stock sale can be returned.
	ErrNotReturnable = errors.New("only a sale can be returned")
	// ErrAlreadyReturned: the sale was returned before.
	ErrAlreadyReturned = errors.New("the sale was returned already")
	// ErrMovementNotFound: no such stock movement in the caller's scope.
	ErrMovementNotFound = errors.New("stock movement not found")
)

// SaleReverser reverses the revenue line of a returned stock sale inside
// the return's transaction (the sales module, TEC-913). A sale without a
// live line (posted before the ledger, or reversed on the books already)
// is not an error for the stock.
type SaleReverser func(ctx context.Context, q *db.Queries, orgID int64, movement uuid.UUID, reason string, actorID *int64) error

// SetSaleReverser installs the sale line reverser (nil: a return moves the
// stock only).
func (s *Service) SetSaleReverser(fn SaleReverser) {
	s.reverse = fn
}

// ReturnSale takes back the product of a stock sale (a manager's
// operation, requirements "teslim edilmiş ürün iadesi"): one return
// movement brings every unit of the sale back to its branch, and the
// sale's revenue line is reversed in the same transaction. A sale is
// returned at most once (uq_stock_movements_sale_return). Money paid for
// it is refunded separately.
func (s *Service) ReturnSale(ctx context.Context, orgID, actorID int64, id uuid.UUID, in model.ReturnInput) (MovementResult, error) {
	q := s.repo.Q()
	sale, err := q.GetStockMovement(ctx, db.GetStockMovementParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return MovementResult{}, mapNotFound(repository.MapErr(err), ErrMovementNotFound)
	}
	if orgctx.CheckBranch(ctx, sale.BranchID) != nil {
		return MovementResult{}, ErrMovementNotFound
	}
	ve := &ValidationError{}
	var reason, note string
	text("reason", &in.Reason, &reason, maxReasonLen, true, true, ve)
	text("note", &in.Note, &note, maxNoteLen, false, true, ve)
	if err := ve.orNil(); err != nil {
		return MovementResult{}, err
	}
	if sale.Kind != model.KindSale {
		return MovementResult{}, ErrNotReturnable
	}
	if sale.Returned {
		return MovementResult{}, ErrAlreadyReturned
	}
	actor := pgtype.Int8{Int64: actorID, Valid: actorID > 0}
	var created uuid.UUID
	err = s.repo.InTx(ctx, func(tx *db.Queries) error {
		level, err := lockLevel(ctx, tx, orgID, sale.BranchID, sale.ProductID, sale.VariantID)
		if err != nil {
			return err
		}
		quantity := -sale.Quantity
		created, err = apply(ctx, tx, orgID, level, db.InsertStockMovementParams{
			OrganizationID: orgID, BranchID: sale.BranchID, ProductID: sale.ProductID, VariantID: sale.VariantID,
			Kind: model.KindReturn, Quantity: quantity, BalanceAfter: level.OnHand + quantity, UnitPrice: sale.UnitPrice,
			StaffID: sale.StaffID, Reason: reason, Note: note, Source: model.SourceSaleReturn,
			SourceUuid: pgtype.UUID{Bytes: sale.Uuid, Valid: true}, OccurredAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			CreatedBy: actor,
		})
		if err != nil {
			return err
		}
		if s.reverse != nil {
			return s.reverse(ctx, tx, orgID, sale.Uuid, reason, optional(actor))
		}
		return nil
	})
	if err != nil {
		if errors.Is(repository.MapErr(err), repository.ErrSaleReturned) {
			return MovementResult{}, ErrAlreadyReturned
		}
		return MovementResult{}, err
	}
	m, err := s.GetMovement(ctx, orgID, created)
	if err != nil {
		return MovementResult{}, err
	}
	out := MovementResult{Movements: []model.Movement{m}}
	out.Product, err = s.Get(ctx, orgID, m.Product.UUID)
	return out, err
}
