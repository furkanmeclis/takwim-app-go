package usecase

import (
	"context"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sessionpackages/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
)

// Engines of the session packages (TEC-984): export of the sold packages
// (ioengine), bulk activate / deactivate of the definitions (bulkengine)
// and the sold packages in Cmd+K (searchengine).

// SearchSpec is the Cmd+K spec id of the sold packages.
const SearchSpec = "customer_packages"

// SearchIndexer schedules Cmd+K index updates (fail-soft).
type SearchIndexer interface {
	EnqueueUpsert(ctx context.Context, spec, id string)
	EnqueueDelete(ctx context.Context, spec, id string)
}

// SetIndexer plugs in the search indexer (nil = none).
func (s *Service) SetIndexer(ix SearchIndexer) {
	s.indexer = ix
}

func (s *Service) index(ctx context.Context, orgID int64, id uuid.UUID) {
	if s.indexer != nil {
		s.indexer.EnqueueUpsert(ctx, SearchSpec, searchengine.TenantDocumentID(orgID, id.String()))
	}
}

// ExportCustomerPackages returns up to limit sold packages matching the
// list filter, in the caller's branch scope (like the list).
func (s *Service) ExportCustomerPackages(ctx context.Context, orgID int64, f model.CustomerPackageFilter, limit int32) ([]model.CustomerPackage, error) {
	f.Limit, f.Offset = limit, 0
	items, _, err := s.ListCustomerPackages(ctx, orgID, f)
	return items, err
}

// PackageUUIDs returns up to limit definitions matching the list filter
// (bulk "all matching").
func (s *Service) PackageUUIDs(ctx context.Context, orgID int64, f model.PackageFilter, limit int32) ([]uuid.UUID, error) {
	f.Limit, f.Offset = limit, 0
	items, _, err := s.ListPackages(ctx, orgID, f)
	if err != nil {
		return nil, err
	}
	out := make([]uuid.UUID, 0, len(items))
	for _, p := range items {
		out = append(out, p.UUID)
	}
	return out, nil
}

// SetPackageActive switches a definition on or off and answers the state
// before. Like every definition change it needs access to every branch.
func (s *Service) SetPackageActive(ctx context.Context, orgID int64, id uuid.UUID, active bool) (bool, error) {
	if err := orgctx.RequireAllBranches(ctx); err != nil {
		return false, ErrAllBranches
	}
	q := s.repo.Q()
	row, err := q.GetSessionPackage(ctx, db.GetSessionPackageParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return false, mapNotFound(err, ErrPackageNotFound)
	}
	n, err := q.SetSessionPackageActive(ctx, db.SetSessionPackageActiveParams{OrganizationID: orgID, Uuid: id, IsActive: active})
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, ErrPackageNotFound
	}
	return row.IsActive, nil
}
