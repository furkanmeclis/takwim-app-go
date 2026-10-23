package events

import (
	"time"

	"github.com/google/uuid"
)

// Event is the standard envelope published on the platform bus.
type Event struct {
	Name           string
	OrganizationID *int64
	ActorUserID    *int64
	EntityType     string
	EntityID       *int64
	EntityUUID     *uuid.UUID
	Payload        map[string]any
	OccurredAt     time.Time
	EventID        uuid.UUID
}

// New builds an Event with a fresh EventID and UTC OccurredAt.
func New(name string) Event {
	return Event{
		Name:       name,
		Payload:    map[string]any{},
		OccurredAt: time.Now().UTC(),
		EventID:    uuid.New(),
	}
}

// WithOrganization stamps the event with the organization it belongs to
// (tenant events); handlers pass it on, e.g. to organization-stamped
// notifications. Platform events leave it nil.
func (e Event) WithOrganization(organizationID int64) Event {
	e.OrganizationID = &organizationID
	return e
}

// WithActor sets ActorUserID.
func (e Event) WithActor(userID int64) Event {
	e.ActorUserID = &userID
	return e
}

// WithEntity sets EntityType and optional numeric / UUID identifiers.
func (e Event) WithEntity(entityType string, entityID *int64, entityUUID *uuid.UUID) Event {
	e.EntityType = entityType
	e.EntityID = entityID
	e.EntityUUID = entityUUID
	return e
}

// WithPayload replaces the payload map.
func (e Event) WithPayload(payload map[string]any) Event {
	if payload == nil {
		payload = map[string]any{}
	}
	e.Payload = payload
	return e
}
