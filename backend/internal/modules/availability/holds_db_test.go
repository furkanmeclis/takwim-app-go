package availability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability"
	availabilityhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	orgsettingsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
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

// holdFixture is one business with branches A and B, Ayşe (works Monday
// 09:00–12:00 in A and, for the A04 check, 09:00–12:00 in B too), a 30
// minute cut she performs, a bundle part with buffers and a room.
type holdFixture struct {
	pool               *pgxpool.Pool
	q                  *db.Queries
	org                orgmodel.Organization
	owner, emre, fatma db.User
	branchA, branchB   uuid.UUID
	ayseID             int64
	ayse               uuid.UUID
	cut, wash, colour  uuid.UUID
	settings           *orgsettingsusecase.Service
	exec               func(sql string, args ...any)
}

func newHoldFixture(t *testing.T) *holdFixture {
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
	f := &holdFixture{pool: pool, q: q}
	newUser := func(name string) db.User {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: name + "-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	f.owner, f.emre, f.fatma = newUser("owner"), newUser("emre"), newUser("fatma")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	f.org, err = orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Holds " + suffix, OwnerUserID: f.owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", f.org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{f.owner.ID, f.emre.ID, f.fatma.ID})
	})
	for _, u := range []db.User{f.emre, f.fatma} {
		if _, err := orgSvc.AddMember(ctx, f.org.UUID, u.Uuid, "member"); err != nil {
			t.Fatal(err)
		}
	}
	f.exec = func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
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
	f.exec(`UPDATE branches SET working_hours = $3, holiday_exceptions = '[]', timezone = NULL WHERE organization_id = $1 AND id = $2`, f.org.ID, aID, hours)
	bID, bUUID := idOf(`INSERT INTO branches (organization_id, code, name, working_hours) VALUES ($1, 'B', 'B', $2) RETURNING id, uuid`, f.org.ID, hours)
	f.branchA, f.branchB = aUUID, bUUID
	f.ayseID, f.ayse = idOf(`INSERT INTO staff (organization_id, name, user_id) VALUES ($1, 'Ayşe', $2) RETURNING id, uuid`, f.org.ID, f.emre.ID)
	// The same hours in both branches skip the staff validation on purpose:
	// the exclusion constraint has to hold even then.
	f.exec(`INSERT INTO staff_branches (organization_id, staff_id, branch_id, shifts) VALUES
		($1, $2, $3, '{"mon":[{"start":"09:00","end":"12:00","breaks":[]}]}'),
		($1, $2, $4, '{"mon":[{"start":"09:00","end":"12:00","breaks":[]}]}')`, f.org.ID, f.ayseID, aID, bID)
	svc := func(name string, duration, before, after int, kinds string) (int64, uuid.UUID) {
		id, u := idOf(`INSERT INTO services (organization_id, name, duration_minutes, buffer_before_minutes, buffer_after_minutes, resource_kinds)
			VALUES ($1, $2, $3, $4, $5, $6::TEXT[]) RETURNING id, uuid`, f.org.ID, name, duration, before, after, kinds)
		f.exec(`INSERT INTO service_branches (organization_id, service_id, branch_id) VALUES ($1, $2, $3), ($1, $2, $4)`, f.org.ID, id, aID, bID)
		f.exec(`INSERT INTO service_staff (organization_id, service_id, staff_id) VALUES ($1, $2, $3)`, f.org.ID, id, f.ayseID)
		return id, u
	}
	_, f.cut = svc("Kesim", 30, 0, 0, "{}")
	_, f.wash = svc("Yıkama", 30, 0, 15, "{room}")
	_, f.colour = svc("Boya", 30, 15, 0, "{room}")
	f.exec(`INSERT INTO resources (organization_id, branch_id, kind, name) VALUES ($1, $2, 'room', 'Oda')`, f.org.ID, aID)
	f.settings = orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))
	return f
}

