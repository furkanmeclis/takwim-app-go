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
	// Refundable is what may still be paid back (the live collections less
	// their refunds and the refunds under way).
	Refundable money.Money `json:"refundable"`
	// Refunds are the refund requests of the visit, newest first.
	Refunds []Refund `json:"refunds"`
	// CanRefund: the caller may pay money back for this visit now; then
	// RefundCollections and RefundAccounts list what and from where.
	CanRefund         bool               `json:"can_refund"`
	RefundCollections []RefundCollection `json:"refund_collections"`
	RefundAccounts    []CollectAccount   `json:"refund_accounts"`
	// CanClose: the caller may close the refund to make without paying the
	// money back (TEC-978): money is held, no refund is under way, and a
	// refund is waiting or the last one failed.
	CanClose bool `json:"can_close"`
	// Kept is the last closing without a refund while the payment state is
	// kept (null otherwise).
	Kept *Kept `json:"kept"`
}

// Kept is a closing without a refund: when, why and by whom.
type Kept struct {
	At     time.Time `json:"at"`
	Reason string    `json:"reason"`
	By     *Ref      `json:"by"`
}

// CloseInput closes a refund to make without paying the money back; the
// reason is required (it goes into the visit's history).
type CloseInput struct {
	Reason string `json:"reason"`
}

// Refund statuses: requested until the money is returned and posted to the
// books (succeeded), or failed.
const (
	RefundRequested = "requested"
	RefundSucceeded = "succeeded"
	RefundFailed    = "failed"
)

// The provider's answer to a refund ("" = not asked: a manual refund).
const (
	ProviderSucceeded = "succeeded"
	ProviderFailed    = "failed"
	ProviderUncertain = "uncertain"
)

// Refund channels: the provider's refund call, or money handed back /
// refunded elsewhere and recorded by staff.
const (
	ChannelProvider = "provider"
	ChannelManual   = "manual"
)

// RefundChannels lists the channels.
var RefundChannels = []string{ChannelProvider, ChannelManual}

// RefundMethods are the ways money is paid back by hand ("online": refunded
// in the provider's own panel).
var RefundMethods = []string{salesmodel.MethodCash, salesmodel.MethodCard, salesmodel.MethodTransfer, salesmodel.MethodOnline}

// SourceRefund names refunds on the ledger entries they write.
const SourceRefund = "refund"

// Refund is one refund request of a visit with its three steps: the
// request, the provider's answer and the posting to the books.
type Refund struct {
	UUID             uuid.UUID   `json:"uuid"`
	Channel          string      `json:"channel"`
	Provider         string      `json:"provider"`
	Method           string      `json:"method"`
	Amount           money.Money `json:"amount"`
	Currency         string      `json:"currency"`
	Status           string      `json:"status"`
	ProviderStatus   string      `json:"provider_status"`
	ProviderRefundID string      `json:"provider_refund_id"`
	FailureReason    string      `json:"failure_reason"`
	Reason           string      `json:"reason"`
	ResolutionNote   string      `json:"resolution_note"`
	CollectionUUID   uuid.UUID   `json:"collection_uuid"`
	CashAccount      Ref         `json:"cash_account"`
	LedgerEntryUUID  *uuid.UUID  `json:"ledger_entry_uuid"`
	RequestedBy      *Ref        `json:"requested_by"`
	ProviderAt       *time.Time  `json:"provider_answered_at"`
	ReflectedAt      *time.Time  `json:"reflected_at"`
	CreatedAt        time.Time   `json:"created_at"`
	// NeedsAction: the provider's result is unknown, or it refunded and the
	// posting is still missing; a person settles it (resolve).
	NeedsAction bool `json:"needs_action"`
	// AutoCheck: the result is unknown and the worker asks the provider's
	// refund status query (TEC-978); ProviderChecks counts the queries made
	// and NextCheckAt is when the next one is due.
	AutoCheck         bool       `json:"auto_check"`
	ProviderChecks    int32      `json:"provider_checks"`
	ProviderCheckedAt *time.Time `json:"provider_checked_at"`
	NextCheckAt       *time.Time `json:"next_check_at"`
}

