package usecase

import (
	"context"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

type resourceEvents = resourceevents.Publisher

// maxEventUUIDs caps the affected uuids collected for one event.
const maxEventUUIDs = resourceevents.MaxUUIDs

// SetResourceEvents publishes one "bulk" resource event per finished bulk job (and rollback)
// (optional; nil keeps the service silent).
func (s *Service) SetResourceEvents(pub resourceevents.Publisher) {
	s.events = pub
}

func (s *Service) publishBulk(ctx context.Context, resource string, uuids []string) {
	if s.events == nil || len(uuids) == 0 {
		return
	}
	s.events.Publish(ctx, resource, resourceevents.ActionBulk, uuids...)
}
