package usecase

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/repository"
)

// Categories lists every live category with its service count (the catalog
// has few, so there is no paging).
func (s *Service) Categories(ctx context.Context, orgID int64) ([]model.Category, error) {
	rows, err := s.repo.Q().ListServiceCategories(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]model.Category, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.Category{
			UUID: row.Uuid, Name: row.Name, Description: row.Description, SortOrder: row.SortOrder,
			ServicesCount: row.ServicesCount, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		})
	}
	return out, nil
}

// CreateCategory stores a new category.
func (s *Service) CreateCategory(ctx context.Context, orgID int64, in model.CategoryInput) (model.Category, error) {
	f := categoryFields{}
	ve := &ValidationError{}
	f.apply(ve, in, true)
	if err := ve.orNil(); err != nil {
		return model.Category{}, err
	}
	row, err := s.repo.Q().CreateServiceCategory(ctx, db.CreateServiceCategoryParams{
		OrganizationID: orgID, Name: f.Name, Description: f.Description, SortOrder: f.SortOrder,
	})
	if err != nil {
		return model.Category{}, mapNameTaken(err)
	}
	return mapCategory(row, 0), nil
}

// PatchCategory updates the given category fields. Renaming reindexes its
// services (the category name is searchable).
func (s *Service) PatchCategory(ctx context.Context, orgID int64, id uuid.UUID, in model.CategoryInput) (model.Category, model.Category, error) {
	q := s.repo.Q()
	row, err := q.GetServiceCategory(ctx, db.GetServiceCategoryParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return model.Category{}, model.Category{}, mapNotFound(repository.MapErr(err), ErrCategoryNotFound)
	}
	before := mapCategory(row, 0)
	f := categoryFields{Name: row.Name, Description: row.Description, SortOrder: row.SortOrder}
	ve := &ValidationError{}
	f.apply(ve, in, false)
	if err := ve.orNil(); err != nil {
		return model.Category{}, model.Category{}, err
	}
	updated, err := q.UpdateServiceCategory(ctx, db.UpdateServiceCategoryParams{
		OrganizationID: orgID, ID: row.ID, Name: f.Name, Description: f.Description, SortOrder: f.SortOrder,
	})
	if err != nil {
		return model.Category{}, model.Category{}, mapNotFound(mapNameTaken(err), ErrCategoryNotFound)
	}
	if updated.Name != row.Name {
		s.reindexCategory(ctx, orgID, row.ID)
	}
	return before, mapCategory(updated, 0), nil
}

// DeleteCategory soft-deletes a category; its services become
// uncategorized.
func (s *Service) DeleteCategory(ctx context.Context, orgID int64, id uuid.UUID) (model.Category, error) {
	q := s.repo.Q()
	row, err := q.GetServiceCategory(ctx, db.GetServiceCategoryParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return model.Category{}, mapNotFound(repository.MapErr(err), ErrCategoryNotFound)
	}
	ids, err := s.categoryServiceUUIDs(ctx, orgID, row.ID)
	if err != nil {
		return model.Category{}, err
	}
	err = s.repo.InTx(ctx, func(tx *db.Queries) error {
		if err := tx.UncategorizeServices(ctx, db.UncategorizeServicesParams{OrganizationID: orgID, CategoryID: pgInt8(row.ID)}); err != nil {
			return err
		}
		n, err := tx.SoftDeleteServiceCategory(ctx, db.SoftDeleteServiceCategoryParams{OrganizationID: orgID, ID: row.ID})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrCategoryNotFound
		}
		return nil
	})
	if err != nil {
		return model.Category{}, err
	}
	for _, sid := range ids {
		s.index(ctx, orgID, sid)
	}
	return mapCategory(row, int64(len(ids))), nil
}

func (s *Service) reindexCategory(ctx context.Context, orgID, categoryID int64) {
	ids, err := s.categoryServiceUUIDs(ctx, orgID, categoryID)
	if err != nil {
		return
	}
	for _, id := range ids {
		s.index(ctx, orgID, id)
	}
}

func (s *Service) categoryServiceUUIDs(ctx context.Context, orgID, categoryID int64) ([]uuid.UUID, error) {
	return s.repo.Q().ListServiceUUIDs(ctx, db.ListServiceUUIDsParams{
		OrganizationID: orgID, CategoryIds: []int64{categoryID}, LimitCount: 10000,
	})
}

type categoryFields struct {
	Name, Description string
	SortOrder         int32
}

func (f *categoryFields) apply(ve *ValidationError, in model.CategoryInput, create bool) {
	if in.Name != nil {
		v := strings.TrimSpace(*in.Name)
		switch {
		case v == "":
			ve.add("name", "required", "name is required")
		case utf8.RuneCountInString(v) > maxCategoryName:
			ve.add("name", "too_long", fmt.Sprintf("name must be at most %d characters", maxCategoryName))
		default:
			f.Name = v
		}
	} else if create {
		ve.add("name", "required", "name is required")
	}
	if in.Description != nil {
		v := strings.TrimSpace(*in.Description)
		if utf8.RuneCountInString(v) > maxDescriptionLen {
			ve.add("description", "too_long", fmt.Sprintf("description must be at most %d characters", maxDescriptionLen))
		} else {
			f.Description = v
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

func mapCategory(row db.ServiceCategory, count int64) model.Category {
	return model.Category{
		UUID: row.Uuid, Name: row.Name, Description: row.Description, SortOrder: row.SortOrder,
		ServicesCount: count, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}
