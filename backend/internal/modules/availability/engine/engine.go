// Package engine is the pure slot planner of Takwim (TEC-897): given the
// opening hours of a branch, the working time and commitments of its staff
// and resources and the service lines of a booking, it answers which start
// times can be booked and who / what serves each line. It does no I/O; the
// usecase loads the inputs (panel and online channel alike) and the same
// planner will check a booking again inside the reservation transaction.
//
// Every interval is half-open [Start, End). Lines run back to back: line
// i+1 starts when the customer's time of line i ends. Buffers hold the
// staff member and the resources before / after a line without being part of
// the customer's time.
package engine

import (
	"slices"
	"sort"
	"time"
)

// Staff modes of a line (services.staff_mode).
const (
	StaffRequired = "required"
	StaffOptional = "optional"
	StaffNone     = "none"
)

// Day statuses. A day is "available" when it has at least one slot; the
// others say why it has none, in this order of precedence.
const (
	DayAvailable = "available"
	// DayClosed: the branch is closed (weekly hours or a special day).
	DayClosed = "closed"
	// DayTooFar: after the latest bookable day (max advance).
	DayTooFar = "too_far"
	// DayTooSoon: every start of the day is before the earliest bookable
	// instant (past or minimum notice).
	DayTooSoon = "too_soon"
	// DayNoStaff: a line needs a staff member and nobody who may perform it
	// works that day (shift, time off).
	DayNoStaff = "no_staff"
	// DayFull: open and staffed, but every start is taken.
	DayFull = "full"
)

// Interval is the half-open range [Start, End).
type Interval struct {
	Start time.Time
	End   time.Time
}

// Overlaps reports whether two half-open intervals share an instant.
func (i Interval) Overlaps(o Interval) bool {
	return i.Start.Before(o.End) && o.Start.Before(i.End)
}

// Within reports whether i lies inside o.
func (i Interval) Within(o Interval) bool {
	return !i.Start.Before(o.Start) && !i.End.After(o.End)
}

// Staff is one person who may serve lines of the request.
type Staff struct {
	ID int64
	// Work is when the person works in the requested branch: shifts minus
	// breaks, as instants.
	Work []Interval
	// Busy is everything that keeps the person from serving, in any branch:
	// time off, bookings and holds. A person has a single calendar (A04):
	// a booking in another branch is busy here too.
	Busy []Interval
}

// Resource is one bookable resource of the branch.
type Resource struct {
	ID   int64
	Kind string
	// Open is when the resource can be used (own hours or the branch's).
	Open []Interval
	// Busy holds closures, bookings and holds.
	Busy []Interval
}

// Line is one service of the booking.
type Line struct {
	// Key identifies the line for the caller (service uuid).
	Key          string
	Duration     time.Duration
	BufferBefore time.Duration
	BufferAfter  time.Duration
	StaffMode    string
	// Staff are the ids who may perform the line, in preference order (a
	// single id when the customer picked someone). Ties of the "any staff"
	// choice go to the person with the least booked time that day.
	Staff []int64
	// StaffDuration overrides Duration for a person (service_staff).
	StaffDuration map[int64]time.Duration
	// ResourceKinds are needed one of each for the line's held time.
	ResourceKinds []string
}

// Request is one availability question.
type Request struct {
	Location *time.Location
	// FirstDay and LastDay are the local dates asked (inclusive, any time of
	// day; only the date in Location counts).
	FirstDay time.Time
	LastDay  time.Time
	// Step is the distance between candidate starts, counted from the
	// opening of each open interval.
	Step time.Duration
	// Open are the branch's open intervals: the customer's whole time must
	// sit inside one of them.
	Open      []Interval
	Lines     []Line
	Staff     []Staff
	Resources []Resource
	// NotBefore is the earliest start (now + minimum notice); zero = none.
	NotBefore time.Time
	// NotAfter is the instant at and after which nothing starts (end of the
	// latest bookable day); zero = none.
	NotAfter time.Time
}

// Assignment is who and what serves one line of a slot.
type Assignment struct {
	Key   string
	Start time.Time
	End   time.Time
	// Held is the time the staff member and resources are held (buffers
	// included).
	Held Interval
	// StaffID is 0 when the line runs without a staff member.
	StaffID     int64
	ResourceIDs []int64
}

// Slot is one bookable start with a plan for every line.
type Slot struct {
	Start time.Time
	// End is the end of the customer's time (buffers excluded).
	End   time.Time
	Lines []Assignment
}

// Day is the answer for one local date.
type Day struct {
	// Date is the local date (yyyy-MM-dd).
	Date   string
	Status string
	Slots  []Slot
}

// Search answers every day of the request.
func Search(req Request) []Day {
	p := newPlanner(req)
	loc := p.loc
	first := dayStart(req.FirstDay, loc)
	last := dayStart(req.LastDay, loc)
	var days []Day
	for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
		days = append(days, p.day(d))
	}
	return days
}

