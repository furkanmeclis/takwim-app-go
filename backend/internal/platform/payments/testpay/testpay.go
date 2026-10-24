// Package testpay is the development payment provider (TEC-912): its
// hosted page is a Takwim test page (/pay/test/{ref}) with "pay" and
// "fail" buttons, and its notifications are signed with the account's own
// secret exactly like a real provider's, so the whole online payment flow
// (checkout, signed notification, idempotency, ledger) runs locally before
// a real provider is chosen (TEC-884). It is registered outside production
// only and moves no money.
package testpay

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// ProviderCode is the provider code of the test provider.
const ProviderCode = "testpay"

// SignatureHeader carries hex(HMAC-SHA256(secret_key, body)).
const SignatureHeader = "X-Testpay-Signature"

// Provider implements payments.Provider without any network call.
type Provider struct {
	// PageBase is the frontend origin of the test page.
	PageBase string
}

// New builds the provider; pageBase is the frontend origin.
func New(pageBase string) *Provider {
	return &Provider{PageBase: strings.TrimRight(pageBase, "/")}
}

var _ payments.Provider = (*Provider)(nil)

// Kind implements integrations.Adapter.
func (p *Provider) Kind() string { return integrations.KindPayment }

// Provider implements integrations.Adapter.
func (p *Provider) Provider() string { return ProviderCode }

// Fields implements integrations.Adapter: one signing secret.
func (p *Provider) Fields() []integrations.Field {
	return []integrations.Field{
		{Key: "secret_key", LabelKey: "integrations.fields.secret_key", Secret: true, Required: true},
	}
}

// Check implements integrations.Adapter (always reachable).
func (p *Provider) Check(context.Context, integrations.Account) error { return nil }

// CreateCheckout implements payments.Provider: the page is the test page.
func (p *Provider) CreateCheckout(_ context.Context, _ integrations.Account, in payments.CheckoutRequest) (payments.Checkout, error) {
	if in.OrderRef == "" || !in.Amount.IsPositive() {
		return payments.Checkout{}, &payments.ProviderError{Code: "invalid_request", Message: "order reference and amount are required"}
	}
	return payments.Checkout{PageURL: p.PageBase + "/pay/test/" + url.PathEscape(in.OrderRef), Token: "test-" + in.OrderRef}, nil
}

// Body is the notification body of the test provider.
type Body struct {
	OrderRef  string `json:"order_ref"`
	Status    string `json:"status"`
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
	PaymentID string `json:"payment_id"`
}

// ParseNotification implements payments.Provider.
func (p *Provider) ParseNotification(_ http.Header, body []byte) (payments.Notification, error) {
	var b Body
	if err := json.Unmarshal(body, &b); err != nil || b.OrderRef == "" {
		return payments.Notification{}, payments.ErrMalformed
	}
	if b.Status != payments.StatusSucceeded && b.Status != payments.StatusFailed {
		return payments.Notification{}, payments.ErrMalformed
	}
	amount, err := money.Parse(b.Amount)
	if err != nil {
		return payments.Notification{}, payments.ErrMalformed
	}
	n := payments.Notification{
		OrderRef: b.OrderRef, EventKey: b.OrderRef + ":" + b.Status, Status: b.Status, Amount: amount, PaidAmount: amount,
		Currency: b.Currency, ProviderPaymentID: b.PaymentID, Test: true,
	}
	if b.Status == payments.StatusFailed {
		n.FailureCode, n.FailureMessage = "test_declined", "declined on the test page"
	}
	return n, nil
}

// Sign is the signature of a body with the account's secret.
func Sign(account integrations.Account, body []byte) string {
	mac := hmac.New(sha256.New, []byte(account.Credentials["secret_key"]))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyNotification implements payments.Provider.
func (p *Provider) VerifyNotification(account integrations.Account, _ payments.Notification, header http.Header, body []byte) error {
	if account.Credentials["secret_key"] == "" {
		return payments.ErrInvalidSignature
	}
	got := header.Get(SignatureHeader)
	if got == "" || !hmac.Equal([]byte(got), []byte(Sign(account, body))) {
		return payments.ErrInvalidSignature
	}
	return nil
}

// Acknowledge implements payments.Provider.
func (p *Provider) Acknowledge(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// Notify builds the signed notification the test page sends for a
// payment: what a real provider would post to the notification URL.
func Notify(account integrations.Account, orderRef, status string, amount money.Money, currency string) (http.Header, []byte, error) {
	if status != payments.StatusSucceeded && status != payments.StatusFailed {
		return nil, nil, fmt.Errorf("testpay: status %q", status)
	}
	body, err := json.Marshal(Body{
		OrderRef: orderRef, Status: status, Amount: amount.String(), Currency: currency, PaymentID: "test_" + orderRef,
	})
	if err != nil {
		return nil, nil, err
	}
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set(SignatureHeader, Sign(account, body))
	return h, body, nil
}
