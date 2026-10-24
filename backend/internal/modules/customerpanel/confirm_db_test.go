package customerpanel_test

import (
	"net/http"
	"testing"
	"time"

	confirmationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/confirmations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/model"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
)

// TestCustomerPanelConfirmation (TEC-921): the visit link and the signed-in
// panel offer "Geleceğim" and a change request while the business asks for
// a confirmation; opening the link answers nothing; an answer is taken
// once; without the add-on nothing is offered.
func TestCustomerPanelConfirmation(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	f.exec(`UPDATE organizations SET plan = 'enterprise', addons = '{confirmation}' WHERE id = $1`, f.orgA.ID)
	engine := confirmationsusecase.New(f.pool, f.q, nil)
	engine.SetClock(func() time.Time { return f.now })
	f.reservations.SetNotifier(engine)
	engine.SetReservations(f.reservations)
	f.svc.SetConfirmations(engine)

	f.now = at(8, 0).Add(26 * time.Hour) // Tuesday 10:00
	start := at(10, 0).AddDate(0, 0, 7)  // next Monday 10:00
	first := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, start)
	if first.ConfirmationStatus != reservationsmodel.ConfirmationScheduled {
		t.Fatalf("confirmation = %s", first.ConfirmationStatus)
	}
	link, err := f.svc.IssueVisitLink(ctx, f.orgA.ID, first.UUID)
	if err != nil {
		t.Fatal(err)
	}
	visit := linkRE.FindStringSubmatch(link.URL)[2]
	r := f.do(t, http.MethodPost, "/v1/customer/visit", "", map[string]any{"token": visit})
	detail := decode[model.Reservation](t, r)
	deadline := start.Add(-2 * time.Hour)
	if !detail.Actions.Confirm.Allowed || !detail.Actions.Confirm.Until.Equal(deadline) || !detail.Actions.RequestChange.Allowed ||
		detail.Confirmation == nil || !detail.Confirmation.DeadlineAt.Equal(deadline) {
		t.Fatalf("visit = %+v %+v", detail.Actions, detail.Confirmation)
	}
	// Opening the link (a preview bot) changed nothing.
	if n := f.count(`SELECT count(*) FROM reservations WHERE organization_id = $1 AND uuid = $2 AND confirmation_status = 'scheduled'`, f.orgA.ID, first.UUID); n != 1 {
		t.Fatal("opening the visit link answered")
	}
	r = f.do(t, http.MethodPost, "/v1/customer/visit/change-request", "", map[string]any{"token": visit, "note": "Salı olur mu?"})
	if r.code != http.StatusOK {
		t.Fatalf("change request = %d %s", r.code, r.raw)
	}
	detail = decode[model.Reservation](t, r)
	if detail.Confirmation.ChangeRequestedAt == nil || detail.Actions.RequestChange.Allowed || detail.Status != reservationsmodel.StatusActive {
		t.Fatalf("after change request = %+v", detail)
	}
	r = f.do(t, http.MethodPost, "/v1/customer/visit/confirm", "", map[string]any{"token": visit})
	if r.code != http.StatusOK || decode[model.Reservation](t, r).ConfirmationStatus != reservationsmodel.ConfirmationConfirmed {
		t.Fatalf("visit confirm = %d %s", r.code, r.raw)
	}
	r = f.do(t, http.MethodPost, "/v1/customer/visit/confirm", "", map[string]any{"token": visit})
	if r.code != http.StatusConflict || r.errCode() != handler.CodeActionNotAllowed || r.detail() != "confirm/"+model.ReasonAnswered {
		t.Fatalf("second confirm = %d %s", r.code, r.raw)
	}

	// Signed in: the panel's own confirm.
	second := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, start.Add(time.Hour))
	s := f.signIn(t, f.huseyinPhone, f.huseyinPhone)
	r = f.do(t, http.MethodPost, "/v1/customer/reservations/"+second.UUID.String()+"/confirm", s.AccessToken, map[string]any{})
	if r.code != http.StatusOK || decode[model.Reservation](t, r).ConfirmationStatus != reservationsmodel.ConfirmationConfirmed {
		t.Fatalf("panel confirm = %d %s", r.code, r.raw)
	}

	// Without the add-on: nothing to answer.
	f.exec(`UPDATE organizations SET addons = '{}' WHERE id = $1`, f.orgA.ID)
	third := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, start.Add(-time.Hour))
	r = f.do(t, http.MethodPost, "/v1/customer/reservations/"+third.UUID.String()+"/confirm", s.AccessToken, map[string]any{})
	if r.code != http.StatusConflict || r.detail() != "confirm/"+model.ReasonNotRequested {
		t.Fatalf("confirm without the add-on = %d %s", r.code, r.raw)
	}
}
