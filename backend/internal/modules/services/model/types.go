// Package model holds the API shapes of the services tenant resource.
package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Slug is the stable slug of the services resource (activity, io, bulk).
const Slug = "tenant.services"

// ResourceKinds lists the resource kinds a service may need (sector.ResourceKind values).
var ResourceKinds = []string{"chair", "table", "area", "room", "device", "bay"}

// PaymentModes are how a customer pays for the service.
var PaymentModes = []string{"on_site", "online", "both"}

// StaffModes say whether a booking holds a staff member's time.
var StaffModes = []string{"required", "optional", "none"}

// Category groups services on the panel and the booking page.
type Category struct {
	UUID          uuid.UUID `json:"uuid"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	SortOrder     int32     `json:"sort_order"`
	ServicesCount int64     `json:"services_count"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// CategoryInput is the create / patch body of a category.
type CategoryInput struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	SortOrder   *int32  `json:"sort_order"`
}

// CategoryRef names the category of a service.
type CategoryRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// Offer is a branch offering the service. Price / DurationMinutes are the
// branch overrides (null = catalog value); Effective* are what the branch
// charges and books. Editable is false for branches outside the caller's
// scope (detail only).
type Offer struct {
	BranchUUID               uuid.UUID    `json:"branch_uuid"`
	BranchName               string       `json:"branch_name"`
	BranchCode               string       `json:"branch_code"`
	Price                    *money.Money `json:"price"`
	DurationMinutes          *int32       `json:"duration_minutes"`
	EffectivePrice           money.Money  `json:"effective_price"`
	EffectiveDurationMinutes int32        `json:"effective_duration_minutes"`
	IsActive                 bool         `json:"is_active"`
	Editable                 bool         `json:"editable"`
}

// StaffLink is a staff member who may perform the service, with optional
// personal price and duration.
type StaffLink struct {
	StaffUUID       uuid.UUID    `json:"staff_uuid"`
	Name            string       `json:"name"`
	Title           string       `json:"title"`
	Color           string       `json:"color"`
	IsActive        bool         `json:"is_active"`
	Price           *money.Money `json:"price"`
	DurationMinutes *int32       `json:"duration_minutes"`
}

// Service is the API projection of a catalog service.
type Service struct {
	UUID                uuid.UUID    `json:"uuid"`
	Category            *CategoryRef `json:"category"`
	Name                string       `json:"name"`
	Description         string       `json:"description"`
	DurationMinutes     int32        `json:"duration_minutes"`
	BufferBeforeMinutes int32        `json:"buffer_before_minutes"`
	BufferAfterMinutes  int32        `json:"buffer_after_minutes"`
	Price               money.Money  `json:"price"`
	Currency            string       `json:"currency"`
	PaymentMode         string       `json:"payment_mode"`
	StaffMode           string       `json:"staff_mode"`
	ResourceKinds       []string     `json:"resource_kinds"`
	// GroupCapacity makes the service a group class (gym, TEC-904): each
	// start is one session with this many seats. Null = individual.
	GroupCapacity *int32      `json:"group_capacity"`
	ShowOnline    bool        `json:"show_online"`
	IsActive      bool        `json:"is_active"`
	SortOrder     int32       `json:"sort_order"`
	ImageURL      *string     `json:"image_url"`
	Offers        []Offer     `json:"offers"`
	Staff         []StaffLink `json:"staff,omitempty"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

// OptionalUUID distinguishes an absent uuid field from null.
type OptionalUUID struct {
	Set   bool
	Value *uuid.UUID
}

func (o *OptionalUUID) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	var v uuid.UUID
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// OptionalInt distinguishes an absent number field from null.
type OptionalInt struct {
	Set   bool
	Value *int32
}

func (o *OptionalInt) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	var v int32
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// Input is the create / patch body. Absent fields keep their value on patch
// and take their defaults on create. BranchUUIDs (create only) adds offers
// at catalog price in those branches.
type Input struct {
	CategoryUUID        OptionalUUID `json:"category_uuid"`
	Name                *string      `json:"name"`
	Description         *string      `json:"description"`
	DurationMinutes     *int32       `json:"duration_minutes"`
	BufferBeforeMinutes *int32       `json:"buffer_before_minutes"`
	BufferAfterMinutes  *int32       `json:"buffer_after_minutes"`
	Price               *json.Number `json:"price"`
	PaymentMode         *string      `json:"payment_mode"`
	StaffMode           *string      `json:"staff_mode"`
	ResourceKinds       *[]string    `json:"resource_kinds"`
	GroupCapacity       OptionalInt  `json:"group_capacity"`
	ShowOnline          *bool        `json:"show_online"`
	IsActive            *bool        `json:"is_active"`
	SortOrder           *int32       `json:"sort_order"`
	BranchUUIDs         []uuid.UUID  `json:"branch_uuids"`
}

// OfferInput is one branch offer of PUT …/offers.
type OfferInput struct {
	BranchUUID      uuid.UUID    `json:"branch_uuid"`
	Price           *json.Number `json:"price"`
	DurationMinutes *int32       `json:"duration_minutes"`
	IsActive        *bool        `json:"is_active"`
}

// OffersInput replaces the offers of the branches the caller manages.
type OffersInput struct {
	Offers []OfferInput `json:"offers"`
}

// StaffInput is one staff link of PUT …/staff.
type StaffInput struct {
	StaffUUID       uuid.UUID    `json:"staff_uuid"`
	Price           *json.Number `json:"price"`
	DurationMinutes *int32       `json:"duration_minutes"`
}

// StaffLinksInput replaces the staff who may perform the service.
type StaffLinksInput struct {
	Staff []StaffInput `json:"staff"`
}

// ListFilter selects services of one organization.
type ListFilter struct {
	Limit         int32
	Offset        int32
	Q             string
	Categories    []uuid.UUID
	Uncategorized bool
	Branches      []uuid.UUID
	PaymentModes  []string
	IsActive      *bool
	ShowOnline    *bool
	SortField     string
	SortDesc      bool
}

// Facet is one option of a faceted filter with its row count.
type Facet struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
	Count int64  `json:"count"`
}

// Facets are the dynamic filter options of the list. Category "none" is
// the uncategorized services.
type Facets struct {
	Category    []Facet `json:"category"`
	Branch      []Facet `json:"branch"`
	PaymentMode []Facet `json:"payment_mode"`
}

// UncategorizedFacet is the category facet value of services without a category.
const UncategorizedFacet = "none"

// ActivityActor is the user behind an activity entry.
type ActivityActor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// ActivityEntry is one audit event of a service.
type ActivityEntry struct {
	UUID      uuid.UUID       `json:"uuid"`
	Action    string          `json:"action"`
	Payload   json.RawMessage `json:"payload"`
	Actor     *ActivityActor  `json:"actor"`
	CreatedAt time.Time       `json:"created_at"`
}
