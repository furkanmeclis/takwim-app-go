package classes_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilitymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	reservationmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

var ist = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

// at is a wall clock time in Istanbul on a day of January 2030 (the 7th
// is a Monday).
func at(day, hour, minute int) time.Time {
	return time.Date(2030, 1, day, hour, minute, 0, 0, ist)
}

var none = availabilityusecase.Caller{}

// gym is one gym (the sector template's studio and group class), open on
// Mondays and Tuesdays 09:00–21:00, with a trainer who also gives
// individual sessions.
type gym struct {
	pool                *pgxpool.Pool
	q                   *db.Queries
	ctx                 context.Context
	org                 orgmodel.Organization
	ownerID             int64
	branch              uuid.UUID
	branchID            int64
	class, personal     uuid.UUID
	classID             int64
	trainer, coach      uuid.UUID
	studio, hall        uuid.UUID
	now                 time.Time
	holds               *availabilityusecase.Holds
	search              *availabilityusecase.Service
	classes             *usecase.Service
	reservations        *reservationusecase.Service
	exec                func(sql string, args ...any)
	customer            func() uuid.UUID
	scalar              func(sql string, args ...any) (int64, uuid.UUID)
	settings            *orgsettingsusecase.Service
	otherOrg            orgmodel.Organization
	otherBranch         uuid.UUID
	otherClass, otherTr uuid.UUID
}

