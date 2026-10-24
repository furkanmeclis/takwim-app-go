package customerpanel_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/model"
	customersrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/repository"
	customersusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
)

// TestCustomerPanelHistoryRestore (A29, A30, TEC-991): after a release
// closed the earlier history, the person asks one business from the panel
// to reopen it; the business reopens only its own history after checking
// who the person is. Other businesses stay closed, a request while the
// history is open or a restore with nothing hidden is refused, and the next
// closing clears the trail.
func TestCustomerPanelHistoryRestore(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	customers := customersusecase.New(customersrepository.NewPostgres(f.pool, f.q), nil)

	f.signInAs(t, f.huseyinPhone, "holder")
	visitA := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, at(10, 0))
	visitB := f.book(t, f.orgB.ID, f.branchB, f.cutB, f.ayseB, f.huseyinB, at(10, 0))
	f.exec("UPDATE reservations SET created_at = NOW() - INTERVAL '1 day' WHERE organization_id = ANY($1)", []int64{f.orgA.ID, f.orgB.ID})
	f.exec("UPDATE customer_identities SET phone_verified_at = NOW() - INTERVAL '1 hour' WHERE phone_e164 = $1", f.huseyinPhone)
	if r := f.platformDo(t, http.MethodPost, "/v1/platform/customer-phone-releases", map[string]any{"phone": f.huseyinPhone}); r.code != http.StatusCreated {
		t.Fatalf("reassignment = %d %s", r.code, r.raw)
	}

	// The same person comes back: everything is hidden, nothing requested.
	holder := f.signInAs(t, f.huseyinPhone, "same person, later")
	businessOf := func(p model.Profile, id uuid.UUID) model.Business {
		t.Helper()
		for _, b := range p.Businesses {
			if b.UUID == id {
				return b
			}
		}
		t.Fatalf("business %v not in %+v", id, p.Businesses)
		return model.Business{}
	}
	if b := businessOf(holder.Profile, f.orgA.UUID); b.HistoryFrom == nil || b.HistoryRequestedAt != nil || b.HistoryRestoredAt != nil {
		t.Fatalf("closed business = %+v", b)
	}

	// The request: unknown business 404; the first request keeps its time.
	if r := f.do(t, http.MethodPost, "/v1/customer/businesses/no-such-business/history-request", holder.AccessToken, nil); r.code != http.StatusNotFound {
		t.Fatalf("unknown business = %d %s", r.code, r.raw)
	}
	r := f.do(t, http.MethodPost, "/v1/customer/businesses/"+f.orgA.Slug+"/history-request", holder.AccessToken, nil)
	if r.code != http.StatusOK {
		t.Fatalf("request = %d %s", r.code, r.raw)
	}
	first := businessOf(decode[model.Profile](t, r), f.orgA.UUID)
	if first.HistoryRequestedAt == nil || first.HistoryFrom == nil {
		t.Fatalf("requested business = %+v", first)
	}
	if b := businessOf(decode[model.Profile](t, r), f.orgB.UUID); b.HistoryRequestedAt != nil {
		t.Fatalf("other business got the request = %+v", b)
	}
	r = f.do(t, http.MethodPost, "/v1/customer/businesses/"+f.orgA.Slug+"/history-request", holder.AccessToken, nil)
	if again := businessOf(decode[model.Profile](t, r), f.orgA.UUID); r.code != http.StatusOK || !again.HistoryRequestedAt.Equal(*first.HistoryRequestedAt) {
		t.Fatalf("second request = %d %+v", r.code, again)
	}
	if n := f.count("SELECT COUNT(*) FROM customer_auth_events WHERE phone_e164 = $1 AND kind = 'history_requested'", f.huseyinPhone); n != 2 {
		t.Fatalf("history_requested events = %d", n)
	}

	// Business A sees the request on its record and reopens the history.
	rec, err := customers.Get(ctx, f.orgA.ID, f.huseyinA)
	if err != nil {
		t.Fatal(err)
	}
	if v := rec.Verification; v == nil || v.HistoryHiddenFrom == nil || v.HistoryRequestedAt == nil || v.HistoryRestoredAt != nil {
		t.Fatalf("record before restore = %+v", rec.Verification)
	}
	restored, err := customers.RestoreHistory(ctx, f.orgA.ID, f.huseyinA, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v := restored.Verification; v == nil || v.HistoryHiddenFrom != nil || v.HistoryRequestedAt != nil || v.HistoryRestoredAt == nil || v.HistoryRestoredBy != "owner Test" {
		t.Fatalf("record after restore = %+v", restored.Verification)
	}
	if n := f.count("SELECT COUNT(*) FROM customer_identity_links WHERE organization_id = $1 AND visible_from IS NULL AND history_restored_from IS NOT NULL AND history_restored_by = $2", f.orgA.ID, f.owner.ID); n != 1 {
		t.Fatalf("restored links = %d", n)
	}
	if n := f.count("SELECT COUNT(*) FROM customer_auth_events WHERE phone_e164 = $1 AND kind = 'history_restored'", f.huseyinPhone); n != 1 {
		t.Fatalf("history_restored events = %d", n)
	}

	// The panel: A's history is back (visits, name); B stays closed.
	me := f.do(t, http.MethodGet, "/v1/customer/me", holder.AccessToken, nil)
	if me.code != http.StatusOK {
		t.Fatalf("me = %d %s", me.code, me.raw)
	}
	profile := decode[model.Profile](t, me)
	if a := businessOf(profile, f.orgA.UUID); a.HistoryFrom != nil || a.HistoryRestoredAt == nil || a.ReservationCount != 1 || a.FirstName != "Hüseyin" {
		t.Fatalf("reopened business = %+v", a)
	}
	if b := businessOf(profile, f.orgB.UUID); b.HistoryFrom == nil || b.ReservationCount != 0 || b.FirstName != "" {
		t.Fatalf("other business = %+v", b)
	}
	if r := f.do(t, http.MethodGet, "/v1/customer/reservations/"+visitA.UUID.String(), holder.AccessToken, nil); r.code != http.StatusOK {
		t.Fatalf("reopened visit = %d %s", r.code, r.raw)
	}
	if r := f.do(t, http.MethodGet, "/v1/customer/reservations/"+visitB.UUID.String(), holder.AccessToken, nil); r.code != http.StatusNotFound {
		t.Fatalf("other business's visit = %d", r.code)
	}

	// Nothing left to reopen: the panel request and the restore are refused.
	if r := f.do(t, http.MethodPost, "/v1/customer/businesses/"+f.orgA.Slug+"/history-request", holder.AccessToken, nil); r.code != http.StatusConflict ||
		r.errCode() != handler.CodeActionNotAllowed || r.detail() != "history_request/"+model.ReasonHistoryOpen {
		t.Fatalf("request with open history = %d %s", r.code, r.raw)
	}
	if _, err := customers.RestoreHistory(ctx, f.orgA.ID, f.huseyinA, f.owner.ID); !errors.Is(err, customersusecase.ErrHistoryOpen) {
		t.Fatalf("second restore = %v", err)
	}
	// An unlinked record has no history to reopen; B's record is B's.
	if _, err := customers.RestoreHistory(ctx, f.orgA.ID, f.zeynep, f.owner.ID); !errors.Is(err, customersusecase.ErrHistoryOpen) {
		t.Fatalf("unlinked restore = %v", err)
	}
	if _, err := customers.RestoreHistory(ctx, f.orgA.ID, f.huseyinB, f.owner.ID); !errors.Is(err, customersusecase.ErrHistoryOpen) && !errors.Is(err, customersusecase.ErrNotFound) {
		t.Fatalf("cross-business restore = %v", err)
	}

	// The next closing hides A again and clears the trail of this reopening.
	f.exec("UPDATE customer_identities SET phone_verified_at = NOW() WHERE phone_e164 = $1", f.huseyinPhone)
	if r := f.platformDo(t, http.MethodPost, "/v1/platform/customer-phone-releases", map[string]any{"phone": f.huseyinPhone}); r.code != http.StatusCreated {
		t.Fatalf("second reassignment = %d %s", r.code, r.raw)
	}
	rec, err = customers.Get(ctx, f.orgA.ID, f.huseyinA)
	if err != nil {
		t.Fatal(err)
	}
	if v := rec.Verification; v == nil || v.HistoryHiddenFrom == nil || v.HistoryRequestedAt != nil || v.HistoryRestoredAt != nil || v.HistoryRestoredBy != "" {
		t.Fatalf("record after the next closing = %+v", rec.Verification)
	}
}
