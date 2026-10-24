package classes_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	bulkusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/bulk/usecase"
	classesadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/usecase"
	reservationmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// TestClassSessionCancelWithBookings (TEC-989): a booked session is
// cancelled in one step: the session leaves the timetable, its bookings
// are cancelled in one bulk run (seats free, customer told by the cancel's
// own notice) and the run can be rolled back, which opens the session
// again with its bookings. A booking changed since the cancel stays
// cancelled.
func TestClassSessionCancelWithBookings(t *testing.T) {
	g := newGym(t)
	evening, err := g.classes.Create(g.ctx, g.org.ID, &g.ownerID, g.input("mon", "18:00"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := g.book(t, g.class, at(7, 18, 0), 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := g.book(t, g.class, at(7, 18, 0), 2)
	if err != nil {
		t.Fatal(err)
	}
	// A booking of another session of the class is not touched.
	later, err := g.book(t, g.class, at(14, 18, 0), 0)
	if err != nil {
		t.Fatal(err)
	}

	ctx := authctx.WithPrincipal(orgctx.WithScope(context.Background(), orgctx.Scope{
		ID: g.org.ID, Sector: "gym", Timezone: "Europe/Istanbul",
		Permissions: []string{rbac.PermTenantClassesWrite, rbac.PermTenantReservationsWrite},
	}), authctx.Principal{UserInternal: g.ownerID})
	bulk := bulkusecase.New(g.q, bulkengine.NewRegistry(classesadapters.NewSessionBulk(g.classes, g.reservations)),
		nil, nil, nil, config.BulkConfig{}, nil)
	run := func(targets []uuid.UUID, date string) bulkusecase.ExecuteResult {
		t.Helper()
		ids := make([]string, 0, len(targets))
		for _, id := range targets {
			ids = append(ids, id.String())
		}
		res, err := bulk.Execute(ctx, g.ownerID, bulkusecase.ExecuteInput{
			Resource: model.SessionsSlug, Action: model.CancelBookingsAction, Record: true,
			Target: bulkengine.BulkTarget{Scope: "ids", IDs: ids, Params: map[string]string{"class": evening.UUID.String(), "date": date}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	status := func(id uuid.UUID) reservationmodel.Reservation {
		t.Helper()
		r, err := g.reservations.Get(ctx, g.org.ID, id, none)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}

	// A session that started is not cancelled.
	g.now = at(7, 18, 30)
	if _, _, err := g.classes.CancelWithBookings(ctx, g.org.ID, evening.UUID, "2030-01-07", &g.ownerID, ""); !errors.Is(err, usecase.ErrPast) {
		t.Fatalf("started session err = %v", err)
	}
	g.now = at(7, 8, 0)

	before, targets, err := g.classes.CancelWithBookings(ctx, g.org.ID, evening.UUID, "2030-01-07", &g.ownerID, "Eğitmen hasta")
	if err != nil {
		t.Fatal(err)
	}
	if before.Booked != 3 || len(targets) != 2 {
		t.Fatalf("before = %+v, targets = %v", before, targets)
	}
	// The session is off the timetable at once: no booking slips in.
	if _, got := g.slots(t, "2030-01-07"); len(got) != 0 {
		t.Fatalf("cancelled session still offered: %v", got)
	}
	// A booking of another session given to the run is refused.
	res := run(append(targets, later.UUID), "2030-01-07")
	if res.Async || res.Summary.Succeeded != 2 || res.Summary.Failed != 1 || res.Job.UUID == uuid.Nil || res.Job.RollbackUntil == nil {
		t.Fatalf("run = %+v", res)
	}
	for _, id := range []uuid.UUID{first.UUID, second.UUID} {
		if r := status(id); r.Status != reservationmodel.StatusCancelled || r.CancelReason == nil || *r.CancelReason != model.CancelReason {
			t.Fatalf("booking after the run = %s %v", r.Status, r.CancelReason)
		}
	}
	if r := status(later.UUID); r.Status != reservationmodel.StatusActive {
		t.Fatalf("other session's booking = %s", r.Status)
	}
	detail, err := g.classes.SessionDetail(ctx, g.org.ID, evening.UUID, "2030-01-07")
	if err != nil {
		t.Fatal(err)
	}
	if detail.State != model.StateCancelled || detail.Booked != 0 || len(detail.Attendees) != 0 || detail.Reason != "Eğitmen hasta" {
		t.Fatalf("session after the run = %+v", detail)
	}
	job, err := g.classes.CancelJob(ctx, g.org.ID, g.ownerID, evening.UUID, "2030-01-07")
	if err != nil || job == nil || job.JobUUID != res.Job.UUID || job.Succeeded != 2 || job.Failed != 1 {
		t.Fatalf("cancel job = %+v %v", job, err)
	}
	if other, err := g.classes.CancelJob(ctx, g.org.ID, g.ownerID, evening.UUID, "2030-01-14"); err != nil || other != nil {
		t.Fatalf("cancel job of another session = %+v %v", other, err)
	}

	// The rollback opens the session again with both bookings.
	back, err := bulk.Rollback(ctx, res.Job.UUID, g.ownerID)
	if err != nil || back.Status != "rolled_back" {
		t.Fatalf("rollback = %+v %v", back, err)
	}
	for _, id := range []uuid.UUID{first.UUID, second.UUID} {
		r := status(id)
		if r.Status != reservationmodel.StatusActive || r.CancelReason != nil || !hasEvent(r, "reinstated") {
			t.Fatalf("booking after the rollback = %s %v", r.Status, r.CancelReason)
		}
	}
	detail, err = g.classes.SessionDetail(ctx, g.org.ID, evening.UUID, "2030-01-07")
	if err != nil {
		t.Fatal(err)
	}
	if detail.State != model.StateFull || detail.Booked != 3 || len(detail.Attendees) != 2 {
		t.Fatalf("session after the rollback = %+v", detail)
	}
	if _, err := bulk.Rollback(ctx, res.Job.UUID, g.ownerID); !errors.Is(err, bulkusecase.ErrInvalidRequest) {
		t.Fatalf("second rollback err = %v", err)
	}
	if job, err := g.classes.CancelJob(ctx, g.org.ID, g.ownerID, evening.UUID, "2030-01-07"); err != nil || job != nil {
		t.Fatalf("cancel job after the rollback = %+v %v", job, err)
	}

	// Cancelled again; one booking is changed meanwhile and stays
	// cancelled, the other comes back.
	_, targets, err = g.classes.CancelWithBookings(ctx, g.org.ID, evening.UUID, "2030-01-07", &g.ownerID, "")
	if err != nil || len(targets) != 2 {
		t.Fatalf("cancel again: %v %v", targets, err)
	}
	res = run(targets, "2030-01-07")
	if res.Summary.Succeeded != 2 {
		t.Fatalf("second run = %+v", res.Summary)
	}
	note := "aradı"
	if _, err := g.reservations.UpdateNotes(ctx, g.org.ID, first.UUID, none, g.ownerID, reservationmodel.NotesInput{InternalNote: note}); err != nil {
		t.Fatal(err)
	}
	back, err = bulk.Rollback(ctx, res.Job.UUID, g.ownerID)
	if err != nil || back.Status != "rolled_back_partial" {
		t.Fatalf("partial rollback = %+v %v", back, err)
	}
	if r := status(first.UUID); r.Status != reservationmodel.StatusCancelled {
		t.Fatalf("changed booking after the rollback = %s", r.Status)
	}
	if r := status(second.UUID); r.Status != reservationmodel.StatusActive {
		t.Fatalf("other booking after the rollback = %s", r.Status)
	}
	if _, got := g.slots(t, "2030-01-07"); got["18:00 Eğitmen Stüdyo"] != 1 {
		t.Fatalf("session after the partial rollback: %v", got)
	}

	// Without the reservations permission nothing runs.
	limited := authctx.WithPrincipal(orgctx.WithScope(context.Background(), orgctx.Scope{
		ID: g.org.ID, Sector: "gym", Timezone: "Europe/Istanbul", Permissions: []string{rbac.PermTenantClassesWrite},
	}), authctx.Principal{UserInternal: g.ownerID})
	if _, err := bulk.Execute(limited, g.ownerID, bulkusecase.ExecuteInput{
		Resource: model.SessionsSlug, Action: model.CancelBookingsAction, Record: true,
		Target: bulkengine.BulkTarget{Scope: "ids", IDs: []string{second.UUID.String()}, Params: map[string]string{"class": evening.UUID.String(), "date": "2030-01-07"}},
	}); !errors.Is(err, bulkusecase.ErrInvalidRequest) {
		t.Fatalf("run without the reservations permission err = %v", err)
	}
}

func hasEvent(r reservationmodel.Reservation, typ string) bool {
	for _, e := range r.Events {
		if e.Type == typ {
			return true
		}
	}
	return false
}
