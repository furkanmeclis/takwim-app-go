package publicbooking_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicbooking/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// TestPublicBookingBundles (TEC-985): the bundle step lists active bundles
// shown online whose every service is bookable online in the branch, priced
// for the branch; availability and checkout take a bundle and expand it to
// its services; the booking keeps one line per service, splits the bundle
// price over the lines as the panel does (by the final line prices, the
// remainder on the last line) and names the bundle on each line. Hidden,
// inactive and foreign bundles read as not found.
func TestPublicBookingBundles(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	base := f.base(f.org.Slug)
	scan := func(dst []any, sql string, args ...any) {
		t.Helper()
		if err := f.pool.QueryRow(ctx, sql, args...).Scan(dst...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	var branchID, hiddenBranchID, cutID, blowID, ayseID int64
	scan([]any{&branchID}, `SELECT id FROM branches WHERE organization_id = $1 AND uuid = $2`, f.org.ID, f.branch)
	scan([]any{&hiddenBranchID}, `SELECT id FROM branches WHERE organization_id = $1 AND uuid = $2`, f.org.ID, f.hidden)
	scan([]any{&cutID}, `SELECT id FROM services WHERE organization_id = $1 AND uuid = $2`, f.org.ID, f.cut)
	scan([]any{&ayseID}, `SELECT id FROM staff WHERE organization_id = $1 AND uuid = $2`, f.org.ID, f.ayse)
	// Fön: list 100, 80 in the branch, Ayşe performs it at the branch price.
	var blow uuid.UUID
	scan([]any{&blowID, &blow}, `INSERT INTO services (organization_id, name, duration_minutes, price, payment_mode)
		VALUES ($1, 'Fön', 30, 100, 'both') RETURNING id, uuid`, f.org.ID)
	f.exec(`INSERT INTO service_branches (organization_id, service_id, branch_id, price) VALUES ($1, $2, $3, 80)`, f.org.ID, blowID, branchID)
	f.exec(`INSERT INTO service_staff (organization_id, service_id, staff_id) VALUES ($1, $2, $3)`, f.org.ID, blowID, ayseID)

	newBundle := func(name string, price int, showOnline bool, services []int64, branches []int64) uuid.UUID {
		t.Helper()
		var id int64
		var u uuid.UUID
		scan([]any{&id, &u}, `INSERT INTO service_bundles (organization_id, name, price, show_online) VALUES ($1, $2, $3, $4) RETURNING id, uuid`,
			f.org.ID, name, price, showOnline)
		for i, sid := range services {
			f.exec(`INSERT INTO service_bundle_items (organization_id, bundle_id, service_id, position) VALUES ($1, $2, $3, $4)`, f.org.ID, id, sid, i)
		}
		for _, bid := range branches {
			f.exec(`INSERT INTO service_bundle_branches (organization_id, bundle_id, branch_id) VALUES ($1, $2, $3)`, f.org.ID, id, bid)
		}
		return u
	}
	spring := newBundle("İlkbahar paketi", 200, true, []int64{cutID, blowID}, []int64{branchID, hiddenBranchID})
	secret := newBundle("Gizli paket", 150, false, []int64{cutID, blowID}, []int64{branchID})
	// The hidden service is not bookable online: the bundle is not offered.
	var secretID int64
	scan([]any{&secretID}, `SELECT id FROM services WHERE organization_id = $1 AND uuid = $2`, f.org.ID, f.secret)
	newBundle("Özel paket", 150, true, []int64{cutID, secretID}, []int64{branchID})
	// A bundle of the hidden branch only.
	newBundle("Şube paketi", 150, true, []int64{cutID, blowID}, []int64{hiddenBranchID})
	// B's bundle.
	var b2ID, cut2ID int64
	scan([]any{&b2ID}, `SELECT id FROM branches WHERE organization_id = $1 AND uuid = $2`, f.org2.ID, f.branch2)
	scan([]any{&cut2ID}, `SELECT id FROM services WHERE organization_id = $1 AND uuid = $2`, f.org2.ID, f.cut2)
	var foreignID int64
	var foreign uuid.UUID
	scan([]any{&foreignID, &foreign}, `INSERT INTO service_bundles (organization_id, name, price) VALUES ($1, 'Yabancı paket', 10) RETURNING id, uuid`, f.org2.ID)
	f.exec(`INSERT INTO service_bundle_items (organization_id, bundle_id, service_id, position) VALUES ($1, $2, $3, 0), ($1, $2, $3, 1)`, f.org2.ID, foreignID, cut2ID)
	f.exec(`INSERT INTO service_bundle_branches (organization_id, bundle_id, branch_id) VALUES ($1, $2, $3)`, f.org2.ID, foreignID, b2ID)

	// The catalog: only the spring bundle, priced for the branch (120 + 80).
	r := f.do(t, http.MethodGet, base+"/bundles?branch_uuid="+f.branch.String(), nil, nil)
	if r.code != http.StatusOK {
		t.Fatalf("bundles = %d %+v", r.code, r.env.Error)
	}
	list := decode[[]model.Bundle](t, r)
	if len(list) != 1 || list[0].UUID != spring || !list[0].Price.Equal(money.MustParse("200")) ||
		!list[0].ListPrice.Equal(money.MustParse("200")) || !list[0].Discount.IsZero() || list[0].DurationMinutes != 60 ||
		len(list[0].Items) != 2 || !list[0].Items[0].Price.Equal(money.MustParse("120")) || !list[0].Items[1].Price.Equal(money.MustParse("80")) ||
		!list[0].Items[0].AllocatedPrice.Equal(money.MustParse("120")) || len(list[0].BranchUUIDs) != 1 || list[0].BranchUUIDs[0] != f.branch ||
		len(list[0].PaymentModes) != 2 {
		t.Fatalf("bundles = %+v", list)
	}
	// Without a branch: the list prices (100 + 100), the public branch only.
	list = decode[[]model.Bundle](t, f.do(t, http.MethodGet, base+"/bundles", nil, nil))
	if len(list) != 1 || !list[0].ListPrice.Equal(money.MustParse("200")) || !list[0].Items[0].Price.Equal(money.MustParse("100")) ||
		len(list[0].BranchUUIDs) != 1 {
		t.Fatalf("bundles without branch = %+v", list)
	}
	for name, path := range map[string]string{
		"hidden branch":  base + "/bundles?branch_uuid=" + f.hidden.String(),
		"foreign branch": base + "/bundles?branch_uuid=" + f.branch2.String(),
	} {
		if r := f.do(t, http.MethodGet, path, nil, nil); r.code != http.StatusBadRequest || r.detail() != "branch_uuid/not_found" {
			t.Fatalf("%s = %d %s", name, r.code, r.detail())
		}
	}
	if other := decode[[]model.Bundle](t, f.do(t, http.MethodGet, f.base(f.org2.Slug)+"/bundles", nil, nil)); len(other) != 0 {
		// B's bundle holds the same service twice and is not shown online in
		// a public branch with an active offer of a public service: fine
		// either way, but never A's bundle.
		for _, b := range other {
			if b.UUID == spring {
				t.Fatalf("A's bundle listed for B")
			}
		}
	}

	// Availability with the bundle: two lines, Ayşe serving both in a row.
	r = f.do(t, http.MethodPost, base+"/availability", map[string]any{"branch_uuid": f.branch, "bundle_uuid": spring, "lines": []any{}}, nil)
	if r.code != http.StatusOK {
		t.Fatalf("availability = %d %+v", r.code, r.env.Error)
	}
	av := decode[model.Availability](t, r)
	if len(av.Lines) != 2 || av.Lines[0].ServiceUUID != f.cut || av.Lines[1].ServiceUUID != blow || len(av.Days) == 0 || len(av.Days[0].Slots) == 0 {
		t.Fatalf("availability = %+v", av)
	}
	for name, body := range map[string]map[string]any{
		"hidden bundle":   {"branch_uuid": f.branch, "bundle_uuid": secret},
		"foreign bundle":  {"branch_uuid": f.branch, "bundle_uuid": foreign},
		"unknown bundle":  {"branch_uuid": f.branch, "bundle_uuid": uuid.New()},
		"other lines":     {"branch_uuid": f.branch, "bundle_uuid": spring, "lines": lines(f.cut)},
		"reversed lines":  {"branch_uuid": f.branch, "bundle_uuid": spring, "lines": lines(blow, f.cut)},
		"inactive bundle": {"branch_uuid": f.branch, "bundle_uuid": spring, "__inactive": true},
	} {
		if body["__inactive"] != nil {
			delete(body, "__inactive")
			f.exec(`UPDATE service_bundles SET is_active = false WHERE organization_id = $1 AND uuid = $2`, f.org.ID, spring)
		}
		r := f.do(t, http.MethodPost, base+"/availability", body, nil)
		f.exec(`UPDATE service_bundles SET is_active = true WHERE organization_id = $1 AND uuid = $2`, f.org.ID, spring)
		if r.code != http.StatusBadRequest {
			t.Fatalf("%s = %d %s", name, r.code, r.detail())
		}
		want := "bundle_uuid/not_found"
		if name == "other lines" || name == "reversed lines" {
			want = "lines/bundle_mismatch"
		}
		if r.detail() != want {
			t.Fatalf("%s = %s", name, r.detail())
		}
	}

	// Checkout with the bundle: the lines carry the bundle.
	r = f.do(t, http.MethodPost, base+"/checkouts", map[string]any{
		"branch_uuid": f.branch, "bundle_uuid": spring, "lines": []map[string]any{{"service_uuid": f.cut, "staff_uuid": f.ayse}, {"service_uuid": blow}},
		"starts_at": at(9, 0),
	}, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("checkout = %d %+v", r.code, r.env.Error)
	}
	co := decode[model.Checkout](t, r)
	if co.Bundle == nil || co.Bundle.UUID != spring || co.Bundle.Name != "İlkbahar paketi" || len(co.Lines) != 2 ||
		co.Lines[0].Bundle == nil || co.Lines[1].Bundle == nil || co.Lines[0].Staff == nil || co.Lines[0].Staff.UUID != f.ayse ||
		!co.Lines[1].StartsAt.Equal(at(9, 30)) {
		t.Fatalf("checkout = %+v", co)
	}
	// A single-service checkout has no bundle.
	if plain := decode[model.Checkout](t, f.checkout(t, f.org.Slug, at(11, 0), nil)); plain.Bundle != nil || plain.Lines[0].Bundle != nil {
		t.Fatalf("plain checkout = %+v", plain)
	}

	// Booking: Ayşe's cut 150 and the branch's fön 80 (230) sold at 200 →
	// 130.43 + 69.57, one line per service, each naming the bundle.
	r = f.do(t, http.MethodPost, base+"/reservations", map[string]any{"checkout_token": co.Token, "customer": customer("05321234567")}, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("book = %d %+v", r.code, r.env.Error)
	}
	res := decode[model.Reservation](t, r)
	if !res.TotalPrice.Equal(money.MustParse("200")) || len(res.Lines) != 2 ||
		res.Lines[0].BundleName == nil || *res.Lines[0].BundleName != "İlkbahar paketi" || res.Lines[1].BundleName == nil ||
		!res.Lines[0].UnitPrice.Equal(money.MustParse("130.43")) || !res.Lines[1].UnitPrice.Equal(money.MustParse("69.57")) ||
		res.Lines[0].ListPrice == nil || !res.Lines[0].ListPrice.Equal(money.MustParse("150")) ||
		res.Lines[1].ListPrice == nil || !res.Lines[1].ListPrice.Equal(money.MustParse("80")) ||
		res.Lines[0].Staff == nil || res.Lines[0].Staff.Name != "Ayşe" || res.Lines[1].Staff == nil {
		t.Fatalf("reservation = %+v", res)
	}
	var bundleName string
	var n int
	scan([]any{&n, &bundleName}, `SELECT count(*), min(bundle_name) FROM reservation_lines l JOIN reservations r ON r.id = l.reservation_id
		WHERE r.organization_id = $1 AND r.uuid = $2 AND l.bundle_id IS NOT NULL`, f.org.ID, res.UUID)
	if n != 2 || bundleName != "İlkbahar paketi" {
		t.Fatalf("stored lines = %d %q", n, bundleName)
	}
	// The status route shows the bundle too.
	status := decode[model.Reservation](t, f.do(t, http.MethodGet, base+"/reservations/"+res.UUID.String(), nil, map[string]string{"X-Reservation-Token": res.AccessToken}))
	if status.Lines[1].BundleName == nil || *status.Lines[1].BundleName != "İlkbahar paketi" {
		t.Fatalf("status = %+v", status)
	}

	// A bundle hidden online after the checkout is refused at booking.
	r = f.do(t, http.MethodPost, base+"/checkouts", map[string]any{"branch_uuid": f.branch, "bundle_uuid": spring, "starts_at": at(10, 0)}, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("second checkout = %d %+v", r.code, r.env.Error)
	}
	token := decode[model.Checkout](t, r).Token
	f.exec(`UPDATE service_bundles SET show_online = false WHERE organization_id = $1 AND uuid = $2`, f.org.ID, spring)
	if r := f.do(t, http.MethodPost, base+"/reservations", map[string]any{"checkout_token": token, "customer": customer("05321234568")}, nil); r.code != http.StatusBadRequest || r.detail() != "bundle_uuid/not_found" {
		t.Fatalf("hidden at booking = %d %s", r.code, r.detail())
	}
}
