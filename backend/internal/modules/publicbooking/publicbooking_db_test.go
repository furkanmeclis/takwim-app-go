package publicbooking_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	orgsettingsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicbooking"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicbooking/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicbooking/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicbooking/usecase"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/idempotency"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ratelimit"
)

var ist = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

// at is a Monday 2030-01-07 wall clock time in Istanbul.
func at(hour, minute int) time.Time {
	return time.Date(2030, 1, 7, hour, minute, 0, 0, ist)
}

// fixture: business A with a public branch (Ayşe 09:00–12:00 on Mondays,
// a cut with list price 100 and branch price 120, a hidden service, a
// hidden person), a hidden branch, the web channel on and the widget off;
// business B with its own branch and service. The clock is 08:00.
type fixture struct {
	pool           *pgxpool.Pool
	q              *db.Queries
	org, org2      orgmodel.Organization
	owner          db.User
	branch, hidden uuid.UUID
	branch2        uuid.UUID
	cut, secret    uuid.UUID
	cut2           uuid.UUID
	ayse, gizli    uuid.UUID
	settings       *orgsettingsusecase.Service
	now            time.Time
	svc            *usecase.Service
	h              *handler.Handler
	mux            http.Handler
	exec           func(sql string, args ...any)
	count          func(sql string, args ...any) int
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	suffix := uuid.NewString()[:8]
	f := &fixture{pool: pool, q: q, now: at(8, 0)}
	newUser := func(name string) db.User {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: name + "-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	f.owner = newUser("owner")
	other := newUser("other")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	f.org, err = orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Public " + suffix, OwnerUserID: f.owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	f.org2, err = orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Public other " + suffix, OwnerUserID: other.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{f.org.ID, f.org2.ID})
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{f.owner.ID, other.ID})
	})
	f.exec = func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	f.count = func(sql string, args ...any) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return n
	}
	idOf := func(sql string, args ...any) (int64, uuid.UUID) {
		t.Helper()
		var id int64
		var u uuid.UUID
		if err := pool.QueryRow(ctx, sql, args...).Scan(&id, &u); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return id, u
	}
	hours := `{"mon":{"closed":false,"open":"09:00","close":"18:00"}}`
	aID, aUUID := idOf("SELECT id, uuid FROM branches WHERE organization_id = $1 ORDER BY id LIMIT 1", f.org.ID)
	f.exec(`UPDATE branches SET working_hours = $3, holiday_exceptions = '[{"date":"2030-01-01","closed":true,"open":"","close":"","note":"Yılbaşı"},{"date":"2030-02-01","closed":true,"open":"","close":"","note":"Bakım"}]', timezone = NULL
		WHERE organization_id = $1 AND id = $2`, f.org.ID, aID, hours)
	f.branch = aUUID
	hiddenID, hidden := idOf(`INSERT INTO branches (organization_id, code, name, working_hours, show_online) VALUES ($1, 'GIZ', 'Gizli şube', $2, false) RETURNING id, uuid`, f.org.ID, hours)
	f.hidden = hidden
	ayseID, ayse := idOf(`INSERT INTO staff (organization_id, name, title) VALUES ($1, 'Ayşe', 'Usta') RETURNING id, uuid`, f.org.ID)
	f.ayse = ayse
	gizliID, gizli := idOf(`INSERT INTO staff (organization_id, name, show_online) VALUES ($1, 'Gizli', false) RETURNING id, uuid`, f.org.ID)
	f.gizli = gizli
	for _, id := range []int64{ayseID, gizliID} {
		f.exec(`INSERT INTO staff_branches (organization_id, staff_id, branch_id, shifts) VALUES
			($1, $2, $3, '{"mon":[{"start":"09:00","end":"12:00","breaks":[]}]}')`, f.org.ID, id, aID)
	}
	f.exec(`INSERT INTO staff_branches (organization_id, staff_id, branch_id, shifts) VALUES ($1, $2, $3, '{}')`, f.org.ID, ayseID, hiddenID)
	cutID, cut := idOf(`INSERT INTO services (organization_id, name, duration_minutes, price, payment_mode)
		VALUES ($1, 'Kesim', 30, 100, 'both') RETURNING id, uuid`, f.org.ID)
	f.cut = cut
	f.exec(`INSERT INTO service_branches (organization_id, service_id, branch_id, price) VALUES ($1, $2, $3, 120)`, f.org.ID, cutID, aID)
	f.exec(`INSERT INTO service_branches (organization_id, service_id, branch_id, price) VALUES ($1, $2, $3, 90)`, f.org.ID, cutID, hiddenID)
	f.exec(`INSERT INTO service_staff (organization_id, service_id, staff_id, price) VALUES ($1, $2, $3, 150)`, f.org.ID, cutID, ayseID)
	f.exec(`INSERT INTO service_staff (organization_id, service_id, staff_id) VALUES ($1, $2, $3)`, f.org.ID, cutID, gizliID)
	secretID, secret := idOf(`INSERT INTO services (organization_id, name, duration_minutes, show_online) VALUES ($1, 'Özel', 30, false) RETURNING id, uuid`, f.org.ID)
	f.secret = secret
	f.exec(`INSERT INTO service_branches (organization_id, service_id, branch_id) VALUES ($1, $2, $3)`, f.org.ID, secretID, aID)
	f.exec(`INSERT INTO service_staff (organization_id, service_id, staff_id) VALUES ($1, $2, $3)`, f.org.ID, secretID, ayseID)

	b2ID, b2 := idOf("SELECT id, uuid FROM branches WHERE organization_id = $1 ORDER BY id LIMIT 1", f.org2.ID)
	f.branch2 = b2
	cut2ID, cut2 := idOf(`INSERT INTO services (organization_id, name, duration_minutes) VALUES ($1, 'Yabancı kesim', 30) RETURNING id, uuid`, f.org2.ID)
	f.cut2 = cut2
	f.exec(`INSERT INTO service_branches (organization_id, service_id, branch_id) VALUES ($1, $2, $3)`, f.org2.ID, cut2ID, b2ID)

	f.settings = orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))
	f.saveSettings(t, f.org.ID, "auto", true)
	f.saveSettings(t, f.org2.ID, "auto", true)

	clock := func() time.Time { return f.now }
	holds := availabilityusecase.NewHolds(pool, q, f.settings)
	holds.SetClock(clock)
	holds.SetExpiryHook(reservationsusecase.ExpireRequests)
	avail := availabilityusecase.New(q, f.settings)
	avail.SetClock(clock)
	avail.SetBusySource(holds.Busy())
	cipher, err := crypto.NewFieldCipher(strings.Repeat("k", 32))
	if err != nil {
		t.Fatal(err)
	}
	f.svc = usecase.New(q, f.settings, avail, holds, reservationsusecase.New(holds, q, f.settings), cipher)
	f.svc.SetClock(clock)
	f.h = handler.New(f.svc, nil, nil)
	mux := http.NewServeMux()
	publicbooking.RegisterRoutes(mux, f.h)
	tokens, err := jwt.NewManager("public-booking-test-secret-0123456789", time.Hour, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	f.mux = middleware.Idempotency(idempotency.NewMemoryStore(), tokens, slog.New(slog.NewTextHandler(io.Discard, nil)))(mux)
	return f
}

