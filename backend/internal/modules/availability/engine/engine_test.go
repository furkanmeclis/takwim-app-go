package engine

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

var ist = mustLoc("Europe/Istanbul")

func mustLoc(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// monday is the date most tests plan on.
var monday = time.Date(2026, 10, 26, 0, 0, 0, 0, ist)

// clock is "HH:MM" on monday (or on another day with dayOffset).
func clock(s string, dayOffset ...int) time.Time {
	m, ok := ParseClock(s)
	if !ok {
		panic(s)
	}
	d := monday
	if len(dayOffset) > 0 {
		d = d.AddDate(0, 0, dayOffset[0])
	}
	return at(d, m, ist)
}

func iv(start, end string, dayOffset ...int) Interval {
	return Interval{Start: clock(start, dayOffset...), End: clock(end, dayOffset...)}
}

func weekdays(open, close string) Weekly {
	w := Weekly{}
	for d := time.Monday; d <= time.Saturday; d++ {
		w[d] = Hours{Open: open, Close: close}
	}
	w[time.Sunday] = Hours{Closed: true}
	return w
}

func shiftsEveryDay(shift Shift) WeeklyShifts {
	w := WeeklyShifts{}
	for d := time.Sunday; d <= time.Saturday; d++ {
		w[d] = []Shift{shift}
	}
	return w
}

// base is one branch open 09:00–18:00, one service of 30 minutes that needs
// staff member 1 (working 09:00–18:00), steps of 30 minutes, monday only.
func base() Request {
	r := Request{
		Location: ist,
		FirstDay: monday,
		LastDay:  monday,
		Step:     30 * time.Minute,
		Lines: []Line{{
			Key: "cut", Duration: 30 * time.Minute, StaffMode: StaffRequired, Staff: []int64{1},
		}},
	}
	r.Open = OpenIntervals(ist, monday, monday.AddDate(0, 0, 6), weekdays("09:00", "18:00"), nil)
	r.Staff = []Staff{{ID: 1, Work: WorkIntervals(ist, monday, monday.AddDate(0, 0, 6), shiftsEveryDay(Shift{Span: Span{"09:00", "18:00"}}))}}
	return r
}

func starts(d Day) string {
	parts := make([]string, 0, len(d.Slots))
	for _, s := range d.Slots {
		parts = append(parts, s.Start.In(ist).Format("15:04"))
	}
	return strings.Join(parts, " ")
}

func only(t *testing.T, req Request) Day {
	t.Helper()
	days := Search(req)
	if len(days) != 1 {
		t.Fatalf("days = %d, want 1", len(days))
	}
	return days[0]
}

func TestSingleServiceFollowsShiftAndBreak(t *testing.T) {
	req := base()
	req.Staff[0].Work = WorkIntervals(ist, monday, monday, shiftsEveryDay(Shift{
		Span: Span{"09:00", "12:00"}, Breaks: []Span{{"10:00", "10:30"}},
	}))
	d := only(t, req)
	if d.Status != DayAvailable || d.Date != "2026-10-26" {
		t.Fatalf("day = %+v", d)
	}
	if got, want := starts(d), "09:00 09:30 10:30 11:00 11:30"; got != want {
		t.Fatalf("starts = %q, want %q", got, want)
	}
	s := d.Slots[0]
	if !s.End.Equal(clock("09:30")) || s.Lines[0].StaffID != 1 || len(s.Lines[0].ResourceIDs) != 0 {
		t.Fatalf("slot = %+v", s)
	}
}

func TestCustomerTimeStaysInsideBranchHours(t *testing.T) {
	req := base()
	req.Open = []Interval{iv("09:00", "10:00")}
	req.Lines[0].Duration = 45 * time.Minute
	req.Step = 15 * time.Minute
	if got, want := starts(only(t, req)), "09:00 09:15"; got != want {
		t.Fatalf("starts = %q, want %q", got, want)
	}
}

func TestClosedDaysAndSpecialDays(t *testing.T) {
	req := base()
	req.LastDay = monday.AddDate(0, 0, 6) // through sunday
	exceptions := map[string]Hours{
		"2026-10-27": {Closed: true},                         // tuesday: holiday
		"2026-10-28": {Open: "13:00", Close: "14:00"},        // wednesday: short day
		"2026-10-31": {Open: "10:00", Close: "10:30"},        // saturday: overrides weekly
		"2026-11-01": {Closed: false, Open: "x", Close: "y"}, // invalid hours = closed
	}
	req.Open = OpenIntervals(ist, req.FirstDay, req.LastDay, weekdays("09:00", "18:00"), exceptions)
	days := Search(req)
	if len(days) != 7 {
		t.Fatalf("days = %d", len(days))
	}
	status := map[string]string{}
	for _, d := range days {
		status[d.Date] = d.Status
	}
	if status["2026-10-27"] != DayClosed || status["2026-11-01"] != DayClosed {
		t.Fatalf("status = %v", status)
	}
	if got := starts(days[2]); got != "13:00 13:30" {
		t.Fatalf("wednesday starts = %q", got)
	}
	if got := starts(days[5]); got != "10:00" {
		t.Fatalf("saturday starts = %q", got)
	}
}

func TestTimeOff(t *testing.T) {
	t.Run("whole day", func(t *testing.T) {
		req := base()
		req.Staff[0].Busy = []Interval{{Start: monday, End: monday.AddDate(0, 0, 1)}}
		if d := only(t, req); d.Status != DayNoStaff {
			t.Fatalf("status = %s", d.Status)
		}
	})
	t.Run("part of the day", func(t *testing.T) {
		req := base()
		req.Open = []Interval{iv("09:00", "12:00")}
		req.Staff[0].Busy = []Interval{iv("09:45", "11:00")}
		if got, want := starts(only(t, req)), "09:00 11:00 11:30"; got != want {
			t.Fatalf("starts = %q, want %q", got, want)
		}
	})
	t.Run("no shift that day", func(t *testing.T) {
		req := base()
		req.Staff[0].Work = nil
		if d := only(t, req); d.Status != DayNoStaff {
			t.Fatalf("status = %s", d.Status)
		}
	})
}

// A04: a person working in two branches has one calendar. A booking in the
// other branch reaches this branch as busy time, so the same hours are never
// offered twice.
func TestA04OneCalendarAcrossBranches(t *testing.T) {
	req := base()
	req.Open = []Interval{iv("09:00", "12:00")}
	otherBranchBooking := iv("10:00", "11:00")
	req.Staff[0].Busy = []Interval{otherBranchBooking}
	d := only(t, req)
	if got, want := starts(d), "09:00 09:30 11:00 11:30"; got != want {
		t.Fatalf("starts = %q, want %q", got, want)
	}
	for _, s := range d.Slots {
		if (Interval{Start: s.Start, End: s.End}).Overlaps(otherBranchBooking) {
			t.Fatalf("slot %v overlaps the other branch", s.Start)
		}
	}
	if _, ok := Check(req, clock("10:30")); ok {
		t.Fatal("Check accepted 10:30")
	}
}

func TestBuffersHoldStaffButNotCustomer(t *testing.T) {
	req := base()
	req.Open = []Interval{iv("09:00", "12:00")}
	req.Staff[0].Work = []Interval{iv("09:00", "12:00")}
	req.Step = 15 * time.Minute
	req.Lines[0].BufferBefore = 15 * time.Minute
	req.Lines[0].BufferAfter = 15 * time.Minute
	req.Staff[0].Busy = []Interval{iv("10:30", "10:45")}
	d := only(t, req)
	// held = start-15 .. start+45: inside 09:00–12:00 and clear of 10:30–10:45.
	if got, want := starts(d), "09:15 09:30 09:45 11:00 11:15"; got != want {
		t.Fatalf("starts = %q, want %q", got, want)
	}
	if !d.Slots[0].End.Equal(clock("09:45")) {
		t.Fatalf("customer end = %v", d.Slots[0].End)
	}
}

func TestAnyStaffPicksSomeoneFree(t *testing.T) {
	req := base()
	req.Open = []Interval{iv("09:00", "11:00")}
	work := []Interval{iv("09:00", "11:00")}
	req.Staff = []Staff{
		{ID: 1, Work: work, Busy: []Interval{iv("09:00", "10:00")}},
		{ID: 2, Work: work, Busy: []Interval{iv("10:00", "10:30")}},
	}
	req.Lines[0].Staff = []int64{1, 2}
	d := only(t, req)
	got := map[string]int64{}
	for _, s := range d.Slots {
		got[s.Start.In(ist).Format("15:04")] = s.Lines[0].StaffID
	}
	want := map[string]int64{"09:00": 2, "09:30": 2, "10:00": 1, "10:30": 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("assignments = %v, want %v", got, want)
	}
}

func TestAnyStaffPrefersTheLeastBooked(t *testing.T) {
	req := base()
	work := []Interval{iv("09:00", "18:00")}
	req.Staff = []Staff{
		{ID: 1, Work: work, Busy: []Interval{iv("15:00", "17:00")}},
		{ID: 2, Work: work, Busy: []Interval{iv("16:00", "17:00")}},
		{ID: 3, Work: work, Busy: []Interval{iv("12:00", "12:30")}},
	}
	req.Lines[0].Staff = []int64{1, 2, 3}
	d := only(t, req)
	if d.Slots[0].Lines[0].StaffID != 3 {
		t.Fatalf("09:00 went to %d, want 3", d.Slots[0].Lines[0].StaffID)
	}
	for _, s := range d.Slots {
		if s.Start.Equal(clock("12:00")) && s.Lines[0].StaffID != 2 {
			t.Fatalf("12:00 went to %d, want 2", s.Lines[0].StaffID)
		}
	}
}

func TestPinnedStaffIsTheOnlyCandidate(t *testing.T) {
	req := base()
	work := []Interval{iv("09:00", "10:00")}
	req.Open = []Interval{iv("09:00", "10:00")}
	req.Staff = []Staff{{ID: 1, Work: work, Busy: []Interval{iv("09:00", "09:30")}}, {ID: 2, Work: work}}
	req.Lines[0].Staff = []int64{1}
	if got := starts(only(t, req)); got != "09:30" {
		t.Fatalf("starts = %q", got)
	}
}

func TestPersonalDuration(t *testing.T) {
	req := base()
	req.Open = []Interval{iv("09:00", "12:00")}
	work := []Interval{iv("09:00", "12:00")}
	req.Staff = []Staff{{ID: 1, Work: work}, {ID: 2, Work: work, Busy: []Interval{iv("09:00", "11:30")}}}
	req.Lines[0].Staff = []int64{1, 2}
	req.Lines[0].StaffDuration = map[int64]time.Duration{1: 60 * time.Minute}
	d := only(t, req)
	// Staff 1 needs an hour (last start 11:00); staff 2 is free 11:30–12:00.
	if got, want := starts(d), "09:00 09:30 10:00 10:30 11:00 11:30"; got != want {
		t.Fatalf("starts = %q, want %q", got, want)
	}
	last := d.Slots[len(d.Slots)-1]
	if last.Lines[0].StaffID != 2 || !last.End.Equal(clock("12:00")) {
		t.Fatalf("11:30 = %+v", last)
	}
	if !d.Slots[0].End.Equal(clock("10:00")) {
		t.Fatalf("09:00 ends %v", d.Slots[0].End)
	}
}

func TestResources(t *testing.T) {
	t.Run("one of each kind", func(t *testing.T) {
		req := base()
		req.Open = []Interval{iv("09:00", "11:00")}
		open := []Interval{iv("09:00", "11:00")}
		req.Lines[0].ResourceKinds = []string{"room", "device"}
		req.Resources = []Resource{
			{ID: 10, Kind: "room", Open: open, Busy: []Interval{iv("09:00", "09:30")}},
			{ID: 11, Kind: "room", Open: open, Busy: []Interval{iv("10:00", "11:00")}},
			{ID: 20, Kind: "device", Open: open, Busy: []Interval{iv("10:30", "11:00")}},
		}
		d := only(t, req)
		if got, want := starts(d), "09:00 09:30 10:00"; got != want {
			t.Fatalf("starts = %q, want %q", got, want)
		}
		if ids := d.Slots[0].Lines[0].ResourceIDs; !reflect.DeepEqual(ids, []int64{11, 20}) {
			t.Fatalf("09:00 resources = %v", ids)
		}
		if ids := d.Slots[2].Lines[0].ResourceIDs; !reflect.DeepEqual(ids, []int64{10, 20}) {
			t.Fatalf("10:00 resources = %v", ids)
		}
	})
	t.Run("own hours and closures", func(t *testing.T) {
		req := base()
		req.Open = []Interval{iv("09:00", "12:00")}
		req.Lines[0].StaffMode = StaffNone
		req.Lines[0].Staff = nil
		req.Lines[0].ResourceKinds = []string{"bay"}
		req.Resources = []Resource{{ID: 1, Kind: "bay", Open: []Interval{iv("10:00", "12:00")}, Busy: []Interval{iv("11:00", "11:30")}}}
		d := only(t, req)
		if got, want := starts(d), "10:00 10:30 11:30"; got != want {
			t.Fatalf("starts = %q, want %q", got, want)
		}
		if d.Slots[0].Lines[0].StaffID != 0 {
			t.Fatal("staff assigned to a no-staff service")
		}
	})
	t.Run("missing kind", func(t *testing.T) {
		req := base()
		req.Lines[0].ResourceKinds = []string{"chair"}
		if d := only(t, req); d.Status != DayFull || len(d.Slots) != 0 {
			t.Fatalf("day = %+v", d)
		}
	})
	t.Run("buffers hold the resource", func(t *testing.T) {
		req := base()
		req.Open = []Interval{iv("09:00", "11:00")}
		req.Lines[0].StaffMode = StaffNone
		req.Lines[0].ResourceKinds = []string{"room"}
		req.Lines[0].BufferAfter = 30 * time.Minute
		req.Resources = []Resource{{ID: 1, Kind: "room", Open: []Interval{iv("09:00", "11:00")}, Busy: []Interval{iv("10:00", "10:30")}}}
		// 09:30 would clean until 10:30; 10:30 cleans until 11:30 (closed).
		if got := starts(only(t, req)); got != "09:00" {
			t.Fatalf("starts = %q", got)
		}
	})
}

func TestOptionalStaff(t *testing.T) {
	req := base()
	req.Open = []Interval{iv("09:00", "10:00")}
	req.Lines[0].StaffMode = StaffOptional
	req.Staff[0].Busy = []Interval{iv("09:30", "10:00")}
	d := only(t, req)
	if got := starts(d); got != "09:00 09:30" {
		t.Fatalf("starts = %q", got)
	}
	if d.Slots[0].Lines[0].StaffID != 1 || d.Slots[1].Lines[0].StaffID != 0 {
		t.Fatalf("staff = %d, %d", d.Slots[0].Lines[0].StaffID, d.Slots[1].Lines[0].StaffID)
	}
}

func TestRequiredStaffWithoutCandidates(t *testing.T) {
	req := base()
	req.Lines[0].Staff = nil
	if d := only(t, req); d.Status != DayNoStaff {
		t.Fatalf("status = %s", d.Status)
	}
	req.Lines[0].Staff = []int64{99} // unknown id
	if d := only(t, req); d.Status != DayNoStaff {
		t.Fatalf("status = %s", d.Status)
	}
}

func TestBundleRunsLinesBackToBack(t *testing.T) {
	req := base()
	req.Open = []Interval{iv("09:00", "12:00")}
	work := []Interval{iv("09:00", "12:00")}
	req.Staff = []Staff{{ID: 1, Work: work}, {ID: 2, Work: work, Busy: []Interval{iv("09:00", "10:00")}}}
	req.Resources = []Resource{{ID: 7, Kind: "room", Open: work}}
	req.Lines = []Line{
		{Key: "wash", Duration: 30 * time.Minute, StaffMode: StaffRequired, Staff: []int64{1}},
		{Key: "color", Duration: 60 * time.Minute, StaffMode: StaffRequired, Staff: []int64{2}, ResourceKinds: []string{"room"}},
	}
	d := only(t, req)
	// colour (staff 2) can start at 10:00 at the earliest and must end by 12:00.
	if got, want := starts(d), "09:30 10:00 10:30"; got != want {
		t.Fatalf("starts = %q, want %q", got, want)
	}
	s := d.Slots[0]
	if len(s.Lines) != 2 || s.Lines[0].Key != "wash" || s.Lines[1].Key != "color" {
		t.Fatalf("lines = %+v", s.Lines)
	}
	if !s.Lines[1].Start.Equal(clock("10:00")) || !s.End.Equal(clock("11:00")) || s.Lines[1].StaffID != 2 ||
		!reflect.DeepEqual(s.Lines[1].ResourceIDs, []int64{7}) {
		t.Fatalf("slot = %+v", s)
	}
}

func TestBundleSameStaffKeepsTheirBuffersInside(t *testing.T) {
	req := base()
	req.Open = []Interval{iv("09:00", "10:30")}
	req.Staff[0].Work = []Interval{iv("09:00", "10:30")}
	req.Lines = []Line{
		{Key: "a", Duration: 30 * time.Minute, BufferAfter: 15 * time.Minute, StaffMode: StaffRequired, Staff: []int64{1}},
		{Key: "b", Duration: 30 * time.Minute, BufferBefore: 15 * time.Minute, StaffMode: StaffRequired, Staff: []int64{1}},
	}
	// The booking never conflicts with itself: a's cleaning overlaps b's
	// preparation and both belong to the same visit. 09:30 holds the person
	// until 10:30 (b's customer time ends there; no buffer after b).
	if got := starts(only(t, req)); got != "09:00 09:30" {
		t.Fatalf("starts = %q", got)
	}
}

// The quicker person on the first line is the only way to reach the second
// line's single free window: the planner has to try more than its first
// choice.
func TestBundleBacktracksOverPersonalDurations(t *testing.T) {
	req := base()
	req.Open = []Interval{iv("10:00", "12:00")}
	work := []Interval{iv("10:00", "12:00")}
	req.Staff = []Staff{
		{ID: 1, Work: work},
		{ID: 2, Work: work, Busy: []Interval{iv("11:30", "11:45")}}, // more booked: tried second
		{ID: 3, Work: work, Busy: []Interval{iv("10:00", "10:30"), iv("11:00", "12:00")}},
	}
	req.Lines = []Line{
		{Key: "a", Duration: 30 * time.Minute, StaffMode: StaffRequired, Staff: []int64{1, 2},
			StaffDuration: map[int64]time.Duration{1: 60 * time.Minute}},
		{Key: "b", Duration: 30 * time.Minute, StaffMode: StaffRequired, Staff: []int64{3}},
	}
	slot, ok := Check(req, clock("10:00"))
	if !ok {
		t.Fatal("10:00 refused")
	}
	if slot.Lines[0].StaffID != 2 || !slot.Lines[1].Start.Equal(clock("10:30")) || !slot.End.Equal(clock("11:00")) {
		t.Fatalf("slot = %+v", slot)
	}
}

func TestEarliestAndLatestBooking(t *testing.T) {
	req := base()
	req.FirstDay = monday.AddDate(0, 0, -1) // sunday: closed
	req.LastDay = monday.AddDate(0, 0, 3)
	req.Open = OpenIntervals(ist, req.FirstDay, req.LastDay, weekdays("09:00", "12:00"), nil)
	req.NotBefore = clock("10:10")                          // now + notice
	req.NotAfter = monday.AddDate(0, 0, 2)                  // wednesday is the first day too far
	req.Staff[0].Busy = []Interval{iv("09:00", "12:00", 1)} // tuesday: booked out
	days := Search(req)
	got := []string{}
	for _, d := range days {
		got = append(got, d.Date+"="+d.Status)
	}
	want := []string{
		"2026-10-25=closed", "2026-10-26=available", "2026-10-27=full", "2026-10-28=too_far", "2026-10-29=too_far",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("days = %v, want %v", got, want)
	}
	if s := starts(days[1]); s != "10:30 11:00 11:30" {
		t.Fatalf("monday starts = %q", s)
	}

	req.NotBefore = clock("11:45")
	req.LastDay = monday
	req.FirstDay = monday
	if d := only(t, req); d.Status != DayTooSoon {
		t.Fatalf("status = %s", d.Status)
	}
}

func TestFullDay(t *testing.T) {
	req := base()
	req.Open = []Interval{iv("09:00", "10:00")}
	req.Staff[0].Busy = []Interval{iv("09:00", "09:45")}
	if d := only(t, req); d.Status != DayFull {
		t.Fatalf("status = %s", d.Status)
	}
}

func TestCheck(t *testing.T) {
	req := base()
	req.Open = []Interval{iv("09:00", "12:00")}
	if _, ok := Check(req, clock("08:30")); ok {
		t.Fatal("before opening accepted")
	}
	if _, ok := Check(req, clock("11:45")); ok {
		t.Fatal("past closing accepted")
	}
	slot, ok := Check(req, clock("09:10")) // off the step grid: still a valid start
	if !ok || !slot.End.Equal(clock("09:40")) {
		t.Fatalf("09:10 = %+v, %v", slot, ok)
	}
	req.NotBefore = clock("10:00")
	if _, ok := Check(req, clock("09:30")); ok {
		t.Fatal("start before notice accepted")
	}
	req.NotAfter = clock("11:00")
	if _, ok := Check(req, clock("11:00")); ok {
		t.Fatal("start after the latest day accepted")
	}
}

func TestDefaultStepAndLocation(t *testing.T) {
	req := base()
	req.Step = 0
	req.Open = []Interval{iv("09:00", "10:00")}
	if got := starts(only(t, req)); got != "09:00 09:15 09:30" {
		t.Fatalf("starts = %q", got)
	}
	req.Location = nil
	if days := Search(req); len(days) == 0 {
		t.Fatal("no days without a location")
	}
}

// Many staff and lines stay fast: failed (line, start, open) triples are
// memoised, so an impossible bundle does not try every staff combination.
func TestManyLinesStayFast(t *testing.T) {
	for _, tc := range []struct {
		busyUntil string
		status    string
	}{{"17:00", DayAvailable}, {"17:30", DayFull}} {
		req := base()
		req.Step = 5 * time.Minute
		req.Staff = nil
		req.Lines = nil
		var ids []int64
		durations := map[int64]time.Duration{}
		for i := int64(1); i <= 30; i++ {
			ids = append(ids, i)
			durations[i] = time.Duration(5+i) * time.Minute
			req.Staff = append(req.Staff, Staff{ID: i, Work: []Interval{iv("09:00", "18:00")}, Busy: []Interval{iv("09:00", tc.busyUntil)}})
		}
		for i := 0; i < 8; i++ {
			req.Lines = append(req.Lines, Line{Key: fmt.Sprint(i), Duration: 10 * time.Minute, StaffMode: StaffRequired, Staff: ids, StaffDuration: durations})
		}
		started := time.Now()
		d := only(t, req)
		if elapsed := time.Since(started); elapsed > 2*time.Second {
			t.Fatalf("busy until %s took %v", tc.busyUntil, elapsed)
		}
		if d.Status != tc.status {
			t.Fatalf("busy until %s: status = %s, want %s", tc.busyUntil, d.Status, tc.status)
		}
	}
}

func TestAnyStaffTiesFollowTheLineOrder(t *testing.T) {
	req := base()
	work := []Interval{iv("09:00", "18:00")}
	req.Staff = []Staff{{ID: 1, Work: work}, {ID: 2, Work: work}}
	req.Lines[0].Staff = []int64{2, 1}
	if d := only(t, req); d.Slots[0].Lines[0].StaffID != 2 {
		t.Fatalf("09:00 went to %d, want 2", d.Slots[0].Lines[0].StaffID)
	}
}

// TestPartySeating (TEC-903): a party gets the smallest table that seats it
// in the chosen zone; the panel joins combinable tables of one zone when no
// single table is large enough, online does not.
func TestPartySeating(t *testing.T) {
	table := func(id int64, minCap, capacity int, zone string, combinable bool, busy ...Interval) Resource {
		return Resource{ID: id, Kind: "table", Open: []Interval{iv("09:00", "11:00")}, Busy: busy,
			MinCapacity: minCap, Capacity: capacity, Zone: zone, Combinable: combinable}
	}
	req := func(party int, zone string, combine bool, tables ...Resource) Request {
		r := base()
		r.Open = []Interval{iv("09:00", "11:00")}
		r.Lines[0].StaffMode = StaffNone
		r.Lines[0].Staff = nil
		r.Lines[0].ResourceKinds = []string{"table"}
		r.Resources = tables
		r.PartySize, r.Zone, r.Combine = party, zone, combine
		return r
	}
	first := func(d Day) []int64 {
		if len(d.Slots) == 0 {
			return nil
		}
		return d.Slots[0].Lines[0].ResourceIDs
	}
	tables := []Resource{
		table(1, 1, 6, "salon", false),
		table(2, 1, 2, "salon", true),
		table(3, 3, 4, "salon", true),
		table(4, 1, 4, "bahçe", true),
		table(5, 1, 4, "bahçe", true),
	}
	if ids := first(only(t, req(2, "", false, tables...))); !reflect.DeepEqual(ids, []int64{2}) {
		t.Fatalf("party of 2 = %v, want the two-seat table", ids)
	}
	if ids := first(only(t, req(4, "", false, tables...))); !reflect.DeepEqual(ids, []int64{3}) {
		t.Fatalf("party of 4 = %v, want the smallest four-seat table with the highest minimum", ids)
	}
	if ids := first(only(t, req(4, "bahçe", false, tables...))); !reflect.DeepEqual(ids, []int64{4}) {
		t.Fatalf("party of 4 in the garden = %v", ids)
	}
	if ids := first(only(t, req(1, "", false, table(3, 3, 4, "salon", false)))); ids != nil {
		t.Fatalf("party of 1 at a table for 3+ = %v, want none", ids)
	}
	// Eight people: no single table; online offers nothing, the panel joins
	// the garden tables (the salon's combinable ones seat only six).
	if d := only(t, req(8, "", false, tables...)); d.Status != DayFull {
		t.Fatalf("online party of 8 = %+v, want full", d)
	}
	if ids := first(only(t, req(8, "", true, tables...))); !reflect.DeepEqual(ids, []int64{4, 5}) {
		t.Fatalf("panel party of 8 = %v, want the joined garden tables", ids)
	}
	// A busy table is skipped: the next fit takes the party.
	busy := []Resource{table(2, 1, 2, "salon", false, iv("09:00", "11:00")), table(3, 1, 4, "salon", false)}
	if ids := first(only(t, req(2, "", false, busy...))); !reflect.DeepEqual(ids, []int64{3}) {
		t.Fatalf("busy two-seat table: got %v", ids)
	}
}

// TestGroupSessions (TEC-904): a group class seats several bookings in one
// session. A new session needs a free trainer and area; a running session
// is joined on its own trainer and area while seats are left, and is not
// offered once full. A party larger than the free seats is refused.
func TestGroupSessions(t *testing.T) {
	req := func(party int, sessions ...Session) Request {
		r := base()
		r.Open = []Interval{iv("09:00", "11:00")}
		r.Step = time.Hour
		r.Lines[0] = Line{Key: "pilates", Duration: time.Hour, StaffMode: StaffRequired, Staff: []int64{1}, ResourceKinds: []string{"area"}, Seats: 3}
		// The 09:00 session holds the trainer and the studio already.
		r.Staff[0].Busy = []Interval{iv("09:00", "10:00")}
		r.Resources = []Resource{{ID: 7, Kind: "area", Open: []Interval{iv("09:00", "11:00")}, Busy: []Interval{iv("09:00", "10:00")}}}
		r.PartySize = party
		r.Sessions = sessions
		return r
	}
	running := Session{Key: "pilates", Start: clock("09:00"), StaffID: 1, ResourceIDs: []int64{7}, Taken: 1}
	d := only(t, req(0, running))
	if got := starts(d); got != "09:00 10:00" {
		t.Fatalf("starts = %q, want the running session and a new one", got)
	}
	joined := d.Slots[0].Lines[0]
	if joined.StaffID != 1 || !reflect.DeepEqual(joined.ResourceIDs, []int64{7}) || joined.SeatsLeft != 2 {
		t.Fatalf("joined = %+v", joined)
	}
	if fresh := d.Slots[1].Lines[0]; fresh.SeatsLeft != 3 {
		t.Fatalf("new session seats left = %d, want 3", fresh.SeatsLeft)
	}
	full := running
	full.Taken = 3
	if got := starts(only(t, req(0, full))); got != "10:00" {
		t.Fatalf("starts with a full session = %q, want 10:00", got)
	}
	if got := starts(only(t, req(2, running))); got != "09:00 10:00" {
		t.Fatalf("party of 2 = %q", got)
	}
	if got := starts(only(t, req(3, running))); got != "10:00" {
		t.Fatalf("party of 3 with two seats left = %q, want only the new session", got)
	}
	if got := starts(only(t, req(4))); got != "" {
		t.Fatalf("party of 4 in a class of 3 = %q, want none", got)
	}
	// Without the session record the busy trainer blocks 09:00 as usual.
	if got := starts(only(t, req(0))); got != "10:00" {
		t.Fatalf("starts without the session = %q", got)
	}
	if _, ok := Check(req(0, running), clock("09:00")); !ok {
		t.Fatal("check of the running session failed")
	}
}
