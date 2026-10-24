// Package model holds the API shapes of the shared customer panel
// (TEC-909, takwim.link/panel). Every answer carries customer-facing
// fields only: no internal notes, staff earnings, cash or expense data, no
// internal ids.
package model

import (
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// StartInput asks for a sign-in code.
type StartInput struct {
	Phone string `json:"phone"`
}

// StartStatus is the only answer of a code request: the same whether the
// phone is known, a cooldown applied or nothing was sent.
const StartStatus = "code_sent_if_known"

// StartResult answers a code request.
type StartResult struct {
	Status             string `json:"status"`
	PhoneMasked        string `json:"phone_masked"`
	ResendAfterSeconds int    `json:"resend_after_seconds"`
	CodeTTLSeconds     int    `json:"code_ttl_seconds"`
}

// VerifyInput exchanges a code for a panel session.
type VerifyInput struct {
	Phone string `json:"phone"`
	Code  string `json:"code"`
}

// Session is the answer of a verified sign-in. AccessToken is a customer
// token (kind customer): mobile apps send it as Bearer; the web BFF keeps
// it in an httpOnly cookie and strips it from the answer.
type Session struct {
	AccessToken string    `json:"access_token,omitempty"`
	TokenType   string    `json:"token_type,omitempty"`
	ExpiresAt   time.Time `json:"expires_at"`
	Profile     Profile   `json:"profile"`
}

// Profile is the signed-in person.
type Profile struct {
	Phone            string     `json:"phone"`
	PhoneDisplay     string     `json:"phone_display"`
	VerifiedAt       time.Time  `json:"verified_at"`
	SessionExpiresAt time.Time  `json:"session_expires_at"`
	ActiveSessions   int64      `json:"active_sessions"`
	Businesses       []Business `json:"businesses"`
	// HistoryNotice explains history closed without the person asking
	// (TEC-972): Takwim processed a reassignment notice of the number, or
	// the number had not been proven for a long time. nil otherwise.
	HistoryNotice *HistoryNotice `json:"history_notice"`
}

// HistoryNotice is why and when the profile's earlier history was closed.
type HistoryNotice struct {
	// Reason is ReleaseReassigned (platform notice) or ReleaseDormant.
	Reason   string    `json:"reason"`
	ClosedAt time.Time `json:"closed_at"`
}

// Business is one business that linked the person, with the name it keeps.
// HistoryFrom / HistoryUntil bound the visits shown (by booking time)
// after the number was reassigned to the person or changed (TEC-910); the
// name is empty while HistoryFrom is set (it may be the previous holder's).
type Business struct {
	UUID             uuid.UUID  `json:"uuid"`
	Slug             string     `json:"slug"`
	Name             string     `json:"name"`
	FirstName        string     `json:"first_name"`
	LastName         string     `json:"last_name"`
	LinkedAt         time.Time  `json:"linked_at"`
	ReservationCount int64      `json:"reservation_count"`
	HistoryFrom      *time.Time `json:"history_from"`
	HistoryUntil     *time.Time `json:"history_until"`
}

// LinkInput redeems a panel link (the token after "#" in
// takwim.link/panel/l#…). The token travels in the body, never in a URL.
type LinkInput struct {
	Token string `json:"token"`
}

// PhoneStartInput asks for a code on the new number of a number change.
type PhoneStartInput struct {
	Phone string `json:"phone"`
}

// ReassignedInput reports that the signed-in number was recently given to
// the person: every business's earlier history is hidden except the ones
// the person keeps as their own.
type ReassignedInput struct {
	KeepBusinesses []uuid.UUID `json:"keep_businesses"`
}

// Session methods: how a panel session was opened.
const (
	MethodCode = "code"
	MethodLink = "link"
)

// Device is one live panel session of the person (device list).
type Device struct {
	UUID       uuid.UUID `json:"uuid"`
	Current    bool      `json:"current"`
	Method     string    `json:"method"`
	Device     string    `json:"device"`
	IP         string    `json:"ip"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// DeviceList is the answer of GET /v1/customer/sessions.
type DeviceList struct {
	Items []Device `json:"items"`
}

// VisitInput opens one visit by its visit link (takwim.link/panel/v#…).
type VisitInput struct {
	Token string `json:"token"`
}

// VisitCancelInput cancels the visit of a visit link.
type VisitCancelInput struct {
	Token           string `json:"token"`
	ExpectedVersion *int   `json:"expected_version"`
}

// ConfirmInput is the customer's "Geleceğim" (TEC-921).
type ConfirmInput struct{}

// ChangeRequestInput asks the business for another time; the booking is
// kept until the business moves it.
type ChangeRequestInput struct {
	Note string `json:"note"`
}

// VisitConfirmInput confirms the visit of a visit link.
type VisitConfirmInput struct {
	Token string `json:"token"`
}

// VisitChangeRequestInput asks for a change through a visit link.
type VisitChangeRequestInput struct {
	Token string `json:"token"`
	Note  string `json:"note"`
}

// VisitLink is a link to one visit, for the message that carries it.
type VisitLink struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

// BusinessRef names the business of a record.
type BusinessRef struct {
	UUID uuid.UUID `json:"uuid"`
	Slug string    `json:"slug"`
	Name string    `json:"name"`
}

// Ref names a related record.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// Scopes of the reservation list.
const (
	ScopeUpcoming = "upcoming"
	ScopePast     = "past"
)

// ListFilter narrows the reservation list.
type ListFilter struct {
	Scope         string
	BusinessUUID  *uuid.UUID
	Statuses      []string
	SortDesc      bool
	WithMoney     bool
	Limit, Offset int32
}

// ReservationItem is a row of the reservation list.
type ReservationItem struct {
	UUID               uuid.UUID   `json:"uuid"`
	Number             int64       `json:"number"`
	Business           BusinessRef `json:"business"`
	Branch             Ref         `json:"branch"`
	Timezone           string      `json:"timezone"`
	StartsAt           time.Time   `json:"starts_at"`
	EndsAt             time.Time   `json:"ends_at"`
	Upcoming           bool        `json:"upcoming"`
	Status             string      `json:"status"`
	ApprovalStatus     string      `json:"approval_status"`
	ConfirmationStatus string      `json:"confirmation_status"`
	PaymentStatus      string      `json:"payment_status"`
	CancelReason       *string     `json:"cancel_reason"`
	Services           []string    `json:"services"`
	Staff              []string    `json:"staff"`
	Money              Money       `json:"money"`
	CreatedAt          time.Time   `json:"created_at"`
}

// Payment methods shown to the customer.
const (
	MethodOnSite = "on_site"
	MethodOnline = "online"
)

// Money is what the customer owes and paid for one visit, as recorded.
// Paid and Refunded come from the payment state until the payment ledger
// (TEC-912) records amounts; Balance is null when a finished visit is paid
// at the business (settled there, not recorded here).
type Money struct {
	Total         money.Money  `json:"total"`
	Paid          money.Money  `json:"paid"`
	Refunded      *money.Money `json:"refunded"`
	Balance       *money.Money `json:"balance"`
	Currency      string       `json:"currency"`
	Method        string       `json:"method"`
	PaymentStatus string       `json:"payment_status"`
}

// Line is one service of the visit.
type Line struct {
	ServiceName     string      `json:"service_name"`
	StaffName       *string     `json:"staff_name"`
	StartsAt        time.Time   `json:"starts_at"`
	EndsAt          time.Time   `json:"ends_at"`
	DurationMinutes int         `json:"duration_minutes"`
	UnitPrice       money.Money `json:"unit_price"`
}

// Branch is the place of the visit.
type Branch struct {
	UUID     uuid.UUID `json:"uuid"`
	Name     string    `json:"name"`
	Address  string    `json:"address"`
	City     string    `json:"city"`
	District string    `json:"district"`
	Phone    string    `json:"phone"`
}

// Policies are the business's customer-facing texts and limits.
type Policies struct {
	Cancellation        string `json:"cancellation"`
	Change              string `json:"change"`
	NoShow              string `json:"no_show"`
	CancelCutoffMinutes *int   `json:"cancel_cutoff_minutes"`
	ChangeCutoffMinutes *int   `json:"change_cutoff_minutes"`
}

// Reasons an action is not offered.
const (
	ReasonStatus              = "status"
	ReasonCutoff              = "cutoff"
	ReasonBusinessUnavailable = "business_unavailable"
	ReasonOnlineUnavailable   = "online_unavailable"
	ReasonServicesChanged     = "services_changed"
	// ReasonNotRequested: the business does not ask for a confirmation of
	// this visit (TEC-921).
	ReasonNotRequested = "not_requested"
	// ReasonAnswered: the confirmation was already answered (or a change
	// was already asked for).
	ReasonAnswered = "answered"
	// ReasonSignInRequired: a visit link does not move a visit; the panel
	// (signed in) does.
	ReasonSignInRequired = "sign_in_required"
)

// Action says whether the customer may take an action now, until when, and
// why not.
type Action struct {
	Allowed bool       `json:"allowed"`
	Until   *time.Time `json:"until"`
	Reason  *string    `json:"reason"`
}

// Actions are the customer's actions on a visit.
type Actions struct {
	Cancel     Action `json:"cancel"`
	Reschedule Action `json:"reschedule"`
	// Confirm ("Geleceğim") and RequestChange answer the business's
	// confirmation request (TEC-921); Until is the answer deadline.
	Confirm       Action `json:"confirm"`
	RequestChange Action `json:"request_change"`
}

// Confirmation is what the customer sees of the business's confirmation
// request (nil when the business does not ask).
type Confirmation struct {
	Status            string     `json:"status"`
	DeadlineAt        time.Time  `json:"deadline_at"`
	AnsweredAt        *time.Time `json:"answered_at"`
	ChangeRequestedAt *time.Time `json:"change_requested_at"`
}

// HistoryEntry is a customer-visible step of the visit (no notes, no
// actor).
type HistoryEntry struct {
	Type     string    `json:"type"`
	ToStatus *string   `json:"to_status"`
	At       time.Time `json:"at"`
}

// Reservation is the detail of one visit.
type Reservation struct {
	ReservationItem
	Branch        Branch         `json:"branch"`
	Version       int            `json:"version"`
	CustomerNote  string         `json:"customer_note"`
	HoldExpiresAt *time.Time     `json:"hold_expires_at"`
	Lines         []Line         `json:"lines"`
	Policies      Policies       `json:"policies"`
	Actions       Actions        `json:"actions"`
	Confirmation  *Confirmation  `json:"confirmation"`
	History       []HistoryEntry `json:"history"`
	// Refunds are the refunds of the visit (TEC-913), newest first.
	Refunds []Refund `json:"refunds"`
	// Products are the products taken with the visit (TEC-977).
	Products []Product `json:"products"`
}

// Product is what the customer sees of a product of the visit (TEC-977):
// held (reserved for the visit), delivered (handed over) or released (the
// visit was cancelled, rejected, expired or missed). Lines the business
// removed are not shown; no seller, cost or stock data.
type Product struct {
	UUID        uuid.UUID   `json:"uuid"`
	Name        string      `json:"name"`
	VariantName *string     `json:"variant_name"`
	Quantity    int         `json:"quantity"`
	UnitPrice   money.Money `json:"unit_price"`
	Total       money.Money `json:"total"`
	Status      string      `json:"status"`
}

// Refund is what the customer sees of one refund: requested (under way),
// succeeded (paid back) or failed (the business follows it up).
type Refund struct {
	UUID        uuid.UUID   `json:"uuid"`
	Amount      money.Money `json:"amount"`
	Currency    string      `json:"currency"`
	Status      string      `json:"status"`
	RequestedAt time.Time   `json:"requested_at"`
	RefundedAt  *time.Time  `json:"refunded_at"`
}

// CancelInput cancels a visit.
type CancelInput struct {
	ExpectedVersion *int `json:"expected_version"`
}

// AvailabilityInput asks for new starts of a visit (local dates of the
// branch, at most a week).
type AvailabilityInput struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Day is the free starts of one local date.
type Day struct {
	Date   string      `json:"date"`
	Status string      `json:"status"`
	Starts []time.Time `json:"starts"`
}

// Availability answers a reschedule search.
type Availability struct {
	Timezone      string    `json:"timezone"`
	EarliestStart time.Time `json:"earliest_start"`
	LatestDate    *string   `json:"latest_date"`
	Days          []Day     `json:"days"`
}

// RescheduleInput moves a visit to a new start (same services and staff).
type RescheduleInput struct {
	StartsAt        *time.Time `json:"starts_at"`
	ExpectedVersion *int       `json:"expected_version"`
}

// PaymentTotal sums one currency.
type PaymentTotal struct {
	Currency         string      `json:"currency"`
	Booked           money.Money `json:"booked"`
	Paid             money.Money `json:"paid"`
	RefundPending    money.Money `json:"refund_pending"`
	Refunded         money.Money `json:"refunded"`
	ReservationCount int64       `json:"reservation_count"`
}

// PaymentSummary is the per-currency summary of "Ödemelerim".
type PaymentSummary struct {
	Totals []PaymentTotal `json:"totals"`
}

// Release reasons and sources (customer_phone_releases, TEC-910 / TEC-972).
const (
	ReleaseNumberChanged = "number_changed"
	ReleaseReassigned    = "reassigned"
	ReleaseDormant       = "dormant"

	ReleaseSourceCustomer = "customer"
	ReleaseSourcePlatform = "platform"
	ReleaseSourceSystem   = "system"
)

// ReleaseReasons and ReleaseSources are the filter values of the platform
// list.
var (
	ReleaseReasons = []string{ReleaseNumberChanged, ReleaseReassigned, ReleaseDormant}
	ReleaseSources = []string{ReleaseSourceCustomer, ReleaseSourcePlatform, ReleaseSourceSystem}
)

// ReassignmentInput is a reassignment notice processed from Takwim
// management (TEC-972): the number was given to somebody else (operator
// notice, support request). Note says where the notice came from.
type ReassignmentInput struct {
	Phone string `json:"phone"`
	Note  string `json:"note"`
}

// PhoneRelease is one row of the platform list: a number that changed
// hands. The number is masked; HasProfile says whether a Takwim profile
// used it; BusinessesHidden and SessionsRevoked what the release closed.
type PhoneRelease struct {
	UUID             uuid.UUID `json:"uuid"`
	PhoneMasked      string    `json:"phone_masked"`
	Reason           string    `json:"reason"`
	Source           string    `json:"source"`
	Note             string    `json:"note"`
	CreatedByName    string    `json:"created_by_name"`
	HasProfile       bool      `json:"has_profile"`
	BusinessesHidden int       `json:"businesses_hidden"`
	SessionsRevoked  int       `json:"sessions_revoked"`
	ReleasedAt       time.Time `json:"released_at"`
}

// ReleaseFilter narrows the platform list. Q is a phone (exact match once
// normalized) or a part of the note; Reasons and Sources match any.
type ReleaseFilter struct {
	Q           string
	Reasons     []string
	Sources     []string
	OldestFirst bool
	Limit       int32
	Offset      int32
}
