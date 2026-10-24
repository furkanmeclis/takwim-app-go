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

// Customer confirmation ("Geleceğim"); the confirmation module (TEC-921)
// moves it, every other booking stays not_required.
const (
	ConfirmationNotRequired = "not_required"
	ConfirmationScheduled   = "scheduled"
	ConfirmationPending     = "pending"
	ConfirmationConfirmed   = "confirmed"
	ConfirmationDeclined    = "declined"
	ConfirmationExpired     = "expired"
)

// ReasonConfirmationTimeout is the cancel reason of a booking the customer
// did not confirm by the deadline (K-007).
const ReasonConfirmationTimeout = "confirmation_timeout"

// Confirmation is the confirmation task of a booking (TEC-921): when the
// customer is asked, until when an answer counts and what happened.
type Confirmation struct {
	// Status: scheduled, pending, confirmed, declined, expired, dropped
	// (no longer applies: the visit moved on or the add-on was turned off).
	Status          string      `json:"status"`
	Channels        []string    `json:"channels"`
	OnNoAnswer      string      `json:"on_no_answer"`
	AttemptTimes    []time.Time `json:"attempt_times"`
	AttemptsSent    int         `json:"attempts_sent"`
	AttemptsPlanned int         `json:"attempts_planned"`
	NextAttemptAt   *time.Time  `json:"next_attempt_at"`
	DeadlineAt      time.Time   `json:"deadline_at"`
	// Answer: confirm, cancel or change, with the channel it came on
	// (panel, visit_link, whatsapp, voice_call, phone).
	Answer            *string    `json:"answer"`
	AnswerChannel     *string    `json:"answer_channel"`
	AnsweredAt        *time.Time `json:"answered_at"`
	ChangeRequestedAt *time.Time `json:"change_requested_at"`
	ChangeNote        string     `json:"change_note"`
	// Outcome of an expired task: cancelled, flagged or unreachable.
	Outcome   *string    `json:"outcome"`
	LastError string     `json:"last_error"`
	PlannedAt time.Time  `json:"planned_at"`
	ClosedAt  *time.Time `json:"closed_at"`
}

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
	// ReasonCustomerCancelled: the customer cancelled in the customer panel
	// or through a visit link.
	ReasonCustomerCancelled = "customer_cancelled"
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
	// PartySize and Zone: the number of people and the area asked for
	// (restaurant); the resources must seat the party.
	PartySize *int   `json:"party_size"`
	Zone      string `json:"zone"`
	// Vehicle is the car of a car wash booking (optional).
	Vehicle *Vehicle `json:"vehicle"`
	// Products are taken with the visit (TEC-919); their units are
	// reserved in the branch's stock with the booking.
	Products []ProductInput `json:"products"`
	// BundleUUID books a combined service bundle (TEC-895): the lines are
	// its services in order (empty lines = the bundle's services with any
	// free staff member) and the bundle price is spread over them.
	BundleUUID *uuid.UUID `json:"bundle_uuid"`
}

// States of a product of a visit (TEC-919).
const (
	// ProductHeld: the units are reserved in the branch's stock.
	ProductHeld = "held"
	// ProductDelivered: handed over when the visit was completed (a stock
	// sale and a product sale line).
	ProductDelivered = "delivered"
	// ProductReleased: removed, or the visit was cancelled, rejected,
	// expired or a no-show; the units are free again.
	ProductReleased = "released"
)

// Limits of the products of a visit.
const (
	MaxProductQuantity = 99
	MaxProductLines    = 20
)

// ProductInput adds a product (or one of its variants) to a visit.
// StaffUUID credits the sale to a person (default: the person serving the
// first line); the public booking page never sends it.
type ProductInput struct {
	ProductUUID uuid.UUID  `json:"product_uuid"`
	VariantUUID *uuid.UUID `json:"variant_uuid"`
	Quantity    int        `json:"quantity"`
	StaffUUID   *uuid.UUID `json:"staff_uuid"`
}

// AddProductInput is the body of POST …/{uuid}/products.
type AddProductInput struct {
	ProductInput
	ExpectedVersion *int `json:"expected_version"`
}

