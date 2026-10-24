// Package model holds the API shapes of the staff shares module: share
// rules (the agreed rates), the share of each sale line, payouts and the
// per staff / per branch figures (requirements §11).
package model

import (
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Engine slugs (activity, live lists).
const (
	RulesSlug   = "tenant.staff_share_rules"
	LinesSlug   = "tenant.staff_share_lines"
	PayoutsSlug = "tenant.staff_share_payouts"
)

// Item types a rule applies to.
const (
	ItemService = "service"
	ItemProduct = "product"
)

// ItemTypes lists the item types.
var ItemTypes = []string{ItemService, ItemProduct}

// Rule bases.
const (
	BasisPercent = "percent"
	BasisFixed   = "fixed"
)

// Bases lists the rule bases.
var Bases = []string{BasisPercent, BasisFixed}

// Which rule a share used.
const (
	ScopeDefault = "default"
	ScopeService = "service"
	ScopeProduct = "product"
)

// Share line kinds.
const (
	LineShare    = "share"
	LineReversal = "reversal"
)

// LineKinds lists the share line kinds.
var LineKinds = []string{LineShare, LineReversal}

// Share line statuses: waiting for the visit's collection, or payable.
const (
	StatusPending = "pending"
	StatusPayable = "payable"
)

// Statuses lists the share line statuses.
var Statuses = []string{StatusPending, StatusPayable}

// Payout kinds.
const (
	PayoutPayout   = "payout"
	PayoutReversal = "reversal"
)

// PayoutKinds lists the payout kinds.
var PayoutKinds = []string{PayoutPayout, PayoutReversal}

// Ref names a related record.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// ReservationRef names a reservation.
type ReservationRef struct {
	UUID   uuid.UUID `json:"uuid"`
	Number int64     `json:"number"`
}

// UserRef names the user who posted a row.
type UserRef struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
}

