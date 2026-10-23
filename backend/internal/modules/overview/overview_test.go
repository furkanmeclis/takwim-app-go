package overview

import (
	"testing"
	"time"

	branchmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/model"
)

func TestDaySchedule(t *testing.T) {
	t.Parallel()
	hours := branchmodel.WeeklyHours{
		// 2026-10-23 is a Friday.
		Fri: branchmodel.DayHours{Open: "09:00", Close: "18:00"},
		Sat: branchmodel.DayHours{Open: "18:00", Close: "02:00"},
		Sun: branchmodel.DayHours{Closed: true},
	}
	b := branchmodel.Branch{
		WorkingHours: hours,
		HolidayExceptions: []branchmodel.HolidayException{
			{Date: "2026-10-29", Closed: true, Note: "Cumhuriyet Bayramı"},
		},
	}
	at := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	cases := []struct {
		name     string
		zone     string
		now      string
		wantDate string
		closed   bool
		open     bool
	}{
		// 06:30 UTC = 09:30 Istanbul.
		{"open in branch zone", "Europe/Istanbul", "2026-10-23T06:30:00Z", "2026-10-23", false, true},
		{"before opening", "Europe/Istanbul", "2026-10-23T05:30:00Z", "2026-10-23", false, false},
		{"closing time is closed", "Europe/Istanbul", "2026-10-23T15:00:00Z", "2026-10-23", false, false},
		// 22:30 UTC Friday = 01:30 Saturday in Istanbul: Saturday hours,
		// not yet open (overnight hours count from the opening day).
		{"local day differs from UTC", "Europe/Istanbul", "2026-10-23T22:30:00Z", "2026-10-24", false, false},
		{"overnight after opening", "Europe/Istanbul", "2026-10-24T20:00:00Z", "2026-10-24", false, true},
		{"weekly closed", "Europe/Istanbul", "2026-10-25T10:00:00Z", "2026-10-25", true, false},
		{"no hours configured", "Europe/Istanbul", "2026-10-26T10:00:00Z", "2026-10-26", true, false},
		{"bad zone falls back", "Mars/Olympus", "2026-10-23T06:30:00Z", "2026-10-23", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			day, open := daySchedule(b, tc.zone, at(tc.now))
			if day.Date != tc.wantDate || day.Closed != tc.closed || open != tc.open {
				t.Fatalf("day = %+v open = %v, want date %s closed %v open %v", day, open, tc.wantDate, tc.closed, tc.open)
			}
		})
	}

	// A dated exception wins over the weekly hours.
	// (A copy: the parallel subtests above still read b.)
	short := b
	short.HolidayExceptions = []branchmodel.HolidayException{{Date: "2026-10-23", Open: "12:00", Close: "14:00", Note: "Kısa gün"}}
	day, open := daySchedule(short, "Europe/Istanbul", at("2026-10-23T06:30:00Z"))
	if !day.Exception || day.Open != "12:00" || day.Note != "Kısa gün" || open {
		t.Fatalf("exception day = %+v open = %v", day, open)
	}
}

func TestEffectiveZone(t *testing.T) {
	t.Parallel()
	own := "Europe/Berlin"
	empty := ""
	if got := effectiveZone(&own, "Europe/Istanbul"); got != own {
		t.Fatalf("branch zone = %s", got)
	}
	if got := effectiveZone(&empty, "Asia/Baku"); got != "Asia/Baku" {
		t.Fatalf("business zone = %s", got)
	}
	if got := effectiveZone(nil, ""); got != defaultTimezone {
		t.Fatalf("default zone = %s", got)
	}
}
