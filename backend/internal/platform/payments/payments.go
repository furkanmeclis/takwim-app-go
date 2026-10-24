// Package payments is the payment provider layer of Takwim (TEC-912,
// docs/design/payments.md): the Provider interface every online payment
// adapter implements on top of the integration skeleton (accounts with
// sealed credentials, inbound notification idempotency).
//
// Card data never touches Takwim: CreateCheckout opens the provider's
// hosted page and returns its address; the customer pays there. A browser
// return is never proof of a payment: only a provider notification whose
// signature VerifyNotification accepted with the business's own secret
// records a result.
package payments

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Notification statuses (provider statuses are mapped onto these).
const (
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
)

// Buyer is the payer data a provider asks for (fraud checks). No card data.
type Buyer struct {
	Name  string
	Email string
	Phone string
	IP    string
}

// CheckoutRequest opens a hosted checkout for one full payment.
type CheckoutRequest struct {
	// OrderRef is our reference of the payment (letters and digits only);
	// the provider echoes it in its notification.
	OrderRef string
	Amount   money.Money
	Currency string
	// ItemName names what is paid ("Kesim · #12").
	ItemName string
	Buyer    Buyer
	// SuccessURL / FailURL are where the browser goes back; the page they
	// open shows the server-verified state, never "paid" on its own.
	SuccessURL string
	FailURL    string
	Locale     string
	// ExpiresAt bounds the provider page (the reservation's payment window).
	ExpiresAt time.Time
}

// Checkout is an opened hosted page.
type Checkout struct {
	PageURL string
	// Token is the provider's reference of the page (not a secret).
	Token string
}

// Notification is a parsed provider notification (not trusted before
// VerifyNotification).
type Notification struct {
	// OrderRef is our payment reference (CheckoutRequest.OrderRef).
	OrderRef string
	// EventKey identifies the notification at the provider: the same
	// notification delivered again has the same key.
	EventKey string
	Status   string
	// Amount is what the payment was for; PaidAmount what the customer
	// paid (installment interest included), both in Currency.
	Amount     money.Money
	PaidAmount money.Money
	Currency   string
	// ProviderPaymentID is the provider's payment reference when it has one.
	ProviderPaymentID string
	FailureCode       string
	FailureMessage    string
	Test              bool
}

// Provider is one online payment provider.
type Provider interface {
	integrations.Adapter
	// CreateCheckout opens the hosted payment page of a payment.
	CreateCheckout(ctx context.Context, account integrations.Account, in CheckoutRequest) (Checkout, error)
	// ParseNotification reads a notification body; nothing is trusted yet
	// (the business, hence the secret, is found from its OrderRef).
	ParseNotification(header http.Header, body []byte) (Notification, error)
	// VerifyNotification checks the notification's signature with the
	// account's secret.
	VerifyNotification(account integrations.Account, n Notification, header http.Header, body []byte) error
	// Acknowledge answers a handled notification the way the provider
	// expects (PayTR: the plain text "OK"), so it stops sending it again.
	Acknowledge(w http.ResponseWriter)
}

// RefundRequest pays back (part of) a succeeded payment through the
// provider (TEC-913).
type RefundRequest struct {
	// OrderRef is our reference of the refunded payment (CheckoutRequest.OrderRef).
	OrderRef string
	// ProviderPaymentID is the provider's reference of the payment, when it
	// gave one.
	ProviderPaymentID string
	Amount            money.Money
	Currency          string
	// RefundRef is our reference of this refund (letters and digits): the
	// provider keeps it with the refund where it supports one.
	RefundRef string
}

// RefundResult is a refund the provider carried out.
type RefundResult struct {
	// ProviderRefundID is the provider's reference of the refund ("" when it
	// gives none).
	ProviderRefundID string
	Test             bool
}

// Refunder is implemented by providers that refund through their API. The
// call must classify its failure: a *ProviderError with Temporary false is a
// definite refusal (nothing was refunded); a Temporary error or any other
// error means the result is unknown, and the refund is not sent again
// before a person checked the provider's panel.
type Refunder interface {
	Refund(ctx context.Context, account integrations.Account, in RefundRequest) (RefundResult, error)
}

// RefunderOf returns the refund call of a provider, when it has one.
func RefunderOf(p Provider) (Refunder, bool) {
	r, ok := p.(Refunder)
	return r, ok
}

// What a refund status query found (TEC-978).
const (
	// RefundFound: the provider carried the refund out.
	RefundFound = "succeeded"
	// RefundNotDone: the provider has no such refund, or refused it:
	// nothing was paid back.
	RefundNotDone = "not_done"
	// RefundStillPending: the provider has the refund but not its final
	// state yet; ask again later.
	RefundStillPending = "pending"
)

// RefundCheck is the answer of a refund status query.
type RefundCheck struct {
	// Status is RefundFound, RefundNotDone or RefundStillPending.
	Status string
	// ProviderRefundID is the provider's reference of the refund, when found.
	ProviderRefundID string
	// Message is the provider's reason (a refusal's code and text).
	Message string
}

// RefundChecker is implemented by providers that can tell what happened to
// a refund whose call had an unknown result, by our RefundRef or the
// payment's references. It only asks; it never refunds. An error means the
// state is still unknown: the refund is asked about again later and never
// sent again.
type RefundChecker interface {
	CheckRefund(ctx context.Context, account integrations.Account, in RefundRequest) (RefundCheck, error)
}

// RefundCheckerOf returns the refund status query of a provider, when it
// has one.
func RefundCheckerOf(p Provider) (RefundChecker, bool) {
	c, ok := p.(RefundChecker)
	return c, ok
}

var (
	// ErrInvalidSignature: a notification did not verify.
	ErrInvalidSignature = errors.New("payments: invalid signature")
	// ErrMalformed: the provider message could not be read.
	ErrMalformed = errors.New("payments: malformed provider message")
)

// ProviderError is a provider refusal (code + message, never card data);
// Temporary marks network / 5xx failures worth trying again.
type ProviderError struct {
	Code      string
	Message   string
	Temporary bool
}

func (e *ProviderError) Error() string {
	if e.Code != "" {
		return "payments: provider error " + e.Code + ": " + e.Message
	}
	return "payments: provider error: " + e.Message
}

// Lookup returns the payment provider registered for code.
func Lookup(reg *integrations.Registry, code string) (Provider, bool) {
	a, ok := reg.Get(integrations.KindPayment, code)
	if !ok {
		return nil, false
	}
	p, ok := a.(Provider)
	return p, ok
}
