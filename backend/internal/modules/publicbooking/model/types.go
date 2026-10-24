// Package model holds the shapes of the public booking API (TEC-906): the
// contract the business page (takwim.link/{slug}), the embedded widget and
// the mobile app share. Only customer-facing fields are here: no internal
// ids, notes, resources, buffers, provider keys or staff contact details.
package model

import (
	"time"

	"github.com/google/uuid"

	availability "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	reservations "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Sales channels of a public caller (organization settings
// sales_channels): the takwim.link page, the embedded widget and the API
// (mobile app, partners).
const (
	ChannelWeb    = "web"
	ChannelWidget = "widget"
	ChannelAPI    = "api"
)

// Channels lists the valid channels.
var Channels = []string{ChannelWeb, ChannelWidget, ChannelAPI}

// Reasons a business does not take public bookings (booking.unavailable).
const (
	UnavailableModule   = "module_disabled"
	UnavailableNotReady = "online_not_ready"
	UnavailableChannel  = "channel_disabled"
)

// Checkout statuses.
const (
	CheckoutActive   = "active"
	CheckoutExpired  = "expired"
	CheckoutReleased = "released"
	CheckoutBooked   = "booked"
)

// Ref names a record.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// Contact are the public contact details of the business.
type Contact struct {
	Phone   string `json:"phone"`
	Email   string `json:"email"`
	Website string `json:"website"`
	Address string `json:"address"`
}

// Policies are the customer-facing policy texts.
type Policies struct {
	Cancellation string `json:"cancellation"`
	Change       string `json:"change"`
	NoShow       string `json:"no_show"`
}

// Booking says whether and how the business takes public bookings.
type Booking struct {
	// Channels are the sales channels switched on.
	Channels map[string]bool `json:"channels"`
	// Ready is true when the booking rules are complete and the plan
	// includes the booking page; a channel books when Ready and on.
	Ready bool `json:"ready"`
	// Unavailable lists why bookings are refused (module_disabled,
	// online_not_ready); empty when Ready.
	Unavailable         []string `json:"unavailable"`
	ApprovalMode        string   `json:"approval_mode"`
	MinLeadMinutes      *int     `json:"min_lead_minutes"`
	MaxAdvanceDays      *int     `json:"max_advance_days"`
	CancelCutoffMinutes *int     `json:"cancel_cutoff_minutes"`
	ChangeCutoffMinutes *int     `json:"change_cutoff_minutes"`
	// PaymentMethods: on_site and / or online.
	PaymentMethods []string `json:"payment_methods"`
	// OnlinePaymentReady: online payment is offered and the business has a
	// working payment provider and an online cash account (TEC-912); the
	// customer is sent to the provider's page after booking.
	OnlinePaymentReady bool `json:"online_payment_ready"`
	// CheckoutMinutes is how long a checkout keeps its time.
	CheckoutMinutes int `json:"checkout_minutes"`
	// SlotStepMinutes: the distance between the starts the business offers
	// online (null = the client's choice); availability uses it whatever
	// the request asks.
	SlotStepMinutes *int `json:"slot_step_minutes"`
	// MaxOpenPerCustomer: the most open bookings a customer may hold
	// (null = no limit).
	MaxOpenPerCustomer *int `json:"max_open_per_customer"`
	// HiddenSteps are the optional sector steps the business hides (they
	// are already left out of Flow); the API refuses their choices.
	HiddenSteps []string `json:"hidden_steps"`
	// Fields are the booking form questions asked online, in order; the
	// answers go in the booking's custom_fields by key.
	Fields []Field `json:"fields"`
}

// Field is a booking form question asked on the customer screens.
type Field struct {
	Key string `json:"key"`
	// Label and Help are the business's own text.
	Label string `json:"label"`
	Help  string `json:"help"`
	// Type: text, number, select or checkbox.
	Type     string   `json:"type"`
	Options  []string `json:"options"`
	Required bool     `json:"required"`
}

// Answer is a booking form answer with its question as answered.
type Answer struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Type  string `json:"type"`
	Value any    `json:"value"`
}

// Business is the profile and page context of GET /v1/public/businesses/{slug}.
type Business struct {
	UUID     uuid.UUID `json:"uuid"`
	Slug     string    `json:"slug"`
	Name     string    `json:"name"`
	LogoURL  *string   `json:"logo_url"`
	Sector   string    `json:"sector"`
	Timezone string    `json:"timezone"`
	Currency string    `json:"currency"`
	// Flow is the customer booking flow (step codes): the sector's flow
	// without the optional steps the business hides (TEC-905).
	Flow []string `json:"flow"`
	// Terms are the sector's term codes (staff, resource, booking).
	Terms       map[string]string `json:"terms"`
	Contact     Contact           `json:"contact"`
	Policies    Policies          `json:"policies"`
	Booking     Booking           `json:"booking"`
	BranchCount int               `json:"branch_count"`
	Embed       Embed             `json:"embed"`
	// Theme is the look of the page (TEC-966): only customer-facing values.
	Theme Theme `json:"theme"`
}

// Theme is the business page theme (TEC-966): colours as `#rrggbb` (empty =
// the app's neutral colour), a font code the page ships, the welcome texts
// and the API-owned cover image URL (null = none; the page then falls back
// to the first branch photo). The page checks the contrast of the colours
// and keeps the neutral theme where it is too low.
type Theme struct {
	PrimaryColor string  `json:"primary_color"`
	AccentColor  string  `json:"accent_color"`
	Font         string  `json:"font"`
	Headline     string  `json:"headline"`
	Description  string  `json:"description"`
	CoverURL     *string `json:"cover_url"`
}

// Embed is what the embedded booking screen (TEC-908) needs: the sites
// allowed to frame it (its frame-ancestors policy and the only targets of
// its height messages).
type Embed struct {
	AllowedOrigins []string `json:"allowed_origins"`
}

// Image is an API-owned image URL.
type Image struct {
	UUID uuid.UUID `json:"uuid"`
	URL  string    `json:"url"`
}

// Hours are the opening hours of one weekday.
type Hours struct {
	Closed bool   `json:"closed"`
	Open   string `json:"open"`
	Close  string `json:"close"`
}

// SpecialDay replaces the weekly hours on a date.
type SpecialDay struct {
	Date   string `json:"date"`
	Closed bool   `json:"closed"`
	Open   string `json:"open"`
	Close  string `json:"close"`
	Note   string `json:"note"`
}

// Branch is a branch shown on the customer screens.
type Branch struct {
	UUID         uuid.UUID        `json:"uuid"`
	Slug         string           `json:"slug"`
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	Address      string           `json:"address"`
	City         string           `json:"city"`
	District     string           `json:"district"`
	Latitude     *float64         `json:"latitude"`
	Longitude    *float64         `json:"longitude"`
	Phone        string           `json:"phone"`
	Email        string           `json:"email"`
	Timezone     string           `json:"timezone"`
	WorkingHours map[string]Hours `json:"working_hours"`
	// SpecialDays are today's and later dated exceptions.
	SpecialDays []SpecialDay `json:"special_days"`
	Images      []Image      `json:"images"`
	// Zones are the areas a party may ask for (restaurant tables).
	Zones []string `json:"zones"`
}

// ServiceOffer is a service in one branch.
type ServiceOffer struct {
	BranchUUID      uuid.UUID   `json:"branch_uuid"`
	Price           money.Money `json:"price"`
	DurationMinutes int         `json:"duration_minutes"`
}

// ServiceStaff is a person who may perform a service; Price /
// DurationMinutes are the person's own (null = the branch's).
type ServiceStaff struct {
	StaffUUID       uuid.UUID    `json:"staff_uuid"`
	Price           *money.Money `json:"price"`
	DurationMinutes *int         `json:"duration_minutes"`
}

// Service is a bookable service. With a branch filter Price and
// DurationMinutes are the branch's; otherwise the list values.
type Service struct {
	UUID            uuid.UUID   `json:"uuid"`
	Name            string      `json:"name"`
	Description     string      `json:"description"`
	Category        *Ref        `json:"category"`
	DurationMinutes int         `json:"duration_minutes"`
	Price           money.Money `json:"price"`
	Currency        string      `json:"currency"`
	PaymentMode     string      `json:"payment_mode"`
	StaffMode       string      `json:"staff_mode"`
	ImageURL        *string     `json:"image_url"`
	// GroupCapacity: seats of a group class session (TEC-904); null for
	// an individual service. A group class is booked on its own.
	GroupCapacity *int           `json:"group_capacity"`
	Branches      []ServiceOffer `json:"branches"`
	Staff         []ServiceStaff `json:"staff"`
}

// BundleItem is one service of a bundle as offered online, in serving
// order. Price is the branch's (the list price with no branch filter);
// AllocatedPrice its share of the bundle price at that price (the booking
// splits the bundle price again over the final line prices, the same way).
type BundleItem struct {
	ServiceUUID     uuid.UUID   `json:"service_uuid"`
	ServiceName     string      `json:"service_name"`
	DurationMinutes int         `json:"duration_minutes"`
	Price           money.Money `json:"price"`
	AllocatedPrice  money.Money `json:"allocated_price"`
	StaffMode       string      `json:"staff_mode"`
}

// Bundle is a combined service bundle a customer may book online
// (TEC-985): active, shown online, every service of it bookable online in
// the branch. ListPrice is the sum of the item prices and Discount what the
// bundle saves (zero when it costs more).
type Bundle struct {
	UUID            uuid.UUID    `json:"uuid"`
	Name            string       `json:"name"`
	Description     string       `json:"description"`
	Price           money.Money  `json:"price"`
	ListPrice       money.Money  `json:"list_price"`
	Discount        money.Money  `json:"discount"`
	Currency        string       `json:"currency"`
	DurationMinutes int          `json:"duration_minutes"`
	PaymentModes    []string     `json:"payment_modes"`
	Items           []BundleItem `json:"items"`
	BranchUUIDs     []uuid.UUID  `json:"branch_uuids"`
}

// Staff is a person shown on the customer screens.
type Staff struct {
	UUID         uuid.UUID   `json:"uuid"`
	Name         string      `json:"name"`
	Title        string      `json:"title"`
	Bio          string      `json:"bio"`
	BranchUUIDs  []uuid.UUID `json:"branch_uuids"`
	ServiceUUIDs []uuid.UUID `json:"service_uuids"`
}

// AvailabilityInput is the POST …/availability body. With BundleUUID the
// lines are the bundle's services in order (empty = every service with any
// free person; given = those services, each with the person picked).
type AvailabilityInput struct {
	BranchUUID  uuid.UUID                `json:"branch_uuid"`
	BundleUUID  *uuid.UUID               `json:"bundle_uuid"`
	Lines       []availability.LineInput `json:"lines"`
	From        string                   `json:"from"`
	To          string                   `json:"to"`
	StepMinutes *int                     `json:"step_minutes"`
	Channel     string                   `json:"channel"`
	// PartySize (restaurant: required) and the preferred area.
	PartySize *int   `json:"party_size"`
	Zone      string `json:"zone"`
}

// SlotLine is who serves one line of a slot.
type SlotLine struct {
	ServiceUUID uuid.UUID `json:"service_uuid"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
	Staff       *Ref      `json:"staff"`
	// SeatsLeft: free seats of a group class session (null otherwise).
	SeatsLeft *int `json:"seats_left"`
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

// LineInfo is a requested line as planned.
type LineInfo struct {
	ServiceUUID     uuid.UUID `json:"service_uuid"`
	ServiceName     string    `json:"service_name"`
	DurationMinutes int       `json:"duration_minutes"`
	StaffMode       string    `json:"staff_mode"`
	Staff           []Ref     `json:"staff"`
	// GroupCapacity: seats of a group class session (null otherwise).
	GroupCapacity *int `json:"group_capacity"`
}

// Availability is the answer of POST …/availability.
type Availability struct {
	Branch        Ref        `json:"branch"`
	Timezone      string     `json:"timezone"`
	EarliestStart time.Time  `json:"earliest_start"`
	LatestDate    *string    `json:"latest_date"`
	StepMinutes   int        `json:"step_minutes"`
	Lines         []LineInfo `json:"lines"`
	Days          []Day      `json:"days"`
}

// CheckoutInput is the POST …/checkouts body: the slot the customer picked.
// BundleUUID books a bundle (TEC-985); see AvailabilityInput.
type CheckoutInput struct {
	BranchUUID uuid.UUID                `json:"branch_uuid"`
	BundleUUID *uuid.UUID               `json:"bundle_uuid"`
	Lines      []availability.LineInput `json:"lines"`
	StartsAt   *time.Time               `json:"starts_at"`
	Channel    string                   `json:"channel"`
	PartySize  *int                     `json:"party_size"`
	Zone       string                   `json:"zone"`
}

// CheckoutLine is a planned line of a checkout (stored as JSON). Bundle
// names the service bundle the line belongs to (TEC-985); the booking then
// books the bundle, so its price is split as in the panel.
type CheckoutLine struct {
	ServiceUUID uuid.UUID `json:"service_uuid"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
	Staff       *Ref      `json:"staff"`
	Bundle      *Ref      `json:"bundle,omitempty"`
}

// Checkout is a timed hold of the customer's slot. Bundle is the service
// bundle the lines belong to (null for single services).
type Checkout struct {
	Token     uuid.UUID      `json:"token"`
	Status    string         `json:"status"`
	Channel   string         `json:"channel"`
	Branch    Ref            `json:"branch"`
	Bundle    *Ref           `json:"bundle"`
	StartsAt  time.Time      `json:"starts_at"`
	EndsAt    time.Time      `json:"ends_at"`
	ExpiresAt *time.Time     `json:"expires_at"`
	Lines     []CheckoutLine `json:"lines"`
	PartySize *int           `json:"party_size"`
	Zone      string         `json:"zone"`
	// ReservationUUID is set once the checkout was booked.
	ReservationUUID *uuid.UUID `json:"reservation_uuid"`
}

// CustomerInput identifies the customer by phone.
type CustomerInput struct {
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Phone        string `json:"phone"`
	Email        string `json:"email"`
	SMSConsent   bool   `json:"sms_consent"`
	EmailConsent bool   `json:"email_consent"`
}

// BookingInput is the POST …/reservations body. Website is a honeypot:
// people never see the field, bots fill it in.
type BookingInput struct {
	CheckoutToken uuid.UUID      `json:"checkout_token"`
	Customer      CustomerInput  `json:"customer"`
	Payment       string         `json:"payment"`
	CustomerNote  string         `json:"customer_note"`
	CustomFields  map[string]any `json:"custom_fields"`
	Website       string         `json:"website"`
	// Vehicle: the car of a car wash booking (optional).
	Vehicle *reservations.Vehicle `json:"vehicle"`
	// Products: optional products taken with the visit (TEC-919), from
	// GET …/products of the checkout's branch.
	Products []BookingProduct `json:"products"`
	// PackageSessions pay lines of the checkout (by index) with sessions of
	// the customer's packages (TEC-984). Only a signed-in customer panel
	// session may name them (POST /v1/customer/businesses/{slug}/reservations);
	// the anonymous route refuses them.
	PackageSessions []reservations.PackageSessionInput `json:"package_sessions"`
}

// BookingProduct is one product of a booking.
type BookingProduct struct {
	ProductUUID uuid.UUID  `json:"product_uuid"`
	VariantUUID *uuid.UUID `json:"variant_uuid"`
	Quantity    int        `json:"quantity"`
}

// MaxProductQuantity is the most units of one item a customer may take.
const MaxProductQuantity = 10

// Product is a product (or variant) the customer may add to a booking in
// a branch: active, shown online, sold there and with units available.
type Product struct {
	ProductUUID uuid.UUID   `json:"product_uuid"`
	Name        string      `json:"name"`
	VariantUUID *uuid.UUID  `json:"variant_uuid"`
	VariantName *string     `json:"variant_name"`
	Price       money.Money `json:"price"`
	// MaxQuantity is how many the customer may take (the available units,
	// at most MaxProductQuantity); the stock itself is not shown.
	MaxQuantity int     `json:"max_quantity"`
	ImageURL    *string `json:"image_url"`
}

// BookedProduct is a product of a booked visit.
type BookedProduct struct {
	Name        string      `json:"name"`
	VariantName *string     `json:"variant_name"`
	Quantity    int         `json:"quantity"`
	UnitPrice   money.Money `json:"unit_price"`
	// Status: held (reserved for the visit), delivered or released.
	Status string `json:"status"`
}

// Line is one service of a booked visit.
type Line struct {
	ServiceUUID     *uuid.UUID  `json:"service_uuid"`
	ServiceName     string      `json:"service_name"`
	Staff           *StaffName  `json:"staff"`
	StartsAt        time.Time   `json:"starts_at"`
	EndsAt          time.Time   `json:"ends_at"`
	DurationMinutes int         `json:"duration_minutes"`
	UnitPrice       money.Money `json:"unit_price"`
	// PackageName: the line is paid by a session of this package of the
	// customer (TEC-984); it is not part of the total.
	PackageName *string `json:"package_name"`
	// BundleName: the line was booked as part of this service bundle
	// (TEC-985); UnitPrice is its share of the bundle price and ListPrice
	// the price before the bundle discount.
	BundleName *string      `json:"bundle_name"`
	ListPrice  *money.Money `json:"list_price"`
}

// StaffName is the staff member of a line (UUID nil when deleted).
type StaffName struct {
	UUID *uuid.UUID `json:"uuid"`
	Name string     `json:"name"`
}

// Reservation is the customer's view of a booking. AccessToken is only in
// the answer of the booking itself; the status route needs it.
type Reservation struct {
	UUID               uuid.UUID   `json:"uuid"`
	Number             int64       `json:"number"`
	Status             string      `json:"status"`
	ApprovalStatus     string      `json:"approval_status"`
	ConfirmationStatus string      `json:"confirmation_status"`
	PaymentStatus      string      `json:"payment_status"`
	Branch             Ref         `json:"branch"`
	Timezone           string      `json:"timezone"`
	StartsAt           time.Time   `json:"starts_at"`
	EndsAt             time.Time   `json:"ends_at"`
	TotalPrice         money.Money `json:"total_price"`
	Currency           string      `json:"currency"`
	CustomerNote       string      `json:"customer_note"`
	// Answers are the booking form answers of the questions asked online.
	Answers   []Answer              `json:"answers"`
	PartySize *int                  `json:"party_size"`
	Zone      string                `json:"zone"`
	Vehicle   *reservations.Vehicle `json:"vehicle"`
	// ExpiresAt: a request waiting for the business or for online payment
	// lets its time go at this instant.
	ExpiresAt    *time.Time `json:"expires_at"`
	CancelReason *string    `json:"cancel_reason"`
	// CanPayOnline: the booking waits for its online payment and the page
	// can be opened now (POST …/reservations/{uuid}/payment).
	CanPayOnline bool            `json:"can_pay_online"`
	Lines        []Line          `json:"lines"`
	Products     []BookedProduct `json:"products"`
	CreatedAt    time.Time       `json:"created_at"`
	AccessToken  string          `json:"access_token,omitempty"`
}

// Class session states of the public timetable (TEC-990).
const (
	// ClassOpen: bookable now on the channel for the asked seats.
	ClassOpen = "open"
	// ClassFull: fewer free seats than asked.
	ClassFull = "full"
	// ClassCancelled: the business cancelled the session.
	ClassCancelled = "cancelled"
	// ClassPast: the session has started.
	ClassPast = "past"
	// ClassTooSoon / ClassTooFar: inside the minimum notice, or after the
	// furthest day the business books online.
	ClassTooSoon = "too_soon"
	ClassTooFar  = "too_far"
	// ClassUnavailable: not offered for another reason (the trainer or the
	// area is taken, the branch is closed, the daily online limit).
	ClassUnavailable = "unavailable"
)

// ClassesInput is the query of GET …/classes.
type ClassesInput struct {
	BranchUUID  uuid.UUID
	ServiceUUID *uuid.UUID
	From        string
	To          string
	Channel     string
	// PartySize is the seats the customer wants to take (default 1).
	PartySize *int
}

// ClassSession is one session of the class timetable as a customer sees
// it. The area is only its name (no resource id leaves the API).
type ClassSession struct {
	Service Ref `json:"service"`
	// Date is the local date of the session in the branch.
	Date     string    `json:"date"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	// Trainer: the person of the session's bookings, else the timetable's,
	// else the one the slot engine would assign; null when none is known.
	Trainer *Ref `json:"trainer"`
	// TrainerChanged: the session keeps the trainer of its bookings, who
	// is no longer the timetable's.
	TrainerChanged bool    `json:"trainer_changed"`
	Area           *string `json:"area"`
	Capacity       int     `json:"capacity"`
	// CapacityChanged: the session has its own seats.
	CapacityChanged bool `json:"capacity_changed"`
	SeatsLeft       int  `json:"seats_left"`
	// State: open, full, cancelled, past, too_soon, too_far or unavailable.
	State string `json:"state"`
}

// ClassTimetable is the answer of GET …/classes: the sessions of the
// branch's group classes on the local dates [From, To], in order.
type ClassTimetable struct {
	Branch        Ref            `json:"branch"`
	Timezone      string         `json:"timezone"`
	From          string         `json:"from"`
	To            string         `json:"to"`
	PartySize     int            `json:"party_size"`
	EarliestStart *time.Time     `json:"earliest_start"`
	LatestDate    *string        `json:"latest_date"`
	Sessions      []ClassSession `json:"sessions"`
}
