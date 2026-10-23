// Package model holds the API shapes of the cash module: cash / bank /
// online accounts, their movements and the daily closings of a till.
package model

import (
	"encoding/json"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Engine slugs (activity, io, bulk, live lists).
const (
	AccountsSlug  = "tenant.cash_accounts"
	MovementsSlug = "tenant.cash_movements"
)

// Account kinds.
const (
	KindCash   = "cash"
	KindBank   = "bank"
	KindOnline = "online"
)

// Kinds lists the account kinds.
var Kinds = []string{KindCash, KindBank, KindOnline}

// Account statuses.
const (
	StatusActive   = "active"
	StatusArchived = "archived"
)

// Statuses lists the account statuses.
var Statuses = []string{StatusActive, StatusArchived}

// Movement kinds. Deposit / withdrawal are the manual panel movements; the
// kinds after Reversal are posted by other modules through Service.Post.
const (
	MovementDeposit           = "deposit"
	MovementWithdrawal        = "withdrawal"
	MovementTransferIn        = "transfer_in"
	MovementTransferOut       = "transfer_out"
	MovementClosingDifference = "closing_difference"
	MovementReversal          = "reversal"
	MovementCollection        = "collection"
	MovementRefund            = "refund"
	MovementIncome            = "income"
	MovementExpense           = "expense"
	MovementStaffPayment      = "staff_payment"
)

// MovementKinds lists every movement kind.
var MovementKinds = []string{
	MovementDeposit, MovementWithdrawal, MovementTransferIn, MovementTransferOut, MovementClosingDifference,
	MovementReversal, MovementCollection, MovementRefund, MovementIncome, MovementExpense, MovementStaffPayment,
}

// Directions.
const (
	DirectionIn  = "in"
	DirectionOut = "out"
)

// HeadOffice is the branch filter value of business-wide accounts.
const HeadOffice = "head_office"

// BranchRef names the branch of an account (nil = business-wide).
type BranchRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	Code string    `json:"code"`
}

// Account is the API projection of a cash / bank / online account.
// Balance = opening balance + money in - money out.
type Account struct {
	UUID           uuid.UUID   `json:"uuid"`
	Branch         *BranchRef  `json:"branch"`
	Kind           string      `json:"kind"`
	Name           string      `json:"name"`
	Currency       string      `json:"currency"`
	OpeningBalance money.Money `json:"opening_balance"`
	OpeningDate    string      `json:"opening_date"`
	Balance        money.Money `json:"balance"`
	BankName       string      `json:"bank_name"`
	IBAN           string      `json:"iban"`
	Provider       string      `json:"provider"`
	Note           string      `json:"note"`
	Status         string      `json:"status"`
	SortOrder      int32       `json:"sort_order"`
	// LastClosedDate is the latest closed day (tills); movements dated on
	// or before it are refused.
	LastClosedDate *string `json:"last_closed_date"`
	// HasActivity (detail only) is true once the account has a movement or
	// a closing; the opening balance and date are fixed from then on.
	HasActivity bool      `json:"has_activity"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AccountInput is the create / patch body. Branch, kind and currency are
// fixed after creation; the opening balance and date until the first
// movement.
type AccountInput struct {
	BranchUUID     *uuid.UUID   `json:"branch_uuid"`
	Kind           *string      `json:"kind"`
	Name           *string      `json:"name"`
	OpeningBalance *money.Money `json:"opening_balance"`
	OpeningDate    *string      `json:"opening_date"`
	BankName       *string      `json:"bank_name"`
	IBAN           *string      `json:"iban"`
	Provider       *string      `json:"provider"`
	Note           *string      `json:"note"`
	Status         *string      `json:"status"`
	SortOrder      *int32       `json:"sort_order"`
}

// AccountFilter selects accounts of one organization.
type AccountFilter struct {
	Limit, Offset int32
	Q             string
	Branches      []uuid.UUID
	HeadOffice    bool
	Kinds         []string
	Statuses      []string
	SortField     string
	SortDesc      bool
}

// Facet is one option of a faceted filter with its row count.
type Facet struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
	Count int64  `json:"count"`
}

// AccountFacets are the dynamic filter options of the account list.
type AccountFacets struct {
	Branch []Facet `json:"branch"`
}

// AccountOption is an account in a picker.
type AccountOption struct {
	UUID       uuid.UUID `json:"uuid"`
	Name       string    `json:"name"`
	Kind       string    `json:"kind"`
	Status     string    `json:"status"`
	Currency   string    `json:"currency"`
	BranchName string    `json:"branch_name"`
}

// AccountRef names an account on a movement.
type AccountRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	Kind string    `json:"kind,omitempty"`
}

// UserRef names the user behind a movement or closing.
type UserRef struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
}

// Movement is one row of the cash book.
type Movement struct {
	UUID           uuid.UUID   `json:"uuid"`
	Account        AccountRef  `json:"account"`
	Kind           string      `json:"kind"`
	Direction      string      `json:"direction"`
	Amount         money.Money `json:"amount"`
	Currency       string      `json:"currency"`
	MovementDate   string      `json:"movement_date"`
	Description    string      `json:"description"`
	CounterAccount *AccountRef `json:"counter_account"`
	TransferGroup  *uuid.UUID  `json:"transfer_group"`
	ReversesUUID   *uuid.UUID  `json:"reverses_uuid"`
	ReversedByUUID *uuid.UUID  `json:"reversed_by_uuid"`
	ClosingUUID    *uuid.UUID  `json:"closing_uuid"`
	SourceType     string      `json:"source_type"`
	SourceUUID     *uuid.UUID  `json:"source_uuid"`
	CreatedBy      *UserRef    `json:"created_by"`
	// BalanceAfter is the account balance after the movement (account
	// movement list only).
	BalanceAfter *money.Money `json:"balance_after,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
}