// saveSettings sets the online rules of a business: approval mode, no
// minimum notice, 30 days ahead, both payment methods, the web channel on
// (and the widget off).
func (f *fixture) saveSettings(t *testing.T, orgID int64, approval string, web bool) {
	t.Helper()
	lead, advance, window, payment := 0, 30, 30, 15
	s := orgsettingsmodel.Defaults()
	s.Reservation = orgsettingsmodel.Reservation{
		ApprovalMode: approval, StaffReviewWindowMinutes: &window, PaymentHoldMinutes: &payment,
		MinLeadMinutes: &lead, MaxAdvanceDays: &advance,
	}
	s.Payment = orgsettingsmodel.Payment{PayAtVenue: true, PayOnline: true}
	s.Company.Phone = "+90 212 000 00 00"
	s.Policies.Cancellation = "24 saat öncesine kadar ücretsiz iptal"
	for i := range s.SalesChannels {
		s.SalesChannels[i].IsActive = s.SalesChannels[i].Code == "web" && web || s.SalesChannels[i].Code == "api"
	}
	if _, err := f.settings.Save(context.Background(), orgID, f.owner.ID, orgsettingsmodel.SaveInput{Settings: s}); err != nil {
		t.Fatal(err)
	}
}

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Details []struct {
			Field string `json:"field"`
			Code  string `json:"code"`
		} `json:"details"`
	} `json:"error"`
}

