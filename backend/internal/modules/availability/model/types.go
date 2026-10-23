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
}

// Slot is one bookable start.
type Slot struct {
	StartsAt time.Time  `json:"starts_at"`
	EndsAt   time.Time  `json:"ends_at"`
	Lines    []SlotLine `json:"lines"`
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
