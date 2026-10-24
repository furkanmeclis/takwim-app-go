package paytr_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/paytr"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

func TestRefund(t *testing.T) {
	in := payments.RefundRequest{OrderRef: "abc123", Amount: money.MustParse("40.50"), Currency: "TRY", RefundRef: "rf1"}
	p := paytr.New("", nil)
	f, err := p.RefundFields(account, in)
	if err != nil {
		t.Fatal(err)
	}
	if f.Get("return_amount") != "40.50" || f.Get("merchant_oid") != "abc123" || f.Get("reference_no") != "rf1" {
		t.Fatalf("fields = %v", f)
	}
	if want := hmacB64("KEY123", "123456"+"abc123"+"40.50"+"SALT456"); f.Get("paytr_token") != want {
		t.Fatalf("token = %s, want %s", f.Get("paytr_token"), want)
	}

	answer := `{"status":"success","is_test":1,"merchant_oid":"abc123","return_amount":"40.50","reference_no":"rf1"}`
	status := http.StatusOK
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		got = r.URL.Path + "?" + r.PostForm.Get("return_amount")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	}))
	defer srv.Close()
	p = paytr.New(srv.URL, srv.Client())
	res, err := p.Refund(context.Background(), account, in)
	if err != nil || res.ProviderRefundID != "rf1" || !res.Test || got != "/odeme/iade?40.50" {
		t.Fatalf("refund = %+v, %v (%s)", res, err, got)
	}

	// A refusal is definite.
	answer = `{"status":"error","err_no":"006","err_msg":"Iade tutari odeme tutarindan fazla"}`
	_, err = p.Refund(context.Background(), account, in)
	var pe *payments.ProviderError
	if !errors.As(err, &pe) || pe.Temporary || pe.Code != "006" {
		t.Fatalf("refused = %v", err)
	}
	// A 5xx or an unreadable answer leaves the result unknown.
	status, answer = http.StatusBadGateway, "oops"
	if _, err := p.Refund(context.Background(), account, in); !errors.As(err, &pe) || !pe.Temporary {
		t.Fatalf("5xx = %v", err)
	}
	status = http.StatusOK
	if _, err := p.Refund(context.Background(), account, in); !errors.As(err, &pe) || !pe.Temporary {
		t.Fatalf("unreadable = %v", err)
	}
}
