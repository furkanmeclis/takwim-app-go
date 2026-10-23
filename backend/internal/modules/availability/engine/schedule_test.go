package engine

import (
	"testing"
	"time"
)

func TestParseClock(t *testing.T) {
	cases := map[string]int{"00:00": 0, "09:30": 570, "23:59": 1439, "24:00": 1440}
	for in, want := range cases {
		if got, ok := ParseClock(in); !ok || got != want {
			t.Fatalf("%s = %d, %v", in, got, ok)
		}
	}
	for _, bad := range []string{"", "9:30", "24:01", "25:00", "12:60", "ab:cd", "12-30"} {
		if _, ok := ParseClock(bad); ok {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestWorkIntervalsSubtractBreaks(t *testing.T) {
	shifts := WeeklyShifts{time.Monday: {
		{Span: Span{"09:00", "13:00"}, Breaks: []Span{{"10:00", "10:15"}, {"12:30", "13:00"}, {"bad", "x"}}},
		{Span: Span{"14:00", "24:00"}},
		{Span: Span{"18:00", "17:00"}}, // invalid: skipped
	}}
	got := WorkIntervals(ist, monday, monday.AddDate(0, 0, 1), shifts)
	want := []Interval{iv("09:00", "10:00"), iv("10:15", "12:30"), {Start: clock("14:00"), End: monday.AddDate(0, 0, 1)}}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if !got[i].Start.Equal(want[i].Start) || !got[i].End.Equal(want[i].End) {
			t.Fatalf("interval %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// Wall-clock hours stay wall-clock on a daylight saving change.
func TestOpenIntervalsKeepWallClockOnDSTDays(t *testing.T) {
	berlin := mustLoc("Europe/Berlin")
	sunday := time.Date(2026, 10, 25, 12, 0, 0, 0, berlin) // clocks go back at 03:00
	got := OpenIntervals(berlin, sunday, sunday, Weekly{time.Sunday: {Open: "01:00", Close: "05:00"}}, nil)
	if len(got) != 1 {
		t.Fatalf("got %v", got)
	}
	if h := got[0].Start.In(berlin).Format("15:04"); h != "01:00" {
		t.Fatalf("start = %s", h)
	}
	if h := got[0].End.In(berlin).Format("15:04"); h != "05:00" {
		t.Fatalf("end = %s", h)
	}
	if d := got[0].End.Sub(got[0].Start); d != 5*time.Hour {
		t.Fatalf("length = %v, want 5h (one hour repeats)", d)
	}
}

func TestIntersect(t *testing.T) {
	got := Intersect([]Interval{iv("09:00", "12:00"), iv("13:00", "18:00")}, []Interval{iv("11:00", "14:00")})
	if len(got) != 2 || !got[0].Start.Equal(clock("11:00")) || !got[0].End.Equal(clock("12:00")) ||
		!got[1].Start.Equal(clock("13:00")) || !got[1].End.Equal(clock("14:00")) {
		t.Fatalf("got %v", got)
	}
}
