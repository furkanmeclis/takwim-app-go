// Package model holds the report payloads of /v1/tenant/reports (TEC-925).
package model

import (
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// HeadOffice is the branch filter value of the business-wide records
// (accounts without a branch: transfers, income / expenses and staff
// payouts made from them).
const HeadOffice = "head_office"

// Money figures the staff filter cannot narrow: they are not tied to a
// staff member, so a staff report leaves them out (zero) and lists them in
// Scope.OmittedForStaff. The stock is the branches' stock and is never
// narrowed by staff.
const (
	FieldCollections = "collections"
	FieldTransfers   = "transfers"
	FieldIncome      = "income"
	FieldExpenses    = "expenses"
)

// Filter is what the report covers. Every part is optional; the dates
// default to today (business time zone).
type Filter struct {
	DateFrom *time.Time
	DateTo   *time.Time
	// Branches narrows to these branches; HeadOffice adds the business-wide
	// records. Both empty = every branch in the caller's scope.
	Branches   []uuid.UUID
	HeadOffice bool
	Staff      []uuid.UUID
}

// Reservations counts the visits that start in the period by status and by
// channel. Total = the sum of the statuses.
type Reservations struct {
	Total      int64 `json:"total"`
	Requested  int64 `json:"requested"`
	Active     int64 `json:"active"`
	InService  int64 `json:"in_service"`
	Completed  int64 `json:"completed"`
	Cancelled  int64 `json:"cancelled"`
	Rejected   int64 `json:"rejected"`
	NoShow     int64 `json:"no_show"`
	FromPanel  int64 `json:"from_panel"`
	FromOnline int64 `json:"from_online"`
	FromEmbed  int64 `json:"from_embed"`
}

// Confirmations counts the confirmation tasks of the visits in the period
// by the state the confirmation list shows.
type Confirmations struct {
	Total           int64 `json:"total"`
	Scheduled       int64 `json:"scheduled"`
	Pending         int64 `json:"pending"`
	Confirmed       int64 `json:"confirmed"`
	Declined        int64 `json:"declined"`
	ChangeRequested int64 `json:"change_requested"`
	// Expired tasks by outcome: the visit was cancelled, left to the
	// business (flagged), or no request could be sent (unreachable).
	Cancelled   int64 `json:"cancelled"`
	Flagged     int64 `json:"flagged"`
	Unreachable int64 `json:"unreachable"`
	// Expired without an outcome (should not happen; kept so totals add up).
	Expired int64 `json:"expired"`
	// The visit was cancelled or moved before the task ended.
	Dropped int64 `json:"dropped"`
}

// Revenue is what was delivered (sale lines): gross price, discounts,
// reversals of earlier lines (negative) and the net, split into services
// and products.
type Revenue struct {
	Gross      money.Money `json:"gross"`
	Discount   money.Money `json:"discount"`
	Reversed   money.Money `json:"reversed"`
	Net        money.Money `json:"net"`
	ServiceNet money.Money `json:"service_net"`
	ProductNet money.Money `json:"product_net"`
	SaleCount  int64       `json:"sale_count"`
}

// Collections is the money taken and paid back (ledger); net = collected -
// refunded + reversed (reversed is negative when a collection was undone).
type Collections struct {
	Collected money.Money `json:"collected"`
	Refunded  money.Money `json:"refunded"`
	Reversed  money.Money `json:"reversed"`
	Net       money.Money `json:"net"`
	Count     int64       `json:"count"`
}

// Money are the figures of one currency. Revenue, collections, transfers
// and staff payouts are separate records and never derived from one
// another (A20).
//
//   - BusinessShare = revenue net - staff share (before product cost and
//     other expenses).
//   - OperationalResult = revenue net - product cost - staff share -
//     expenses. Other income, transfers, collections and staff payouts are
//     outside it (a payout settles a share that is already counted).
type Money struct {
	Currency          string      `json:"currency"`
	Revenue           Revenue     `json:"revenue"`
	Collections       Collections `json:"collections"`
	Transfers         money.Money `json:"transfers"`
	StaffShare        money.Money `json:"staff_share"`
	StaffPayments     money.Money `json:"staff_payments"`
	BusinessShare     money.Money `json:"business_share"`
	ProductCost       money.Money `json:"product_cost"`
	Income            money.Money `json:"income"`
	Expenses          money.Money `json:"expenses"`
	OperationalResult money.Money `json:"operational_result"`
}

// Stock is the stock of the branches: units moved in the period (by kind;
// adjustments signed) and the stock now (not bound to the period). The
// value is at the current unit cost in the business currency.
type Stock struct {
	Received       int64       `json:"received"`
	Sold           int64       `json:"sold"`
	Returned       int64       `json:"returned"`
	Adjusted       int64       `json:"adjusted"`
	TransferredIn  int64       `json:"transferred_in"`
	TransferredOut int64       `json:"transferred_out"`
	Movements      int64       `json:"movements"`
	OnHand         int64       `json:"on_hand"`
	Reserved       int64       `json:"reserved"`
	Items          int64       `json:"items"`
	LowItems       int64       `json:"low_items"`
	OutItems       int64       `json:"out_items"`
	Value          money.Money `json:"value"`
	Currency       string      `json:"currency"`
}

// Figures are the report of one day, one branch or the whole range.
type Figures struct {
	Reservations  Reservations  `json:"reservations"`
	Confirmations Confirmations `json:"confirmations"`
	// One entry per currency; at least the business currency.
	Money []Money `json:"money"`
}

// Day is one business day of the range that has any record.
type Day struct {
	Date string `json:"date"`
	Figures
}

// BranchRef names a branch; nil on the business-wide row.
type BranchRef struct {
	UUID     uuid.UUID `json:"uuid"`
	Code     string    `json:"code"`
	Name     string    `json:"name"`
	IsActive bool      `json:"is_active"`
}

// Branch is one row of the branch comparison. Branch nil = the
// business-wide records (head office).
type Branch struct {
	Branch *BranchRef `json:"branch"`
	Figures
	Stock Stock `json:"stock"`
}

// Totals are the figures of the whole range.
type Totals struct {
	Figures
	Stock Stock `json:"stock"`
}

// Scope says what the figures cover, so the screen can state it.
type Scope struct {
	Timezone string `json:"timezone"`
	Currency string `json:"currency"`
	// The branch filter of the request (empty = every branch the caller
	// sees) and whether business-wide records are included.
	Branches   []uuid.UUID `json:"branches"`
	HeadOffice bool        `json:"head_office"`
	Staff      []uuid.UUID `json:"staff"`
	// Money figures left out (zero) because the staff filter cannot narrow
	// them; empty without a staff filter.
	OmittedForStaff []string `json:"omitted_for_staff"`
}

// Report is the payload of GET /v1/tenant/reports/summary.
type Report struct {
	DateFrom string   `json:"date_from"`
	DateTo   string   `json:"date_to"`
	Scope    Scope    `json:"scope"`
	Totals   Totals   `json:"totals"`
	Days     []Day    `json:"days"`
	Branches []Branch `json:"branches"`
}
