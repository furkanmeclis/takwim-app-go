package usecase

import (
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/confirmations/model"
)

var ist = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

func local(day, hour, minute int) time.Time {
	return time.Date(2030, 1, day, hour, minute, 0, 0, ist)
}

func times(ts []time.Time) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.In(ist).Format("02 15:04"))
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestPlanSchedule(t *testing.T) {
	start := local(8, 14, 0)
	def := model.DefaultRule()
	cases := []struct {
		name string
		rule func(r *model.Rule)
		now  time.Time
		want []string
		ok   bool
	}{
		{"template", nil, local(1, 10, 0), []string{"07 14:00", "08 11:00"}, true},
		// Booked inside the window: the first request at once.
		{"near", nil, local(8, 9, 30), []string{"08 09:30", "08 11:00"}, true},
		// Booked after the follow-up time: one request only.
		{"after follow-up", nil, local(8, 11, 15), []string{"08 11:15"}, true},
		// No time left to answer before the deadline (12:00 − 30 min).
		{"too late", nil, local(8, 11, 31), nil, false},
		// The first request falls at night (22:00, outside 09–21): it waits
		// for the window to open on the visit day.
		{"window", func(r *model.Rule) { r.FirstRequestMinutes = 16 * 60 }, local(1, 10, 0), []string{"08 09:00", "08 11:00"}, true},
		// Three attempts: the extra follow-up is spread until the deadline
		// less the answer time.
		{"spread", func(r *model.Rule) { r.MaxAttempts = 3 }, local(1, 10, 0), []string{"07 14:00", "08 11:00", "08 11:30"}, true},
		{"no follow-up", func(r *model.Rule) { r.MaxAttempts, r.FollowUpMinutes = 1, 0 }, local(1, 10, 0), []string{"07 14:00"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := def
			if c.rule != nil {
				c.rule(&r)
			}
			got, ok := PlanSchedule(r, start, c.now, ist)
			if ok != c.ok || (ok && !eq(times(got.Attempts), c.want)) {
				t.Fatalf("got %v %v, want %v %v", times(got.Attempts), ok, c.want, c.ok)
			}
			if ok && !got.Deadline.Equal(local(8, 12, 0)) {
				t.Fatalf("deadline %v", got.Deadline)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	if err := Validate(model.DefaultRule()); err != nil {
		t.Fatalf("template invalid: %v", err)
	}
	bad := map[string]func(r *model.Rule){
		"deadline_minutes":  func(r *model.Rule) { r.DeadlineMinutes = r.FirstRequestMinutes - 10 },
		"follow_up_minutes": func(r *model.Rule) { r.FollowUpMinutes = r.FirstRequestMinutes },
		"max_attempts":      func(r *model.Rule) { r.FollowUpMinutes = 0 },
		"channels":          func(r *model.Rule) { r.Channels = nil },
		"channels.1":        func(r *model.Rule) { r.Channels = []string{"sms", "sms"} },
		"window_end_minute": func(r *model.Rule) { v := 9*60 + 30; r.WindowEndMinute = &v },
		"on_no_answer":      func(r *model.Rule) { r.OnNoAnswer = "ignore" },
		"first_request_minutes": func(r *model.Rule) {
			r.FirstRequestMinutes, r.FollowUpMinutes, r.MaxAttempts, r.DeadlineMinutes = 10, 0, 1, 0
		},
		"channels.2":               func(r *model.Rule) { r.Channels = []string{"sms", "email", "fax"} },
		"call_window_end_minute":   func(r *model.Rule) { v := 10*60 + 30; r.CallWindowEndMinute = &v },
		"call_retries":             func(r *model.Rule) { r.CallRetries = 4 },
		"call_retry_minutes":       func(r *model.Rule) { r.CallRetryMinutes = 5 },
		"call_window_start_minute": func(r *model.Rule) { v := -1; r.CallWindowStartMinute = &v },
	}
	for field, mutate := range bad {
		r := model.DefaultRule()
		mutate(&r)
		err := Validate(r)
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Details[0].Field != field {
			t.Fatalf("%s: got %v", field, err)
		}
	}
	// A follow-up that leaves no time to answer before the deadline.
	r := model.DefaultRule()
	r.FollowUpMinutes = r.DeadlineMinutes + 10
	if err := Validate(r); err == nil {
		t.Fatal("follow-up without response time saved")
	}
}

// TestValidateCalls (TEC-923): the voice call is a channel; its hours must
// overlap the request hours by an hour when both are set.
func TestValidateCalls(t *testing.T) {
	r := model.DefaultRule()
	r.Channels = []string{"voice_call", "sms"}
	if err := Validate(r); err != nil {
		t.Fatalf("voice call rule invalid: %v", err)
	}
	s, e := 20*60+30, 23*60
	r.CallWindowStartMinute, r.CallWindowEndMinute = &s, &e
	var ve *ValidationError
	if err := Validate(r); !errors.As(err, &ve) || ve.Details[0].Code != "no_overlap" {
		t.Fatalf("calls outside the request hours: %v", err)
	}
	// Without the call channel the call hours do not matter.
	r.Channels = []string{"sms"}
	if err := Validate(r); err != nil {
		t.Fatalf("call hours without calls: %v", err)
	}
	// No call hours of its own: the request hours apply.
	r.Channels = []string{"voice_call"}
	r.CallWindowStartMinute, r.CallWindowEndMinute = nil, nil
	if err := Validate(r); err != nil {
		t.Fatalf("calls in the request hours: %v", err)
	}
}

func TestWindowContains(t *testing.T) {
	s, e := 10*60, 20*60
	w := window{start: &s, end: &e, loc: ist}
	for at, want := range map[time.Time]bool{
		local(13, 9, 59): false, local(13, 10, 0): true, local(13, 19, 59): true, local(13, 20, 0): false,
	} {
		if w.contains(at) != want {
			t.Errorf("contains(%s) = %v", at, !want)
		}
	}
	if !(window{loc: ist}).contains(local(13, 3, 0)) {
		t.Error("an open window refused a time")
	}
}

func TestRequestOf(t *testing.T) {
	for key, want := range map[string][3]int{
		"confirmation.request|reservation:x|r2:a1":    {2, 1, 0},
		"confirmation.request|reservation:x|r1:a2:c3": {1, 2, 3},
		"reservation.reminder|reservation:x|remind":   {0, 0, 0},
	} {
		r, a, c := requestOf(key)
		if [3]int{r, a, c} != want {
			t.Errorf("requestOf(%q) = %d %d %d", key, r, a, c)
		}
	}
}
