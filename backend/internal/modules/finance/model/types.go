// Package model holds the API shapes of the income / expense module
// (TEC-915): categories, the business's own records and the combined list
// in which sale lines appear as automatic income.
package model

import (
	"encoding/json"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Engine slugs (activity, live lists, export).
const (
	CategoriesSlug = "tenant.finance_categories"
	EntriesSlug    = "tenant.finance_entries"
)

// Kinds of a category and a record.
const (
	KindIncome  = "income"
	KindExpense = "expense"
)

// Kinds lists the kinds.
var Kinds = []string{KindIncome, KindExpense}

// Category statuses.
const (
	StatusActive   = "active"
	StatusArchived = "archived"
)

// Statuses lists the category statuses.
var Statuses = []string{StatusActive, StatusArchived}

// Record types: the record itself (or the replacement of a corrected one)
// and the exact negative of one record.
const (
	TypeEntry    = "entry"
	TypeReversal = "reversal"
)

// Sources of a list row: a record of the business, or a sale line
// (automatic income).
const (
	SourceManual = "manual"
	SourceSale   = "sale"
)

// Sources lists the list row sources.
var Sources = []string{SourceManual, SourceSale}

// Sale item types; the category filter takes them as "sale_service" /
// "sale_product".
const (
	ItemService = "service"
	ItemProduct = "product"
)

// Category filter values of the sale rows.
const (
	SaleServiceCategory = "sale_service"
	SaleProductCategory = "sale_product"
)

// HeadOffice is the branch filter value of business-wide records.
const HeadOffice = "head_office"

// Ref names a related record.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// AccountRef names the paying / receiving cash account.
type AccountRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	Kind string    `json:"kind,omitempty"`
}

// UserRef names the user who posted a record.
type UserRef struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
}

// Category is an income or expense category.
type Category struct {
	UUID       uuid.UUID `json:"uuid"`
	Kind       string    `json:"kind"`
	Code       *string   `json:"code"`
	Name       string    `json:"name"`
	IsSystem   bool      `json:"is_system"`
	Status     string    `json:"status"`
	SortOrder  int32     `json:"sort_order"`
	EntryCount int64     `json:"entry_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CategoryInput creates or patches a category. Kind is fixed after create.
type CategoryInput struct {
	Kind      *string `json:"kind"`
	Name      *string `json:"name"`
	Status    *string `json:"status"`
	SortOrder *int32  `json:"sort_order"`
}

// CategoryFilter narrows the category list.
type CategoryFilter struct {
	Kinds     []string
	Statuses  []string
	Q         string
	SortField string
	SortDesc  bool
	Limit     int32
	Offset    int32
}

// Document is the file attached to a record (receipt, invoice). URL is
// the API path that streams it.
type Document struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	URL         string `json:"url"`
}

// Entry is one income / expense record with its links: the reversal of it
// (or the record it reverses) and the replacement of a corrected record.
type Entry struct {
	UUID             uuid.UUID   `json:"uuid"`
	EntryType        string      `json:"entry_type"`
	Kind             string      `json:"kind"`
	Category         Ref         `json:"category"`
	Branch           *Ref        `json:"branch"`
	Staff            *Ref        `json:"staff"`
	Account          AccountRef  `json:"account"`
	CashMovementUUID uuid.UUID   `json:"cash_movement_uuid"`
	Amount           money.Money `json:"amount"`
	Currency         string      `json:"currency"`
	EntryDate        string      `json:"entry_date"`
	Description      string      `json:"description"`
	Reason           string      `json:"reason"`
	Document         *Document   `json:"document"`
	ReversesUUID     *uuid.UUID  `json:"reverses_uuid"`
	ReversedByUUID   *uuid.UUID  `json:"reversed_by_uuid"`
	ReplacesUUID     *uuid.UUID  `json:"replaces_uuid"`
	ReplacedByUUID   *uuid.UUID  `json:"replaced_by_uuid"`
	CreatedBy        *UserRef    `json:"created_by"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
}

// EntryInput records money in or out outside sales. The amount leaves (or
// enters) the cash account in the same transaction.
type EntryInput struct {
	Kind            string      `json:"kind"`
	CategoryUUID    uuid.UUID   `json:"category_uuid"`
	CashAccountUUID uuid.UUID   `json:"cash_account_uuid"`
	BranchUUID      *uuid.UUID  `json:"branch_uuid"`
	StaffUUID       *uuid.UUID  `json:"staff_uuid"`
	Amount          money.Money `json:"amount"`
	EntryDate       *string     `json:"entry_date"`
	Description     string      `json:"description"`
	IdempotencyKey  string      `json:"idempotency_key"`
}

