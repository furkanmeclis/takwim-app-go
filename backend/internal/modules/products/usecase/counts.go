package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

// Bulk stock count import (TEC-964): every row names a branch code, a
// stock code (product or variant SKU, or its barcode) and the counted
// units. A row writes one adjustment movement through RecordMovement, so
// the count rules of the panel apply unchanged (branch scope, counted
// units between 0 and maxQuantity, stock never below zero). Rolling the
// import back writes a compensating adjustment of the opposite change; it
// is refused when the units have left the stock since.

// ErrNotImportCount: the movement is not an imported count.
var ErrNotImportCount = errors.New("the movement is not an imported count")

// CountTarget is the stock item a count row resolves to.
type CountTarget struct {
	BranchUUID  uuid.UUID
	BranchName  string
	ProductUUID uuid.UUID
	ProductName string
	VariantUUID *uuid.UUID
	VariantName string
	// OnHand is the current stock of the item in the branch.
	OnHand int32
}

// CountRow is one parsed row of a count import.
type CountRow struct {
	BranchCode string
	StockCode  string
	Counted    int32
	Reason     string
	Note       string
}

// ResolveCount finds the branch (inside the caller's scope) and the stock
// item of a count row, with its current stock in that branch.
func (s *Service) ResolveCount(ctx context.Context, orgID int64, branchCode, stockCode string) (CountTarget, error) {
	q := s.repo.Q()
	branchCode = strings.ToUpper(strings.TrimSpace(branchCode))
	stockCode = strings.TrimSpace(stockCode)
	if branchCode == "" {
		return CountTarget{}, &ValidationError{Details: []Detail{{Field: "branch_code", Code: "required", Message: "branch_code is required"}}}
	}
	if stockCode == "" {
		return CountTarget{}, &ValidationError{Details: []Detail{{Field: "stock_code", Code: "required", Message: "stock_code is required"}}}
	}
	branch, err := q.GetBranchByCode(ctx, db.GetBranchByCodeParams{OrganizationID: orgID, Code: branchCode})
	if err != nil {
		if errors.Is(repository.MapErr(err), repository.ErrNotFound) {
			return CountTarget{}, &ValidationError{Details: []Detail{{Field: "branch_code", Code: "not_found", Message: "unknown branch code " + branchCode}}}
		}
		return CountTarget{}, err
	}
	if orgctx.CheckBranch(ctx, branch.ID) != nil {
		// Outside the requester's branches reads the same as unknown.
		return CountTarget{}, &ValidationError{Details: []Detail{{Field: "branch_code", Code: "not_found", Message: "unknown branch code " + branchCode}}}
	}
	matches, err := q.LookupStockCode(ctx, db.LookupStockCodeParams{OrganizationID: orgID, Code: stockCode})
	if err != nil {
		return CountTarget{}, err
	}
	switch {
	case len(matches) == 0:
		return CountTarget{}, &ValidationError{Details: []Detail{{Field: "stock_code", Code: "not_found", Message: "unknown stock code " + stockCode}}}
	case len(matches) > 1:
		return CountTarget{}, &ValidationError{Details: []Detail{{Field: "stock_code", Code: "ambiguous", Message: "stock code " + stockCode + " matches more than one item; use the SKU"}}}
	}
	match := matches[0]
	product, err := s.load(ctx, q, orgID, match.ProductUuid)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return CountTarget{}, &ValidationError{Details: []Detail{{Field: "stock_code", Code: "not_found", Message: "unknown stock code " + stockCode}}}
		}
		return CountTarget{}, err
	}
	out := CountTarget{BranchUUID: branch.Uuid, BranchName: branch.Name, ProductUUID: product.Uuid, ProductName: product.Name}
	var variantID *int64
	if match.VariantUuid.Valid {
		v, err := s.loadVariant(ctx, q, orgID, product.ID, uuid.UUID(match.VariantUuid.Bytes))
		if err != nil {
			return CountTarget{}, err
		}
		id := v.Uuid
		out.VariantUUID, out.VariantName, variantID = &id, v.Name, &v.ID
	} else if product.VariantsCount > 0 {
		return CountTarget{}, &ValidationError{Details: []Detail{{Field: "stock_code", Code: "variant_required", Message: "the product has variants; use the variant's stock code"}}}
	}
	levels, err := q.ListStockLevels(ctx, db.ListStockLevelsParams{OrganizationID: orgID, ProductIds: []int64{product.ID}, BranchIds: []int64{branch.ID}})
	if err != nil {
		return CountTarget{}, err
	}
	for _, l := range levels {
		if l.BranchID != branch.ID {
			continue
		}
		if (variantID == nil && !l.VariantID.Valid) || (variantID != nil && l.VariantID.Valid && l.VariantID.Int64 == *variantID) {
			out.OnHand = l.OnHand
		}
	}
	return out, nil
}

