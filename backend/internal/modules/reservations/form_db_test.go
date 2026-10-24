package reservations_test

import (
	"context"
	"errors"
	"testing"

	orgsettingsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
)

// saveForm stores the booking rules with the given questions and limits.
func (f *fixture) saveForm(t *testing.T, fields []orgsettingsmodel.CustomField, maxOpen, maxDay *int) int {
	t.Helper()
	lead, advance, window, payment := 0, 30, 30, 15
	s := orgsettingsmodel.Defaults()
	s.Reservation = orgsettingsmodel.Reservation{
		ApprovalMode: orgsettingsmodel.ApprovalAuto, StaffReviewWindowMinutes: &window, PaymentHoldMinutes: &payment,
		MinLeadMinutes: &lead, MaxAdvanceDays: &advance, MaxOpenPerCustomer: maxOpen, MaxOnlinePerDay: maxDay,
	}
	s.Payment = orgsettingsmodel.Payment{PayAtVenue: true, PayOnline: true}
	s.Booking.Fields = fields
	v, err := f.settings.Save(context.Background(), f.org.ID, f.owner.ID, orgsettingsmodel.SaveInput{Settings: s})
	if err != nil {
		t.Fatal(err)
	}
	return v.Version
}

func detailOf(err error) string {
	var ve *usecase.ValidationError
	if errors.As(err, &ve) && len(ve.Details) > 0 {
		return ve.Details[0].Field + "/" + ve.Details[0].Code
	}
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}

