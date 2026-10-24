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
	// DayNoClass: a group class with a timetable (TEC-970) has no session
	// that day.
	DayNoClass = "no_class"
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
	// MinCapacity–Capacity is the people range the resource seats; Zone the
	// free-text area (iç salon, bahçe); Combinable marks tables the business
	// may join for a large party. Used only when the request has a party.
	MinCapacity int
	Capacity    int
	Zone        string
	Combinable  bool
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
	// Seats > 0 makes the line a group class (TEC-904): every start is one
	// session with this many seats. The first booking of a session holds
	// its staff member and resources; later ones join it (Request.Sessions)
	// while seats are left. The booking takes Request.PartySize seats (at
	// least one); resources are not sized to the party.
	Seats int
	// Scheduled limits a group line to the sessions of its class timetable
	// (TEC-970): it is offered only at the starts of Planned, with their
	// seats, trainer and area.
	Scheduled bool
	Planned   map[int64]Planned
}

// Planned is one session of a class timetable, keyed by its start
// (UnixNano) in Line.Planned.
type Planned struct {
	// Seats of the session (the timetable's, or the single session's).
	Seats int
	// StaffID is the trainer of the session (0 = the first booking picks
	// one by the staff mode). The trainer is committed by the timetable:
	// it must be free of other commitments, not on shift.
	StaffID int64
	// ResourceIDs are the session's own resources (the area); kinds of
	// the line they do not cover are picked like any line.
	ResourceIDs []int64
}

// Session is a running group session of a line: its bookings share the
// staff member and resources of the first one.
type Session struct {
	// Key is the line key (service), Start the session start.
	Key         string
	Start       time.Time
	StaffID     int64
	ResourceIDs []int64
	// Taken is the number of seats already booked.
	Taken int
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
	// OnGrid makes Check refuse a start off the Step grid of its open
	// interval (the business's online slot step, TEC-905).
	OnGrid bool
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
	// PartySize > 0 asks every resource to seat the party (restaurant):
	// one resource whose range holds it, the smallest that fits first;
	// with Combine, combinable resources of one zone are joined when no
	// single one is large enough (the panel; online the business keeps
	// combining to itself).
	PartySize int
	Combine   bool
	// Zone narrows the resources to one area (the customer's choice).
	Zone string
	// Sessions are the running group sessions of the group lines.
	Sessions []Session
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
	// SeatsLeft is, for a group line, the free seats of the session before
	// this booking (0 for other lines).
	SeatsLeft int
	// SessionSeats is, for a group line, the seats of the session.
	SessionSeats int
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
	// A timetable fixes the starts; otherwise online starts sit on the grid.
	if p.starts != nil {
		if !p.scheduledAt(start) {
			return Slot{}, false
		}
	} else if req.OnGrid && start.Sub(open.Start)%p.req.Step != 0 {
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
	byID      map[int64]*Resource
	// starts are the session starts of a scheduled group line in order
	// (nil without a timetable: starts follow the step grid).
	starts []time.Time
	// load is the booked time of each person on the day being planned.
	load map[int64]time.Duration
	// sessions are the running group sessions by line key and start.
	sessions map[sessionKey]*Session
	// failed memoises (line, start, open interval) triples that have no
	// plan: feasibility of the remaining lines depends only on them, because
	// a booking never conflicts with itself.
	failed map[failKey]bool
}

type sessionKey struct {
	key   string
	start int64
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
		byID:      map[int64]*Resource{},
		sessions:  map[sessionKey]*Session{},
		failed:    map[failKey]bool{},
	}
	for i := range req.Sessions {
		s := &req.Sessions[i]
		p.sessions[sessionKey{key: s.Key, start: s.Start.UnixNano()}] = s
	}
	for i := range req.Staff {
		p.staff[req.Staff[i].ID] = &req.Staff[i]
	}
	for i := range req.Resources {
		r := &req.Resources[i]
		p.resources[r.Kind] = append(p.resources[r.Kind], r)
		p.byID[r.ID] = r
	}
	for _, l := range req.Lines {
		if !l.Scheduled {
			continue
		}
		p.starts = make([]time.Time, 0, len(l.Planned))
		for ns := range l.Planned {
			p.starts = append(p.starts, time.Unix(0, ns).In(loc))
		}
		sort.Slice(p.starts, func(a, b int) bool { return p.starts[a].Before(p.starts[b]) })
		break
	}
	return p
}

