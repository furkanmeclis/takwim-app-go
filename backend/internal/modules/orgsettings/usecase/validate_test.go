package usecase

import (
	"errors"
	"strings"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
)

func TestValidVKN(t *testing.T) {
	t.Parallel()
	valid := []string{"4540536920", "1111111114"}
	for _, v := range valid {
		if !ValidVKN(v) {
			t.Errorf("ValidVKN(%q) = false, want true", v)
		}
	}
	invalid := []string{"4540536921", "1111111111", "123456789", "12345678901", "45405369a0", ""}
	for _, v := range invalid {
		if ValidVKN(v) {
			t.Errorf("ValidVKN(%q) = true, want false", v)
		}
	}
}

func TestValidTCKN(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"10000000146", "11111111110", "12345678950"} {
		if !ValidTCKN(v) {
			t.Errorf("ValidTCKN(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"10000000147", "01234567890", "1234567890", "1000000014a", "12345678901"} {
		if ValidTCKN(v) {
			t.Errorf("ValidTCKN(%q) = true, want false", v)
		}
	}
}

func fields(err error) map[string]string {
	out := map[string]string{}
	var ve *ValidationError
	if errors.As(err, &ve) {
		for _, d := range ve.Details {
			out[d.Field] = d.Code
		}
	}
	return out
}

func intp(v int) *int { return &v }

func TestNormalizeValidation(t *testing.T) {
	t.Parallel()
	s := model.Defaults()
	s.Company.TaxNumber = "4540536921"
	s.Company.Email = "not-an-email"
	s.Reservation.ApprovalMode = "maybe"
	s.Reservation.PaymentHoldMinutes = intp(0)
	s.Payment.PrepaymentRule = model.PrepaymentPercent
	s.Payment.PrepaymentValue = "120"
	s.Policies.ExtraHourFee = "12.345"
	s.SalesChannels = append(s.SalesChannels, model.SalesChannel{Code: "fax"})
	s.Tax.DefaultVATRate = "abc"
	got := fields(mustErr(t, s))
	for field, code := range map[string]string{
		"company.tax_number":               "invalid_checksum",
		"company.email":                    "invalid",
		"reservation.approval_mode":        "invalid",
		"reservation.payment_hold_minutes": "out_of_range",
		"payment.prepayment_value":         "invalid",
		"policies.extra_hour_fee":          "invalid",
		"sales_channels.6.code":            "invalid",
		"tax.default_vat_rate":             "invalid",
	} {
		if got[field] != code {
			t.Errorf("%s = %q, want %q (all: %v)", field, got[field], code, got)
		}
	}

	// TCKN is accepted for sole proprietors; a wrong length is invalid.
	s = model.Defaults()
	s.Company.TaxNumber = "10000000146"
	if _, err := Normalize(s); err != nil {
		t.Fatalf("valid TCKN: %v", err)
	}
	s.Company.TaxNumber = "123"
	if fields(mustErr(t, s))["company.tax_number"] != "invalid" {
		t.Fatal("short tax number must be invalid")
	}
	// Fixed prepayment needs an amount.
	s = model.Defaults()
	s.Payment.PrepaymentRule = model.PrepaymentFixed
	if fields(mustErr(t, s))["payment.prepayment_value"] != "required" {
		t.Fatal("fixed prepayment without amount must be required")
	}
}

func mustErr(t *testing.T, s model.Settings) error {
	t.Helper()
	_, err := Normalize(s)
	if err == nil {
		t.Fatal("expected a validation error")
	}
	return err
}

func TestNormalizeFillsChannelsAndMappings(t *testing.T) {
	t.Parallel()
	s := model.Settings{
		Company:       model.Company{LegalName: "  Acme Oto Kiralama A.Ş. ", TaxNumber: "454 053 6920"},
		SalesChannels: []model.SalesChannel{{Code: "obilet", IsActive: true}},
		Tax:           model.Tax{InvoiceItemMappings: []model.InvoiceItemMapping{{ItemType: "fuel", AccountCode: " 600.01 "}}},
	}
	out, err := Normalize(s)
	if err != nil {
		t.Fatal(err)
	}
	if out.Company.LegalName != "Acme Oto Kiralama A.Ş." || out.Company.TaxNumber != "4540536920" {
		t.Fatalf("company = %+v", out.Company)
	}
	if out.Payment.PrepaymentRule != model.PrepaymentNone {
		t.Fatalf("prepayment rule default = %q", out.Payment.PrepaymentRule)
	}
	if len(out.SalesChannels) != len(model.SalesChannels) {
		t.Fatalf("channels = %v", out.SalesChannels)
	}
	for _, ch := range out.SalesChannels {
		if ch.IsActive != (ch.Code == "obilet") {
			t.Fatalf("channel %s active = %v", ch.Code, ch.IsActive)
		}
	}
	if len(out.Tax.InvoiceItemMappings) != len(model.InvoiceItemTypes) {
		t.Fatalf("mappings = %v", out.Tax.InvoiceItemMappings)
	}
	for _, m := range out.Tax.InvoiceItemMappings {
		if m.ItemType == "fuel" && m.AccountCode != "600.01" {
			t.Fatalf("fuel mapping = %+v", m)
		}
	}
	// No hardcoded legal VAT rate.
	if out.Tax.DefaultVATRate != "" || model.Defaults().Tax.DefaultVATRate != "" {
		t.Fatal("default VAT rate must stay empty")
	}
}

func TestReadiness(t *testing.T) {
	t.Parallel()
	ready, missing := model.Readiness(model.Defaults())
	if ready || len(missing) != 4 {
		t.Fatalf("defaults ready=%v missing=%v", ready, missing)
	}
	s := model.Defaults()
	s.Reservation = model.Reservation{
		ApprovalMode: model.ApprovalStaff, StaffReviewWindowMinutes: intp(30),
		PaymentHoldMinutes: intp(15),
	}
	ready, missing = model.Readiness(s)
	if ready || strings.Join(missing, ",") != "reservation.quote_validity_minutes" {
		t.Fatalf("partial ready=%v missing=%v", ready, missing)
	}
	s.Reservation.QuoteValidityMinutes = intp(60)
	if ready, missing = model.Readiness(s); !ready || len(missing) != 0 {
		t.Fatalf("complete ready=%v missing=%v", ready, missing)
	}
}
