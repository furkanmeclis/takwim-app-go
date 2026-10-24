// Package timetable expands the weekly class timetable of a gym (TEC-970)
// into sessions. It does no I/O: the classes module (session lists,
// single-session changes) and the availability engine (a group class is
// offered only at its sessions; a session holds its trainer and area)
// load the rows and ask the same questions here, so both agree on which
// sessions exist.
package timetable

import (
	"strconv"
	"strings"
	"time"
)

// DateLayout is the layout of local dates.
const DateLayout = "2006-01-02"

// Weekdays maps the stored day keys to weekdays, in display order in
// WeekdayKeys.
var Weekdays = map[string]time.Weekday{
	"mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday, "thu": time.Thursday,
	"fri": time.Friday, "sat": time.Saturday, "sun": time.Sunday,
}

// WeekdayKeys lists the day keys Monday first.
var WeekdayKeys = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

// Entry is one weekly class of the timetable.
type Entry struct {
	ID int64
	// Weekday is the stored key ("mon" … "sun"), Start the wall clock
	// ("HH:MM") in Location.
	Weekday  string
	Start    string
	Location *time.Location
	// ValidFrom / ValidUntil are local dates (ValidUntil empty = no end).
	ValidFrom  string
	ValidUntil string
}

// Change is a single session that differs from its entry.
type Change struct {
	Cancelled bool
	// Capacity replaces the seats of the entry when > 0.
	Capacity int
}

// Occurrence is one session of an entry.
type Occurrence struct {
	// Date is the local date of the session (its identity with the entry).
	Date  string
	Start time.Time
}

// Clock reads "HH:MM" as hour and minute.
func Clock(s string) (int, int, bool) {
	h, m, ok := strings.Cut(s, ":")
	if !ok || len(h) != 2 || len(m) != 2 {
		return 0, 0, false
	}
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, 0, false
	}
	return hh, mm, true
}

// ValidDate reports whether s is a local date.
func ValidDate(s string) bool {
	_, err := time.Parse(DateLayout, s)
	return err == nil
}

func (e Entry) loc() *time.Location {
	if e.Location == nil {
		return time.UTC
	}
	return e.Location
}

// runsOn reports whether the entry has a session on the local date d
// (civil date at UTC midnight).
func (e Entry) runsOn(d time.Time) bool {
	wd, ok := Weekdays[e.Weekday]
	if !ok || d.Weekday() != wd {
		return false
	}
	date := d.Format(DateLayout)
	if date < e.ValidFrom {
		return false
	}
	return e.ValidUntil == "" || date <= e.ValidUntil
}

// startOn is the instant of the session on the local date d. Building it
// from the date fields keeps the wall clock right on daylight saving days.
func (e Entry) startOn(d time.Time) (time.Time, bool) {
	hh, mm, ok := Clock(e.Start)
	if !ok {
		return time.Time{}, false
	}
	return time.Date(d.Year(), d.Month(), d.Day(), hh, mm, 0, 0, e.loc()), true
}

// Occurrences lists the sessions of the entry on the local dates
// [first, last] (inclusive, "2006-01-02"), in order.
func (e Entry) Occurrences(first, last string) []Occurrence {
	from, err1 := time.Parse(DateLayout, first)
	to, err2 := time.Parse(DateLayout, last)
	if err1 != nil || err2 != nil {
		return nil
	}
	var out []Occurrence
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if !e.runsOn(d) {
			continue
		}
		if start, ok := e.startOn(d); ok {
			out = append(out, Occurrence{Date: d.Format(DateLayout), Start: start})
		}
	}
	return out
}

// On returns the session of the entry on a local date.
func (e Entry) On(date string) (Occurrence, bool) {
	d, err := time.Parse(DateLayout, date)
	if err != nil || !e.runsOn(d) {
		return Occurrence{}, false
	}
	start, ok := e.startOn(d)
	if !ok {
		return Occurrence{}, false
	}
	return Occurrence{Date: date, Start: start}, true
}

// At reports whether the entry has a session starting at the instant and
// returns its local date.
func (e Entry) At(t time.Time) (string, bool) {
	local := t.In(e.loc())
	date := local.Format(DateLayout)
	occ, ok := e.On(date)
	if !ok || !occ.Start.Equal(t) {
		return "", false
	}
	return date, true
}

// Seats are the seats of a session: the change's, else the entry's, else
// the service's group capacity.
func Seats(change *Change, entry, service int) int {
	if change != nil && change.Capacity > 0 {
		return change.Capacity
	}
	if entry > 0 {
		return entry
	}
	return service
}

// Overlap reports whether two entries can run at the same time: same
// weekday, overlapping validity and overlapping held minutes of the day
// (start − before … start + duration + after). Entries in different time
// zones compare their wall clocks.
func Overlap(a Entry, aFrom, aTo int, b Entry, bFrom, bTo int) bool {
	if a.Weekday != b.Weekday {
		return false
	}
	if a.ValidUntil != "" && a.ValidUntil < b.ValidFrom {
		return false
	}
	if b.ValidUntil != "" && b.ValidUntil < a.ValidFrom {
		return false
	}
	return aFrom < bTo && bFrom < aTo
}

// Minutes is the minute of the day of a wall clock ("HH:MM").
func Minutes(s string) int {
	hh, mm, _ := Clock(s)
	return hh*60 + mm
}