// Check reports whether the booking can start at start and returns its plan.
// The reservation flow uses it to re-check a slot under its lock.
func Check(req Request, start time.Time) (Slot, bool) {
	p := newPlanner(req)
	if !p.inWindow(start) {
		return Slot{}, false
	}
	open, ok := p.openAt(start)
	if !ok {
		return Slot{}, false
	}
	day := dayStart(start.In(p.loc), p.loc)
	p.load = p.staffLoad(Interval{Start: day, End: day.AddDate(0, 0, 1)})
	return p.plan(start, open)
}

type planner struct {
	req       Request
	loc       *time.Location
	staff     map[int64]*Staff
	resources map[string][]*Resource
	// load is the booked time of each person on the day being planned.
	load map[int64]time.Duration
	// failed memoises (line, start, open interval) triples that have no
	// plan: feasibility of the remaining lines depends only on them, because
	// a booking never conflicts with itself.
	failed map[failKey]bool
}

type failKey struct {
	line  int
	start int64
	open  int64
}

func newPlanner(req Request) *planner {
	loc := req.Location
	if loc == nil {
		loc = time.UTC
	}
	if req.Step <= 0 {
		req.Step = 15 * time.Minute
	}
	p := &planner{
		req:       req,
		loc:       loc,
		staff:     make(map[int64]*Staff, len(req.Staff)),
		resources: map[string][]*Resource{},
		failed:    map[failKey]bool{},
	}
	for i := range req.Staff {
		p.staff[req.Staff[i].ID] = &req.Staff[i]
	}
	for i := range req.Resources {
		r := &req.Resources[i]
		p.resources[r.Kind] = append(p.resources[r.Kind], r)
	}
	return p
}

