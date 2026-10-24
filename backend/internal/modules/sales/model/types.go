// Package model holds the API shapes of the sales module: sale lines
// (revenue), ledger entries (what a customer was charged, paid and paid
// back), balances and the daily figures that keep revenue, collections,
// transfers and staff payouts apart.
package model

import (
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Engine slugs (activity, live lists).
const (
	LinesSlug   = "tenant.sale_lines"
	EntriesSlug = "tenant.ledger_entries"
)

// Sale line kinds.
const (
	LineSale     = "sale"
	LineReversal = "reversal"
)

// LineKinds lists the sale line kinds.
var LineKinds = []string{LineSale, LineReversal}

// Item types of a sale line.
const (
	ItemService = "service"
	ItemProduct = "product"
)

// ItemTypes lists the item types.
var ItemTypes = []string{ItemService, ItemProduct}

// Sources of a sale line.
const (
	SourceReservation   = "reservation"
	SourceStockMovement = "stock_movement"
)

// Ledger entry kinds.
const (
	EntrySale       = "sale"
	EntryCollection = "collection"
	EntryRefund     = "refund"
	EntryReversal   = "reversal"
)

// EntryKinds lists the ledger entry kinds.
var EntryKinds = []string{EntrySale, EntryCollection, EntryRefund, EntryReversal}

// Directions of a ledger entry.
const (
	Debit  = "debit"
	Credit = "credit"
)

// Payment methods of a collection or refund.
const (
	MethodCash     = "cash"
	MethodCard     = "card"
	MethodTransfer = "transfer"
	MethodOnline   = "online"
	MethodOther    = "other"
)

// Methods lists the payment methods.
var Methods = []string{MethodCash, MethodCard, MethodTransfer, MethodOnline, MethodOther}

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

// SaleLine is one service delivered or product handed over (kind sale),
// or the reversal of one (kind reversal, negative amounts).
type SaleLine struct {
	UUID           uuid.UUID       `json:"uuid"`
	Kind           string          `json:"kind"`
	ItemType       string          `json:"item_type"`
	SourceType     string          `json:"source_type"`
	SourceUUID     *uuid.UUID      `json:"source_uuid"`
	Branch         Ref             `json:"branch"`
	Customer       *Ref            `json:"customer"`
	Reservation    *ReservationRef `json:"reservation"`
	LineIndex      *int            `json:"line_index"`
	ServiceUUID    *uuid.UUID      `json:"service_uuid"`
	ProductUUID    *uuid.UUID      `json:"product_uuid"`
	VariantUUID    *uuid.UUID      `json:"variant_uuid"`
	Staff          *Ref            `json:"staff"`
	Description    string          `json:"description"`
	Quantity       int             `json:"quantity"`
	UnitPrice      money.Money     `json:"unit_price"`
	GrossAmount    money.Money     `json:"gross_amount"`
	DiscountAmount money.Money     `json:"discount_amount"`
	NetAmount      money.Money     `json:"net_amount"`
	Currency       string          `json:"currency"`
	SaleDate       string          `json:"sale_date"`
	Generation     int             `json:"generation"`
	ReversesUUID   *uuid.UUID      `json:"reverses_uuid"`
	ReversedByUUID *uuid.UUID      `json:"reversed_by_uuid"`
	ReplacesUUID   *uuid.UUID      `json:"replaces_uuid"`
	ReplacedByUUID *uuid.UUID      `json:"replaced_by_uuid"`
	Reason         string          `json:"reason"`
	CreatedBy      *UserRef        `json:"created_by"`
	CreatedAt      time.Time       `json:"created_at"`
}

// LineFilter is the sale line list query.
type LineFilter struct {
	Branches        []uuid.UUID
	Kinds           []string
	ItemTypes       []string
	ReservationUUID *uuid.UUID
	CustomerUUID    *uuid.UUID
	Staff           []uuid.UUID
	DateFrom        *time.Time
	DateTo          *time.Time
	Q               string
	SortField       string
	SortDesc        bool
	Limit, Offset   int32
}

// ReverseInput is the body of a reversal.
type ReverseInput struct {
	Reason string `json:"reason"`
}

// CorrectInput is the body of a sale line correction. Fields left out keep
// the value of the corrected line.
type CorrectInput struct {
	Quantity       *int         `json:"quantity"`
	UnitPrice      *money.Money `json:"unit_price"`
	DiscountAmount *money.Money `json:"discount_amount"`
	Reason         string       `json:"reason"`
}

// Correction is what a correction wrote: the reversal of the old line and
// the line that replaces it.
type Correction struct {
	Reversal    SaleLine `json:"reversal"`
	Replacement SaleLine `json:"replacement"`
}

// LedgerEntry is one row of the customer ledger.
type LedgerEntry struct {
	UUID uuid.UUID `json:"uuid"`
	Kind string    `json:"kind"`
	// EffectiveKind is the kind a reversal undoes (the kind itself otherwise).
	EffectiveKind    string          `json:"effective_kind"`
	Direction        string          `json:"direction"`
	Amount           money.Money     `json:"amount"`
	Currency         string          `json:"currency"`
	EntryDate        string          `json:"entry_date"`
	Method           string          `json:"method"`
	Branch           Ref             `json:"branch"`
	Customer         *Ref            `json:"customer"`
	Reservation      *ReservationRef `json:"reservation"`
	SaleLineUUID     *uuid.UUID      `json:"sale_line_uuid"`
	CashMovementUUID *uuid.UUID      `json:"cash_movement_uuid"`
	CashAccount      *Ref            `json:"cash_account"`
	RefundsUUID      *uuid.UUID      `json:"refunds_uuid"`
	ReversesUUID     *uuid.UUID      `json:"reverses_uuid"`
	ReversedByUUID   *uuid.UUID      `json:"reversed_by_uuid"`
	SourceType       string          `json:"source_type"`
	SourceUUID       *uuid.UUID      `json:"source_uuid"`
	Description      string          `json:"description"`
	CreatedBy        *UserRef        `json:"created_by"`
	CreatedAt        time.Time       `json:"created_at"`
}

// EntryFilter is the ledger list query.
type EntryFilter struct {
	Branches        []uuid.UUID
	Kinds           []string
	ReservationUUID *uuid.UUID
	CustomerUUID    *uuid.UUID
	DateFrom        *time.Time
	DateTo          *time.Time
	Q               string
	SortField       string
	SortDesc        bool
	Limit, Offset   int32
}

// Totals are the ledger figures of one currency. Paid = collected -
// refunded; Balance = charged - paid (positive: the customer owes,
// negative: money held for the customer).
type Totals struct {
	Currency  string      `json:"currency"`
	Charged   money.Money `json:"charged"`
	Collected money.Money `json:"collected"`
	Refunded  money.Money `json:"refunded"`
	Paid      money.Money `json:"paid"`
	Balance   money.Money `json:"balance"`
}

// ReservationBalance is what a visit costs and what was paid for it. Until
// the visit is completed (no sale line yet) the expected amount is the
// booked total of a standing booking (0 once cancelled, rejected or not
// attended); afterwards it is the net of its live sale lines.
type ReservationBalance struct {
	Reservation ReservationRef `json:"reservation"`
	Status      string         `json:"status"`
	Currency    string         `json:"currency"`
	BookedTotal money.Money    `json:"booked_total"`
	SalesPosted bool           `json:"sales_posted"`
	Expected    money.Money    `json:"expected"`
	Collected   money.Money    `json:"collected"`
	Refunded    money.Money    `json:"refunded"`
	Paid        money.Money    `json:"paid"`
	// Remaining is what the customer still owes; Credit what was paid
	// beyond the expected amount (to refund or to keep).
	Remaining money.Money   `json:"remaining"`
	Credit    money.Money   `json:"credit"`
	Lines     []SaleLine    `json:"lines"`
	Entries   []LedgerEntry `json:"entries"`
}

// CustomerBalance is the ledger of one customer per currency.
type CustomerBalance struct {
	Customer uuid.UUID `json:"customer_uuid"`
	Totals   []Totals  `json:"totals"`
}

// Revenue are the sale line figures (ciro).
type Revenue struct {
	Gross      money.Money `json:"gross"`
	Discount   money.Money `json:"discount"`
	Reversed   money.Money `json:"reversed"`
	Net        money.Money `json:"net"`
	ServiceNet money.Money `json:"service_net"`
	ProductNet money.Money `json:"product_net"`
	SaleCount  int64       `json:"sale_count"`
}

// Collections are the money taken and paid back (tahsilat).
type Collections struct {
	Collected money.Money `json:"collected"`
	Refunded  money.Money `json:"refunded"`
	Reversed  money.Money `json:"reversed"`
	Net       money.Money `json:"net"`
	Count     int64       `json:"count"`
}

// Figures are the separate figures of one currency (A20): revenue,
// collections, transfers between accounts, staff payouts, and the income
// and expenses entered outside sales (TEC-915). None of them is derived
// from another; staff payouts are not expenses (A17).
type Figures struct {
	Currency      string      `json:"currency"`
	Revenue       Revenue     `json:"revenue"`
	Collections   Collections `json:"collections"`
	Transfers     money.Money `json:"transfers"`
	StaffPayments money.Money `json:"staff_payments"`
	Income        money.Money `json:"income"`
	Expenses      money.Money `json:"expenses"`
}

// Day are the figures of one business day.
type Day struct {
	Date    string    `json:"date"`
	Figures []Figures `json:"figures"`
}

// Summary are the figures of a date range: totals per currency and one
// entry per day that has any.
type Summary struct {
	DateFrom string    `json:"date_from"`
	DateTo   string    `json:"date_to"`
	Totals   []Figures `json:"totals"`
	Days     []Day     `json:"days"`
}

// CollectionInput records money taken (TEC-912 posts it): for a
// reservation (its branch and customer), or for a customer / counter sale
// in a branch. The money goes into the cash account in the same
// transaction. A posting with an IdempotencyKey that was written before
// returns the first entry.
type CollectionInput struct {
	ReservationUUID *uuid.UUID
	CustomerUUID    *uuid.UUID
	BranchUUID      *uuid.UUID
	CashAccountUUID uuid.UUID
	Amount          money.Money
	Method          string
	Date            time.Time // zero = today
	Description     string
	SourceType      string
	SourceUUID      *uuid.UUID
	IdempotencyKey  string
	CreatedBy       *int64
}

// RefundInput pays back (part of) a collection (TEC-913 posts it), out of
// a cash account in the same transaction.
type RefundInput struct {
	CollectionUUID  uuid.UUID
	CashAccountUUID uuid.UUID
	Amount          money.Money
	Method          string
	Date            time.Time // zero = today
	Description     string
	SourceType      string
	SourceUUID      *uuid.UUID
	IdempotencyKey  string
	CreatedBy       *int64
}