// scheduledAt reports whether every scheduled line has a session at t.
func (p *planner) scheduledAt(t time.Time) bool {
	for _, l := range p.req.Lines {
		if l.Scheduled {
			if _, ok := l.Planned[t.UnixNano()]; !ok {
				return false
			}
		}
	}
	return true
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
	if p.starts != nil {
		return p.scheduledDay(window, out)
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

// scheduledDay plans the sessions of a timetable that start inside the
// day and inside an open interval of the branch.
func (p *planner) scheduledDay(window Interval, out Day) Day {
	p.load = p.staffLoad(window)
	sessions, tried := false, false
	for _, t := range p.starts {
		if t.Before(window.Start) || !t.Before(window.End) {
			continue
		}
		o, ok := p.openAt(t)
		if !ok {
			continue
		}
		sessions = true
		if !p.inWindow(t) {
			continue
		}
		tried = true
		if slot, ok := p.plan(t, o); ok {
			out.Slots = append(out.Slots, slot)
		}
	}
	switch {
	case len(out.Slots) > 0:
		out.Status = DayAvailable
	case !sessions:
		out.Status = DayNoClass
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
		// A group class with running sessions can be joined even when its
		// trainers are fully booked by those sessions.
		if l.StaffMode != StaffRequired || (l.Seats > 0 && len(p.sessions) > 0) {
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
	if l.Seats > 0 {
		if p.assignGroup(i, l, t, open, out) {
			return true
		}
		p.failed[key] = true
		return false
	}
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
		resources, ok := p.pickResources(l.ResourceKinds, held, out[:i], p.req.PartySize)
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

// assignGroup plans a group line at t: it joins the running session of
// the start while seats are left, otherwise it opens a session on a free
// staff member and free resources like any line (one session per start).
// With a timetable only its sessions are planned, with their seats, and a
// session opens on its own trainer and area.
func (p *planner) assignGroup(i int, l Line, t time.Time, open Interval, out []Assignment) bool {
	need := p.req.PartySize
	if need < 1 {
		need = 1
	}
	seats := l.Seats
	var planned *Planned
	if l.Scheduled {
		pl, ok := l.Planned[t.UnixNano()]
		if !ok {
			return false
		}
		planned = &pl
		seats = pl.Seats
	}
	if s, ok := p.sessions[sessionKey{key: l.Key, start: t.UnixNano()}]; ok {
		if s.Taken+need > seats {
			return false
		}
		d := l.Duration
		if sd, ok := l.StaffDuration[s.StaffID]; ok && sd > 0 && s.StaffID != 0 {
			d = sd
		}
		customer := Interval{Start: t, End: t.Add(d)}
		if !customer.Within(open) {
			return false
		}
		held := Interval{Start: t.Add(-l.BufferBefore), End: customer.End.Add(l.BufferAfter)}
		out[i] = Assignment{
			Key: l.Key, Start: customer.Start, End: customer.End, Held: held,
			StaffID: s.StaffID, ResourceIDs: slices.Clone(s.ResourceIDs), SeatsLeft: seats - s.Taken, SessionSeats: seats,
		}
		return p.assign(i+1, customer.End, open, out)
	}
	if need > seats {
		return false
	}
	candidates := p.candidates(l)
	if planned != nil && planned.StaffID != 0 {
		if p.staff[planned.StaffID] == nil {
			return false
		}
		candidates = []int64{planned.StaffID}
	}
	for _, staffID := range candidates {
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
		if staffID != 0 {
			if planned != nil && planned.StaffID == staffID {
				if overlapsAny(held, p.staff[staffID].Busy) {
					continue
				}
			} else if !p.staffFree(staffID, held) {
				continue
			}
		}
		kinds := l.ResourceKinds
		var fixed []int64
		if planned != nil && len(planned.ResourceIDs) > 0 {
			var ok bool
			fixed, kinds, ok = p.fixedResources(planned.ResourceIDs, kinds, held)
			if !ok {
				return false
			}
		}
		resources, ok := p.pickResources(kinds, held, out[:i], 0)
		if !ok {
			continue
		}
		out[i] = Assignment{
			Key: l.Key, Start: customer.Start, End: customer.End, Held: held, StaffID: staffID,
			ResourceIDs: append(fixed, resources...), SeatsLeft: seats, SessionSeats: seats,
		}
		if p.assign(i+1, customer.End, open, out) {
			return true
		}
	}
	return false
}

// fixedResources checks the resources a timetable gives a session: each
// must be loaded (active, of a kind of the line), open and free for the
// held time. It returns them and the kinds still to pick.
func (p *planner) fixedResources(ids []int64, kinds []string, held Interval) ([]int64, []string, bool) {
	rest := slices.Clone(kinds)
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		r := p.byID[id]
		if r == nil || !insideAny(held, r.Open) || overlapsAny(held, r.Busy) {
			return nil, nil, false
		}
		if k := slices.Index(rest, r.Kind); k >= 0 {
			rest = slices.Delete(rest, k, k+1)
		}
		out = append(out, id)
	}
	return out, rest, true
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
// visit) and otherwise the first free one in input order. With a party the
// resource must seat it (see Request.PartySize) and a kind may take several
// joined resources.
func (p *planner) pickResources(kinds []string, held Interval, earlier []Assignment, party int) ([]int64, bool) {
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
		var free []*Resource
		for _, r := range p.resources[kind] {
			if taken[r.ID] || !insideAny(held, r.Open) || overlapsAny(held, r.Busy) {
				continue
			}
			if p.req.Zone != "" && r.Zone != p.req.Zone {
				continue
			}
			free = append(free, r)
		}
		var choice []*Resource
		if party > 0 {
			choice = p.seat(free, used, party)
		} else {
			for _, r := range free {
				if used[r.ID] {
					choice = []*Resource{r}
					break
				}
				if choice == nil {
					choice = []*Resource{r}
				}
			}
		}
		if choice == nil {
			return nil, false
		}
		for _, r := range choice {
			taken[r.ID] = true
			picked = append(picked, r.ID)
		}
	}
	return picked, true
}

// MaxJoined is the most resources joined for one party.
const MaxJoined = 4

// seat picks the resources for the party among free ones: a resource an
// earlier line uses when it still fits, else the single best fit (smallest
// capacity, then the highest minimum, then input order), else with Combine
// the fewest combinable resources of one zone whose capacities add up to
// the party (largest first).
func (p *planner) seat(free []*Resource, used map[int64]bool, party int) []*Resource {
	fits := func(r *Resource) bool { return r.MinCapacity <= party && party <= r.Capacity }
	var best *Resource
	for _, r := range free {
		if !fits(r) {
			continue
		}
		if used[r.ID] {
			return []*Resource{r}
		}
		if best == nil || r.Capacity < best.Capacity || (r.Capacity == best.Capacity && r.MinCapacity > best.MinCapacity) {
			best = r
		}
	}
	if best != nil {
		return []*Resource{best}
	}
	if !p.req.Combine {
		return nil
	}
	zones := map[string][]*Resource{}
	var order []string
	for _, r := range free {
		if !r.Combinable {
			continue
		}
		if _, ok := zones[r.Zone]; !ok {
			order = append(order, r.Zone)
		}
		zones[r.Zone] = append(zones[r.Zone], r)
	}
	var out []*Resource
	for _, zone := range order {
		list := slices.Clone(zones[zone])
		sort.SliceStable(list, func(a, b int) bool { return list[a].Capacity > list[b].Capacity })
		seats := 0
		var group []*Resource
		for _, r := range list {
			if len(group) == MaxJoined || seats >= party {
				break
			}
			group = append(group, r)
			seats += r.Capacity
		}
		if seats >= party && len(group) > 1 && (out == nil || len(group) < len(out)) {
			out = group
		}
	}
	return out
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
