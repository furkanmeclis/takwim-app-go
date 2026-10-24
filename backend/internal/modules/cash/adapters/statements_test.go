package adapters

import "testing"

func TestParseSheetAmount(t *testing.T) {
	cases := map[string]string{
		"1.250,50":    "1250.50",
		"1,250.50":    "1250.50",
		"1250":        "1250.00",
		"1.250":       "1250.00",
		"250,5":       "250.50",
		"250.5":       "250.50",
		"-80,00":      "-80.00",
		"80,00-":      "-80.00",
		"(80,00)":     "-80.00",
		"₺ 1.250,50":  "1250.50",
		"1.250,50 TL": "1250.50",
		"+12.00":      "12.00",
		"1,234,567":   "1234567.00",
	}
	for raw, want := range cases {
		got, err := parseSheetAmount(raw)
		if err != nil {
			t.Errorf("%q: %v", raw, err)
			continue
		}
		if got.String() != want {
			t.Errorf("%q = %s, want %s", raw, got, want)
		}
	}
	for _, raw := range []string{"", "abc", "-"} {
		if _, err := parseSheetAmount(raw); err == nil {
			t.Errorf("%q parsed", raw)
		}
	}
}

func TestParseSheetDate(t *testing.T) {
	cases := map[string]string{
		"2026-06-15":           "2026-06-15",
		"15.06.2026":           "2026-06-15",
		"15/06/2026":           "2026-06-15",
		"2026-06-15T10:20:30Z": "2026-06-15",
		"46188":                "2026-06-15",
	}
	for raw, want := range cases {
		got, err := parseSheetDate(raw)
		if err != nil {
			t.Errorf("%q: %v", raw, err)
			continue
		}
		if got.Format("2006-01-02") != want {
			t.Errorf("%q = %s, want %s", raw, got.Format("2006-01-02"), want)
		}
	}
	if _, err := parseSheetDate("yesterday"); err == nil {
		t.Error("free text parsed as a date")
	}
}

func TestLineRow(t *testing.T) {
	defaults := map[string]any{"statement_uuid": "4b1b1c3e-0d7e-4d4b-9b8e-2b8c6c9f1a11"}
	st, in, err := lineRow(map[string]any{
		"line_date": "15.06.2026", "credit": "1.250,50", "debit": "", "description": "POS", "reference": "R1", "fee": "-25,00",
		"line_type": "Tahsilat",
	}, defaults)
	if err != nil {
		t.Fatal(err)
	}
	if st.String() != defaults["statement_uuid"] || in.Direction != "in" || in.Amount.String() != "1250.50" ||
		in.Fee == nil || in.Fee.String() != "25.00" || in.LineType != "payment" || in.LineDate != "2026-06-15" {
		t.Fatalf("row = %+v", in)
	}
	_, in, err = lineRow(map[string]any{"line_date": "2026-06-16", "amount": "-80"}, defaults)
	if err != nil || in.Direction != "out" || in.Amount.String() != "80.00" || in.LineType != "" {
		t.Fatalf("signed amount row = %+v, %v", in, err)
	}
	_, in, err = lineRow(map[string]any{"line_date": "2026-06-16", "amount": "80", "direction": "Borç"}, defaults)
	if err != nil || in.Direction != "out" {
		t.Fatalf("direction alias row = %+v, %v", in, err)
	}
	if _, _, err := lineRow(map[string]any{"line_date": "2026-06-16", "credit": "1", "debit": "1"}, defaults); err == nil {
		t.Fatal("credit and debit together accepted")
	}
	if _, _, err := lineRow(map[string]any{"line_date": "2026-06-16", "amount": "1"}, nil); err == nil {
		t.Fatal("row without a statement accepted")
	}
}
