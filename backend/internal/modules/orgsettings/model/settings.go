// Package model holds the typed organization settings document (TEC-690):
// company / billing identity, reservation and payment rules, policies,
// sales channels and tax mapping. The document is stored as JSONB in
// organization_settings_versions; every save is a new immutable version.
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

// Prepayment rules.
const (
	PrepaymentNone    = "none"
	PrepaymentFixed   = "fixed"
	PrepaymentPercent = "percent"
)

// SalesChannels are the channels an organization can sell through, in
// display order.
var SalesChannels = []string{"web", "widget", "api", "branch", "yolcu360", "obilet"}

// InvoiceItemTypes are the invoice line kinds that need an accounting
// mapping (codes are filled in by the organization; placeholders for the
// e-invoice integration).
var InvoiceItemTypes = []string{
	"rental", "extra_service", "insurance", "delivery", "fuel", "mileage", "late_return", "damage", "deposit",
}

// Settings is the full settings document.
type Settings struct {
	Company       Company        `json:"company"`
	Reservation   Reservation    `json:"reservation"`
	Payment       Payment        `json:"payment"`
	Policies      Policies       `json:"policies"`
	SalesChannels []SalesChannel `json:"sales_channels"`
	Tax           Tax            `json:"tax"`
}

// Company is the legal / billing identity printed on contracts and invoices.
type Company struct {
	LegalName string `json:"legal_name"`
	// TaxNumber is a VKN (10 digits) or, for sole proprietors, a TCKN
	// (11 digits); both are checksum-validated.
	TaxNumber string `json:"tax_number"`
	TaxOffice string `json:"tax_office"`
	Address   string `json:"address"`
	Phone     string `json:"phone"`
	Email     string `json:"email"`
}

// Reservation holds the online reservation flow rules. All four values are
// required before online sales can be published (Readiness).
type Reservation struct {
	// ApprovalMode is "auto" (confirmed on payment) or "staff" (an employee
	// confirms within StaffReviewWindowMinutes); empty = not configured.
	ApprovalMode             string `json:"approval_mode"`
	StaffReviewWindowMinutes *int   `json:"staff_review_window_minutes"`
	PaymentHoldMinutes       *int   `json:"payment_hold_minutes"`
	QuoteValidityMinutes     *int   `json:"quote_validity_minutes"`
}

// Payment holds the prepayment rule.
type Payment struct {
	// PrepaymentRule is none, fixed (PrepaymentValue is an amount) or percent
	// (PrepaymentValue is a percentage of the total).
	PrepaymentRule     string `json:"prepayment_rule"`
	PrepaymentValue    string `json:"prepayment_value"`
	FullPaymentAllowed bool   `json:"full_payment_allowed"`
}

// Policies are customer-facing policy texts and the late return rules.
type Policies struct {
	Cancellation           string `json:"cancellation"`
	NoShow                 string `json:"no_show"`
	EarlyReturn            string `json:"early_return"`
	Extension              string `json:"extension"`
	LateReturnGraceMinutes *int   `json:"late_return_grace_minutes"`
	// ExtraHourFee is a decimal amount ("150.00"); empty = not charged.
	ExtraHourFee string `json:"extra_hour_fee"`
	Fuel         string `json:"fuel"`
	Mileage      string `json:"mileage"`
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

// Defaults returns the settings of an organization that never saved any.
func Defaults() Settings {
	channels := make([]SalesChannel, 0, len(SalesChannels))
	for _, c := range SalesChannels {
		channels = append(channels, SalesChannel{Code: c, IsActive: c == "branch"})
	}
	mappings := make([]InvoiceItemMapping, 0, len(InvoiceItemTypes))
	for _, t := range InvoiceItemTypes {
		mappings = append(mappings, InvoiceItemMapping{ItemType: t})
	}
	return Settings{
		Payment:       Payment{PrepaymentRule: PrepaymentNone, FullPaymentAllowed: true},
		SalesChannels: channels,
		Tax:           Tax{InvoiceItemMappings: mappings},
	}
}

// Readiness reports whether online sales can be published and which
// settings are still missing (dot paths of the form fields).
func Readiness(s Settings) (bool, []string) {
	missing := []string{}
	if s.Reservation.ApprovalMode == "" {
		missing = append(missing, "reservation.approval_mode")
	}
	if s.Reservation.StaffReviewWindowMinutes == nil {
		missing = append(missing, "reservation.staff_review_window_minutes")
	}
	if s.Reservation.PaymentHoldMinutes == nil {
		missing = append(missing, "reservation.payment_hold_minutes")
	}
	if s.Reservation.QuoteValidityMinutes == nil {
		missing = append(missing, "reservation.quote_validity_minutes")
	}
	return len(missing) == 0, missing
}
