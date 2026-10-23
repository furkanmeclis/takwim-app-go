// Package model holds the API shapes of the resources tenant resource.
package model

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	branchmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/model"
)

// Slug is the stable slug of the resources resource (activity, io, bulk).
const Slug = "tenant.resources"

// Kinds lists the resource kinds (sector.ResourceKind values).
var Kinds = []string{"chair", "table", "area", "room", "device", "bay"}

// WeeklyHours is the branch schedule shape (closed / open / close per day).
type WeeklyHours = branchmodel.WeeklyHours

// BranchRef names the branch of a resource.
type BranchRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	Code string    `json:"code"`
}

// Resource is the API projection of a resource. WorkingHours is null when
// the resource follows its branch's hours; BranchHours (detail only) is the
// branch schedule for reference.
type Resource struct {
	UUID         uuid.UUID       `json:"uuid"`
	Branch       BranchRef       `json:"branch"`
	Kind         string          `json:"kind"`
	Name         string          `json:"name"`
	Zone         string          `json:"zone"`
	Description  string          `json:"description"`
	MinCapacity  int32           `json:"min_capacity"`
	Capacity     int32           `json:"capacity"`
	Combinable   bool            `json:"combinable"`
	ShowOnline   bool            `json:"show_online"`
	IsActive     bool            `json:"is_active"`
	SortOrder    int32           `json:"sort_order"`
	WorkingHours *WeeklyHours    `json:"working_hours"`
	BranchHours  json.RawMessage `json:"branch_hours,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// OptionalHours distinguishes an absent working_hours field from null
// (null = follow the branch).
type OptionalHours struct {
	Set   bool
	Value *WeeklyHours
}

func (o *OptionalHours) UnmarshalJSON(b []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		o.Value = nil
		return nil
	}
	var v WeeklyHours
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// Input is the create / patch body. Absent fields keep their value on patch
// and take their defaults on create.
type Input struct {
	BranchUUID   *uuid.UUID    `json:"branch_uuid"`
	Kind         *string       `json:"kind"`
	Name         *string       `json:"name"`
	Zone         *string       `json:"zone"`
	Description  *string       `json:"description"`
	MinCapacity  *int32        `json:"min_capacity"`
	Capacity     *int32        `json:"capacity"`
	Combinable   *bool         `json:"combinable"`
	ShowOnline   *bool         `json:"show_online"`
	IsActive     *bool         `json:"is_active"`
	SortOrder    *int32        `json:"sort_order"`
	WorkingHours OptionalHours `json:"working_hours"`
}

// ListFilter selects resources of one organization.
type ListFilter struct {
	Limit      int32
	Offset     int32
	Q          string
	Branches   []uuid.UUID
	Kinds      []string
	Zones      []string
	IsActive   *bool
	ShowOnline *bool
	SortField  string
	SortDesc   bool
}

// Facet is one option of a faceted filter with its row count.
type Facet struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
	Count int64  `json:"count"`
}

// Facets are the dynamic filter options of the list.
type Facets struct {
	Branch []Facet `json:"branch"`
	Kind   []Facet `json:"kind"`
	Zone   []Facet `json:"zone"`
}

// Closure is a period the resource cannot be booked.
type Closure struct {
	UUID      uuid.UUID `json:"uuid"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

// ClosureInput is the create / patch body of a closure.
type ClosureInput struct {
	StartsAt *time.Time `json:"starts_at"`
	EndsAt   *time.Time `json:"ends_at"`
	Reason   *string    `json:"reason"`
}

// ClosureFilter pages the closures of one resource; From/To keep the
// entries overlapping that range.
type ClosureFilter struct {
	Limit, Offset int32
	From, To      *time.Time
}

// ActivityActor is the user behind an activity entry.
type ActivityActor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// ActivityEntry is one audit event of a resource.
type ActivityEntry struct {
	UUID      uuid.UUID       `json:"uuid"`
	Action    string          `json:"action"`
	Payload   json.RawMessage `json:"payload"`
	Actor     *ActivityActor  `json:"actor"`
	CreatedAt time.Time       `json:"created_at"`
}