// Product is one product of a visit with its price snapshot and state.
type Product struct {
	UUID          uuid.UUID   `json:"uuid"`
	Product       Ref         `json:"product"`
	Variant       *Ref        `json:"variant"`
	SKU           string      `json:"sku"`
	Quantity      int         `json:"quantity"`
	UnitPrice     money.Money `json:"unit_price"`
	Total         money.Money `json:"total"`
	Staff         *Ref        `json:"staff"`
	Status        string      `json:"status"`
	ReleaseReason *string     `json:"release_reason"`
	Source        string      `json:"source"`
	DeliveredAt   *time.Time  `json:"delivered_at"`
	ReleasedAt    *time.Time  `json:"released_at"`
	CreatedAt     time.Time   `json:"created_at"`
}

// States of a package session (TEC-920).
const (
	// SessionHeld: the session is reserved for the line.
	SessionHeld = "held"
	// SessionConsumed: the visit was completed; the session is used.
	SessionConsumed = "consumed"
	// SessionReleased: removed, or the visit was cancelled, rejected,
	// expired, a no-show or moved to another service.
	SessionReleased = "released"
)

// SessionValue is the value of session no (1-based) of a package of
// total sessions for price: the price split evenly, the remainder on the
// last session, so the sessions add up to the price (5 × 2.500,00 TL =
// 500,00 TL each).
func SessionValue(price money.Money, total, no int) money.Money {
	if total < 1 {
		return money.Zero()
	}
	base := price.Minor() / int64(total)
	if no >= total {
		return money.NewIn(price.Minor()-base*int64(total-1), price.Currency())
	}
	return money.NewIn(base, price.Currency())
}

// PackageSession is one session of a customer's package given to a line
// of the visit (TEC-920).
type PackageSession struct {
	UUID            uuid.UUID   `json:"uuid"`
	LineIndex       int         `json:"line_index"`
	CustomerPackage Ref         `json:"customer_package"`
	SessionNo       int         `json:"session_no"`
	TotalSessions   int         `json:"total_sessions"`
	Amount          money.Money `json:"amount"`
	Status          string      `json:"status"`
	ReleaseReason   *string     `json:"release_reason"`
	ConsumedAt      *time.Time  `json:"consumed_at"`
	ReleasedAt      *time.Time  `json:"released_at"`
	CreatedAt       time.Time   `json:"created_at"`
}

// ApplyPackageInput is the body of POST …/{uuid}/package-sessions: use a
// session of one of the customer's packages for a line of the visit.
type ApplyPackageInput struct {
	CustomerPackageUUID uuid.UUID `json:"customer_package_uuid"`
	LineIndex           int       `json:"line_index"`
	ExpectedVersion     *int      `json:"expected_version"`
}

// PackageOption is a package of the visit's customer that can give a
// session to some of its lines: active, valid on the visit day, usable in
// the branch, with sessions left.
type PackageOption struct {
	CustomerPackageUUID uuid.UUID   `json:"customer_package_uuid"`
	Name                string      `json:"name"`
	ServiceName         string      `json:"service_name"`
	ServiceUUID         *uuid.UUID  `json:"service_uuid"`
	TotalSessions       int         `json:"total_sessions"`
	Remaining           int         `json:"remaining"`
	ExpiresOn           *string     `json:"expires_on"`
	NextValue           money.Money `json:"next_value"`
	// LineIndexes are the lines of the visit it can serve (same service,
	// not on a session yet).
	LineIndexes []int `json:"line_indexes"`
}

// ProductOption is a stock item that can be added to a visit in its
// branch, with the units available there (on hand - reserved).
type ProductOption struct {
	ProductUUID uuid.UUID   `json:"product_uuid"`
	ProductName string      `json:"product_name"`
	SKU         string      `json:"sku"`
	VariantUUID *uuid.UUID  `json:"variant_uuid"`
	VariantName *string     `json:"variant_name"`
	Price       money.Money `json:"price"`
	Available   int64       `json:"available"`
	// ImageUUID is the product cover (public media route).
	ImageUUID *uuid.UUID `json:"-"`
}

// Vehicle types of a car wash booking.
var VehicleTypes = []string{"car", "suv", "van", "motorcycle", "other"}

// MaxPlateLength limits a licence plate (stored upper case, single spaces).
const MaxPlateLength = 16

