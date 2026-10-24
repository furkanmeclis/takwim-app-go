package customerpanel_test

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/whatsapp"
	whatsapphandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/whatsapp/handler"
	whatsappusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/whatsapp/meta"
)

// graph is a fake WhatsApp Cloud API: it records the messages and answers
// with numbered message ids.
type graph struct {
	mu   sync.Mutex
	sent []map[string]any
}

func (g *graph) handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		_, _ = w.Write([]byte(`{"display_phone_number":"+90 850 000 00 00","id":"1234567890"}`))
		return
	}
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	g.mu.Lock()
	g.sent = append(g.sent, body)
	n := len(g.sent)
	g.mu.Unlock()
	_, _ = fmt.Fprintf(w, `{"messaging_product":"whatsapp","messages":[{"id":"wamid.OUT%d"}]}`, n)
}

func (g *graph) last(typ string) (map[string]any, string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for i := len(g.sent) - 1; i >= 0; i-- {
		if g.sent[i]["type"] == typ {
			return g.sent[i], fmt.Sprintf("wamid.OUT%d", i+1)
		}
	}
	return nil, ""
}

func (g *graph) count(typ string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := 0
	for _, m := range g.sent {
		if m["type"] == typ {
			n++
		}
	}
	return n
}

func (c *capture) notices(phone string) []sms.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []sms.Message
	for _, m := range c.msgs {
		// Confirmation requests only (the booking notices go out too).
		if m.To == phone && m.Purpose == sms.PurposeCustomerNotice && strings.Contains(m.Text, "teyit") {
			out = append(out, m)
		}
	}
	return out
}

