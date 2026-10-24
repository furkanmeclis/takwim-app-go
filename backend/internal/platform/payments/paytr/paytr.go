// Package paytr is the PayTR iFrame API adapter (TEC-912, requirements R3):
// a business connects its own PayTR merchant (merchant id, key and salt,
// sealed by the integration vault). Takwim asks PayTR for a payment token
// and sends the customer to PayTR's hosted page; PayTR posts the result to
// the notification URL configured in the merchant panel
// (/api/v1/public/payments/paytr/notify), signed with the merchant key.
//
// The adapter is registered only when PAYMENTS_PAYTR_ENABLED is true: the
// provider choice (TEC-884) and an end-to-end test with a real merchant
// are still open, see docs/design/payments.md.
package paytr

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// ProviderCode is the provider code of PayTR.
const ProviderCode = "paytr"

// DefaultBaseURL is PayTR's API origin (test and live use the same host;
// test_mode selects the test environment).
const DefaultBaseURL = "https://www.paytr.com"

// Provider implements payments.Provider for PayTR.
type Provider struct {
	BaseURL string
	Client  *http.Client
}

// New builds the adapter (empty base = DefaultBaseURL).
func New(baseURL string, client *http.Client) *Provider {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &Provider{BaseURL: strings.TrimRight(baseURL, "/"), Client: client}
}

var _ payments.Provider = (*Provider)(nil)

// Kind implements integrations.Adapter.
func (p *Provider) Kind() string { return integrations.KindPayment }

// Provider implements integrations.Adapter.
func (p *Provider) Provider() string { return ProviderCode }

// Fields implements integrations.Adapter.
func (p *Provider) Fields() []integrations.Field {
	return []integrations.Field{
		{Key: "merchant_id", LabelKey: "integrations.fields.merchant_id", Secret: false, Required: true},
		{Key: "merchant_key", LabelKey: "integrations.fields.merchant_key", Secret: true, Required: true},
		{Key: "merchant_salt", LabelKey: "integrations.fields.merchant_salt", Secret: true, Required: true},
	}
}

var merchantIDRE = regexp.MustCompile(`^[0-9]{1,20}$`)

// Check implements integrations.Adapter. PayTR has no call that only
// authenticates (a token request opens a payment), so the check validates
// the credentials' form; the first checkout proves them.
func (p *Provider) Check(_ context.Context, account integrations.Account) error {
	c := account.Credentials
	if !merchantIDRE.MatchString(c["merchant_id"]) {
		return errors.New("paytr: merchant_id must be the numeric merchant number")
	}
	if c["merchant_key"] == "" || c["merchant_salt"] == "" {
		return errors.New("paytr: merchant_key and merchant_salt are required")
	}
	return nil
}

func sign(key, message string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(message))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// currencyCode maps ISO codes to PayTR's.
func currencyCode(c string) string {
	if c == "" || c == "TRY" {
		return "TL"
	}
	return c
}

func isoCurrency(c string) string {
	if c == "TL" || c == "" {
		return "TRY"
	}
	return c
}

func testMode(account integrations.Account) string {
	if account.Mode == integrations.ModeLive {
		return "0"
	}
	return "1"
}