// EntryPatch changes the classification of a live record; money, date,
// account and branch change through a correction only.
type EntryPatch struct {
	CategoryUUID *uuid.UUID   `json:"category_uuid"`
	Description  *string      `json:"description"`
	StaffUUID    OptionalUUID `json:"staff_uuid"`
}

// CorrectInput replaces a record: the old one is reversed and a new one
// written with the given values (missing ones are copied).
type CorrectInput struct {
	CategoryUUID    *uuid.UUID   `json:"category_uuid"`
	CashAccountUUID *uuid.UUID   `json:"cash_account_uuid"`
	BranchUUID      OptionalUUID `json:"branch_uuid"`
	StaffUUID       OptionalUUID `json:"staff_uuid"`
	Amount          *money.Money `json:"amount"`
	EntryDate       *string      `json:"entry_date"`
	Description     *string      `json:"description"`
	Reason          string       `json:"reason"`
}

// ReverseInput carries the written reason of a reversal.
type ReverseInput struct {
	Reason string `json:"reason"`
}

// OptionalUUID distinguishes an absent uuid field from null.
type OptionalUUID struct {
	Set   bool
	Value *uuid.UUID
}

// UnmarshalJSON marks the field as sent; null clears it.
func (o *OptionalUUID) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	var v uuid.UUID
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// LedgerRow is one row of the income / expense list: a record of the
// business (source manual) or a sale line (source sale, automatic income).
type LedgerRow struct {
	Source          string      `json:"source"`
	UUID            uuid.UUID   `json:"uuid"`
	EntryType       string      `json:"entry_type"`
	Kind            string      `json:"kind"`
	EntryDate       string      `json:"entry_date"`
	Amount          money.Money `json:"amount"`
	Currency        string      `json:"currency"`
	Branch          *Ref        `json:"branch"`
	Category        *Ref        `json:"category"`
	ItemType        string      `json:"item_type,omitempty"`
	Staff           *Ref        `json:"staff"`
	Account         *Ref        `json:"account"`
	Description     string      `json:"description"`
	HasDocument     bool        `json:"has_document"`
	Reversed        bool        `json:"reversed"`
	ReservationUUID *uuid.UUID  `json:"reservation_uuid"`
	CreatedAt       time.Time   `json:"created_at"`
}

// LedgerFilter narrows the income / expense list.
type LedgerFilter struct {
	Sources    []string
	Kinds      []string
	Categories []string // category uuids, sale_service, sale_product
	Branches   []uuid.UUID
	HeadOffice bool
	Staff      []uuid.UUID
	Accounts   []uuid.UUID
	DateFrom   *time.Time
	DateTo     *time.Time
	Q          string
	SortField  string
	SortDesc   bool
	Limit      int32
	Offset     int32
}

// Facet is one filter option with its count.
type Facet struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
	Kind  string `json:"kind,omitempty"`
	Count int64  `json:"count"`
}

// Facets are the dynamic filter options of the list.
type Facets struct {
	Category []Facet `json:"category"`
	Branch   []Facet `json:"branch"`
	Staff    []Facet `json:"staff"`
}

// CategoryTotal is the total of one category in a period (reversals
// netted).
type CategoryTotal struct {
	Kind       string      `json:"kind"`
	Category   Ref         `json:"category"`
	Currency   string      `json:"currency"`
	Total      money.Money `json:"total"`
	EntryCount int64       `json:"entry_count"`
}

// SummaryTotals are the figures of one currency, kept apart (A20):
// revenue from the sale lines, the income and expenses entered here, and
// the staff payouts from the cash book. Net = revenue + income - expenses;
// staff payouts and product costs are outside it.
type SummaryTotals struct {
	Currency      string      `json:"currency"`
	Revenue       money.Money `json:"revenue"`
	Income        money.Money `json:"income"`
	Expenses      money.Money `json:"expenses"`
	StaffPayments money.Money `json:"staff_payments"`
	Net           money.Money `json:"net"`
}

// Summary are the figures of a period.
type Summary struct {
	DateFrom   string          `json:"date_from"`
	DateTo     string          `json:"date_to"`
	Totals     []SummaryTotals `json:"totals"`
	Categories []CategoryTotal `json:"categories"`
}

// ActivityActor / ActivityEntry are the audit events of a record.
type ActivityActor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// ActivityEntry is one audit event.
type ActivityEntry struct {
	UUID      uuid.UUID       `json:"uuid"`
	Action    string          `json:"action"`
	Actor     *ActivityActor  `json:"actor"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}
