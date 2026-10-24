package customerpanel_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/usecase"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
)

var linkRE = regexp.MustCompile(`/(l|v)#([A-Za-z0-9_-]{32})`)

// lastMessage is the text of the last message of purpose to phone.
func (c *capture) lastMessage(t *testing.T, phone, purpose string) string {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.msgs) - 1; i >= 0; i-- {
		if c.msgs[i].To == phone && c.msgs[i].Purpose == purpose {
			return c.msgs[i].Text
		}
	}
	t.Fatalf("no %s message to %s", purpose, phone)
	return ""
}

// panelLink issues the panel link a customer notice carries and returns
// its token.
func (f *fixture) panelLink(t *testing.T, phone string) string {
	t.Helper()
	link := f.svc.PanelLink(context.Background(), phone)
	m := linkRE.FindStringSubmatch(link)
	if m == nil || m[1] != "l" {
		t.Fatalf("no panel link in %q", link)
	}
	return m[2]
}

// signInAs signs in with a user agent (device list).
func (f *fixture) signInAs(t *testing.T, phone, userAgent string) model.Session {
	t.Helper()
	f.now = f.now.Add(2 * time.Minute) // past the resend cooldown
	if r := f.do(t, http.MethodPost, "/v1/customer/auth/start", "", map[string]any{"phone": phone}); r.code != http.StatusAccepted {
		t.Fatalf("start = %d %s", r.code, r.raw)
	}
	body := `{"phone":"` + phone + `","code":"` + f.sender.lastCode(t, phone) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/customer/auth/verify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("verify = %d %s", rec.Code, rec.Body.String())
	}
	var env envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	return decode[model.Session](t, result{code: rec.Code, raw: rec.Body.String(), env: env})
}

