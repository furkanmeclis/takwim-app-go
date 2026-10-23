// Package usecase holds the product catalog and stock rules. Every method
// takes the organization id of the caller explicitly (from orgctx.Scope in
// handlers, from the job in workers).
//
// The catalog (products, variants) is business-level: every member with
// tenant.products.read sees all of it. Branch offers and stock are
// branch-level: a member limited to some branches sees stock totals of
// those branches only, changes the offers of those branches only and
// records movements there only.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// SearchSpec is the Cmd+K spec id of products.
const SearchSpec = "products"

// Limits of the product fields.
const (
	maxNameLength     = 160
	maxVariantName    = 120
	maxCodeLength     = 64
	maxDescriptionLen = 2000
	maxSortOrder      = 100000
	maxThreshold      = 1000000
	maxLinks          = 500
	maxVariants       = 200
)

var (
	ErrNotFound         = errors.New("product not found")
	ErrVariantNotFound  = errors.New("product variant not found")
	ErrCodeTaken        = errors.New("a product or variant already uses this stock code")
	ErrVariantNameTaken = errors.New("the product already has a variant with this name")
	// ErrHasStock: the product or variant still has units on hand or
	// reserved; the stock must be moved out (or counted to zero) first.
	ErrHasStock = errors.New("stock is not zero")
)

// Detail is one field-level validation problem.
type Detail struct {
	Field   string
	Message string
	Code    string
}

// ValidationError carries field details; handlers answer 400 VALIDATION_ERROR.
type ValidationError struct {
	Details []Detail
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Details) == 0 {
		return "validation failed"
	}
	return e.Details[0].Field + ": " + e.Details[0].Message
}

func (e *ValidationError) add(field, code, msg string) {
	e.Details = append(e.Details, Detail{Field: field, Code: code, Message: msg})
}

func (e *ValidationError) orNil() error {
	if len(e.Details) == 0 {
		return nil
	}
	return e
}

// IsValidation reports whether err is a field validation error.
func IsValidation(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}

// SearchIndexer schedules Cmd+K index updates (fail-soft).
type SearchIndexer interface {
	EnqueueUpsert(ctx context.Context, spec, id string)
	EnqueueDelete(ctx context.Context, spec, id string)
}

// Service implements the product and stock use cases.
type Service struct {
	repo    *repository.Postgres
	indexer SearchIndexer
	storage storage.Driver
}

// New builds the service. indexer may be nil.
func New(repo *repository.Postgres, indexer SearchIndexer) *Service {
	return &Service{repo: repo, indexer: indexer}
}

// SetStorage attaches the object store of product images. Without it
// image uploads answer ErrStorageUnavailable.
func (s *Service) SetStorage(store storage.Driver) {
	s.storage = store
}

// SortColumns are the sortable list fields.
var SortColumns = map[string]string{
	"name": "name", "sku": "sku", "price": "price", "cost": "cost", "stock": "stock",
	"is_active": "is_active", "created_at": "created_at", "updated_at": "updated_at",
}

// listArgs are the shared filter arguments of the list queries.
type listArgs struct {
	branches             []int64
	stockStatus          []string
	isActive, showOnline pgtype.Bool
	q                    pgtype.Text
}

func (s *Service) listArgs(ctx context.Context, q *db.Queries, orgID int64, f model.ListFilter) (listArgs, error) {
	a := listArgs{
		stockStatus: nilIfEmpty(f.StockStatus), isActive: boolArg(f.IsActive),
		showOnline: boolArg(f.ShowOnline), q: textArg(f.Q),
	}
	if len(f.Branches) > 0 {
		ids, err := branchIDs(ctx, q, orgID, f.Branches)
		if err != nil {
			return a, err
		}
		a.branches = ids
	}
	return a, nil
}

