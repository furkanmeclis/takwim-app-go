// Package model holds the shapes of the reservation record (TEC-899).
package model

import (
	"time"

	"github.com/google/uuid"

	availability "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Lifecycle of the visit (requirements §8).
const (
	StatusRequested = "requested"
	StatusActive    = "active"
	StatusInService = "in_service"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"
	StatusRejected  = "rejected"
	StatusNoShow    = "no_show"
)

// The business's decision and how it was made.
const (
	ApprovalPending  = "pending"
	ApprovalAccepted = "accepted"
	ApprovalRejected = "rejected"

	ApprovalModeAuto  = "auto"
	ApprovalModeStaff = "staff"
	ApprovalModePanel = "panel"
)

// Customer confirmation ("Geleceğim"); the confirmation module (TEC-90x)
// moves it, every other booking stays not_required.
const (
	ConfirmationNotRequired = "not_required"
	ConfirmationScheduled   = "scheduled"
	ConfirmationPending     = "pending"
	ConfirmationConfirmed   = "confirmed"
	ConfirmationDeclined    = "declined"
	ConfirmationExpired     = "expired"
)

// Payment of the booking.
const (
	PaymentOnSite            = "on_site"
	PaymentOnlinePending     = "online_pending"
	PaymentPaid              = "paid"
	PaymentRefundPending     = "refund_pending"
	PaymentPartiallyRefunded = "partially_refunded"
	PaymentRefunded          = "refunded"
	PaymentFailed            = "failed"
)

// Payment methods a booking asks for.
const (
	MethodOnSite = "on_site"
	MethodOnline = "online"
)

// Where the booking came from.
const (
	SourcePanel  = "panel"
	SourceOnline = "online"
	SourceEmbed  = "embed"
)

// Cancel reasons set by the system (people give their own).
const (
	ReasonApprovalTimeout = "approval_timeout"
	ReasonPaymentTimeout  = "payment_timeout"
)

// CreateInput books a visit. Lines are the services in the order the
// customer is served (availability.LineInput: service and, optionally, a
// staff member). Payment is on_site or online; empty picks on_site when the
// business and every service allow it, else online.
type CreateInput struct {
	BranchUUID   uuid.UUID                `json:"branch_uuid"`
	CustomerUUID uuid.UUID                `json:"customer_uuid"`
	Lines        []availability.LineInput `json:"lines"`
	StartsAt     *time.Time               `json:"starts_at"`
	Source       string                   `json:"source"`
	Payment      string                   `json:"payment"`
	CustomFields map[string]any           `json:"custom_fields"`
	CustomerNote string                   `json:"customer_note"`
	InternalNote string                   `json:"internal_note"`
}

// Ref names a related record.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// Line is one service of the visit as it was booked.
type Line struct {
	Index               int                        `json:"index"`
	ServiceUUID         *uuid.UUID                 `json:"service_uuid"`
	ServiceName         string                     `json:"service_name"`
	Staff               *LineStaff                 `json:"staff"`
	Resources           []availability.ResourceRef `json:"resources"`
	StartsAt            time.Time                  `json:"starts_at"`
	EndsAt              time.Time                  `json:"ends_at"`
	DurationMinutes     int                        `json:"duration_minutes"`
	BufferBeforeMinutes int                        `json:"buffer_before_minutes"`
	BufferAfterMinutes  int                        `json:"buffer_after_minutes"`
	UnitPrice           money.Money                `json:"unit_price"`
}

// LineStaff is the staff member of a line; UUID is nil when the record was
// deleted (the name stays).
type LineStaff struct {
	UUID *uuid.UUID `json:"uuid"`
	Name string     `json:"name"`
}

// Event is one entry of the history.
type Event struct {
	Version    int       `json:"version"`
	Type       string    `json:"type"`
	FromStatus *string   `json:"from_status"`
	ToStatus   *string   `json:"to_status"`
	Actor      *string   `json:"actor"`
	Note       string    `json:"note"`
	CreatedAt  time.Time `json:"created_at"`
}

// Reservation is the record with its lines and history.
type Reservation struct {
	UUID               uuid.UUID      `json:"uuid"`
	Number             int64          `json:"number"`
	Status             string         `json:"status"`
	ApprovalMode       string         `json:"approval_mode"`
	ApprovalStatus     string         `json:"approval_status"`
	ConfirmationStatus string         `json:"confirmation_status"`
	PaymentStatus      string         `json:"payment_status"`
	Source             string         `json:"source"`
	Branch             Ref            `json:"branch"`
	Customer           Ref            `json:"customer"`
	StartsAt           time.Time      `json:"starts_at"`
	EndsAt             time.Time      `json:"ends_at"`
	TotalPrice         money.Money    `json:"total_price"`
	Currency           string         `json:"currency"`
	CustomFields       map[string]any `json:"custom_fields"`
	CustomerNote       string         `json:"customer_note"`
	InternalNote       string         `json:"internal_note"`
	Version            int            `json:"version"`
	CancelReason       *string        `json:"cancel_reason"`
	// HoldExpiresAt is when a waiting request lets its capacity go.
	HoldExpiresAt *time.Time `json:"hold_expires_at"`
	Lines         []Line     `json:"lines"`
	Events        []Event    `json:"events"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// Slug is the resource slug (activity, live lists).
const Slug = "tenant.reservations"

// RescheduleInput moves a booking: a new start and, optionally, new lines
// (empty = the current services and staff). Every line is planned again.
type RescheduleInput struct {
	StartsAt        *time.Time               `json:"starts_at"`
	Lines           []availability.LineInput `json:"lines"`
	ExpectedVersion *int                     `json:"expected_version"`
	Note            string                   `json:"note"`
}

// NotesInput edits the notes and form answers.
type NotesInput struct {
	CustomerNote    string         `json:"customer_note"`
	InternalNote    string         `json:"internal_note"`
	CustomFields    map[string]any `json:"custom_fields"`
	ExpectedVersion *int           `json:"expected_version"`
}

// ListFilter narrows the panel list.
type ListFilter struct {
	Q                string
	Branches         []uuid.UUID
	Staff            []uuid.UUID
	CustomerUUID     *uuid.UUID
	Statuses         []string
	ApprovalStatuses []string
	PaymentStatuses  []string
	Sources          []string
	StartsFrom       *time.Time
	StartsTo         *time.Time
	SortField        string
	SortDesc         bool
	Limit, Offset    int32
}

// Customer is the customer of a list row.
type Customer struct {
	UUID  uuid.UUID `json:"uuid"`
	Name  string    `json:"name"`
	Phone *string   `json:"phone"`
}

// ListItem is one row of the panel list.
type ListItem struct {
	UUID               uuid.UUID   `json:"uuid"`
	Number             int64       `json:"number"`
	Status             string      `json:"status"`
	ApprovalStatus     string      `json:"approval_status"`
	ConfirmationStatus string      `json:"confirmation_status"`
	PaymentStatus      string      `json:"payment_status"`
	Source             string      `json:"source"`
	Branch             Ref         `json:"branch"`
	Customer           Customer    `json:"customer"`
	Services           string      `json:"services"`
	Staff              string      `json:"staff"`
	StartsAt           time.Time   `json:"starts_at"`
	EndsAt             time.Time   `json:"ends_at"`
	TotalPrice         money.Money `json:"total_price"`
	Currency           string      `json:"currency"`
	Version            int         `json:"version"`
	CreatedAt          time.Time   `json:"created_at"`
}

// Facet is one filter option with its count.
type Facet struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Count int64  `json:"count"`
}

// Facets are the dynamic filter options of the list.
type Facets struct {
	Branch []Facet `json:"branch"`
	Staff  []Facet `json:"staff"`
}

// Calendar views (TEC-901): the columns of a branch calendar.
const (
	CalendarByStaff    = "staff"
	CalendarByResource = "resource"
	CalendarByBranch   = "branch"
	// CalendarUnassigned is the column of lines without a person / resource.
	CalendarUnassigned = "unassigned"
)

// CalendarFilter asks for the calendar of local dates From..To.
type CalendarFilter struct {
	By               string
	BranchUUID       *uuid.UUID
	From, To         string
	IncludeCancelled bool
}

// Interval is a range of instants.
type Interval struct {
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
}

// CalendarColumn is one column: a person, a resource, a branch or the
// unassigned lines. Work is when the column can take visits (shifts minus
// breaks, resource or branch hours); Off is leave or closures.
type CalendarColumn struct {
	Key          string     `json:"key"`
	Name         string     `json:"name"`
	Kind         string     `json:"kind"`
	ResourceKind *string    `json:"resource_kind"`
	Color        *string    `json:"color"`
	Work         []Interval `json:"work"`
	Off          []Interval `json:"off"`
}

// CalendarItem is one line of a visit placed in its columns.
type CalendarItem struct {
	ReservationUUID     uuid.UUID `json:"reservation_uuid"`
	Number              int64     `json:"number"`
	LineIndex           int       `json:"line_index"`
	Columns             []string  `json:"columns"`
	BranchUUID          uuid.UUID `json:"branch_uuid"`
	ServiceName         string    `json:"service_name"`
	StaffName           *string   `json:"staff_name"`
	CustomerName        string    `json:"customer_name"`
	StartsAt            time.Time `json:"starts_at"`
	EndsAt              time.Time `json:"ends_at"`
	BufferBeforeMinutes int       `json:"buffer_before_minutes"`
	BufferAfterMinutes  int       `json:"buffer_after_minutes"`
	Status              string    `json:"status"`
	ApprovalStatus      string    `json:"approval_status"`
	ConfirmationStatus  string    `json:"confirmation_status"`
	PaymentStatus       string    `json:"payment_status"`
	Source              string    `json:"source"`
	Version             int       `json:"version"`
}

// Calendar is the answer of the calendar view.
type Calendar struct {
	By       string           `json:"by"`
	Branch   *Ref             `json:"branch"`
	Timezone string           `json:"timezone"`
	From     string           `json:"from"`
	To       string           `json:"to"`
	Open     []Interval       `json:"open"`
	Columns  []CalendarColumn `json:"columns"`
	Items    []CalendarItem   `json:"items"`
}

// AgendaFilter asks for a member's own programme of local dates From..To.
type AgendaFilter struct {
	From, To         string
	IncludeCancelled bool
}

// AgendaStaff is the member's staff record.
type AgendaStaff struct {
	UUID  uuid.UUID `json:"uuid"`
	Name  string    `json:"name"`
	Color string    `json:"color"`
}

// AgendaWork is a working period of the member in one branch.
type AgendaWork struct {
	Branch   Ref       `json:"branch"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
}

// AgendaItem is one line the member serves, with what the member needs at
// the chair: the customer's phone and note, the branch.
type AgendaItem struct {
	CalendarItem
	BranchName    string  `json:"branch_name"`
	CustomerPhone *string `json:"customer_phone"`
	CustomerNote  string  `json:"customer_note"`
}

// Agenda is the member's own programme (TEC-902): shifts in every branch,
// leave and the lines it serves. Staff is nil when the member has no staff
// record (nothing to serve).
type Agenda struct {
	Staff    *AgendaStaff `json:"staff"`
	Timezone string       `json:"timezone"`
	From     string       `json:"from"`
	To       string       `json:"to"`
	Work     []AgendaWork `json:"work"`
	Off      []Interval   `json:"off"`
	Items    []AgendaItem `json:"items"`
}