// RefundCollection is a live collection of the visit and what is left of it.
type RefundCollection struct {
	UUID        uuid.UUID   `json:"uuid"`
	Method      string      `json:"method"`
	EntryDate   string      `json:"entry_date"`
	Amount      money.Money `json:"amount"`
	Refunded    money.Money `json:"refunded"`
	Pending     money.Money `json:"pending"`
	Refundable  money.Money `json:"refundable"`
	CashAccount Ref         `json:"cash_account"`
	// Provider is the provider of the online payment behind it ("" = none);
	// ProviderRefund: its refund call can pay it back.
	Provider       string `json:"provider"`
	ProviderRefund bool   `json:"provider_refund"`
}

// RefundInput pays back (part of) one collection. Channel provider uses
// the provider's refund call (method online, out of the collection's own
// account); manual records money handed back (method and account).
// IdempotencyKey identifies the request: the same key answers the same
// refund.
type RefundInput struct {
	CollectionUUID  uuid.UUID   `json:"collection_uuid"`
	Amount          money.Money `json:"amount"`
	Channel         string      `json:"channel"`
	Method          string      `json:"method"`
	CashAccountUUID *uuid.UUID  `json:"cash_account_uuid"`
	Reason          string      `json:"reason"`
	IdempotencyKey  string      `json:"idempotency_key"`
}

// Resolutions of a refund whose provider result is unknown.
const (
	ResolveSucceeded = "succeeded"
	ResolveFailed    = "failed"
)

// ResolveInput settles a refund after a person checked the provider's
// panel: the provider refunded it (succeeded: posted now) or not (failed).
type ResolveInput struct {
	Outcome string `json:"outcome"`
	Note    string `json:"note"`
}

// Follow-up kinds of the payment follow-up list.
const (
	FollowRefundPending    = "refund_pending"
	FollowRefundFailed     = "refund_failed"
	FollowRefundUnverified = "refund_unverified"
	FollowUnpaid           = "unpaid"
)

// FollowUpKinds lists the kinds.
var FollowUpKinds = []string{FollowRefundPending, FollowRefundFailed, FollowRefundUnverified, FollowUnpaid}

// FollowUpsSlug is the table slug of the follow-up list.
const FollowUpsSlug = "tenant.payment_follow_ups"

// FollowUp is a visit whose money needs a person: a refund to make (a paid
// visit cancelled, a late online payment), a refund that failed or whose
// provider result is unknown, or a finished visit not paid in full.
// Amount is what is to be refunded (paid less refunded) or, for unpaid,
// what is still owed.
type FollowUp struct {
	UUID          uuid.UUID   `json:"uuid"`
	Number        int64       `json:"number"`
	Kind          string      `json:"kind"`
	Status        string      `json:"status"`
	PaymentStatus string      `json:"payment_status"`
	CancelReason  *string     `json:"cancel_reason"`
	StartsAt      time.Time   `json:"starts_at"`
	Branch        Ref         `json:"branch"`
	Customer      *Ref        `json:"customer"`
	Currency      string      `json:"currency"`
	Charged       money.Money `json:"charged"`
	Collected     money.Money `json:"collected"`
	Refunded      money.Money `json:"refunded"`
	Amount        money.Money `json:"amount"`
	// Late: an online payment arrived after the visit was cancelled.
	Late              bool       `json:"late"`
	OpenRefunds       int64      `json:"open_refunds"`
	LastRefundStatus  string     `json:"last_refund_status"`
	LastRefundFailure string     `json:"last_refund_failure"`
	LastRefundAt      *time.Time `json:"last_refund_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// FollowUpFilter is the follow-up list query.
type FollowUpFilter struct {
	Branches      []uuid.UUID
	Kinds         []string
	Q             string
	SortField     string
	SortDesc      bool
	Limit, Offset int32
}

// FollowUpFacet is one kind with its count.
type FollowUpFacet struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// FollowUpFacets are the kind counts of the list (with the branch and
// search filters, before the kind filter).
type FollowUpFacets struct {
	Kind []FollowUpFacet `json:"kind"`
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
