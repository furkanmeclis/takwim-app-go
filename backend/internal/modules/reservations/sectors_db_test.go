package reservations_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	availabilitymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	servicesmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/model"
	servicesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/repository"
	servicesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// sectorOrg is a business of one sector created through the organization
// hooks (so the sector template ran), open Mondays 09:00–18:00.
type sectorOrg struct {
	org      orgmodel.Organization
	ctx      context.Context
	branchID int64
	branch   uuid.UUID
}

func (f *fixture) sectorOrg(t *testing.T, code sector.Code) sectorOrg {
	t.Helper()
	bg := context.Background()
	orgSvc := orgusecase.New(orgrepository.NewPostgres(f.pool, f.q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(bg, orgmodel.CreateOrganizationInput{
		Name: string(code) + " " + uuid.NewString()[:8], OwnerUserID: f.owner.ID, Sector: string(code),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = f.pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", org.ID) })
	out := sectorOrg{org: org, ctx: orgctx.WithScope(bg, orgctx.Scope{ID: org.ID, Sector: org.Sector, Timezone: "Europe/Istanbul"})}
	if err := f.pool.QueryRow(bg, `SELECT id, uuid FROM branches WHERE organization_id = $1`, org.ID).Scan(&out.branchID, &out.branch); err != nil {
		t.Fatal(err)
	}
	f.exec(`UPDATE branches SET working_hours = '{"mon":{"closed":false,"open":"09:00","close":"18:00"}}', holiday_exceptions = '[]', timezone = NULL
		WHERE organization_id = $1`, org.ID)
	return out
}

func (f *fixture) newCustomer(t *testing.T, orgID int64, name string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(), `INSERT INTO customers (organization_id, first_name) VALUES ($1, $2) RETURNING uuid`, orgID, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// TestGymTemplate (TEC-904, TEC-970, A02 gym, A03 group seats): a new gym gets a
// group class with a studio. Every booking of one start joins the same
// session on its trainer and studio until the seats run out; the trainer
// is busy for individual sessions meanwhile; a cancelled booking frees its
// seat; parallel writers never overbook the last seats, even without the
// business lock.
func TestGymTemplate(t *testing.T) {
	f := newFixture(t)
	g := f.sectorOrg(t, sector.Gym)
	var classID int64
	var class uuid.UUID
	if err := f.pool.QueryRow(g.ctx, `SELECT id, uuid FROM services WHERE organization_id = $1 AND group_capacity = 12
		AND staff_mode = 'optional' AND resource_kinds = '{area}'`, g.org.ID).Scan(&classID, &class); err != nil {
		t.Fatalf("starter group class: %v", err)
	}
	if n := f.count(`SELECT count(*) FROM resources WHERE organization_id = $1 AND kind = 'area' AND name = 'Stüdyo'`, g.org.ID); n != 1 {
		t.Fatalf("studio = %d", n)
	}
	f.exec(`UPDATE services SET group_capacity = 3 WHERE id = $1`, classID)
	var trainerID int64
	var trainer uuid.UUID
	if err := f.pool.QueryRow(g.ctx, `INSERT INTO staff (organization_id, name) VALUES ($1, 'Eğitmen') RETURNING id, uuid`, g.org.ID).Scan(&trainerID, &trainer); err != nil {
		t.Fatal(err)
	}
	f.exec(`INSERT INTO staff_branches (organization_id, staff_id, branch_id, shifts)
		VALUES ($1, $2, $3, '{"mon":[{"start":"09:00","end":"18:00","breaks":[]}]}')`, g.org.ID, trainerID, g.branchID)
	f.exec(`INSERT INTO service_staff (organization_id, service_id, staff_id) VALUES ($1, $2, $3)`, g.org.ID, classID, trainerID)
	var personalID int64
	var personal uuid.UUID
	if err := f.pool.QueryRow(g.ctx, `INSERT INTO services (organization_id, name, duration_minutes, price) VALUES ($1, 'Bireysel seans', 60, 500)
		RETURNING id, uuid`, g.org.ID).Scan(&personalID, &personal); err != nil {
		t.Fatal(err)
	}
	f.exec(`INSERT INTO service_branches (organization_id, service_id, branch_id) VALUES ($1, $2, $3)`, g.org.ID, personalID, g.branchID)
	f.exec(`INSERT INTO service_staff (organization_id, service_id, staff_id) VALUES ($1, $2, $3)`, g.org.ID, personalID, trainerID)
	// TEC-970: the class runs on Mondays at 10, 14, 16 and 17 with the
	// trainer in the studio; no other start is offered.
	for _, start := range []string{"10:00", "14:00", "16:00", "17:00"} {
		f.exec(`INSERT INTO class_schedules (organization_id, branch_id, service_id, weekday, start_time, staff_id, resource_id, valid_from)
			SELECT $1, $2, $3, 'mon', $4, $5, r.id, '2029-01-01' FROM resources r WHERE r.organization_id = $1 AND r.name = 'Stüdyo'`,
			g.org.ID, g.branchID, classID, start, trainerID)
	}

	book := func(svc *usecase.Service, start int, party int, services ...uuid.UUID) (model.Reservation, error) {
		s := at(start, 0)
		in := model.CreateInput{BranchUUID: g.branch, CustomerUUID: f.newCustomer(t, g.org.ID, "Üye"), StartsAt: &s}
		for _, id := range services {
			in.Lines = append(in.Lines, availabilitymodel.LineInput{ServiceUUID: id})
		}
		if party > 0 {
			in.PartySize = &party
		}
		return svc.Create(g.ctx, g.org.ID, in, none, f.owner.ID)
	}
	seatsAt := func(hour int) (left int, offered bool) {
		t.Helper()
		search := availabilityusecase.New(f.q, f.settings)
		search.SetClock(func() time.Time { return f.now })
		search.SetBusySource(f.holds.Busy())
		step := 60
		res, err := search.Search(g.ctx, g.org.ID, availabilitymodel.SearchInput{
			BranchUUID: g.branch, Lines: []availabilitymodel.LineInput{{ServiceUUID: class}}, From: "2030-01-07", To: "2030-01-07", StepMinutes: &step,
		}, none)
		if err != nil {
			t.Fatal(err)
		}
		if res.Lines[0].GroupCapacity == nil || *res.Lines[0].GroupCapacity != 3 {
			t.Fatalf("group capacity = %v", res.Lines[0].GroupCapacity)
		}
		for _, slot := range res.Days[0].Slots {
			if slot.StartsAt.Equal(at(hour, 0)) {
				return *slot.Lines[0].SeatsLeft, true
			}
		}
		return 0, false
	}

	// Outside the timetable a group class cannot be booked.
	if _, ok := seatsAt(11); ok {
		t.Fatal("11:00 is offered without a session")
	}
	if _, err := book(f.svc, 11, 0, class); !errors.Is(err, availabilityusecase.ErrSlotTaken) {
		t.Fatalf("booking outside the timetable err = %v", err)
	}
	// An empty session already holds its trainer.
	if _, err := book(f.svc, 16, 0, personal); !errors.Is(err, availabilityusecase.ErrSlotTaken) {
		t.Fatalf("individual session over an empty class err = %v", err)
	}
	first, err := book(f.svc, 10, 0, class)
	if err != nil {
		t.Fatal(err)
	}
	if left, ok := seatsAt(10); !ok || left != 2 {
		t.Fatalf("10:00 after one booking: left %d offered %v", left, ok)
	}
	second, err := book(f.svc, 10, 2, class)
	if err != nil {
		t.Fatalf("joining with two people: %v", err)
	}
	for _, r := range []model.Reservation{first, second} {
		l := r.Lines[0]
		if l.Staff == nil || l.Staff.Name != "Eğitmen" || len(l.Resources) != 1 || l.Resources[0].Name != "Stüdyo" {
			t.Fatalf("session line = %+v", l)
		}
	}
	if _, ok := seatsAt(10); ok {
		t.Fatal("a full session is still offered")
	}
	if _, err := book(f.svc, 10, 0, class); !errors.Is(err, availabilityusecase.ErrSlotTaken) {
		t.Fatalf("booking a full session err = %v", err)
	}
	// The session holds the trainer: no individual session at the same time.
	if _, err := book(f.svc, 10, 0, personal); !errors.Is(err, availabilityusecase.ErrSlotTaken) {
		t.Fatalf("individual session over the class err = %v", err)
	}
	if _, err := book(f.svc, 12, 0, personal); err != nil {
		t.Fatalf("individual session later: %v", err)
	}
	var ave *availabilityusecase.ValidationError
	if _, err := book(f.svc, 15, 0, class, personal); !errors.As(err, &ave) || ave.Details[0].Code != "group_alone" {
		t.Fatalf("class in a bundle err = %v", err)
	}
	// A cancelled booking frees its seat.
	if _, err := f.svc.Cancel(g.ctx, g.org.ID, first.UUID, none, f.owner.ID, usecase.ChangeInput{Reason: "test"}); err != nil {
		t.Fatal(err)
	}
	if left, ok := seatsAt(10); !ok || left != 1 {
		t.Fatalf("10:00 after a cancel: left %d offered %v", left, ok)
	}
	// A move into the session takes a seat there and frees the old one.
	moved, err := f.svc.Reschedule(g.ctx, g.org.ID, second.UUID, none, f.owner.ID, model.RescheduleInput{StartsAt: ptr(at(14, 0))})
	if err != nil {
		t.Fatal(err)
	}
	if left, ok := seatsAt(14); !ok || left != 1 || moved.Lines[0].Resources[0].Name != "Stüdyo" {
		t.Fatalf("14:00 after the move: left %d offered %v", left, ok)
	}
	if left, ok := seatsAt(10); !ok || left != 3 {
		t.Fatalf("10:00 after the move: left %d", left)
	}

	// A03: eight people ask for the three seats of a new session at once,
	// with and without the business lock: exactly three win.
	for _, tc := range []struct {
		name  string
		lock  bool
		start int
	}{{"locked", true, 16}, {"seats-only", false, 17}} {
		t.Run(tc.name, func(t *testing.T) {
			var wg sync.WaitGroup
			var mu sync.Mutex
			won, taken := 0, 0
			gate := make(chan struct{})
			for i := 0; i < 8; i++ {
				customer := f.newCustomer(t, g.org.ID, "Yarış")
				wg.Add(1)
				go func() {
					defer wg.Done()
					h := availabilityusecase.NewHolds(f.pool, f.q, f.settings)
					h.SetClock(func() time.Time { return f.now })
					h.SetLocking(tc.lock)
					svc := usecase.New(h, f.q, f.settings)
					s := at(tc.start, 0)
					<-gate
					_, err := svc.Create(g.ctx, g.org.ID, model.CreateInput{
						BranchUUID: g.branch, CustomerUUID: customer, StartsAt: &s,
						Lines: []availabilitymodel.LineInput{{ServiceUUID: class}},
					}, none, f.owner.ID)
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
			if won != 3 || taken != 5 {
				t.Fatalf("won = %d, taken = %d, want 3 and 5", won, taken)
			}
			if n := f.count(`SELECT COALESCE(SUM(seats), 0) FROM capacity_holds WHERE organization_id = $1 AND status = 'active' AND starts_at = $2`, g.org.ID, at(tc.start, 0)); n != 3 {
				t.Fatalf("seats taken = %d, want 3", n)
			}
		})
	}
}

// TestCarWashTemplate (TEC-904, A02 car wash): a new car wash gets two
// wash bays and no services; a new service asks for a bay and takes a
// person when one is free; a booking carries the vehicle (type and a
// normalised plate) and two bookings at once take both bays.
func TestCarWashTemplate(t *testing.T) {
	f := newFixture(t)
	w := f.sectorOrg(t, sector.CarWash)
	if n := f.count(`SELECT count(*) FROM resources WHERE organization_id = $1 AND kind = 'bay'`, w.org.ID); n != 2 {
		t.Fatalf("bays = %d, want 2", n)
	}
	if n := f.count(`SELECT count(*) FROM services WHERE organization_id = $1`, w.org.ID); n != 0 {
		t.Fatalf("services = %d, want none", n)
	}
	services := servicesusecase.New(servicesrepository.NewPostgres(f.pool, f.q), nil)
	name, duration := "Dış yıkama", int32(30)
	svc, err := services.Create(w.ctx, w.org.ID, servicesmodel.Input{Name: &name, DurationMinutes: &duration, BranchUUIDs: []uuid.UUID{w.branch}})
	if err != nil {
		t.Fatal(err)
	}
	if svc.StaffMode != "optional" || len(svc.ResourceKinds) != 1 || svc.ResourceKinds[0] != "bay" || svc.GroupCapacity != nil {
		t.Fatalf("car wash service defaults = %s %v %v", svc.StaffMode, svc.ResourceKinds, svc.GroupCapacity)
	}
	book := func(v *model.Vehicle) (model.Reservation, error) {
		s := at(10, 0)
		return f.svc.Create(w.ctx, w.org.ID, model.CreateInput{
			BranchUUID: w.branch, CustomerUUID: f.newCustomer(t, w.org.ID, "Sürücü"), StartsAt: &s, Vehicle: v,
			Lines: []availabilitymodel.LineInput{{ServiceUUID: svc.UUID}},
		}, none, f.owner.ID)
	}
	var ve *usecase.ValidationError
	if _, err := book(&model.Vehicle{Type: "truck"}); !errors.As(err, &ve) || ve.Details[0].Field != "vehicle.type" {
		t.Fatalf("unknown vehicle type err = %v", err)
	}
	if _, err := book(&model.Vehicle{Type: "car", Plate: "34-ABC-123"}); !errors.As(err, &ve) || ve.Details[0].Field != "vehicle.plate" {
		t.Fatalf("plate with dashes err = %v", err)
	}
	r, err := book(&model.Vehicle{Type: "suv", Plate: "  34 abc  123 "})
	if err != nil {
		t.Fatal(err)
	}
	if r.Vehicle == nil || r.Vehicle.Type != "suv" || r.Vehicle.Plate != "34 ABC 123" || r.Lines[0].Resources[0].Kind != "bay" {
		t.Fatalf("booking = %+v %+v", r.Vehicle, r.Lines[0].Resources)
	}
	if _, err := book(nil); err != nil {
		t.Fatalf("second bay: %v", err)
	}
	if _, err := book(nil); !errors.Is(err, availabilityusecase.ErrSlotTaken) {
		t.Fatalf("third car with two bays err = %v", err)
	}
	list, _, err := f.svc.List(w.ctx, w.org.ID, model.ListFilter{Q: "34abc", Limit: 10}, none)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Vehicle == nil || list[0].Vehicle.Plate != "34 ABC 123" {
		t.Fatalf("plate search = %+v", list)
	}
}

// TestClinicAndBeautyTemplates (TEC-904, A02 clinic and beauty salon): a
// clinic gets two examination rooms and no appointment types (and no
// health record fields anywhere); a beauty salon gets nothing. Their
// bookings run on the specialist and, when the service asks, a room.
func TestClinicAndBeautyTemplates(t *testing.T) {
	f := newFixture(t)
	c := f.sectorOrg(t, sector.Clinic)
	if n := f.count(`SELECT count(*) FROM resources WHERE organization_id = $1 AND kind = 'room'`, c.org.ID); n != 2 {
		t.Fatalf("clinic rooms = %d, want 2", n)
	}
	b := f.sectorOrg(t, sector.BeautySalon)
	if n := f.count(`SELECT (SELECT count(*) FROM resources WHERE organization_id = $1) + (SELECT count(*) FROM services WHERE organization_id = $1)`, b.org.ID); n != 0 {
		t.Fatalf("beauty salon seeded %d records, want none", n)
	}
	for _, o := range []sectorOrg{c, b} {
		var specialistID, serviceID int64
		if err := f.pool.QueryRow(o.ctx, `INSERT INTO staff (organization_id, name) VALUES ($1, 'Uzman') RETURNING id`, o.org.ID).Scan(&specialistID); err != nil {
			t.Fatal(err)
		}
		f.exec(`INSERT INTO staff_branches (organization_id, staff_id, branch_id, shifts)
			VALUES ($1, $2, $3, '{"mon":[{"start":"09:00","end":"18:00","breaks":[]}]}')`, o.org.ID, specialistID, o.branchID)
		if o.org.ID == b.org.ID {
			f.exec(`INSERT INTO resources (organization_id, branch_id, kind, name) VALUES ($1, $2, 'device', 'Lazer cihazı')`, o.org.ID, o.branchID)
		}
		kind := "{room}"
		if o.org.ID == b.org.ID {
			kind = "{device}"
		}
		var service uuid.UUID
		if err := f.pool.QueryRow(o.ctx, `INSERT INTO services (organization_id, name, duration_minutes, price, resource_kinds)
			VALUES ($1, 'Muayene', 30, 900, $2::TEXT[]) RETURNING id, uuid`, o.org.ID, kind).Scan(&serviceID, &service); err != nil {
			t.Fatal(err)
		}
		f.exec(`INSERT INTO service_branches (organization_id, service_id, branch_id) VALUES ($1, $2, $3)`, o.org.ID, serviceID, o.branchID)
		f.exec(`INSERT INTO service_staff (organization_id, service_id, staff_id) VALUES ($1, $2, $3)`, o.org.ID, serviceID, specialistID)
		s := at(9, 0)
		r, err := f.svc.Create(o.ctx, o.org.ID, model.CreateInput{
			BranchUUID: o.branch, CustomerUUID: f.newCustomer(t, o.org.ID, "Hasta"), StartsAt: &s,
			Lines: []availabilitymodel.LineInput{{ServiceUUID: service}},
		}, none, f.owner.ID)
		if err != nil {
			t.Fatalf("%s booking: %v", o.org.Sector, err)
		}
		if r.Lines[0].Staff == nil || r.Lines[0].Staff.Name != "Uzman" || len(r.Lines[0].Resources) != 1 {
			t.Fatalf("%s booking line = %+v", o.org.Sector, r.Lines[0])
		}
	}
}