// TestWhatsAppConfirmation (TEC-922): the confirmation request goes out as
// the business's WhatsApp template with three buttons; delivery and read
// reports are receipt times, never a confirmation; only the customer's own
// button press on the current request answers; a failed or unreported
// message falls back to SMS; a repeated webhook is applied once.
func TestWhatsAppConfirmation(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	f.exec(`UPDATE organizations SET plan = 'enterprise', addons = '{confirmation}' WHERE id = $1`, f.orgA.ID)
	// WhatsApp first, SMS when it fails; one request 24 hours before.
	f.exec(`INSERT INTO confirmation_settings (organization_id, enabled, channels, fallback, first_request_minutes,
		follow_up_minutes, max_attempts, deadline_minutes, window_start_minute, window_end_minute, on_no_answer)
		VALUES ($1, true, '{whatsapp,sms}', true, 1440, 0, 1, 120, 540, 1260, 'cancel')`, f.orgA.ID)

	g := &graph{}
	srv := httptest.NewServer(http.HandlerFunc(g.handler))
	t.Cleanup(srv.Close)
	client := meta.New(srv.URL, "", nil)
	cipher, err := crypto.NewFieldCipher(strings.Repeat("k", 32))
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	kit, err := integrationsmodule.New(f.q, cipher, "production", log, whatsapp.Adapters(client)...)
	if err != nil {
		t.Fatal(err)
	}
	acc, err := kit.Accounts.Connect(ctx, f.orgA.ID, accountsusecase.ConnectInput{
		Kind: integrations.KindWhatsApp, Provider: meta.ProviderCode, DisplayName: "WhatsApp", Mode: integrations.ModeLive,
		Credentials: map[string]string{
			meta.FieldPhoneNumberID: "1234567890", meta.FieldAccessToken: "token-1", meta.FieldAppSecret: "app-secret",
			meta.FieldVerifyToken: "verify-me",
		},
	})
	if err != nil {
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
	wa := whatsappusecase.New(f.q, client)
	wa.SetAccounts(kit.Accounts, kit.Runner)
	wa.SetAnswers(f.svc, confirm)
	wa.SetLogger(log)
	notify.Register(customernotifyadapters.SMS{Sender: f.sender}, whatsappusecase.NewChannel(wa))
	mux := http.NewServeMux()
	whatsapp.RegisterRoutes(mux, whatsapphandler.New(wa))
	hook := "/v1/public/whatsapp/meta/" + acc.UUID.String() + "/webhook"
	post := func(body string, secret string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, hook, strings.NewReader(body))
		req.Header.Set("X-Hub-Signature-256", meta.Sign(secret, []byte(body)))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}
	wire := func(statuses, messages string) string {
		return `{"object":"whatsapp_business_account","entry":[{"id":"1","changes":[{"field":"messages","value":{
			"messaging_product":"whatsapp","metadata":{"phone_number_id":"1234567890"},
			"statuses":[` + statuses + `],"messages":[` + messages + `]}}]}]}`
	}
	status := func(id, st string) string {
		return fmt.Sprintf(`{"id":%q,"status":%q,"timestamp":"%d","recipient_id":"x"}`, id, st, f.now.Unix())
	}
	button := func(in, from, ctxID, payload string) string {
		return fmt.Sprintf(`{"from":%q,"id":%q,"timestamp":"%d","type":"button","context":{"id":%q},"button":{"payload":%q,"text":"x"}}`,
			from, in, f.now.Unix(), ctxID, payload)
	}
	confirmation := func(id fmt.Stringer) string {
		var s string
		if err := f.pool.QueryRow(ctx, `SELECT confirmation_status FROM reservations WHERE organization_id = $1 AND uuid = $2`,
			f.orgA.ID, id.String()).Scan(&s); err != nil {
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
	phone := strings.TrimPrefix(f.huseyinPhone, "+")

	// The handshake answers only the account's own verify token.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, hook+"?hub.mode=subscribe&hub.verify_token=verify-me&hub.challenge=42", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "42" {
		t.Fatalf("challenge = %d %q", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, hook+"?hub.mode=subscribe&hub.verify_token=nope&hub.challenge=42", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong verify token = %d", rec.Code)
	}

	// 1. Sent as the template with the three buttons.
	f.now = at(8, 0).Add(26 * time.Hour) // Tuesday 10:00
	start := at(10, 0).AddDate(0, 0, 7)  // next Monday 10:00
	first := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, start)
	f.now = start.Add(-23 * time.Hour)
	run()
	tpl, out1 := g.last("template")
	if tpl == nil || tpl["to"] != phone {
		t.Fatalf("template = %v", tpl)
	}
	comps := tpl["template"].(map[string]any)["components"].([]any)
	params := comps[0].(map[string]any)["parameters"].([]any)
	if len(comps) != 4 || params[0].(map[string]any)["text"] != "Hüseyin K" || params[2].(map[string]any)["text"] != "14.01.2030 10:00" ||
		params[3].(map[string]any)["text"] != "14.01.2030 08:00" {
		t.Fatalf("components = %v", comps)
	}
	if p := comps[1].(map[string]any)["parameters"].([]any)[0].(map[string]any)["payload"]; p != "tkw1:confirm" {
		t.Fatalf("first button = %v", p)
	}
	if n := len(f.sender.notices(f.huseyinPhone)); n != 0 {
		t.Fatalf("SMS sent while WhatsApp waits for its report: %d", n)
	}
	if n := f.count(`SELECT count(*) FROM customer_notification_deliveries WHERE organization_id = $1 AND channel = 'whatsapp'
		AND status = 'sent' AND provider_ref = $2 AND sent_at IS NOT NULL AND finished_at IS NULL`, f.orgA.ID, out1); n != 1 {
		t.Fatal("accepted attempt not recorded")
	}

	// A wrong signature changes nothing.
	if code := post(wire(status(out1, "delivered"), ""), "other-secret"); code != http.StatusUnauthorized {
		t.Fatalf("bad signature = %d", code)
	}
	// Delivered and read are receipts, never a confirmation.
	if code := post(wire(status(out1, "delivered")+","+status(out1, "read"), ""), "app-secret"); code != http.StatusOK {
		t.Fatalf("statuses = %d", code)
	}
	if n := f.count(`SELECT count(*) FROM customer_notification_deliveries WHERE organization_id = $1 AND provider_ref = $2
		AND status = 'delivered' AND delivered_at IS NOT NULL AND read_at IS NOT NULL`, f.orgA.ID, out1); n != 1 {
		t.Fatal("receipts not recorded")
	}
	if s := confirmation(first.UUID); s != reservationsmodel.ConfirmationPending {
		t.Fatalf("after delivered / read = %s", s)
	}
	// Another number's press and a free text answer nothing.
	if code := post(wire("", button("wamid.IN1", "905550000000", out1, "tkw1:confirm")), "app-secret"); code != http.StatusOK {
		t.Fatalf("other number = %d", code)
	}
	text := fmt.Sprintf(`{"from":%q,"id":"wamid.IN2","timestamp":"%d","type":"text","context":{"id":%q},"text":{"body":"evet"}}`,
		phone, f.now.Unix(), out1)
	if code := post(wire("", text), "app-secret"); code != http.StatusOK {
		t.Fatalf("text = %d", code)
	}
	if s := confirmation(first.UUID); s != reservationsmodel.ConfirmationPending {
		t.Fatalf("after other number / text = %s", s)
	}
	// "Geleceğim" from the customer's number confirms, once.
	press := wire("", button("wamid.IN3", phone, out1, "tkw1:confirm"))
	if code := post(press, "app-secret"); code != http.StatusOK {
		t.Fatalf("confirm = %d", code)
	}
	if code := post(press, "app-secret"); code != http.StatusOK {
		t.Fatalf("repeated webhook = %d", code)
	}
	if s := confirmation(first.UUID); s != reservationsmodel.ConfirmationConfirmed {
		t.Fatalf("after the button = %s", s)
	}
	if n := f.count(`SELECT count(*) FROM reservation_events e JOIN reservations r ON r.id = e.reservation_id
		WHERE r.uuid = $1 AND e.type = 'confirmation_confirmed' AND e.note = 'whatsapp'`, first.UUID); n != 1 {
		t.Fatalf("confirmed events = %d", n)
	}
	if ack, _ := g.last("text"); ack == nil || !strings.Contains(ack["text"].(map[string]any)["body"].(string), "teyit edildi") {
		t.Fatalf("ack = %v", ack)
	}
	attempts, err := confirm.Attempts(ctx, f.orgA.ID, first.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].DeliveredAt == nil || attempts[0].ReadAt == nil || len(attempts[0].Responses) != 3 {
		t.Fatalf("attempts = %+v", attempts)
	}
	got := map[string]string{}
	for _, r := range attempts[0].Responses {
		got[r.Answer+"/"+r.Result] = r.Reason
	}
	if got["confirm/ignored"] != whatsappusecase.ReasonOtherNumber || got["unknown/ignored"] != whatsappusecase.ReasonNotAnswer ||
		got["confirm/applied"] != "" {
		t.Fatalf("responses = %v", got)
	}

	// 2. A failed message runs SMS (with the visit link); a press on an
	// older request does nothing; İptal cancels under the cancel rules.
	f.now = at(8, 0).Add(26 * time.Hour)
	second := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, start.Add(time.Hour))
	f.now = start.Add(-23 * time.Hour)
	run()
	_, out2 := g.last("template")
	if out2 == out1 {
		t.Fatal("no second template")
	}
	if code := post(wire(`{"id":"`+out2+`","status":"failed","timestamp":"1","errors":[{"code":131026,"title":"Message undeliverable"}]}`, ""),
		"app-secret"); code != http.StatusOK {
		t.Fatalf("failed status = %d", code)
	}
	run()
	notices := f.sender.notices(f.huseyinPhone)
	if len(notices) != 1 || !strings.Contains(notices[0].Text, "/panel/v#") {
		t.Fatalf("SMS fallback = %+v", notices)
	}
	f.exec(`UPDATE reservation_confirmations SET round = round + 1 WHERE organization_id = $1
		AND reservation_id = (SELECT id FROM reservations WHERE uuid = $2)`, f.orgA.ID, second.UUID)
	if code := post(wire("", button("wamid.IN4", phone, out2, "tkw1:cancel")), "app-secret"); code != http.StatusOK {
		t.Fatalf("outdated = %d", code)
	}
	if n := f.count(`SELECT count(*) FROM reservations WHERE uuid = $1 AND status = 'active'`, second.UUID); n != 1 {
		t.Fatal("an older request cancelled the visit")
	}
	if n := f.count(`SELECT count(*) FROM customer_notification_responses WHERE organization_id = $1 AND provider_ref = 'wamid.IN4'
		AND result = 'rejected' AND reason = $2`, f.orgA.ID, whatsappusecase.ReasonOutdated); n != 1 {
		t.Fatal("outdated reply not recorded")
	}
	f.exec(`UPDATE reservation_confirmations SET round = round - 1 WHERE organization_id = $1
		AND reservation_id = (SELECT id FROM reservations WHERE uuid = $2)`, f.orgA.ID, second.UUID)
	if code := post(wire("", button("wamid.IN5", phone, out2, "tkw1:cancel")), "app-secret"); code != http.StatusOK {
		t.Fatalf("cancel = %d", code)
	}
	if n := f.count(`SELECT count(*) FROM reservations WHERE uuid = $1 AND status = 'cancelled' AND confirmation_status = 'declined'`,
		second.UUID); n != 1 {
		t.Fatal("İptal did not cancel")
	}

	// 3. No delivery report within the timeout: SMS runs.
	f.now = at(8, 0).Add(26 * time.Hour)
	third := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, start.Add(90*time.Minute))
	f.now = start.Add(90*time.Minute - 23*time.Hour)
	run()
	before := len(f.sender.notices(f.huseyinPhone))
	f.now = f.now.Add(customernotifyusecase.DefaultReceiptTimeout + time.Minute)
	if n, err := notify.ExpireReceipts(ctx); err != nil || n != 1 {
		t.Fatalf("expire = %d %v", n, err)
	}
	if len(f.sender.notices(f.huseyinPhone)) != before+1 {
		t.Fatal("no SMS after the receipt timeout")
	}
	if s := confirmation(third.UUID); s != reservationsmodel.ConfirmationPending {
		t.Fatalf("third = %s", s)
	}
	if g.count("template") != 3 {
		t.Fatalf("templates = %d", g.count("template"))
	}
}
