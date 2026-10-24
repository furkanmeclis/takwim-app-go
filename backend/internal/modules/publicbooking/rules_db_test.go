package publicbooking_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	orgsettingsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicbooking/model"
)

// TestBookingRules (TEC-905): the business hides the staff and products
// steps, asks its own questions online and offers starts every 30
// minutes. The profile shows the flow without the hidden steps and the
// online questions; the API refuses a person, products and starts off the
// grid, checks the answers and returns them with the booking.
func TestBookingRules(t *testing.T) {
	f := newFixture(t)
	base := f.base(f.org.Slug)
	ctx := context.Background()

	cur, err := f.settings.Current(ctx, f.org.ID)
	if err != nil {
		t.Fatal(err)
	}
	s := cur.Settings
	step := 30
	s.Reservation.SlotStepMinutes = &step
	s.Booking.HiddenSteps = []string{"products", "staff"}
	s.Booking.Fields = []orgsettingsmodel.CustomField{
		{Label: "Saç tipi", Type: "select", Options: []string{"Düz", "Kıvırcık"}, Required: true, Online: true},
		{Label: "İç not", Type: "text", Panel: true},
	}
	saved, err := f.settings.Save(ctx, f.org.ID, f.owner.ID, orgsettingsmodel.SaveInput{Settings: s})
	if err != nil {
		t.Fatal(err)
	}
	if got := saved.Settings.Booking; !slices.Equal(got.HiddenSteps, []string{"staff", "products"}) || got.Fields[0].Key != "sac_tipi" || got.Fields[1].Key != "ic_not" {
		t.Fatalf("saved booking = %+v", got)
	}

	biz := decode[model.Business](t, f.do(t, http.MethodGet, base, nil, nil))
	if slices.Contains(biz.Flow, "staff") || slices.Contains(biz.Flow, "products") || !slices.Contains(biz.Flow, "datetime") ||
		len(biz.Booking.Fields) != 1 || biz.Booking.Fields[0].Key != "sac_tipi" || biz.Booking.SlotStepMinutes == nil || *biz.Booking.SlotStepMinutes != 30 {
		t.Fatalf("profile = %+v / %+v", biz.Flow, biz.Booking)
	}

	withStaff := []map[string]any{{"service_uuid": f.cut, "staff_uuid": f.ayse}}
	if r := f.do(t, http.MethodPost, base+"/availability", map[string]any{"branch_uuid": f.branch, "lines": withStaff}, nil); r.code != http.StatusBadRequest || r.detail() != "lines.0.staff_uuid/not_offered" {
		t.Fatalf("availability with a person = %d %s", r.code, r.detail())
	}
	r := f.do(t, http.MethodPost, base+"/availability", map[string]any{
		"branch_uuid": f.branch, "lines": lines(f.cut), "from": "2030-01-07", "to": "2030-01-07", "step_minutes": 15,
	}, nil)
	av := decode[model.Availability](t, r)
	if av.StepMinutes != 30 || len(av.Days) != 1 || len(av.Days[0].Slots) == 0 {
		t.Fatalf("availability = %+v", av)
	}
	for _, sl := range av.Days[0].Slots {
		if sl.StartsAt.Minute()%30 != 0 {
			t.Fatalf("start off the grid: %s", sl.StartsAt)
		}
	}
	if r := f.checkout(t, f.org.Slug, at(9, 15), nil); r.code != http.StatusConflict || r.detail() != "starts_at/taken" {
		t.Fatalf("checkout off the grid = %d %s", r.code, r.detail())
	}
	if r := f.do(t, http.MethodPost, base+"/checkouts", map[string]any{"branch_uuid": f.branch, "lines": withStaff, "starts_at": at(9, 0)}, nil); r.code != http.StatusBadRequest {
		t.Fatalf("checkout with a person = %d %s", r.code, r.detail())
	}
	if items := decode[[]model.Product](t, f.do(t, http.MethodGet, base+"/products?branch_uuid="+f.branch.String(), nil, nil)); len(items) != 0 {
		t.Fatalf("products of a hidden step = %+v", items)
	}

	co := decode[model.Checkout](t, f.checkout(t, f.org.Slug, at(9, 30), nil))
	book := func(extra map[string]any) result {
		body := map[string]any{"checkout_token": co.Token, "customer": customer("05321234567")}
		for k, v := range extra {
			body[k] = v
		}
		return f.do(t, http.MethodPost, base+"/reservations", body, nil)
	}
	if r := book(nil); r.code != http.StatusBadRequest || r.detail() != "custom_fields.sac_tipi/required" {
		t.Fatalf("booking without the answer = %d %s", r.code, r.detail())
	}
	if r := book(map[string]any{"custom_fields": map[string]any{"sac_tipi": "Düz"}, "products": []map[string]any{{"product_uuid": f.cut, "quantity": 1}}}); r.code != http.StatusBadRequest || r.detail() != "products/not_offered" {
		t.Fatalf("booking with products = %d %s", r.code, r.detail())
	}
	r = book(map[string]any{"custom_fields": map[string]any{"sac_tipi": "Kıvırcık", "ic_not": "x"}})
	if r.code != http.StatusCreated {
		t.Fatalf("booking = %d %+v", r.code, r.env.Error)
	}
	res := decode[model.Reservation](t, r)
	if len(res.Answers) != 1 || res.Answers[0].Label != "Saç tipi" || res.Answers[0].Value != "Kıvırcık" {
		t.Fatalf("answers = %+v", res.Answers)
	}
	if n := f.count(`SELECT count(*) FROM reservations WHERE organization_id = $1 AND settings_version = $2 AND custom_fields = '{"sac_tipi":"Kıvırcık"}'`, f.org.ID, saved.Version); n != 1 {
		t.Fatalf("stored = %d", n)
	}

	// One online booking a day: the next checkout of the day is refused
	// before the customer fills in the form.
	one := 1
	s = saved.Settings
	s.Reservation.MaxOnlinePerDay = &one
	if _, err := f.settings.Save(ctx, f.org.ID, f.owner.ID, orgsettingsmodel.SaveInput{Settings: s}); err != nil {
		t.Fatal(err)
	}
	if r := f.checkout(t, f.org.Slug, at(10, 30), nil); r.code != http.StatusBadRequest || r.detail() != "starts_at/daily_limit" {
		t.Fatalf("checkout over the daily limit = %d %s", r.code, r.detail())
	}
}