// TestCustomerPanelLinks (A27): a panel link is single use and expires;
// the token is stored only as a hash; a visit link opens exactly one visit
// and nothing else; the two token types never cross.
func TestCustomerPanelLinks(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	token := f.panelLink(t, f.huseyinPhone)
	if f.count("SELECT COUNT(*) FROM customer_access_links WHERE token_hash = $1 OR token_hash LIKE '%' || $1 || '%'", token) != 0 ||
		f.count("SELECT COUNT(*) FROM customer_access_links WHERE phone_e164 = $1", f.huseyinPhone) != 1 {
		t.Fatal("link token stored in clear or not stored")
	}
	r := f.do(t, http.MethodPost, "/v1/customer/auth/link", "", map[string]any{"token": token})
	if r.code != http.StatusOK {
		t.Fatalf("redeem = %d %s", r.code, r.raw)
	}
	session := decode[model.Session](t, r)
	if session.AccessToken == "" || session.Profile.Phone != f.huseyinPhone || len(session.Profile.Businesses) != 2 {
		t.Fatalf("link session = %+v", session)
	}
	if n := f.count(`SELECT COUNT(*) FROM customer_identity_links l WHERE l.organization_id = ANY($1) AND l.source = 'panel_link'`, []int64{f.orgA.ID, f.orgB.ID}); n != 2 {
		t.Fatalf("panel_link links = %d", n)
	}
	devices := decode[model.DeviceList](t, f.do(t, http.MethodGet, "/v1/customer/sessions", session.AccessToken, nil))
	if len(devices.Items) != 1 || devices.Items[0].Method != model.MethodLink || !devices.Items[0].Current {
		t.Fatalf("devices = %+v", devices)
	}
	// Used once: a second redemption is refused.
	if r := f.do(t, http.MethodPost, "/v1/customer/auth/link", "", map[string]any{"token": token}); r.code != http.StatusBadRequest || r.errCode() != handler.CodeLinkInvalid {
		t.Fatalf("reused link = %d %s", r.code, r.raw)
	}
	// Expired: a day later the link opens nothing.
	late := f.panelLink(t, f.huseyinPhone)
	f.now = f.now.Add(25 * time.Hour)
	if r := f.do(t, http.MethodPost, "/v1/customer/auth/link", "", map[string]any{"token": late}); r.code != http.StatusBadRequest || r.errCode() != handler.CodeLinkInvalid {
		t.Fatalf("expired link = %d %s", r.code, r.raw)
	}
	for _, bad := range []string{"", "short", strings.Repeat("A", 32), token + "x"} {
		if r := f.do(t, http.MethodPost, "/v1/customer/auth/link", "", map[string]any{"token": bad}); r.code != http.StatusBadRequest {
			t.Fatalf("bad link %q = %d", bad, r.code)
		}
	}
	if n := f.count("SELECT COUNT(*) FROM customer_auth_events WHERE kind = 'link_failed' AND created_at >= NOW() - INTERVAL '1 minute'"); n < 6 {
		t.Fatalf("link failures recorded = %d", n)
	}
	// Daily cap: past five links the message carries the plain address.
	var last string
	for i := 0; i < 6; i++ {
		last = f.svc.PanelLink(ctx, f.zeynepPhone)
	}
	if strings.Contains(last, "#") || !strings.HasSuffix(last, "/panel") {
		t.Fatalf("link past the cap: %q", last)
	}

	// Visit link: one reservation, no panel.
	f.now = at(8, 0).Add(26 * time.Hour) // Tuesday 10:00
	first := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, at(10, 0).AddDate(0, 0, 7))
	other := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, at(11, 0).AddDate(0, 0, 7))
	link, err := f.svc.IssueVisitLink(ctx, f.orgA.ID, first.UUID)
	if err != nil {
		t.Fatal(err)
	}
	m := linkRE.FindStringSubmatch(link.URL)
	if m == nil || m[1] != "v" {
		t.Fatalf("visit link = %s", link.URL)
	}
	visit := m[2]
	if _, err := f.svc.IssueVisitLink(ctx, f.orgB.ID, first.UUID); err == nil {
		t.Fatal("visit link of another business's reservation")
	}
	r = f.do(t, http.MethodPost, "/v1/customer/visit", "", map[string]any{"token": visit})
	if r.code != http.StatusOK {
		t.Fatalf("visit = %d %s", r.code, r.raw)
	}
	detail := decode[model.Reservation](t, r)
	if detail.UUID != first.UUID || detail.Actions.Reschedule.Allowed || detail.Actions.Reschedule.Reason == nil ||
		*detail.Actions.Reschedule.Reason != model.ReasonSignInRequired || !detail.Actions.Cancel.Allowed {
		t.Fatalf("visit detail = %+v", detail)
	}
	if strings.Contains(r.raw, other.UUID.String()) || strings.Contains(r.raw, f.huseyinPhone) || strings.Contains(r.raw, "personel notu") {
		t.Fatal("visit link shows more than its visit")
	}
	// Separate token types: a visit link is no panel link, no session.
	if r := f.do(t, http.MethodPost, "/v1/customer/auth/link", "", map[string]any{"token": visit}); r.code != http.StatusBadRequest {
		t.Fatalf("visit token as panel link = %d", r.code)
	}
	if r := f.do(t, http.MethodGet, "/v1/customer/reservations", visit, nil); r.code != http.StatusUnauthorized {
		t.Fatalf("visit token as session = %d", r.code)
	}
	panel := f.panelLink(t, f.huseyinPhone)
	if r := f.do(t, http.MethodPost, "/v1/customer/visit", "", map[string]any{"token": panel}); r.code != http.StatusBadRequest || r.errCode() != handler.CodeLinkInvalid {
		t.Fatalf("panel token as visit link = %d", r.code)
	}
	// The visit link cancels its visit under the business's rules.
	r = f.do(t, http.MethodPost, "/v1/customer/visit/cancel", "", map[string]any{"token": visit, "expected_version": detail.Version})
	if r.code != http.StatusOK || decode[model.Reservation](t, r).Status != reservationsmodel.StatusCancelled {
		t.Fatalf("visit cancel = %d %s", r.code, r.raw)
	}
	if n := f.count("SELECT COUNT(*) FROM reservations WHERE organization_id = $1 AND uuid = $2 AND status = 'active'", f.orgA.ID, other.UUID); n != 1 {
		t.Fatal("the other visit changed")
	}
	// A day after the visit the link is over.
	f.now = first.EndsAt.Add(25 * time.Hour)
	if r := f.do(t, http.MethodPost, "/v1/customer/visit", "", map[string]any{"token": visit}); r.code != http.StatusBadRequest {
		t.Fatalf("expired visit link = %d", r.code)
	}
}

