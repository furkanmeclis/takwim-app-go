// Package model holds the API shapes of the payments module (TEC-912):
// online payment attempts, the start of an online payment, a collection at
// the business and the payment view of a reservation.
package model

import (
	"time"

	salesmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Payment attempt statuses.
const (
	StatusPending   = "pending"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
)

// SourcePayment names online payments on the ledger entries they write;
// SourceReservation names collections taken at the business.
const (
	SourcePayment     = "payment"
	SourceReservation = "reservation"
)

// InboundKind is the integration job kind of a provider notification.
const InboundKind = "payment.webhook"

// OnSiteMethods are the ways money is taken at the business.
var OnSiteMethods = []string{salesmodel.MethodCash, salesmodel.MethodCard, salesmodel.MethodTransfer}

// Ref names a related record.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// Attempt is one online payment attempt of a reservation.
type Attempt struct {
	UUID          uuid.UUID    `json:"uuid"`
	Provider      string       `json:"provider"`
	Amount        money.Money  `json:"amount"`
	PaidAmount    *money.Money `json:"paid_amount"`
	Currency      string       `json:"currency"`
	Status        string       `json:"status"`
	Outcome       string       `json:"outcome"`
	FailureReason string       `json:"failure_reason"`
	CashAccount   Ref          `json:"cash_account"`
	ExpiresAt     time.Time    `json:"expires_at"`
	CompletedAt   *time.Time   `json:"completed_at"`
	CreatedAt     time.Time    `json:"created_at"`
}

// CollectAccount is a till or bank account a collection may go into.
type CollectAccount struct {
	UUID     uuid.UUID `json:"uuid"`
	Name     string    `json:"name"`
	Kind     string    `json:"kind"`
	Currency string    `json:"currency"`
	// Branch is false for a business-wide account.
	Branch bool `json:"branch"`
}

// ReservationPayments is the payment view of a reservation in the panel:
// the amounts from the ledger, the money entries (collections, refunds and
// their reversals), the online attempts and, for a member who may collect,
// the accounts a collection may go into.
type ReservationPayments struct {
	Reservation   salesmodel.ReservationRef `json:"reservation"`
	Status        string                    `json:"status"`
	PaymentStatus string                    `json:"payment_status"`
	Currency      string                    `json:"currency"`
	Expected      money.Money               `json:"expected"`
	Paid          money.Money               `json:"paid"`
	Remaining     money.Money               `json:"remaining"`
	Credit        money.Money               `json:"credit"`
	Entries       []salesmodel.LedgerEntry  `json:"entries"`
	Online        []Attempt                 `json:"online"`
	// CanCollect: the caller may take money for this visit now.
	CanCollect      bool             `json:"can_collect"`
	CollectAccounts []CollectAccount `json:"collect_accounts"`
}

// CollectInput is money taken at the business for a reservation.
type CollectInput struct {
	Amount          money.Money `json:"amount"`
	Method          string      `json:"method"`
	CashAccountUUID uuid.UUID   `json:"cash_account_uuid"`
	Note            string      `json:"note"`
}

// OnlineStart is the hosted payment page of a reservation.
type OnlineStart struct {
	PaymentUUID uuid.UUID   `json:"payment_uuid"`
	RedirectURL string      `json:"redirect_url"`
	Amount      money.Money `json:"amount"`
	Currency    string      `json:"currency"`
	ExpiresAt   time.Time   `json:"expires_at"`
}

// NotifyResult is what a provider notification did.
type NotifyResult struct {
	Status    string `json:"status"`
	Duplicate bool   `json:"duplicate"`
}

// TestPage is what the development provider's test page shows.
type TestPage struct {
	Business          string      `json:"business"`
	Slug              string      `json:"slug"`
	ReservationUUID   uuid.UUID   `json:"reservation_uuid"`
	ReservationNumber int64       `json:"reservation_number"`
	Amount            money.Money `json:"amount"`
	Currency          string      `json:"currency"`
	Status            string      `json:"status"`
	ExpiresAt         time.Time   `json:"expires_at"`
}
