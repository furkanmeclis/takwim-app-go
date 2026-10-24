// Package model holds the API shapes of the combined service bundles
// (birleşik hizmet paketi, TEC-895) and the price split shared with the
// reservation lines.
package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Slug is the stable slug of the bundles resource (activity, live list).
const Slug = "tenant.service_bundles"

// Limits of a bundle.
const (
	MinItems          = 2
	MaxItems          = 10
	MaxNameLength     = 160
	MaxDescriptionLen = 2000
	MaxSortOrder      = 100000
)

// Item is one service of the bundle, in the order the customer is served,
// with its catalog values. AllocatedPrice is its share of the bundle price
// at catalog prices (a booking splits the price again over the prices of
// the branch and the staff member it books).
type Item struct {
	Position            int         `json:"position"`
	ServiceUUID         uuid.UUID   `json:"service_uuid"`
	ServiceName         string      `json:"service_name"`
	DurationMinutes     int32       `json:"duration_minutes"`
	BufferBeforeMinutes int32       `json:"buffer_before_minutes"`
	BufferAfterMinutes  int32       `json:"buffer_after_minutes"`
	ListPrice           money.Money `json:"list_price"`
	AllocatedPrice      money.Money `json:"allocated_price"`
	StaffMode           string      `json:"staff_mode"`
	ResourceKinds       []string    `json:"resource_kinds"`
	// Bookable is false when the service was deactivated or deleted.
	Bookable bool `json:"bookable"`
}

// BranchRef is a branch offering the bundle. Editable is false for
// branches outside the caller's branch scope.
type BranchRef struct {
	UUID     uuid.UUID `json:"uuid"`
	Name     string    `json:"name"`
	Code     string    `json:"code"`
	Editable bool      `json:"editable"`
}

// Bundle is the API projection of a combined service bundle. The duration,
// list price, staff need and resource kinds follow from the contents.
type Bundle struct {
	UUID        uuid.UUID   `json:"uuid"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Price       money.Money `json:"price"`
	// ListPrice is the sum of the catalog prices of the contents; Discount
	// is what the bundle saves (zero when the bundle costs more).
	ListPrice       money.Money `json:"list_price"`
	Discount        money.Money `json:"discount"`
	Currency        string      `json:"currency"`
	DurationMinutes int32       `json:"duration_minutes"`
	// StaffRequired: at least one service holds a staff member's time.
	StaffRequired bool        `json:"staff_required"`
	ResourceKinds []string    `json:"resource_kinds"`
	Items         []Item      `json:"items"`
	Branches      []BranchRef `json:"branches"`
	ShowOnline    bool        `json:"show_online"`
	IsActive      bool        `json:"is_active"`
	// Bookable: active and every service of it can be booked.
	Bookable  bool      `json:"bookable"`
	SortOrder int32     `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Input is the create / patch body. ServiceUUIDs (the contents in order)
// and BranchUUIDs replace the stored lists when given; omitted, they stay.
type Input struct {
	Name         *string      `json:"name"`
	Description  *string      `json:"description"`
	Price        *json.Number `json:"price"`
	ShowOnline   *bool        `json:"show_online"`
	IsActive     *bool        `json:"is_active"`
	SortOrder    *int32       `json:"sort_order"`
	ServiceUUIDs []uuid.UUID  `json:"service_uuids"`
	BranchUUIDs  []uuid.UUID  `json:"branch_uuids"`
}

// ListFilter narrows the bundle list.
type ListFilter struct {
	Branches   []uuid.UUID
	IsActive   *bool
	ShowOnline *bool
	Q          string
	SortField  string
	SortDesc   bool
	Limit      int32
	Offset     int32
}

// Facet is one option of a faceted filter.
type Facet struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Count int64  `json:"count"`
}

// Facets are the dynamic filter options of the list.
type Facets struct {
	Branch []Facet `json:"branch"`
}

// ActivityActor is the user behind an activity entry.
type ActivityActor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// ActivityEntry is one audit event of a bundle.
type ActivityEntry struct {
	UUID      uuid.UUID       `json:"uuid"`
	Action    string          `json:"action"`
	Payload   json.RawMessage `json:"payload"`
	Actor     *ActivityActor  `json:"actor"`
	CreatedAt time.Time       `json:"created_at"`
}

// Distribute splits the bundle price over the lines in proportion to their
// list prices. Every share but the last is rounded half up to the kuruş and
// the last line takes the rest, so the shares always add up to the bundle
// price (requirements §11: 800 + 400 sold at 1000 → 666.67 + 333.33).
// Lines that all cost nothing share the price equally.
func Distribute(price money.Money, lists []money.Money) []money.Money {
	if len(lists) == 0 {
		return nil
	}
	weights := make([]int64, len(lists))
	var sum int64
	for i, l := range lists {
		if l.Minor() > 0 {
			weights[i] = l.Minor()
			sum += weights[i]
		}
	}
	if sum == 0 {
		for i := range weights {
			weights[i] = 1
		}
		sum = int64(len(weights))
	}
	out := make([]money.Money, len(lists))
	used := money.NewIn(0, price.Currency())
	for i, w := range weights[:len(weights)-1] {
		out[i] = price.Ratio(w, sum)
		used = used.Add(out[i])
	}
	out[len(out)-1] = price.Sub(used)
	if out[len(out)-1].IsNegative() {
		// Rounding up left nothing for a near-free last line: round down
		// instead (the remainder still goes to the last line).
		if parts, err := price.Allocate(weights...); err == nil {
			return parts
		}
	}
	return out
}
