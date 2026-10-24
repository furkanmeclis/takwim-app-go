package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/servicebundles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
)

// SearchSpec is the Cmd+K spec id of service bundles.
const SearchSpec = "service_bundles"

// SearchIndexer schedules Cmd+K index updates (fail-soft).
type SearchIndexer interface {
	EnqueueUpsert(ctx context.Context, spec, id string)
	EnqueueDelete(ctx context.Context, spec, id string)
}

// SetIndexer plugs in the Cmd+K indexer (nil = no indexing).
func (s *Service) SetIndexer(ix SearchIndexer) {
	s.indexer = ix
}

func (s *Service) index(ctx context.Context, orgID int64, id uuid.UUID) {
	if s.indexer != nil {
		s.indexer.EnqueueUpsert(ctx, SearchSpec, searchengine.TenantDocumentID(orgID, id.String()))
	}
}

func (s *Service) unindex(ctx context.Context, orgID int64, id uuid.UUID) {
	if s.indexer != nil {
		s.indexer.EnqueueDelete(ctx, SearchSpec, searchengine.TenantDocumentID(orgID, id.String()))
	}
}

// SetActive switches is_active and returns the previous value (bulk engine).
func (s *Service) SetActive(ctx context.Context, orgID int64, id uuid.UUID, active bool) (bool, error) {
	q := s.store.Q()
	row, err := s.load(ctx, q, orgID, id)
	if err != nil {
		return false, err
	}
	if row.IsActive != active {
		if err := q.SetServiceBundleActive(ctx, db.SetServiceBundleActiveParams{OrganizationID: orgID, ID: row.ID, IsActive: active}); err != nil {
			return row.IsActive, err
		}
		s.index(ctx, orgID, id)
	}
	return row.IsActive, nil
}

// TargetUUIDs resolves the bundles matching a list filter (bulk "all
// matching" scope), capped at limit.
func (s *Service) TargetUUIDs(ctx context.Context, orgID int64, f model.ListFilter, limit int32) ([]uuid.UUID, error) {
	q := s.store.Q()
	a, err := s.listArgs(ctx, q, orgID, f)
	if err != nil {
		return nil, err
	}
	return q.ListServiceBundleUUIDs(ctx, db.ListServiceBundleUUIDsParams{
		OrganizationID: orgID, FilterBranchIds: a.branches, IsActive: a.isActive, ShowOnline: a.showOnline, Q: a.q, LimitCount: limit,
	})
}

// ExportRows returns every bundle matching a list filter (capped at limit).
func (s *Service) ExportRows(ctx context.Context, orgID int64, f model.ListFilter, limit int32) ([]model.Bundle, error) {
	f.Limit, f.Offset = limit, 0
	out, _, err := s.List(ctx, orgID, f)
	return out, err
}

// ImportRow updates the bundle with the same name, or creates one. The
// contents are named by service names (in serving order) and the branches
// by branch codes; both are resolved inside the organization. It returns
// the previous state of an update.
func (s *Service) ImportRow(ctx context.Context, orgID int64, serviceNames, branchCodes []string, in model.Input) (uuid.UUID, bool, *model.Bundle, error) {
	q := s.store.Q()
	if serviceNames != nil {
		in.ServiceUUIDs = make([]uuid.UUID, 0, len(serviceNames))
		for _, name := range serviceNames {
			svc, err := q.GetServiceByName(ctx, db.GetServiceByNameParams{OrganizationID: orgID, Name: strings.TrimSpace(name)})
			if errors.Is(err, pgx.ErrNoRows) {
				return uuid.Nil, false, nil, &ValidationError{Details: []Detail{{Field: "services", Code: "not_found", Message: "unknown service " + name}}}
			}
			if err != nil {
				return uuid.Nil, false, nil, err
			}
			in.ServiceUUIDs = append(in.ServiceUUIDs, svc.Uuid)
		}
	}
	if branchCodes != nil {
		in.BranchUUIDs = make([]uuid.UUID, 0, len(branchCodes))
		for _, code := range branchCodes {
			branch, err := q.GetBranchByCode(ctx, db.GetBranchByCodeParams{OrganizationID: orgID, Code: strings.ToUpper(strings.TrimSpace(code))})
			if errors.Is(err, pgx.ErrNoRows) {
				return uuid.Nil, false, nil, &ValidationError{Details: []Detail{{Field: "branch_codes", Code: "not_found", Message: "unknown branch code " + code}}}
			}
			if err != nil {
				return uuid.Nil, false, nil, err
			}
			in.BranchUUIDs = append(in.BranchUUIDs, branch.Uuid)
		}
	}
	if in.Name != nil {
		existing, err := q.GetServiceBundleByName(ctx, db.GetServiceBundleByNameParams{OrganizationID: orgID, Name: strings.TrimSpace(*in.Name)})
		if err == nil {
			prev, err := s.Get(ctx, orgID, existing.Uuid)
			if err != nil {
				return uuid.Nil, false, nil, err
			}
			b, err := s.Patch(ctx, orgID, existing.Uuid, in)
			if err != nil {
				return uuid.Nil, false, nil, err
			}
			return b.UUID, false, &prev, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, false, nil, err
		}
	}
	b, err := s.Create(ctx, orgID, in)
	if err != nil {
		return uuid.Nil, false, nil, err
	}
	return b.UUID, true, nil, nil
}

// IsValidation reports whether err is a field validation error.
func IsValidation(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}
