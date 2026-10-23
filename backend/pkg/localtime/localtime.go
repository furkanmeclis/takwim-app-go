// Package localtime holds the time rules of the product (TEC-878).
//
//   - Instants are stored and compared in UTC (timestamptz columns).
//   - Calendar decisions (which local day an appointment falls on, opening
//     hours, "today") use the zone of the business or branch. The default
//     zone is Europe/Istanbul: DefaultZone / Default().
//   - Ranges are half-open: [start, end). An appointment 10:00–10:30 and the
//     next one 10:30–11:00 touch but do not overlap; a date range
//     "1 June – 1 September" covers June 1 .. August 31.
package localtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
	// Zones must resolve in minimal containers too.
	_ "time/tzdata"
)

// DefaultZone is the zone of a business that did not choose one.
const DefaultZone = "Europe/Istanbul"

// Day is 24 hours. Calendar days are not always 24h in zones with DST; use
// Date.AddDays and DayBounds for calendar arithmetic.
const Day = 24 * time.Hour

var (
	// ErrInvalidRange is returned when end is not after start.
	ErrInvalidRange = errors.New("localtime: end must be after start")
	// ErrInvalidZone is returned for an unknown IANA zone name.
	ErrInvalidZone = errors.New("localtime: unknown time zone")
)

var defaultLoc = func() *time.Location {
	loc, err := time.LoadLocation(DefaultZone)
	if err != nil {
		// Turkey is UTC+3 all year since 2016.
		return time.FixedZone(DefaultZone, 3*60*60)
	}
	return loc
}()

// Default returns the Europe/Istanbul location.
func Default() *time.Location { return defaultLoc }

// LoadZone resolves an IANA zone name ("Europe/Istanbul"). Empty means the
// default zone; "Local" and unknown names are rejected so a stored zone is
// always explicit.
func LoadZone(name string) (*time.Location, error) {
	if name == "" {
		return defaultLoc, nil
	}
	if name == "Local" {
		return nil, ErrInvalidZone
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, ErrInvalidZone
	}
	return loc, nil
}

// ValidZone reports whether name is an IANA zone LoadZone accepts.
func ValidZone(name string) bool {
	_, err := LoadZone(name)
	return name != "" && err == nil
}

// ZoneOr returns the first non-empty zone name, else DefaultZone. A branch
// without its own zone inherits the business zone: ZoneOr(branch, org).
func ZoneOr(names ...string) string {
	for _, n := range names {
		if n != "" {
			return n
		}
	}
	return DefaultZone
}

// UTC normalises an instant for storage (UTC, microsecond precision like
// timestamptz).
func UTC(t time.Time) time.Time { return t.UTC().Truncate(time.Microsecond) }

// In returns t in loc (nil loc = default zone).
func In(t time.Time, loc *time.Location) time.Time { return t.In(orDefault(loc)) }

func orDefault(loc *time.Location) *time.Location {
	if loc == nil {
		return defaultLoc
	}
	return loc
}

// ------------------------------------------------------------------ ranges

// Range is a half-open instant range [Start, End).
type Range struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// NewRange validates end > start and normalises both to UTC.
func NewRange(start, end time.Time) (Range, error) {
	if !end.After(start) {
		return Range{}, ErrInvalidRange
	}
	return Range{Start: UTC(start), End: UTC(end)}, nil
}

// Duration returns End - Start.
func (r Range) Duration() time.Duration { return r.End.Sub(r.Start) }

// Contains reports whether t is in [Start, End).
func (r Range) Contains(t time.Time) bool { return !t.Before(r.Start) && t.Before(r.End) }

// Overlaps reports whether two half-open ranges share an instant
// (touching ranges, a.End == b.Start, do not overlap).
func (r Range) Overlaps(o Range) bool { return r.Start.Before(o.End) && o.Start.Before(r.End) }

// Covers reports whether o lies entirely inside r.
func (r Range) Covers(o Range) bool { return !o.Start.Before(r.Start) && !o.End.After(r.End) }

// ------------------------------------------------------------------ dates

