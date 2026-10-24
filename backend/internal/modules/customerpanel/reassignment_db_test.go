package customerpanel_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
)

// platformDo calls the platform routes as the fixture's owner (a platform
// operator; the permission middleware is covered by the route test below).
func (f *fixture) platformDo(t *testing.T, method, path string, body any) result {
	t.Helper()
	ph := handler.NewPlatform(f.svc, nil)
	asOperator := func(next http.HandlerFunc) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next(w, r.WithContext(authctx.WithPrincipal(r.Context(), authctx.Principal{UserInternal: f.owner.ID})))
		})
	}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/platform/customer-phone-releases", asOperator(ph.ListReleases))
	mux.Handle("POST /v1/platform/customer-phone-releases", asOperator(ph.CreateReassignment))
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	out := result{code: rec.Code, raw: rec.Body.String()}
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out.env); err != nil {
			t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body.String())
		}
	}
	return out
}

// TestCustomerPanelPlatformReassignment (A29, TEC-972): a reassignment
// notice processed from Takwim management hides every business's earlier
// history of the number's profile, ends every session of it, and the next
// holder opens only what is booked from then on. A second notice before
// anybody proves the number again is refused.
func TestCustomerPanelPlatformReassignment(t *testing.T) {
	f := newFixture(t)
	previous := f.signInAs(t, f.huseyinPhone, "previous holder")
	visitA := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, at(10, 0))
	f.book(t, f.orgB.ID, f.branchB, f.cutB, f.ayseB, f.huseyinB, at(10, 0))
	f.exec("UPDATE reservations SET created_at = NOW() - INTERVAL '1 day' WHERE organization_id = ANY($1)", []int64{f.orgA.ID, f.orgB.ID})
	// Release times use the database clock; so does the proof here.
	f.exec("UPDATE customer_identities SET phone_verified_at = NOW() - INTERVAL '1 hour' WHERE phone_e164 = $1", f.huseyinPhone)

	if r := f.platformDo(t, http.MethodPost, "/v1/platform/customer-phone-releases", map[string]any{"phone": "12"}); r.code != http.StatusBadRequest || r.detail() != "phone/invalid" {
		t.Fatalf("invalid phone = %d %s", r.code, r.raw)
	}
	r := f.platformDo(t, http.MethodPost, "/v1/platform/customer-phone-releases", map[string]any{
		"phone": national(f.huseyinPhone), "note": "Operatör bildirimi #42",
	})
	if r.code != http.StatusCreated {
		t.Fatalf("reassignment = %d %s", r.code, r.raw)
	}
	rel := decode[model.PhoneRelease](t, r)
	if !rel.HasProfile || rel.BusinessesHidden != 2 || rel.SessionsRevoked != 1 || rel.Reason != model.ReleaseReassigned ||
		rel.Source != model.ReleaseSourcePlatform || rel.PhoneMasked == f.huseyinPhone {
		t.Fatalf("release = %+v", rel)
	}
	if r := f.do(t, http.MethodGet, "/v1/customer/me", previous.AccessToken, nil); r.code != http.StatusUnauthorized {
		t.Fatalf("previous holder's session = %d", r.code)
	}
	if r := f.platformDo(t, http.MethodPost, "/v1/platform/customer-phone-releases", map[string]any{"phone": f.huseyinPhone}); r.code != http.StatusConflict || r.errCode() != handler.CodeAlreadyReleased {
		t.Fatalf("second notice = %d %s", r.code, r.raw)
	}

	holder := f.signInAs(t, f.huseyinPhone, "new holder")
	for _, b := range holder.Profile.Businesses {
		if b.HistoryFrom == nil || b.FirstName != "" || b.ReservationCount != 0 {
			t.Fatalf("business after the notice = %+v", b)
		}
	}
	if n := holder.Profile.HistoryNotice; n == nil || n.Reason != model.ReleaseReassigned {
		t.Fatalf("history notice = %+v", n)
	}
	if r := f.do(t, http.MethodGet, "/v1/customer/reservations/"+visitA.UUID.String(), holder.AccessToken, nil); r.code != http.StatusNotFound {
		t.Fatalf("previous holder's visit = %d", r.code)
	}

	// The platform list: masked number, operator, note; exact phone search.
	list := f.platformDo(t, http.MethodGet, "/v1/platform/customer-phone-releases?q="+url.QueryEscape(f.huseyinPhone), nil)
	if list.code != http.StatusOK {
		t.Fatalf("list = %d %s", list.code, list.raw)
	}
	page := decode[apiquery.Page[model.PhoneRelease]](t, list)
	if page.Total != 1 || page.Items[0].UUID != rel.UUID || page.Items[0].CreatedByName != "owner Test" || page.Items[0].Note != "Operatör bildirimi #42" {
		t.Fatalf("list page = %+v", page)
	}
	if r := f.platformDo(t, http.MethodGet, "/v1/platform/customer-phone-releases?q="+url.QueryEscape("bildirimi #42")+"&source=platform,system", nil); r.code != http.StatusOK || decode[apiquery.Page[model.PhoneRelease]](t, r).Total != 1 {
		t.Fatalf("note search = %d %s", r.code, r.raw)
	}
	if r := f.platformDo(t, http.MethodGet, "/v1/platform/customer-phone-releases?reason=lost", nil); r.code != http.StatusBadRequest || r.detail() != "reason/invalid" {
		t.Fatalf("bad reason = %d %s", r.code, r.raw)
	}
	if r := f.platformDo(t, http.MethodGet, "/v1/platform/customer-phone-releases?sort=phone_e164", nil); r.code != http.StatusBadRequest {
		t.Fatalf("bad sort = %d %s", r.code, r.raw)
	}
	if n := f.count("SELECT COUNT(*) FROM customer_auth_events WHERE phone_e164 = $1 AND kind = 'platform_reassigned'", f.huseyinPhone); n != 1 {
		t.Fatalf("platform_reassigned events = %d", n)
	}

	// A number without a profile is released too, once.
	stranger := "+90534" + f.suffix[:7]
	f.extraPhones = append(f.extraPhones, stranger)
	r = f.platformDo(t, http.MethodPost, "/v1/platform/customer-phone-releases", map[string]any{"phone": stranger})
	if r.code != http.StatusCreated || decode[model.PhoneRelease](t, r).HasProfile {
		t.Fatalf("no profile = %d %s", r.code, r.raw)
	}
	if r := f.platformDo(t, http.MethodPost, "/v1/platform/customer-phone-releases", map[string]any{"phone": stranger}); r.code != http.StatusConflict {
		t.Fatalf("second notice without profile = %d %s", r.code, r.raw)
	}
}