// TestCustomerPanelDevices: the device list shows the person's live
// sessions only; one device is signed out at once; another person's
// session is not reachable.
func TestCustomerPanelDevices(t *testing.T) {
	f := newFixture(t)
	phoneUA := "Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 Chrome/126.0 Mobile Safari/537.36"
	s1 := f.signInAs(t, f.huseyinPhone, phoneUA)
	s2 := f.signInAs(t, f.huseyinPhone, "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 Version/17.5 Mobile/15E148 Safari/604.1")
	z := f.signInAs(t, f.zeynepPhone, phoneUA)

	r := f.do(t, http.MethodGet, "/v1/customer/sessions", s1.AccessToken, nil)
	list := decode[model.DeviceList](t, r)
	if r.code != http.StatusOK || len(list.Items) != 2 {
		t.Fatalf("devices = %d %s", r.code, r.raw)
	}
	var other uuid.UUID
	labels := map[string]bool{}
	for _, d := range list.Items {
		labels[d.Device] = true
		if !d.Current {
			other = d.UUID
		}
		if d.IP != "" && !strings.HasSuffix(d.IP, "*") {
			t.Fatalf("unmasked ip %q", d.IP)
		}
	}
	if !labels["Chrome · Android"] || !labels["Safari · iPhone"] || other == uuid.Nil {
		t.Fatalf("devices = %+v", list.Items)
	}
	// Zeynep's session is not Hüseyin's to end.
	zl := decode[model.DeviceList](t, f.do(t, http.MethodGet, "/v1/customer/sessions", z.AccessToken, nil))
	if r := f.do(t, http.MethodDelete, "/v1/customer/sessions/"+zl.Items[0].UUID.String(), s1.AccessToken, nil); r.code != http.StatusNotFound {
		t.Fatalf("other person's session = %d", r.code)
	}
	if r := f.do(t, http.MethodDelete, "/v1/customer/sessions/not-a-uuid", s1.AccessToken, nil); r.code != http.StatusNotFound {
		t.Fatalf("bad uuid = %d", r.code)
	}
	if r := f.do(t, http.MethodDelete, "/v1/customer/sessions/"+other.String(), s1.AccessToken, nil); r.code != http.StatusNoContent {
		t.Fatalf("revoke = %d %s", r.code, r.raw)
	}
	if r := f.do(t, http.MethodGet, "/v1/customer/me", s2.AccessToken, nil); r.code != http.StatusUnauthorized {
		t.Fatalf("revoked device = %d", r.code)
	}
	if r := f.do(t, http.MethodGet, "/v1/customer/me", z.AccessToken, nil); r.code != http.StatusOK {
		t.Fatalf("zeynep = %d", r.code)
	}
}

