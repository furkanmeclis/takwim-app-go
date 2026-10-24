package usecase

import (
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/confirmations/model"
)

// minGap keeps two requests of a booking apart.
const minGap = 15 * time.Minute

// Schedule is the planned confirmation of one visit.
type Schedule struct {
	Attempts []time.Time
	Deadline time.Time
}

// window is a contact window in a time zone (nil bounds = any time).
type window struct {
	start, end *int
	loc        *time.Location
}

func (w window) open() bool { return w.start == nil || w.end == nil }

// shift moves t into the window: unchanged inside it, else the next
// opening (the same day before it opens, the next day after it closes).
func (w window) shift(t time.Time) time.Time {
	if w.open() {
		return t
	}
	local := t.In(w.loc)
	minute := local.Hour()*60 + local.Minute()
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, w.loc)
	switch {
	case minute < *w.start:
		return day.Add(time.Duration(*w.start) * time.Minute)
	case minute >= *w.end:
		return day.AddDate(0, 0, 1).Add(time.Duration(*w.start) * time.Minute)
	}
	return t
}

// PlanSchedule plans the requests of a visit starting at start, booked or
// moved at now, under rule (times of the window in loc). ok is false when
// no request can leave the customer time to answer before the deadline
// (a booking made at the last moment needs no confirmation).
//
//   - The first request goes out FirstRequestMinutes before the start, or
//     at once for a booking made later (inside the contact window; when
//     waiting for the window would leave no time to answer, at once).
//   - Follow-ups (MaxAttempts − 1) start FollowUpMinutes before the start
//     and are spread evenly until the deadline less the answer time. A
//     follow-up that falls before the first request, outside the window
//     past the deadline, or too close to the previous one is left out.
func PlanSchedule(rule model.Rule, start, now time.Time, loc *time.Location) (Schedule, bool) {
	if loc == nil {
		loc = time.UTC
	}
	response := time.Duration(model.MinResponseMinutes) * time.Minute
	deadline := start.Add(-time.Duration(rule.DeadlineMinutes) * time.Minute)
	latest := deadline.Add(-response)
	if now.After(latest) {
		return Schedule{}, false
	}
	w := window{start: rule.WindowStartMinute, end: rule.WindowEndMinute, loc: loc}
	first := start.Add(-time.Duration(rule.FirstRequestMinutes) * time.Minute)
	if first.Before(now) {
		first = now
	}
	if shifted := w.shift(first); !shifted.After(latest) {
		first = shifted
	}
	out := Schedule{Attempts: []time.Time{first.UTC()}, Deadline: deadline.UTC()}
	followUps := rule.MaxAttempts - 1
	if followUps <= 0 || rule.FollowUpMinutes <= 0 {
		return out, true
	}
	from := start.Add(-time.Duration(rule.FollowUpMinutes) * time.Minute)
	step := time.Duration(0)
	if followUps > 1 && latest.After(from) {
		step = latest.Sub(from) / time.Duration(followUps-1)
	}
	prev := first
	for i := 0; i < followUps; i++ {
		t := w.shift(from.Add(time.Duration(i) * step))
		if t.After(latest) || t.Sub(prev) < minGap {
			continue
		}
		out.Attempts = append(out.Attempts, t.UTC())
		prev = t
	}
	return out, true
}
