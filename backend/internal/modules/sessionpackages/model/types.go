// Package model holds the types of the session packages module (TEC-920).
package model

import (
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Engine slugs (activity resources and live channels).
const (
	PackagesSlug         = "tenant.session_packages"
	CustomerPackagesSlug = "tenant.customer_packages"
)

// States of a customer package.
const (
	// StateActive: sessions left and valid.
	StateActive = "active"
	// StateUsedUp: every session was used.
	StateUsedUp = "used_up"
	// StateExpired: sessions were left after the last valid day.
	StateExpired = "expired"
	// StateRefunded: the package was refunded (closed).
	StateRefunded = "refunded"
)

// States lists the customer package states.
var States = []string{StateActive, StateUsedUp, StateExpired, StateRefunded}

// Methods a package is paid or refunded with.
var Methods = []string{"cash", "card", "transfer", "online", "other"}

// Limits.
const (
	MaxNameLength        = 160
	MaxDescriptionLength = 2000
	MaxSessions          = 1000
	MaxValidityDays      = 3650
	MaxNoteLength        = 500
	MaxIdempotencyKey    = 100
)

// Ref names a related record.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// Package is a session package the business sells.
type Package struct {
	UUID          uuid.UUID   `json:"uuid"`
	Name          string      `json:"name"`
	Description   string      `json:"description"`
	Service       Ref         `json:"service"`
	TotalSessions int         `json:"total_sessions"`
	Price         money.Money `json:"price"`
	// SessionValue is the value of one session (the last one also takes
	// the rounding remainder).
	SessionValue money.Money `json:"session_value"`
	Currency     string      `json:"currency"`
	// ValidityDays: days from the sale a package can be used; nil = no end.
	ValidityDays *int      `json:"validity_days"`
	AllBranches  bool      `json:"all_branches"`
	Branches     []Ref     `json:"branches"`
	IsActive     bool      `json:"is_active"`
	SoldCount    int64     `json:"sold_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// PackageInput is the body of POST and PATCH /v1/tenant/session-packages
// (the whole definition each time).
type PackageInput struct {
	Name          string      `json:"name"`
	Description   string      `json:"description"`
	ServiceUUID   uuid.UUID   `json:"service_uuid"`
	TotalSessions int         `json:"total_sessions"`
	Price         money.Money `json:"price"`
	ValidityDays  *int        `json:"validity_days"`
	AllBranches   bool        `json:"all_branches"`
	BranchUUIDs   []uuid.UUID `json:"branch_uuids"`
	IsActive      *bool       `json:"is_active"`
}

// PackageFilter narrows the package list.
type PackageFilter struct {
	Q         string
	IsActive  *bool
	Services  []uuid.UUID
	Branch    *uuid.UUID
	SortField string
	SortDesc  bool
	Limit     int32
	Offset    int32
}

// CustomerRef is the customer of a package.
type CustomerRef struct {
	UUID  uuid.UUID `json:"uuid"`
	Name  string    `json:"name"`
	Phone *string   `json:"phone"`
}

// Refund is how a package was closed.
type Refund struct {
	Amount     money.Money `json:"amount"`
	Reason     string      `json:"reason"`
	RefundedOn string      `json:"refunded_on"`
	RefundedAt time.Time   `json:"refunded_at"`
	EntryUUID  *uuid.UUID  `json:"entry_uuid"`
}

// CustomerPackage is a package sold to a customer with its balance.
type CustomerPackage struct {
	UUID        uuid.UUID   `json:"uuid"`
	Name        string      `json:"name"`
	ServiceName string      `json:"service_name"`
	ServiceUUID *uuid.UUID  `json:"service_uuid"`
	PackageUUID *uuid.UUID  `json:"package_uuid"`
	Customer    CustomerRef `json:"customer"`
	// Branch sold it; Branches are where the sessions can be used (empty
	// with AllBranches).
	Branch        Ref         `json:"branch"`
	AllBranches   bool        `json:"all_branches"`
	Branches      []Ref       `json:"branches"`
	TotalSessions int         `json:"total_sessions"`
	Used          int         `json:"used"`
	Held          int         `json:"held"`
	Remaining     int         `json:"remaining"`
	Price         money.Money `json:"price"`
	Currency      string      `json:"currency"`
	SessionValue  money.Money `json:"session_value"`
	UsedAmount    money.Money `json:"used_amount"`
	HeldAmount    money.Money `json:"held_amount"`
	// RemainingAmount is the value of the sessions neither used nor held.
	RemainingAmount money.Money `json:"remaining_amount"`
	SoldOn          string      `json:"sold_on"`
	ExpiresOn       *string     `json:"expires_on"`
	State           string      `json:"state"`
	Staff           *Ref        `json:"staff"`
	Note            string      `json:"note"`
	CollectionUUID  *uuid.UUID  `json:"collection_uuid"`
	Refund          *Refund     `json:"refund"`
	CreatedAt       time.Time   `json:"created_at"`
	// Sessions: on the detail only.
	Sessions []Session `json:"sessions,omitempty"`
}

// ReservationRef names the visit of a session.
type ReservationRef struct {
	UUID     uuid.UUID `json:"uuid"`
	Number   int64     `json:"number"`
	StartsAt time.Time `json:"starts_at"`
	Status   string    `json:"status"`
}

// Session is one session of a package given to a visit.
type Session struct {
	UUID          uuid.UUID      `json:"uuid"`
	SessionNo     int            `json:"session_no"`
	Amount        money.Money    `json:"amount"`
	Status        string         `json:"status"`
	ReleaseReason *string        `json:"release_reason"`
	Reservation   ReservationRef `json:"reservation"`
	ServiceName   *string        `json:"service_name"`
	StaffName     *string        `json:"staff_name"`
	ConsumedOn    *string        `json:"consumed_on"`
	ConsumedAt    *time.Time     `json:"consumed_at"`
	ReleasedAt    *time.Time     `json:"released_at"`
	CreatedAt     time.Time      `json:"created_at"`
}

// SellInput is the body of POST /v1/tenant/customer-packages: sell a
// package to a customer in a branch and take its price into a cash or
// bank account.
type SellInput struct {
	PackageUUID     uuid.UUID  `json:"package_uuid"`
	CustomerUUID    uuid.UUID  `json:"customer_uuid"`
	BranchUUID      uuid.UUID  `json:"branch_uuid"`
	Method          string     `json:"method"`
	CashAccountUUID *uuid.UUID `json:"cash_account_uuid"`
	StaffUUID       *uuid.UUID `json:"staff_uuid"`
	Note            string     `json:"note"`
	IdempotencyKey  string     `json:"idempotency_key"`
}

// RefundInput is the body of POST /v1/tenant/customer-packages/{uuid}/refund:
// close the package and pay back amount (0 = nothing paid back).
type RefundInput struct {
	Amount          money.Money `json:"amount"`
	Method          string      `json:"method"`
	CashAccountUUID *uuid.UUID  `json:"cash_account_uuid"`
	Reason          string      `json:"reason"`
	IdempotencyKey  string      `json:"idempotency_key"`
}

// CustomerPackageFilter narrows the customer package list.
type CustomerPackageFilter struct {
	Q         string
	Branches  []uuid.UUID
	Customer  *uuid.UUID
	Packages  []uuid.UUID
	States    []string
	DateFrom  *time.Time
	DateTo    *time.Time
	SortField string
	SortDesc  bool
	Limit     int32
	Offset    int32
}

// Facet is one filter option with its count.
type Facet struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Count int64  `json:"count"`
}

// Facets are the dynamic filter options of the customer package list.
type Facets struct {
	Package []Facet `json:"package"`
	Branch  []Facet `json:"branch"`
}

// Count is a number of records and their amount.
type Count struct {
	Count    int64       `json:"count"`
	Sessions int64       `json:"sessions"`
	Amount   money.Money `json:"amount"`
}

// Figures are the package figures of one currency.
type Figures struct {
	Currency string `json:"currency"`
	// Sold: packages sold in the range (money taken; not revenue).
	Sold Count `json:"sold"`
	// Used: sessions used in the range (their value is service revenue
	// through the visits' sale lines).
	Used Count `json:"used"`
	// Refunded: packages refunded in the range and the money paid back.
	Refunded Count `json:"refunded"`
	// Open: valid packages today, the sessions neither used nor held and
	// their value (unused rights).
	Open Count `json:"open"`
	// Held: sessions booked on visits not completed yet.
	Held Count `json:"held"`
	// Expired: sessions left on packages past their last day and their
	// value.
	Expired Count `json:"expired"`
	// UsedUp: packages whose every session was used.
	UsedUp int64 `json:"used_up"`
}

// Summary is the package report of a date range.
type Summary struct {
	DateFrom string    `json:"date_from"`
	DateTo   string    `json:"date_to"`
	Today    string    `json:"today"`
	Totals   []Figures `json:"totals"`
}
