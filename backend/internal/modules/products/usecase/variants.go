package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/repository"
)

// A product with variants keeps its stock on the variants; a product
// without variants keeps it on the product row. Adding the first variant
// therefore needs the product-level stock at zero, and a variant with
// stock cannot be deleted.

// CreateVariant adds a variant with its own stock code.
func (s *Service) CreateVariant(ctx context.Context, orgID int64, productID uuid.UUID, in model.VariantInput) (model.Product, model.Variant, error) {
	q := s.repo.Q()
	product, err := s.load(ctx, q, orgID, productID)
	if err != nil {
		return model.Product{}, model.Variant{}, err
	}
	if product.VariantsCount >= maxVariants {
		return model.Product{}, model.Variant{}, &ValidationError{Details: []Detail{{
			Field: "variants", Code: "too_many", Message: fmt.Sprintf("at most %d variants", maxVariants),
		}}}
	}
	f := variantFields{IsActive: true}
	ve := &ValidationError{}
	f.apply(ve, in, true)
	if err := ve.orNil(); err != nil {
		return model.Product{}, model.Variant{}, err
	}
	if err := s.checkCode(ctx, q, orgID, f.SKU, nil, nil); err != nil {
		return model.Product{}, model.Variant{}, err
	}
	if err := s.checkVariantName(ctx, q, orgID, product.ID, f.Name, nil); err != nil {
		return model.Product{}, model.Variant{}, err
	}
	base, err := q.SumStockItem(ctx, db.SumStockItemParams{OrganizationID: orgID, ProductID: product.ID})
	if err != nil {
		return model.Product{}, model.Variant{}, err
	}
	if base.OnHand != 0 || base.Reserved != 0 {
		return model.Product{}, model.Variant{}, ErrHasStock
	}
	row, err := q.CreateProductVariant(ctx, db.CreateProductVariantParams{
		OrganizationID: orgID, ProductID: product.ID, Name: f.Name, Sku: f.SKU, Barcode: f.Barcode,
		Price: f.Price, Cost: f.Cost, IsActive: f.IsActive, SortOrder: f.SortOrder,
	})
	if err != nil {
		return model.Product{}, model.Variant{}, mapDomain(err)
	}
	s.index(ctx, orgID, productID)
	return s.withVariant(ctx, orgID, productID, row.Uuid)
}

// PatchVariant updates the given variant fields. It returns the variant
// before the change (activity diff).
func (s *Service) PatchVariant(ctx context.Context, orgID int64, productID, variantID uuid.UUID, in model.VariantInput) (model.Product, model.Variant, model.Variant, error) {
	q := s.repo.Q()
	product, err := s.load(ctx, q, orgID, productID)
	if err != nil {
		return model.Product{}, model.Variant{}, model.Variant{}, err
	}
	_, before, err := s.withVariant(ctx, orgID, productID, variantID)
	if err != nil {
		return model.Product{}, model.Variant{}, model.Variant{}, err
	}
	row, err := s.loadVariant(ctx, q, orgID, product.ID, variantID)
	if err != nil {
		return model.Product{}, model.Variant{}, model.Variant{}, err
	}
	f := variantFields{
		Name: row.Name, SKU: row.Sku, Barcode: row.Barcode, Price: row.Price, Cost: row.Cost,
		IsActive: row.IsActive, SortOrder: row.SortOrder,
	}
	ve := &ValidationError{}
	f.apply(ve, in, false)
	if err := ve.orNil(); err != nil {
		return model.Product{}, model.Variant{}, model.Variant{}, err
	}
	if err := s.checkCode(ctx, q, orgID, f.SKU, nil, &row.ID); err != nil {
		return model.Product{}, model.Variant{}, model.Variant{}, err
	}
	if err := s.checkVariantName(ctx, q, orgID, product.ID, f.Name, &row.ID); err != nil {
		return model.Product{}, model.Variant{}, model.Variant{}, err
	}
	if _, err := q.UpdateProductVariant(ctx, db.UpdateProductVariantParams{
		OrganizationID: orgID, ID: row.ID, Name: f.Name, Sku: f.SKU, Barcode: f.Barcode,
		Price: f.Price, Cost: f.Cost, IsActive: f.IsActive, SortOrder: f.SortOrder,
	}); err != nil {
		return model.Product{}, model.Variant{}, model.Variant{}, mapNotFound(mapDomain(err), ErrVariantNotFound)
	}
	s.index(ctx, orgID, productID)
	p, after, err := s.withVariant(ctx, orgID, productID, variantID)
	return p, after, before, err
}

