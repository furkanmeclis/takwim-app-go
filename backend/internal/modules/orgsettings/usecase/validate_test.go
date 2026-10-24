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
	s.Company.Website = "ftp://example.com"
	s.Reservation.ApprovalMode = "maybe"
	s.Reservation.PaymentHoldMinutes = intp(0)
	s.Reservation.MaxAdvanceDays = intp(0)
	s.Reservation.CancelCutoffMinutes = intp(-5)
	s.Payment = model.Payment{}
	s.SalesChannels = append(s.SalesChannels, model.SalesChannel{Code: "fax"})
	s.Tax.DefaultVATRate = "abc"
	got := fields(mustErr(t, s))
	for field, code := range map[string]string{
		"company.tax_number":                "invalid_checksum",
		"company.email":                     "invalid",
		"company.website":                   "invalid",
		"reservation.approval_mode":         "invalid",
		"reservation.payment_hold_minutes":  "out_of_range",
		"reservation.max_advance_days":      "out_of_range",
		"reservation.cancel_cutoff_minutes": "out_of_range",
		"payment.pay_at_venue":              "required",
		"sales_channels.3.code":             "invalid",
		"tax.default_vat_rate":              "invalid",
	} {
		if got[field] != code {
			t.Errorf("%s = %q, want %q (all: %v)", field, got[field], code, got)
		}
	}

	// TCKN is accepted for sole proprietors; a wrong length is invalid.
	s = model.Defaults()
	s.Company.TaxNumber = "10000000146"
	s.Company.Website = "https://kuafor.example.com"
	if _, err := Normalize(s); err != nil {
		t.Fatalf("valid TCKN: %v", err)
	}
	s.Company.TaxNumber = "123"
	if fields(mustErr(t, s))["company.tax_number"] != "invalid" {
		t.Fatal("short tax number must be invalid")
	}
	// The earliest start must fall inside the booking window.
	s = model.Defaults()
	s.Reservation.MinLeadMinutes = intp(3 * 24 * 60)
	s.Reservation.MaxAdvanceDays = intp(2)
	if fields(mustErr(t, s))["reservation.min_lead_minutes"] != "after_max_advance" {
		t.Fatal("lead time beyond the booking window must be rejected")
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
		Company:       model.Company{LegalName: "  Acme Kuaför Ltd. Şti. ", TaxNumber: "454 053 6920"},
		Payment:       model.Payment{PayOnline: true},
		SalesChannels: []model.SalesChannel{{Code: "widget", IsActive: true}},
		Tax:           model.Tax{InvoiceItemMappings: []model.InvoiceItemMapping{{ItemType: "product", AccountCode: " 600.01 "}}},
	}
	out, err := Normalize(s)
	if err != nil {
		t.Fatal(err)
	}
	if out.Company.LegalName != "Acme Kuaför Ltd. Şti." || out.Company.TaxNumber != "4540536920" {
		t.Fatalf("company = %+v", out.Company)
	}
	if len(out.SalesChannels) != len(model.SalesChannels) {
		t.Fatalf("channels = %v", out.SalesChannels)
	}
	for _, ch := range out.SalesChannels {
		if ch.IsActive != (ch.Code == "widget") {
			t.Fatalf("channel %s active = %v", ch.Code, ch.IsActive)
		}
	}
	if len(out.Tax.InvoiceItemMappings) != len(model.InvoiceItemTypes) {
		t.Fatalf("mappings = %v", out.Tax.InvoiceItemMappings)
	}
	for _, m := range out.Tax.InvoiceItemMappings {
		if m.ItemType == "product" && m.AccountCode != "600.01" {
			t.Fatalf("product mapping = %+v", m)
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
	if ready || strings.Join(missing, ",") != "reservation.approval_mode,reservation.min_lead_minutes,reservation.max_advance_days" {
		t.Fatalf("defaults ready=%v missing=%v", ready, missing)
	}
	s := model.Defaults()
	s.Payment.PayOnline = true
	s.Reservation = model.Reservation{ApprovalMode: model.ApprovalStaff, MinLeadMinutes: intp(60), MaxAdvanceDays: intp(30)}
	ready, missing = model.Readiness(s)
	if ready || strings.Join(missing, ",") != "reservation.staff_review_window_minutes,reservation.payment_hold_minutes" {
		t.Fatalf("staff + online ready=%v missing=%v", ready, missing)
	}
	s.Reservation.StaffReviewWindowMinutes = intp(30)
	s.Reservation.PaymentHoldMinutes = intp(15)
	if ready, missing = model.Readiness(s); !ready || len(missing) != 0 {
		t.Fatalf("complete ready=%v missing=%v", ready, missing)
	}
	// Auto approval at the venue needs neither window.
	s = model.Defaults()
	s.Reservation = model.Reservation{ApprovalMode: model.ApprovalAuto, MinLeadMinutes: intp(0), MaxAdvanceDays: intp(60)}
	if ready, missing = model.Readiness(s); !ready {
		t.Fatalf("auto at venue ready=%v missing=%v", ready, missing)
	}
}

func TestNormalizeOrigin(t *testing.T) {
	t.Parallel()
	ok := map[string]string{
		"example.com":                 "https://example.com",
		" HTTPS://Shop.Example.com/ ": "https://shop.example.com",
		"https://*.example.com":       "https://*.example.com",
		"https://example.com:8443":    "https://example.com:8443",
		"http://localhost:3000":       "http://localhost:3000",
		"http://127.0.0.1:8080":       "http://127.0.0.1:8080",
		"xn--kuafr-jua.com.tr":        "https://xn--kuafr-jua.com.tr",
	}
	for in, want := range ok {
		got, valid := NormalizeOrigin(in)
		if !valid || got != want {
			t.Errorf("NormalizeOrigin(%q) = %q, %v; want %q", in, got, valid, want)
		}
	}
	for _, in := range []string{
		"", "http://example.com", "https://example.com/booking", "https://example.com?x=1",
		"https://user@example.com", "ftp://example.com", "https://com", "https://10.0.0.1",
		"https://*.localhost", "https://exa mple.com", "https://example.com; script-src *",
		"https://example.com:99999", "https://-bad.com", "javascript:alert(1)", "*",
	} {
		if got, valid := NormalizeOrigin(in); valid {
			t.Errorf("NormalizeOrigin(%q) = %q, want invalid", in, got)
		}
	}
}

func TestNormalizeEmbedOrigins(t *testing.T) {
	t.Parallel()
	in := model.Defaults()
	in.Embed.AllowedOrigins = []string{"example.com", "https://example.com/", "", "https://*.shop.com"}
	out, err := Normalize(in)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if strings.Join(out.Embed.AllowedOrigins, ",") != "https://example.com,https://*.shop.com" {
		t.Fatalf("origins = %v", out.Embed.AllowedOrigins)
	}

	in.Embed.AllowedOrigins = []string{"https://ok.com", "http://bad.com"}
	_, err = Normalize(in)
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Details[0].Field != "embed.allowed_origins.1" {
		t.Fatalf("want embed.allowed_origins.1 error, got %v", err)
	}

	in.Embed.AllowedOrigins = nil
	for i := 0; i <= model.MaxEmbedOrigins; i++ {
		in.Embed.AllowedOrigins = append(in.Embed.AllowedOrigins, "https://s"+strings.Repeat("a", i+1)+".com")
	}
	if _, err := Normalize(in); !errors.As(err, &ve) || ve.Details[0].Code != "too_many" {
		t.Fatalf("want too_many, got %v", err)
	}

	in.Embed.AllowedOrigins = nil
	out, err = Normalize(in)
	if err != nil || out.Embed.AllowedOrigins == nil || len(out.Embed.AllowedOrigins) != 0 {
		t.Fatalf("nil origins = %v, %v", out.Embed.AllowedOrigins, err)
	}
}