// Date is a calendar date without a zone ("2026-06-01").
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// DateOf returns the calendar date of t in t's own location.
func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date{Year: y, Month: m, Day: d}
}

// DateIn returns the calendar date of an instant in loc (nil = default).
func DateIn(t time.Time, loc *time.Location) Date { return DateOf(t.In(orDefault(loc))) }

// Today returns the current date in loc (nil = default).
func Today(loc *time.Location) Date { return DateIn(time.Now(), loc) }

// NewDate builds a date (normalised: NewDate(2026, 2, 30) = 2026-03-02).
func NewDate(y int, m time.Month, d int) Date {
	return DateOf(time.Date(y, m, d, 0, 0, 0, 0, time.UTC))
}

// ParseDate reads "2006-01-02".
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return Date{}, fmt.Errorf("localtime: invalid date %q", s)
	}
	return DateOf(t), nil
}

// MustDate is ParseDate that panics (tests).
func MustDate(s string) Date {
	d, err := ParseDate(s)
	if err != nil {
		panic(err)
	}
	return d
}

// IsZero reports the zero date.
func (d Date) IsZero() bool { return d.Year == 0 && d.Month == 0 && d.Day == 0 }

// Time returns midnight UTC of the date (pgtype.Date values).
func (d Date) Time() time.Time { return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC) }

// Weekday returns the day of the week.
func (d Date) Weekday() time.Weekday { return d.Time().Weekday() }

// StartIn returns local midnight of the date in loc as an instant.
func (d Date) StartIn(loc *time.Location) time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, orDefault(loc))
}

// At returns the instant of a local wall-clock time on the date in loc
// (minutes since midnight, e.g. 9*60+30 for 09:30).
func (d Date) At(minutes int, loc *time.Location) time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, minutes, 0, 0, orDefault(loc))
}

// Bounds returns the instant range of the local calendar day in loc:
// [midnight, next midnight). It is 23 or 25 hours long on DST change days.
func (d Date) Bounds(loc *time.Location) Range {
	return Range{Start: UTC(d.StartIn(loc)), End: UTC(d.AddDays(1).StartIn(loc))}
}

// AddDays returns the date n days later.
func (d Date) AddDays(n int) Date { return DateOf(d.Time().AddDate(0, 0, n)) }

// Before, After and Equal compare dates.
func (d Date) Before(o Date) bool { return d.Time().Before(o.Time()) }
func (d Date) After(o Date) bool  { return d.Time().After(o.Time()) }
func (d Date) Equal(o Date) bool  { return d == o }

// String renders "2006-01-02".
func (d Date) String() string { return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day) }

// MarshalJSON renders "2006-01-02".
func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// UnmarshalJSON reads "2006-01-02".
func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// DateRange is a half-open date range [Start, End): End is excluded.
type DateRange struct {
	Start Date `json:"start"`
	End   Date `json:"end"`
}

// NewDateRange validates end > start.
func NewDateRange(start, end Date) (DateRange, error) {
	if !end.After(start) {
		return DateRange{}, ErrInvalidRange
	}
	return DateRange{Start: start, End: end}, nil
}

// Contains reports whether d is in [Start, End).
func (r DateRange) Contains(d Date) bool { return !d.Before(r.Start) && d.Before(r.End) }

// Overlaps reports whether two half-open date ranges share a day.
func (r DateRange) Overlaps(o DateRange) bool { return r.Start.Before(o.End) && o.Start.Before(r.End) }

// Days returns the number of days in the range.
func (r DateRange) Days() int { return int(r.End.Time().Sub(r.Start.Time()) / Day) }

// Dates lists every date in the range.
func (r DateRange) Dates() []Date {
	n := r.Days()
	out := make([]Date, 0, max(n, 0))
	for i := 0; i < n; i++ {
		out = append(out, r.Start.AddDays(i))
	}
	return out
}

// Bounds returns the instant range of the whole date range in loc.
func (r DateRange) Bounds(loc *time.Location) Range {
	return Range{Start: UTC(r.Start.StartIn(loc)), End: UTC(r.End.StartIn(loc))}
}
