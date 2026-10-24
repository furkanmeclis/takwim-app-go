package confirmations_test

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/confirmations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/confirmations/usecase"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	resusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

func states(items []model.Item) map[string]model.Item {
	out := map[string]model.Item{}
	for _, it := range items {
		out[it.State] = it
	}
	return out
}

// TestConfirmationList (TEC-924, A08): the list shows the business's
// approval and the customer's confirmation side by side; a change request
// needs a person; a phone confirmation closes the task with the member in
// the history; the branch scope applies to the list, the attempts and the
// quick confirm.
func TestConfirmationList(t *testing.T) {
	f := newFixture(t, true)
	ctx := orgctx.WithScope(context.Background(), f.scope())
	r1 := f.book(t, at(7, 14, 0))
	r2 := f.book(t, at(7, 15, 0))
	r3 := f.book(t, at(7, 16, 0))
	f.tick(t, at(6, 16, 0))
	if err := f.engine.Answer(ctx, f.org.ID, r2.UUID, usecase.AnswerInput{Answer: model.AnswerChange, Note: "Akşam olur mu?"}); err != nil {
		t.Fatal(err)
	}
	stale := r3.Version
	if err := f.engine.ConfirmByPhone(ctx, f.org.ID, r3.UUID, f.owner.ID, model.ConfirmInput{Note: "Aradım", ExpectedVersion: &stale}); !errors.Is(err, resusecase.ErrStaleVersion) {
		t.Fatalf("stale phone confirm = %v", err)
	}
	if err := f.engine.ConfirmByPhone(ctx, f.org.ID, r3.UUID, f.owner.ID, model.ConfirmInput{Note: "Aradım, gelecek"}); err != nil {
		t.Fatal(err)
	}
	if err := f.engine.ConfirmByPhone(ctx, f.org.ID, r3.UUID, f.owner.ID, model.ConfirmInput{}); !errors.Is(err, usecase.ErrNotOpen) {
		t.Fatalf("second phone confirm = %v", err)
	}
	got := f.get(t, r3)
	last := got.Events[len(got.Events)-1]
	if got.ConfirmationStatus != resmodel.ConfirmationConfirmed || *got.Confirmation.AnswerChannel != model.SourcePhone ||
		last.Type != usecase.EventConfirmed || last.Note != "Aradım, gelecek" || last.Actor == nil {
		t.Fatalf("phone confirm: %s %+v %+v", got.ConfirmationStatus, got.Confirmation, last)
	}

	items, total, err := f.engine.List(ctx, f.org.ID, model.ListFilter{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	by := states(items)
	if total != 3 || len(items) != 3 || by[model.StatePending].UUID != r1.UUID || by[model.StateChangeRequested].UUID != r2.UUID ||
		by[model.StateConfirmed].UUID != r3.UUID {
		t.Fatalf("list %d %+v", total, items)
	}
	pending := by[model.StatePending]
	if pending.ApprovalStatus != resmodel.ApprovalAccepted || pending.AttemptsSent != 1 || pending.AttemptsPlanned != 2 ||
		pending.LastSend == nil || pending.Attention || !pending.CanConfirm || pending.Customer.Phone == nil {
		t.Fatalf("pending row %+v", pending)
	}
	if ch := by[model.StateChangeRequested]; !ch.Attention || ch.ChangeNote != "Akşam olur mu?" || !ch.CanConfirm {
		t.Fatalf("change request row %+v", ch)
	}
	if c := by[model.StateConfirmed]; c.Attention || c.CanConfirm {
		t.Fatalf("confirmed row %+v", c)
	}

	yes := true
	items, total, err = f.engine.List(ctx, f.org.ID, model.ListFilter{Limit: 20, Attention: &yes})
	if err != nil || total != 1 || len(items) != 1 || items[0].UUID != r2.UUID {
		t.Fatalf("attention list %d %+v %v", total, items, err)
	}
	items, total, err = f.engine.List(ctx, f.org.ID, model.ListFilter{Limit: 20, States: []string{model.StateConfirmed, model.StatePending}})
	if err != nil || total != 2 || len(items) != 2 {
		t.Fatalf("state filter %d %+v %v", total, items, err)
	}
	from, to := at(7, 14, 30), at(7, 15, 30)
	if items, total, err = f.engine.List(ctx, f.org.ID, model.ListFilter{Limit: 20, StartsFrom: &from, StartsTo: &to}); err != nil ||
		total != 1 || items[0].UUID != r2.UUID {
		t.Fatalf("time filter %d %+v %v", total, items, err)
	}
	if items, _, err = f.engine.List(ctx, f.org.ID, model.ListFilter{Limit: 20, Q: "5321234"}); err != nil || len(items) != 3 {
		t.Fatalf("phone search %+v %v", items, err)
	}

	facets, err := f.engine.Facets(ctx, f.org.ID, model.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int64{}
	for _, fc := range facets.State {
		counts[fc.Value] = fc.Count
	}
	if facets.Attention != 1 || counts[model.StatePending] != 1 || counts[model.StateChangeRequested] != 1 || counts[model.StateConfirmed] != 1 {
		t.Fatalf("facets %+v", facets)
	}
	sum, err := f.engine.Summary(ctx, f.org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sum != (model.Summary{Attention: 1, ChangeRequested: 1, Waiting: 1}) {
		t.Fatalf("summary %+v", sum)
	}

	attempts, err := f.engine.Attempts(ctx, f.org.ID, r1.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].Channel == nil || *attempts[0].Channel != "sms" || attempts[0].Round != 1 ||
		attempts[0].Request != 1 || attempts[0].Recipient == "" {
		t.Fatalf("attempts %+v", attempts)
	}

	// A member limited to another branch sees nothing of this one.
	other := f.scope()
	other.BranchIDs = []int64{f.branchID + 1000000}
	octx := orgctx.WithScope(context.Background(), other)
	if items, total, err = f.engine.List(octx, f.org.ID, model.ListFilter{Limit: 20}); err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("other branch list %d %+v %v", total, items, err)
	}
	if _, err := f.engine.Attempts(octx, f.org.ID, r1.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("other branch attempts = %v", err)
	}
	if err := f.engine.ConfirmByPhone(octx, f.org.ID, r1.UUID, f.owner.ID, model.ConfirmInput{}); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("other branch confirm = %v", err)
	}
	if sum, err = f.engine.Summary(octx, f.org.ID); err != nil || sum != (model.Summary{}) {
		t.Fatalf("other branch summary %+v %v", sum, err)
	}
}

// TestConfirmationAlerts: requests no channel could take need a person
// before the deadline; past the deadline the booking is unreachable and
// still needs one until the visit; a phone confirmation clears it.
func TestConfirmationAlerts(t *testing.T) {
	f := newFixture(t, true)
	ctx := orgctx.WithScope(context.Background(), f.scope())
	f.sms.ready = false
	r := f.book(t, at(7, 14, 0))
	f.tick(t, at(6, 14, 0))
	items, _, err := f.engine.List(ctx, f.org.ID, model.ListFilter{Limit: 20})
	if err != nil || len(items) != 1 {
		t.Fatalf("list %+v %v", items, err)
	}
	if it := items[0]; !it.SendFailed || !it.Attention || it.FailedSends != 1 || it.State != model.StatePending {
		t.Fatalf("failed request row %+v", it)
	}
	sum, err := f.engine.Summary(ctx, f.org.ID)
	if err != nil || sum.SendFailed != 1 || sum.Attention != 1 {
		t.Fatalf("summary %+v %v", sum, err)
	}
	f.tick(t, at(7, 11, 0))
	f.tick(t, at(7, 12, 0))
	sum, err = f.engine.Summary(ctx, f.org.ID)
	if err != nil || sum.Unreachable != 1 || sum.Attention != 1 || sum.SendFailed != 0 {
		t.Fatalf("unreachable summary %+v %v", sum, err)
	}
	if err := f.engine.ConfirmByPhone(ctx, f.org.ID, r.UUID, f.owner.ID, model.ConfirmInput{}); err != nil {
		t.Fatal(err)
	}
	if sum, err = f.engine.Summary(ctx, f.org.ID); err != nil || sum.Attention != 0 {
		t.Fatalf("after the call %+v %v", sum, err)
	}
	// After the visit nothing needs a person.
	f.now = at(7, 12, 30)
	f.sms.ready = true
	r2 := f.book(t, at(7, 16, 0))
	f.tick(t, at(7, 12, 31))
	if err := f.engine.Answer(ctx, f.org.ID, r2.UUID, usecase.AnswerInput{Answer: model.AnswerChange}); err != nil {
		t.Fatal(err)
	}
	if sum, err = f.engine.Summary(ctx, f.org.ID); err != nil || sum.ChangeRequested != 1 {
		t.Fatalf("change request %+v %v", sum, err)
	}
	f.now = at(7, 16, 30)
	if sum, err = f.engine.Summary(ctx, f.org.ID); err != nil || sum.Attention != 0 || sum.ChangeRequested != 0 {
		t.Fatalf("after the visit %+v %v", sum, err)
	}
}