// ImportCount records the count of one import row as an adjustment
// movement (source "import"). It returns the movement and the stock on
// hand before it.
func (s *Service) ImportCount(ctx context.Context, orgID, actorID int64, row CountRow) (model.Movement, error) {
	target, err := s.ResolveCount(ctx, orgID, row.BranchCode, row.StockCode)
	if err != nil {
		return model.Movement{}, err
	}
	counted := row.Counted
	in := model.MovementInput{
		Kind: model.KindAdjustment, BranchUUID: target.BranchUUID, ProductUUID: target.ProductUUID,
		VariantUUID: target.VariantUUID, CountedQuantity: &counted, Source: model.SourceImport,
	}
	if row.Reason != "" {
		in.Reason = &row.Reason
	}
	if row.Note != "" {
		in.Note = &row.Note
	}
	res, err := s.RecordMovement(ctx, orgID, actorID, in)
	if err != nil {
		return model.Movement{}, err
	}
	if len(res.Movements) == 0 {
		return model.Movement{}, fmt.Errorf("count import: no movement recorded")
	}
	return res.Movements[0], nil
}

// RevertImportCount undoes an imported count: a compensating adjustment
// (source "import_rollback", source_uuid the count) takes the counted
// change back. A count without change needs nothing. The rollback is
// refused when the stock would go below zero (the units were sold or
// moved since the count).
func (s *Service) RevertImportCount(ctx context.Context, orgID, actorID int64, id uuid.UUID) error {
	q := s.repo.Q()
	m, err := q.GetStockMovement(ctx, db.GetStockMovementParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return mapNotFound(repository.MapErr(err), ErrMovementNotFound)
	}
	if orgctx.CheckBranch(ctx, m.BranchID) != nil {
		return ErrMovementNotFound
	}
	if m.Kind != model.KindAdjustment || m.Source != model.SourceImport {
		return ErrNotImportCount
	}
	if m.Quantity == 0 {
		return nil
	}
	actor := pgtype.Int8{Int64: actorID, Valid: actorID > 0}
	err = s.repo.InTx(ctx, func(tx *db.Queries) error {
		level, err := lockLevel(ctx, tx, orgID, m.BranchID, m.ProductID, m.VariantID)
		if err != nil {
			return err
		}
		quantity := -m.Quantity
		_, err = apply(ctx, tx, orgID, level, db.InsertStockMovementParams{
			OrganizationID: orgID, BranchID: m.BranchID, ProductID: m.ProductID, VariantID: m.VariantID,
			Kind: model.KindAdjustment, Quantity: quantity, BalanceAfter: level.OnHand + quantity,
			Reason: m.Reason, Note: m.Note, Source: model.SourceImportRollback,
			SourceUuid: pgtype.UUID{Bytes: m.Uuid, Valid: true},
			OccurredAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}, CreatedBy: actor,
		})
		return err
	})
	if errors.Is(repository.MapErr(err), repository.ErrNegativeStock) {
		return &ValidationError{Details: []Detail{{Field: "quantity", Code: "insufficient_stock", Message: "the counted units have left the stock since; the count cannot be rolled back"}}}
	}
	return err
}

// ExportMovements returns the ledger rows of an export (the list filter,
// at most limit rows).
func (s *Service) ExportMovements(ctx context.Context, orgID int64, f model.MovementFilter, limit int32) ([]model.Movement, error) {
	f.Limit, f.Offset = limit, 0
	rows, _, err := s.ListMovements(ctx, orgID, f)
	return rows, err
}