// TestBookingForm (TEC-905): the panel asks its questions (required ones
// must be answered, types are checked) and keeps other answers; online
// bookings answer only the online questions. A record keeps the question
// it was answered under and the settings version it was booked under, so
// later changes to the form do not rewrite it. The online limits count the
// customer's open bookings and the branch's online bookings of a day;
// panel bookings are not limited.
func TestBookingForm(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	fields := []orgsettingsmodel.CustomField{
		{Key: "floor", Label: "Kat", Type: "number", Required: true, Panel: true},
		{Key: "allergy", Label: "Alerji", Type: "text", Online: true, Panel: true},
		{Key: "color", Label: "Renk", Type: "select", Options: []string{"Sarı", "Kahve"}, Online: true},
		{Key: "consent", Label: "Onay", Type: "checkbox", Required: true, Online: true},
	}
	one := 1
	version := f.saveForm(t, fields, &one, nil)

	// Panel: the required number, a bad type.
	in := f.input(at(9, 0), "", "", f.cut)
	if _, err := f.svc.Create(ctx, f.org.ID, in, none, f.owner.ID); detailOf(err) != "custom_fields.floor/required" {
		t.Fatalf("panel without floor = %s", detailOf(err))
	}
	in.CustomFields = map[string]any{"floor": "iki"}
	if _, err := f.svc.Create(ctx, f.org.ID, in, none, f.owner.ID); detailOf(err) != "custom_fields.floor/invalid" {
		t.Fatalf("panel bad floor = %s", detailOf(err))
	}
	// The online-only required checkbox is not asked in the panel; an
	// older free key stays.
	in.CustomFields = map[string]any{"floor": float64(2), "allergy": "  yok ", "legacy": "x"}
	r, err := f.svc.Create(ctx, f.org.ID, in, none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if r.CustomFields["allergy"] != "yok" || r.CustomFields["legacy"] != "x" || r.SettingsVersion == nil || *r.SettingsVersion != version ||
		len(r.CustomAnswers) != 3 || r.CustomAnswers[0].Label != "Kat" || r.CustomAnswers[1].Label != "Alerji" || r.CustomAnswers[2].Label != "legacy" {
		t.Fatalf("panel record = %+v / %+v", r.CustomFields, r.CustomAnswers)
	}
	// The open-bookings limit does not apply to the panel.
	in2 := f.input(at(10, 0), "", "", f.cut)
	in2.CustomFields = map[string]any{"floor": float64(1)}
	if _, err := f.svc.Create(ctx, f.org.ID, in2, none, f.owner.ID); err != nil {
		t.Fatalf("second panel booking = %v", err)
	}

	// The form changes: the record keeps its labels; editing the notes
	// checks the new panel question and keeps the old label.
	fields[0].Label = "Bulunduğu kat"
	fields[1].Label = "Alerjisi"
	f.saveForm(t, fields, nil, nil)
	got, err := f.svc.Get(ctx, f.org.ID, r.UUID, none)
	if err != nil {
		t.Fatal(err)
	}
	if got.CustomAnswers[0].Label != "Kat" || got.CustomAnswers[1].Label != "Alerji" {
		t.Fatalf("after the form changed = %+v", got.CustomAnswers)
	}
	if _, err := f.svc.UpdateNotes(ctx, f.org.ID, r.UUID, none, f.owner.ID, model.NotesInput{CustomFields: map[string]any{"allergy": "polen"}}); detailOf(err) != "custom_fields.floor/required" {
		t.Fatalf("notes without floor = %s", detailOf(err))
	}
	got, err = f.svc.UpdateNotes(ctx, f.org.ID, r.UUID, none, f.owner.ID, model.NotesInput{CustomFields: map[string]any{"floor": float64(3), "allergy": "polen"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.CustomFields["floor"] != float64(3) || got.CustomAnswers[0].Label != "Kat" || len(got.CustomAnswers) != 2 {
		t.Fatalf("notes = %+v / %+v", got.CustomFields, got.CustomAnswers)
	}
	// Notes without custom_fields keep the answers.
	got, err = f.svc.UpdateNotes(ctx, f.org.ID, r.UUID, none, f.owner.ID, model.NotesInput{CustomerNote: "kapıda"})
	if err != nil || got.CustomFields["floor"] != float64(3) || len(got.CustomAnswers) != 2 {
		t.Fatalf("notes only = %v %+v", err, got.CustomFields)
	}

	// Online: only the online questions; the required checkbox; a select
	// option; unknown keys are dropped.
	f.saveForm(t, fields, &one, &one)
	f.exec(`UPDATE reservations SET status = 'cancelled' WHERE organization_id = $1`, f.org.ID)
	on := f.input(at(11, 0), model.SourceOnline, "on_site", f.cut)
	on.CustomFields = map[string]any{"consent": false}
	if _, err := f.svc.Create(ctx, f.org.ID, on, none, 0); detailOf(err) != "custom_fields.consent/required" {
		t.Fatalf("online without consent = %s", detailOf(err))
	}
	on.CustomFields = map[string]any{"consent": true, "color": "Mavi"}
	if _, err := f.svc.Create(ctx, f.org.ID, on, none, 0); detailOf(err) != "custom_fields.color/invalid" {
		t.Fatalf("online bad option = %s", detailOf(err))
	}
	on.CustomFields = map[string]any{"consent": true, "color": "Sarı", "floor": float64(9), "junk": "x"}
	ro, err := f.svc.Create(ctx, f.org.ID, on, none, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ro.CustomFields["floor"]; ok || ro.CustomFields["junk"] != nil || ro.CustomFields["color"] != "Sarı" || len(ro.CustomAnswers) != 2 {
		t.Fatalf("online record = %+v", ro.CustomFields)
	}

	// Limits: the customer holds one open booking; the branch took one
	// online booking that day.
	on2 := f.input(at(11, 30), model.SourceOnline, "on_site", f.cut)
	on2.CustomFields = map[string]any{"consent": true}
	if _, err := f.svc.Create(ctx, f.org.ID, on2, none, 0); detailOf(err) != "customer/open_limit" {
		t.Fatalf("second open booking = %s", detailOf(err))
	}
	two := 2
	f.saveForm(t, fields, &two, &one)
	if _, err := f.svc.Create(ctx, f.org.ID, on2, none, 0); detailOf(err) != "starts_at/daily_limit" {
		t.Fatalf("second online booking of the day = %s", detailOf(err))
	}
	if n := f.reservationsOf(); n != 3 {
		t.Fatalf("reservations = %d, want 3 (refused bookings write nothing)", n)
	}
}