type result struct {
	code   int
	header http.Header
	env    envelope
}

func (r result) detail() string {
	if r.env.Error == nil || len(r.env.Error.Details) == 0 {
		return ""
	}
	return r.env.Error.Details[0].Field + "/" + r.env.Error.Details[0].Code
}

func (f *fixture) do(t *testing.T, method, path string, body any, header map[string]string) result {
	t.Helper()
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	var env envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body.String())
	}
	return result{code: rec.Code, header: rec.Header(), env: env}
}

func decode[T any](t *testing.T, r result) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(r.env.Data, &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, r.env.Data)
	}
	return out
}

func (f *fixture) base(slug string) string {
	return "/v1/public/businesses/" + slug
}

// TestPublicCatalog: the profile, branches, services and staff of a
// business show only active records shown online; unknown, suspended and
// expired businesses are 404; filters naming another business's or a
// hidden branch are 400.
func TestPublicCatalog(t *testing.T) {
	f := newFixture(t)
	base := f.base(f.org.Slug)

	r := f.do(t, http.MethodGet, base, nil, nil)
	if r.code != http.StatusOK {
		t.Fatalf("profile = %d %+v", r.code, r.env.Error)
	}
	biz := decode[model.Business](t, r)
	if biz.UUID != f.org.UUID || !biz.Booking.Ready || !biz.Booking.Channels["web"] || biz.Booking.Channels["widget"] ||
		biz.BranchCount != 1 || biz.Contact.Phone == "" || biz.Policies.Cancellation == "" || len(biz.Flow) == 0 ||
		len(biz.Booking.PaymentMethods) != 2 || biz.Booking.CheckoutMinutes != 10 || biz.LogoURL != nil {
		t.Fatalf("profile = %+v", biz)
	}
	if raw := string(r.env.Data); strings.Contains(raw, "tax_number") || strings.Contains(raw, f.org.UUID.String()+"\",\"id") {
		t.Fatalf("profile leaks internal fields: %s", raw)
	}
	if !strings.Contains(string(r.env.Data), `"embed":{"allowed_origins":[]}`) {
		t.Fatalf("profile embed = %s", r.env.Data)
	}

	// Embed (TEC-908): the sites allowed to frame the booking screen,
	// normalized by the settings and answered only for their business.
	cur, err := f.settings.Current(context.Background(), f.org.ID)
	if err != nil {
		t.Fatal(err)
	}
	cur.Settings.Embed.AllowedOrigins = []string{"Kuafor.example.com/", "https://*.example.org"}
	if _, err := f.settings.Save(context.Background(), f.org.ID, f.owner.ID, orgsettingsmodel.SaveInput{Settings: cur.Settings}); err != nil {
		t.Fatal(err)
	}
	biz = decode[model.Business](t, f.do(t, http.MethodGet, base, nil, nil))
	if strings.Join(biz.Embed.AllowedOrigins, " ") != "https://kuafor.example.com https://*.example.org" {
		t.Fatalf("embed origins = %v", biz.Embed.AllowedOrigins)
	}
	if other := decode[model.Business](t, f.do(t, http.MethodGet, f.base(f.org2.Slug), nil, nil)); len(other.Embed.AllowedOrigins) != 0 {
		t.Fatalf("embed origins of B = %v", other.Embed.AllowedOrigins)
	}

	for _, slug := range []string{"yok-boyle-bir-isletme", strings.ToUpper(f.org.Slug) + "x"} {
		if r := f.do(t, http.MethodGet, f.base(slug), nil, nil); r.code != http.StatusNotFound {
			t.Fatalf("unknown slug = %d", r.code)
		}
	}

	r = f.do(t, http.MethodGet, base+"/branches", nil, nil)
	branches := decode[[]model.Branch](t, r)
	if len(branches) != 1 || branches[0].UUID != f.branch || branches[0].WorkingHours["mon"].Open != "09:00" ||
		len(branches[0].SpecialDays) != 1 || branches[0].SpecialDays[0].Date != "2030-02-01" {
		t.Fatalf("branches = %+v", branches)
	}

	r = f.do(t, http.MethodGet, base+"/services", nil, nil)
	services := decode[[]model.Service](t, r)
	if len(services) != 1 || services[0].UUID != f.cut || services[0].Price.String() != "100.00" ||
		len(services[0].Branches) != 1 || services[0].Branches[0].Price.String() != "120.00" ||
		len(services[0].Staff) != 1 || services[0].Staff[0].StaffUUID != f.ayse || services[0].Staff[0].Price.String() != "150.00" {
		t.Fatalf("services = %+v", services)
	}
	r = f.do(t, http.MethodGet, base+"/services?branch_uuid="+f.branch.String(), nil, nil)
	if services = decode[[]model.Service](t, r); len(services) != 1 || services[0].Price.String() != "120.00" {
		t.Fatalf("services of branch = %+v", services)
	}
	for _, id := range []uuid.UUID{f.hidden, f.branch2} {
		if r := f.do(t, http.MethodGet, base+"/services?branch_uuid="+id.String(), nil, nil); r.code != http.StatusBadRequest || r.detail() != "branch_uuid/not_found" {
			t.Fatalf("services of %s = %d %s", id, r.code, r.detail())
		}
	}

	r = f.do(t, http.MethodGet, base+"/staff", nil, nil)
	staff := decode[[]model.Staff](t, r)
	if len(staff) != 1 || staff[0].UUID != f.ayse || staff[0].Title != "Usta" || len(staff[0].BranchUUIDs) != 1 ||
		len(staff[0].ServiceUUIDs) != 1 || staff[0].ServiceUUIDs[0] != f.cut {
		t.Fatalf("staff = %+v", staff)
	}
	if raw := string(r.env.Data); strings.Contains(raw, "phone") || strings.Contains(raw, "email") {
		t.Fatalf("staff leaks contact details: %s", raw)
	}
	if r := f.do(t, http.MethodGet, base+"/staff?service_uuid="+f.cut2.String(), nil, nil); r.code != http.StatusBadRequest {
		t.Fatalf("staff of another business's service = %d", r.code)
	}

	// Another business's slug never shows A's records.
	r = f.do(t, http.MethodGet, f.base(f.org2.Slug)+"/services", nil, nil)
	if services = decode[[]model.Service](t, r); len(services) != 1 || services[0].UUID != f.cut2 {
		t.Fatalf("services of B = %+v", services)
	}

	// Suspended or outside the access window: 404 like an unknown slug.
	f.exec(`UPDATE organizations SET status = 'suspended' WHERE id = $1`, f.org.ID)
	if r := f.do(t, http.MethodGet, base, nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("suspended = %d", r.code)
	}
	f.exec(`UPDATE organizations SET status = 'active', access_ends_at = $2 WHERE id = $1`, f.org.ID, f.now.Add(-time.Hour))
	if r := f.do(t, http.MethodGet, base+"/branches", nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("access ended = %d", r.code)
	}
}

