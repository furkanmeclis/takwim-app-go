// Package model holds the typed organization settings document (TEC-690,
// reworked for Takwim in TEC-892): company / billing identity, the default
// booking rules, payment options, customer-facing policies, sales channels
// and tax mapping. The document is stored as JSONB in
// organization_settings_versions; every save is a new immutable version.
//
// These are the business-wide defaults. The booking section (TEC-905)
// fine-tunes the sector's booking flow: optional steps the business hides
// and its own booking form questions.
package model

import (
	"time"

	"github.com/google/uuid"
)

// Resource is the activity / permission resource of organization settings.
const Resource = "tenant.settings"

// Reservation approval modes.
const (
	ApprovalAuto  = "auto"
	ApprovalStaff = "staff"
)

// SalesChannels are the online channels an organization can take bookings
// through, in display order: the takwim.link page, the embedded widget and
// the public API. Bookings entered in the panel are always allowed.
var SalesChannels = []string{"web", "widget", "api"}

// InvoiceItemTypes are the invoice line kinds that need an accounting
// mapping (codes are filled in by the organization; placeholders for the
// e-invoice integration).
var InvoiceItemTypes = []string{"service", "product", "package"}

// Settings is the full settings document.
type Settings struct {
	Company       Company        `json:"company"`
	Reservation   Reservation    `json:"reservation"`
	Payment       Payment        `json:"payment"`
	Policies      Policies       `json:"policies"`
	SalesChannels []SalesChannel `json:"sales_channels"`
	Embed         Embed          `json:"embed"`
	Tax           Tax            `json:"tax"`
	Booking       Booking        `json:"booking"`
}

// MaxEmbedOrigins is the most sites one business may embed its booking
// screen on.
const MaxEmbedOrigins = 20

// Embed holds the sites allowed to frame the booking screen (TEC-908). Each
// entry is a normalized origin (`https://example.com`, optionally
// `https://*.example.com`); the embed page answers them in its
// `frame-ancestors` policy. An empty list means no other site may embed it.
type Embed struct {
	AllowedOrigins []string `json:"allowed_origins"`
}

// Company is the legal / billing identity and the public contact details.
type Company struct {
	LegalName string `json:"legal_name"`
	// TaxNumber is a VKN (10 digits) or, for sole proprietors, a TCKN
	// (11 digits); both are checksum-validated.
	TaxNumber string `json:"tax_number"`
	TaxOffice string `json:"tax_office"`
	Address   string `json:"address"`
	Phone     string `json:"phone"`
	Email     string `json:"email"`
	Website   string `json:"website"`
}

// Reservation holds the default booking rules (requirements §6 and §8).
// A nil value means "not configured"; Readiness lists the ones online
// bookings need.
type Reservation struct {
	// ApprovalMode is "auto" (accepted immediately) or "staff" (the customer
	// leaves a request, an employee accepts it within
	// StaffReviewWindowMinutes while the capacity is held); empty = not
	// configured.
	ApprovalMode             string `json:"approval_mode"`
	StaffReviewWindowMinutes *int   `json:"staff_review_window_minutes"`
	// PaymentHoldMinutes is how long a booking waiting for online payment
	// keeps its capacity.
	PaymentHoldMinutes *int `json:"payment_hold_minutes"`
	// MinLeadMinutes is the earliest bookable start, counted from now.
	MinLeadMinutes *int `json:"min_lead_minutes"`
	// MaxAdvanceDays is the latest bookable day, counted from today.
	MaxAdvanceDays *int `json:"max_advance_days"`
	// CancelCutoffMinutes / ChangeCutoffMinutes: the customer may cancel /
	// ask for a change until this many minutes before the start; nil = no
	// limit.
	CancelCutoffMinutes *int `json:"cancel_cutoff_minutes"`
	ChangeCutoffMinutes *int `json:"change_cutoff_minutes"`
	// SlotStepMinutes is the distance between the starts offered online
	// (randevu aralığı); nil = the client's choice (15 minutes on the
	// business page). Online starts off the grid are refused.
	SlotStepMinutes *int `json:"slot_step_minutes"`
	// MaxOpenPerCustomer limits the open bookings (waiting, booked, in
	// service and not over) a customer may hold when booking online; nil
	// = no limit. Bookings entered in the panel are not limited.
	MaxOpenPerCustomer *int `json:"max_open_per_customer"`
	// MaxOnlinePerDay limits the online bookings a branch takes per local
	// day (page, widget and API; cancelled and rejected ones do not count);
	// nil = no limit.
	MaxOnlinePerDay *int `json:"max_online_per_day"`
	// NoShowUsesPackageSession: a visit marked as a no-show uses the
	// package sessions booked on it (the right is forfeited) instead of
	// giving them back to the package (TEC-984; default false).
	NoShowUsesPackageSession bool `json:"no_show_uses_package_session"`
}

// Payment says how customers may pay. Online payment is always the full
// amount in the beta (no deposits).
type Payment struct {
	PayAtVenue bool `json:"pay_at_venue"`
	PayOnline  bool `json:"pay_online"`
}

// Policies are customer-facing policy texts.
type Policies struct {
	Cancellation string `json:"cancellation"`
	NoShow       string `json:"no_show"`
	Change       string `json:"change"`
}

// SalesChannel switches one channel on or off.
type SalesChannel struct {
	Code     string `json:"code"`
	IsActive bool   `json:"is_active"`
}

