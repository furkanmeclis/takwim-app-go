package customerpanel_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	availabilitymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/usecase"
	customersrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/repository"
	customersusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	orgsettingsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/idempotency"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
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

// capture records the messages instead of sending them.
type capture struct {
	mu   sync.Mutex
	msgs []sms.Message
}

func (c *capture) Send(_ context.Context, m sms.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = append(c.msgs, m)
	return nil
}

func (c *capture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.msgs)
}

var codeRE = regexp.MustCompile(`\b(\d{6})\b`)

// lastCode is the code of the last message to phone.
func (c *capture) lastCode(t *testing.T, phone string) string {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.msgs) - 1; i >= 0; i-- {
		if c.msgs[i].To == phone && c.msgs[i].Purpose == sms.PurposeCustomerOTP {
			return codeRE.FindStringSubmatch(c.msgs[i].Text)[1]
		}
	}
	t.Fatalf("no code sent to %s", phone)
	return ""
}

// Hüseyin (+905321112233) is a customer of business A (kuaför) and of
// business B (klinik); Zeynep (+905329998877) is another customer of A.
// Each business has a branch with Kesim (30 min) and Ayşe working 09–12 on
// Mondays. Settings: automatic approval, cancel cutoff 120 minutes, change
// cutoff 60 minutes. The clock is Monday 08:00.
type fixture struct {
	pool                      *pgxpool.Pool
	q                         *db.Queries
	orgA, orgB                orgmodel.Organization
	owner                     db.User
	branchA, branchB          uuid.UUID
	cutA, cutB                uuid.UUID
	ayseA, ayseB              uuid.UUID
	huseyinA, huseyinB        uuid.UUID
	zeynep                    uuid.UUID
	settings                  *orgsettingsusecase.Service
	reservations              *reservationsusecase.Service
	svc                       *usecase.Service
	sender                    *capture
	tokens                    *jwt.Manager
	mux                       http.Handler
	now                       time.Time
	exec                      func(sql string, args ...any)
	count                     func(sql string, args ...any) int
	huseyinPhone, zeynepPhone string
	// extraPhones are removed from the platform tables after the test.
	extraPhones []string
	suffix      string
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
	// Unique phones per run: the identity table is platform-wide.
	digits := func(s string) string {
		var b strings.Builder
		for _, r := range s {
			b.WriteRune('0' + r%10)
		}
		return b.String()
	}
	f := &fixture{pool: pool, q: q, now: at(8, 0), sender: &capture{}, suffix: digits(suffix),
		huseyinPhone: "+90532" + digits(suffix)[:7], zeynepPhone: "+90533" + digits(suffix)[:7]}
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
	f.orgA, err = orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Kuaför " + suffix, OwnerUserID: f.owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	f.orgB, err = orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Klinik " + suffix, OwnerUserID: other.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{f.orgA.ID, f.orgB.ID})
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{f.owner.ID, other.ID})
		phones := append([]string{f.huseyinPhone, f.zeynepPhone}, f.extraPhones...)
		for _, table := range []string{"customer_otp_challenges", "customer_access_links", "customer_phone_releases", "customer_auth_events", "customer_identities"} {
			_, _ = pool.Exec(bg, "DELETE FROM "+table+" WHERE phone_e164 = ANY($1)", phones)
		}
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
	setup := func(orgID int64) (branch, cut, ayse uuid.UUID) {
		bID, b := idOf("SELECT id, uuid FROM branches WHERE organization_id = $1 ORDER BY id LIMIT 1", orgID)
		f.exec(`UPDATE branches SET working_hours = $3, timezone = NULL, address = 'Bağdat Cad. 1' WHERE organization_id = $1 AND id = $2`, orgID, bID, hours)
		sID, s := idOf(`INSERT INTO staff (organization_id, name) VALUES ($1, 'Ayşe') RETURNING id, uuid`, orgID)
		f.exec(`INSERT INTO staff_branches (organization_id, staff_id, branch_id, shifts) VALUES
			($1, $2, $3, '{"mon":[{"start":"09:00","end":"12:00","breaks":[]}]}')`, orgID, sID, bID)
		cID, c := idOf(`INSERT INTO services (organization_id, name, duration_minutes, price, payment_mode)
			VALUES ($1, 'Kesim', 30, 100, 'both') RETURNING id, uuid`, orgID)
		f.exec(`INSERT INTO service_branches (organization_id, service_id, branch_id) VALUES ($1, $2, $3)`, orgID, cID, bID)
		f.exec(`INSERT INTO service_staff (organization_id, service_id, staff_id) VALUES ($1, $2, $3)`, orgID, cID, sID)
		return b, c, s
	}
	f.branchA, f.cutA, f.ayseA = setup(f.orgA.ID)
	f.branchB, f.cutB, f.ayseB = setup(f.orgB.ID)
	// Staff typed the numbers; nothing is linked yet.
	_, f.huseyinA = idOf(`INSERT INTO customers (organization_id, first_name, last_name, phone_e164, notes) VALUES ($1, 'Hüseyin', 'K', $2, 'kuaför iç notu') RETURNING id, uuid`, f.orgA.ID, f.huseyinPhone)
	_, f.huseyinB = idOf(`INSERT INTO customers (organization_id, first_name, last_name, phone_e164, notes) VALUES ($1, 'Hüseyin', 'Klinik', $2, 'klinik iç notu') RETURNING id, uuid`, f.orgB.ID, f.huseyinPhone)
	_, f.zeynep = idOf(`INSERT INTO customers (organization_id, first_name, phone_e164) VALUES ($1, 'Zeynep', $2) RETURNING id, uuid`, f.orgA.ID, f.zeynepPhone)

	f.settings = orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))
	f.saveSettings(t, f.orgA.ID)
	f.saveSettings(t, f.orgB.ID)

	clock := func() time.Time { return f.now }
	holds := availabilityusecase.NewHolds(pool, q, f.settings)
	holds.SetClock(clock)
	holds.SetExpiryHook(reservationsusecase.ExpireRequests)
	avail := availabilityusecase.New(q, f.settings)
	avail.SetClock(clock)
	avail.SetBusySource(holds.Busy())
	f.reservations = reservationsusecase.New(holds, q, f.settings)
	cipher, err := crypto.NewFieldCipher(strings.Repeat("k", 32))
	if err != nil {
		t.Fatal(err)
	}
	f.tokens, err = jwt.NewManager("customer-panel-test-secret-0123456789", time.Hour, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	customers := customersusecase.New(customersrepository.NewPostgres(pool, q), nil)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	f.svc = usecase.New(q, f.tokens, cipher, f.sender, customers, usecase.PanelDeps{
		Reservations: f.reservations, Availability: avail, Settings: f.settings,
	}, log)
	f.svc.SetClock(clock)
	f.svc.SetTx(customersrepository.NewPostgres(pool, q))
	h := handler.New(f.svc, nil)
	mux := http.NewServeMux()
	customerpanel.RegisterRoutes(mux, h)
	// A staff route, to prove a customer token never reaches one.
	mux.Handle("GET /v1/staff-probe", middleware.Authenticate(f.tokens, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	f.mux = middleware.Idempotency(idempotency.NewMemoryStore(), f.tokens, log)(mux)
	return f
}

// saveSettings: automatic approval, no minimum notice, 30 days ahead,
// cancel until 2 hours and change until 1 hour before the start.
func (f *fixture) saveSettings(t *testing.T, orgID int64) {
	t.Helper()
	lead, advance, window, payment, cancel, change := 0, 30, 30, 15, 120, 60
	s := orgsettingsmodel.Defaults()
	s.Reservation = orgsettingsmodel.Reservation{
		ApprovalMode: "auto", StaffReviewWindowMinutes: &window, PaymentHoldMinutes: &payment,
		MinLeadMinutes: &lead, MaxAdvanceDays: &advance, CancelCutoffMinutes: &cancel, ChangeCutoffMinutes: &change,
	}
	s.Payment = orgsettingsmodel.Payment{PayAtVenue: true, PayOnline: true}
	s.Policies.Cancellation = "2 saat öncesine kadar ücretsiz iptal"
	s.Policies.Change = "1 saat öncesine kadar değişiklik"
	for i := range s.SalesChannels {
		s.SalesChannels[i].IsActive = true
	}
	if _, err := f.settings.Save(context.Background(), orgID, f.owner.ID, orgsettingsmodel.SaveInput{Settings: s}); err != nil {
		t.Fatal(err)
	}
}

// book books Kesim with Ayşe at start for a customer (panel booking).
func (f *fixture) book(t *testing.T, orgID int64, branch, cut, ayse, customer uuid.UUID, start time.Time) reservationsmodel.Reservation {
	t.Helper()
	staff := ayse
	r, err := f.reservations.Create(context.Background(), orgID, reservationsmodel.CreateInput{
		BranchUUID: branch, CustomerUUID: customer, StartsAt: &start, InternalNote: "personel notu",
		Lines: []availabilitymodel.LineInput{{ServiceUUID: cut, StaffUUID: &staff}},
	}, availabilityusecase.Caller{}, f.owner.ID)
	if err != nil {
		t.Fatalf("book %s: %v", start, err)
	}
	return r
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
	code int
	raw  string
	env  envelope
}

func (r result) detail() string {
	if r.env.Error == nil || len(r.env.Error.Details) == 0 {
		return ""
	}
	return r.env.Error.Details[0].Field + "/" + r.env.Error.Details[0].Code
}

func (r result) errCode() string {
	if r.env.Error == nil {
		return ""
	}
	return r.env.Error.Code
}

func (f *fixture) do(t *testing.T, method, path, token string, body any) result {
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
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	out := result{code: rec.Code, raw: rec.Body.String()}
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out.env); err != nil {
			t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body.String())
		}
	}
	return out
}

