// Package model holds the API shapes of the branches tenant resource.
package model

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Resource is the stable slug of the branches resource (activity, io, bulk).
const Resource = "tenant.branches"

// Service point types.
const (
	ServicePointOffice      = "office"
	ServicePointAirport     = "airport"
	ServicePointAddressZone = "address_zone"
	ServicePointHotel       = "hotel"
)

// ServicePointTypes lists the allowed service point types.
var ServicePointTypes = []string{ServicePointOffice, ServicePointAirport, ServicePointAddressZone, ServicePointHotel}

// DayHours is the schedule of one weekday. Open/Close are "HH:MM" (24h).
type DayHours struct {
	Closed bool   `json:"closed"`
	Open   string `json:"open"`
	Close  string `json:"close"`
}

// WeeklyHours is a 7-day schedule (Monday first).
type WeeklyHours struct {
	Mon DayHours `json:"mon"`
	Tue DayHours `json:"tue"`
	Wed DayHours `json:"wed"`
	Thu DayHours `json:"thu"`
	Fri DayHours `json:"fri"`
	Sat DayHours `json:"sat"`
	Sun DayHours `json:"sun"`
}

// Days returns the weekday keys with pointers to their schedule (Monday first).
func (w *WeeklyHours) Days() []struct {
	Key   string
	Hours *DayHours
} {
	return []struct {
		Key   string
		Hours *DayHours
	}{
		{"mon", &w.Mon}, {"tue", &w.Tue}, {"wed", &w.Wed}, {"thu", &w.Thu},
		{"fri", &w.Fri}, {"sat", &w.Sat}, {"sun", &w.Sun},
	}
}

// DefaultWeeklyHours is used when a branch is created without a schedule:
// weekdays 09:00–18:00, Saturday 09:00–14:00, Sunday closed.
func DefaultWeeklyHours() WeeklyHours {
	day := DayHours{Open: "09:00", Close: "18:00"}
	return WeeklyHours{
		Mon: day, Tue: day, Wed: day, Thu: day, Fri: day,
		Sat: DayHours{Open: "09:00", Close: "14:00"},
		Sun: DayHours{Closed: true},
	}
}

// HolidayException overrides the weekly schedule on one date (yyyy-MM-dd).
type HolidayException struct {
	Date   string `json:"date"`
	Closed bool   `json:"closed"`
	Open   string `json:"open"`
	Close  string `json:"close"`
	Note   string `json:"note"`
}

// Branch is the API projection of a branch.
type Branch struct {
	UUID               uuid.UUID          `json:"uuid"`
	Code               string             `json:"code"`
	Name               string             `json:"name"`
	Address            string             `json:"address"`
	City               string             `json:"city"`
	District           string             `json:"district"`
	Latitude           *float64           `json:"latitude"`
	Longitude          *float64           `json:"longitude"`
	Phone              string             `json:"phone"`
	Email              string             `json:"email"`
	WorkingHours       WeeklyHours        `json:"working_hours"`
	HolidayExceptions  []HolidayException `json:"holiday_exceptions"`
	PrepMinutes        int32              `json:"prep_minutes"`
	IsActive           bool               `json:"is_active"`
	ServicePointsCount int64              `json:"service_points_count"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
}

// ServicePoint is the API projection of a service point.
type ServicePoint struct {
	UUID       uuid.UUID `json:"uuid"`
	BranchUUID uuid.UUID `json:"branch_uuid"`
	Type       string    `json:"type"`
	Name       string    `json:"name"`
	// Description is the street address, or the zone description of an
	// address_zone point.
	Description string `json:"description"`
	// ExtraFee is a decimal string with two fraction digits ("150.00").
	ExtraFee string `json:"extra_fee"`
	// WorkingHours is null when the point follows the branch hours.
	WorkingHours     *WeeklyHours `json:"working_hours"`
	OnlineSelectable bool         `json:"online_selectable"`
	IsActive         bool         `json:"is_active"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
}

// OptionalFloat distinguishes an absent JSON field from an explicit null.
type OptionalFloat struct {
	Set   bool
	Value *float64
}

func (o *OptionalFloat) UnmarshalJSON(b []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		o.Value = nil
		return nil
	}
	var v float64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// OptionalHours distinguishes an absent working_hours field from null.
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

// BranchInput is the create / patch body. Nil fields are left unchanged on
// patch and take their defaults on create.
type BranchInput struct {
	Code              *string             `json:"code"`
	Name              *string             `json:"name"`
	Address           *string             `json:"address"`
	City              *string             `json:"city"`
	District          *string             `json:"district"`
	Latitude          OptionalFloat       `json:"latitude"`
	Longitude         OptionalFloat       `json:"longitude"`
	Phone             *string             `json:"phone"`
	Email             *string             `json:"email"`
	WorkingHours      *WeeklyHours        `json:"working_hours"`
	HolidayExceptions *[]HolidayException `json:"holiday_exceptions"`
	PrepMinutes       *int32              `json:"prep_minutes"`
	IsActive          *bool               `json:"is_active"`
}

// ServicePointInput is the create / patch body of a service point.
type ServicePointInput struct {
	Type             *string       `json:"type"`
	Name             *string       `json:"name"`
	Description      *string       `json:"description"`
	ExtraFee         *json.Number  `json:"extra_fee"`
	WorkingHours     OptionalHours `json:"working_hours"`
	OnlineSelectable *bool         `json:"online_selectable"`
	IsActive         *bool         `json:"is_active"`
}

// ListFilter selects branches of one organization.
type ListFilter struct {
	Limit  int32
	Offset int32
	Q      string
	// Cities matches any of the given cities (exact match).
	Cities    []string
	IsActive  *bool
	SortField string
	SortDesc  bool
	// BranchIDs is the caller's branch scope (orgctx.BranchScope): nil means
	// every branch, otherwise only these ids. Set by the usecase, never
	// from the request.
	BranchIDs []int64
}

// Facet is one value of a faceted filter with its row count.
type Facet struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// Facets are the dynamic filter options of the branches list.
type Facets struct {
	City []Facet `json:"city"`
}

// ActivityActor is the user behind an activity entry.
type ActivityActor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// ActivityEntry is one audit event of a branch.
type ActivityEntry struct {
	UUID      uuid.UUID       `json:"uuid"`
	Action    string          `json:"action"`
	Payload   json.RawMessage `json:"payload"`
	Actor     *ActivityActor  `json:"actor"`
	CreatedAt time.Time       `json:"created_at"`
}
