package money

import (
	"encoding/json"
	"errors"
	"math/big"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestParse(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want int64
		err  error
	}{
		{"1300", 130000, nil},
		{"1300.5", 130050, nil},
		{"1300.50", 130050, nil},
		{"1300,50", 130050, nil},
		{" 0.01 ", 1, nil},
		{".5", 50, nil},
		{"-20", -2000, nil},
		{"+7", 700, nil},
		{"007.10", 710, nil},
		{"999999999999.99", MaxMinor, nil},
		{"1000000000000", 0, ErrRange},
		{"1.005", 0, ErrPrecision},
		{"", 0, ErrInvalid},
		{"-", 0, ErrInvalid},
		{".", 0, ErrInvalid},
		{"1.2.3", 0, ErrInvalid},
		{"1 300", 0, ErrInvalid},
		{"1e3", 0, ErrInvalid},
		{"abc", 0, ErrInvalid},
	}
	for _, c := range cases {
		got, err := Parse(c.in)
		if !errors.Is(err, c.err) {
			t.Errorf("Parse(%q) error = %v, want %v", c.in, err, c.err)
			continue
		}
		if err == nil && got.Minor() != c.want {
			t.Errorf("Parse(%q) = %d, want %d", c.in, got.Minor(), c.want)
		}
	}
}

func TestParseRoundHalfUp(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]int64{
		"1.005": 101, "1.004": 100, "1.0049": 100, "-1.005": -101, "0.995": 100, "2.675": 268,
	} {
		got, err := ParseRound(in)
		if err != nil || got.Minor() != want {
			t.Errorf("ParseRound(%q) = %d %v, want %d", in, got.Minor(), err, want)
		}
	}
}

func TestStringAndJSON(t *testing.T) {
	t.Parallel()
	for minor, want := range map[int64]string{0: "0.00", 1: "0.01", -1: "-0.01", 130000: "1300.00", -123456: "-1234.56"} {
		if got := New(minor).String(); got != want {
			t.Errorf("String(%d) = %q, want %q", minor, got, want)
		}
	}
	b, _ := json.Marshal(struct {
		P Money `json:"p"`
	}{MustParse("1560")})
	if string(b) != `{"p":"1560.00"}` {
		t.Fatalf("json = %s", b)
	}
	var in struct {
		A Money `json:"a"`
		B Money `json:"b"`
		C Money `json:"c"`
	}
	if err := json.Unmarshal([]byte(`{"a":"12,5","b":1300.25,"c":null}`), &in); err != nil {
		t.Fatal(err)
	}
	if in.A.Minor() != 1250 || in.B.Minor() != 130025 || !in.C.IsZero() {
		t.Fatalf("unmarshal = %+v", in)
	}
	if err := json.Unmarshal([]byte(`{"a":"1.234"}`), &in); err == nil {
		t.Fatal("three decimals must be rejected")
	}
}

func TestArithmeticAndRounding(t *testing.T) {
	t.Parallel()
	p := MustParse("1300")
	if got := p.MulRate(Percent(20)); got.String() != "260.00" {
		t.Fatalf("20%% = %s", got)
	}
	if got := p.Add(p.MulRate(Percent(20))); got.String() != "1560.00" {
		t.Fatalf("+20%% = %s", got)
	}
	if got := p.MulRate(Percent(-20)); got.String() != "-260.00" {
		t.Fatalf("-20%% = %s", got)
	}
	// 0.05 * 10% = 0.005 → 0.01 (half up); -0.005 → -0.01 (away from zero).
	if got := New(5).MulRate(Percent(10)); got.Minor() != 1 {
		t.Fatalf("half up = %d", got.Minor())
	}
	if got := New(-5).MulRate(Percent(10)); got.Minor() != -1 {
		t.Fatalf("half away from zero = %d", got.Minor())
	}
	// VAT base of 1180.00 at 18 %: 1180 / 1.18 = 1000.00.
	if got := MustParse("1180").Ratio(RateScale, RateScale+int64(Percent(18))); got.String() != "1000.00" {
		t.Fatalf("vat base = %s", got)
	}
	// 100.00 / 3 = 33.333.. → 33.33; 200.00 / 3 = 66.666.. → 66.67.
	if got := MustParse("100").Ratio(1, 3); got.String() != "33.33" {
		t.Fatalf("100/3 = %s", got)
	}
	if got := MustParse("200").Ratio(1, 3); got.String() != "66.67" {
		t.Fatalf("200/3 = %s", got)
	}
	// Large amounts do not overflow the intermediate product.
	big1 := New(MaxMinor)
	if got := big1.MulRate(Percent(100)); got.Minor() != MaxMinor {
		t.Fatalf("max * 100%% = %d", got.Minor())
	}
	if got := MustParse("-5").ClampZero(); !got.IsZero() {
		t.Fatalf("clamp = %s", got)
	}
	if got := MustParse("10").Min(MustParse("7")); got.String() != "7.00" {
		t.Fatalf("min = %s", got)
	}
	if MustParse("1").Cmp(MustParse("2")) != -1 || !MustParse("1").Equal(New(100)) {
		t.Fatal("cmp/equal")
	}
}

func TestCurrencyMismatchPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()
	_ = New(1).Add(NewIn(1, "EUR"))
}

func TestNumeric(t *testing.T) {
	t.Parallel()
	var n pgtype.Numeric
	if err := n.Scan("1300.50"); err != nil {
		t.Fatal(err)
	}
	m, err := FromNumeric(n)
	if err != nil || m.Minor() != 130050 {
		t.Fatalf("from numeric = %v %v", m, err)
	}
	// Wider scale is rounded once (half up).
	wide := pgtype.Numeric{Int: big.NewInt(12345), Exp: -3, Valid: true} // 12.345
	if m, _ := FromNumeric(wide); m.Minor() != 1235 {
		t.Fatalf("12.345 = %d", m.Minor())
	}
	whole := pgtype.Numeric{Int: big.NewInt(13), Exp: 2, Valid: true} // 1300
	if m, _ := FromNumeric(whole); m.String() != "1300.00" {
		t.Fatalf("13e2 = %s", m)
	}
	if _, err := FromNumeric(pgtype.Numeric{}); err == nil {
		t.Fatal("NULL must fail")
	}
	if got := FormatNumeric(pgtype.Numeric{}); got != "0.00" {
		t.Fatalf("NULL format = %q", got)
	}
	back := MustParse("-12.30").Numeric()
	if v, _ := back.Value(); v != "-1230e-2" && v != "-12.30" {
		// pgtype renders "<int>e<exp>"; both spellings mean the same value.
		if m, _ := FromNumeric(back); m.String() != "-12.30" {
			t.Fatalf("round trip = %v", v)
		}
	}
	var scanned Money
	if err := scanned.ScanNumeric(n); err != nil || scanned.String() != "1300.50" {
		t.Fatalf("scan = %v %v", scanned, err)
	}
}

func TestRate(t *testing.T) {
	t.Parallel()
	r, err := ParseRate("18.25")
	if err != nil || r != 1825 || r.String() != "18.25" {
		t.Fatalf("rate = %d %v", r, err)
	}
	if _, err := ParseRate("1.234"); err == nil {
		t.Fatal("three decimals")
	}
	var in struct {
		R Rate `json:"r"`
	}
	if err := json.Unmarshal([]byte(`{"r":"-10"}`), &in); err != nil || in.R != Percent(-10) {
		t.Fatalf("rate json = %v %v", in.R, err)
	}
}
