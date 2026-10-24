package publicbooking_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicbooking"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicbooking/model"
)

// testSession stands in for the customer panel session: X-Test-Customer
// signs in, and the header names the business's customer record ("none":
// the business does not know the person).
type testSession struct{ customer map[string]uuid.UUID }

func (s testSession) Authenticate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test-Customer") == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"success":false}`))
			return
		}
		next(w, r)
	}
}

func (s testSession) CustomerOf(r *http.Request, _ int64) (uuid.UUID, error) {
	return s.customer[r.Header.Get("X-Test-Customer")], nil
}

// TestPublicBookingPackages (TEC-984): the anonymous route refuses package
// sessions; the signed-in route books for the business's record of the
// person and pays the line with a session of that customer's package.
func TestPublicBookingPackages(t *testing.T) {
	f := newFixture(t)
	var zeynep, ali uuid.UUID
	if err := f.pool.QueryRow(context.Background(), `INSERT INTO customers (organization_id, first_name, last_name, phone_e164)
		VALUES ($1, 'Zeynep', 'Kaya', '+905551112233') RETURNING uuid`, f.org.ID).Scan(&zeynep); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(context.Background(), `INSERT INTO customers (organization_id, first_name) VALUES ($1, 'Ali') RETURNING uuid`, f.org.ID).Scan(&ali); err != nil {
		t.Fatal(err)
	}
	sell := func(customer uuid.UUID) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		if err := f.pool.QueryRow(context.Background(), `INSERT INTO customer_packages
			(organization_id, customer_id, branch_id, service_id, name, service_name, total_sessions, price, currency, sold_on)
			SELECT $1, c.id, b.id, s.id, 'Kesim 3', 'Kesim', 3, 300, 'TRY', DATE '2030-01-01'
			FROM customers c, branches b, services s
			WHERE c.organization_id = $1 AND c.uuid = $2 AND b.organization_id = $1 AND b.uuid = $3 AND s.organization_id = $1 AND s.uuid = $4
			RETURNING uuid`, f.org.ID, customer, f.branch, f.cut).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	pkg, alis := sell(zeynep), sell(ali)
	mux := http.NewServeMux()
	publicbooking.RegisterRoutes(mux, f.h)
	publicbooking.RegisterCustomerRoutes(mux, f.h, testSession{customer: map[string]uuid.UUID{"zeynep": zeynep, "none": uuid.Nil}})
	f.mux = mux
	base := f.base(f.org.Slug)
	checkout := func(hour, minute int) uuid.UUID {
		t.Helper()
		r := f.checkout(t, f.org.Slug, at(hour, minute), nil)
		if r.code != http.StatusCreated {
			t.Fatalf("checkout = %d %+v", r.code, r.env.Error)
		}
		return decode[model.Checkout](t, r).Token
	}
	sessions := func(id uuid.UUID) []map[string]any {
		return []map[string]any{{"customer_package_uuid": id, "line_index": 0}}
	}
	as := func(who string) map[string]string { return map[string]string{"X-Test-Customer": who} }

	token := checkout(9, 0)
	body := map[string]any{"checkout_token": token, "payment": "on_site", "package_sessions": sessions(pkg)}
	anon := map[string]any{"checkout_token": token, "payment": "on_site", "customer": customer("+905551112233"), "package_sessions": sessions(pkg)}
	if r := f.do(t, http.MethodPost, base+"/reservations", anon, nil); r.code != http.StatusBadRequest || r.detail() != "package_sessions/sign_in_required" {
		t.Fatalf("anonymous with a package = %d %s", r.code, r.detail())
	}
	path := "/v1/customer/businesses/" + f.org.Slug + "/reservations"
	if r := f.do(t, http.MethodPost, path, body, nil); r.code != http.StatusUnauthorized {
		t.Fatalf("signed out = %d", r.code)
	}
	if r := f.do(t, http.MethodPost, path, body, as("none")); r.code != http.StatusBadRequest || r.detail() != "customer/not_linked" {
		t.Fatalf("unknown person = %d %s", r.code, r.detail())
	}
	if r := f.do(t, http.MethodPost, path, map[string]any{"checkout_token": token, "payment": "on_site", "package_sessions": sessions(alis)}, as("zeynep")); r.code != http.StatusBadRequest ||
		r.detail() != "package_sessions.0.customer_package_uuid/not_found" {
		t.Fatalf("another customer's package = %d %s", r.code, r.detail())
	}
	if r := f.do(t, http.MethodPost, path, map[string]any{"checkout_token": token, "payment": "online", "package_sessions": sessions(pkg)}, as("zeynep")); r.code != http.StatusBadRequest ||
		r.detail() != "payment/not_allowed" {
		t.Fatalf("online payment of a covered visit = %d %s", r.code, r.detail())
	}
	r := f.do(t, http.MethodPost, path, body, as("zeynep"))
	if r.code != http.StatusCreated {
		t.Fatalf("signed-in booking = %d %+v %s", r.code, r.env.Error, r.detail())
	}
	res := decode[model.Reservation](t, r)
	if len(res.Lines) != 1 || res.Lines[0].PackageName == nil || *res.Lines[0].PackageName != "Kesim 3" || !res.TotalPrice.IsZero() {
		t.Fatalf("booked = %+v", res)
	}
	if n := f.count(`SELECT count(*) FROM package_sessions ps JOIN reservations r ON r.id = ps.reservation_id
		JOIN customers c ON c.id = r.customer_id WHERE ps.organization_id = $1 AND r.uuid = $2 AND ps.status = 'held' AND c.uuid = $3`,
		f.org.ID, res.UUID, zeynep); n != 1 {
		t.Fatalf("held sessions = %d", n)
	}
	// Without sessions the signed-in route books for the same record.
	r = f.do(t, http.MethodPost, path, map[string]any{"checkout_token": checkout(10, 0), "payment": "on_site"}, as("zeynep"))
	if r.code != http.StatusCreated || decode[model.Reservation](t, r).TotalPrice.IsZero() {
		t.Fatalf("plain signed-in booking = %d %+v", r.code, r.env.Error)
	}
	if n := f.count(`SELECT count(*) FROM reservations r JOIN customers c ON c.id = r.customer_id WHERE r.organization_id = $1 AND c.uuid = $2`, f.org.ID, zeynep); n != 2 {
		t.Fatalf("Zeynep's visits = %d", n)
	}
}