func decode[T any](t *testing.T, r result) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(r.env.Data, &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, r.env.Data)
	}
	return out
}

// signIn requests a code for typed (any spelling of the phone) and
// verifies it.
func (f *fixture) signIn(t *testing.T, typed, e164 string) model.Session {
	t.Helper()
	if r := f.do(t, http.MethodPost, "/v1/customer/auth/start", "", map[string]any{"phone": typed}); r.code != http.StatusAccepted {
		t.Fatalf("start = %d %s", r.code, r.raw)
	}
	r := f.do(t, http.MethodPost, "/v1/customer/auth/verify", "", map[string]any{"phone": typed, "code": f.sender.lastCode(t, e164)})
	if r.code != http.StatusOK {
		t.Fatalf("verify = %d %s", r.code, r.raw)
	}
	return decode[model.Session](t, r)
}

// national writes +90XXXXXXXXXX as "0 (5XX) XXX XX XX".
func national(e164 string) string {
	d := strings.TrimPrefix(e164, "+90")
	return "0 (" + d[:3] + ") " + d[3:6] + " " + d[6:8] + " " + d[8:]
}

// TestCustomerPanelSignIn: codes go only to known phones with one generic
// answer, cooldowns and attempt limits hold, a verified phone links the
// records of every business (any spelling), and the customer token is its
// own kind with a revocable, expiring session.
func TestCustomerPanelSignIn(t *testing.T) {
	f := newFixture(t)

	// Unknown phone: the same answer, no message.
	unknown := f.do(t, http.MethodPost, "/v1/customer/auth/start", "", map[string]any{"phone": "+905550000000"})
	if unknown.code != http.StatusAccepted || f.sender.count() != 0 {
		t.Fatalf("unknown start = %d %s (sent %d)", unknown.code, unknown.raw, f.sender.count())
	}
	if r := f.do(t, http.MethodPost, "/v1/customer/auth/start", "", map[string]any{"phone": "12"}); r.code != http.StatusBadRequest || r.detail() != "phone/invalid" {
		t.Fatalf("bad phone = %d %s", r.code, r.raw)
	}
	known := f.do(t, http.MethodPost, "/v1/customer/auth/start", "", map[string]any{"phone": national(f.huseyinPhone)})
	if known.code != http.StatusAccepted || f.sender.count() != 1 {
		t.Fatalf("known start = %d %s (sent %d)", known.code, known.raw, f.sender.count())
	}
	a, b := decode[model.StartResult](t, unknown), decode[model.StartResult](t, known)
	if a.Status != b.Status || a.ResendAfterSeconds != b.ResendAfterSeconds || a.CodeTTLSeconds != b.CodeTTLSeconds || b.Status != model.StartStatus {
		t.Fatalf("answers differ: %+v vs %+v", a, b)
	}
	if strings.Contains(known.raw, f.huseyinPhone) || f.count("SELECT COUNT(*) FROM customer_otp_challenges WHERE code_hash LIKE '%' || $1 || '%'", f.sender.lastCode(t, f.huseyinPhone)) != 0 {
		t.Fatal("phone in the answer or code stored in clear")
	}
	// Cooldown: a second request within a minute sends nothing.
	if r := f.do(t, http.MethodPost, "/v1/customer/auth/start", "", map[string]any{"phone": f.huseyinPhone}); r.code != http.StatusAccepted || f.sender.count() != 1 {
		t.Fatalf("cooldown = %d (sent %d)", r.code, f.sender.count())
	}
	code := f.sender.lastCode(t, f.huseyinPhone)
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	if r := f.do(t, http.MethodPost, "/v1/customer/auth/verify", "", map[string]any{"phone": f.huseyinPhone, "code": wrong}); r.code != http.StatusBadRequest || r.errCode() != handler.CodeInvalidCode {
		t.Fatalf("wrong code = %d %s", r.code, r.raw)
	}
	// Another phone never uses Hüseyin's code.
	if r := f.do(t, http.MethodPost, "/v1/customer/auth/verify", "", map[string]any{"phone": f.zeynepPhone, "code": code}); r.code != http.StatusBadRequest {
		t.Fatalf("other phone = %d", r.code)
	}
	if n := f.count("SELECT COUNT(*) FROM customer_identity_links WHERE organization_id = ANY($1)", []int64{f.orgA.ID, f.orgB.ID}); n != 0 {
		t.Fatalf("links before verification = %d", n)
	}
	r := f.do(t, http.MethodPost, "/v1/customer/auth/verify", "", map[string]any{"phone": "+90 " + strings.TrimPrefix(f.huseyinPhone, "+90"), "code": code})
	if r.code != http.StatusOK {
		t.Fatalf("verify = %d %s", r.code, r.raw)
	}
	session := decode[model.Session](t, r)
	if session.AccessToken == "" || session.TokenType != "Bearer" || len(session.Profile.Businesses) != 2 || session.Profile.Phone != f.huseyinPhone {
		t.Fatalf("session = %+v", session)
	}
	// A23: both businesses' records (typed in different places) are linked
	// to one identity through the verified phone; Zeynep's is not.
	if n := f.count(`SELECT COUNT(DISTINCT l.identity_id) FROM customer_identity_links l WHERE l.organization_id = ANY($1) AND l.source = 'panel_login'`, []int64{f.orgA.ID, f.orgB.ID}); n != 1 {
		t.Fatalf("identities = %d", n)
	}
	if n := f.count(`SELECT COUNT(*) FROM customer_identity_links l WHERE l.organization_id = ANY($1)`, []int64{f.orgA.ID, f.orgB.ID}); n != 2 {
		t.Fatalf("links = %d", n)
	}
	if strings.Contains(r.raw, "iç notu") {
		t.Fatal("business notes in the session answer")
	}
	// The code is single use.
	if r := f.do(t, http.MethodPost, "/v1/customer/auth/verify", "", map[string]any{"phone": f.huseyinPhone, "code": code}); r.code != http.StatusBadRequest {
		t.Fatalf("reused code = %d", r.code)
	}

	tok := session.AccessToken
	if r := f.do(t, http.MethodGet, "/v1/customer/me", tok, nil); r.code != http.StatusOK {
		t.Fatalf("me = %d %s", r.code, r.raw)
	}
	// Token kinds never cross.
	if r := f.do(t, http.MethodGet, "/v1/staff-probe", tok, nil); r.code != http.StatusUnauthorized {
		t.Fatalf("customer token on a staff route = %d", r.code)
	}
	staff, _, err := f.tokens.IssueAccess(jwt.AccessInput{UserID: f.owner.Uuid})
	if err != nil {
		t.Fatal(err)
	}
	if r := f.do(t, http.MethodGet, "/v1/customer/me", staff, nil); r.code != http.StatusUnauthorized {
		t.Fatalf("staff token on the panel = %d", r.code)
	}
	for _, bad := range []string{"", "garbage", tok + "x"} {
		if r := f.do(t, http.MethodGet, "/v1/customer/reservations", bad, nil); r.code != http.StatusUnauthorized {
			t.Fatalf("token %q = %d", bad, r.code)
		}
	}

	// Attempt limit: five wrong tries lock the code; the right one fails.
	f.now = f.now.Add(2 * time.Minute)
	if r := f.do(t, http.MethodPost, "/v1/customer/auth/start", "", map[string]any{"phone": f.huseyinPhone}); r.code != http.StatusAccepted {
		t.Fatalf("second start = %d", r.code)
	}
	code = f.sender.lastCode(t, f.huseyinPhone)
	wrong = "000000"
	if code == wrong {
		wrong = "111111"
	}
	for i := 0; i < 5; i++ {
		f.do(t, http.MethodPost, "/v1/customer/auth/verify", "", map[string]any{"phone": f.huseyinPhone, "code": wrong})
	}
	if r := f.do(t, http.MethodPost, "/v1/customer/auth/verify", "", map[string]any{"phone": f.huseyinPhone, "code": code}); r.code != http.StatusBadRequest {
		t.Fatalf("locked code = %d", r.code)
	}
	// Expiry: a code older than its TTL fails.
	f.now = f.now.Add(2 * time.Minute)
	f.do(t, http.MethodPost, "/v1/customer/auth/start", "", map[string]any{"phone": f.huseyinPhone})
	code = f.sender.lastCode(t, f.huseyinPhone)
	f.now = f.now.Add(6 * time.Minute)
	if r := f.do(t, http.MethodPost, "/v1/customer/auth/verify", "", map[string]any{"phone": f.huseyinPhone, "code": code}); r.code != http.StatusBadRequest {
		t.Fatalf("expired code = %d", r.code)
	}

	// Session idle expiry slides with use; a week of silence ends it.
	f.now = f.now.Add(6 * 24 * time.Hour)
	if r := f.do(t, http.MethodGet, "/v1/customer/me", tok, nil); r.code != http.StatusOK {
		t.Fatalf("me after 6 days = %d", r.code)
	}
	f.now = f.now.Add(6 * 24 * time.Hour)
	if r := f.do(t, http.MethodGet, "/v1/customer/me", tok, nil); r.code != http.StatusOK {
		t.Fatalf("me after 12 days (used) = %d", r.code)
	}
	f.now = f.now.Add(8 * 24 * time.Hour)
	if r := f.do(t, http.MethodGet, "/v1/customer/me", tok, nil); r.code != http.StatusUnauthorized {
		t.Fatalf("me after a silent week = %d", r.code)
	}

	// Logout revokes the session; everywhere revokes all of them.
	f.now = f.now.Add(time.Hour)
	s1 := f.signIn(t, f.huseyinPhone, f.huseyinPhone)
	f.now = f.now.Add(2 * time.Minute)
	s2 := f.signIn(t, f.huseyinPhone, f.huseyinPhone)
	if r := f.do(t, http.MethodPost, "/v1/customer/auth/logout", s1.AccessToken, nil); r.code != http.StatusNoContent {
		t.Fatalf("logout = %d %s", r.code, r.raw)
	}
	if r := f.do(t, http.MethodGet, "/v1/customer/me", s1.AccessToken, nil); r.code != http.StatusUnauthorized {
		t.Fatalf("after logout = %d", r.code)
	}
	if r := f.do(t, http.MethodGet, "/v1/customer/me", s2.AccessToken, nil); r.code != http.StatusOK {
		t.Fatalf("other session = %d", r.code)
	}
	f.do(t, http.MethodPost, "/v1/customer/auth/logout", s2.AccessToken, map[string]any{"everywhere": true})
	if r := f.do(t, http.MethodGet, "/v1/customer/me", s2.AccessToken, nil); r.code != http.StatusUnauthorized {
		t.Fatalf("after logout everywhere = %d", r.code)
	}

	// Hourly cap: at most five codes per phone in an hour.
	f.now = f.now.Add(2 * time.Hour)
	before := f.sender.count()
	for i := 0; i < 7; i++ {
		f.do(t, http.MethodPost, "/v1/customer/auth/start", "", map[string]any{"phone": f.zeynepPhone})
		f.now = f.now.Add(61 * time.Second)
	}
	if sent := f.sender.count() - before; sent != 5 {
		t.Fatalf("codes in an hour = %d", sent)
	}

}