// MovementInput is a manual money in / out of one account.
type MovementInput struct {
	Direction    string      `json:"direction"`
	Amount       money.Money `json:"amount"`
	MovementDate *string     `json:"movement_date"`
	Description  string      `json:"description"`
}

// TransferInput moves money between two accounts of the organization.
type TransferInput struct {
	FromAccountUUID uuid.UUID   `json:"from_account_uuid"`
	ToAccountUUID   uuid.UUID   `json:"to_account_uuid"`
	Amount          money.Money `json:"amount"`
	MovementDate    *string     `json:"movement_date"`
	Description     string      `json:"description"`
}

// Transfer is the answer of a transfer: both legs.
type Transfer struct {
	TransferGroup uuid.UUID `json:"transfer_group"`
	Out           Movement  `json:"out"`
	In            Movement  `json:"in"`
}

// ReverseInput reverses a movement (both legs of a transfer).
type ReverseInput struct {
	Reason string `json:"reason"`
}

// MovementFilter selects movements of one organization.
type MovementFilter struct {
	Limit, Offset int32
	Q             string
	Accounts      []uuid.UUID
	Kinds         []string
	Direction     string
	DateFrom      *time.Time
	DateTo        *time.Time
	SortField     string
	SortDesc      bool
}

// MovementFacets are the filter options of the movement list.
type MovementFacets struct {
	Kind    []Facet `json:"kind"`
	Account []Facet `json:"account"`
}

// Closing is the daily closing of a till.
type Closing struct {
	UUID         uuid.UUID   `json:"uuid"`
	BusinessDate string      `json:"business_date"`
	Currency     string      `json:"currency"`
	Opening      money.Money `json:"opening_amount"`
	TotalIn      money.Money `json:"total_in"`
	TotalOut     money.Money `json:"total_out"`
	Expected     money.Money `json:"expected_amount"`
	Counted      money.Money `json:"counted_amount"`
	Difference   money.Money `json:"difference"`
	Note         string      `json:"note"`
	Status       string      `json:"status"`
	ClosedBy     *UserRef    `json:"closed_by"`
	ClosedAt     time.Time   `json:"closed_at"`
	ReopenedBy   *UserRef    `json:"reopened_by"`
	ReopenedAt   *time.Time  `json:"reopened_at"`
	ReopenReason string      `json:"reopen_reason"`
}

// ClosingPreview is the expected state of a day before it is closed.
type ClosingPreview struct {
	BusinessDate   string      `json:"business_date"`
	Currency       string      `json:"currency"`
	Opening        money.Money `json:"opening_amount"`
	TotalIn        money.Money `json:"total_in"`
	TotalOut       money.Money `json:"total_out"`
	Expected       money.Money `json:"expected_amount"`
	MovementCount  int64       `json:"movement_count"`
	Closed         bool        `json:"closed"`
	LastClosedDate *string     `json:"last_closed_date"`
}

// ClosingInput closes a day with the counted amount.
type ClosingInput struct {
	BusinessDate *string      `json:"business_date"`
	Counted      *money.Money `json:"counted_amount"`
	Note         string       `json:"note"`
}

// ReopenInput reopens the latest closed day.
type ReopenInput struct {
	Reason string `json:"reason"`
}

// DailyAccount is one account of the daily summary.
type DailyAccount struct {
	Account     AccountRef  `json:"account"`
	Branch      *BranchRef  `json:"branch"`
	Currency    string      `json:"currency"`
	Status      string      `json:"status"`
	Opening     money.Money `json:"opening_amount"`
	MoneyIn     money.Money `json:"money_in"`
	MoneyOut    money.Money `json:"money_out"`
	TransferIn  money.Money `json:"transfer_in"`
	TransferOut money.Money `json:"transfer_out"`
	Difference  money.Money `json:"difference"`
	Closing     money.Money `json:"closing_amount"`
	Closed      bool        `json:"closed"`
	ClosingUUID *uuid.UUID  `json:"closing_uuid"`
	ClosedAt    *time.Time  `json:"closed_at"`
}

// DailyTotals sums the daily summary per currency.
type DailyTotals struct {
	Currency    string      `json:"currency"`
	Opening     money.Money `json:"opening_amount"`
	MoneyIn     money.Money `json:"money_in"`
	MoneyOut    money.Money `json:"money_out"`
	TransferIn  money.Money `json:"transfer_in"`
	TransferOut money.Money `json:"transfer_out"`
	Difference  money.Money `json:"difference"`
	Closing     money.Money `json:"closing_amount"`
}

// DailySummary is the günlük kapanış özeti of one day.
type DailySummary struct {
	Date     string         `json:"date"`
	Accounts []DailyAccount `json:"accounts"`
	Totals   []DailyTotals  `json:"totals"`
}

// ActivityActor is the user behind an activity entry.
type ActivityActor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// ActivityEntry is one audit event of an account.
type ActivityEntry struct {
	UUID      uuid.UUID       `json:"uuid"`
	Action    string          `json:"action"`
	Payload   json.RawMessage `json:"payload"`
	Actor     *ActivityActor  `json:"actor"`
	CreatedAt time.Time       `json:"created_at"`
}