// TestCustomerPanelNumberChange (A29, A30): a number change keeps the
// person's history on the new number and never hands it to the next holder
// of the old one; the codes of the two purposes never cross; a number with
// its own profile is refused.
func TestCustomerPanelNumberChange(t *testing.T) {
	f := newFixture(t)
	newPhone := "+90534" + f.suffix[:7]
	f.extraPhones = append(f.extraPhones, newPhone)
	old := f.huseyinPhone

	s1 := f.signInAs(t, old, "web")
	s2 := f.signInAs(t, old, "phone")
	visit := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, at(10, 0))

	if r := f.do(t, http.MethodPost, "/v1/customer/phone/start", s1.AccessToken, map[string]any{"phone": old}); r.code != http.StatusBadRequest || r.detail() != "phone/same" {
		t.Fatalf("same phone = %d %s", r.code, r.raw)
	}
	if r := f.do(t, http.MethodPost, "/v1/customer/phone/start", "", map[string]any{"phone": newPhone}); r.code != http.StatusUnauthorized {
		t.Fatalf("anonymous change = %d", r.code)
	}
	if r := f.do(t, http.MethodPost, "/v1/customer/phone/start", s1.AccessToken, map[string]any{"phone": newPhone}); r.code != http.StatusAccepted {
		t.Fatalf("change start = %d %s", r.code, r.raw)
	}
	code := codeRE.FindStringSubmatch(f.sender.lastMessage(t, newPhone, sms.PurposeCustomerPhoneChange))[1]
	// A phone change code is no sign-in code, and Zeynep cannot use it.
	if r := f.do(t, http.MethodPost, "/v1/customer/auth/verify", "", map[string]any{"phone": newPhone, "code": code}); r.code != http.StatusBadRequest {
		t.Fatalf("change code as sign-in = %d", r.code)
	}
	z := f.signInAs(t, f.zeynepPhone, "z")
	if r := f.do(t, http.MethodPost, "/v1/customer/phone/verify", z.AccessToken, map[string]any{"phone": newPhone, "code": code}); r.code != http.StatusBadRequest {
		t.Fatalf("someone else's change code = %d", r.code)
	}
	r := f.do(t, http.MethodPost, "/v1/customer/phone/verify", s1.AccessToken, map[string]any{"phone": newPhone, "code": code})
	if r.code != http.StatusOK {
		t.Fatalf("change verify = %d %s", r.code, r.raw)
	}
	profile := decode[model.Profile](t, r)
	if profile.Phone != newPhone || len(profile.Businesses) != 2 || profile.Businesses[0].HistoryUntil == nil {
		t.Fatalf("profile after change = %+v", profile)
	}
	// This session continues on the new number; the other device is out.
	if r := f.do(t, http.MethodGet, "/v1/customer/me", s2.AccessToken, nil); r.code != http.StatusUnauthorized {
		t.Fatalf("other device after change = %d", r.code)
	}
	page := decode[struct {
		Items []model.ReservationItem `json:"items"`
	}](t, f.do(t, http.MethodGet, "/v1/customer/reservations", s1.AccessToken, nil))
	if len(page.Items) != 1 || page.Items[0].UUID != visit.UUID {
		t.Fatalf("history after change = %+v", page.Items)
	}

	// A booking made with the old number after the change is the next
	// holder's: Hüseyin does not see it.
	later := f.book(t, f.orgB.ID, f.branchB, f.cutB, f.ayseB, f.huseyinB, at(11, 0))
	f.exec("UPDATE reservations SET created_at = NOW() + INTERVAL '1 second' WHERE organization_id = $1 AND uuid = $2", f.orgB.ID, later.UUID)
	page = decode[struct {
		Items []model.ReservationItem `json:"items"`
	}](t, f.do(t, http.MethodGet, "/v1/customer/reservations", s1.AccessToken, nil))
	if len(page.Items) != 1 {
		t.Fatalf("Hüseyin sees the old number's later booking: %+v", page.Items)
	}

	// The next holder of the old number starts empty (A29): the records
	// proven by Hüseyin stay his.
	next := f.signInAs(t, old, "next")
	nextProfile := decode[model.Profile](t, f.do(t, http.MethodGet, "/v1/customer/me", next.AccessToken, nil))
	if len(nextProfile.Businesses) != 0 || strings.Contains(nextProfile.Phone, newPhone) {
		t.Fatalf("next holder profile = %+v", nextProfile)
	}
	if r := f.do(t, http.MethodGet, "/v1/customer/reservations/"+visit.UUID.String(), next.AccessToken, nil); r.code != http.StatusNotFound {
		t.Fatalf("next holder opens Hüseyin's visit = %d", r.code)
	}
	// An old record the next holder's number finds later opens only what
	// was booked after the release.
	var oldRecord uuid.UUID
	f.exec("UPDATE customers SET deleted_at = NOW() WHERE organization_id = $1 AND uuid = $2", f.orgB.ID, f.huseyinB)
	if err := f.pool.QueryRow(context.Background(), `INSERT INTO customers (organization_id, first_name, phone_e164, created_at)
		VALUES ($1, 'Eski', $2, NOW() - INTERVAL '1 day') RETURNING uuid`, f.orgB.ID, old).Scan(&oldRecord); err != nil {
		t.Fatal(err)
	}
	before := f.book(t, f.orgB.ID, f.branchB, f.cutB, f.ayseB, oldRecord, at(9, 0))
	f.exec("UPDATE reservations SET created_at = NOW() - INTERVAL '1 day' WHERE organization_id = $1 AND uuid = $2", f.orgB.ID, before.UUID)
	after := f.book(t, f.orgB.ID, f.branchB, f.cutB, f.ayseB, oldRecord, at(9, 30))
	f.exec("UPDATE reservations SET created_at = NOW() + INTERVAL '1 second' WHERE organization_id = $1 AND uuid = $2", f.orgB.ID, after.UUID)
	page = decode[struct {
		Items []model.ReservationItem `json:"items"`
	}](t, f.do(t, http.MethodGet, "/v1/customer/reservations", next.AccessToken, nil))
	if len(page.Items) != 1 || page.Items[0].UUID != after.UUID {
		t.Fatalf("next holder sees = %+v", page.Items)
	}
	nextProfile = decode[model.Profile](t, f.do(t, http.MethodGet, "/v1/customer/me", next.AccessToken, nil))
	if len(nextProfile.Businesses) != 1 || nextProfile.Businesses[0].HistoryFrom == nil || nextProfile.Businesses[0].FirstName != "" {
		t.Fatalf("next holder businesses = %+v", nextProfile.Businesses)
	}

	// Moving to a number with its own profile is refused: no merge.
	f.now = f.now.Add(2 * time.Minute)
	if r := f.do(t, http.MethodPost, "/v1/customer/phone/start", s1.AccessToken, map[string]any{"phone": f.zeynepPhone}); r.code != http.StatusAccepted {
		t.Fatalf("start to zeynep = %d", r.code)
	}
	zcode := codeRE.FindStringSubmatch(f.sender.lastMessage(t, f.zeynepPhone, sms.PurposeCustomerPhoneChange))[1]
	if r := f.do(t, http.MethodPost, "/v1/customer/phone/verify", s1.AccessToken, map[string]any{"phone": f.zeynepPhone, "code": zcode}); r.code != http.StatusConflict || r.errCode() != handler.CodePhoneInUse {
		t.Fatalf("change to a profile's number = %d %s", r.code, r.raw)
	}
	if r := f.do(t, http.MethodGet, "/v1/customer/me", z.AccessToken, nil); r.code != http.StatusOK || decode[model.Profile](t, r).Phone != f.zeynepPhone {
		t.Fatal("zeynep's profile changed")
	}
}

