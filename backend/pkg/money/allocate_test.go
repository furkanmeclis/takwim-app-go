package money

import "testing"

func sum(parts []Money) Money {
	s := Zero()
	for _, p := range parts {
		s = s.Add(p)
	}
	return s
}

func TestSplitRemainderToLast(t *testing.T) {
	t.Parallel()
	parts, err := MustParse("100.00").Split(3)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"33.33", "33.33", "33.34"}
	for i, p := range parts {
		if p.String() != want[i] {
			t.Fatalf("part %d = %s, want %s", i, p, want[i])
		}
	}
	if !sum(parts).Equal(MustParse("100.00")) {
		t.Fatalf("sum = %s", sum(parts))
	}
}

func TestAllocateWeights(t *testing.T) {
	t.Parallel()
	parts, err := NewIn(1001, "EUR").Allocate(1, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if parts[0].Minor() != 250 || parts[1].Minor() != 250 || parts[2].Minor() != 501 {
		t.Fatalf("parts = %v", parts)
	}
	if parts[2].Currency() != "EUR" {
		t.Fatalf("currency = %s", parts[2].Currency())
	}
	neg, _ := New(-100).Split(3)
	if !sum(neg).Equal(New(-100)) || neg[0].Minor() != -33 {
		t.Fatalf("negative split = %v", neg)
	}
	large, _ := New(MaxMinor).Allocate(MaxMinor, 1)
	if large[0].Minor() != MaxMinor-1 || large[1].Minor() != 1 {
		t.Fatalf("large allocate = %v", large)
	}
}

func TestAllocateInvalid(t *testing.T) {
	t.Parallel()
	for _, w := range [][]int64{nil, {0, 0}, {1, -1}} {
		if _, err := New(100).Allocate(w...); err == nil {
			t.Fatalf("Allocate(%v) accepted", w)
		}
	}
	if _, err := New(100).Split(0); err == nil {
		t.Fatal("Split(0) accepted")
	}
}

func TestParseCurrency(t *testing.T) {
	t.Parallel()
	if c, err := ParseCurrency("EUR"); err != nil || c != "EUR" {
		t.Fatalf("EUR = %q %v", c, err)
	}
	for _, s := range []string{"", "try", "TL", "EURO"} {
		if _, err := ParseCurrency(s); err == nil {
			t.Fatalf("%q accepted", s)
		}
	}
}
