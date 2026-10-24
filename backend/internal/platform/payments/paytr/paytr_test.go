package paytr_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/paytr"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

var account = integrations.Account{
	Kind: integrations.KindPayment, Provider: paytr.ProviderCode, Mode: integrations.ModeSandbox,
	Credentials: integrations.Credentials{"merchant_id": "123456", "merchant_key": "KEY123", "merchant_salt": "SALT456"},
}

func hmacB64(key, msg string) string {
	m := hmac.New(sha256.New, []byte(key))
	m.Write([]byte(msg))
	return base64.StdEncoding.EncodeToString(m.Sum(nil))
}

func checkout() payments.CheckoutRequest {
	return payments.CheckoutRequest{
		OrderRef: "abc123", Amount: money.MustParse("450.00"), Currency: "TRY", ItemName: "Kesim",
		Buyer:      payments.Buyer{Name: "Zeynep Kaya", Email: "z@example.test", Phone: "5551112233", IP: "10.0.0.1"},
		SuccessURL: "https://app.test/ok", FailURL: "https://app.test/fail", ExpiresAt: time.Now().Add(15 * time.Minute),
	}
}

func TestTokenFields(t *testing.T) {
	p := paytr.New("", nil)
	f, err := p.TokenFields(account, checkout())
	if err != nil {
		t.Fatal(err)
	}
	if f.Get("payment_amount") != "45000" || f.Get("currency") != "TL" || f.Get("test_mode") != "1" || f.Get("no_installment") != "1" {
		t.Fatalf("fields = %v", f)
	}
	want := hmacB64("KEY123", "123456"+"10.0.0.1"+"abc123"+"z@example.test"+"45000"+f.Get("user_basket")+"1"+"0"+"TL"+"1"+"SALT456")
	if f.Get("paytr_token") != want {
		t.Fatalf("paytr_token = %s, want %s", f.Get("paytr_token"), want)
	}
	basket, _ := base64.StdEncoding.DecodeString(f.Get("user_basket"))
	if string(basket) != `[["Kesim","450.00",1]]` {
		t.Fatalf("basket = %s", basket)
	}
	if _, err := p.TokenFields(account, payments.CheckoutRequest{OrderRef: "x"}); err == nil {
		t.Fatal("zero amount accepted")
	}
	live := account
	live.Mode = integrations.ModeLive
	if f, _ := p.TokenFields(live, checkout()); f.Get("test_mode") != "0" {
		t.Fatal("live account in test mode")
	}
}

func TestCreateCheckout(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/odeme/api/get-token" {
			http.NotFound(w, r)
			return
		}
		_ = r.ParseForm()
		got = r.PostForm
		if got.Get("merchant_oid") == "refuse" {
			_, _ = w.Write([]byte(`{"status":"failed","reason":"paytr_token gecersiz"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","token":"tok_1"}`))
	}))
	defer srv.Close()
	p := paytr.New(srv.URL, srv.Client())
	c, err := p.CreateCheckout(context.Background(), account, checkout())
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != "tok_1" || c.PageURL != srv.URL+"/odeme/guvenli/tok_1" || got.Get("merchant_id") != "123456" {
		t.Fatalf("checkout = %+v, form %v", c, got)
	}
	in := checkout()
	in.OrderRef = "refuse"
	var pe *payments.ProviderError
	if _, err := p.CreateCheckout(context.Background(), account, in); !errors.As(err, &pe) || pe.Temporary {
		t.Fatalf("refused token = %v", err)
	}
}

func notification(status, total string, acc integrations.Account) []byte {
	v := url.Values{
		"merchant_oid": {"abc123"}, "status": {status}, "total_amount": {total}, "payment_amount": {"45000"},
		"hash": {paytr.NotificationHash(acc, "abc123", status, total)}, "currency": {"TL"}, "test_mode": {"1"},
	}
	if status == "failed" {
		v.Set("failed_reason_code", "2")
		v.Set("failed_reason_msg", "Yetersiz bakiye")
	}
	return []byte(v.Encode())
}

func TestNotification(t *testing.T) {
	p := paytr.New("", nil)
	body := notification("success", "45000", account)
	n, err := p.ParseNotification(nil, body)
	if err != nil {
		t.Fatal(err)
	}
	if n.OrderRef != "abc123" || n.Status != payments.StatusSucceeded || n.Amount.Minor() != 45000 || n.Currency != "TRY" || n.EventKey != "abc123:success" {
		t.Fatalf("notification = %+v", n)
	}
	if err := p.VerifyNotification(account, n, nil, body); err != nil {
		t.Fatalf("verify = %v", err)
	}
	// The hash formula PayTR documents.
	if want := hmacB64("KEY123", "abc123"+"SALT456"+"success"+"45000"); paytr.NotificationHash(account, "abc123", "success", "45000") != want {
		t.Fatal("notification hash formula")
	}
	// Another merchant's key, a tampered amount or status: refused.
	other := account
	other.Credentials = integrations.Credentials{"merchant_id": "1", "merchant_key": "OTHER", "merchant_salt": "SALT456"}
	if err := p.VerifyNotification(other, n, nil, body); !errors.Is(err, payments.ErrInvalidSignature) {
		t.Fatalf("other key = %v", err)
	}
	tampered := []byte(strings.Replace(string(body), "total_amount=45000", "total_amount=1", 1))
	if err := p.VerifyNotification(account, n, nil, tampered); !errors.Is(err, payments.ErrInvalidSignature) {
		t.Fatalf("tampered = %v", err)
	}
	failed := notification("failed", "0", account)
	n, err = p.ParseNotification(nil, failed)
	if err != nil || n.Status != payments.StatusFailed || n.FailureMessage != "Yetersiz bakiye" {
		t.Fatalf("failed = %+v, %v", n, err)
	}
	if _, err := p.ParseNotification(nil, []byte("status=success")); !errors.Is(err, payments.ErrMalformed) {
		t.Fatalf("malformed = %v", err)
	}
	rec := httptest.NewRecorder()
	p.Acknowledge(rec)
	if rec.Body.String() != "OK" {
		t.Fatalf("ack = %q", rec.Body.String())
	}
}

func TestCheck(t *testing.T) {
	p := paytr.New("", nil)
	if err := p.Check(context.Background(), account); err != nil {
		t.Fatal(err)
	}
	bad := account
	bad.Credentials = integrations.Credentials{"merchant_id": "abc", "merchant_key": "k", "merchant_salt": "s"}
	if err := p.Check(context.Background(), bad); err == nil {
		t.Fatal("non-numeric merchant id accepted")
	}
}
