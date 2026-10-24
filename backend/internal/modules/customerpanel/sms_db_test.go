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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/smsprovider"
	smsproviderusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/smsprovider/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms/netgsm"
)

// netgsmFake is a fake Netgsm SMS API: it records the texts, answers with
// numbered job ids and serves the delivery reports the test sets.
type netgsmFake struct {
	mu      sync.Mutex
	sent    []map[string]any
	reports map[string]string // job id → report row (JSON)
	calls   int
}

func (n *netgsmFake) handler(w http.ResponseWriter, r *http.Request) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if u, pw, ok := r.BasicAuth(); !ok || u != "8508500000" || pw != "api-pass" {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"code":"30"}`)
		return
	}
	raw, _ := io.ReadAll(r.Body)
	switch r.URL.Path {
	case "/sms/rest/v2/msgheader":
		_, _ = io.WriteString(w, `{"code":"00","msgheader":["TAKWIM"]}`)
	case "/sms/rest/v2/send":
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		n.sent = append(n.sent, body)
		_, _ = fmt.Fprintf(w, `{"code":"00","jobid":"JOB%d","description":"ok"}`, len(n.sent))
	case "/sms/rest/v2/report":
		n.calls++
		var body struct {
			JobIDs []string `json:"jobids"`
		}
		_ = json.Unmarshal(raw, &body)
		rows := []string{}
		for _, id := range body.JobIDs {
			if row, ok := n.reports[id]; ok {
				rows = append(rows, row)
			}
		}
		if len(rows) == 0 {
			_, _ = io.WriteString(w, `{"code":"60","description":"no records"}`)
			return
		}
		_, _ = io.WriteString(w, `{"code":"00","response":{"jobs":[`+strings.Join(rows, ",")+`]}}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (n *netgsmFake) report(job, status, deliveredAt string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.reports == nil {
		n.reports = map[string]string{}
	}
	n.reports[job] = fmt.Sprintf(`{"jobid":%q,"number":"5321112233","status":%q,"deliveredDate":%q}`, job, status, deliveredAt)
}

func text(body map[string]any) string {
	msgs, _ := body["messages"].([]any)
	if len(msgs) == 0 {
		return ""
	}
	m, _ := msgs[0].(map[string]any)
	s, _ := m["msg"].(string)
	return s
}

// last is the last confirmation request sent and its job id (the booking
// notices go out as texts too).
func (n *netgsmFake) last() (map[string]any, string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for i := len(n.sent) - 1; i >= 0; i-- {
		if strings.Contains(text(n.sent[i]), "teyit") {
			return n.sent[i], fmt.Sprintf("JOB%d", i+1)
		}
	}
	return nil, ""
}

// count is how many confirmation requests were sent.
func (n *netgsmFake) count() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	c := 0
	for _, s := range n.sent {
		if strings.Contains(text(s), "teyit") {
			c++
		}
	}
	return c
}