// TokenFields builds the get-token form of a checkout (exported for tests).
func (p *Provider) TokenFields(account integrations.Account, in payments.CheckoutRequest) (url.Values, error) {
	c := account.Credentials
	if !in.Amount.IsPositive() || in.OrderRef == "" {
		return nil, &payments.ProviderError{Code: "invalid_request", Message: "order reference and amount are required"}
	}
	basket, err := json.Marshal([][]any{{in.ItemName, in.Amount.String(), 1}})
	if err != nil {
		return nil, err
	}
	userBasket := base64.StdEncoding.EncodeToString(basket)
	amount := strconv.FormatInt(in.Amount.Minor(), 10)
	noInstallment, maxInstallment := "1", "0" // a full payment, no installments
	currency := currencyCode(in.Currency)
	mode := testMode(account)
	email := in.Buyer.Email
	if email == "" {
		// PayTR requires an e-mail; a booking may have none.
		email = "musteri+" + strings.ToLower(in.OrderRef) + "@takwim.link"
	}
	ip := in.Buyer.IP
	if ip == "" {
		ip = "127.0.0.1"
	}
	timeout := 30
	if !in.ExpiresAt.IsZero() {
		if m := int(time.Until(in.ExpiresAt).Minutes()); m > 0 && m < timeout {
			timeout = m
		}
		if timeout < 1 {
			timeout = 1
		}
	}
	hashStr := c["merchant_id"] + ip + in.OrderRef + email + amount + userBasket + noInstallment + maxInstallment + currency + mode
	lang := "tr"
	if in.Locale == "en" {
		lang = "en"
	}
	name := in.Buyer.Name
	if name == "" {
		name = "Takwim"
	}
	phone := in.Buyer.Phone
	if phone == "" {
		phone = "0000000000"
	}
	return url.Values{
		"merchant_id":       {c["merchant_id"]},
		"user_ip":           {ip},
		"merchant_oid":      {in.OrderRef},
		"email":             {email},
		"payment_amount":    {amount},
		"paytr_token":       {sign(c["merchant_key"], hashStr+c["merchant_salt"])},
		"user_basket":       {userBasket},
		"debug_on":          {"0"},
		"no_installment":    {noInstallment},
		"max_installment":   {maxInstallment},
		"user_name":         {name},
		"user_address":      {"-"},
		"user_phone":        {phone},
		"merchant_ok_url":   {in.SuccessURL},
		"merchant_fail_url": {in.FailURL},
		"timeout_limit":     {strconv.Itoa(timeout)},
		"currency":          {currency},
		"test_mode":         {mode},
		"lang":              {lang},
	}, nil
}

