package usecase

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
)

// CategorySortColumns are the sortable category list fields.
var CategorySortColumns = map[string]string{
	"name": "name", "kind": "kind", "status": "status", "sort_order": "sort_order", "created_at": "created_at",
}

// ListCategories returns one page of the organization's categories. The
// categories are business-wide: every member with finance read sees them.
func (s *Service) ListCategories(ctx context.Context, orgID int64, f model.CategoryFilter) ([]model.Category, int64, error) {
	if _, ok := CategorySortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "", false
	}
	q := s.repo.Q()
	rows, err := q.ListFinanceCategories(ctx, db.ListFinanceCategoriesParams{
		OrganizationID: orgID, Kinds: nilIfEmpty(f.Kinds), Statuses: nilIfEmpty(f.Statuses), Q: textArg(f.Q),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountFinanceCategories(ctx, db.CountFinanceCategoriesParams{
		OrganizationID: orgID, Kinds: nilIfEmpty(f.Kinds), Statuses: nilIfEmpty(f.Statuses), Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Category, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapCategory(db.GetFinanceCategoryRow(r)))
	}
	return out, total, nil
}

// GetCategory returns one category.
func (s *Service) GetCategory(ctx context.Context, orgID int64, id uuid.UUID) (model.Category, error) {
	row, err := s.repo.Q().GetFinanceCategory(ctx, db.GetFinanceCategoryParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if isNotFound(err) {
			return model.Category{}, ErrCategoryNotFound
		}
		return model.Category{}, err
	}
	return mapCategory(row), nil
}

// CreateCategory adds a business category. Categories are business-wide,
// so only members with every branch manage them.
func (s *Service) CreateCategory(ctx context.Context, orgID int64, in model.CategoryInput) (model.Category, error) {
	if err := orgctx.RequireAllBranches(ctx); err != nil {
		return model.Category{}, err
	}
	ve := &ValidationError{}
	kind := ""
	switch {
	case in.Kind == nil:
		ve.add("kind", "required", "kind is required")
	case !slices.Contains(model.Kinds, *in.Kind):
		ve.add("kind", "invalid", "kind must be income or expense")
	default:
		kind = *in.Kind
	}
	f := categoryFields{Status: model.StatusActive, SortOrder: 500}
	f.apply(ve, in, true)
	if err := ve.orNil(); err != nil {
		return model.Category{}, err
	}
	row, err := s.repo.Q().InsertFinanceCategory(ctx, db.InsertFinanceCategoryParams{
		OrganizationID: orgID, Kind: kind, Name: f.Name, Status: f.Status, SortOrder: f.SortOrder,
	})
	if err != nil {
		return model.Category{}, mapRepoErr(err)
	}
	return s.GetCategory(ctx, orgID, row.Uuid)
}

// PatchCategory renames, re-sorts, archives or activates a category. The
// kind is fixed: records of the category keep their kind.
func (s *Service) PatchCategory(ctx context.Context, orgID int64, id uuid.UUID, in model.CategoryInput) (model.Category, model.Category, error) {
	if err := orgctx.RequireAllBranches(ctx); err != nil {
		return model.Category{}, model.Category{}, err
	}
	q := s.repo.Q()
	row, err := q.GetFinanceCategory(ctx, db.GetFinanceCategoryParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if isNotFound(err) {
			return model.Category{}, model.Category{}, ErrCategoryNotFound
		}
		return model.Category{}, model.Category{}, err
	}
	before := mapCategory(row)
	ve := &ValidationError{}
	if in.Kind != nil && *in.Kind != row.Kind {
		ve.add("kind", "immutable", "the kind of a category cannot change")
	}
	f := categoryFields{Name: row.Name, Status: row.Status, SortOrder: row.SortOrder}
	f.apply(ve, in, false)
	if err := ve.orNil(); err != nil {
		return model.Category{}, model.Category{}, err
	}
	if _, err := q.UpdateFinanceCategory(ctx, db.UpdateFinanceCategoryParams{
		OrganizationID: orgID, ID: row.ID, Name: f.Name, Status: f.Status, SortOrder: f.SortOrder,
	}); err != nil {
		return model.Category{}, model.Category{}, mapRepoErr(err)
	}
	after, err := s.GetCategory(ctx, orgID, id)
	return before, after, err
}

// DeleteCategory removes a business category no record uses; default and
// used categories are archived instead.
func (s *Service) DeleteCategory(ctx context.Context, orgID int64, id uuid.UUID) (model.Category, error) {
	if err := orgctx.RequireAllBranches(ctx); err != nil {
		return model.Category{}, err
	}
	q := s.repo.Q()
	row, err := q.GetFinanceCategory(ctx, db.GetFinanceCategoryParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if isNotFound(err) {
			return model.Category{}, ErrCategoryNotFound
		}
		return model.Category{}, err
	}
	n, err := q.DeleteFinanceCategory(ctx, db.DeleteFinanceCategoryParams{OrganizationID: orgID, ID: row.ID})
	if err != nil {
		return model.Category{}, err
	}
	if n == 0 {
		return model.Category{}, ErrCategoryInUse
	}
	return mapCategory(row), nil
}

type categoryFields struct {
	Name      string
	Status    string
	SortOrder int32
}

func (f *categoryFields) apply(ve *ValidationError, in model.CategoryInput, create bool) {
	if in.Name == nil {
		if create {
			ve.add("name", "required", "name is required")
		}
	} else {
		v := strings.TrimSpace(*in.Name)
		switch {
		case v == "":
			ve.add("name", "required", "name is required")
		case utf8.RuneCountInString(v) > maxCategoryName:
			ve.add("name", "too_long", fmt.Sprintf("name must be at most %d characters", maxCategoryName))
		default:
			f.Name = v
		}
	}
	if in.Status != nil {
		if !slices.Contains(model.Statuses, *in.Status) {
			ve.add("status", "invalid", "status must be active or archived")
		} else {
			f.Status = *in.Status
		}
	}
	if in.SortOrder != nil {
		if *in.SortOrder < 0 || *in.SortOrder > maxSortOrder {
			ve.add("sort_order", "invalid", fmt.Sprintf("sort_order must be between 0 and %d", maxSortOrder))
		} else {
			f.SortOrder = *in.SortOrder
		}
	}
}

func mapCategory(r db.GetFinanceCategoryRow) model.Category {
	c := model.Category{
		UUID: r.Uuid, Kind: r.Kind, Name: r.Name, IsSystem: r.IsSystem, Status: r.Status, SortOrder: r.SortOrder,
		EntryCount: r.EntryCount, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.Code.Valid {
		code := r.Code.String
		c.Code = &code
	}
	return c
}