// List returns one page of the catalog with each product's offers and the
// stock of the caller's branches.
func (s *Service) List(ctx context.Context, orgID int64, f model.ListFilter) ([]model.Product, int64, error) {
	if _, ok := SortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "", false
	}
	q := s.repo.Q()
	a, err := s.listArgs(ctx, q, orgID, f)
	if err != nil {
		return nil, 0, err
	}
	scope := orgctx.BranchScope(ctx)
	rows, err := q.ListProducts(ctx, db.ListProductsParams{
		OrganizationID: orgID, StockBranchIds: scope, FilterBranchIds: a.branches, IsActive: a.isActive,
		ShowOnline: a.showOnline, StockStatus: a.stockStatus, Q: a.q, SortField: f.SortField, SortDesc: f.SortDesc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountProducts(ctx, db.CountProductsParams{
		OrganizationID: orgID, StockBranchIds: scope, FilterBranchIds: a.branches, IsActive: a.isActive,
		ShowOnline: a.showOnline, StockStatus: a.stockStatus, Q: a.q,
	})
	if err != nil {
		return nil, 0, err
	}
	conv := make([]db.GetProductRow, 0, len(rows))
	for _, row := range rows {
		conv = append(conv, db.GetProductRow(row))
	}
	out, err := s.mapRows(ctx, q, orgID, conv, false)
	return out, total, err
}

// Facets returns the offering branches of the caller's scope.
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	out := model.Facets{Branch: []model.Facet{}}
	rows, err := s.repo.Q().ListProductBranchFacets(ctx, db.ListProductBranchFacetsParams{OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx)})
	if err != nil {
		return out, err
	}
	for _, row := range rows {
		out.Branch = append(out.Branch, model.Facet{Value: row.Uuid.String(), Label: row.Name, Count: row.Count})
	}
	return out, nil
}

// Get returns one product with its offers, variants and branch stock.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Product, error) {
	q := s.repo.Q()
	row, err := s.load(ctx, q, orgID, id)
	if err != nil {
		return model.Product{}, err
	}
	out, err := s.mapRows(ctx, q, orgID, []db.GetProductRow{row}, true)
	if err != nil {
		return model.Product{}, err
	}
	return out[0], nil
}