func newGym(t *testing.T) *gym {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	bg := context.Background()
	pool, err := pgxpool.New(bg, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	suffix := uuid.NewString()[:8]
	g := &gym{pool: pool, q: q, now: at(7, 8, 0)}
	owner, err := q.CreateUser(bg, db.CreateUserParams{Email: "classes-" + suffix + "@example.test", PasswordHash: "x", Name: "Owner", Surname: "Test", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	g.ownerID = owner.ID
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	g.org, err = orgSvc.CreateOrganization(bg, orgmodel.CreateOrganizationInput{Name: "Gym " + suffix, OwnerUserID: owner.ID, Sector: "gym"})
	if err != nil {
		t.Fatal(err)
	}
	g.otherOrg, err = orgSvc.CreateOrganization(bg, orgmodel.CreateOrganizationInput{Name: "Gym other " + suffix, OwnerUserID: owner.ID, Sector: "gym"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = ANY($1)", []int64{g.org.ID, g.otherOrg.ID})
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", owner.ID)
	})
	g.ctx = orgctx.WithScope(bg, orgctx.Scope{ID: g.org.ID, Sector: g.org.Sector, Timezone: "Europe/Istanbul"})
	g.exec = func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(bg, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	g.scalar = func(sql string, args ...any) (int64, uuid.UUID) {
		t.Helper()
		var id int64
		var u uuid.UUID
		if err := pool.QueryRow(bg, sql, args...).Scan(&id, &u); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return id, u
	}
	g.customer = func() uuid.UUID {
		t.Helper()
		_, u := g.scalar(`INSERT INTO customers (organization_id, first_name, last_name) VALUES ($1, 'Üye', 'Test') RETURNING id, uuid`, g.org.ID)
		return u
	}
	g.exec(`UPDATE branches SET working_hours = '{"mon":{"closed":false,"open":"09:00","close":"21:00"},"tue":{"closed":false,"open":"09:00","close":"21:00"}}',
		holiday_exceptions = '[]', timezone = NULL WHERE organization_id = ANY($1)`, []int64{g.org.ID, g.otherOrg.ID})
	g.branchID, g.branch = g.scalar(`SELECT id, uuid FROM branches WHERE organization_id = $1`, g.org.ID)
	_, g.otherBranch = g.scalar(`SELECT id, uuid FROM branches WHERE organization_id = $1`, g.otherOrg.ID)
	g.classID, g.class = g.scalar(`SELECT id, uuid FROM services WHERE organization_id = $1 AND group_capacity IS NOT NULL`, g.org.ID)
	_, g.otherClass = g.scalar(`SELECT id, uuid FROM services WHERE organization_id = $1 AND group_capacity IS NOT NULL`, g.otherOrg.ID)
	g.exec(`UPDATE services SET group_capacity = 3, duration_minutes = 60 WHERE id = $1`, g.classID)
	_, g.studio = g.scalar(`SELECT id, uuid FROM resources WHERE organization_id = $1 AND kind = 'area'`, g.org.ID)
	_, g.hall = g.scalar(`INSERT INTO resources (organization_id, branch_id, kind, name, min_capacity, capacity) VALUES ($1, $2, 'area', 'Salon', 1, 20) RETURNING id, uuid`, g.org.ID, g.branchID)
	staff := func(orgID, branchID, serviceID int64, name string) uuid.UUID {
		id, u := g.scalar(`INSERT INTO staff (organization_id, name) VALUES ($1, $2) RETURNING id, uuid`, orgID, name)
		g.exec(`INSERT INTO staff_branches (organization_id, staff_id, branch_id, shifts) VALUES ($1, $2, $3, '{"mon":[{"start":"09:00","end":"21:00","breaks":[]}]}')`, orgID, id, branchID)
		g.exec(`INSERT INTO service_staff (organization_id, service_id, staff_id) VALUES ($1, $2, $3)`, orgID, serviceID, id)
		return u
	}
	g.trainer = staff(g.org.ID, g.branchID, g.classID, "Eğitmen")
	g.coach = staff(g.org.ID, g.branchID, g.classID, "Koç")
	otherBranchID, _ := g.scalar(`SELECT id, uuid FROM branches WHERE organization_id = $1`, g.otherOrg.ID)
	otherClassID, _ := g.scalar(`SELECT id, uuid FROM services WHERE organization_id = $1 AND group_capacity IS NOT NULL`, g.otherOrg.ID)
	g.otherTr = staff(g.otherOrg.ID, otherBranchID, otherClassID, "Yabancı")
	personalID, personal := g.scalar(`INSERT INTO services (organization_id, name, duration_minutes, price) VALUES ($1, 'Bireysel', 60, 500) RETURNING id, uuid`, g.org.ID)
	g.personal = personal
	g.exec(`INSERT INTO service_branches (organization_id, service_id, branch_id) VALUES ($1, $2, $3)`, g.org.ID, personalID, g.branchID)
	g.exec(`INSERT INTO service_staff (organization_id, service_id, staff_id) SELECT $1, $2, id FROM staff WHERE uuid = $3`, g.org.ID, personalID, g.trainer)

	g.settings = orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))
	g.holds = availabilityusecase.NewHolds(pool, q, g.settings)
	g.holds.SetClock(func() time.Time { return g.now })
	g.search = availabilityusecase.New(q, g.settings)
	g.search.SetClock(func() time.Time { return g.now })
	g.search.SetBusySource(g.holds.Busy())
	g.classes = usecase.New(q, g.holds)
	g.classes.SetClock(func() time.Time { return g.now })
	g.reservations = reservationusecase.New(g.holds, q, g.settings)
	return g
}

func ptr[T any](v T) *T { return &v }

func (g *gym) input(weekday, start string) model.ScheduleInput {
	return model.ScheduleInput{
		BranchUUID: g.branch, ServiceUUID: g.class, Weekday: weekday, StartTime: start,
		StaffUUID: ptr(g.trainer), ResourceUUID: ptr(g.studio), ValidFrom: "2030-01-01",
	}
}

// slots answers the class on one local date: start (HH:MM) → seats left.
func (g *gym) slots(t *testing.T, date string) (string, map[string]int) {
	t.Helper()
	res, err := g.search.Search(g.ctx, g.org.ID, availabilitymodel.SearchInput{
		BranchUUID: g.branch, Lines: []availabilitymodel.LineInput{{ServiceUUID: g.class}}, From: date, To: date,
	}, none)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int{}
	for _, s := range res.Days[0].Slots {
		l := s.Lines[0]
		if l.Staff == nil || len(l.Resources) != 1 {
			t.Fatalf("session line without its trainer and area: %+v", l)
		}
		out[s.StartsAt.In(ist).Format("15:04")+" "+l.Staff.Name+" "+l.Resources[0].Name] = *l.SeatsLeft
	}
	return res.Days[0].Status, out
}

func (g *gym) book(t *testing.T, service uuid.UUID, start time.Time, party int) (reservationmodel.Reservation, error) {
	t.Helper()
	in := reservationmodel.CreateInput{
		BranchUUID: g.branch, CustomerUUID: g.customer(), StartsAt: &start,
		Lines: []availabilitymodel.LineInput{{ServiceUUID: service}},
	}
	if party > 0 {
		in.PartySize = &party
	}
	return g.reservations.Create(g.ctx, g.org.ID, in, none, g.ownerID)
}

func code(err error) string {
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		return ve.Details[0].Field + "/" + ve.Details[0].Code
	}
	var ce *usecase.ConflictError
	if errors.As(err, &ce) {
		return ce.Field + "/" + ce.Code
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

// TestClassTimetable (TEC-970): a group class runs only at the sessions of
// its timetable, with the class's trainer, area and seats; a session holds
// its trainer and area before its first booking; single sessions are
// cancelled or get other seats; booked sessions are protected; A03 holds
// on scheduled sessions.
func TestClassTimetable(t *testing.T) {
	g := newGym(t)

	// Validation and isolation of the form.
	for _, tc := range []struct {
		name string
		edit func(*model.ScheduleInput)
		want string
	}{
		{"weekday", func(in *model.ScheduleInput) { in.Weekday = "xyz" }, "weekday/invalid"},
		{"time", func(in *model.ScheduleInput) { in.StartTime = "25:00" }, "start_time/invalid"},
		{"individual service", func(in *model.ScheduleInput) { in.ServiceUUID = g.personal }, "service_uuid/not_group"},
		{"other business's branch", func(in *model.ScheduleInput) { in.BranchUUID = g.otherBranch }, "branch_uuid/not_found"},
		{"other business's class", func(in *model.ScheduleInput) { in.ServiceUUID = g.otherClass }, "service_uuid/not_found"},
		{"other business's trainer", func(in *model.ScheduleInput) { in.StaffUUID = ptr(g.otherTr) }, "staff_uuid/not_found"},
		{"until before from", func(in *model.ScheduleInput) { in.ValidUntil = ptr("2029-12-01") }, "valid_until/before_from"},
		{"seats", func(in *model.ScheduleInput) { in.Capacity = ptr(0) }, "capacity/invalid"},
	} {
		in := g.input("mon", "18:00")
		tc.edit(&in)
		if _, err := g.classes.Create(g.ctx, g.org.ID, &g.ownerID, in); code(err) != tc.want {
			t.Errorf("%s: err = %v, want %s", tc.name, err, tc.want)
		}
	}

	evening, err := g.classes.Create(g.ctx, g.org.ID, &g.ownerID, g.input("mon", "18:00"))
	if err != nil {
		t.Fatal(err)
	}
	if evening.Seats != 3 || evening.EndTime != "19:00" || evening.Staff.Name != "Eğitmen" || evening.Resource.Name != "Stüdyo" {
		t.Fatalf("class = %+v", evening)
	}
	// Overlaps: the same trainer or area, the same class at the same start.
	clash := g.input("mon", "18:30")
	clash.ResourceUUID = ptr(g.hall)
	if _, err := g.classes.Create(g.ctx, g.org.ID, &g.ownerID, clash); code(err) != "staff_uuid/staff_busy" {
		t.Fatalf("trainer overlap err = %v", err)
	}
	clash.StaffUUID = ptr(g.coach)
	clash.ResourceUUID = ptr(g.studio)
	if _, err := g.classes.Create(g.ctx, g.org.ID, &g.ownerID, clash); code(err) != "resource_uuid/resource_busy" {
		t.Fatalf("area overlap err = %v", err)
	}
	dup := g.input("mon", "18:00")
	dup.StaffUUID, dup.ResourceUUID = ptr(g.coach), ptr(g.hall)
	if _, err := g.classes.Create(g.ctx, g.org.ID, &g.ownerID, dup); code(err) != "start_time/duplicate" {
		t.Fatalf("duplicate err = %v", err)
	}
	// A morning class with its own seats, the coach and the hall.
	morning := g.input("mon", "10:00")
	morning.StaffUUID, morning.ResourceUUID, morning.Capacity = ptr(g.coach), ptr(g.hall), ptr(5)
	if _, err := g.classes.Create(g.ctx, g.org.ID, &g.ownerID, morning); err != nil {
		t.Fatal(err)
	}

	// Only the sessions are offered, with their trainer, area and seats.
	status, got := g.slots(t, "2030-01-07")
	if status != "available" || len(got) != 2 || got["10:00 Koç Salon"] != 5 || got["18:00 Eğitmen Stüdyo"] != 3 {
		t.Fatalf("Monday = %s %v", status, got)
	}
	if status, got := g.slots(t, "2030-01-08"); status != "no_class" || len(got) != 0 {
		t.Fatalf("Tuesday = %s %v", status, got)
	}
	if _, err := g.book(t, g.class, at(7, 12, 0), 0); !errors.Is(err, availabilityusecase.ErrSlotTaken) {
		t.Fatalf("booking outside the timetable err = %v", err)
	}
	// The empty evening session holds the trainer and the studio.
	if _, err := g.book(t, g.personal, at(7, 18, 30), 0); !errors.Is(err, availabilityusecase.ErrSlotTaken) {
		t.Fatalf("individual session over the class err = %v", err)
	}

	// Bookings take seats of the scheduled session.
	first, err := g.book(t, g.class, at(7, 18, 0), 2)
	if err != nil {
		t.Fatal(err)
	}
	if l := first.Lines[0]; l.Staff == nil || l.Staff.Name != "Eğitmen" || l.Resources[0].Name != "Stüdyo" {
		t.Fatalf("booked line = %+v", l)
	}
	if _, got := g.slots(t, "2030-01-07"); got["18:00 Eğitmen Stüdyo"] != 1 {
		t.Fatalf("after two seats: %v", got)
	}
	sessions, total, err := g.classes.Sessions(g.ctx, g.org.ID, model.SessionFilter{From: "2030-01-07", To: "2030-01-13"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || sessions[0].StartsAt != at(7, 10, 0).UTC() || sessions[1].Booked != 2 || sessions[1].SeatsLeft != 1 || sessions[1].State != model.StateOpen {
		t.Fatalf("sessions = %d %+v", total, sessions)
	}
	detail, err := g.classes.SessionDetail(g.ctx, g.org.ID, evening.UUID, "2030-01-07")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Attendees) != 1 || detail.Attendees[0].ReservationUUID != first.UUID || detail.Attendees[0].Seats != 2 ||
		detail.Attendees[0].Customer.Name != "Üye Test" {
		t.Fatalf("attendees = %+v", detail.Attendees)
	}
	if _, err := g.classes.SessionDetail(g.ctx, g.org.ID, evening.UUID, "2030-01-08"); !errors.Is(err, usecase.ErrSessionNotFound) {
		t.Fatalf("no session on Tuesday err = %v", err)
	}

	// A booked session is not cancelled and keeps at least its bookings'
	// seats; the class cannot move away from it or be removed.
	if _, _, err := g.classes.ChangeSession(g.ctx, g.org.ID, evening.UUID, "2030-01-07", &g.ownerID, model.SessionChangeInput{Cancelled: true}); !errors.Is(err, usecase.ErrHasBookings) {
		t.Fatalf("cancel a booked session err = %v", err)
	}
	if _, _, err := g.classes.ChangeSession(g.ctx, g.org.ID, evening.UUID, "2030-01-07", &g.ownerID, model.SessionChangeInput{Capacity: ptr(1)}); !errors.Is(err, usecase.ErrBelowBooked) {
		t.Fatalf("seats below the bookings err = %v", err)
	}
	moved := g.input("tue", "18:00")
	if _, _, err := g.classes.Update(g.ctx, g.org.ID, evening.UUID, moved); !errors.Is(err, usecase.ErrHasBookings) {
		t.Fatalf("moving a booked class err = %v", err)
	}
	if _, err := g.classes.Delete(g.ctx, g.org.ID, evening.UUID); !errors.Is(err, usecase.ErrHasBookings) {
		t.Fatalf("removing a booked class err = %v", err)
	}
	// Other seats for the session: the last seat goes, then the session is full.
	if _, after, err := g.classes.ChangeSession(g.ctx, g.org.ID, evening.UUID, "2030-01-07", &g.ownerID, model.SessionChangeInput{Capacity: ptr(3)}); err != nil || !after.CapacityChanged {
		t.Fatalf("seats change: %+v %v", after, err)
	}
	if _, err := g.book(t, g.class, at(7, 18, 0), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := g.book(t, g.class, at(7, 18, 0), 0); !errors.Is(err, availabilityusecase.ErrSlotTaken) {
		t.Fatalf("full session err = %v", err)
	}
	if _, after, err := g.classes.ChangeSession(g.ctx, g.org.ID, evening.UUID, "2030-01-07", &g.ownerID, model.SessionChangeInput{Capacity: ptr(4)}); err != nil || after.SeatsLeft != 1 {
		t.Fatalf("one more seat: %+v %v", after, err)
	}
	if _, err := g.book(t, g.class, at(7, 18, 0), 0); err != nil {
		t.Fatalf("booking the extra seat: %v", err)
	}

	// A cancelled session is not offered and frees its trainer.
	if _, after, err := g.classes.ChangeSession(g.ctx, g.org.ID, evening.UUID, "2030-01-14", &g.ownerID, model.SessionChangeInput{Cancelled: true, Reason: "Bayram"}); err != nil || after.State != model.StateCancelled {
		t.Fatalf("cancel: %+v %v", after, err)
	}
	if _, got := g.slots(t, "2030-01-14"); len(got) != 1 || got["10:00 Koç Salon"] != 5 {
		t.Fatalf("Monday with the evening cancelled: %v", got)
	}
	if _, err := g.book(t, g.personal, at(14, 18, 0), 0); err != nil {
		t.Fatalf("individual session in the cancelled class's time: %v", err)
	}
	// Restoring it would put the trainer in two places: the session is back
	// in the timetable but not bookable.
	if _, _, err := g.classes.ChangeSession(g.ctx, g.org.ID, evening.UUID, "2030-01-14", &g.ownerID, model.SessionChangeInput{}); err != nil {
		t.Fatal(err)
	}
	if _, got := g.slots(t, "2030-01-14"); len(got) != 1 {
		t.Fatalf("restored session over an individual booking is offered: %v", got)
	}
	// Past sessions are not changed; a date without a session is not found.
	g.now = at(7, 19, 0)
	if _, _, err := g.classes.ChangeSession(g.ctx, g.org.ID, evening.UUID, "2030-01-07", &g.ownerID, model.SessionChangeInput{Capacity: ptr(5)}); !errors.Is(err, usecase.ErrPast) {
		t.Fatalf("past session err = %v", err)
	}
	if _, _, err := g.classes.ChangeSession(g.ctx, g.org.ID, evening.UUID, "2030-01-09", &g.ownerID, model.SessionChangeInput{}); !errors.Is(err, usecase.ErrSessionNotFound) {
		t.Fatalf("Wednesday err = %v", err)
	}
	// Once its bookings are in the past the class can change and go.
	if _, after, err := g.classes.Update(g.ctx, g.org.ID, evening.UUID, moved); err != nil || after.Weekday != "tue" {
		t.Fatalf("move after the bookings: %+v %v", after, err)
	}
	if _, err := g.classes.Delete(g.ctx, g.org.ID, evening.UUID); err != nil {
		t.Fatal(err)
	}
	// Another business's class reads as not found.
	other := orgctx.WithScope(context.Background(), orgctx.Scope{ID: g.otherOrg.ID, Sector: "gym", Timezone: "Europe/Istanbul"})
	list, _, err := g.classes.List(other, g.otherOrg.ID, model.ScheduleFilter{Limit: 50})
	if err != nil || len(list) != 0 {
		t.Fatalf("other business's timetable = %+v %v", list, err)
	}
}

// TestClassSeatsRace (A03 on the timetable): eight people ask for the
// three seats of a scheduled session at once; exactly three win.
func TestClassSeatsRace(t *testing.T) {
	g := newGym(t)
	if _, err := g.classes.Create(g.ctx, g.org.ID, &g.ownerID, g.input("mon", "17:00")); err != nil {
		t.Fatal(err)
	}
	customers := make([]uuid.UUID, 8)
	for i := range customers {
		customers[i] = g.customer()
	}
	results := make(chan error, len(customers))
	gate := make(chan struct{})
	for _, c := range customers {
		go func(customer uuid.UUID) {
			h := availabilityusecase.NewHolds(g.pool, g.q, g.settings)
			h.SetClock(func() time.Time { return g.now })
			h.SetLocking(false)
			svc := reservationusecase.New(h, g.q, g.settings)
			start := at(7, 17, 0)
			<-gate
			_, err := svc.Create(g.ctx, g.org.ID, reservationmodel.CreateInput{
				BranchUUID: g.branch, CustomerUUID: customer, StartsAt: &start,
				Lines: []availabilitymodel.LineInput{{ServiceUUID: g.class}},
			}, none, g.ownerID)
			results <- err
		}(c)
	}
	close(gate)
	won := 0
	for range customers {
		err := <-results
		switch {
		case err == nil:
			won++
		case errors.Is(err, availabilityusecase.ErrSlotTaken):
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if won != 3 {
		t.Fatalf("won = %d, want 3", won)
	}
}
