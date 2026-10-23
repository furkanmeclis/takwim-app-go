package localtime

import (
	"encoding/json"
	"testing"
	"time"
)

func TestLoadZone(t *testing.T) {
	t.Parallel()
	if loc, err := LoadZone(""); err != nil || loc.String() != DefaultZone {
		t.Fatalf("empty zone = %v %v", loc, err)
	}
	if loc, err := LoadZone("Europe/Berlin"); err != nil || loc.String() != "Europe/Berlin" {
		t.Fatalf("berlin = %v %v", loc, err)
	}
	for _, n := range []string{"Local", "Mars/Olympus", "../etc"} {
		if _, err := LoadZone(n); err == nil {
			t.Fatalf("%q accepted", n)
		}
	}
	if ValidZone("") || !ValidZone("UTC") {
		t.Fatal("ValidZone")
	}
	if ZoneOr("", "Europe/Berlin") != "Europe/Berlin" || ZoneOr("", "") != DefaultZone {
		t.Fatal("ZoneOr")
	}
}

func TestRangeHalfOpen(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 6, 1, 7, 0, 0, 0, time.UTC)
	a, _ := NewRange(base, base.Add(30*time.Minute))
	b, _ := NewRange(base.Add(30*time.Minute), base.Add(time.Hour))
	if a.Overlaps(b) || b.Overlaps(a) {
		t.Fatal("touching ranges overlap")
	}
	if !a.Contains(base) || a.Contains(a.End) {
		t.Fatal("Contains is not half-open")
	}
	whole, _ := NewRange(base, base.Add(time.Hour))
	if !whole.Covers(a) || !whole.Covers(b) || a.Covers(whole) {
		t.Fatal("Covers")
	}
	if _, err := NewRange(base, base); err == nil {
		t.Fatal("empty range accepted")
	}
}

func TestDateInZone(t *testing.T) {
	t.Parallel()
	// 22:30 UTC on May 31 is already June 1 in Istanbul (UTC+3).
	at := time.Date(2026, 5, 31, 22, 30, 0, 0, time.UTC)
	if got := DateIn(at, nil); got != MustDate("2026-06-01") {
		t.Fatalf("istanbul date = %s", got)
	}
	if got := DateIn(at, time.UTC); got != MustDate("2026-05-31") {
		t.Fatalf("utc date = %s", got)
	}
	d := MustDate("2026-06-01")
	b := d.Bounds(nil)
	if b.Start != time.Date(2026, 5, 31, 21, 0, 0, 0, time.UTC) || b.Duration() != Day {
		t.Fatalf("bounds = %v", b)
	}
	if got := d.At(9*60+30, nil).UTC(); got != time.Date(2026, 6, 1, 6, 30, 0, 0, time.UTC) {
		t.Fatalf("At = %v", got)
	}
}

func TestDayBoundsDST(t *testing.T) {
	t.Parallel()
	berlin, err := LoadZone("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	if got := MustDate("2026-03-29").Bounds(berlin).Duration(); got != 23*time.Hour {
		t.Fatalf("spring forward day = %v", got)
	}
	if got := MustDate("2026-10-25").Bounds(berlin).Duration(); got != 25*time.Hour {
		t.Fatalf("fall back day = %v", got)
	}
}

func TestDateRange(t *testing.T) {
	t.Parallel()
	r, err := NewDateRange(MustDate("2026-06-01"), MustDate("2026-09-01"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Days() != 92 || !r.Contains(MustDate("2026-08-31")) || r.Contains(MustDate("2026-09-01")) {
		t.Fatalf("range = %+v days=%d", r, r.Days())
	}
	week, _ := NewDateRange(MustDate("2026-06-01"), MustDate("2026-06-08"))
	dates := week.Dates()
	if len(dates) != 7 || dates[0].Weekday() != time.Monday || dates[6] != MustDate("2026-06-07") {
		t.Fatalf("dates = %v", dates)
	}
	if b := week.Bounds(nil); b.Duration() != 7*Day {
		t.Fatalf("week bounds = %v", b)
	}
	if _, err := NewDateRange(MustDate("2026-06-01"), MustDate("2026-06-01")); err == nil {
		t.Fatal("empty date range accepted")
	}
}

func TestDateJSON(t *testing.T) {
	t.Parallel()
	b, _ := json.Marshal(MustDate("2026-02-03"))
	if string(b) != `"2026-02-03"` {
		t.Fatalf("marshal = %s", b)
	}
	var d Date
	if err := json.Unmarshal([]byte(`"2026-12-31"`), &d); err != nil || d != NewDate(2026, 12, 31) {
		t.Fatalf("unmarshal = %v %v", d, err)
	}
	if err := json.Unmarshal([]byte(`"31.12.2026"`), &d); err == nil {
		t.Fatal("bad date accepted")
	}
	if NewDate(2026, 2, 30) != MustDate("2026-03-02") {
		t.Fatal("NewDate normalisation")
	}
}
