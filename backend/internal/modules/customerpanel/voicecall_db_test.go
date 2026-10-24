package customerpanel_test

import (
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	confirmationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/confirmations/usecase"
	customernotifyadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/adapters"
	customernotifycatalog "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/catalog"
	customernotifyusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/usecase"
	integrationsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/voicecall"
	voicecallhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/voicecall/handler"
	voicecallusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/voicecall/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/voice/netgsm"
)

// switchboard is a fake Netgsm voice API: it records the call orders and
// answers with numbered order ids.
type switchboard struct {
	mu     sync.Mutex
	orders []string
}

func (s *switchboard) handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		_, _ = io.WriteString(w, "60")
		return
	}
	raw, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.orders = append(s.orders, string(raw))
	n := len(s.orders)
	s.mu.Unlock()
	_, _ = fmt.Fprintf(w, "00 9000%d", n)
}

func (s *switchboard) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.orders)
}

var orderURL = regexp.MustCompile(`<url>([^<]*)</url>`)

// last answers the last order, its id and its webhook address.
func (s *switchboard) last(t *testing.T) (string, string, string) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.orders) == 0 {
		t.Fatal("no call ordered")
	}
	body := s.orders[len(s.orders)-1]
	m := orderURL.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("order without a webhook: %s", body)
	}
	return body, fmt.Sprintf("9000%d", len(s.orders)), html.UnescapeString(m[1])
}