// CreateCheckout implements payments.Provider (iFrame API step 1: token).
func (p *Provider) CreateCheckout(ctx context.Context, account integrations.Account, in payments.CheckoutRequest) (payments.Checkout, error) {
	form, err := p.TokenFields(account, in)
	if err != nil {
		return payments.Checkout{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/odeme/api/get-token", strings.NewReader(form.Encode()))
	if err != nil {
		return payments.Checkout{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.Client.Do(req)
	if err != nil {
		return payments.Checkout{}, &payments.ProviderError{Message: err.Error(), Temporary: true}
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return payments.Checkout{}, &payments.ProviderError{Message: err.Error(), Temporary: true}
	}
	if resp.StatusCode >= 500 {
		return payments.Checkout{}, &payments.ProviderError{Code: strconv.Itoa(resp.StatusCode), Message: "provider unavailable", Temporary: true}
	}
	var out struct {
		Status string `json:"status"`
		Token  string `json:"token"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return payments.Checkout{}, fmt.Errorf("%w: %v", payments.ErrMalformed, err)
	}
	if out.Status != "success" || out.Token == "" {
		return payments.Checkout{}, &payments.ProviderError{Code: "token_refused", Message: out.Reason}
	}
	return payments.Checkout{PageURL: p.BaseURL + "/odeme/guvenli/" + url.PathEscape(out.Token), Token: out.Token}, nil
}

// ParseNotification implements payments.Provider (iFrame API step 2: the
// form PayTR posts to the notification URL).
func (p *Provider) ParseNotification(_ http.Header, body []byte) (payments.Notification, error) {
	v, err := url.ParseQuery(string(bytes.TrimSpace(body)))
	if err != nil {
		return payments.Notification{}, payments.ErrMalformed
	}
	oid, status, total := v.Get("merchant_oid"), v.Get("status"), v.Get("total_amount")
	if oid == "" || v.Get("hash") == "" || (status != "success" && status != "failed") {
		return payments.Notification{}, payments.ErrMalformed
	}
	n := payments.Notification{
		OrderRef: oid, EventKey: oid + ":" + status, Currency: isoCurrency(v.Get("currency")),
		Test: v.Get("test_mode") == "1", ProviderPaymentID: oid,
	}
	if status == "success" {
		n.Status = payments.StatusSucceeded
		paid, err := strconv.ParseInt(total, 10, 64)
		if err != nil {
			return payments.Notification{}, payments.ErrMalformed
		}
		n.PaidAmount = money.New(paid)
		n.Amount = n.PaidAmount
		if raw := v.Get("payment_amount"); raw != "" {
			amount, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return payments.Notification{}, payments.ErrMalformed
			}
			n.Amount = money.New(amount)
		}
	} else {
		n.Status = payments.StatusFailed
		n.FailureCode, n.FailureMessage = v.Get("failed_reason_code"), v.Get("failed_reason_msg")
	}
	return n, nil
}

// NotificationHash is the hash PayTR sends with a notification:
// base64(HMAC-SHA256(merchant_key, merchant_oid + merchant_salt + status + total_amount)).
func NotificationHash(account integrations.Account, oid, status, total string) string {
	c := account.Credentials
	return sign(c["merchant_key"], oid+c["merchant_salt"]+status+total)
}

// VerifyNotification implements payments.Provider.
func (p *Provider) VerifyNotification(account integrations.Account, _ payments.Notification, _ http.Header, body []byte) error {
	if account.Credentials["merchant_key"] == "" || account.Credentials["merchant_salt"] == "" {
		return payments.ErrInvalidSignature
	}
	v, err := url.ParseQuery(string(bytes.TrimSpace(body)))
	if err != nil {
		return payments.ErrMalformed
	}
	want := NotificationHash(account, v.Get("merchant_oid"), v.Get("status"), v.Get("total_amount"))
	if !hmac.Equal([]byte(want), []byte(v.Get("hash"))) {
		return payments.ErrInvalidSignature
	}
	return nil
}

// Acknowledge implements payments.Provider: PayTR keeps notifying until it
// reads the plain text "OK".
func (p *Provider) Acknowledge(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

var _ payments.Refunder = (*Provider)(nil)

// RefundFields builds the form of PayTR's refund (iade) API (exported for
// tests): paytr_token = base64(HMAC-SHA256(merchant_key, merchant_id +
// merchant_oid + return_amount + merchant_salt)).
func (p *Provider) RefundFields(account integrations.Account, in payments.RefundRequest) (url.Values, error) {
	c := account.Credentials
	if in.OrderRef == "" || !in.Amount.IsPositive() {
		return nil, &payments.ProviderError{Code: "invalid_request", Message: "order reference and amount are required"}
	}
	amount := in.Amount.String()
	v := url.Values{
		"merchant_id":   {c["merchant_id"]},
		"merchant_oid":  {in.OrderRef},
		"return_amount": {amount},
		"paytr_token":   {sign(c["merchant_key"], c["merchant_id"]+in.OrderRef+amount+c["merchant_salt"])},
	}
	if in.RefundRef != "" {
		v.Set("reference_no", in.RefundRef)
	}
	return v, nil
}

// Refund implements payments.Refunder (PayTR refund API, R4): a full or
// partial refund of a paid order. A transport failure, a 5xx or an
// unreadable answer leaves the result unknown (Temporary).
func (p *Provider) Refund(ctx context.Context, account integrations.Account, in payments.RefundRequest) (payments.RefundResult, error) {
	form, err := p.RefundFields(account, in)
	if err != nil {
		return payments.RefundResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/odeme/iade", strings.NewReader(form.Encode()))
	if err != nil {
		return payments.RefundResult{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.Client.Do(req)
	if err != nil {
		return payments.RefundResult{}, &payments.ProviderError{Message: err.Error(), Temporary: true}
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return payments.RefundResult{}, &payments.ProviderError{Message: err.Error(), Temporary: true}
	}
	if resp.StatusCode >= 500 {
		return payments.RefundResult{}, &payments.ProviderError{Code: strconv.Itoa(resp.StatusCode), Message: "provider unavailable", Temporary: true}
	}
	var out struct {
		Status      string `json:"status"`
		IsTest      any    `json:"is_test"`
		ReferenceNo string `json:"reference_no"`
		ErrNo       any    `json:"err_no"`
		ErrMsg      string `json:"err_msg"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return payments.RefundResult{}, &payments.ProviderError{Message: "unreadable refund answer", Temporary: true}
	}
	switch out.Status {
	case "success":
		ref := out.ReferenceNo
		if ref == "" {
			ref = in.RefundRef
		}
		return payments.RefundResult{ProviderRefundID: ref, Test: fmt.Sprint(out.IsTest) == "1"}, nil
	case "error", "failed":
		code := ""
		if out.ErrNo != nil {
			code = fmt.Sprint(out.ErrNo)
		}
		return payments.RefundResult{}, &payments.ProviderError{Code: code, Message: out.ErrMsg}
	}
	return payments.RefundResult{}, &payments.ProviderError{Message: "unexpected refund answer", Temporary: true}
}