func (f *holdFixture) holds(now *time.Time) (*availabilityusecase.Holds, *availabilityusecase.Service) {
	h := availabilityusecase.NewHolds(f.pool, f.q, f.settings)
	h.SetClock(func() time.Time { return *now })
	s := availabilityusecase.New(f.q, f.settings)
	s.SetClock(func() time.Time { return *now })
	s.SetBusySource(h.Busy())
	return h, s
}

func (f *holdFixture) cutAt(branch uuid.UUID, start time.Time, reason string) model.HoldInput {
	return model.HoldInput{BranchUUID: branch, Lines: []model.LineInput{{ServiceUUID: f.cut}}, StartsAt: &start, Reason: reason}
}

func starts(t *testing.T, s *availabilityusecase.Service, orgID int64, branch, service uuid.UUID) []string {
	t.Helper()
	step := 30
	res, err := s.Search(context.Background(), orgID, model.SearchInput{
		BranchUUID: branch, Lines: []model.LineInput{{ServiceUUID: service}}, From: "2030-01-07", To: "2030-01-07", StepMinutes: &step,
	}, availabilityusecase.Caller{})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, slot := range res.Days[0].Slots {
		out = append(out, slot.StartsAt.In(ist).Format("15:04"))
	}
	return out
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// A03: many customers ask for the last free time at once; exactly one hold
// is written. With the organization lock and without it (the exclusion
// constraint alone decides).
func TestA03LastSlotConcurrency(t *testing.T) {
	f := newHoldFixture(t)
	for _, tc := range []struct {
		name    string
		lock    bool
		workers int
		start   time.Time
	}{
		{"locked-2", true, 2, at(9, 0)},
		{"locked-6", true, 6, at(9, 30)},
		{"exclude-only-6", false, 6, at(10, 0)},
		{"exclude-only-12", false, 12, at(10, 30)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := at(8, 0)
			var wg sync.WaitGroup
			var mu sync.Mutex
			won, taken := 0, 0
			gate := make(chan struct{})
			for i := 0; i < tc.workers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					h, _ := f.holds(&now) // own service, own connections from the pool
					h.SetLocking(tc.lock)
					<-gate
					_, err := h.Create(context.Background(), f.org.ID, f.cutAt(f.branchA, tc.start, model.ReasonCheckout), availabilityusecase.Caller{}, f.owner.ID)
					mu.Lock()
					defer mu.Unlock()
					switch {
					case err == nil:
						won++
					case errors.Is(err, availabilityusecase.ErrSlotTaken):
						taken++
					default:
						t.Errorf("unexpected error: %v", err)
					}
				}()
			}
			close(gate)
			wg.Wait()
			if won != 1 || taken != tc.workers-1 {
				t.Fatalf("won = %d, taken = %d, want 1 and %d", won, taken, tc.workers-1)
			}
			var n int
			if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM capacity_allocations a
				JOIN capacity_holds h ON h.organization_id = a.organization_id AND h.id = a.hold_id
				WHERE a.organization_id = $1 AND lower(a.period) = $2`, f.org.ID, tc.start).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n != 1 {
				t.Fatalf("allocations at the start = %d, want 1", n)
			}
		})
	}
}

// The hold lifecycle: a quote holds nothing; a hold consumes until it
// lapses (before the sweep too); confirm consumes once; release frees
// exactly once.
func TestHoldLifecycle(t *testing.T) {
	f := newHoldFixture(t)
	ctx := context.Background()
	now := at(8, 0)
	h, s := f.holds(&now)
	if got := starts(t, s, f.org.ID, f.branchA, f.cut); !contains(got, "09:00") {
		t.Fatalf("09:00 not offered before the hold: %v", got)
	}

	// Checkout hold: 10 minutes by default.
	hold, err := h.Create(ctx, f.org.ID, f.cutAt(f.branchA, at(9, 0), model.ReasonCheckout), availabilityusecase.Caller{}, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if hold.Kind != model.KindHold || hold.Status != model.StatusActive || !hold.ExpiresAt.Equal(at(8, 10)) ||
		len(hold.Allocations) != 1 || hold.Allocations[0].Staff == nil || hold.Allocations[0].Staff.UUID != f.ayse {
		t.Fatalf("hold = %+v", hold)
	}
	if got := starts(t, s, f.org.ID, f.branchA, f.cut); contains(got, "09:00") {
		t.Fatalf("09:00 still offered while held: %v", got)
	}
	// A04: the same person is busy in branch B too.
	if got := starts(t, s, f.org.ID, f.branchB, f.cut); contains(got, "09:00") {
		t.Fatalf("09:00 offered in B while held in A: %v", got)
	}
	if _, err := h.Create(ctx, f.org.ID, f.cutAt(f.branchB, at(9, 0), model.ReasonCheckout), availabilityusecase.Caller{}, f.owner.ID); !errors.Is(err, availabilityusecase.ErrSlotTaken) {
		t.Fatalf("second hold in B = %v", err)
	}

	// The customer leaves the payment page: the hold lapses and counts no
	// more, even before the sweep.
	now = at(8, 10)
	if got := starts(t, s, f.org.ID, f.branchA, f.cut); !contains(got, "09:00") {
		t.Fatalf("09:00 not offered after expiry: %v", got)
	}
	if got, err := h.Get(ctx, f.org.ID, hold.UUID, availabilityusecase.Caller{}); err != nil || got.Status != model.StatusExpired {
		t.Fatalf("lapsed hold = %+v, %v", got, err)
	}
	if _, err := h.Confirm(ctx, f.org.ID, hold.UUID, availabilityusecase.Caller{}, "", nil); !errors.Is(err, availabilityusecase.ErrHoldExpired) {
		t.Fatalf("confirm lapsed = %v", err)
	}
	var status string
	_ = f.pool.QueryRow(ctx, "SELECT status FROM capacity_holds WHERE organization_id = $1 AND uuid = $2", f.org.ID, hold.UUID).Scan(&status)
	if status != model.StatusExpired {
		t.Fatalf("stored status = %s", status)
	}

	// Approval hold: the business's review window (30 minutes).
	lead, advance, window := 0, 30, 30
	settings := orgsettingsmodel.Defaults()
	settings.Reservation = orgsettingsmodel.Reservation{ApprovalMode: orgsettingsmodel.ApprovalStaff, StaffReviewWindowMinutes: &window, MinLeadMinutes: &lead, MaxAdvanceDays: &advance}
	if _, err := f.settings.Save(ctx, f.org.ID, f.owner.ID, orgsettingsmodel.SaveInput{Settings: settings}); err != nil {
		t.Fatal(err)
	}
	hold, err = h.Create(ctx, f.org.ID, f.cutAt(f.branchA, at(9, 0), model.ReasonApproval), availabilityusecase.Caller{}, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !hold.ExpiresAt.Equal(at(8, 40)) {
		t.Fatalf("approval expiry = %v", hold.ExpiresAt)
	}
	hold, err = h.Extend(ctx, f.org.ID, hold.UUID, availabilityusecase.Caller{}, 45)
	if err != nil || !hold.ExpiresAt.Equal(at(8, 55)) {
		t.Fatalf("extend = %+v, %v", hold.ExpiresAt, err)
	}
	if _, err := h.Extend(ctx, f.org.ID, hold.UUID, availabilityusecase.Caller{}, 0); err == nil {
		t.Fatal("extend by 0 accepted")
	}
	ref := uuid.New()
	hold, err = h.Confirm(ctx, f.org.ID, hold.UUID, availabilityusecase.Caller{}, "reservation", &ref)
	if err != nil || hold.Kind != model.KindBooking || hold.ExpiresAt != nil || hold.RefUUID == nil || *hold.RefUUID != ref {
		t.Fatalf("confirm = %+v, %v", hold, err)
	}
	now = at(10, 0) // a booking never lapses
	if got := starts(t, s, f.org.ID, f.branchA, f.cut); contains(got, "09:00") {
		t.Fatalf("09:00 offered over a booking: %v", got)
	}
	now = at(8, 0)
	if _, err := h.Extend(ctx, f.org.ID, hold.UUID, availabilityusecase.Caller{}, 10); !errors.Is(err, availabilityusecase.ErrNotHold) {
		t.Fatalf("extend booking = %v", err)
	}
	if _, err := h.Confirm(ctx, f.org.ID, hold.UUID, availabilityusecase.Caller{}, "", nil); !errors.Is(err, availabilityusecase.ErrNotHold) {
		t.Fatalf("confirm twice = %v", err)
	}
	released, err := h.Release(ctx, f.org.ID, hold.UUID, availabilityusecase.Caller{}, "cancelled")
	if err != nil || released.Status != model.StatusReleased || len(released.Allocations) != 0 || *released.ReleaseReason != "cancelled" {
		t.Fatalf("release = %+v, %v", released, err)
	}
	if _, err := h.Release(ctx, f.org.ID, hold.UUID, availabilityusecase.Caller{}, ""); !errors.Is(err, availabilityusecase.ErrNotActive) {
		t.Fatalf("release twice = %v", err)
	}
	if got := starts(t, s, f.org.ID, f.branchA, f.cut); !contains(got, "09:00") {
		t.Fatalf("09:00 not offered after release: %v", got)
	}

	// The minute sweep marks lapsed holds and frees their rows.
	minutes := 5
	in := f.cutAt(f.branchA, at(11, 0), model.ReasonPayment)
	in.Minutes = &minutes
	lapsing, err := h.Create(ctx, f.org.ID, in, availabilityusecase.Caller{}, f.owner.ID)
	if err != nil || !lapsing.ExpiresAt.Equal(at(8, 5)) {
		t.Fatalf("payment hold = %+v, %v", lapsing, err)
	}
	now = at(8, 6)
	n, err := h.ExpireDue(ctx)
	if err != nil || n < 1 {
		t.Fatalf("sweep = %d, %v", n, err)
	}
	var rows int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM capacity_allocations a JOIN capacity_holds h ON h.organization_id = a.organization_id AND h.id = a.hold_id
		WHERE a.organization_id = $1 AND h.uuid = $2`, f.org.ID, lapsing.UUID).Scan(&rows)
	if rows != 0 {
		t.Fatalf("allocations after the sweep = %d", rows)
	}
	if n, err := h.ExpireDue(ctx); err != nil || n != 0 {
		t.Fatalf("second sweep = %d, %v", n, err)
	}

	// Validation.
	bad := f.cutAt(f.branchA, at(9, 0), "later")
	if _, err := h.Create(ctx, f.org.ID, bad, availabilityusecase.Caller{}, f.owner.ID); err == nil {
		t.Fatal("unknown reason accepted")
	}
	bad = model.HoldInput{BranchUUID: f.branchA, Lines: []model.LineInput{{ServiceUUID: f.cut}}, Reason: model.ReasonCheckout}
	if _, err := h.Create(ctx, f.org.ID, bad, availabilityusecase.Caller{}, f.owner.ID); err == nil {
		t.Fatal("missing start accepted")
	}
	if _, err := h.Create(ctx, f.org.ID, f.cutAt(f.branchA, at(13, 0), model.ReasonCheckout), availabilityusecase.Caller{}, f.owner.ID); !errors.Is(err, availabilityusecase.ErrSlotTaken) {
		t.Fatalf("outside the shift = %v", err)
	}
	if _, err := h.Get(ctx, f.org.ID, uuid.New(), availabilityusecase.Caller{}); !errors.Is(err, availabilityusecase.ErrHoldNotFound) {
		t.Fatalf("unknown hold = %v", err)
	}
}