// Tax holds the VAT default and the invoice item mapping placeholders. There
// is no built-in legal VAT rate: DefaultVATRate stays empty until the
// organization sets it.
type Tax struct {
	DefaultVATRate      string               `json:"default_vat_rate"`
	InvoiceItemMappings []InvoiceItemMapping `json:"invoice_item_mappings"`
}

// InvoiceItemMapping maps an invoice line kind to an accounting code.
type InvoiceItemMapping struct {
	ItemType    string `json:"item_type"`
	AccountCode string `json:"account_code"`
	VATRate     string `json:"vat_rate"`
	Description string `json:"description"`
}

// Optional steps of a sector flow a business may hide (requirements §6).
// Every other step (branch, service, date and time, party size, contact,
// summary) and every capacity and availability check stays: hiding the
// staff step lets the engine assign a free person, hiding the area step
// lets it pick any table, hiding products drops the optional products.
const (
	StepStaff    = "staff"
	StepProducts = "products"
	StepArea     = "area"
)

// HideableSteps lists the optional steps in display order.
var HideableSteps = []string{StepStaff, StepProducts, StepArea}

// Types of a booking form question.
const (
	FieldText     = "text"
	FieldNumber   = "number"
	FieldSelect   = "select"
	FieldCheckbox = "checkbox"
)

// FieldTypes lists the question types.
var FieldTypes = []string{FieldText, FieldNumber, FieldSelect, FieldCheckbox}

// Limits of the booking form.
const (
	MaxFields       = 20
	MaxFieldLabel   = 80
	MaxFieldHelp    = 200
	MaxFieldOptions = 20
	MaxOptionLength = 80
	// MaxTextAnswer limits a text answer.
	MaxTextAnswer = 500
)

// Booking fine-tunes the customer booking flow of the sector.
type Booking struct {
	// HiddenSteps are optional steps (HideableSteps) the customer screens
	// skip; the API refuses the choice they would make.
	HiddenSteps []string `json:"hidden_steps"`
	// Fields are the business's own booking form questions, in order.
	Fields []CustomField `json:"fields"`
}

// Hides reports whether the business hides an optional step.
func (b Booking) Hides(step string) bool {
	for _, s := range b.HiddenSteps {
		if s == step {
			return true
		}
	}
	return false
}

// CustomField is one booking form question. Key names the answer in a
// reservation's custom_fields and never changes once saved; Online /
// Panel say where it is asked (the booking page, widget and API / the
// panel booking form). A required checkbox must be ticked (consent).
type CustomField struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Options  []string `json:"options"`
	Required bool     `json:"required"`
	Online   bool     `json:"online"`
	Panel    bool     `json:"panel"`
	Help     string   `json:"help"`
}

// Actor is the user who saved a version.
type Actor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// Version is one stored settings version. Version 0 means nothing was saved
// yet (defaults).
type Version struct {
	Version             int        `json:"version"`
	Settings            Settings   `json:"settings"`
	ReadyForOnlineSales bool       `json:"ready_for_online_sales"`
	Missing             []string   `json:"missing"`
	CreatedAt           *time.Time `json:"created_at"`
	CreatedBy           *Actor     `json:"created_by"`
}

// VersionSummary is a row of the version history list.
type VersionSummary struct {
	Version             int       `json:"version"`
	ReadyForOnlineSales bool      `json:"ready_for_online_sales"`
	CreatedAt           time.Time `json:"created_at"`
	CreatedBy           *Actor    `json:"created_by"`
}

// SaveInput is the PUT body. BaseVersion, when set, must match the current
// version (optimistic concurrency: 409 otherwise).
type SaveInput struct {
	Settings    Settings `json:"settings"`
	BaseVersion *int     `json:"base_version"`
}

// Defaults returns the settings of an organization that never saved any:
// payment at the venue, every online channel off.
func Defaults() Settings {
	channels := make([]SalesChannel, 0, len(SalesChannels))
	for _, c := range SalesChannels {
		channels = append(channels, SalesChannel{Code: c})
	}
	mappings := make([]InvoiceItemMapping, 0, len(InvoiceItemTypes))
	for _, t := range InvoiceItemTypes {
		mappings = append(mappings, InvoiceItemMapping{ItemType: t})
	}
	return Settings{
		Payment:       Payment{PayAtVenue: true},
		SalesChannels: channels,
		Embed:         Embed{AllowedOrigins: []string{}},
		Tax:           Tax{InvoiceItemMappings: mappings},
		Booking:       Booking{HiddenSteps: []string{}, Fields: []CustomField{}},
	}
}

// Readiness reports whether online bookings can be published and which
// settings are still missing (dot paths of the form fields). The review
// window only matters for staff approval and the payment hold only when
// online payment is on.
func Readiness(s Settings) (bool, []string) {
	missing := []string{}
	r := s.Reservation
	if r.ApprovalMode == "" {
		missing = append(missing, "reservation.approval_mode")
	}
	if r.ApprovalMode == ApprovalStaff && r.StaffReviewWindowMinutes == nil {
		missing = append(missing, "reservation.staff_review_window_minutes")
	}
	if s.Payment.PayOnline && r.PaymentHoldMinutes == nil {
		missing = append(missing, "reservation.payment_hold_minutes")
	}
	if r.MinLeadMinutes == nil {
		missing = append(missing, "reservation.min_lead_minutes")
	}
	if r.MaxAdvanceDays == nil {
		missing = append(missing, "reservation.max_advance_days")
	}
	return len(missing) == 0, missing
}