func lines(services ...uuid.UUID) []map[string]any {
	out := []map[string]any{}
	for _, s := range services {
		out = append(out, map[string]any{"service_uuid": s})
	}
	return out
}

func (f *fixture) checkout(t *testing.T, slug string, start time.Time, header map[string]string) result {
	t.Helper()
	return f.do(t, http.MethodPost, f.base(slug)+"/checkouts", map[string]any{
		"branch_uuid": f.branch, "lines": lines(f.cut), "starts_at": start,
	}, header)
}

func customer(phone string) map[string]any {
	return map[string]any{"first_name": "Zeynep", "last_name": "Kaya", "phone": phone, "email": "Zeynep@Example.test"}
}

// TestPublicBookingFlow: availability on the online channel, a checkout
// hold, booking it for a customer found or created by phone, the replay of
// a retried booking, the status route behind the access token, releasing
// and expired checkouts, and the channel gate.
func TestPublicBookingFlow(t *testing.T) {
	f := newFixture(t)
	base := f.base(f.org.Slug)

	// Availability: Ayşe's free starts; the hidden person and service never
	// show up.
	r := f.do(t, http.MethodPost, base+"/availability", map[string]any{
		"branch_uuid": f.branch, "lines": lines(f.cut), "from": "2030-01-07", "to": "2030-01-07",
	}, nil)
	if r.code != http.StatusOK {
		t.Fatalf("availability = %d %+v", r.code, r.env.Error)
	}
	av := decode[model.Availability](t, r)
	if len(av.Days) != 1 || av.Days[0].Status != "available" || len(av.Days[0].Slots) == 0 ||
		av.Days[0].Slots[0].Lines[0].Staff == nil || len(av.Lines[0].Staff) != 1 || av.Lines[0].Staff[0].UUID != f.ayse {
		t.Fatalf("availability = %+v", av)
	}
	if strings.Contains(string(r.env.Data), "buffer") || strings.Contains(string(r.env.Data), "resources") {
		t.Fatalf("availability leaks internal fields: %s", r.env.Data)
	}
	for name, body := range map[string]map[string]any{
		"hidden branch":   {"branch_uuid": f.hidden, "lines": lines(f.cut)},
		"foreign branch":  {"branch_uuid": f.branch2, "lines": lines(f.cut)},
		"hidden service":  {"branch_uuid": f.branch, "lines": lines(f.secret)},
		"foreign service": {"branch_uuid": f.branch, "lines": lines(f.cut2)},
		"hidden staff":    {"branch_uuid": f.branch, "lines": []map[string]any{{"service_uuid": f.cut, "staff_uuid": f.gizli}}},
		"bad channel":     {"branch_uuid": f.branch, "lines": lines(f.cut), "channel": "fax"},
	} {
		if r := f.do(t, http.MethodPost, base+"/availability", body, nil); r.code != http.StatusBadRequest {
			t.Fatalf("%s = %d %s", name, r.code, r.detail())
		}
	}
	r = f.do(t, http.MethodPost, base+"/availability", map[string]any{"branch_uuid": f.branch, "lines": lines(f.cut), "channel": "widget"}, nil)
	if r.code != http.StatusConflict || r.env.Error.Code != handler.CodeBookingUnavailable || r.detail() != "channel/channel_disabled" {
		t.Fatalf("widget = %d %+v", r.code, r.env.Error)
	}

	// Checkout: the time is held; another customer cannot take it.
	r = f.checkout(t, f.org.Slug, at(9, 0), nil)
	if r.code != http.StatusCreated {
		t.Fatalf("checkout = %d %+v", r.code, r.env.Error)
	}
	co := decode[model.Checkout](t, r)
	if co.Status != model.CheckoutActive || co.ExpiresAt == nil || !co.ExpiresAt.Equal(f.now.Add(10*time.Minute)) ||
		len(co.Lines) != 1 || co.Lines[0].Staff == nil || co.Lines[0].Staff.UUID != f.ayse || !co.StartsAt.Equal(at(9, 0)) {
		t.Fatalf("checkout = %+v", co)
	}
	if r := f.checkout(t, f.org.Slug, at(9, 0), nil); r.code != http.StatusConflict || r.detail() != "starts_at/taken" {
		t.Fatalf("second checkout = %d %s", r.code, r.detail())
	}
	if r := f.do(t, http.MethodGet, f.base(f.org2.Slug)+"/checkouts/"+co.Token.String(), nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("checkout via B = %d", r.code)
	}

	book := func(token uuid.UUID, cust map[string]any, extra map[string]any) result {
		body := map[string]any{"checkout_token": token, "customer": cust, "customer_note": "Kısa kesim"}
		for k, v := range extra {
			body[k] = v
		}
		return f.do(t, http.MethodPost, base+"/reservations", body, nil)
	}
	for name, tc := range map[string]struct {
		cust  map[string]any
		extra map[string]any
		want  string
	}{
		"honeypot":      {customer("05321234567"), map[string]any{"website": "http://spam"}, "website/rejected"},
		"no phone":      {customer(""), nil, "customer.phone/required"},
		"bad phone":     {customer("12"), nil, "customer.phone/invalid"},
		"no name":       {map[string]any{"phone": "05321234567"}, nil, "customer.first_name/required"},
		"bad email":     {map[string]any{"first_name": "Z", "phone": "05321234567", "email": "x@"}, nil, "customer.email/invalid"},
		"bad payment":   {customer("05321234567"), map[string]any{"payment": "cash"}, "payment/invalid"},
		"foreign token": {customer("05321234567"), map[string]any{"checkout_token": uuid.New()}, "checkout_token/not_found"},
	} {
		if r := book(co.Token, tc.cust, tc.extra); r.code != http.StatusBadRequest || r.detail() != tc.want {
			t.Fatalf("%s = %d %s", name, r.code, r.detail())
		}
	}
	if n := f.count(`SELECT count(*) FROM customers WHERE organization_id = $1`, f.org.ID); n != 0 {
		t.Fatalf("customers after refused bookings = %d", n)
	}

	r = book(co.Token, customer("0532 123 45 67"), nil)
	if r.code != http.StatusCreated {
		t.Fatalf("book = %d %+v", r.code, r.env.Error)
	}
	res := decode[model.Reservation](t, r)
	if res.Status != "active" || res.ApprovalStatus != "accepted" || res.PaymentStatus != "on_site" || res.AccessToken == "" ||
		!res.StartsAt.Equal(at(9, 0)) || res.TotalPrice.String() != "150.00" || res.Lines[0].Staff == nil ||
		res.Lines[0].Staff.Name != "Ayşe" || res.CustomerNote != "Kısa kesim" || res.Timezone != "Europe/Istanbul" {
		t.Fatalf("reservation = %+v", res)
	}
	if strings.Contains(string(r.env.Data), "internal_note") || strings.Contains(string(r.env.Data), "events") {
		t.Fatalf("reservation leaks internal fields: %s", r.env.Data)
	}
	var source, firstName, email, phone string
	if err := f.pool.QueryRow(context.Background(), `SELECT r.source, c.first_name, c.email, c.phone_e164 FROM reservations r
		JOIN customers c ON c.organization_id = r.organization_id AND c.id = r.customer_id
		WHERE r.organization_id = $1 AND r.uuid = $2`, f.org.ID, res.UUID).Scan(&source, &firstName, &email, &phone); err != nil {
		t.Fatal(err)
	}
	if source != "online" || firstName != "Zeynep" || email != "zeynep@example.test" || phone != "+905321234567" {
		t.Fatalf("record = %s %s %s %s", source, firstName, email, phone)
	}
	if n := f.count(`SELECT count(*) FROM customers WHERE organization_id = $1 AND source = 'online'`, f.org.ID); n != 1 {
		t.Fatalf("online customers = %d", n)
	}
	// One active commitment: the checkout hold was freed and booked again.
	if n := f.count(`SELECT count(*) FROM capacity_holds WHERE organization_id = $1 AND status = 'active'`, f.org.ID); n != 1 {
		t.Fatalf("active holds = %d", n)
	}

	// A retry answers the same reservation; the checkout reads booked.
	r = book(co.Token, customer("05321234567"), nil)
	if r.code != http.StatusOK || r.header.Get("Idempotent-Replayed") != "true" || decode[model.Reservation](t, r).UUID != res.UUID {
		t.Fatalf("replay = %d %+v", r.code, r.env.Error)
	}
	if n := f.count(`SELECT count(*) FROM reservations WHERE organization_id = $1`, f.org.ID); n != 1 {
		t.Fatalf("reservations = %d", n)
	}
	r = f.do(t, http.MethodGet, base+"/checkouts/"+co.Token.String(), nil, nil)
	if got := decode[model.Checkout](t, r); got.Status != model.CheckoutBooked || got.ReservationUUID == nil || *got.ReservationUUID != res.UUID {
		t.Fatalf("booked checkout = %+v", got)
	}

	// Status: only with the access token, only under its business.
	status := base + "/reservations/" + res.UUID.String()
	r = f.do(t, http.MethodGet, status, nil, map[string]string{handler.ReservationTokenHeader: res.AccessToken})
	if got := decode[model.Reservation](t, r); r.code != http.StatusOK || got.UUID != res.UUID || got.AccessToken != "" {
		t.Fatalf("status = %d %+v", r.code, got)
	}
	for name, tc := range map[string]struct {
		path, token string
	}{
		"no token":    {status, ""},
		"wrong token": {status, strings.Repeat("a", 64)},
		"other slug":  {f.base(f.org2.Slug) + "/reservations/" + res.UUID.String(), res.AccessToken},
		"bad uuid":    {base + "/reservations/x", res.AccessToken},
	} {
		if r := f.do(t, http.MethodGet, tc.path, nil, map[string]string{handler.ReservationTokenHeader: tc.token}); r.code != http.StatusNotFound {
			t.Fatalf("%s = %d", name, r.code)
		}
	}

	// The same phone books again: the existing record is reused, unchanged.
	r = f.checkout(t, f.org.Slug, at(10, 0), nil)
	co2 := decode[model.Checkout](t, r)
	r = book(co2.Token, map[string]any{"first_name": "Başkası", "phone": "+905321234567"}, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("second booking = %d %+v", r.code, r.env.Error)
	}
	if n := f.count(`SELECT count(*) FROM customers WHERE organization_id = $1 AND first_name = 'Zeynep' AND deleted_at IS NULL`, f.org.ID); n != 1 {
		t.Fatalf("customers = %d", n)
	}
	if n := f.count(`SELECT count(*) FROM customers WHERE organization_id = $1`, f.org.ID); n != 1 {
		t.Fatalf("all customers = %d", n)
	}

	// Releasing a checkout frees its time.
	r = f.checkout(t, f.org.Slug, at(11, 0), nil)
	co3 := decode[model.Checkout](t, r)
	r = f.do(t, http.MethodDelete, base+"/checkouts/"+co3.Token.String(), nil, nil)
	if got := decode[model.Checkout](t, r); r.code != http.StatusOK || got.Status != model.CheckoutReleased {
		t.Fatalf("released = %d %+v", r.code, got)
	}
	if r := f.checkout(t, f.org.Slug, at(11, 0), nil); r.code != http.StatusCreated {
		t.Fatalf("checkout after release = %d %+v", r.code, r.env.Error)
	}

	// An expired checkout still books while its time is free.
	r = f.checkout(t, f.org.Slug, at(11, 30), nil)
	co4 := decode[model.Checkout](t, r)
	f.now = f.now.Add(11 * time.Minute)
	r = f.do(t, http.MethodGet, base+"/checkouts/"+co4.Token.String(), nil, nil)
	if got := decode[model.Checkout](t, r); got.Status != model.CheckoutExpired {
		t.Fatalf("lapsed checkout = %+v", got)
	}
	r = book(co4.Token, customer("05559876543"), map[string]any{"payment": "online"})
	if r.code != http.StatusCreated {
		t.Fatalf("expired checkout booking = %d %+v", r.code, r.env.Error)
	}
	if got := decode[model.Reservation](t, r); got.Status != "requested" || got.PaymentStatus != "online_pending" || got.ExpiresAt == nil {
		t.Fatalf("online payment booking = %+v", got)
	}

	// Channel switched off: catalog still reads, bookings are refused.
	f.saveSettings(t, f.org.ID, "auto", false)
	r = f.checkout(t, f.org.Slug, at(9, 30), nil)
	if r.code != http.StatusConflict || r.detail() != "channel/channel_disabled" {
		t.Fatalf("checkout with web off = %d %s", r.code, r.detail())
	}
	r = f.do(t, http.MethodPost, base+"/checkouts", map[string]any{
		"branch_uuid": f.branch, "lines": lines(f.cut), "starts_at": at(9, 30), "channel": "api",
	}, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("api checkout = %d %+v", r.code, r.env.Error)
	}
	if r := f.do(t, http.MethodGet, base+"/services", nil, nil); r.code != http.StatusOK {
		t.Fatalf("catalog with web off = %d", r.code)
	}
}

