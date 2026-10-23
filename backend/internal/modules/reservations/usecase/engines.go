package usecase

import (
	"context"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
)

// SearchSpec is the Cmd+K spec id of reservations (TEC-963).
const SearchSpec = "reservations"

// SearchIndexer schedules Cmd+K index updates (fail-soft).
type SearchIndexer interface {
	EnqueueUpsert(ctx context.Context, spec, id string)
	EnqueueDelete(ctx context.Context, spec, id string)
}

// SetSearchIndexer keeps the Cmd+K documents in step with bookings and
// moves (number, customer, time, branch and the staff serving it). A state
// change does not touch the document.
func (s *Service) SetSearchIndexer(ix SearchIndexer) {
	s.indexer = ix
}

func (s *Service) index(ctx context.Context, orgID int64, id uuid.UUID) {
	if s.indexer != nil {
		s.indexer.EnqueueUpsert(ctx, SearchSpec, searchengine.TenantDocumentID(orgID, id.String()))
	}
}