// TestVoiceCallConfirmation (TEC-923): the confirmation call reads out the
// visit with keys 1 / 2 / 3; the picked up call is never a confirmation;
// key 1 confirms, 2 cancels under the cancel rules, 3 asks for a change; a
// call not answered or without a valid key runs the next channel and is
// made again within the limit; calls ring only inside the call hours; the
// webhook takes only the address it gave out, and a repeated result acts
// once.
func TestVoiceCallConfirmation(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	f.exec(`UPDATE organizations SET plan = 'enterprise', addons = '{confirmation}' WHERE id = $1`, f.orgA.ID)
	// The call first, SMS when it brings no answer; one request 24 hours
	// before; calls in the request hours; one more call 30 minutes later.
	f.exec(`INSERT INTO confirmation_settings (organization_id, enabled, channels, fallback, first_request_minutes,
		follow_up_minutes, max_attempts, deadline_minutes, window_start_minute, window_end_minute, on_no_answer,
		call_retries, call_retry_minutes)
		VALUES ($1, true, '{voice_call,sms}', true, 1440, 0, 1, 120, 540, 1260, 'cancel', 1, 30)`, f.orgA.ID)

	sb := &switchboard{}
	srv := httptest.NewServer(http.HandlerFunc(sb.handler))
	t.Cleanup(srv.Close)
	client := netgsm.New(srv.URL, nil)
	cipher, err := crypto.NewFieldCipher(strings.Repeat("k", 32))
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	kit, err := integrationsmodule.New(f.q, cipher, "production", log, voicecall.Adapters(client)...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kit.Accounts.Connect(ctx, f.orgA.ID, accountsusecase.ConnectInput{
		Kind: integrations.KindVoiceCall, Provider: netgsm.ProviderCode, DisplayName: "Netgsm", Mode: integrations.ModeLive,
		Credentials: map[string]string{netgsm.FieldUserCode: "8508500000", netgsm.FieldPassword: "secret"},
	}); err != nil {
		t.Fatal(err)
	}

	clock := func() time.Time { return f.now }
	notify := customernotifyusecase.New(f.pool, f.q, "https://takwim.link/panel")
	notify.SetClock(clock)
	notify.SetLogger(log)
	confirm := confirmationsusecase.New(f.pool, f.q, notify)
	confirm.SetClock(clock)
	notify.SetDispatchGuard(confirm.Guard)
	f.reservations.SetNotifier(reservationsusecase.Notifiers(notify, confirm))
	confirm.SetReservations(f.reservations)
	f.svc.SetConfirmations(confirm)
	notify.SetLinkIssuerFor(customernotifycatalog.ConfirmationRequest, customernotifyadapters.VisitLinks(f.svc))
	const base = "https://app.test/api"
	voice := voicecallusecase.New(f.q, client, base)
	voice.SetAccounts(kit.Accounts, kit.Runner, cipher)
	voice.SetAnswers(f.svc, confirm)
	voice.SetLogger(log)
	voice.SetClock(clock)
	notify.Register(customernotifyadapters.SMS{Sender: f.sender}, voicecallusecase.NewChannel(voice))
	mux := http.NewServeMux()
	voicecall.RegisterRoutes(mux, voicecallhandler.New(voice))

	post := func(hook, body string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, strings.TrimPrefix(hook, base), strings.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}
	phone := netgsm.Recipient(f.huseyinPhone)
	result := func(bulk string, state int, key string) string {
		answered := f.now.In(netgsm.Location).Format("2006-01-02 15:04:05.000")
		return fmt.Sprintf(`{"bulkid":%q,"caller":"8508500000","callee":%q,"state":%d,"type":3,"answer_time":%q,
			"hangup_time":%q,"detail":{"push_button":%s}}`, bulk, phone, state, answered, answered, key)
	}
	confirmation := func(id fmt.Stringer) string {
		var s string
		if err := f.pool.QueryRow(ctx, `SELECT confirmation_status FROM reservations WHERE organization_id = $1 AND uuid = $2`,
			f.orgA.ID, id.String()).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	task := func(id fmt.Stringer, cols string) string {
		var s string
		if err := f.pool.QueryRow(ctx, `SELECT concat_ws('|', `+cols+`) FROM reservation_confirmations c
			JOIN reservations r ON r.organization_id = c.organization_id AND r.id = c.reservation_id
			WHERE c.organization_id = $1 AND r.uuid = $2`, f.orgA.ID, id.String()).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	run := func() {
		t.Helper()
		if _, err := confirm.RunDueOrganization(ctx, f.orgA.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := notify.ProcessDueOrganization(ctx, f.orgA.ID); err != nil {
			t.Fatal(err)
		}
	}
	sms := func() int { return len(f.sender.notices(f.huseyinPhone)) }

	// 1. The call reads out the visit and takes 1 / 2 / 3; key 1 confirms.
	f.now = at(8, 0).Add(26 * time.Hour) // Tuesday 10:00
	start := at(10, 0).AddDate(0, 0, 7)  // next Monday 10:00
	first := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, start)
	f.now = start.Add(-23 * time.Hour) // Sunday 11:00
	run()
	order, bulk1, hook1 := sb.last(t)
	for _, want := range []string{"<no>" + phone + "</no>", "14.01.2030 tarihli, saat 10:00", "<keyinfo>1</keyinfo>",
		"<keyinfo>3</keyinfo>", "<key>1</key>"} {
		if !strings.Contains(order, want) {
			t.Fatalf("order lacks %q: %s", want, order)
		}
	}
	if n := sms(); n != 0 {
		t.Fatalf("SMS sent while the call waits for its result: %d", n)
	}
	if n := f.count(`SELECT count(*) FROM customer_notification_deliveries WHERE organization_id = $1 AND channel = 'voice_call'
		AND status = 'sent' AND provider_ref = $2 AND finished_at IS NULL`, f.orgA.ID, bulk1); n != 1 {
		t.Fatal("ordered call not recorded")
	}
	// Only the address we gave out is taken.
	forged := strings.Replace(hook1, "&s=", "&s=0", 1)
	if code := post(forged, result(bulk1, netgsm.StateAnswered, `"1"`)); code != http.StatusUnauthorized {
		t.Fatalf("forged token = %d", code)
	}
	if s := confirmation(first.UUID); s != reservationsmodel.ConfirmationPending {
		t.Fatalf("after the forged result = %s", s)
	}
	press := result(bulk1, netgsm.StateAnswered, `"1"`)
	if code := post(hook1, press); code != http.StatusOK {
		t.Fatalf("key 1 = %d", code)
	}
	if code := post(hook1, press); code != http.StatusOK {
		t.Fatalf("repeated result = %d", code)
	}
	if s := confirmation(first.UUID); s != reservationsmodel.ConfirmationConfirmed {
		t.Fatalf("after key 1 = %s", s)
	}
	if n := f.count(`SELECT count(*) FROM reservation_events e JOIN reservations r ON r.id = e.reservation_id
		WHERE r.uuid = $1 AND e.type = 'confirmation_confirmed' AND e.note = 'voice_call'`, first.UUID); n != 1 {
		t.Fatalf("confirmed events = %d", n)
	}
	if n := f.count(`SELECT count(*) FROM customer_notification_deliveries WHERE organization_id = $1 AND provider_ref = $2
		AND status = 'delivered' AND delivered_at IS NOT NULL`, f.orgA.ID, bulk1); n != 1 {
		t.Fatal("answered call not recorded as delivered")
	}
	run()
	if n := sms(); n != 0 {
		t.Fatalf("SMS after a confirmed call: %d", n)
	}

	// 2. Picked up without a key: no confirmation, SMS runs, the call is
	// made again 30 minutes later; key 2 on it cancels.
	f.now = at(8, 0).Add(26 * time.Hour)
	second := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, start.Add(30*time.Minute))
	f.now = start.Add(-23 * time.Hour)
	run()
	_, bulk2, hook2 := sb.last(t)
	if code := post(hook2, result(bulk2, netgsm.StateAnswered, "null")); code != http.StatusOK {
		t.Fatalf("no key = %d", code)
	}
	if s := confirmation(second.UUID); s != reservationsmodel.ConfirmationPending {
		t.Fatalf("a picked up call confirmed: %s", s)
	}
	if n := f.count(`SELECT count(*) FROM customer_notification_deliveries WHERE organization_id = $1 AND provider_ref = $2
		AND status = 'delivered' AND delivered_at IS NOT NULL AND error = $3`, f.orgA.ID, bulk2, voicecallusecase.ErrNoKey); n != 1 {
		t.Fatal("reached call without a key not recorded")
	}
	run()
	if n := sms(); n != 1 {
		t.Fatalf("SMS fallback after no key = %d", n)
	}
	if got, want := task(second.UUID, "calls_made, next_call_at IS NOT NULL"), "0|t"; got != want {
		t.Fatalf("call again = %s", got)
	}
	f.now = f.now.Add(29 * time.Minute)
	run()
	if sb.count() != 2 {
		t.Fatalf("called again too early: %d", sb.count())
	}
	f.now = f.now.Add(2 * time.Minute)
	run()
	if sb.count() != 3 {
		t.Fatalf("no second call: %d", sb.count())
	}
	order, bulk3, hook3 := sb.last(t)
	if !strings.Contains(order, "<no>"+phone+"</no>") || sms() != 1 {
		t.Fatalf("second call = %s, SMS %d", order, sms())
	}
	if got := task(second.UUID, "calls_made, next_call_at IS NULL"); got != "1|t" {
		t.Fatalf("after the second call = %s", got)
	}
	attempts, err := confirm.Attempts(ctx, f.orgA.ID, second.UUID)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	for _, a := range attempts {
		if a.Call == 1 && a.Request == 1 && a.Channel != nil && *a.Channel == "voice_call" {
			calls++
		}
	}
	if calls != 1 {
		t.Fatalf("attempts = %+v", attempts)
	}
	if code := post(hook3, result(bulk3, netgsm.StateAnswered, "2")); code != http.StatusOK {
		t.Fatalf("key 2 = %d", code)
	}
	if n := f.count(`SELECT count(*) FROM reservations WHERE uuid = $1 AND status = 'cancelled' AND confirmation_status = 'declined'`,
		second.UUID); n != 1 {
		t.Fatal("key 2 did not cancel")
	}

	// 3. Not answered: SMS runs and the call is made again; key 3 asks for
	// a change (the visit stays).
	f.now = at(8, 0).Add(26 * time.Hour)
	third := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, start.Add(time.Hour))
	f.now = start.Add(-23 * time.Hour)
	run()
	_, bulk4, hook4 := sb.last(t)
	if code := post(hook4, result(bulk4, netgsm.StateNotAnswered, "null")); code != http.StatusOK {
		t.Fatalf("not answered = %d", code)
	}
	if n := f.count(`SELECT count(*) FROM customer_notification_deliveries WHERE organization_id = $1 AND provider_ref = $2
		AND status = 'failed' AND delivered_at IS NULL AND error = $3`, f.orgA.ID, bulk4, voicecallusecase.ErrNotAnswered); n != 1 {
		t.Fatal("unanswered call not recorded as failed")
	}
	run()
	if n := sms(); n != 2 {
		t.Fatalf("SMS fallback after no answer = %d", n)
	}
	f.now = f.now.Add(31 * time.Minute)
	run()
	_, bulk5, hook5 := sb.last(t)
	if bulk5 == bulk4 {
		t.Fatal("not called again")
	}
	if code := post(hook5, result(bulk5, netgsm.StateAnswered, "3")); code != http.StatusOK {
		t.Fatalf("key 3 = %d", code)
	}
	if got := task(third.UUID, "c.status, c.change_requested_at IS NOT NULL"); got != "pending|t" {
		t.Fatalf("after key 3 = %s", got)
	}
	if n := f.count(`SELECT count(*) FROM reservations WHERE uuid = $1 AND status = 'active'`, third.UUID); n != 1 {
		t.Fatal("key 3 changed the visit")
	}

	// 4. Another key is recorded, never applied; the last extra call used,
	// no more calls.
	f.now = at(8, 0).Add(26 * time.Hour)
	fourth := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, start.Add(90*time.Minute))
	f.now = start.Add(90*time.Minute - 23*time.Hour)
	f.exec(`UPDATE reservation_confirmations SET calls_made = call_retries WHERE organization_id = $1
		AND reservation_id = (SELECT id FROM reservations WHERE uuid = $2)`, f.orgA.ID, fourth.UUID)
	run()
	_, bulk6, hook6 := sb.last(t)
	// A result of another order on this attempt's address changes nothing.
	if code := post(hook6, result("123", netgsm.StateAnswered, "1")); code != http.StatusOK {
		t.Fatalf("other order = %d", code)
	}
	if code := post(hook6, result(bulk6, netgsm.StateAnswered, "7")); code != http.StatusOK {
		t.Fatalf("key 7 = %d", code)
	}
	if s := confirmation(fourth.UUID); s != reservationsmodel.ConfirmationPending {
		t.Fatalf("after key 7 = %s", s)
	}
	if n := f.count(`SELECT count(*) FROM customer_notification_responses WHERE organization_id = $1 AND provider_ref = $2
		AND channel = 'voice_call' AND answer = 'unknown' AND result = 'ignored' AND reason = $3`,
		f.orgA.ID, bulk6, voicecallusecase.ReasonInvalidKey); n != 1 {
		t.Fatal("invalid key not recorded")
	}
	if got := task(fourth.UUID, "next_call_at IS NULL"); got != "t" {
		t.Fatalf("called again past the limit: %s", got)
	}

	// 5. Outside the call hours the request goes out on SMS alone.
	f.now = at(8, 0).Add(26 * time.Hour)
	fifth := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, start.Add(-time.Hour))
	f.exec(`UPDATE reservation_confirmations SET call_window_start_minute = 720, call_window_end_minute = 780
		WHERE organization_id = $1 AND reservation_id = (SELECT id FROM reservations WHERE uuid = $2)`, f.orgA.ID, fifth.UUID)
	f.now = start.Add(-24 * time.Hour) // Sunday 10:00
	before, smsBefore := sb.count(), sms()
	run()
	if sb.count() != before || sms() != smsBefore+1 {
		t.Fatalf("outside the call hours: calls %d → %d, SMS %d → %d", before, sb.count(), smsBefore, sms())
	}
}
