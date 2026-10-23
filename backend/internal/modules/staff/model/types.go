// Package model holds the API shapes of the staff tenant resource.
package model

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Resource is the stable slug of the staff resource (activity, io, bulk).
const Resource = "tenant.staff"

// Time off kinds.
const (
	TimeOffLeave    = "leave"
	TimeOffSick     = "sick"
	TimeOffTraining = "training"
	TimeOffOther    = "other"
)

// TimeOffKinds lists the allowed time off kinds.
var TimeOffKinds = []string{TimeOffLeave, TimeOffSick, TimeOffTraining, TimeOffOther}

// Break is a pause inside a shift. Start/End are "HH:MM" (24h).
type Break struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// Shift is one working period of a weekday in a branch, with its breaks.
type Shift struct {
	Start  string  `json:"start"`
	End    string  `json:"end"`
	Breaks []Break `json:"breaks"`
}

// WeeklyShifts are the shifts of one person in one branch (Monday first).
// A day without shifts is a day off in that branch.
type WeeklyShifts struct {
	Mon []Shift `json:"mon"`
	Tue []Shift `json:"tue"`
	Wed []Shift `json:"wed"`
	Thu []Shift `json:"thu"`
	Fri []Shift `json:"fri"`
	Sat []Shift `json:"sat"`
	Sun []Shift `json:"sun"`
}

// Day is one weekday of a WeeklyShifts.
type Day struct {
	Key    string
	Shifts *[]Shift
}

// Days returns the weekday keys with pointers to their shifts (Monday first).
func (w *WeeklyShifts) Days() []Day {
	return []Day{
		{"mon", &w.Mon}, {"tue", &w.Tue}, {"wed", &w.Wed}, {"thu", &w.Thu},
		{"fri", &w.Fri}, {"sat", &w.Sat}, {"sun", &w.Sun},
	}
}

// Normalize replaces nil days and break lists with empty ones, so the JSON
// always carries arrays.
func (w *WeeklyShifts) Normalize() {
	for _, d := range w.Days() {
		if *d.Shifts == nil {
			*d.Shifts = []Shift{}
		}
		for i := range *d.Shifts {
			if (*d.Shifts)[i].Breaks == nil {
				(*d.Shifts)[i].Breaks = []Break{}
			}
		}
	}
}

// Account is the member account a staff record is linked to.
type Account struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// BranchRef names a branch the person works in.
type BranchRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// Assignment is a branch of the person with the shifts there. BranchHours
// is the branch's weekly schedule (reference for the shift editor).
// Editable is false for branches outside the caller's branch scope.
type Assignment struct {
	BranchUUID  uuid.UUID       `json:"branch_uuid"`
	BranchName  string          `json:"branch_name"`
	BranchCode  string          `json:"branch_code"`
	BranchHours json.RawMessage `json:"branch_hours"`
	Shifts      WeeklyShifts    `json:"shifts"`
	Editable    bool            `json:"editable"`
}

// Staff is the API projection of a staff member. Account is null for staff
// without an account (or whose account left the business). Assignments is
// filled on the detail response only.
type Staff struct {
	UUID        uuid.UUID    `json:"uuid"`
	Name        string       `json:"name"`
	Title       string       `json:"title"`
	Bio         string       `json:"bio"`
	Phone       string       `json:"phone"`
	Email       string       `json:"email"`
	Color       string       `json:"color"`
	ShowOnline  bool         `json:"show_online"`
	IsActive    bool         `json:"is_active"`
	Account     *Account     `json:"account"`
	Branches    []BranchRef  `json:"branches"`
	Assignments []Assignment `json:"assignments,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// OptionalUUID distinguishes an absent JSON field from an explicit null.
type OptionalUUID struct {
	Set   bool
	Value *uuid.UUID
}

func (o *OptionalUUID) UnmarshalJSON(b []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
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

// StaffInput is the create / patch body. Absent fields keep their value on
// patch. BranchUUIDs replaces the assignments the caller may manage
// (branches outside a limited caller's scope are kept).
type StaffInput struct {
	Name        *string      `json:"name"`
	Title       *string      `json:"title"`
	Bio         *string      `json:"bio"`
	Phone       *string      `json:"phone"`
	Email       *string      `json:"email"`
	Color       *string      `json:"color"`
	ShowOnline  *bool        `json:"show_online"`
	IsActive    *bool        `json:"is_active"`
	AccountUUID OptionalUUID `json:"account_uuid"`
	BranchUUIDs *[]uuid.UUID `json:"branch_uuids"`
}

// ListFilter selects staff of one organization.
type ListFilter struct {
	Limit      int32
	Offset     int32
	Q          string
	IsActive   *bool
	ShowOnline *bool
	HasAccount *bool
	// Branches matches staff assigned to any of these branches.
	Branches  []uuid.UUID
	SortField string
	SortDesc  bool
	// BranchIDs is the caller's branch scope (orgctx.BranchScope), set by
	// the usecase, never from the request.
	BranchIDs []int64
}

// BranchFacet is one branch of the branch filter with its staff count.
type BranchFacet struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Count int64  `json:"count"`
}

// Facets are the dynamic filter options of the staff list.
type Facets struct {
	Branch []BranchFacet `json:"branch"`
}

// LinkableMember is a member of the business without a staff record.
type LinkableMember struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// TimeOff is a period the person is unavailable in every branch.
type TimeOff struct {
	UUID      uuid.UUID `json:"uuid"`
	Kind      string    `json:"kind"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

// TimeOffInput is the create / patch body of a time off.
type TimeOffInput struct {
	Kind     *string    `json:"kind"`
	StartsAt *time.Time `json:"starts_at"`
	EndsAt   *time.Time `json:"ends_at"`
	Note     *string    `json:"note"`
}

// TimeOffFilter pages the time off of one person; From/To keep the entries
// overlapping that range.
type TimeOffFilter struct {
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

// ActivityEntry is one audit event of a staff member.
type ActivityEntry struct {
	UUID      uuid.UUID       `json:"uuid"`
	Action    string          `json:"action"`
	Payload   json.RawMessage `json:"payload"`
	Actor     *ActivityActor  `json:"actor"`
	CreatedAt time.Time       `json:"created_at"`
}