// TestCustomerPanelReassignedNumber (A29): the holder of a recycled number
// hides the previous holder's history, keeps what is theirs and ends the
// previous holder's sessions.
func TestCustomerPanelReassignedNumber(t *testing.T) {
	f := newFixture(t)
	previous := f.signInAs(t, f.huseyinPhone, "previous holder")
	visitA := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, at(10, 0))
	visitB := f.book(t, f.orgB.ID, f.branchB, f.cutB, f.ayseB, f.huseyinB, at(10, 0))
	f.exec("UPDATE reservations SET created_at = NOW() - INTERVAL '1 day' WHERE organization_id = ANY($1)", []int64{f.orgA.ID, f.orgB.ID})

	holder := f.signInAs(t, f.huseyinPhone, "new holder")
	if r := f.do(t, http.MethodPost, "/v1/customer/phone/reassigned", holder.AccessToken, map[string]any{"keep_businesses": []string{uuid.NewString()}}); r.code != http.StatusBadRequest || r.detail() != "keep_businesses/unknown" {
		t.Fatalf("unknown business = %d %s", r.code, r.raw)
	}
	r := f.do(t, http.MethodPost, "/v1/customer/phone/reassigned", holder.AccessToken, map[string]any{"keep_businesses": []string{f.orgB.UUID.String()}})
	if r.code != http.StatusOK {
		t.Fatalf("reassigned = %d %s", r.code, r.raw)
	}
	for _, b := range decode[model.Profile](t, r).Businesses {
		switch b.UUID {
		case f.orgA.UUID:
			if b.HistoryFrom == nil || b.FirstName != "" || b.ReservationCount != 0 {
				t.Fatalf("hidden business = %+v", b)
			}
		case f.orgB.UUID:
			if b.HistoryFrom != nil || b.ReservationCount != 1 {
				t.Fatalf("kept business = %+v", b)
			}
		}
	}
	if r := f.do(t, http.MethodGet, "/v1/customer/me", previous.AccessToken, nil); r.code != http.StatusUnauthorized {
		t.Fatalf("previous holder's session = %d", r.code)
	}
	if r := f.do(t, http.MethodGet, "/v1/customer/reservations/"+visitA.UUID.String(), holder.AccessToken, nil); r.code != http.StatusNotFound {
		t.Fatalf("hidden visit = %d", r.code)
	}
	if r := f.do(t, http.MethodGet, "/v1/customer/reservations/"+visitB.UUID.String(), holder.AccessToken, nil); r.code != http.StatusOK {
		t.Fatalf("kept visit = %d", r.code)
	}
	// A new booking at the hidden business is the holder's.
	fresh := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, at(11, 0))
	f.exec("UPDATE reservations SET created_at = NOW() + INTERVAL '1 second' WHERE organization_id = $1 AND uuid = $2", f.orgA.ID, fresh.UUID)
	if r := f.do(t, http.MethodGet, "/v1/customer/reservations/"+fresh.UUID.String(), holder.AccessToken, nil); r.code != http.StatusOK {
		t.Fatalf("fresh visit = %d", r.code)
	}
	if n := f.count("SELECT COUNT(*) FROM customer_auth_events WHERE phone_e164 = $1 AND kind = 'reassigned'", f.huseyinPhone); n != 1 {
		t.Fatalf("reassigned events = %d", n)
	}
}