// Vehicle is the car of a car wash booking: its type and licence plate.
type Vehicle struct {
	Type  string `json:"type"`
	Plate string `json:"plate"`
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
	// Bundle is set on a line booked as part of a service bundle; ListPrice
	// is then the line's price before the bundle discount and UnitPrice its
	// share of the bundle price.
	Bundle    *LineBundle  `json:"bundle"`
	ListPrice *money.Money `json:"list_price"`
}

// LineBundle names the bundle of a line; UUID is nil when the bundle was
// deleted (the name stays).
type LineBundle struct {
	UUID *uuid.UUID `json:"uuid"`
	Name string     `json:"name"`
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
	// CustomAnswers are the booking form answers with the question each
	// was answered under (TEC-905), in the form's order.
	CustomAnswers []CustomAnswer `json:"custom_answers"`
	// SettingsVersion is the business settings version the booking was
	// made under (null for bookings made before TEC-905).
	SettingsVersion *int     `json:"settings_version"`
	CustomerNote    string   `json:"customer_note"`
	InternalNote    string   `json:"internal_note"`
	PartySize       *int     `json:"party_size"`
	Zone            string   `json:"zone"`
	Vehicle         *Vehicle `json:"vehicle"`
	Version         int      `json:"version"`
	CancelReason    *string  `json:"cancel_reason"`
	// HoldExpiresAt is when a waiting request lets its capacity go.
	HoldExpiresAt *time.Time `json:"hold_expires_at"`
	Lines         []Line     `json:"lines"`
	Products      []Product  `json:"products"`
	// PackageSessions are the sessions of the customer's packages the
	// lines use (TEC-920), in the order they were given.
	PackageSessions []PackageSession `json:"package_sessions"`
	Events          []Event          `json:"events"`
	// Confirmation is the customer confirmation task (nil: none planned).
	Confirmation *Confirmation `json:"confirmation"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// CustomAnswer is one booking form answer with its question as it was
// when answered. A key without a stored question (an older free-form
// answer) has its key as label.
type CustomAnswer struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Type: text, number, select or checkbox.
	Type  string `json:"type"`
	Value any    `json:"value"`
	// Online: the question is asked on the customer screens.
	Online bool `json:"online"`
}

// FormField is a booking form question of the business (TEC-905).
type FormField struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Help     string   `json:"help"`
	Type     string   `json:"type"`
	Options  []string `json:"options"`
	Required bool     `json:"required"`
	// Online / Panel: asked on the customer screens / in the panel.
	Online bool `json:"online"`
	Panel  bool `json:"panel"`
}

// Form is GET /v1/tenant/reservations/form: the booking form questions
// and the optional flow steps the business hides, for the panel's booking
// form and detail.
type Form struct {
	Fields      []FormField `json:"fields"`
	HiddenSteps []string    `json:"hidden_steps"`
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
	// PartySize / Zone change the party (nil keeps the booked one).
	PartySize *int    `json:"party_size"`
	Zone      *string `json:"zone"`
	// Channel plans the new start on the panel channel (empty) or the
	// online one (the customer panel: the online booking rules apply).
	// Never read from a request body.
	Channel string `json:"-"`
	// Guard, when set, runs on the locked record before anything changes
	// (the customer panel checks ownership and the change cutoff there).
	Guard func(Locked) error `json:"-"`
}

// Locked is what a guard sees of a reservation locked for a change.
type Locked struct {
	CustomerID         int64
	BranchID           int64
	Status             string
	PaymentStatus      string
	ConfirmationStatus string
	StartsAt           time.Time
	Version            int
}

// NotesInput edits the notes and form answers; a nil CustomFields keeps
// the stored answers.
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
	// ConfirmationStatuses: customer confirmation (TEC-921).
	ConfirmationStatuses []string
	PaymentStatuses      []string
	Sources              []string
	StartsFrom           *time.Time
	StartsTo             *time.Time
	SortField            string
	SortDesc             bool
	Limit, Offset        int32
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
	PartySize          *int        `json:"party_size"`
	Zone               string      `json:"zone"`
	Vehicle            *Vehicle    `json:"vehicle"`
	// CustomAnswers are the booking form answers (TEC-905).
	CustomAnswers []CustomAnswer `json:"custom_answers"`
	Version       int            `json:"version"`
	CreatedAt     time.Time      `json:"created_at"`
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
	PartySize           *int      `json:"party_size"`
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
