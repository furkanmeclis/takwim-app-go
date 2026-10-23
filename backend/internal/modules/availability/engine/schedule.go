package engine

import (
	"strconv"
	"strings"
	"time"
)

// Hours is the schedule of one day: closed, or open from Open to Close
// ("HH:MM", "24:00" allowed as Close).
type Hours struct {
	Closed bool
	Open   string
	Close  string
}

// Weekly is a weekly schedule keyed by weekday; a missing day is closed.
type Weekly map[time.Weekday]Hours

// Span is a wall-clock range of one day ("HH:MM").
type Span struct {
	Start string
	End   string
}

// Shift is one working period of a weekday with its breaks.
type Shift struct {
	Span
	Breaks []Span
}

// WeeklyShifts are the shifts of one person in one branch by weekday.
type WeeklyShifts map[time.Weekday][]Shift

// WeekdayKeys maps the stored day keys ("mon" … "sun") to weekdays.
var WeekdayKeys = map[string]time.Weekday{
	"mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday, "thu": time.Thursday,
	"fri": time.Friday, "sat": time.Saturday, "sun": time.Sunday,
}

// ParseClock reads "HH:MM" as minutes after midnight (24:00 = 1440).
func ParseClock(s string) (int, bool) {
	h, m, ok := strings.Cut(s, ":")
	if !ok || len(h) != 2 || len(m) != 2 {
		return 0, false
	}
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hh < 0 || mm < 0 || mm > 59 || hh > 24 || (hh == 24 && mm != 0) {
		return 0, false
	}
	return hh*60 + mm, true
}

// at is the instant of a wall-clock minute on a local date. Building it from
// the date fields (not by adding minutes to midnight) keeps the wall clock
// right on daylight saving days.
func at(day time.Time, minute int, loc *time.Location) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), minute/60, minute%60, 0, 0, loc)
}

func span(day time.Time, s Span, loc *time.Location) (Interval, bool) {
	start, ok1 := ParseClock(s.Start)
	end, ok2 := ParseClock(s.End)
	if !ok1 || !ok2 || end <= start {
		return Interval{}, false
	}
	return Interval{Start: at(day, start, loc), End: at(day, end, loc)}, true
}

// eachDay calls fn with local midnight of every date from first to last.
func eachDay(first, last time.Time, loc *time.Location, fn func(day time.Time)) {
	for d := dayStart(first, loc); !d.After(dayStart(last, loc)); d = d.AddDate(0, 0, 1) {
		fn(d)
	}
}

// OpenIntervals turns weekly hours and date exceptions (keyed yyyy-MM-dd,
// replacing the weekly hours of that date) into instants for the dates
// first..last.
func OpenIntervals(loc *time.Location, first, last time.Time, weekly Weekly, exceptions map[string]Hours) []Interval {
	var out []Interval
	eachDay(first, last, loc, func(day time.Time) {
		h, ok := exceptions[day.Format("2006-01-02")]
		if !ok {
			h, ok = weekly[day.Weekday()]
		}
		if !ok || h.Closed {
			return
		}
		if iv, ok := span(day, Span{Start: h.Open, End: h.Close}, loc); ok {
			out = append(out, iv)
		}
	})
	return out
}

// WorkIntervals turns weekly shifts into working instants (shifts minus
// breaks) for the dates first..last.
func WorkIntervals(loc *time.Location, first, last time.Time, shifts WeeklyShifts) []Interval {
	var out []Interval
	eachDay(first, last, loc, func(day time.Time) {
		for _, sh := range shifts[day.Weekday()] {
			iv, ok := span(day, sh.Span, loc)
			if !ok {
				continue
			}
			pieces := []Interval{iv}
			for _, b := range sh.Breaks {
				bi, ok := span(day, b, loc)
				if !ok {
					continue
				}
				pieces = subtract(pieces, bi)
			}
			out = append(out, pieces...)
		}
	})
	return out
}

// Intersect returns the parts of a that are also in b.
func Intersect(a, b []Interval) []Interval {
	var out []Interval
	for _, x := range a {
		for _, y := range b {
			start, end := x.Start, x.End
			if y.Start.After(start) {
				start = y.Start
			}
			if y.End.Before(end) {
				end = y.End
			}
			if start.Before(end) {
				out = append(out, Interval{Start: start, End: end})
			}
		}
	}
	return out
}

func subtract(pieces []Interval, cut Interval) []Interval {
	var out []Interval
	for _, p := range pieces {
		if !p.Overlaps(cut) {
			out = append(out, p)
			continue
		}
		if p.Start.Before(cut.Start) {
			out = append(out, Interval{Start: p.Start, End: cut.Start})
		}
		if cut.End.Before(p.End) {
			out = append(out, Interval{Start: cut.End, End: p.End})
		}
	}
	return out
}
