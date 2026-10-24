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

// TestClassSessionMoveBookings (TEC-994): the open bookings of a session
// are moved to another session of the class in one bulk run through the
// reservation's own reschedule. A booking that finds no seat stays where it
// was and is reported; the run can be rolled back, which moves the
// bookings back. A booking changed since the move stays where it is.
func TestClassSessionMoveBookings(t *testing.T) {
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
	// The target already has one of its three seats taken.
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
	run := func(targets []uuid.UUID, date, toDate string) bulkusecase.ExecuteResult {
		t.Helper()
		ids := make([]string, 0, len(targets))
		for _, id := range targets {
			ids = append(ids, id.String())
		}
		res, err := bulk.Execute(ctx, g.ownerID, bulkusecase.ExecuteInput{
			Resource: model.SessionsSlug, Action: model.MoveBookingsAction, Record: true,
			Target: bulkengine.BulkTarget{Scope: "ids", IDs: ids, Params: map[string]string{"class": evening.UUID.String(), "date": date, "to_date": toDate}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	get := func(id uuid.UUID) reservationmodel.Reservation {
		t.Helper()
		r, err := g.reservations.Get(ctx, g.org.ID, id, none)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	moveTargets := func(date, toDate string) (model.Session, model.Session, []uuid.UUID, error) {
		return g.classes.MoveTargets(ctx, g.org.ID, evening.UUID, date, model.SessionMoveInput{ToDate: toDate})
	}

	// The target must be another open session of the class.
	if _, _, _, err := moveTargets("2030-01-07", ""); code(err) != "to_date/required" {
		t.Fatalf("empty target err = %v", err)
	}
	if _, _, _, err := moveTargets("2030-01-07", "2030-01-07"); code(err) != "to_date/same_session" {
		t.Fatalf("same session err = %v", err)
	}
	if _, _, _, err := moveTargets("2030-01-07", "2030-01-08"); code(err) != "to_date/not_found" {
		t.Fatalf("day without a session err = %v", err)
	}
	if _, _, _, err := moveTargets("2030-01-08", "2030-01-14"); !errors.Is(err, usecase.ErrSessionNotFound) {
		t.Fatalf("source without a session err = %v", err)
	}
	if _, _, err := g.classes.ChangeSession(ctx, g.org.ID, evening.UUID, "2030-01-21", &g.ownerID, model.SessionChangeInput{Cancelled: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := moveTargets("2030-01-07", "2030-01-21"); code(err) != "to_date/cancelled" {
		t.Fatalf("cancelled target err = %v", err)
	}
	g.now = at(14, 18, 30)
	if _, _, _, err := moveTargets("2030-01-07", "2030-01-14"); code(err) != "to_date/past" {
		t.Fatalf("started target err = %v", err)
	}
	g.now = at(7, 8, 0)

	from, to, targets, err := moveTargets("2030-01-07", "2030-01-14")
	if err != nil {
		t.Fatal(err)
	}
	if from.Booked != 3 || to.SeatsLeft != 2 || len(targets) != 2 {
		t.Fatalf("from = %+v, to = %+v, targets = %v", from, to, targets)
	}
	// One seat fits; the booking of two seats stays and is reported.
	res := run(append(targets, later.UUID), "2030-01-07", "2030-01-14")
	if res.Async || res.Summary.Succeeded != 1 || res.Summary.Failed != 2 || res.Job.UUID == uuid.Nil || res.Job.RollbackUntil == nil {
		t.Fatalf("run = %+v", res)
	}
	issues := map[string]string{}
	for _, i := range res.Summary.Issues {
		issues[i.EntityUUID] = i.Error
	}
	if issues[second.UUID.String()] != "slot_taken" || issues[later.UUID.String()] != "not_in_session" {
		t.Fatalf("issues = %v", issues)
	}
	if r := get(first.UUID); !r.StartsAt.Equal(at(14, 18, 0)) || r.Status != reservationmodel.StatusActive || !hasEvent(r, "rescheduled") {
		t.Fatalf("moved booking = %s %s", r.StartsAt, r.Status)
	}
	if r := get(second.UUID); !r.StartsAt.Equal(at(7, 18, 0)) {
		t.Fatalf("booking that did not fit = %s", r.StartsAt)
	}
	detail, err := g.classes.SessionDetail(ctx, g.org.ID, evening.UUID, "2030-01-07")
	if err != nil || detail.Booked != 2 || len(detail.Attendees) != 1 {
		t.Fatalf("source after the run = %+v %v", detail, err)
	}
	if detail, err = g.classes.SessionDetail(ctx, g.org.ID, evening.UUID, "2030-01-14"); err != nil || detail.Booked != 2 || len(detail.Attendees) != 2 {
		t.Fatalf("target after the run = %+v %v", detail, err)
	}
	job, err := g.classes.MoveJob(ctx, g.org.ID, g.ownerID, evening.UUID, "2030-01-07")
	if err != nil || job == nil || job.JobUUID != res.Job.UUID || job.ToDate != "2030-01-14" || job.Succeeded != 1 || job.Failed != 2 || len(job.Issues) != 2 {
		t.Fatalf("move job = %+v %v", job, err)
	}
	if other, err := g.classes.MoveJob(ctx, g.org.ID, g.ownerID, evening.UUID, "2030-01-14"); err != nil || other != nil {
		t.Fatalf("move job of the target = %+v %v", other, err)
	}
	// A full target is refused up front.
	if _, _, err := g.classes.ChangeSession(ctx, g.org.ID, evening.UUID, "2030-01-14", &g.ownerID, model.SessionChangeInput{Capacity: ptr(2)}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := moveTargets("2030-01-07", "2030-01-14"); code(err) != "to_date/full" {
		t.Fatalf("full target err = %v", err)
	}
	if _, _, err := g.classes.ChangeSession(ctx, g.org.ID, evening.UUID, "2030-01-14", &g.ownerID, model.SessionChangeInput{}); err != nil {
		t.Fatal(err)
	}

	// The rollback moves the booking back.
	back, err := bulk.Rollback(ctx, res.Job.UUID, g.ownerID)
	if err != nil || back.Status != "rolled_back" {
		t.Fatalf("rollback = %+v %v", back, err)
	}
	if r := get(first.UUID); !r.StartsAt.Equal(at(7, 18, 0)) {
		t.Fatalf("booking after the rollback = %s", r.StartsAt)
	}
	if detail, err = g.classes.SessionDetail(ctx, g.org.ID, evening.UUID, "2030-01-07"); err != nil || detail.Booked != 3 || len(detail.Attendees) != 2 {
		t.Fatalf("source after the rollback = %+v %v", detail, err)
	}
	if job, err := g.classes.MoveJob(ctx, g.org.ID, g.ownerID, evening.UUID, "2030-01-07"); err != nil || job != nil {
		t.Fatalf("move job after the rollback = %+v %v", job, err)
	}

	// Moved again; the booking is changed meanwhile and stays in the target.
	_, _, targets, err = moveTargets("2030-01-07", "2030-01-14")
	if err != nil || len(targets) != 2 {
		t.Fatalf("move again: %v %v", targets, err)
	}
	res = run([]uuid.UUID{first.UUID}, "2030-01-07", "2030-01-14")
	if res.Summary.Succeeded != 1 {
		t.Fatalf("second run = %+v", res.Summary)
	}
	if _, err := g.reservations.UpdateNotes(ctx, g.org.ID, first.UUID, none, g.ownerID, reservationmodel.NotesInput{InternalNote: "aradı"}); err != nil {
		t.Fatal(err)
	}
	back, err = bulk.Rollback(ctx, res.Job.UUID, g.ownerID)
	if err != nil || back.Status != "rolled_back_partial" {
		t.Fatalf("partial rollback = %+v %v", back, err)
	}
	if r := get(first.UUID); !r.StartsAt.Equal(at(14, 18, 0)) {
		t.Fatalf("changed booking after the rollback = %s", r.StartsAt)
	}

	// Without the reservations permission nothing runs.
	limited := authctx.WithPrincipal(orgctx.WithScope(context.Background(), orgctx.Scope{
		ID: g.org.ID, Sector: "gym", Timezone: "Europe/Istanbul", Permissions: []string{rbac.PermTenantClassesWrite},
	}), authctx.Principal{UserInternal: g.ownerID})
	if _, err := bulk.Execute(limited, g.ownerID, bulkusecase.ExecuteInput{
		Resource: model.SessionsSlug, Action: model.MoveBookingsAction, Record: true,
		Target: bulkengine.BulkTarget{Scope: "ids", IDs: []string{second.UUID.String()}, Params: map[string]string{"class": evening.UUID.String(), "date": "2030-01-07", "to_date": "2030-01-14"}},
	}); !errors.Is(err, bulkusecase.ErrInvalidRequest) {
		t.Fatalf("run without the reservations permission err = %v", err)
	}
}
