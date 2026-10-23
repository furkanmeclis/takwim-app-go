// Package model holds the API shapes of the availability engine (TEC-897).
package model

import (
	"time"

	"github.com/google/uuid"
)

// Channels: the panel books for customers at the counter or on the phone;
// online is what the booking page, the widget and the public API offer.
const (
	ChannelPanel  = "panel"
	ChannelOnline = "online"
)

// LineInput is one service of the booking, in the order the customer is
// served. StaffUUID picks a person; nil = "fark etmez" (anyone free).
type LineInput struct {
	ServiceUUID uuid.UUID  `json:"service_uuid"`
	StaffUUID   *uuid.UUID `json:"staff_uuid"`
}

// SearchInput is the POST /v1/tenant/availability/search body. From / To are
// local dates (yyyy-MM-dd) of the branch; From defaults to today and To to
// From + 6 days.
type SearchInput struct {
	BranchUUID  uuid.UUID   `json:"branch_uuid"`
	Lines       []LineInput `json:"lines"`
	From        string      `json:"from"`
	To          string      `json:"to"`
	StepMinutes *int        `json:"step_minutes"`
	Channel     string      `json:"channel"`
	// PartySize is the number of people (restaurant): every resource must
	// seat them. Zone narrows the resources to one area.
	PartySize *int   `json:"party_size"`
	Zone      string `json:"zone"`
}

// Ref names a staff member or a resource.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// ResourceRef is a resource with its kind.
type ResourceRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	Kind string    `json:"kind"`
}

// Party limits.
const (
	MinPartySize = 1
	MaxPartySize = 1000
	MaxZone      = 64
)

// LineInfo describes a requested line as the engine planned it.
type LineInfo struct {
	ServiceUUID         uuid.UUID `json:"service_uuid"`
	ServiceName         string    `json:"service_name"`
	DurationMinutes     int       `json:"duration_minutes"`
	BufferBeforeMinutes int       `json:"buffer_before_minutes"`
	BufferAfterMinutes  int       `json:"buffer_after_minutes"`
	StaffMode           string    `json:"staff_mode"`
	ResourceKinds       []string  `json:"resource_kinds"`
	// Staff are the people who may perform the line in the branch (the
	// picked person only when one was given).
	Staff []Ref `json:"staff"`
}

// SlotLine is who and what serves one line of a slot.
type SlotLine struct {
	ServiceUUID uuid.UUID     `json:"service_uuid"`
	StartsAt    time.Time     `json:"starts_at"`
	EndsAt      time.Time     `json:"ends_at"`
	Staff       *Ref          `json:"staff"`
	Resources   []ResourceRef `json:"resources"`
	// The plan behind the line, for the hold writer (not serialized).
	StaffID     int64     `json:"-"`
	ResourceIDs []int64   `json:"-"`
	HeldFrom    time.Time `json:"-"`
	HeldUntil   time.Time `json:"-"`
}

// Slot is one bookable start.
type Slot struct {
	StartsAt time.Time  `json:"starts_at"`
	EndsAt   time.Time  `json:"ends_at"`
	Lines    []SlotLine `json:"lines"`
	// BranchID is the planned branch, for the hold writer (not serialized).
	BranchID int64 `json:"-"`
}

// Day is the answer for one local date.
type Day struct {
	Date   string `json:"date"`
	Status string `json:"status"`
	Slots  []Slot `json:"slots"`
}

// Result is the search answer.
type Result struct {
	Branch   Ref    `json:"branch"`
	Timezone string `json:"timezone"`
	Channel  string `json:"channel"`
	// EarliestStart is the first instant a booking may start; LatestDate the
	// last local date (null = no limit).
	EarliestStart time.Time  `json:"earliest_start"`
	LatestDate    *string    `json:"latest_date"`
	StepMinutes   int        `json:"step_minutes"`
	Lines         []LineInfo `json:"lines"`
	Days          []Day      `json:"days"`
}

// Hold kinds, reasons and statuses (capacity_holds).
const (
	KindHold    = "hold"
	KindBooking = "booking"

	ReasonApproval = "approval"
	ReasonPayment  = "payment"
	ReasonCheckout = "checkout"

	StatusActive   = "active"
	StatusReleased = "released"
	StatusExpired  = "expired"
)

// HoldInput is the POST /v1/tenant/availability/holds body: the search of
// one visit, its start and why it is held. Minutes overrides the hold time
// of the reason (business settings, else the defaults).
type HoldInput struct {
	BranchUUID uuid.UUID   `json:"branch_uuid"`
	Lines      []LineInput `json:"lines"`
	Channel    string      `json:"channel"`
	StartsAt   *time.Time  `json:"starts_at"`
	Reason     string      `json:"reason"`
	Minutes    *int        `json:"minutes"`
	PartySize  *int        `json:"party_size"`
	Zone       string      `json:"zone"`
}

// ExtendInput is the POST /v1/tenant/availability/holds/{uuid}/extend body.
type ExtendInput struct {
	Minutes int `json:"minutes"`
}

// Allocation is what one line of a commitment holds.
type Allocation struct {
	LineIndex int          `json:"line_index"`
	StartsAt  time.Time    `json:"starts_at"`
	EndsAt    time.Time    `json:"ends_at"`
	Staff     *Ref         `json:"staff"`
	Resource  *ResourceRef `json:"resource"`
}

// Hold is a capacity commitment: a timed hold or a confirmed booking.
type Hold struct {
	UUID    uuid.UUID `json:"uuid"`
	Kind    string    `json:"kind"`
	Reason  *string   `json:"reason"`
	Status  string    `json:"status"`
	Channel string    `json:"channel"`
	Branch  Ref       `json:"branch"`
	// StartsAt / EndsAt are the customer's time of the whole visit.
	StartsAt      time.Time    `json:"starts_at"`
	EndsAt        time.Time    `json:"ends_at"`
	ExpiresAt     *time.Time   `json:"expires_at"`
	RefType       *string      `json:"ref_type"`
	RefUUID       *uuid.UUID   `json:"ref_uuid"`
	ReleasedAt    *time.Time   `json:"released_at"`
	ReleaseReason *string      `json:"release_reason"`
	Allocations   []Allocation `json:"allocations"`
	CreatedAt     time.Time    `json:"created_at"`
}