// DeleteVariant soft-deletes a variant without stock in any branch.
func (s *Service) DeleteVariant(ctx context.Context, orgID int64, productID, variantID uuid.UUID) (model.Product, model.Variant, error) {
	q := s.repo.Q()
	product, err := s.load(ctx, q, orgID, productID)
	if err != nil {
		return model.Product{}, model.Variant{}, err
	}
	_, gone, err := s.withVariant(ctx, orgID, productID, variantID)
	if err != nil {
		return model.Product{}, model.Variant{}, err
	}
	row, err := s.loadVariant(ctx, q, orgID, product.ID, variantID)
	if err != nil {
		return model.Product{}, model.Variant{}, err
	}
	sum, err := q.SumStockItem(ctx, db.SumStockItemParams{
		OrganizationID: orgID, ProductID: product.ID, VariantID: pgtype.Int8{Int64: row.ID, Valid: true},
	})
	if err != nil {
		return model.Product{}, model.Variant{}, err
	}
	if sum.OnHand != 0 || sum.Reserved != 0 {
		return model.Product{}, model.Variant{}, ErrHasStock
	}
	n, err := q.SoftDeleteProductVariant(ctx, db.SoftDeleteProductVariantParams{OrganizationID: orgID, ID: row.ID})
	if err != nil {
		return model.Product{}, model.Variant{}, err
	}
	if n == 0 {
		return model.Product{}, model.Variant{}, ErrVariantNotFound
	}
	s.index(ctx, orgID, productID)
	p, err := s.Get(ctx, orgID, productID)
	return p, gone, err
}

// withVariant returns the product detail and one of its variants.
func (s *Service) withVariant(ctx context.Context, orgID int64, productID, variantID uuid.UUID) (model.Product, model.Variant, error) {
	p, err := s.Get(ctx, orgID, productID)
	if err != nil {
		return model.Product{}, model.Variant{}, err
	}
	for _, v := range p.Variants {
		if v.UUID == variantID {
			return p, v, nil
		}
	}
	return model.Product{}, model.Variant{}, ErrVariantNotFound
}

func (s *Service) loadVariant(ctx context.Context, q *db.Queries, orgID, productID int64, id uuid.UUID) (db.ProductVariant, error) {
	row, err := q.GetProductVariant(ctx, db.GetProductVariantParams{OrganizationID: orgID, ProductID: productID, Uuid: id})
	if err != nil {
		return db.ProductVariant{}, mapNotFound(repository.MapErr(err), ErrVariantNotFound)
	}
	return row, nil
}

func (s *Service) checkVariantName(ctx context.Context, q *db.Queries, orgID, productID int64, name string, exclude *int64) error {
	taken, err := q.ProductVariantNameTaken(ctx, db.ProductVariantNameTakenParams{
		OrganizationID: orgID, ProductID: productID, Name: name, ExcludeID: int8Ptr(exclude),
	})
	if err != nil {
		return err
	}
	if taken {
		return ErrVariantNameTaken
	}
	return nil
}

// variantFields are the writable columns of a variant.
type variantFields struct {
	Name, SKU, Barcode string
	Price, Cost        pgtype.Numeric
	IsActive           bool
	SortOrder          int32
}

func (f *variantFields) apply(ve *ValidationError, in model.VariantInput, create bool) {
	text("name", in.Name, &f.Name, maxVariantName, true, create, ve)
	code("sku", in.SKU, &f.SKU, true, create, ve)
	code("barcode", in.Barcode, &f.Barcode, false, create, ve)
	for _, a := range []struct {
		field string
		src   model.OptionalNumber
		dst   *pgtype.Numeric
	}{{"price", in.Price, &f.Price}, {"cost", in.Cost, &f.Cost}} {
		if !a.src.Set {
			continue
		}
		if a.src.Value == nil {
			*a.dst = pgtype.Numeric{}
			continue
		}
		if m, ok := parseAmount(ve, a.field, *a.src.Value); ok {
			*a.dst = m.Numeric()
		}
	}
	if in.IsActive != nil {
		f.IsActive = *in.IsActive
	}
	if in.SortOrder != nil {
		if *in.SortOrder < 0 || *in.SortOrder > maxSortOrder {
			ve.add("sort_order", "invalid", fmt.Sprintf("sort_order must be between 0 and %d", maxSortOrder))
		} else {
			f.SortOrder = *in.SortOrder
		}
	}
}

// IsHasStock reports whether err is the "stock is not zero" conflict.
func IsHasStock(err error) bool { return errors.Is(err, ErrHasStock) }