// Lookup resolves a scanned barcode or a typed SKU to products / variants.
func (s *Service) Lookup(ctx context.Context, orgID int64, code string) ([]model.LookupMatch, error) {
	code = strings.TrimSpace(code)
	if code == "" || utf8.RuneCountInString(code) > maxCodeLength {
		return nil, &ValidationError{Details: []Detail{{Field: "code", Code: "required", Message: "code is required"}}}
	}
	rows, err := s.repo.Q().LookupStockCode(ctx, db.LookupStockCodeParams{OrganizationID: orgID, Code: code})
	if err != nil {
		return nil, err
	}
	out := make([]model.LookupMatch, 0, len(rows))
	for _, row := range rows {
		p, err := s.Get(ctx, orgID, row.ProductUuid)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		m := model.LookupMatch{Product: p}
		if row.VariantUuid.Valid {
			vid := uuid.UUID(row.VariantUuid.Bytes)
			for i := range p.Variants {
				if p.Variants[i].UUID == vid {
					v := p.Variants[i]
					m.Variant = &v
				}
			}
			if m.Variant == nil {
				continue
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// Create validates and stores a new product, offered in BranchUUIDs at the
// catalog price.
func (s *Service) Create(ctx context.Context, orgID int64, in model.Input) (model.Product, error) {
	f := fields{Unit: "piece", Price: money.Zero(), Cost: money.Zero(), ShowOnline: true, IsActive: true}
	ve := &ValidationError{}
	q := s.repo.Q()
	f.apply(ve, in, true)
	offerBranches, err := s.resolveOfferBranches(ctx, q, orgID, in.BranchUUIDs, ve)
	if err != nil {
		return model.Product{}, err
	}
	if err := ve.orNil(); err != nil {
		return model.Product{}, err
	}
	if err := s.checkCode(ctx, q, orgID, f.SKU, nil, nil); err != nil {
		return model.Product{}, err
	}
	var created db.Product
	err = s.repo.InTx(ctx, func(tx *db.Queries) error {
		row, err := tx.CreateProduct(ctx, db.CreateProductParams{
			OrganizationID: orgID, Name: f.Name, Description: f.Description, Sku: f.SKU, Barcode: f.Barcode,
			Unit: f.Unit, Price: f.Price.Numeric(), Cost: f.Cost.Numeric(), LowStockThreshold: f.LowStock,
			ShowOnline: f.ShowOnline, IsActive: f.IsActive,
		})
		if err != nil {
			return err
		}
		created = row
		for _, branchID := range offerBranches {
			if err := tx.UpsertProductBranch(ctx, db.UpsertProductBranchParams{
				OrganizationID: orgID, ProductID: row.ID, BranchID: branchID, IsActive: true,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return model.Product{}, mapDomain(err)
	}
	s.index(ctx, orgID, created.Uuid)
	return s.Get(ctx, orgID, created.Uuid)
}

// Patch updates the given catalog fields (branch_uuids is ignored; offers
// have their own endpoint).
func (s *Service) Patch(ctx context.Context, orgID int64, id uuid.UUID, in model.Input) (model.Product, error) {
	q := s.repo.Q()
	row, err := s.load(ctx, q, orgID, id)
	if err != nil {
		return model.Product{}, err
	}
	f := fieldsFromRow(row)
	ve := &ValidationError{}
	f.apply(ve, in, false)
	if err := ve.orNil(); err != nil {
		return model.Product{}, err
	}
	if err := s.checkCode(ctx, q, orgID, f.SKU, &row.ID, nil); err != nil {
		return model.Product{}, err
	}
	if _, err := q.UpdateProduct(ctx, db.UpdateProductParams{
		OrganizationID: orgID, ID: row.ID, Name: f.Name, Description: f.Description, Sku: f.SKU, Barcode: f.Barcode,
		Unit: f.Unit, Price: f.Price.Numeric(), Cost: f.Cost.Numeric(), LowStockThreshold: f.LowStock,
		ShowOnline: f.ShowOnline, IsActive: f.IsActive,
	}); err != nil {
		return model.Product{}, mapNotFound(mapDomain(err), ErrNotFound)
	}
	s.index(ctx, orgID, id)
	return s.Get(ctx, orgID, id)
}

// SetActive switches is_active and returns the previous value (bulk engine).
func (s *Service) SetActive(ctx context.Context, orgID int64, id uuid.UUID, active bool) (bool, error) {
	row, err := s.load(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return false, err
	}
	if row.IsActive != active {
		if err := s.repo.Q().SetProductActive(ctx, db.SetProductActiveParams{OrganizationID: orgID, ID: row.ID, IsActive: active}); err != nil {
			return row.IsActive, err
		}
		s.index(ctx, orgID, id)
	}
	return row.IsActive, nil
}

// Delete soft-deletes a product without stock in any branch (the ledger
// stays). A product with units on hand or reserved answers ErrHasStock.
func (s *Service) Delete(ctx context.Context, orgID int64, id uuid.UUID) (model.Product, error) {
	p, err := s.Get(ctx, orgID, id)
	if err != nil {
		return model.Product{}, err
	}
	q := s.repo.Q()
	row, err := s.load(ctx, q, orgID, id)
	if err != nil {
		return model.Product{}, err
	}
	// Stock of every branch, not only the caller's.
	full, err := q.GetProduct(ctx, db.GetProductParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return model.Product{}, mapNotFound(repository.MapErr(err), ErrNotFound)
	}
	if full.StockOnHand != 0 || full.StockReserved != 0 {
		return model.Product{}, ErrHasStock
	}
	n, err := q.SoftDeleteProduct(ctx, db.SoftDeleteProductParams{OrganizationID: orgID, ID: row.ID})
	if err != nil {
		return model.Product{}, err
	}
	if n == 0 {
		return model.Product{}, ErrNotFound
	}
	if s.indexer != nil {
		s.indexer.EnqueueDelete(ctx, SearchSpec, searchengine.TenantDocumentID(orgID, id.String()))
	}
	return p, nil
}

// SetOffers replaces the offers of the branches inside the caller's scope.
// Offers of other branches stay as they are. It returns the offers before
// (activity diff).
func (s *Service) SetOffers(ctx context.Context, orgID int64, id uuid.UUID, in []model.OfferInput) (model.Product, []model.Offer, error) {
	before, err := s.Get(ctx, orgID, id)
	if err != nil {
		return model.Product{}, nil, err
	}
	q := s.repo.Q()
	row, err := s.load(ctx, q, orgID, id)
	if err != nil {
		return model.Product{}, nil, err
	}
	ve := &ValidationError{}
	if len(in) > maxLinks {
		ve.add("offers", "too_many", fmt.Sprintf("at most %d offers", maxLinks))
		return model.Product{}, nil, ve
	}
	uuids := make([]uuid.UUID, 0, len(in))
	for _, o := range in {
		uuids = append(uuids, o.BranchUUID)
	}
	known, err := branchMap(ctx, q, orgID, uuids)
	if err != nil {
		return model.Product{}, nil, err
	}
	type offer struct {
		branchID int64
		price    pgtype.Numeric
		active   bool
	}
	keep := map[int64]offer{}
	for i, o := range in {
		prefix := fmt.Sprintf("offers.%d", i)
		branchID, ok := known[o.BranchUUID]
		if !ok || orgctx.CheckBranch(ctx, branchID) != nil {
			ve.add(prefix+".branch_uuid", "not_found", "branch not found")
			continue
		}
		if _, dup := keep[branchID]; dup {
			ve.add(prefix+".branch_uuid", "duplicate", "branch listed twice")
			continue
		}
		next := offer{branchID: branchID, active: true}
		if o.IsActive != nil {
			next.active = *o.IsActive
		}
		if o.Price != nil {
			if m, ok := parseAmount(ve, prefix+".price", *o.Price); ok {
				next.price = m.Numeric()
			}
		}
		keep[branchID] = next
	}
	if err := ve.orNil(); err != nil {
		return model.Product{}, nil, err
	}
	current, err := q.ListProductBranches(ctx, db.ListProductBranchesParams{OrganizationID: orgID, ProductIds: []int64{row.ID}})
	if err != nil {
		return model.Product{}, nil, err
	}
	err = s.repo.InTx(ctx, func(tx *db.Queries) error {
		for _, c := range current {
			if _, stays := keep[c.BranchID]; !stays && orgctx.CheckBranch(ctx, c.BranchID) == nil {
				if err := tx.DeleteProductBranch(ctx, db.DeleteProductBranchParams{OrganizationID: orgID, ProductID: row.ID, BranchID: c.BranchID}); err != nil {
					return err
				}
			}
		}
		for _, o := range keep {
			if err := tx.UpsertProductBranch(ctx, db.UpsertProductBranchParams{
				OrganizationID: orgID, ProductID: row.ID, BranchID: o.branchID, Price: o.price, IsActive: o.active,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return model.Product{}, nil, err
	}
	after, err := s.Get(ctx, orgID, id)
	return after, before.Offers, err
}

// TargetUUIDs resolves the products matching a list filter (bulk "all
// matching" scope), capped at limit.
func (s *Service) TargetUUIDs(ctx context.Context, orgID int64, f model.ListFilter, limit int32) ([]uuid.UUID, error) {
	q := s.repo.Q()
	a, err := s.listArgs(ctx, q, orgID, f)
	if err != nil {
		return nil, err
	}
	return q.ListProductUUIDs(ctx, db.ListProductUUIDsParams{
		OrganizationID: orgID, StockBranchIds: orgctx.BranchScope(ctx), FilterBranchIds: a.branches,
		IsActive: a.isActive, ShowOnline: a.showOnline, StockStatus: a.stockStatus, Q: a.q, LimitCount: limit,
	})
}

// ExportRows returns every product matching a list filter (capped at limit).
func (s *Service) ExportRows(ctx context.Context, orgID int64, f model.ListFilter, limit int32) ([]model.Product, error) {
	q := s.repo.Q()
	a, err := s.listArgs(ctx, q, orgID, f)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListProductsForExport(ctx, db.ListProductsForExportParams{
		OrganizationID: orgID, StockBranchIds: orgctx.BranchScope(ctx), FilterBranchIds: a.branches,
		IsActive: a.isActive, ShowOnline: a.showOnline, StockStatus: a.stockStatus, Q: a.q, LimitCount: limit,
	})
	if err != nil {
		return nil, err
	}
	conv := make([]db.GetProductRow, 0, len(rows))
	for _, row := range rows {
		conv = append(conv, db.GetProductRow(row))
	}
	return s.mapRows(ctx, q, orgID, conv, false)
}

// ImportRow updates the product with the same SKU, or creates one.
// branchCodes set the offers of a new product. It returns the previous
// state of an update.
func (s *Service) ImportRow(ctx context.Context, orgID int64, branchCodes []string, in model.Input) (uuid.UUID, bool, *model.Product, error) {
	q := s.repo.Q()
	for _, code := range branchCodes {
		branch, err := q.GetBranchByCode(ctx, db.GetBranchByCodeParams{OrganizationID: orgID, Code: strings.ToUpper(strings.TrimSpace(code))})
		if err != nil {
			if errors.Is(repository.MapErr(err), repository.ErrNotFound) {
				return uuid.Nil, false, nil, &ValidationError{Details: []Detail{{Field: "branch_codes", Code: "not_found", Message: "unknown branch code " + code}}}
			}
			return uuid.Nil, false, nil, err
		}
		in.BranchUUIDs = append(in.BranchUUIDs, branch.Uuid)
	}
	if in.SKU != nil {
		existing, err := q.GetProductBySKU(ctx, db.GetProductBySKUParams{OrganizationID: orgID, Sku: strings.TrimSpace(*in.SKU)})
		if err == nil {
			prev, err := s.Get(ctx, orgID, existing.Uuid)
			if err != nil {
				return uuid.Nil, false, nil, err
			}
			in.BranchUUIDs = nil
			p, err := s.Patch(ctx, orgID, existing.Uuid, in)
			if err != nil {
				return uuid.Nil, false, nil, err
			}
			return p.UUID, false, &prev, nil
		}
		if !errors.Is(repository.MapErr(err), repository.ErrNotFound) {
			return uuid.Nil, false, nil, err
		}
	}
	p, err := s.Create(ctx, orgID, in)
	if err != nil {
		return uuid.Nil, false, nil, err
	}
	return p.UUID, true, nil, nil
}

// Activity lists the audit events of a product (catalog changes and its
// stock movements).
func (s *Service) Activity(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]model.ActivityEntry, int64, error) {
	if _, err := s.load(ctx, s.repo.Q(), orgID, id); err != nil {
		return nil, 0, err
	}
	q := s.repo.Q()
	rows, err := q.ListOrganizationResourceActivity(ctx, db.ListOrganizationResourceActivityParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, Resource: model.Slug,
		ResourceUuid: pgtype.UUID{Bytes: id, Valid: true}, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountOrganizationResourceActivity(ctx, db.CountOrganizationResourceActivityParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, Resource: model.Slug,
		ResourceUuid: pgtype.UUID{Bytes: id, Valid: true},
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.ActivityEntry, 0, len(rows))
	for _, row := range rows {
		e := model.ActivityEntry{UUID: row.Uuid, Action: row.Action, Payload: json.RawMessage(row.Payload), CreatedAt: row.CreatedAt.Time}
		if row.ActorUuid.Valid {
			e.Actor = &model.ActivityActor{
				UUID: uuid.UUID(row.ActorUuid.Bytes), Name: row.ActorName.String,
				Surname: row.ActorSurname.String, Email: row.ActorEmail.String,
			}
		}
		out = append(out, e)
	}
	return out, total, nil
}

// load reads one live product of the organization with the stock of the
// caller's branches.
func (s *Service) load(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (db.GetProductRow, error) {
	row, err := q.GetProduct(ctx, db.GetProductParams{OrganizationID: orgID, Uuid: id, StockBranchIds: orgctx.BranchScope(ctx)})
	if err != nil {
		return db.GetProductRow{}, mapNotFound(repository.MapErr(err), ErrNotFound)
	}
	return row, nil
}

// mapRows maps product rows with their offers (and variants and branch
// stock when detail).
func (s *Service) mapRows(ctx context.Context, q *db.Queries, orgID int64, rows []db.GetProductRow, detail bool) ([]model.Product, error) {
	out := make([]model.Product, 0, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	offers, err := q.ListProductBranches(ctx, db.ListProductBranchesParams{OrganizationID: orgID, ProductIds: ids})
	if err != nil {
		return nil, err
	}
	byProduct := map[int64][]db.ListProductBranchesRow{}
	for _, o := range offers {
		byProduct[o.ProductID] = append(byProduct[o.ProductID], o)
	}
	variantsBy := map[int64][]db.ProductVariant{}
	levelsBy := map[int64][]db.ListStockLevelsRow{}
	if detail {
		variants, err := q.ListProductVariants(ctx, db.ListProductVariantsParams{OrganizationID: orgID, ProductIds: ids})
		if err != nil {
			return nil, err
		}
		for _, v := range variants {
			variantsBy[v.ProductID] = append(variantsBy[v.ProductID], v)
		}
		levels, err := q.ListStockLevels(ctx, db.ListStockLevelsParams{OrganizationID: orgID, ProductIds: ids, BranchIds: orgctx.BranchScope(ctx)})
		if err != nil {
			return nil, err
		}
		for _, l := range levels {
			levelsBy[l.ProductID] = append(levelsBy[l.ProductID], l)
		}
	}
	currency := currencyOf(ctx)
	orgUUID, _ := scopeUUID(ctx)
	for _, row := range rows {
		p := MapProduct(row, orgUUID, currency)
		for _, o := range byProduct[row.ID] {
			p.Offers = append(p.Offers, mapOffer(ctx, o, p))
		}
		if detail {
			p.Variants = []model.Variant{}
			p.Levels = []model.StockLevel{}
			variantStock := map[int64]*model.Stock{}
			for _, l := range levelsBy[row.ID] {
				p.Levels = append(p.Levels, mapLevel(l, row.LowStockThreshold))
				if l.VariantID.Valid {
					st := variantStock[l.VariantID.Int64]
					if st == nil {
						st = &model.Stock{}
						variantStock[l.VariantID.Int64] = st
					}
					st.OnHand += int64(l.OnHand)
					st.Reserved += int64(l.Reserved)
				}
			}
			for _, v := range variantsBy[row.ID] {
				mv := mapVariant(v, p)
				if st := variantStock[v.ID]; st != nil {
					mv.Stock = stockOf(st.OnHand, st.Reserved, row.LowStockThreshold)
				}
				p.Variants = append(p.Variants, mv)
			}
		}
		out = append(out, p)
	}
	return out, nil
}

// resolveOfferBranches maps the branch uuids of a new product; unknown
// branches and branches outside the caller's scope are the same error.
func (s *Service) resolveOfferBranches(ctx context.Context, q *db.Queries, orgID int64, ids []uuid.UUID, ve *ValidationError) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	known, err := branchMap(ctx, q, orgID, ids)
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(ids))
	for i, id := range ids {
		branchID, ok := known[id]
		if !ok || orgctx.CheckBranch(ctx, branchID) != nil {
			ve.add(fmt.Sprintf("branch_uuids.%d", i), "not_found", "branch not found")
			continue
		}
		if !slices.Contains(out, branchID) {
			out = append(out, branchID)
		}
	}
	return out, nil
}

// checkCode answers ErrCodeTaken when another live product or variant of
// the organization uses the SKU (the unique indexes are the backstop
// inside each table).
func (s *Service) checkCode(ctx context.Context, q *db.Queries, orgID int64, sku string, excludeProduct, excludeVariant *int64) error {
	taken, err := q.StockCodeTaken(ctx, db.StockCodeTakenParams{
		OrganizationID: orgID, Sku: sku, ExcludeProductID: int8Ptr(excludeProduct), ExcludeVariantID: int8Ptr(excludeVariant),
	})
	if err != nil {
		return err
	}
	if taken {
		return ErrCodeTaken
	}
	return nil
}

func (s *Service) index(ctx context.Context, orgID int64, id uuid.UUID) {
	if s.indexer != nil {
		s.indexer.EnqueueUpsert(ctx, SearchSpec, searchengine.TenantDocumentID(orgID, id.String()))
	}
}

// fields are the writable columns of a product.
type fields struct {
	Name, Description    string
	SKU, Barcode, Unit   string
	Price, Cost          money.Money
	LowStock             int32
	ShowOnline, IsActive bool
}

func fieldsFromRow(row db.GetProductRow) fields {
	return fields{
		Name: row.Name, Description: row.Description, SKU: row.Sku, Barcode: row.Barcode, Unit: row.Unit,
		Price: money.FromNumericOrZero(row.Price), Cost: money.FromNumericOrZero(row.Cost),
		LowStock: row.LowStockThreshold, ShowOnline: row.ShowOnline, IsActive: row.IsActive,
	}
}

func (f *fields) apply(ve *ValidationError, in model.Input, create bool) {
	text("name", in.Name, &f.Name, maxNameLength, true, create, ve)
	text("description", in.Description, &f.Description, maxDescriptionLen, false, create, ve)
	code("sku", in.SKU, &f.SKU, true, create, ve)
	code("barcode", in.Barcode, &f.Barcode, false, create, ve)
	if in.Unit != nil {
		if !slices.Contains(model.Units, *in.Unit) {
			ve.add("unit", "invalid", "unit must be one of "+strings.Join(model.Units, ", "))
		} else {
			f.Unit = *in.Unit
		}
	}
	if in.Price != nil {
		if m, ok := parseAmount(ve, "price", *in.Price); ok {
			f.Price = m
		}
	} else if create {
		ve.add("price", "required", "price is required")
	}
	if in.Cost != nil {
		if m, ok := parseAmount(ve, "cost", *in.Cost); ok {
			f.Cost = m
		}
	}
	if in.LowStockThreshold != nil {
		if v := *in.LowStockThreshold; v < 0 || v > maxThreshold {
			ve.add("low_stock_threshold", "invalid", fmt.Sprintf("low_stock_threshold must be between 0 and %d", maxThreshold))
		} else {
			f.LowStock = v
		}
	}
	if in.ShowOnline != nil {
		f.ShowOnline = *in.ShowOnline
	}
	if in.IsActive != nil {
		f.IsActive = *in.IsActive
	}
}

// text validates a trimmed text field.
func text(field string, src *string, dst *string, max int, required, create bool, ve *ValidationError) {
	if src == nil {
		if create && required {
			ve.add(field, "required", field+" is required")
		}
		return
	}
	v := strings.TrimSpace(*src)
	switch {
	case required && v == "":
		ve.add(field, "required", field+" is required")
	case utf8.RuneCountInString(v) > max:
		ve.add(field, "too_long", fmt.Sprintf("%s must be at most %d characters", field, max))
	default:
		*dst = v
	}
}

// code validates a SKU or barcode: no spaces, at most 64 characters.
func code(field string, src *string, dst *string, required, create bool, ve *ValidationError) {
	if src == nil {
		if create && required {
			ve.add(field, "required", field+" is required")
		}
		return
	}
	v := strings.TrimSpace(*src)
	switch {
	case required && v == "":
		ve.add(field, "required", field+" is required")
	case utf8.RuneCountInString(v) > maxCodeLength:
		ve.add(field, "too_long", fmt.Sprintf("%s must be at most %d characters", field, maxCodeLength))
	case strings.ContainsAny(v, " \t\r\n"):
		ve.add(field, "invalid", field+" must not contain spaces")
	default:
		*dst = v
	}
}

// parseAmount reads a non-negative amount with at most two decimals.
func parseAmount(ve *ValidationError, field string, raw json.Number) (money.Money, bool) {
	m, err := money.Parse(strings.TrimSpace(raw.String()))
	if err != nil || m.IsNegative() {
		ve.add(field, "invalid", field+" must be a non-negative amount with at most 2 decimals")
		return money.Money{}, false
	}
	return m, true
}

func currencyOf(ctx context.Context) string {
	if scope, ok := orgctx.From(ctx); ok && scope.Currency != "" {
		return scope.Currency
	}
	return string(money.TRY)
}

func scopeUUID(ctx context.Context) (uuid.UUID, bool) {
	scope, ok := orgctx.From(ctx)
	if !ok || scope.UUID == uuid.Nil {
		return uuid.Nil, false
	}
	return scope.UUID, true
}

// PublicImagePath is the API-owned public URL of a product image; booking
// pages and the mobile app use it as is.
func PublicImagePath(orgUUID, imageUUID uuid.UUID) string {
	return "/v1/public/media/products/" + orgUUID.String() + "/images/" + imageUUID.String()
}

// stockOf builds a stock summary; the status compares available units
// with the low stock threshold.
func stockOf(onHand, reserved int64, threshold int32) model.Stock {
	st := model.Stock{OnHand: onHand, Reserved: reserved, Available: onHand - reserved}
	switch {
	case st.Available <= 0:
		st.Status = model.StockOut
	case st.Available <= int64(threshold):
		st.Status = model.StockLow
	default:
		st.Status = model.StockIn
	}
	return st
}

// MapProduct maps a product row (list, detail and export share the columns).
func MapProduct(row db.GetProductRow, orgUUID uuid.UUID, currency string) model.Product {
	p := model.Product{
		UUID: row.Uuid, Name: row.Name, Description: row.Description, SKU: row.Sku, Barcode: row.Barcode,
		Unit: row.Unit, Price: money.FromNumericOrZero(row.Price), Cost: money.FromNumericOrZero(row.Cost),
		Currency: currency, LowStockThreshold: row.LowStockThreshold, ShowOnline: row.ShowOnline,
		IsActive: row.IsActive, VariantsCount: row.VariantsCount,
		Stock:  stockOf(row.StockOnHand, row.StockReserved, row.LowStockThreshold),
		Offers: []model.Offer{}, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if row.ImageUuid.Valid && orgUUID != uuid.Nil {
		url := PublicImagePath(orgUUID, uuid.UUID(row.ImageUuid.Bytes))
		p.ImageURL = &url
	}
	return p
}

func mapOffer(ctx context.Context, o db.ListProductBranchesRow, p model.Product) model.Offer {
	out := model.Offer{
		BranchUUID: o.BranchUuid, BranchName: o.BranchName, BranchCode: o.BranchCode,
		EffectivePrice: p.Price, IsActive: o.IsActive, Editable: orgctx.CheckBranch(ctx, o.BranchID) == nil,
	}
	if o.Price.Valid {
		price := money.FromNumericOrZero(o.Price)
		out.Price, out.EffectivePrice = &price, price
	}
	return out
}

func mapVariant(v db.ProductVariant, p model.Product) model.Variant {
	out := model.Variant{
		UUID: v.Uuid, Name: v.Name, SKU: v.Sku, Barcode: v.Barcode, EffectivePrice: p.Price, EffectiveCost: p.Cost,
		IsActive: v.IsActive, SortOrder: v.SortOrder, Stock: stockOf(0, 0, p.LowStockThreshold),
		CreatedAt: v.CreatedAt.Time, UpdatedAt: v.UpdatedAt.Time,
	}
	if v.Price.Valid {
		price := money.FromNumericOrZero(v.Price)
		out.Price, out.EffectivePrice = &price, price
	}
	if v.Cost.Valid {
		cost := money.FromNumericOrZero(v.Cost)
		out.Cost, out.EffectiveCost = &cost, cost
	}
	return out
}

func mapLevel(l db.ListStockLevelsRow, threshold int32) model.StockLevel {
	st := stockOf(int64(l.OnHand), int64(l.Reserved), threshold)
	out := model.StockLevel{
		BranchUUID: l.BranchUuid, BranchName: l.BranchName, BranchCode: l.BranchCode,
		VariantName: l.VariantName.String, VariantSKU: l.VariantSku.String,
		OnHand: st.OnHand, Reserved: st.Reserved, Available: st.Available, Status: st.Status, UpdatedAt: l.UpdatedAt.Time,
	}
	if l.VariantUuid.Valid {
		id := uuid.UUID(l.VariantUuid.Bytes)
		out.VariantUUID = &id
	}
	return out
}

// branchMap resolves branch uuids of the organization to ids.
func branchMap(ctx context.Context, q *db.Queries, orgID int64, list []uuid.UUID) (map[uuid.UUID]int64, error) {
	known := map[uuid.UUID]int64{}
	if len(list) == 0 {
		return known, nil
	}
	rows, err := q.ListBranchIDsByUUIDs(ctx, db.ListBranchIDsByUUIDsParams{OrganizationID: orgID, Uuids: list})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		known[r.Uuid] = r.ID
	}
	return known, nil
}

// branchIDs maps a filter of branch uuids; unknown branches match nothing
// (an empty, non-nil filter).
func branchIDs(ctx context.Context, q *db.Queries, orgID int64, ids []uuid.UUID) ([]int64, error) {
	rows, err := q.ListBranchIDsByUUIDs(ctx, db.ListBranchIDsByUUIDsParams{OrganizationID: orgID, Uuids: ids})
	if err != nil {
		return nil, err
	}
	out := []int64{}
	for _, row := range rows {
		out = append(out, row.ID)
	}
	return out, nil
}

func mapNotFound(err, target error) error {
	if errors.Is(err, repository.ErrNotFound) {
		return target
	}
	return err
}

func mapDomain(err error) error {
	switch mapped := repository.MapErr(err); {
	case errors.Is(mapped, repository.ErrCodeTaken):
		return ErrCodeTaken
	case errors.Is(mapped, repository.ErrVariantNameTaken):
		return ErrVariantNameTaken
	case errors.Is(mapped, repository.ErrNotFound):
		return mapped
	}
	return err
}

func nilIfEmpty(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

func textArg(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func boolArg(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}

func int8Ptr(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}
