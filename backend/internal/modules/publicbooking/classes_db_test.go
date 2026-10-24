package publicbooking_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	classesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
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
)

// TestPublicClassTimetable (TEC-990): the class timetable of a gym on the
// booking page. Sessions come from the classes module with their trainer,
// area and seats; the state is open exactly when the slot engine offers
// the start for the asked seats; cancelled, past, full and too far
// sessions say so; a booked session keeps the trainer of its bookings
// when the timetable's trainer changes.
func TestPublicClassTimetable(t *testing.T) {
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
	owner, err := q.CreateUser(ctx, db.CreateUserParams{Email: "public-classes-" + suffix + "@example.test", PasswordHash: "x", Name: "Owner", Surname: "Test", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Public gym " + suffix, OwnerUserID: owner.ID, Sector: "gym"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Public gym other " + suffix, OwnerUserID: owner.ID, Sector: "gym"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = ANY($1)", []int64{org.ID, other.ID})
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", owner.ID)
	})
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	scalar := func(sql string, args ...any) (int64, uuid.UUID) {
		t.Helper()
		var id int64
		var u uuid.UUID
		if err := pool.QueryRow(ctx, sql, args...).Scan(&id, &u); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return id, u
	}
	exec(`UPDATE branches SET working_hours = '{"mon":{"closed":false,"open":"09:00","close":"21:00"},"tue":{"closed":false,"open":"09:00","close":"21:00"}}',
		holiday_exceptions = '[]', timezone = NULL WHERE organization_id = ANY($1)`, []int64{org.ID, other.ID})
	branchID, branch := scalar(`SELECT id, uuid FROM branches WHERE organization_id = $1`, org.ID)
	_, otherBranch := scalar(`SELECT id, uuid FROM branches WHERE organization_id = $1`, other.ID)
	classID, class := scalar(`SELECT id, uuid FROM services WHERE organization_id = $1 AND group_capacity IS NOT NULL`, org.ID)
	exec(`UPDATE services SET group_capacity = 3, duration_minutes = 60, price = 0 WHERE id = $1`, classID)
	studioID, _ := scalar(`SELECT id, uuid FROM resources WHERE organization_id = $1 AND kind = 'area'`, org.ID)
	var studio string
	if err := pool.QueryRow(ctx, `SELECT name FROM resources WHERE id = $1`, studioID).Scan(&studio); err != nil {
		t.Fatal(err)
	}
	trainer := func(name string) (int64, uuid.UUID) {
		id, u := scalar(`INSERT INTO staff (organization_id, name) VALUES ($1, $2) RETURNING id, uuid`, org.ID, name)
		exec(`INSERT INTO staff_branches (organization_id, staff_id, branch_id, shifts) VALUES ($1, $2, $3, '{"mon":[{"start":"09:00","end":"21:00","breaks":[]}]}')`, org.ID, id, branchID)
		exec(`INSERT INTO service_staff (organization_id, service_id, staff_id) VALUES ($1, $2, $3)`, org.ID, classID, id)
		return id, u
	}
	egitmenID, egitmen := trainer("Eğitmen")
	kocID, koc := trainer("Koç")
	personalID, personal := scalar(`INSERT INTO services (organization_id, name, duration_minutes, price) VALUES ($1, 'Bireysel', 60, 500) RETURNING id, uuid`, org.ID)
	exec(`INSERT INTO service_branches (organization_id, service_id, branch_id) VALUES ($1, $2, $3)`, org.ID, personalID, branchID)
	schedule := func(weekday, start string) int64 {
		id, _ := scalar(`INSERT INTO class_schedules (organization_id, branch_id, service_id, weekday, start_time, staff_id, resource_id, valid_from)
			VALUES ($1, $2, $3, $4, $5, $6, $7, DATE '2030-01-01') RETURNING id, uuid`, org.ID, branchID, classID, weekday, start, egitmenID, studioID)
		return id
	}
	schedule("mon", "07:30")
	morning := schedule("mon", "10:00")
	evening := schedule("mon", "18:00")
	tuesday := schedule("tue", "10:00")
	exec(`INSERT INTO class_session_changes (organization_id, schedule_id, session_date, capacity) VALUES ($1, $2, DATE '2030-01-07', 5)`, org.ID, evening)
	exec(`INSERT INTO class_session_changes (organization_id, schedule_id, session_date, cancelled, reason) VALUES ($1, $2, DATE '2030-01-08', TRUE, 'Eğitmen izinli')`, org.ID, tuesday)

	settings := orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))
	f := &fixture{pool: pool, q: q, owner: owner, settings: settings, now: at(8, 0)}
	f.saveSettings(t, org.ID, "auto", true)
	f.saveSettings(t, other.ID, "auto", true)
	clock := func() time.Time { return f.now }
	holds := availabilityusecase.NewHolds(pool, q, settings)
	holds.SetClock(clock)
	holds.SetExpiryHook(reservationsusecase.ExpireRequests)
	avail := availabilityusecase.New(q, settings)
	avail.SetClock(clock)
	avail.SetBusySource(holds.Busy())
	classes := classesusecase.New(q, holds)
	classes.SetClock(clock)
	cipher, err := crypto.NewFieldCipher(strings.Repeat("k", 32))
	if err != nil {
		t.Fatal(err)
	}
	f.svc = usecase.New(q, settings, avail, holds, reservationsusecase.New(holds, q, settings), cipher)
	f.svc.SetClock(clock)
	f.svc.SetClasses(classes)
	f.h = handler.New(f.svc, nil, nil)
	mux := http.NewServeMux()
	publicbooking.RegisterRoutes(mux, f.h)
	tokens, err := jwt.NewManager("public-booking-test-secret-0123456789", time.Hour, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	f.mux = middleware.Idempotency(idempotency.NewMemoryStore(), tokens, slog.New(slog.NewTextHandler(io.Discard, nil)))(mux)
	base := f.base(org.Slug)

	timetable := func(query string) model.ClassTimetable {
		t.Helper()
		r := f.do(t, http.MethodGet, base+"/classes?branch_uuid="+branch.String()+query, nil, nil)
		if r.code != http.StatusOK {
			t.Fatalf("classes%s = %d %+v %s", query, r.code, r.env.Error, r.detail())
		}
		return decode[model.ClassTimetable](t, r)
	}
	find := func(tt model.ClassTimetable, start time.Time) model.ClassSession {
		t.Helper()
		for _, s := range tt.Sessions {
			if s.StartsAt.Equal(start) {
				return s
			}
		}
		t.Fatalf("no session at %s in %+v", start, tt.Sessions)
		return model.ClassSession{}
	}
	monday := func(day, hour, minute int) time.Time { return time.Date(2030, 1, day, hour, minute, 0, 0, ist) }

	// Validation and isolation.
	for _, tc := range []struct{ query, want string }{
		{"", "branch_uuid/required"},
		{"?branch_uuid=" + otherBranch.String(), "branch_uuid/not_found"},
		{"?branch_uuid=" + branch.String() + "&service_uuid=" + personal.String(), "service_uuid/not_found"},
		{"?branch_uuid=" + branch.String() + "&from=2030-01-07&to=2030-03-01", "to/too_long"},
		{"?branch_uuid=" + branch.String() + "&party_size=0", "party_size/invalid"},
		{"?branch_uuid=" + branch.String() + "&channel=fax", "channel/invalid"},
	} {
		if r := f.do(t, http.MethodGet, base+"/classes"+tc.query, nil, nil); r.code != http.StatusBadRequest || r.detail() != tc.want {
			t.Fatalf("classes%s = %d %s, want %s", tc.query, r.code, r.detail(), tc.want)
		}
	}
	if r := f.do(t, http.MethodGet, base+"/classes?branch_uuid="+branch.String()+"&channel=widget", nil, nil); r.code != http.StatusConflict {
		t.Fatalf("widget channel off = %d", r.code)
	}

	// The week: past, open, own seats, cancelled.
	tt := timetable("&from=2030-01-07&to=2030-01-14")
	if tt.From != "2030-01-07" || tt.To != "2030-01-14" || tt.PartySize != 1 || tt.Timezone != "Europe/Istanbul" || len(tt.Sessions) != 7 {
		t.Fatalf("timetable = %+v", tt)
	}
	for i := 1; i < len(tt.Sessions); i++ {
		if tt.Sessions[i].StartsAt.Before(tt.Sessions[i-1].StartsAt) {
			t.Fatalf("sessions out of order: %+v", tt.Sessions)
		}
	}
	if s := find(tt, monday(7, 7, 30)); s.State != model.ClassPast {
		t.Fatalf("started session = %+v", s)
	}
	s := find(tt, monday(7, 10, 0))
	if s.State != model.ClassOpen || s.Service.UUID != class || s.Trainer == nil || s.Trainer.UUID != egitmen || s.TrainerChanged ||
		s.Area == nil || *s.Area != studio || s.Capacity != 3 || s.SeatsLeft != 3 || s.Date != "2030-01-07" || !s.EndsAt.Equal(monday(7, 11, 0)) {
		t.Fatalf("open session = %+v", s)
	}
	if s := find(tt, monday(7, 18, 0)); s.State != model.ClassOpen || s.Capacity != 5 || !s.CapacityChanged || s.SeatsLeft != 5 {
		t.Fatalf("session with own seats = %+v", s)
	}
	if s := find(tt, monday(8, 10, 0)); s.State != model.ClassCancelled {
		t.Fatalf("cancelled session = %+v", s)
	}
	if raw := string(f.do(t, http.MethodGet, base+"/classes?branch_uuid="+branch.String()+"&from=2030-01-07&to=2030-01-14", nil, nil).env.Data); strings.Contains(raw, "Eğitmen izinli") ||
		strings.Contains(raw, `"booked"`) || strings.Contains(raw, "resource") {
		t.Fatalf("timetable leaks internal fields: %s", raw)
	}

	// Two seats booked online in the morning session.
	r := f.do(t, http.MethodPost, base+"/checkouts", map[string]any{
		"branch_uuid": branch, "lines": lines(class), "starts_at": monday(7, 10, 0), "party_size": 2,
	}, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("checkout = %d %+v %s", r.code, r.env.Error, r.detail())
	}
	token := decode[model.Checkout](t, r).Token
	if r := f.do(t, http.MethodPost, base+"/reservations", map[string]any{
		"checkout_token": token, "payment": "on_site", "customer": customer("+905551112233"),
	}, nil); r.code != http.StatusCreated {
		t.Fatalf("booking = %d %+v %s", r.code, r.env.Error, r.detail())
	}
	if s := find(timetable("&from=2030-01-07&to=2030-01-07"), monday(7, 10, 0)); s.State != model.ClassOpen || s.SeatsLeft != 1 {
		t.Fatalf("one seat left = %+v", s)
	}
	if s := find(timetable("&from=2030-01-07&to=2030-01-07&party_size=2"), monday(7, 10, 0)); s.State != model.ClassFull || s.SeatsLeft != 1 {
		t.Fatalf("too few seats for two = %+v", s)
	}

	// The timetable's trainer changes: the booked session keeps its trainer,
	// the next one has the new one.
	exec(`UPDATE class_schedules SET staff_id = $2 WHERE id = $1`, morning, kocID)
	tt = timetable("&from=2030-01-07&to=2030-01-14&service_uuid=" + class.String())
	if s := find(tt, monday(7, 10, 0)); s.Trainer == nil || s.Trainer.UUID != egitmen || !s.TrainerChanged {
		t.Fatalf("booked session after the change = %+v", s)
	}
	if s := find(tt, monday(14, 10, 0)); s.Trainer == nil || s.Trainer.UUID != koc || s.TrainerChanged || s.State != model.ClassOpen {
		t.Fatalf("next session after the change = %+v", s)
	}

	// After the furthest online day (30 days ahead).
	tt = timetable("&from=2030-02-04&to=2030-02-12")
	if tt.LatestDate == nil || *tt.LatestDate != "2030-02-06" {
		t.Fatalf("latest date = %v", tt.LatestDate)
	}
	if s := find(tt, time.Date(2030, 2, 4, 10, 0, 0, 0, ist)); s.State != model.ClassOpen {
		t.Fatalf("session inside the window = %+v", s)
	}
	if s := find(tt, time.Date(2030, 2, 11, 10, 0, 0, 0, ist)); s.State != model.ClassTooFar {
		t.Fatalf("session after the window = %+v", s)
	}

	// Without the classes module the list is empty.
	exec(`UPDATE organizations SET sector = 'hairdresser' WHERE id = $1`, org.ID)
	if tt := timetable("&from=2030-01-14&to=2030-01-14"); len(tt.Sessions) != 0 {
		t.Fatalf("hairdresser timetable = %+v", tt.Sessions)
	}
}