func dayStart(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

func (p *planner) inWindow(t time.Time) bool {
	if !p.req.NotBefore.IsZero() && t.Before(p.req.NotBefore) {
		return false
	}
	if !p.req.NotAfter.IsZero() && !t.Before(p.req.NotAfter) {
		return false
	}
	return true
}

func (p *planner) openAt(t time.Time) (Interval, bool) {
	for _, o := range p.req.Open {
		if !t.Before(o.Start) && t.Before(o.End) {
			return o, true
		}
	}
	return Interval{}, false
}

func (p *planner) day(d time.Time) Day {
	window := Interval{Start: d, End: d.AddDate(0, 0, 1)}
	out := Day{Date: d.Format("2006-01-02"), Slots: []Slot{}}
	var opens []Interval
	for _, o := range p.req.Open {
		if !o.Start.Before(window.Start) && o.Start.Before(window.End) {
			opens = append(opens, o)
		}
	}
	if len(opens) == 0 {
		out.Status = DayClosed
		return out
	}
	if !p.req.NotAfter.IsZero() && !window.Start.Before(p.req.NotAfter) {
		out.Status = DayTooFar
		return out
	}
	if !p.staffed(window) {
		out.Status = DayNoStaff
		return out
	}
	p.load = p.staffLoad(window)
	tried := false
	for _, o := range opens {
		for t := o.Start; t.Before(o.End); t = t.Add(p.req.Step) {
			if !p.inWindow(t) {
				continue
			}
			tried = true
			if slot, ok := p.plan(t, o); ok {
				out.Slots = append(out.Slots, slot)
			}
		}
	}
	switch {
	case len(out.Slots) > 0:
		out.Status = DayAvailable
	case !tried:
		out.Status = DayTooSoon
	default:
		out.Status = DayFull
	}
	return out
}

// staffed reports whether every line that needs a staff member has someone
// working that day.
func (p *planner) staffed(window Interval) bool {
	for _, l := range p.req.Lines {
		if l.StaffMode != StaffRequired {
			continue
		}
		found := false
		for _, id := range l.Staff {
			s := p.staff[id]
			if s == nil {
				continue
			}
			for _, w := range s.Work {
				if w.Overlaps(window) && !covered(w, s.Busy) {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// covered reports whether busy intervals cover all of w.
func covered(w Interval, busy []Interval) bool {
	cur := w.Start
	sorted := slices.Clone(busy)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start.Before(sorted[j].Start) })
	for _, b := range sorted {
		if b.Start.After(cur) {
			break
		}
		if b.End.After(cur) {
			cur = b.End
		}
		if !cur.Before(w.End) {
			return true
		}
	}
	return !cur.Before(w.End)
}

// staffLoad is the booked time of each person inside the window; "any
// staff" tries the least booked first, the line's own order breaks ties.
func (p *planner) staffLoad(window Interval) map[int64]time.Duration {
	load := make(map[int64]time.Duration, len(p.staff))
	for id, s := range p.staff {
		var total time.Duration
		for _, b := range s.Busy {
			if !b.Overlaps(window) {
				continue
			}
			start, end := b.Start, b.End
			if start.Before(window.Start) {
				start = window.Start
			}
			if end.After(window.End) {
				end = window.End
			}
			total += end.Sub(start)
		}
		load[id] = total
	}
	return load
}

func (p *planner) plan(start time.Time, open Interval) (Slot, bool) {
	lines := make([]Assignment, len(p.req.Lines))
	if !p.assign(0, start, open, lines) {
		return Slot{}, false
	}
	end := start
	if n := len(lines); n > 0 {
		end = lines[n-1].End
	}
	return Slot{Start: start, End: end, Lines: lines}, true
}

func (p *planner) assign(i int, t time.Time, open Interval, out []Assignment) bool {
	if i == len(p.req.Lines) {
		return true
	}
	key := failKey{line: i, start: t.UnixNano(), open: open.Start.UnixNano()}
	if p.failed[key] {
		return false
	}
	l := p.req.Lines[i]
	for _, staffID := range p.candidates(l) {
		d := l.Duration
		if staffID != 0 {
			if sd, ok := l.StaffDuration[staffID]; ok && sd > 0 {
				d = sd
			}
		}
		customer := Interval{Start: t, End: t.Add(d)}
		if !customer.Within(open) {
			continue
		}
		held := Interval{Start: t.Add(-l.BufferBefore), End: customer.End.Add(l.BufferAfter)}
		if staffID != 0 && !p.staffFree(staffID, held) {
			continue
		}
		resources, ok := p.pickResources(l.ResourceKinds, held, out[:i])
		if !ok {
			continue
		}
		out[i] = Assignment{Key: l.Key, Start: customer.Start, End: customer.End, Held: held, StaffID: staffID, ResourceIDs: resources}
		if p.assign(i+1, customer.End, open, out) {
			return true
		}
	}
	p.failed[key] = true
	return false
}

// candidates lists the staff choices of a line, best first; 0 = no staff.
func (p *planner) candidates(l Line) []int64 {
	switch l.StaffMode {
	case StaffNone:
		return []int64{0}
	case StaffOptional, StaffRequired:
		ids := make([]int64, 0, len(l.Staff)+1)
		for _, id := range l.Staff {
			if p.staff[id] != nil {
				ids = append(ids, id)
			}
		}
		if len(ids) > 1 {
			pos := make(map[int64]int, len(ids))
			for i, id := range ids {
				pos[id] = i
			}
			sort.SliceStable(ids, func(a, b int) bool {
				oa, ob := p.load[ids[a]], p.load[ids[b]]
				if oa != ob {
					return oa < ob
				}
				return pos[ids[a]] < pos[ids[b]]
			})
		}
		if l.StaffMode == StaffOptional {
			ids = append(ids, 0)
		}
		return ids
	}
	return nil
}

func (p *planner) staffFree(id int64, held Interval) bool {
	s := p.staff[id]
	if s == nil {
		return false
	}
	return insideAny(held, s.Work) && !overlapsAny(held, s.Busy)
}

// pickResources takes one free resource of each kind, keeping the resource
// an earlier line of the booking already uses (same chair for the whole
// visit) and otherwise the first free one in input order.
func (p *planner) pickResources(kinds []string, held Interval, earlier []Assignment) ([]int64, bool) {
	if len(kinds) == 0 {
		return []int64{}, true
	}
	used := map[int64]bool{}
	for _, a := range earlier {
		for _, id := range a.ResourceIDs {
			used[id] = true
		}
	}
	picked := make([]int64, 0, len(kinds))
	taken := map[int64]bool{}
	for _, kind := range kinds {
		var choice *Resource
		for _, r := range p.resources[kind] {
			if taken[r.ID] || !insideAny(held, r.Open) || overlapsAny(held, r.Busy) {
				continue
			}
			if used[r.ID] {
				choice = r
				break
			}
			if choice == nil {
				choice = r
			}
		}
		if choice == nil {
			return nil, false
		}
		taken[choice.ID] = true
		picked = append(picked, choice.ID)
	}
	return picked, true
}

// insideAny reports whether i lies inside the union of the intervals
// (adjacent intervals join, e.g. two shifts 09–13 and 13–18).
func insideAny(i Interval, set []Interval) bool {
	cur := i.Start
	for {
		advanced := false
		for _, s := range set {
			if !s.Start.After(cur) && s.End.After(cur) {
				cur = s.End
				advanced = true
			}
		}
		if !cur.Before(i.End) {
			return true
		}
		if !advanced {
			return false
		}
	}
}

func overlapsAny(i Interval, set []Interval) bool {
	for _, s := range set {
		if i.Overlaps(s) {
			return true
		}
	}
	return false
}