// Rule is an agreed rate of a staff member: the default of an item type
// (Service and Product nil) or the override for one service or one
// product, from ValidFrom on.
type Rule struct {
	UUID        uuid.UUID    `json:"uuid"`
	Staff       Ref          `json:"staff"`
	ItemType    string       `json:"item_type"`
	Service     *Ref         `json:"service"`
	Product     *Ref         `json:"product"`
	Basis       string       `json:"basis"`
	Rate        *money.Rate  `json:"rate"`
	FixedAmount *money.Money `json:"fixed_amount"`
	ValidFrom   string       `json:"valid_from"`
	Note        string       `json:"note"`
	// ShareCount is the number of shares calculated with the rule; they
	// keep their values whatever happens to the rule.
	ShareCount int64     `json:"share_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// RuleInput creates (every field but Note required, ServiceUUID /
// ProductUUID optional) or patches a rule (Basis, Rate, FixedAmount,
// ValidFrom, Note; the staff member, item type, service and product of a
// rule do not change).
type RuleInput struct {
	StaffUUID   *uuid.UUID   `json:"staff_uuid"`
	ItemType    *string      `json:"item_type"`
	ServiceUUID *uuid.UUID   `json:"service_uuid"`
	ProductUUID *uuid.UUID   `json:"product_uuid"`
	Basis       *string      `json:"basis"`
	Rate        *money.Rate  `json:"rate"`
	FixedAmount *money.Money `json:"fixed_amount"`
	ValidFrom   *string      `json:"valid_from"`
	Note        *string      `json:"note"`
}

// RuleFilter is the rule list query.
type RuleFilter struct {
	Staff         []uuid.UUID
	ItemTypes     []string
	Limit, Offset int32
}

// ShareLine is the share of one sale line (kind share) or the reversal of
// one (kind reversal, negative amounts). Rate / FixedAmount / Basis are the
// values applied at the time; Status follows the visit's collection.
type ShareLine struct {
	UUID           uuid.UUID       `json:"uuid"`
	Kind           string          `json:"kind"`
	ItemType       string          `json:"item_type"`
	Status         string          `json:"status"`
	RuleScope      string          `json:"rule_scope"`
	RuleUUID       *uuid.UUID      `json:"rule_uuid"`
	Basis          string          `json:"basis"`
	Rate           *money.Rate     `json:"rate"`
	FixedAmount    *money.Money    `json:"fixed_amount"`
	BaseAmount     money.Money     `json:"base_amount"`
	ShareAmount    money.Money     `json:"share_amount"`
	Currency       string          `json:"currency"`
	ShareDate      string          `json:"share_date"`
	Branch         Ref             `json:"branch"`
	Staff          Ref             `json:"staff"`
	SaleLineUUID   uuid.UUID       `json:"sale_line_uuid"`
	Description    string          `json:"description"`
	Quantity       int             `json:"quantity"`
	Reservation    *ReservationRef `json:"reservation"`
	Customer       *Ref            `json:"customer"`
	ReversesUUID   *uuid.UUID      `json:"reverses_uuid"`
	ReversedByUUID *uuid.UUID      `json:"reversed_by_uuid"`
	CreatedAt      time.Time       `json:"created_at"`
}

// LineFilter is the share line list query.
type LineFilter struct {
	Branches      []uuid.UUID
	Staff         []uuid.UUID
	Kinds         []string
	ItemTypes     []string
	Status        string
	DateFrom      *time.Time
	DateTo        *time.Time
	Q             string
	SortField     string
	SortDesc      bool
	Limit, Offset int32
}

// Balance is what a staff member is owed over every branch and day:
// Earned (every share), Payable (shares of collected sales), Paid
// (payouts), Remaining = Earned - Paid, PayableNow = Payable - Paid.
type Balance struct {
	Earned     money.Money `json:"earned"`
	Payable    money.Money `json:"payable"`
	Paid       money.Money `json:"paid"`
	Remaining  money.Money `json:"remaining"`
	PayableNow money.Money `json:"payable_now"`
}

// StaffSummary is one staff member in one currency: the period figures
// inside the caller's branch scope and the balance.
type StaffSummary struct {
	Staff    Ref    `json:"staff"`
	Currency string `json:"currency"`
	// The staff member's sales (net, after discounts), the shares of them,
	// the part waiting for the collection, the payable part and what stays
	// with the business (sales - shares, before costs and expenses).
	Sales         money.Money `json:"sales"`
	Share         money.Money `json:"share"`
	Pending       money.Money `json:"pending"`
	Payable       money.Money `json:"payable"`
	BusinessShare money.Money `json:"business_share"`
	SaleCount     int64       `json:"sale_count"`
	// Paid in the period from the accounts inside the caller's scope.
	PeriodPaid money.Money `json:"period_paid"`
	Balance    Balance     `json:"balance"`
}

// BranchSummary is the revenue of a branch in one currency, the staff
// shares of it and what stays with the business.
type BranchSummary struct {
	Branch        Ref         `json:"branch"`
	Currency      string      `json:"currency"`
	Revenue       money.Money `json:"revenue"`
	Share         money.Money `json:"share"`
	BusinessShare money.Money `json:"business_share"`
}

// Totals adds up the branches of one currency.
type Totals struct {
	Currency      string      `json:"currency"`
	Revenue       money.Money `json:"revenue"`
	Share         money.Money `json:"share"`
	BusinessShare money.Money `json:"business_share"`
	Pending       money.Money `json:"pending"`
	PeriodPaid    money.Money `json:"period_paid"`
}

// Summary is GET /v1/tenant/staff-shares/summary. Branches and Totals are
// empty for a member who sees only its own shares.
type Summary struct {
	DateFrom string          `json:"date_from"`
	DateTo   string          `json:"date_to"`
	Staff    []StaffSummary  `json:"staff"`
	Branches []BranchSummary `json:"branches"`
	Totals   []Totals        `json:"totals"`
}

// MyShares is GET /v1/tenant/staff-shares/me: the signed-in member's own
// shares today and in the period, and the balance (one entry per
// currency).
type MyShares struct {
	Staff    Ref            `json:"staff"`
	Today    string         `json:"today"`
	DateFrom string         `json:"date_from"`
	DateTo   string         `json:"date_to"`
	Daily    []StaffSummary `json:"daily"`
	Period   []StaffSummary `json:"period"`
}

// SummaryFilter is the summary query.
type SummaryFilter struct {
	DateFrom *time.Time
	DateTo   *time.Time
	Branches []uuid.UUID
	Staff    []uuid.UUID
}

// Payout is money paid to a staff member (kind payout) or the reversal of
// one (kind reversal), with its cash movement.
type Payout struct {
	UUID             uuid.UUID   `json:"uuid"`
	Kind             string      `json:"kind"`
	Staff            Ref         `json:"staff"`
	Amount           money.Money `json:"amount"`
	Currency         string      `json:"currency"`
	PayoutDate       string      `json:"payout_date"`
	Note             string      `json:"note"`
	CashAccount      Ref         `json:"cash_account"`
	CashMovementUUID uuid.UUID   `json:"cash_movement_uuid"`
	Branch           *Ref        `json:"branch"`
	ReversesUUID     *uuid.UUID  `json:"reverses_uuid"`
	ReversedByUUID   *uuid.UUID  `json:"reversed_by_uuid"`
	CreatedBy        *UserRef    `json:"created_by"`
	CreatedAt        time.Time   `json:"created_at"`
}

// PayoutInput pays a staff member from a cash or bank account. Date
// defaults to today; a request with an IdempotencyKey that was used before
// returns the first payout.
type PayoutInput struct {
	StaffUUID       uuid.UUID   `json:"staff_uuid"`
	CashAccountUUID uuid.UUID   `json:"cash_account_uuid"`
	Amount          money.Money `json:"amount"`
	Date            *string     `json:"date"`
	Note            string      `json:"note"`
	IdempotencyKey  string      `json:"idempotency_key"`
}

// ReverseInput reverses a payout.
type ReverseInput struct {
	Reason string `json:"reason"`
}

// PayoutFilter is the payout list query.
type PayoutFilter struct {
	Branches      []uuid.UUID
	Staff         []uuid.UUID
	Kinds         []string
	DateFrom      *time.Time
	DateTo        *time.Time
	Q             string
	SortField     string
	SortDesc      bool
	Limit, Offset int32
}
