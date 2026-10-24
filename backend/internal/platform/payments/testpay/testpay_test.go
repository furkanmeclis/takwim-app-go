package testpay_test

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/testpay"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

func TestTestpay(t *testing.T) {
	p := testpay.New("https://app.test/")
	acc := integrations.Account{Credentials: integrations.Credentials{"secret_key": "s3cret"}}
	c, err := p.CreateCheckout(context.Background(), acc, payments.CheckoutRequest{OrderRef: "ref1", Amount: money.FromUnits(10)})
	if err != nil || c.PageURL != "https://app.test/pay/test/ref1" {
		t.Fatalf("checkout = %+v, %v", c, err)
	}
	h, body, err := testpay.Notify(acc, "ref1", payments.StatusSucceeded, money.FromUnits(10), "TRY")
	if err != nil {
		t.Fatal(err)
	}
	n, err := p.ParseNotification(h, body)
	if err != nil || n.OrderRef != "ref1" || n.Status != payments.StatusSucceeded || n.Amount.Minor() != 1000 {
		t.Fatalf("parse = %+v, %v", n, err)
	}
	if err := p.VerifyNotification(acc, n, h, body); err != nil {
		t.Fatal(err)
	}
	other := integrations.Account{Credentials: integrations.Credentials{"secret_key": "other"}}
	if err := p.VerifyNotification(other, n, h, body); !errors.Is(err, payments.ErrInvalidSignature) {
		t.Fatalf("other secret = %v", err)
	}
	if _, err := p.ParseNotification(h, []byte(`{"order_ref":"x","status":"maybe","amount":"1"}`)); !errors.Is(err, payments.ErrMalformed) {
		t.Fatalf("bad status = %v", err)
	}
}