// TestCustomerPanelReservations: the person sees their own visits in every
// linked business and nothing else; cancel and reschedule follow the
// business rules through the reservation use cases.
func TestCustomerPanelReservations(t *testing.T) {
	f := newFixture(t)
	f.now = at(7, 0)
	resA := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, at(10, 0))
	resB := f.book(t, f.orgB.ID, f.branchB, f.cutB, f.ayseB, f.huseyinB, at(11, 0))
	done := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, at(9, 0))
	zeynep := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.zeynep, at(11, 0))
	// A paid online visit in B (payment state as the payment layer sets it).
	paid := f.book(t, f.orgB.ID, f.branchB, f.cutB, f.ayseB, f.huseyinB, at(9, 30))
	f.exec("UPDATE reservations SET payment_status = 'paid' WHERE organization_id = $1 AND uuid = $2", f.orgB.ID, paid.UUID)
	if _, err := f.reservations.Complete(context.Background(), f.orgA.ID, done.UUID, availabilityusecase.Caller{}, f.owner.ID, reservationsusecase.ChangeInput{}); err != nil {
		t.Fatal(err)
	}

	tok := f.signIn(t, national(f.huseyinPhone), f.huseyinPhone).AccessToken

	type page struct {
		Items []model.ReservationItem `json:"items"`
		Total int64                   `json:"total"`
	}
	r := f.do(t, http.MethodGet, "/v1/customer/reservations?scope=upcoming", tok, nil)
	up := decode[page](t, r)
	if up.Total != 3 || up.Items[0].UUID != paid.UUID || up.Items[1].UUID != resA.UUID || up.Items[2].UUID != resB.UUID {
		t.Fatalf("upcoming = %+v", up)
	}
	if up.Items[2].Business.UUID != f.orgB.UUID || up.Items[2].Services[0] != "Kesim" || up.Items[2].Staff[0] != "Ayşe" || up.Items[2].Timezone != "Europe/Istanbul" {
		t.Fatalf("item = %+v", up.Items[2])
	}
	if strings.Contains(r.raw, zeynep.UUID.String()) || strings.Contains(r.raw, "personel notu") || strings.Contains(r.raw, "iç notu") {
		t.Fatalf("leak in list: %s", r.raw)
	}
	past := decode[page](t, f.do(t, http.MethodGet, "/v1/customer/reservations?scope=past", tok, nil))
	if past.Total != 1 || past.Items[0].UUID != done.UUID || past.Items[0].Money.Balance != nil || past.Items[0].Money.Method != model.MethodOnSite {
		t.Fatalf("past = %+v", past)
	}
	onlyB := decode[page](t, f.do(t, http.MethodGet, "/v1/customer/reservations?business="+f.orgB.UUID.String(), tok, nil))
	if onlyB.Total != 2 {
		t.Fatalf("business filter = %+v", onlyB)
	}
	for _, q := range []string{"?scope=later", "?sort=number", "?page=2", "?business=x", "?status=gone"} {
		if r := f.do(t, http.MethodGet, "/v1/customer/reservations"+q, tok, nil); r.code != http.StatusBadRequest {
			t.Fatalf("%s = %d", q, r.code)
		}
	}

	// Detail: own visit with policies and actions; no internal fields.
	r = f.do(t, http.MethodGet, "/v1/customer/reservations/"+resA.UUID.String(), tok, nil)
	detail := decode[model.Reservation](t, r)
	if detail.Business.Name != f.orgA.Name || detail.Branch.Address != "Bağdat Cad. 1" || len(detail.Lines) != 1 ||
		detail.Policies.Cancellation == "" || !detail.Actions.Cancel.Allowed || !detail.Actions.Reschedule.Allowed ||
		!detail.Actions.Cancel.Until.Equal(at(8, 0)) || len(detail.History) == 0 {
		t.Fatalf("detail = %+v", detail)
	}
	for _, leak := range []string{"personel notu", "internal_note", "iç notu", "staff_share", "\"resources\"", f.owner.Uuid.String()} {
		if strings.Contains(r.raw, leak) {
			t.Fatalf("detail leaks %q: %s", leak, r.raw)
		}
	}
	// Another person's visit, an unknown and a malformed id: 404 on every
	// route, and nothing changes.
	for _, id := range []string{zeynep.UUID.String(), uuid.NewString(), "not-a-uuid"} {
		for _, call := range []struct{ method, path string }{
			{http.MethodGet, ""}, {http.MethodPost, "/cancel"}, {http.MethodPost, "/availability"}, {http.MethodPost, "/reschedule"},
		} {
			body := any(nil)
			if call.method == http.MethodPost {
				body = map[string]any{}
			}
			if r := f.do(t, call.method, "/v1/customer/reservations/"+id+call.path, tok, body); r.code != http.StatusNotFound {
				t.Fatalf("%s %s%s = %d %s", call.method, id, call.path, r.code, r.raw)
			}
		}
	}
	if n := f.count("SELECT COUNT(*) FROM reservations WHERE organization_id = $1 AND uuid = $2 AND status = 'active'", f.orgA.ID, zeynep.UUID); n != 1 {
		t.Fatal("another person's visit changed")
	}

	// Reschedule A's 10:00 visit: the search offers free starts on the
	// online channel; 11:00 is Zeynep's (taken), 10:30 is free.
	r = f.do(t, http.MethodPost, "/v1/customer/reservations/"+resA.UUID.String()+"/availability", tok, map[string]any{"from": "2030-01-07", "to": "2030-01-07"})
	if r.code != http.StatusOK {
		t.Fatalf("availability = %d %s", r.code, r.raw)
	}
	avail := decode[model.Availability](t, r)
	free := map[time.Time]bool{}
	for _, d := range avail.Days {
		for _, s := range d.Starts {
			free[s.UTC()] = true
		}
	}
	if !free[at(10, 30).UTC()] || free[at(11, 0).UTC()] || free[at(10, 0).UTC()] {
		t.Fatalf("starts = %v", avail.Days)
	}
	if r := f.do(t, http.MethodPost, "/v1/customer/reservations/"+resA.UUID.String()+"/reschedule", tok, map[string]any{"starts_at": at(11, 0)}); r.code != http.StatusConflict || r.detail() != "starts_at/taken" {
		t.Fatalf("taken = %d %s", r.code, r.raw)
	}
	if r := f.do(t, http.MethodPost, "/v1/customer/reservations/"+resA.UUID.String()+"/reschedule", tok, map[string]any{"starts_at": at(10, 30), "expected_version": 99}); r.code != http.StatusConflict || r.detail() != "expected_version/stale" {
		t.Fatalf("stale = %d %s", r.code, r.raw)
	}
	r = f.do(t, http.MethodPost, "/v1/customer/reservations/"+resA.UUID.String()+"/reschedule", tok, map[string]any{"starts_at": at(10, 30), "expected_version": detail.Version})
	moved := decode[model.Reservation](t, r)
	if r.code != http.StatusOK || !moved.StartsAt.Equal(at(10, 30)) || moved.Version != detail.Version+1 || moved.History[len(moved.History)-1].Type != "rescheduled" {
		t.Fatalf("reschedule = %d %s", r.code, r.raw)
	}
	// The old time is free for the business again.
	f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.zeynep, at(10, 0))

	// Cancel B's 11:00 visit (before the 2 hour cutoff).
	r = f.do(t, http.MethodPost, "/v1/customer/reservations/"+resB.UUID.String()+"/cancel", tok, map[string]any{})
	cancelled := decode[model.Reservation](t, r)
	if r.code != http.StatusOK || cancelled.Status != reservationsmodel.StatusCancelled || cancelled.CancelReason == nil ||
		*cancelled.CancelReason != usecase.CancelReason || cancelled.Actions.Cancel.Allowed {
		t.Fatalf("cancel = %d %s", r.code, r.raw)
	}
	if r := f.do(t, http.MethodPost, "/v1/customer/reservations/"+resB.UUID.String()+"/cancel", tok, map[string]any{}); r.code != http.StatusConflict || r.detail() != "cancel/status" {
		t.Fatalf("cancel twice = %d %s", r.code, r.raw)
	}
	f.book(t, f.orgB.ID, f.branchB, f.cutB, f.ayseB, f.huseyinB, at(11, 0))

	// Cutoffs: at 08:45 the 10:30 visit can no longer be cancelled (2 h)
	// but can still be moved (1 h); at 09:45 neither.
	f.now = at(8, 45)
	r = f.do(t, http.MethodPost, "/v1/customer/reservations/"+resA.UUID.String()+"/cancel", tok, map[string]any{})
	if r.code != http.StatusConflict || r.errCode() != handler.CodeActionNotAllowed || r.detail() != "cancel/cutoff" {
		t.Fatalf("cancel after cutoff = %d %s", r.code, r.raw)
	}
	detail = decode[model.Reservation](t, f.do(t, http.MethodGet, "/v1/customer/reservations/"+resA.UUID.String(), tok, nil))
	if detail.Actions.Cancel.Allowed || *detail.Actions.Cancel.Reason != model.ReasonCutoff || !detail.Actions.Reschedule.Allowed {
		t.Fatalf("actions at 08:45 = %+v", detail.Actions)
	}
	f.now = at(9, 45)
	if r := f.do(t, http.MethodPost, "/v1/customer/reservations/"+resA.UUID.String()+"/reschedule", tok, map[string]any{"starts_at": at(11, 30)}); r.code != http.StatusConflict || r.detail() != "reschedule/cutoff" {
		t.Fatalf("reschedule after cutoff = %d %s", r.code, r.raw)
	}

	// Payments: the paid visit counts as paid; the cancelled unpaid one is
	// left out; currencies are summed apart.
	pays := decode[page](t, f.do(t, http.MethodGet, "/v1/customer/payments", tok, nil))
	for _, it := range pays.Items {
		if it.UUID == resB.UUID {
			t.Fatal("cancelled unpaid visit in payments")
		}
		if it.UUID == paid.UUID && (it.Money.Paid.String() != "100.00" || it.Money.Balance == nil || it.Money.Balance.String() != "0.00" || it.Money.Method != model.MethodOnline) {
			t.Fatalf("paid item = %+v", it.Money)
		}
	}
	summary := decode[model.PaymentSummary](t, f.do(t, http.MethodGet, "/v1/customer/payments/summary", tok, nil))
	if len(summary.Totals) != 1 || summary.Totals[0].Currency != "TRY" || summary.Totals[0].Paid.String() != "100.00" ||
		summary.Totals[0].Booked.String() != "400.00" || summary.Totals[0].ReservationCount != 4 {
		t.Fatalf("summary = %+v", summary)
	}

	// Zeynep's session sees only her visits, never Hüseyin's.
	f.now = at(9, 50)
	ztok := f.signIn(t, f.zeynepPhone, f.zeynepPhone).AccessToken
	zp := decode[page](t, f.do(t, http.MethodGet, "/v1/customer/reservations", ztok, nil))
	if zp.Total != 2 {
		t.Fatalf("zeynep list = %+v", zp)
	}
	if r := f.do(t, http.MethodGet, "/v1/customer/reservations/"+resA.UUID.String(), ztok, nil); r.code != http.StatusNotFound {
		t.Fatalf("zeynep reads hüseyin's visit = %d", r.code)
	}
}