// A bundle served by one person with buffers between its lines writes one
// merged allocation per holder: the visit never conflicts with itself.
func TestBundleHoldMergesItsOwnBuffers(t *testing.T) {
	f := newHoldFixture(t)
	ctx := context.Background()
	now := at(8, 0)
	h, _ := f.holds(&now)
	start := at(9, 30)
	hold, err := h.Create(ctx, f.org.ID, model.HoldInput{
		BranchUUID: f.branchA, Lines: []model.LineInput{{ServiceUUID: f.wash}, {ServiceUUID: f.colour}},
		StartsAt: &start, Reason: model.ReasonCheckout,
	}, availabilityusecase.Caller{}, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	// wash 09:30–10:00 (+15 after), colour 10:00–10:30 (15 before): Ayşe and
	// the room are held 09:30–10:30 in one piece each.
	if len(hold.Allocations) != 2 || !hold.EndsAt.Equal(at(10, 30)) {
		t.Fatalf("allocations = %+v", hold.Allocations)
	}
	for _, a := range hold.Allocations {
		if !a.StartsAt.Equal(at(9, 30)) || !a.EndsAt.Equal(at(10, 30)) || a.LineIndex != 0 {
			t.Fatalf("allocation = %+v", a)
		}
	}
}

// The database refuses a second commitment of the same person over the same
// time whatever the application does, in any branch (A04).
func TestExclusionConstraintRefusesDoubleBooking(t *testing.T) {
	f := newHoldFixture(t)
	ctx := context.Background()
	insert := func(branch string) error {
		_, err := f.pool.Exec(ctx, `WITH h AS (
			INSERT INTO capacity_holds (organization_id, branch_id, kind, starts_at, ends_at)
			SELECT $1, b.id, 'booking', $3, $4 FROM branches b WHERE b.organization_id = $1 AND b.code = $5
			RETURNING id)
			INSERT INTO capacity_allocations (organization_id, hold_id, line_index, staff_id, period)
			SELECT $1, h.id, 0, $2, tstzrange($3, $4, '[)') FROM h`, f.org.ID, f.ayseID, at(9, 0), at(9, 30), branch)
		return err
	}
	if err := insert("B"); err != nil {
		t.Fatal(err)
	}
	err := insert("B")
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23P01" {
		t.Fatalf("second insert = %v, want 23P01", err)
	}
	// Touching ranges are fine (half-open).
	if _, err := f.pool.Exec(ctx, `WITH h AS (
		INSERT INTO capacity_holds (organization_id, branch_id, kind, starts_at, ends_at)
		SELECT $1, b.id, 'booking', $3, $4 FROM branches b WHERE b.organization_id = $1 AND b.code = 'B' RETURNING id)
		INSERT INTO capacity_allocations (organization_id, hold_id, line_index, staff_id, period)
		SELECT $1, h.id, 0, $2, tstzrange($3, $4, '[)') FROM h`, f.org.ID, f.ayseID, at(9, 30), at(10, 0)); err != nil {
		t.Fatalf("touching insert: %v", err)
	}
	// The planner in branch A sees B's booking and the writer is refused.
	now := at(8, 0)
	h, _ := f.holds(&now)
	if _, err := h.Create(ctx, f.org.ID, f.cutAt(f.branchA, at(9, 0), model.ReasonCheckout), availabilityusecase.Caller{}, f.owner.ID); !errors.Is(err, availabilityusecase.ErrSlotTaken) {
		t.Fatalf("hold over B's booking = %v", err)
	}
}

