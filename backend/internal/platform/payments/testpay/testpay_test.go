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

func TestTestpayRefund(t *testing.T) {
	p := testpay.New("https://app.test/")
	acc := integrations.Account{Credentials: integrations.Credentials{"secret_key": "s3cret"}}
	in := payments.RefundRequest{OrderRef: "ref1", RefundRef: "rf1", Amount: money.MustParse("40.00")}
	if res, err := p.Refund(context.Background(), acc, in); err != nil || res.ProviderRefundID != "test_refund_rf1" {
		t.Fatalf("refund = %+v, %v", res, err)
	}
	var pe *payments.ProviderError
	in.Amount = money.MustParse("40.13")
	if _, err := p.Refund(context.Background(), acc, in); !errors.As(err, &pe) || pe.Temporary {
		t.Fatalf("declined = %v", err)
	}
	in.Amount = money.MustParse("40.99")
	if _, err := p.Refund(context.Background(), acc, in); !errors.As(err, &pe) || !pe.Temporary {
		t.Fatalf("unknown = %v", err)
	}
}

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
