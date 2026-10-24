package usecase

import (
	"errors"
	"slices"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
)

func TestNormalizeBooking(t *testing.T) {
	t.Parallel()
	s := model.Defaults()
	s.Booking = model.Booking{
		HiddenSteps: []string{"area", " staff", "area"},
		Fields: []model.CustomField{
			{Label: " Alerji var mı? ", Type: "text", Online: true},
			{Key: "alerji_var_mi_2", Label: "Kat", Type: "number", Panel: true},
			{Label: "Alerji var mı", Type: "select", Options: []string{" Evet ", "Hayır"}, Online: true},
			{Label: "123", Type: "checkbox", Required: true, Online: true, Options: []string{"x"}},
		},
	}
	got, err := Normalize(s)
	if err != nil {
		t.Fatal(err)
	}
	b := got.Booking
	if !slices.Equal(b.HiddenSteps, []string{"staff", "area"}) {
		t.Errorf("hidden steps = %v", b.HiddenSteps)
	}
	keys := []string{}
	for _, f := range b.Fields {
		keys = append(keys, f.Key)
	}
	if !slices.Equal(keys, []string{"alerji_var_mi", "alerji_var_mi_2", "alerji_var_mi_3", "field_123"}) {
		t.Errorf("keys = %v", keys)
	}
	if b.Fields[0].Label != "Alerji var mı?" || !slices.Equal(b.Fields[2].Options, []string{"Evet", "Hayır"}) || len(b.Fields[3].Options) != 0 {
		t.Errorf("fields = %+v", b.Fields)
	}

	// Defaults stay as they were: nothing hidden, no questions.
	if d, err := Normalize(model.Defaults()); err != nil || len(d.Booking.HiddenSteps) != 0 || len(d.Booking.Fields) != 0 {
		t.Errorf("defaults = %+v, %v", d.Booking, err)
	}

	step := 7
	for name, tc := range map[string]struct {
		edit func(*model.Settings)
		want string
	}{
		"required step": {func(s *model.Settings) { s.Booking.HiddenSteps = []string{"datetime"} }, "booking.hidden_steps.0"},
		"no label":      {func(s *model.Settings) { s.Booking.Fields = []model.CustomField{{Type: "text", Online: true}} }, "booking.fields.0.label"},
		"bad type": {func(s *model.Settings) {
			s.Booking.Fields = []model.CustomField{{Label: "A", Type: "date", Online: true}}
		}, "booking.fields.0.type"},
		"nowhere": {func(s *model.Settings) { s.Booking.Fields = []model.CustomField{{Label: "A", Type: "text"}} }, "booking.fields.0.online"},
		"one option": {func(s *model.Settings) {
			s.Booking.Fields = []model.CustomField{{Label: "A", Type: "select", Options: []string{"x"}, Panel: true}}
		}, "booking.fields.0.options"},
		"duplicate key": {func(s *model.Settings) {
			s.Booking.Fields = []model.CustomField{{Key: "a", Label: "A", Type: "text", Panel: true}, {Key: "a", Label: "B", Type: "text", Panel: true}}
		}, "booking.fields.1.key"},
		"bad key": {func(s *model.Settings) {
			s.Booking.Fields = []model.CustomField{{Key: "A-1", Label: "A", Type: "text", Panel: true}}
		}, "booking.fields.0.key"},
		"bad step":  {func(s *model.Settings) { s.Reservation.SlotStepMinutes = &step }, "reservation.slot_step_minutes"},
		"bad limit": {func(s *model.Settings) { zero := 0; s.Reservation.MaxOnlinePerDay = &zero }, "reservation.max_online_per_day"},
	} {
		s := model.Defaults()
		tc.edit(&s)
		_, err := Normalize(s)
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Details[0].Field != tc.want {
			t.Errorf("%s: %v, want %s", name, err, tc.want)
		}
	}
}

func TestCheckAnswers(t *testing.T) {
	t.Parallel()
	fields := []model.CustomField{
		{Key: "n", Label: "N", Type: "number", Required: true, Online: true},
		{Key: "p", Label: "P", Type: "text", Panel: true},
	}
	out, problems := model.CheckAnswers(fields, map[string]any{"n": float64(2), "p": "x", "other": 1}, model.AskOnline)
	if len(problems) != 0 || len(out) != 1 || out["n"] != float64(2) {
		t.Errorf("online = %v %v", out, problems)
	}
	out, problems = model.CheckAnswers(fields, map[string]any{"p": "  ", "other": 1}, model.AskPanel)
	if len(problems) != 0 || len(out) != 1 || out["other"] != 1 {
		t.Errorf("panel = %v %v", out, problems)
	}
	if _, problems = model.CheckAnswers(fields, map[string]any{"n": nil}, model.AskOnline); len(problems) != 1 || problems[0].Code != "required" {
		t.Errorf("required = %v", problems)
	}
}