// TestPublicBookingRules: approval by staff makes a request; settings that
// are not ready refuse public bookings; an idempotency key replays a
// checkout; rate limits answer 429 with Retry-After.
func TestPublicBookingRules(t *testing.T) {
	f := newFixture(t)
	base := f.base(f.org.Slug)

	f.saveSettings(t, f.org.ID, "staff", true)
	r := f.checkout(t, f.org.Slug, at(9, 0), nil)
	co := decode[model.Checkout](t, r)
	r = f.do(t, http.MethodPost, base+"/reservations", map[string]any{"checkout_token": co.Token, "customer": customer("05321234567")}, nil)
	if got := decode[model.Reservation](t, r); r.code != http.StatusCreated || got.Status != "requested" || got.ApprovalStatus != "pending" || got.ExpiresAt == nil {
		t.Fatalf("request = %d %+v", r.code, got)
	}

	// Not ready: the approval mode was never set.
	f.exec(`DELETE FROM organization_settings_versions WHERE organization_id = $1`, f.org.ID)
	r = f.do(t, http.MethodGet, base, nil, nil)
	if biz := decode[model.Business](t, r); biz.Booking.Ready || len(biz.Booking.Unavailable) == 0 || biz.Booking.Unavailable[0] != model.UnavailableNotReady {
		t.Fatalf("not ready profile = %+v", biz.Booking)
	}
	if r := f.checkout(t, f.org.Slug, at(10, 0), nil); r.code != http.StatusConflict || r.detail() != "channel/online_not_ready" {
		t.Fatalf("not ready checkout = %d %s", r.code, r.detail())
	}
	f.saveSettings(t, f.org.ID, "auto", true)

	// Idempotency-Key: a retried checkout replays the first answer.
	key := map[string]string{"Idempotency-Key": uuid.NewString()}
	first := f.checkout(t, f.org.Slug, at(10, 0), key)
	second := f.checkout(t, f.org.Slug, at(10, 0), key)
	if first.code != http.StatusCreated || second.code != http.StatusCreated || second.header.Get("Idempotent-Replayed") != "true" ||
		decode[model.Checkout](t, first).Token != decode[model.Checkout](t, second).Token {
		t.Fatalf("idempotent checkout = %d / %d %q", first.code, second.code, second.header.Get("Idempotent-Replayed"))
	}

	// Rate limits.
	mr := miniredis.RunT(t)
	f.h.SetRateLimiter(ratelimit.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), "test"))
	limits := handler.DefaultLimits
	limits.Read = handler.Limit{Max: 2, Window: time.Minute}
	limits.BookingPhone = handler.Limit{Max: 1, Window: time.Hour}
	f.h.SetLimits(limits)
	for i := 0; i < 2; i++ {
		if r := f.do(t, http.MethodGet, base+"/branches", nil, nil); r.code != http.StatusOK {
			t.Fatalf("read %d = %d", i, r.code)
		}
	}
	r = f.do(t, http.MethodGet, base+"/branches", nil, nil)
	if r.code != http.StatusTooManyRequests || r.env.Error.Code != "RATE_LIMITED" || r.header.Get("Retry-After") == "" {
		t.Fatalf("over the read limit = %d %q", r.code, r.header.Get("Retry-After"))
	}
	r = f.checkout(t, f.org.Slug, at(11, 0), nil)
	co = decode[model.Checkout](t, r)
	body := map[string]any{"checkout_token": co.Token, "customer": customer("05001112233")}
	if r := f.do(t, http.MethodPost, base+"/reservations", body, nil); r.code != http.StatusCreated {
		t.Fatalf("booking = %d %+v", r.code, r.env.Error)
	}
	body["checkout_token"] = uuid.New()
	if r := f.do(t, http.MethodPost, base+"/reservations", body, nil); r.code != http.StatusTooManyRequests {
		t.Fatalf("second booking of the phone = %d", r.code)
	}
}