// TestNetgsmSMSConfirmation (TEC-982): the confirmation request goes out
// as a text through the business's own Netgsm account and stays open
// until the provider's report; a delivered report is a receipt time, never
// a confirmation; a failed or unreported message falls back to the next
// channel; a business without an account is not ready and nothing is
// recorded as sent for it.
func TestNetgsmSMSConfirmation(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	f.exec(`UPDATE organizations SET plan = 'enterprise', addons = '{confirmation}' WHERE id = ANY($1)`, []int64{f.orgA.ID, f.orgB.ID})
	// SMS first, e-mail when it fails; one request 24 hours before.
	f.exec(`INSERT INTO confirmation_settings (organization_id, enabled, channels, fallback, first_request_minutes,
		follow_up_minutes, max_attempts, deadline_minutes, window_start_minute, window_end_minute, on_no_answer)
		VALUES ($1, true, '{sms,email}', true, 1440, 0, 1, 120, 540, 1260, 'cancel')`, f.orgA.ID)

	fake := &netgsmFake{}
	srv := httptest.NewServer(http.HandlerFunc(fake.handler))
	t.Cleanup(srv.Close)
	client := netgsm.New(srv.URL, nil)
	cipher, err := crypto.NewFieldCipher(strings.Repeat("k", 32))
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	kit, err := integrationsmodule.New(f.q, cipher, "production", log, smsprovider.Adapters(client)...)
	if err != nil {
		t.Fatal(err)
	}
	acc, err := kit.Accounts.Connect(ctx, f.orgA.ID, accountsusecase.ConnectInput{
		Provider: netgsm.ProviderCode, DisplayName: "Netgsm SMS", Mode: integrations.ModeLive,
		Credentials: map[string]string{netgsm.FieldUserCode: "8508500000", netgsm.FieldPassword: "api-pass", netgsm.FieldHeader: "TAKWIM"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if acc.Kind != integrations.KindSMS {
		t.Fatalf("kind = %s", acc.Kind)
	}
	if _, err := kit.Accounts.Test(ctx, f.orgA.ID, acc.UUID); err != nil {
		t.Fatalf("connection test: %v", err)
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
	smsSvc := smsproviderusecase.New(f.q, client)
	smsSvc.SetAccounts(kit.Accounts)
	smsSvc.SetClock(clock)
	smsSvc.SetLogger(log)
	notify.Register(smsproviderusecase.NewChannel(smsSvc, customernotifyadapters.SMS{Sender: f.sender}))

	// Readiness is the business's: A has an account, B has none.
	if !notify.ReadyFor(ctx, f.orgA.ID, "sms") || notify.ReadyFor(ctx, f.orgB.ID, "sms") {
		t.Fatal("readiness does not follow the account")
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
	poll := func(want int) {
		t.Helper()
		if n, err := smsSvc.PollReports(ctx); err != nil || n != want {
			t.Fatalf("poll = %d %v, want %d", n, err, want)
		}
	}
	emails := func(reservation fmt.Stringer) int {
		return f.count(`SELECT count(*) FROM customer_notification_deliveries dl
			JOIN customer_notification_dispatches d ON d.organization_id = dl.organization_id AND d.id = dl.dispatch_id
			JOIN reservations r ON r.id = d.reservation_id
			WHERE dl.organization_id = $1 AND r.uuid = $2 AND d.event_code = 'confirmation.request' AND dl.channel = 'email'`,
			f.orgA.ID, reservation.String())
	}
	phone := strings.TrimPrefix(f.huseyinPhone, "+90")

	// 1. Sent through the account, open until the report.
	f.now = at(8, 0).Add(26 * time.Hour) // Tuesday 10:00
	start := at(10, 0).AddDate(0, 0, 7)  // next Monday 10:00
	first := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, start)
	f.now = start.Add(-23 * time.Hour)
	run()
	sent, job1 := fake.last()
	if sent == nil || sent["msgheader"] != "TAKWIM" {
		t.Fatalf("sent = %v", sent)
	}
	msg := sent["messages"].([]any)[0].(map[string]any)
	if msg["no"] != phone || !strings.Contains(msg["msg"].(string), "/panel/v#") {
		t.Fatalf("message = %v", msg)
	}
	if n := f.sender.count(); n != 0 {
		t.Fatalf("the platform sender was used: %d", n)
	}
	if n := f.count(`SELECT count(*) FROM customer_notification_deliveries WHERE organization_id = $1 AND channel = 'sms'
		AND provider = 'netgsm' AND status = 'sent' AND provider_ref = $2 AND sent_at IS NOT NULL AND finished_at IS NULL`, f.orgA.ID, job1); n != 1 {
		t.Fatal("accepted attempt not recorded")
	}
	// Waiting at the operator: nothing changes.
	fake.report(job1, "0", "")
	poll(0)
	run()
	if emails(first.UUID) != 0 {
		t.Fatal("e-mail ran while the text waits for its report")
	}
	// Delivered: a receipt time, the request is done, no confirmation.
	fake.report(job1, "1", f.now.In(ist).Format("2006-01-02 15:04:05"))
	poll(1)
	if n := f.count(`SELECT count(*) FROM customer_notification_deliveries dl
		JOIN customer_notification_dispatches d ON d.organization_id = dl.organization_id AND d.id = dl.dispatch_id
		WHERE dl.organization_id = $1 AND dl.provider_ref = $2 AND dl.status = 'delivered' AND dl.delivered_at = $3
		AND dl.finished_at IS NOT NULL AND d.status = 'delivered'`, f.orgA.ID, job1, f.now); n != 1 {
		t.Fatal("delivery report not recorded")
	}
	if s := confirmation(first.UUID); s != reservationsmodel.ConfirmationPending {
		t.Fatalf("after delivered = %s", s)
	}
	run()
	if emails(first.UUID) != 0 {
		t.Fatal("e-mail ran after a delivered text")
	}
	// A closed attempt is not asked about again.
	poll(0)
	attempts, err := confirm.Attempts(ctx, f.orgA.ID, first.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].DeliveredAt == nil || attempts[0].Channel == nil || *attempts[0].Channel != "sms" {
		t.Fatalf("attempts = %+v", attempts)
	}

	// 2. A failed report runs the next channel.
	f.now = at(8, 0).Add(26 * time.Hour)
	second := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, start.Add(time.Hour))
	f.now = start.Add(-22 * time.Hour)
	run()
	_, job2 := fake.last()
	if job2 == job1 {
		t.Fatal("no second text")
	}
	fake.report(job2, "3", "")
	poll(1)
	if n := f.count(`SELECT count(*) FROM customer_notification_deliveries WHERE organization_id = $1 AND provider_ref = $2
		AND status = 'failed' AND error = 'invalid number' AND delivered_at IS NULL`, f.orgA.ID, job2); n != 1 {
		t.Fatal("failed report not recorded")
	}
	run()
	if emails(second.UUID) != 1 {
		t.Fatal("no e-mail after the failed text")
	}
	if s := confirmation(second.UUID); s != reservationsmodel.ConfirmationPending {
		t.Fatalf("second = %s", s)
	}

	// 3. No report within the timeout: the next channel runs.
	f.now = at(8, 0).Add(26 * time.Hour)
	third := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, start.Add(90*time.Minute))
	f.now = start.Add(90*time.Minute - 23*time.Hour)
	run()
	_, job3 := fake.last()
	poll(0)
	f.now = f.now.Add(customernotifyusecase.DefaultReceiptTimeout + time.Minute)
	// The booking notices of this test wait for their reports too.
	if n, err := notify.ExpireReceipts(ctx); err != nil || n < 1 {
		t.Fatalf("expire = %d %v", n, err)
	}
	run()
	if emails(third.UUID) != 1 {
		t.Fatal("no e-mail after the receipt timeout")
	}
	if n := f.count(`SELECT count(*) FROM customer_notification_deliveries WHERE organization_id = $1 AND provider_ref = $2
		AND status = 'failed' AND error = 'no delivery report'`, f.orgA.ID, job3); n != 1 {
		t.Fatal("timeout not recorded")
	}
	if fake.count() != 3 {
		t.Fatalf("texts = %d", fake.count())
	}

	// 4. A business without an account: skipped, never sent.
	f.exec(`INSERT INTO confirmation_settings (organization_id, enabled, channels, fallback, first_request_minutes,
		follow_up_minutes, max_attempts, deadline_minutes, window_start_minute, window_end_minute, on_no_answer)
		VALUES ($1, true, '{sms}', true, 1440, 0, 1, 120, 540, 1260, 'flag')`, f.orgB.ID)
	f.now = at(8, 0).Add(26 * time.Hour)
	other := f.book(t, f.orgB.ID, f.branchB, f.cutB, f.ayseB, f.huseyinB, start)
	f.now = start.Add(-23 * time.Hour)
	if _, err := confirm.RunDueOrganization(ctx, f.orgB.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := notify.ProcessDueOrganization(ctx, f.orgB.ID); err != nil {
		t.Fatal(err)
	}
	if n := f.count(`SELECT count(*) FROM customer_notification_deliveries dl
		JOIN customer_notification_dispatches d ON d.organization_id = dl.organization_id AND d.id = dl.dispatch_id
		JOIN reservations r ON r.id = d.reservation_id
		WHERE dl.organization_id = $1 AND r.uuid = $2 AND d.event_code = 'confirmation.request' AND dl.channel = 'sms'
		AND dl.status = 'skipped' AND dl.error = 'SMS account not connected'`,
		f.orgB.ID, other.UUID.String()); n != 1 {
		t.Fatalf("business without an account was not skipped (%d)", n)
	}
	if fake.count() != 3 || f.sender.count() != 0 {
		t.Fatal("a text went out for a business without an account")
	}
}
