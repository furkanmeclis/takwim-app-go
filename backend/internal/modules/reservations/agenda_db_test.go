package reservations_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	availabilitymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
)

// TestStaffPanel (TEC-902): a member with only its own reach cannot change
// the status of a visit shared with a colleague, moves it as a whole only
// when the colleague's line keeps its service and person, and its agenda
// shows its own shifts, leave and lines.
func TestStaffPanel(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var ayse, mehmet uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT uuid FROM staff WHERE organization_id = $1 AND name = 'Ayşe'`, f.org.ID).Scan(&ayse); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO staff (organization_id, name, user_id) VALUES ($1, 'Mehmet', $2) RETURNING uuid`, f.org.ID, f.fatma.ID).Scan(&mehmet); err != nil {
		t.Fatal(err)
	}
	f.exec(`INSERT INTO staff_branches (organization_id, staff_id, branch_id, shifts)
		SELECT $1, s.id, b.id, '{"mon":[{"start":"09:00","end":"12:00","breaks":[]}]}'
		FROM staff s, branches b WHERE s.organization_id = $1 AND s.uuid = $2 AND b.uuid = $3`, f.org.ID, mehmet, f.branch)
	f.exec(`INSERT INTO service_staff (organization_id, service_id, staff_id)
		SELECT $1, sv.id, s.id FROM services sv, staff s WHERE sv.organization_id = $1 AND s.uuid = $2`, f.org.ID, mehmet)

	in := f.input(at(9, 0), "", "")
	in.Lines = []availabilitymodel.LineInput{{ServiceUUID: f.cut, StaffUUID: &ayse}, {ServiceUUID: f.wash, StaffUUID: &mehmet}}
	shared, err := f.svc.Create(ctx, f.org.ID, in, none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	solo, err := f.svc.Create(ctx, f.org.ID, f.input(at(11, 0), "", "", f.cut), none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	emre := availabilityusecase.Caller{OwnUserID: f.emre.ID}

	if _, err := f.svc.Cancel(ctx, f.org.ID, shared.UUID, emre, f.emre.ID, usecase.ChangeInput{}); !errors.Is(err, usecase.ErrOtherStaff) {
		t.Fatalf("own cancel of a shared visit err = %v", err)
	}
	if _, err := f.svc.Start(ctx, f.org.ID, solo.UUID, emre, f.emre.ID, usecase.ChangeInput{}); err != nil {
		t.Fatalf("own start of its visit: %v", err)
	}

	// Taking Mehmet's line is refused; moving the visit with it is not.
	later := at(10, 0)
	if _, err := f.svc.Reschedule(ctx, f.org.ID, shared.UUID, emre, f.emre.ID, model.RescheduleInput{
		StartsAt: &later, Lines: []availabilitymodel.LineInput{{ServiceUUID: f.cut, StaffUUID: &ayse}, {ServiceUUID: f.wash, StaffUUID: &ayse}},
	}); !errors.Is(err, usecase.ErrOtherStaff) {
		t.Fatalf("own reschedule taking a colleague's line err = %v", err)
	}
	moved, err := f.svc.Reschedule(ctx, f.org.ID, shared.UUID, emre, f.emre.ID, model.RescheduleInput{StartsAt: &later})
	if err != nil {
		t.Fatalf("own reschedule of a shared visit: %v", err)
	}
	if !moved.StartsAt.Equal(later) || len(moved.Lines) != 2 || moved.Lines[1].Staff == nil || moved.Lines[1].Staff.UUID == nil || *moved.Lines[1].Staff.UUID != mehmet {
		t.Fatalf("moved = %+v", moved)
	}

	f.exec(`INSERT INTO staff_time_off (organization_id, staff_id, starts_at, ends_at)
		SELECT organization_id, id, $2, $3 FROM staff WHERE organization_id = $1 AND name = 'Ayşe'`, f.org.ID, at(12, 0), at(13, 0))
	day := "2030-01-07"
	agenda, err := f.svc.Agenda(ctx, f.org.ID, f.emre.ID, model.AgendaFilter{From: day})
	if err != nil {
		t.Fatal(err)
	}
	if agenda.Staff == nil || agenda.Staff.UUID != ayse || len(agenda.Work) != 1 || !agenda.Work[0].StartsAt.Equal(at(9, 0)) ||
		!agenda.Work[0].EndsAt.Equal(at(12, 0)) || agenda.Work[0].Branch.UUID != f.branch || len(agenda.Off) != 1 {
		t.Fatalf("agenda = %+v", agenda)
	}
	// Ayşe serves the cut of the shared visit and the solo one; Mehmet's
	// wash is his.
	if len(agenda.Items) != 2 || agenda.Items[0].ReservationUUID != shared.UUID || agenda.Items[0].ServiceName != "Kesim" ||
		agenda.Items[1].ReservationUUID != solo.UUID || agenda.Items[1].BranchName == "" {
		t.Fatalf("agenda items = %+v", agenda.Items)
	}
	his, err := f.svc.Agenda(ctx, f.org.ID, f.fatma.ID, model.AgendaFilter{From: day, To: "2030-01-13"})
	if err != nil {
		t.Fatal(err)
	}
	if his.Staff == nil || his.Staff.UUID != mehmet || len(his.Items) != 1 || his.Items[0].ServiceName != "Yıkama" {
		t.Fatalf("Mehmet's agenda = %+v", his)
	}
	nobody, err := f.svc.Agenda(ctx, f.org.ID, f.owner.ID, model.AgendaFilter{From: day})
	if err != nil {
		t.Fatal(err)
	}
	if nobody.Staff != nil || len(nobody.Items) != 0 || len(nobody.Work) != 0 {
		t.Fatalf("owner without a staff record = %+v", nobody)
	}
	// Another business knows nothing of Emre.
	foreign, err := f.svc.Agenda(ctx, f.org2.ID, f.emre.ID, model.AgendaFilter{From: day})
	if err != nil || foreign.Staff != nil {
		t.Fatalf("foreign agenda = %+v, %v", foreign, err)
	}
	var ve *usecase.ValidationError
	for _, bad := range []model.AgendaFilter{{From: "07.01.2030"}, {From: day, To: "2030-01-14"}, {From: day, To: "2030-01-06"}} {
		if _, err := f.svc.Agenda(ctx, f.org.ID, f.emre.ID, bad); !errors.As(err, &ve) {
			t.Fatalf("%+v: err = %v, want validation", bad, err)
		}
	}
}