// The hold routes over HTTP: create, read, extend, release; the own
// calendar of a staff member; 409 details.
func TestHoldsOverHTTP(t *testing.T) {
	f := newHoldFixture(t)
	now := at(8, 0)
	h, s := f.holds(&now)
	orgSvc := orgusecase.New(orgrepository.NewPostgres(f.pool, f.q), branchusecase.NewOrganizationHooks(nil))
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	loader := mapLoader{f.owner.Uuid: f.owner.ID, f.emre.Uuid: f.emre.ID, f.fatma.Uuid: f.fatma.ID}
	mux := http.NewServeMux()
	availability.RegisterRoutes(mux, availabilityhandler.New(s, h), tokens, loader, orgSvc)
	call := func(user db.User, method, path string, body any) result {
		t.Helper()
		access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: user.Uuid})
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Authorization", "Bearer "+access)
		req.Header.Set(orgctx.HeaderName, f.org.Slug)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		var env struct {
			Data  map[string]any `json:"data"`
			Error struct {
				Details []any `json:"details"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		return result{code: rr.Code, data: env.Data, details: env.Error.Details, raw: rr.Body.String()}
	}
	want := func(r result, code int, step string) result {
		t.Helper()
		if r.code != code {
			t.Fatalf("%s = %d, want %d: %s", step, r.code, code, r.raw)
		}
		return r
	}
	detail := func(r result) string {
		if len(r.details) == 0 {
			return ""
		}
		d := r.details[0].(map[string]any)
		return d["field"].(string) + "/" + d["code"].(string)
	}
	body := func(start time.Time) map[string]any {
		return map[string]any{"branch_uuid": f.branchA, "lines": []map[string]any{{"service_uuid": f.cut}}, "starts_at": start, "reason": "approval"}
	}

	created := want(call(f.owner, http.MethodPost, "/v1/tenant/availability/holds", body(at(9, 0))), http.StatusCreated, "create")
	id := created.data["uuid"].(string)
	if created.data["kind"] != "hold" || created.data["reason"] != "approval" || len(created.data["allocations"].([]any)) != 1 {
		t.Fatalf("created = %v", created.data)
	}
	if r := want(call(f.owner, http.MethodPost, "/v1/tenant/availability/holds", body(at(9, 0))), http.StatusConflict, "taken"); detail(r) != "starts_at/taken" {
		t.Fatalf("detail = %s", detail(r))
	}
	want(call(f.owner, http.MethodGet, "/v1/tenant/availability/holds/"+id, nil), http.StatusOK, "get")
	want(call(f.owner, http.MethodGet, "/v1/tenant/availability/holds/not-a-uuid", nil), http.StatusNotFound, "bad uuid")
	ext := want(call(f.owner, http.MethodPost, "/v1/tenant/availability/holds/"+id+"/extend", map[string]any{"minutes": 20}), http.StatusOK, "extend")
	if ext.data["expires_at"] != at(8, 20).UTC().Format(time.RFC3339) {
		t.Fatalf("expires_at = %v", ext.data["expires_at"])
	}
	want(call(f.owner, http.MethodPost, "/v1/tenant/availability/holds/"+id+"/extend", map[string]any{"minutes": 0}), http.StatusBadRequest, "extend 0")

	// Own calendar: emre (Ayşe) holds for himself; fatma has no staff
	// record and cannot see or touch emre's hold.
	mine := want(call(f.emre, http.MethodPost, "/v1/tenant/availability/holds", body(at(10, 0))), http.StatusCreated, "own create")
	mineID := mine.data["uuid"].(string)
	want(call(f.emre, http.MethodGet, "/v1/tenant/availability/holds/"+mineID, nil), http.StatusOK, "own get")
	want(call(f.emre, http.MethodGet, "/v1/tenant/availability/holds/"+id, nil), http.StatusNotFound, "own get other")
	want(call(f.fatma, http.MethodGet, "/v1/tenant/availability/holds/"+mineID, nil), http.StatusNotFound, "fatma get")
	want(call(f.fatma, http.MethodDelete, "/v1/tenant/availability/holds/"+mineID, nil), http.StatusNotFound, "fatma release")
	want(call(f.fatma, http.MethodPost, "/v1/tenant/availability/holds", body(at(11, 0))), http.StatusForbidden, "fatma create")

	rel := want(call(f.owner, http.MethodDelete, "/v1/tenant/availability/holds/"+id, nil), http.StatusOK, "release")
	if rel.data["status"] != "released" {
		t.Fatalf("released = %v", rel.data)
	}
	if r := want(call(f.owner, http.MethodDelete, "/v1/tenant/availability/holds/"+id, nil), http.StatusConflict, "release twice"); detail(r) != "uuid/not_active" {
		t.Fatalf("detail = %s", detail(r))
	}
	now = at(9, 0) // emre's approval hold (default 60 minutes, no review window set) lapsed
	if r := want(call(f.emre, http.MethodPost, "/v1/tenant/availability/holds/"+mineID+"/extend", map[string]any{"minutes": 5}), http.StatusConflict, "extend lapsed"); detail(r) != "uuid/expired" {
		t.Fatalf("detail = %s", detail(r))
	}
}
