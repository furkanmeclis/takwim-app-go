// Package model holds the shapes of the class timetable (TEC-970).
package model

import (
	"time"

	"github.com/google/uuid"
)

// Slug is the engine / live / activity resource of the timetable.
const Slug = "tenant.classes"

// Limits of the timetable.
const (
	MinCapacity = 1
	MaxCapacity = 1000
	MaxNote     = 500
	MaxReason   = 500
	// MaxSessionDays is the longest period of the session list.
	MaxSessionDays = 31
	// DefaultSessionDays is the period of the session list without dates.
	DefaultSessionDays = 7
)

// Session states.
const (
	StateOpen      = "open"
	StateFull      = "full"
	StateCancelled = "cancelled"
	StatePast      = "past"
)

// States lists the session states (filter values).
var States = []string{StateOpen, StateFull, StateCancelled, StatePast}

// Ref names a record.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// ResourceRef names a resource (the area of a class).
type ResourceRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	Kind string    `json:"kind"`
}

// Schedule is one weekly class of the timetable.
type Schedule struct {
	UUID    uuid.UUID `json:"uuid"`
	Branch  Ref       `json:"branch"`
	Service Ref       `json:"service"`
	// Weekday is mon … sun; StartTime / EndTime the branch wall clock.
	Weekday         string       `json:"weekday"`
	StartTime       string       `json:"start_time"`
	EndTime         string       `json:"end_time"`
	DurationMinutes int          `json:"duration_minutes"`
	Staff           *Ref         `json:"staff"`
	Resource        *ResourceRef `json:"resource"`
	// Capacity is the class's own seats (null = the service's group
	// capacity); Seats the seats a session has.
	Capacity   *int      `json:"capacity"`
	Seats      int       `json:"seats"`
	ValidFrom  string    `json:"valid_from"`
	ValidUntil *string   `json:"valid_until"`
	IsActive   bool      `json:"is_active"`
	Note       string    `json:"note"`
	Timezone   string    `json:"timezone"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ScheduleInput creates or replaces a class (PATCH takes the whole class).
type ScheduleInput struct {
	BranchUUID   uuid.UUID  `json:"branch_uuid"`
	ServiceUUID  uuid.UUID  `json:"service_uuid"`
	Weekday      string     `json:"weekday"`
	StartTime    string     `json:"start_time"`
	StaffUUID    *uuid.UUID `json:"staff_uuid"`
	ResourceUUID *uuid.UUID `json:"resource_uuid"`
	Capacity     *int       `json:"capacity"`
	// ValidFrom defaults to today; ValidUntil null = no end.
	ValidFrom  string  `json:"valid_from"`
	ValidUntil *string `json:"valid_until"`
	IsActive   *bool   `json:"is_active"`
	Note       string  `json:"note"`
}

// ScheduleFilter narrows the timetable list.
type ScheduleFilter struct {
	Q         string
	Branches  []uuid.UUID
	Services  []uuid.UUID
	Staff     []uuid.UUID
	Weekdays  []string
	IsActive  *bool
	SortField string
	SortDesc  bool
	Limit     int
	Offset    int
}

// Facet is one filter option with the number of classes.
type Facet struct {
	UUID  uuid.UUID `json:"uuid"`
	Name  string    `json:"name"`
	Count int64     `json:"count"`
}

// Facets are the filter options of the timetable and the session list.
type Facets struct {
	Branches []Facet `json:"branches"`
	Services []Facet `json:"services"`
	Staff    []Facet `json:"staff"`
}

// Session is one session of a class.
type Session struct {
	ScheduleUUID uuid.UUID `json:"schedule_uuid"`
	// Date is the local date of the session (its identity in the class).
	Date     string       `json:"date"`
	Weekday  string       `json:"weekday"`
	StartsAt time.Time    `json:"starts_at"`
	EndsAt   time.Time    `json:"ends_at"`
	Branch   Ref          `json:"branch"`
	Service  Ref          `json:"service"`
	Staff    *Ref         `json:"staff"`
	Resource *ResourceRef `json:"resource"`
	// Capacity is the seats of the session; Booked the seats taken by
	// bookings (requests waiting for approval or payment included).
	Capacity  int `json:"capacity"`
	Booked    int `json:"booked"`
	SeatsLeft int `json:"seats_left"`
	// State: open, full, cancelled or past.
	State     string `json:"state"`
	Cancelled bool   `json:"cancelled"`
	// CapacityChanged: the session has its own seats.
	CapacityChanged bool   `json:"capacity_changed"`
	Reason          string `json:"reason"`
	Timezone        string `json:"timezone"`
}

// SessionFilter narrows the session list. From / To are local dates.
type SessionFilter struct {
	From      string
	To        string
	Q         string
	Branches  []uuid.UUID
	Services  []uuid.UUID
	Staff     []uuid.UUID
	States    []string
	SortField string
	SortDesc  bool
	Limit     int
	Offset    int
}

// Customer is the customer of an attendee.
type Customer struct {
	UUID  uuid.UUID `json:"uuid"`
	Name  string    `json:"name"`
	Phone *string   `json:"phone"`
}

// Attendee is one booking of a session.
type Attendee struct {
	ReservationUUID    uuid.UUID `json:"reservation_uuid"`
	Number             int64     `json:"number"`
	Status             string    `json:"status"`
	ApprovalStatus     string    `json:"approval_status"`
	PaymentStatus      string    `json:"payment_status"`
	ConfirmationStatus string    `json:"confirmation_status"`
	Source             string    `json:"source"`
	Seats              int       `json:"seats"`
	Customer           Customer  `json:"customer"`
	CreatedAt          time.Time `json:"created_at"`
}

// SessionDetail is a session with its bookings.
type SessionDetail struct {
	Session
	Attendees []Attendee `json:"attendees"`
	// Cancellation is the caller's run that cancelled the session with its
	// bookings (TEC-989) while it can still be rolled back; nil otherwise.
	Cancellation *SessionCancellation `json:"cancellation"`
	// Move is the caller's run that moved the session's bookings to another
	// session of the class (TEC-994) while it can still be rolled back.
	Move *SessionMove `json:"move"`
}

// SessionsSlug is the bulk engine resource of "cancel a session with its
// bookings" (TEC-989): its items are the session's reservations.
const SessionsSlug = "tenant.class_sessions"

// CancelBookingsAction is the bulk action that cancels the bookings of a
// cancelled session; its rollback opens the session again and takes the
// bookings back.
const CancelBookingsAction = "cancel_bookings"

// CancelReason is the cancel reason of a booking whose session was
// cancelled (shown with the reservation, sent to the customer).
const CancelReason = "class_cancelled"

// SessionCancelInput is the body of POST …/sessions/{date}/cancel.
type SessionCancelInput struct {
	Reason string `json:"reason"`
}

// SessionCancellation is a run that cancelled a session's bookings.
type SessionCancellation struct {
	JobUUID   uuid.UUID `json:"job_uuid"`
	Total     int       `json:"total"`
	Succeeded int       `json:"succeeded"`
	Failed    int       `json:"failed"`
	Skipped   int       `json:"skipped"`
	// RollbackUntil ends the undo window (nil: nothing to undo, no booking
	// was cancelled).
	RollbackUntil *time.Time `json:"rollback_until"`
	CreatedAt     time.Time  `json:"created_at"`
}

// SessionCancelResult answers POST …/sessions/{date}/cancel: the session
// as it is now and the run over its bookings (nil when it had none open).
type SessionCancelResult struct {
	Session      SessionDetail        `json:"session"`
	Cancellation *SessionCancellation `json:"cancellation"`
}

// MoveBookingsAction is the bulk action that moves the open bookings of a
// session to another session of the class (TEC-994); its rollback moves
// them back.
const MoveBookingsAction = "move_bookings"

// SessionMoveInput is the body of POST …/sessions/{date}/move: the local
// date of the target session of the same class.
type SessionMoveInput struct {
	ToDate string `json:"to_date"`
}

// SessionMoveIssue is one booking a move left where it was.
type SessionMoveIssue struct {
	ReservationUUID string `json:"reservation_uuid"`
	Label           string `json:"label"`
	// Error: slot_taken (no seat, trainer or area busy), stale, invalid_transition, not_in_session …
	Error   string `json:"error"`
	Skipped bool   `json:"skipped"`
}

// SessionMove is a run that moved a session's bookings to another session.
type SessionMove struct {
	JobUUID   uuid.UUID          `json:"job_uuid"`
	ToDate    string             `json:"to_date"`
	Total     int                `json:"total"`
	Succeeded int                `json:"succeeded"`
	Failed    int                `json:"failed"`
	Skipped   int                `json:"skipped"`
	Issues    []SessionMoveIssue `json:"issues"`
	// RollbackUntil ends the undo window (nil: nothing to undo).
	RollbackUntil *time.Time `json:"rollback_until"`
	CreatedAt     time.Time  `json:"created_at"`
}

// SessionMoveResult answers POST …/sessions/{date}/move: both sessions as
// they are now and the run over the bookings (nil when none was open).
type SessionMoveResult struct {
	Session SessionDetail `json:"session"`
	Target  SessionDetail `json:"target"`
	Move    *SessionMove  `json:"move"`
}

// SessionChangeInput sets how one session differs from its class:
// cancelled, or other seats (capacity null = the class's seats).
type SessionChangeInput struct {
	Cancelled bool   `json:"cancelled"`
	Capacity  *int   `json:"capacity"`
	Reason    string `json:"reason"`
}

// FormTrainer is a person who may teach a group class.
type FormTrainer struct {
	UUID        uuid.UUID   `json:"uuid"`
	Name        string      `json:"name"`
	IsActive    bool        `json:"is_active"`
	BranchUUIDs []uuid.UUID `json:"branch_uuids"`
}

// FormService is a group service with the branches that offer it and the
// people who may teach it.
type FormService struct {
	UUID            uuid.UUID     `json:"uuid"`
	Name            string        `json:"name"`
	GroupCapacity   int           `json:"group_capacity"`
	StaffMode       string        `json:"staff_mode"`
	ResourceKinds   []string      `json:"resource_kinds"`
	DurationMinutes int           `json:"duration_minutes"`
	IsActive        bool          `json:"is_active"`
	BranchUUIDs     []uuid.UUID   `json:"branch_uuids"`
	Trainers        []FormTrainer `json:"trainers"`
}

// FormResource is a resource a class can run in.
type FormResource struct {
	UUID       uuid.UUID `json:"uuid"`
	Name       string    `json:"name"`
	Kind       string    `json:"kind"`
	IsActive   bool      `json:"is_active"`
	BranchUUID uuid.UUID `json:"branch_uuid"`
}

// FormOptions are the choices of the class form.
type FormOptions struct {
	Services  []FormService  `json:"services"`
	Resources []FormResource `json:"resources"`
}
