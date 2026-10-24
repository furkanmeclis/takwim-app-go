package model

import (
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Engine slugs of the reconciliation (activity, io, live lists).
const (
	StatementsSlug     = "tenant.cash_statements"
	StatementLinesSlug = "tenant.cash_statement_lines"
)

// StatementLineSourceType marks movements posted from a statement line.
const StatementLineSourceType = "cash_statement_line"

// Statement sources.
const (
	SourceBank     = "bank"
	SourceProvider = "provider"
)

// StatementSources lists the statement sources.
var StatementSources = []string{SourceBank, SourceProvider}

// Statement statuses.
const (
	StatementOpen   = "open"
	StatementClosed = "closed"
)

// Line types.
const (
	LineTypePayment = "payment"
	LineTypeRefund  = "refund"
	LineTypeFee     = "fee"
	LineTypePayout  = "payout"
	LineTypeOther   = "other"
)

// LineTypes lists the line types.
var LineTypes = []string{LineTypePayment, LineTypeRefund, LineTypeFee, LineTypePayout, LineTypeOther}

// Line reconciliation statuses.
const (
	LinePending    = "pending"
	LineMatched    = "matched"
	LineDifference = "difference"
	LinePosted     = "posted"
	LineIgnored    = "ignored"
)

// LineStatuses lists the line statuses.
var LineStatuses = []string{LinePending, LineMatched, LineDifference, LinePosted, LineIgnored}

// Match methods.
const (
	MatchAuto   = "auto"
	MatchManual = "manual"
)

// Statement is one imported bank statement or provider settlement report.
type Statement struct {
	UUID           uuid.UUID    `json:"uuid"`
	Account        AccountRef   `json:"account"`
	Branch         *BranchRef   `json:"branch"`
	Source         string       `json:"source"`
	Name           string       `json:"name"`
	Currency       string       `json:"currency"`
	PeriodFrom     *string      `json:"period_from"`
	PeriodTo       *string      `json:"period_to"`
	ClosingBalance *money.Money `json:"closing_balance"`
	Note           string       `json:"note"`
	Status         string       `json:"status"`
	ClosingUUID    *uuid.UUID   `json:"closing_uuid"`
	LineCount      int64        `json:"line_count"`
	OpenCount      int64        `json:"open_count"`
	CreatedBy      *UserRef     `json:"created_by"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// StatementInput is the create / patch body. The account and source are
// fixed after creation.
type StatementInput struct {
	AccountUUID    *uuid.UUID   `json:"account_uuid"`
	Source         *string      `json:"source"`
	Name           *string      `json:"name"`
	PeriodFrom     *string      `json:"period_from"`
	PeriodTo       *string      `json:"period_to"`
	ClosingBalance *money.Money `json:"closing_balance"`
	Note           *string      `json:"note"`
}

// StatementFilter selects statements of one organization.
type StatementFilter struct {
	Limit, Offset int32
	Q             string
	Accounts      []uuid.UUID
	Sources       []string
	Statuses      []string
	SortField     string
	SortDesc      bool
}

// StatementSummary is the reconciliation state of one statement: line
// counts per status, sums, and the book balance against the balance the
// document states.
type StatementSummary struct {
	PendingCount    int64       `json:"pending_count"`
	MatchedCount    int64       `json:"matched_count"`
	DifferenceCount int64       `json:"difference_count"`
	PostedCount     int64       `json:"posted_count"`
	IgnoredCount    int64       `json:"ignored_count"`
	TotalIn         money.Money `json:"total_in"`
	TotalOut        money.Money `json:"total_out"`
	TotalFee        money.Money `json:"total_fee"`
	PendingNet      money.Money `json:"pending_net"`
	DifferenceTotal money.Money `json:"difference_total"`
	FirstDate       *string     `json:"first_date"`
	LastDate        *string     `json:"last_date"`
	// BalanceDate is the day the book balance is read on (period end, else
	// the last line date).
	BalanceDate    string       `json:"balance_date"`
	BookBalance    money.Money  `json:"book_balance"`
	ClosingBalance *money.Money `json:"closing_balance"`
	// BalanceDifference = closing balance - book balance (nil when the
	// document states no balance).
	BalanceDifference *money.Money `json:"balance_difference"`
	Reconciled        bool         `json:"reconciled"`
}

// StatementRef names the statement of a line.
type StatementRef struct {
	UUID   uuid.UUID `json:"uuid"`
	Status string    `json:"status"`
}

// MovementRef names a movement linked to a line.
type MovementRef struct {
	UUID          uuid.UUID    `json:"uuid"`
	Kind          string       `json:"kind"`
	MovementDate  string       `json:"movement_date,omitempty"`
	Amount        *money.Money `json:"amount,omitempty"`
	Description   string       `json:"description,omitempty"`
	TransferGroup *uuid.UUID   `json:"transfer_group,omitempty"`
}

// StatementLine is one row of a statement with its reconciliation state.
type StatementLine struct {
	UUID             uuid.UUID    `json:"uuid"`
	Statement        StatementRef `json:"statement"`
	Account          AccountRef   `json:"account"`
	LineNo           int32        `json:"line_no"`
	LineDate         string       `json:"line_date"`
	Direction        string       `json:"direction"`
	Amount           money.Money  `json:"amount"`
	Fee              money.Money  `json:"fee"`
	LineType         string       `json:"line_type"`
	Description      string       `json:"description"`
	Reference        string       `json:"reference"`
	BalanceAfter     *money.Money `json:"balance_after"`
	Currency         string       `json:"currency"`
	Status           string       `json:"status"`
	MatchMethod      string       `json:"match_method"`
	MatchedMovement  *MovementRef `json:"matched_movement"`
	DifferenceAmount money.Money  `json:"difference_amount"`
	PostedMovement   *MovementRef `json:"posted_movement"`
	FeeMovementUUID  *uuid.UUID   `json:"fee_movement_uuid"`
	PostCount        int32        `json:"post_count"`
	Note             string       `json:"note"`
	ResolvedBy       *UserRef     `json:"resolved_by"`
	ResolvedAt       *time.Time   `json:"resolved_at"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
}

// LineInput adds one line to a statement (manual add and import).
type LineInput struct {
	LineDate     string       `json:"line_date"`
	Direction    string       `json:"direction"`
	Amount       money.Money  `json:"amount"`
	Fee          *money.Money `json:"fee"`
	LineType     string       `json:"line_type"`
	Description  string       `json:"description"`
	Reference    string       `json:"reference"`
	BalanceAfter *money.Money `json:"balance_after"`
}

// LineFilter selects statement lines of one organization.
type LineFilter struct {
	Limit, Offset int32
	Q             string
	Statements    []uuid.UUID
	Statuses      []string
	LineTypes     []string
	Direction     string
	SortField     string
	SortDesc      bool
}

// LineFacets are the dynamic filter options of the line list of a statement.
type LineFacets struct {
	Status []Facet `json:"status"`
}

// MatchInput links a line to an existing movement of its account.
type MatchInput struct {
	MovementUUID uuid.UUID `json:"movement_uuid"`
	Note         string    `json:"note"`
}

// PostLineInput writes a line into the book. Kind is a module posting kind
// matching the line direction (in: income / collection, out: expense /
// refund / staff_payment; empty = income / expense). A CounterAccountUUID
// (or a payout line) posts a transfer between the statement account and
// the counter account instead.
type PostLineInput struct {
	Kind               string     `json:"kind"`
	CounterAccountUUID *uuid.UUID `json:"counter_account_uuid"`
	Description        string     `json:"description"`
	Note               string     `json:"note"`
}

// MatchCandidate is a movement a line may be matched with.
type MatchCandidate struct {
	UUID           uuid.UUID   `json:"uuid"`
	Kind           string      `json:"kind"`
	Direction      string      `json:"direction"`
	Amount         money.Money `json:"amount"`
	Currency       string      `json:"currency"`
	MovementDate   string      `json:"movement_date"`
	Description    string      `json:"description"`
	SourceType     string      `json:"source_type"`
	CounterAccount string      `json:"counter_account"`
	AmountDelta    money.Money `json:"amount_delta"`
	DayDelta       int32       `json:"day_delta"`
	Exact          bool        `json:"exact"`
}

// AutoMatchResult counts the lines an auto-match pass decided.
type AutoMatchResult struct {
	Checked int `json:"checked"`
	Matched int `json:"matched"`
	Pending int `json:"pending"`
}
