package reservations_test

import (
	"context"
	"errors"
	"testing"

	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
)

// TestCalendar: the staff view shows the person's shift inside the opening
// hours, its leave and its visits (cancelled ones only when asked); the
// resource and branch views place the same visits; an own calendar member
// sees its own column; bad windows and other businesses' branches fail.
func TestCalendar(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	booked, err := f.svc.Create(ctx, f.org.ID, f.input(at(9, 0), "", "", f.cut), none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	gone, err := f.svc.Create(ctx, f.org.ID, f.input(at(10, 0), "", "", f.cut), none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Cancel(ctx, f.org.ID, gone.UUID, none, f.owner.ID, usecase.ChangeInput{}); err != nil {
		t.Fatal(err)
	}
	f.exec(`INSERT INTO staff_time_off (organization_id, staff_id, starts_at, ends_at)
		SELECT organization_id, id, $2, $3 FROM staff WHERE organization_id = $1 AND name = 'Ayşe'`, f.org.ID, at(11, 0), at(11, 30))
	f.exec(`INSERT INTO resources (organization_id, branch_id, kind, name)
		SELECT organization_id, id, 'chair', 'Koltuk 1' FROM branches WHERE organization_id = $1 AND uuid = $2`, f.org.ID, f.branch)

	day := "2030-01-07"
	cal, err := f.svc.Calendar(ctx, f.org.ID, model.CalendarFilter{BranchUUID: &f.branch, From: day}, none)
	if err != nil {
		t.Fatal(err)
	}
	if cal.By != model.CalendarByStaff || cal.Branch == nil || cal.Timezone != "Europe/Istanbul" || cal.To != day ||
		len(cal.Open) != 1 || !cal.Open[0].StartsAt.Equal(at(9, 0)) || !cal.Open[0].EndsAt.Equal(at(18, 0)) {
		t.Fatalf("calendar = %+v", cal)
	}
	if len(cal.Columns) != 1 || cal.Columns[0].Name != "Ayşe" || len(cal.Columns[0].Work) != 1 ||
		!cal.Columns[0].Work[0].EndsAt.Equal(at(12, 0)) || len(cal.Columns[0].Off) != 1 || cal.Columns[0].Color == nil {
		t.Fatalf("columns = %+v", cal.Columns)
	}
	if len(cal.Items) != 1 || cal.Items[0].ReservationUUID != booked.UUID || cal.Items[0].Columns[0] != cal.Columns[0].Key ||
		cal.Items[0].CustomerName != "Zeynep Kaya" || cal.Items[0].ServiceName != "Kesim" {
		t.Fatalf("items = %+v", cal.Items)
	}
	withCancelled, err := f.svc.Calendar(ctx, f.org.ID, model.CalendarFilter{BranchUUID: &f.branch, From: day, IncludeCancelled: true}, none)
	if err != nil {
		t.Fatal(err)
	}
	if len(withCancelled.Items) != 2 || withCancelled.Items[1].Status != model.StatusCancelled {
		t.Fatalf("with cancelled = %+v", withCancelled.Items)
	}
	// Another week has no visits.
	empty, err := f.svc.Calendar(ctx, f.org.ID, model.CalendarFilter{BranchUUID: &f.branch, From: "2030-01-08", To: "2030-01-14"}, none)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Items) != 0 || len(empty.Open) != 1 {
		t.Fatalf("next week = %+v", empty)
	}

	// The cut takes no resource: the chair is free, the visit is unassigned.
	byResource, err := f.svc.Calendar(ctx, f.org.ID, model.CalendarFilter{By: model.CalendarByResource, BranchUUID: &f.branch, From: day}, none)
	if err != nil {
		t.Fatal(err)
	}
	if len(byResource.Columns) != 2 || byResource.Columns[0].Name != "Koltuk 1" || len(byResource.Columns[0].Work) != 1 ||
		byResource.Columns[1].Kind != model.CalendarUnassigned || byResource.Items[0].Columns[0] != model.CalendarUnassigned {
		t.Fatalf("by resource = %+v", byResource)
	}
	byBranch, err := f.svc.Calendar(ctx, f.org.ID, model.CalendarFilter{By: model.CalendarByBranch, From: day}, none)
	if err != nil {
		t.Fatal(err)
	}
	if byBranch.Branch != nil || len(byBranch.Columns) != 1 || byBranch.Columns[0].Key != f.branch.String() || len(byBranch.Items) != 1 {
		t.Fatalf("by branch = %+v", byBranch)
	}

	// Emre is Ayşe: his own column and visits; Fatma has neither.
	emre, err := f.svc.Calendar(ctx, f.org.ID, model.CalendarFilter{BranchUUID: &f.branch, From: day}, availabilityusecase.Caller{OwnUserID: f.emre.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(emre.Columns) != 1 || len(emre.Items) != 1 {
		t.Fatalf("own (Emre) = %+v", emre)
	}
	fatma, err := f.svc.Calendar(ctx, f.org.ID, model.CalendarFilter{BranchUUID: &f.branch, From: day}, availabilityusecase.Caller{OwnUserID: f.fatma.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(fatma.Columns) != 0 || len(fatma.Items) != 0 {
		t.Fatalf("own (Fatma) = %+v", fatma)
	}

	var ve *usecase.ValidationError
	for _, bad := range []model.CalendarFilter{
		{From: day},
		{BranchUUID: &f.branch, From: "07.01.2030"},
		{BranchUUID: &f.branch, From: day, To: "2030-01-14"},
		{BranchUUID: &f.branch, From: day, To: "2030-01-06"},
		{By: "week", BranchUUID: &f.branch, From: day},
	} {
		if _, err := f.svc.Calendar(ctx, f.org.ID, bad, none); !errors.As(err, &ve) {
			t.Fatalf("%+v: err = %v, want validation", bad, err)
		}
	}
	if _, err := f.svc.Calendar(ctx, f.org2.ID, model.CalendarFilter{BranchUUID: &f.branch, From: day}, none); !errors.Is(err, availabilityusecase.ErrNotFound) {
		t.Fatalf("foreign branch err = %v", err)
	}
}