// TestCustomerPanelAbuseLimits: a phone gets at most ten codes a day; an
// address asking codes for many phones gets no more codes.
func TestCustomerPanelAbuseLimits(t *testing.T) {
	f := newFixture(t)
	cfg := usecase.DefaultConfig
	cfg.IPPhonesPerHour = 2
	f.svc.SetConfig(cfg)
	for i := 0; i < 12; i++ {
		f.now = f.now.Add(13 * time.Minute)
		f.do(t, http.MethodPost, "/v1/customer/auth/start", "", map[string]any{"phone": f.huseyinPhone})
	}
	if n := f.sender.count(); n != 10 {
		t.Fatalf("codes in a day = %d", n)
	}
	// From one address: the third distinct phone gets no message.
	f.now = f.now.Add(25 * time.Hour)
	before := f.sender.count()
	start := func(phone string) {
		req := httptest.NewRequest(http.MethodPost, "/v1/customer/auth/start", strings.NewReader(`{"phone":"`+phone+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.7:4000"
		rec := httptest.NewRecorder()
		f.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("start = %d", rec.Code)
		}
	}
	start("+905550000001")
	start("+905550000002")
	start(f.zeynepPhone)
	if f.sender.count() != before {
		t.Fatalf("sent after the address cap: %d", f.sender.count()-before)
	}
	if n := f.count("SELECT COUNT(*) FROM customer_auth_events WHERE kind = 'ip_throttled' AND ip = '203.0.113.7'"); n != 1 {
		t.Fatalf("throttle events = %d", n)
	}
	f.exec("DELETE FROM customer_otp_challenges WHERE ip = '203.0.113.7'")
	f.exec("DELETE FROM customer_auth_events WHERE ip = '203.0.113.7'")
}