// TestCustomerPanelPlatformRoutes: the platform routes need a staff token.
func TestCustomerPanelPlatformRoutes(t *testing.T) {
	f := newFixture(t)
	mux := http.NewServeMux()
	customerpanel.RegisterPlatformRoutes(mux, handler.NewPlatform(f.svc, nil), f.tokens, nil)
	customer := f.signInAs(t, f.huseyinPhone, "customer")
	for _, token := range []string{"", customer.AccessToken} {
		req := httptest.NewRequest(http.MethodGet, "/v1/platform/customer-phone-releases", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("token %t = %d", token != "", rec.Code)
		}
	}
}

// TestCustomerPanelDormantNumber (A29, A30, TEC-972): a number nobody
// proved for DormantAfter may have changed hands unreported. Its next
// sign-in closes the profile's earlier history and ends its stale
// sessions; later sign-ins change nothing more.
func TestCustomerPanelDormantNumber(t *testing.T) {
	f := newFixture(t)
	stale := f.signInAs(t, f.huseyinPhone, "previous holder")
	visitA := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, at(10, 0))
	f.exec("UPDATE reservations SET created_at = NOW() - INTERVAL '1 day' WHERE organization_id = ANY($1)", []int64{f.orgA.ID, f.orgB.ID})

	// Proven recently: nothing closes.
	again := f.signInAs(t, f.huseyinPhone, "same person")
	if again.Profile.HistoryNotice != nil {
		t.Fatalf("recent number notice = %+v", again.Profile.HistoryNotice)
	}
	for _, b := range again.Profile.Businesses {
		if b.HistoryFrom != nil {
			t.Fatalf("recent number business = %+v", b)
		}
	}

	f.exec("UPDATE customer_identities SET phone_verified_at = $2 WHERE phone_e164 = $1", f.huseyinPhone, f.now.Add(-200*24*time.Hour))
	holder := f.signInAs(t, f.huseyinPhone, "next holder")
	if len(holder.Profile.Businesses) != 2 {
		t.Fatalf("businesses = %+v", holder.Profile.Businesses)
	}
	for _, b := range holder.Profile.Businesses {
		if b.HistoryFrom == nil || b.ReservationCount != 0 {
			t.Fatalf("dormant business = %+v", b)
		}
	}
	if n := holder.Profile.HistoryNotice; n == nil || n.Reason != model.ReleaseDormant {
		t.Fatalf("history notice = %+v", n)
	}
	if r := f.do(t, http.MethodGet, "/v1/customer/reservations/"+visitA.UUID.String(), holder.AccessToken, nil); r.code != http.StatusNotFound {
		t.Fatalf("earlier visit = %d", r.code)
	}
	for _, s := range []model.Session{stale, again} {
		if r := f.do(t, http.MethodGet, "/v1/customer/me", s.AccessToken, nil); r.code != http.StatusUnauthorized {
			t.Fatalf("stale session = %d", r.code)
		}
	}
	if r := f.do(t, http.MethodGet, "/v1/customer/me", holder.AccessToken, nil); r.code != http.StatusOK {
		t.Fatalf("new session = %d %s", r.code, r.raw)
	}

	f.signInAs(t, f.huseyinPhone, "next holder again")
	if n := f.count("SELECT COUNT(*) FROM customer_phone_releases WHERE phone_e164 = $1 AND reason = 'dormant' AND source = 'system' AND businesses_hidden = 2", f.huseyinPhone); n != 1 {
		t.Fatalf("dormant releases = %d", n)
	}
	if n := f.count("SELECT COUNT(*) FROM customer_auth_events WHERE phone_e164 = $1 AND kind = 'dormant_closed'", f.huseyinPhone); n != 1 {
		t.Fatalf("dormant events = %d", n)
	}
}
