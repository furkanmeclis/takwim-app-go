package timetable

import (
	"testing"
	"time"
)

func TestOccurrences(t *testing.T) {
	ist, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		t.Fatal(err)
	}
	e := Entry{Weekday: "mon", Start: "18:30", Location: ist, ValidFrom: "2030-01-08", ValidUntil: "2030-01-28"}
	got := e.Occurrences("2030-01-01", "2030-02-28")
	if len(got) != 3 || got[0].Date != "2030-01-14" || got[1].Date != "2030-01-21" || got[2].Date != "2030-01-28" {
		t.Fatalf("occurrences = %+v", got)
	}
	want := time.Date(2030, 1, 14, 18, 30, 0, 0, ist)
	if !got[0].Start.Equal(want) {
		t.Fatalf("start = %v, want %v", got[0].Start, want)
	}
	if date, ok := e.At(want); !ok || date != "2030-01-14" {
		t.Fatalf("At = %q %v", date, ok)
	}
	if _, ok := e.At(want.Add(time.Minute)); ok {
		t.Fatal("At matched another minute")
	}
	if _, ok := e.On("2030-01-28"); !ok {
		t.Fatal("the last day has a session")
	}
	if _, ok := e.On("2030-01-07"); ok {
		t.Fatal("a session before valid_from")
	}
	if s := Seats(&Change{Capacity: 4}, 6, 12); s != 4 {
		t.Fatalf("seats = %d", s)
	}
	if s := Seats(nil, 0, 12); s != 12 {
		t.Fatalf("seats = %d", s)
	}
	a := Entry{Weekday: "mon", ValidFrom: "2030-01-01"}
	b := Entry{Weekday: "mon", ValidFrom: "2030-02-01"}
	if !Overlap(a, 600, 660, b, 630, 690) {
		t.Fatal("overlapping classes")
	}
	a.ValidUntil = "2030-01-31"
	if Overlap(a, 600, 660, b, 630, 690) {
		t.Fatal("classes of different periods overlap")
	}
}
