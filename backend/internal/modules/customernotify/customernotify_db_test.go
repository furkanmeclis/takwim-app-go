package customernotify_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilitymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/catalog"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/channel"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	orgsettingsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	resusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/mail"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
)

var ist = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

// at is a Monday 2030-01-07 wall clock time in Istanbul.
func at(hour, minute int) time.Time {
	return time.Date(2030, 1, 7, hour, minute, 0, 0, ist)
}

var none = availabilityusecase.Caller{}

// fakeSMS is a ready SMS provider that records what it sent.
type fakeSMS struct {
	mu   sync.Mutex
	msgs []sms.Message
	fail error
}

func (f *fakeSMS) Send(_ context.Context, m sms.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	f.msgs = append(f.msgs, m)
	return nil
}

func (f *fakeSMS) Provider() string { return "fake" }
func (f *fakeSMS) Ready() bool      { return true }

func (f *fakeSMS) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.msgs)
}

type fakeMail struct {
	mu   sync.Mutex
	msgs []mail.Message
}

func (f *fakeMail) Send(_ context.Context, m mail.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgs = append(f.msgs, m)
	return nil
}

func (f *fakeMail) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.msgs)
}

type fixture struct {
	pool      *pgxpool.Pool
	q         *db.Queries
	org, org2 orgmodel.Organization
	owner     db.User
	branch    uuid.UUID
	cut       uuid.UUID
	customer  uuid.UUID
	custID    int64
	now       time.Time
	settings  *orgsettingsusecase.Service
	holds     *availabilityusecase.Holds
	svc       *resusecase.Service
	engine    *usecase.Engine
	sms       *fakeSMS
	mail      *fakeMail
	exec      func(sql string, args ...any)
	count     func(sql string, args ...any) int
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	suffix := uuid.NewString()[:8]
	f := &fixture{pool: pool, q: q, now: at(8, 0), sms: &fakeSMS{}, mail: &fakeMail{}}
	newUser := func(name string) db.User {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: name + "-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	f.owner = newUser("owner")
	other := newUser("other")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	f.org, err = orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Kuaför " + suffix, OwnerUserID: f.owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	f.org2, err = orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Klinik " + suffix, OwnerUserID: other.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{f.org.ID, f.org2.ID})
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{f.owner.ID, other.ID})
	})
	f.exec = func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	f.count = func(sql string, args ...any) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return n
	}
	idOf := func(sql string, args ...any) (int64, uuid.UUID) {
		t.Helper()
		var id int64
		var u uuid.UUID
		if err := pool.QueryRow(ctx, sql, args...).Scan(&id, &u); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return id, u
	}
	hours := `{"mon":{"closed":false,"open":"09:00","close":"18:00"}}`
	aID, aUUID := idOf("SELECT id, uuid FROM branches WHERE organization_id = $1 ORDER BY id LIMIT 1", f.org.ID)
	f.exec(`UPDATE branches SET working_hours = $3, holiday_exceptions = '[]', timezone = NULL WHERE organization_id = $1 AND id = $2`, f.org.ID, aID, hours)
	f.branch = aUUID
	ayseID, _ := idOf(`INSERT INTO staff (organization_id, name) VALUES ($1, 'Ayşe') RETURNING id, uuid`, f.org.ID)
	f.exec(`INSERT INTO staff_branches (organization_id, staff_id, branch_id, shifts) VALUES
		($1, $2, $3, '{"mon":[{"start":"09:00","end":"17:00","breaks":[]}]}')`, f.org.ID, ayseID, aID)
	cutID, cut := idOf(`INSERT INTO services (organization_id, name, duration_minutes, price, payment_mode)
		VALUES ($1, 'Kesim', 30, 100, 'both') RETURNING id, uuid`, f.org.ID)
	f.cut = cut
	f.exec(`INSERT INTO service_branches (organization_id, service_id, branch_id) VALUES ($1, $2, $3)`, f.org.ID, cutID, aID)
	f.exec(`INSERT INTO service_staff (organization_id, service_id, staff_id) VALUES ($1, $2, $3)`, f.org.ID, cutID, ayseID)
	f.custID, f.customer = idOf(`INSERT INTO customers (organization_id, first_name, last_name, phone_e164, email)
		VALUES ($1, 'Zeynep', 'Kaya', '+905321234567', 'zeynep@example.test') RETURNING id, uuid`, f.org.ID)
	f.settings = orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))
	f.engine = usecase.New(pool, q, "https://takwim.link/panel")
	f.engine.SetClock(func() time.Time { return f.now })
	f.engine.Register(adapters.SMS{Sender: f.sms}, adapters.Email{Mail: f.mail, Configured: true})
	f.holds = availabilityusecase.NewHolds(pool, q, f.settings)
	f.holds.SetClock(func() time.Time { return f.now })
	f.holds.SetExpiryHook(resusecase.ExpireRequestsNotifying(f.engine))
	f.svc = resusecase.New(f.holds, q, f.settings)
	f.svc.SetNotifier(f.engine)
	return f
}

func (f *fixture) input(start time.Time, source, payment string) resmodel.CreateInput {
	return resmodel.CreateInput{
		BranchUUID: f.branch, CustomerUUID: f.customer, StartsAt: &start, Source: source, Payment: payment,
		Lines: []availabilitymodel.LineInput{{ServiceUUID: f.cut}},
	}
}

func (f *fixture) online(t *testing.T, approval string) {
	t.Helper()
	lead, advance, window, payment := 0, 30, 30, 15
	s := orgsettingsmodel.Defaults()
	s.Reservation = orgsettingsmodel.Reservation{
		ApprovalMode: approval, StaffReviewWindowMinutes: &window, PaymentHoldMinutes: &payment,
		MinLeadMinutes: &lead, MaxAdvanceDays: &advance,
	}
	s.Payment = orgsettingsmodel.Payment{PayAtVenue: true, PayOnline: true}
	if _, err := f.settings.Save(context.Background(), f.org.ID, f.owner.ID, orgsettingsmodel.SaveInput{Settings: s}); err != nil {
		t.Fatal(err)
	}
}

type dispatchRow struct {
	event, status, lastError string
	vars                     string
}

func (f *fixture) dispatches(t *testing.T, r resmodel.Reservation) []dispatchRow {
	t.Helper()
	rows, err := f.pool.Query(context.Background(), `SELECT d.event_code, d.status, d.last_error, d.vars::text
		FROM customer_notification_dispatches d JOIN reservations r ON r.organization_id = d.organization_id AND r.id = d.reservation_id
		WHERE d.organization_id = $1 AND r.uuid = $2 ORDER BY d.id`, f.org.ID, r.UUID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []dispatchRow
	for rows.Next() {
		var d dispatchRow
		if err := rows.Scan(&d.event, &d.status, &d.lastError, &d.vars); err != nil {
			t.Fatal(err)
		}
		out = append(out, d)
	}
	return out
}

func events(ds []dispatchRow) string {
	var out []string
	for _, d := range ds {
		out = append(out, d.event+"="+d.status)
	}
	return strings.Join(out, ",")
}

func (f *fixture) process(t *testing.T) {
	t.Helper()
	if _, err := f.engine.ProcessDueOrganization(context.Background(), f.org.ID); err != nil {
		t.Fatal(err)
	}
}

// TestPanelBookingNoticeAndIdempotency: a booking writes its notice in the
// same transaction, the worker sends it once, a replayed change and a
// second run send nothing more, and a failed booking leaves nothing.
func TestPanelBookingNoticeAndIdempotency(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	r, err := f.svc.Create(ctx, f.org.ID, f.input(at(9, 0), resmodel.SourcePanel, ""), none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	ds := f.dispatches(t, r)
	if events(ds) != catalog.ReservationAccepted+"=pending" {
		t.Fatalf("dispatches = %s", events(ds))
	}
	for _, want := range []string{f.org.Name, "07.01.2030 09:00", "https://takwim.link/panel", "Zeynep Kaya", "Kesim"} {
		if !strings.Contains(ds[0].vars, want) {
			t.Fatalf("vars %s lack %q", ds[0].vars, want)
		}
	}
	// The same change again (a replay) writes no second notice.
	var resID int64
	if err := f.pool.QueryRow(ctx, "SELECT id FROM reservations WHERE organization_id = $1 AND uuid = $2", f.org.ID, r.UUID).Scan(&resID); err != nil {
		t.Fatal(err)
	}
	if err := f.engine.ReservationChanged(ctx, f.q, resusecase.Change{OrganizationID: f.org.ID, ReservationID: resID, Type: "created", To: "active"}); err != nil {
		t.Fatal(err)
	}
	if n := len(f.dispatches(t, r)); n != 1 {
		t.Fatalf("after replay %d dispatches", n)
	}

	f.process(t)
	f.process(t)
	if f.sms.count() != 1 || f.mail.count() != 0 {
		t.Fatalf("sms %d mail %d", f.sms.count(), f.mail.count())
	}
	msg := f.sms.msgs[0]
	if msg.To != "+905321234567" || msg.Purpose != sms.PurposeCustomerNotice || !strings.Contains(msg.Text, "randevunuz onaylandı") ||
		!strings.Contains(msg.Text, f.org.Name) || !strings.Contains(msg.Text, "https://takwim.link/panel") {
		t.Fatalf("sms = %+v", msg)
	}
	if ds := f.dispatches(t, r); ds[0].status != usecase.DispatchDelivered {
		t.Fatalf("dispatch = %+v", ds[0])
	}
	if n := f.count(`SELECT count(*) FROM customer_notification_deliveries WHERE organization_id = $1 AND status = 'sent' AND recipient_masked = $2`,
		f.org.ID, sms.Mask("+905321234567")); n != 1 {
		t.Fatalf("sent deliveries = %d", n)
	}

	// A booking that fails (the time is taken) writes no notice.
	before := f.count("SELECT count(*) FROM customer_notification_dispatches WHERE organization_id = $1", f.org.ID)
	if _, err := f.svc.Create(ctx, f.org.ID, f.input(at(9, 0), resmodel.SourcePanel, ""), none, f.owner.ID); !errors.Is(err, availabilityusecase.ErrSlotTaken) {
		t.Fatalf("second booking = %v", err)
	}
	if n := f.count("SELECT count(*) FROM customer_notification_dispatches WHERE organization_id = $1", f.org.ID); n != before {
		t.Fatalf("dispatches after a failed booking %d → %d", before, n)
	}

	// Move and cancel: one notice each, in order.
	if _, err := f.svc.Reschedule(ctx, f.org.ID, r.UUID, none, f.owner.ID, resmodel.RescheduleInput{StartsAt: ptr(at(10, 0))}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Cancel(ctx, f.org.ID, r.UUID, none, f.owner.ID, resusecase.ChangeInput{}); err != nil {
		t.Fatal(err)
	}
	f.process(t)
	if got := events(f.dispatches(t, r)); got != "reservation.accepted=delivered,reservation.rescheduled=delivered,reservation.cancelled=delivered" {
		t.Fatalf("dispatches = %s", got)
	}
	if !strings.Contains(f.sms.msgs[1].Text, "07.01.2030 10:00") || !strings.Contains(f.sms.msgs[2].Text, "iptal") {
		t.Fatalf("texts = %q / %q", f.sms.msgs[1].Text, f.sms.msgs[2].Text)
	}

	// The other business sees none of it.
	items, total, err := f.engine.Deliveries(ctx, f.org2.ID, model.DeliveryFilter{})
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("other business deliveries = %d %v", total, err)
	}
	items, total, err = f.engine.Deliveries(ctx, f.org.ID, model.DeliveryFilter{ReservationUUID: &r.UUID})
	if err != nil || total != 3 || items[0].CustomerName != "Zeynep Kaya" {
		t.Fatalf("deliveries = %d %+v %v", total, items, err)
	}
}

func ptr[T any](v T) *T { return &v }

// TestChannelsNotReadyAndFallback: a provider that does not deliver (the
// log sender) is never recorded as sent; the next channel runs; with no
// ready channel the notice ends skipped.
func TestChannelsNotReadyAndFallback(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.engine.Register(adapters.SMS{Sender: sms.NewLogSender(nil, false)})

	r, err := f.svc.Create(ctx, f.org.ID, f.input(at(9, 0), resmodel.SourcePanel, ""), none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.process(t)
	if got := events(f.dispatches(t, r)); got != "reservation.accepted=delivered" || f.mail.count() != 1 {
		t.Fatalf("dispatch = %s, mail %d", got, f.mail.count())
	}
	if n := f.count(`SELECT count(*) FROM customer_notification_deliveries WHERE organization_id = $1 AND channel = 'sms' AND status = 'skipped' AND provider = 'log'`, f.org.ID); n != 1 {
		t.Fatalf("skipped sms = %d", n)
	}
	m := f.mail.msgs[0]
	if m.To[0] != "zeynep@example.test" || !strings.Contains(m.Subject, "onaylandı") || !strings.Contains(m.Body, "Merhaba Zeynep Kaya") {
		t.Fatalf("mail = %+v", m)
	}

	// No ready channel at all: skipped, nothing sent.
	f.engine.Register(adapters.Email{Mail: f.mail, Configured: false})
	if _, err := f.svc.Cancel(ctx, f.org.ID, r.UUID, none, f.owner.ID, resusecase.ChangeInput{}); err != nil {
		t.Fatal(err)
	}
	f.process(t)
	ds := f.dispatches(t, r)
	if ds[1].event != catalog.ReservationCancelled || ds[1].status != usecase.DispatchSkipped || f.mail.count() != 1 {
		t.Fatalf("cancel dispatch = %+v, mail %d", ds[1], f.mail.count())
	}
	if n := f.count(`SELECT count(*) FROM customer_notification_deliveries WHERE organization_id = $1 AND status IN ('sent', 'delivered') AND channel = 'sms'`, f.org.ID); n != 0 {
		t.Fatalf("sms recorded as sent: %d", n)
	}

	// A failing provider: failed attempt, fallback to the next channel.
	f.sms.fail = errors.New("provider down")
	f.engine.Register(adapters.SMS{Sender: f.sms}, adapters.Email{Mail: f.mail, Configured: true})
	r2, err := f.svc.Create(ctx, f.org.ID, f.input(at(11, 0), resmodel.SourcePanel, ""), none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.process(t)
	if got := events(f.dispatches(t, r2)); got != "reservation.accepted=delivered" || f.mail.count() != 2 {
		t.Fatalf("fallback = %s, mail %d", got, f.mail.count())
	}
	if n := f.count(`SELECT count(*) FROM customer_notification_deliveries WHERE organization_id = $1 AND channel = 'sms' AND status = 'failed' AND error = 'provider down'`, f.org.ID); n != 1 {
		t.Fatalf("failed sms = %d", n)
	}

	// A customer without a phone: the SMS attempt is skipped.
	f.sms.fail = nil
	f.exec("UPDATE customers SET phone_e164 = NULL WHERE organization_id = $1 AND id = $2", f.org.ID, f.custID)
	r3, err := f.svc.Create(ctx, f.org.ID, f.input(at(12, 0), resmodel.SourcePanel, ""), none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.process(t)
	if got := events(f.dispatches(t, r3)); got != "reservation.accepted=delivered" || f.sms.count() != 0 || f.mail.count() != 3 {
		t.Fatalf("no phone = %s, sms %d mail %d", got, f.sms.count(), f.mail.count())
	}
}

// TestOnlineRequestLifecycle: request received, rejected; a request whose
// review window lapses is cancelled with a notice; a business that turned
// an event off gets none.
func TestOnlineRequestLifecycle(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.online(t, orgsettingsmodel.ApprovalStaff)

	r, err := f.svc.Create(ctx, f.org.ID, f.input(at(9, 0), resmodel.SourceOnline, resmodel.MethodOnSite), none, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Reject(ctx, f.org.ID, r.UUID, none, f.owner.ID, resusecase.ChangeInput{}); err != nil {
		t.Fatal(err)
	}
	f.process(t)
	if got := events(f.dispatches(t, r)); got != "reservation.requested=delivered,reservation.rejected=delivered" {
		t.Fatalf("dispatches = %s", got)
	}
	if !strings.Contains(f.sms.msgs[0].Text, "talebiniz alındı") || !strings.Contains(f.sms.msgs[1].Text, "onaylanamadı") {
		t.Fatalf("texts = %q / %q", f.sms.msgs[0].Text, f.sms.msgs[1].Text)
	}

	// Accepted once, even when a payment follows the acceptance.
	r2, err := f.svc.Create(ctx, f.org.ID, f.input(at(10, 0), resmodel.SourceOnline, resmodel.MethodOnline), none, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Accept(ctx, f.org.ID, r2.UUID, none, f.owner.ID, resusecase.ChangeInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.MarkPaid(ctx, f.org.ID, r2.UUID, 0, resusecase.ChangeInput{}); err != nil {
		t.Fatal(err)
	}
	if got := events(f.dispatches(t, r2)); got != "reservation.requested=pending,reservation.accepted=pending" {
		t.Fatalf("paid dispatches = %s", got)
	}

	// The business turns "cancelled" off; a lapsed request then gets none.
	if _, err := f.engine.Save(ctx, f.org.ID, f.owner.ID, model.SaveInput{Event: catalog.ReservationCancelled, Enabled: false, Channels: []string{"sms"}}); err != nil {
		t.Fatal(err)
	}
	r3, err := f.svc.Create(ctx, f.org.ID, f.input(at(11, 0), resmodel.SourceOnline, resmodel.MethodOnSite), none, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.now = at(8, 31)
	if n, err := f.holds.ExpireDue(ctx); err != nil || n < 1 {
		t.Fatalf("expire = %d %v", n, err)
	}
	if got := events(f.dispatches(t, r3)); got != "reservation.requested=pending" {
		t.Fatalf("lapsed (off) = %s", got)
	}
	if _, err := f.engine.Reset(ctx, f.org.ID, catalog.ReservationCancelled); err != nil {
		t.Fatal(err)
	}
	r4, err := f.svc.Create(ctx, f.org.ID, f.input(at(12, 0), resmodel.SourceOnline, resmodel.MethodOnSite), none, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.now = at(9, 2)
	if _, err := f.holds.ExpireDue(ctx); err != nil {
		t.Fatal(err)
	}
	if got := events(f.dispatches(t, r4)); got != "reservation.requested=pending,reservation.cancelled=pending" {
		t.Fatalf("lapsed = %s", got)
	}
}

// TestReminders: one reminder per offset and visit time, none twice, none
// for a visit booked after the reminder moment, and a moved visit drops
// the old reminder at send time.
func TestReminders(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.engine.Save(ctx, f.org.ID, f.owner.ID, model.SaveInput{
		Event: catalog.ReservationReminder, Enabled: true, Channels: []string{"sms"}, Fallback: true, ReminderOffsets: []int32{60},
	}); err != nil {
		t.Fatal(err)
	}
	r, err := f.svc.Create(ctx, f.org.ID, f.input(at(14, 0), resmodel.SourcePanel, ""), none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.process(t) // the booking notice
	sent := f.sms.count()

	f.now = at(12, 30)
	if n, _ := f.engine.RemindOrganization(ctx, f.org.ID); n != 0 {
		t.Fatalf("early reminders = %d", n)
	}
	f.now = at(13, 1)
	if n, _ := f.engine.RemindOrganization(ctx, f.org.ID); n != 1 {
		t.Fatalf("reminders = %d", n)
	}
	if n, _ := f.engine.RemindOrganization(ctx, f.org.ID); n != 0 {
		t.Fatalf("second run = %d", n)
	}
	f.process(t)
	if f.sms.count() != sent+1 || !strings.Contains(f.sms.msgs[sent].Text, "Hatırlatma") {
		t.Fatalf("reminder sms = %d", f.sms.count()-sent)
	}

	// A reminder planned, then the visit moves before it goes out.
	f.now = at(8, 0)
	r2, err := f.svc.Create(ctx, f.org.ID, f.input(at(15, 0), resmodel.SourcePanel, ""), none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.process(t)
	f.now = at(14, 1)
	if n, _ := f.engine.RemindOrganization(ctx, f.org.ID); n != 1 {
		t.Fatalf("second visit reminders = %d", n)
	}
	f.now = at(8, 0)
	if _, err := f.svc.Reschedule(ctx, f.org.ID, r2.UUID, none, f.owner.ID, resmodel.RescheduleInput{StartsAt: ptr(at(16, 0))}); err != nil {
		t.Fatal(err)
	}
	f.now = at(14, 2)
	f.process(t)
	ds := f.dispatches(t, r2)
	var reminder dispatchRow
	for _, d := range ds {
		if d.event == catalog.ReservationReminder {
			reminder = d
		}
	}
	if reminder.status != usecase.DispatchCancelled || reminder.lastError != "reservation changed" {
		t.Fatalf("moved visit reminder = %+v (%s)", reminder, events(ds))
	}
	// The new time gets its own reminder.
	f.now = at(15, 1)
	if n, _ := f.engine.RemindOrganization(ctx, f.org.ID); n != 1 {
		t.Fatalf("moved visit new reminder = %d", n)
	}
	_ = r
}

// TestSettingsValidationAndTexts: a business text is used when sending,
// unknown variables and bad offsets are refused, reset goes back to the
// defaults.
func TestSettingsValidationAndTexts(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var ve *usecase.ValidationError
	if _, err := f.engine.Save(ctx, f.org.ID, f.owner.ID, model.SaveInput{
		Event: catalog.ReservationAccepted, Enabled: true, Channels: []string{"sms"},
		Templates: []model.TemplateInput{{Channel: "sms", Body: "Merhaba {{customer_name}} {{iban}}"}},
	}); !errors.As(err, &ve) || ve.Details[0].Code != "unknown_variable" {
		t.Fatalf("unknown variable = %v", err)
	}
	if _, err := f.engine.Save(ctx, f.org.ID, f.owner.ID, model.SaveInput{
		Event: catalog.ReservationReminder, Enabled: true, Channels: []string{"sms", "fax"}, ReminderOffsets: []int32{5},
	}); !errors.As(err, &ve) || len(ve.Details) != 2 {
		t.Fatalf("bad rule = %v", err)
	}
	if _, err := f.engine.Save(ctx, f.org.ID, f.owner.ID, model.SaveInput{
		Event: catalog.ReservationAccepted, Enabled: true, Channels: []string{"whatsapp", "sms"}, Fallback: true,
		Templates: []model.TemplateInput{{Channel: "sms", Body: "{{business_name}} seni bekliyor {{customer_name}}, {{time}}"}},
	}); err != nil {
		t.Fatal(err)
	}
	s, err := f.engine.Settings(ctx, f.org.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, ch := range s.Channels {
		if ch.Name == channel.WhatsApp && ch.Ready {
			t.Fatal("whatsapp reported ready without an adapter")
		}
	}
	r, err := f.svc.Create(ctx, f.org.ID, f.input(at(9, 0), resmodel.SourcePanel, ""), none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.process(t)
	if got := events(f.dispatches(t, r)); got != "reservation.accepted=delivered" || f.sms.msgs[0].Text != f.org.Name+" seni bekliyor Zeynep Kaya, 09:00" {
		t.Fatalf("custom text = %s %q", got, f.sms.msgs[0].Text)
	}
	if n := f.count(`SELECT count(*) FROM customer_notification_deliveries WHERE organization_id = $1 AND channel = 'whatsapp' AND status = 'skipped'`, f.org.ID); n != 1 {
		t.Fatalf("whatsapp skipped = %d", n)
	}
	ev, err := f.engine.Reset(ctx, f.org.ID, catalog.ReservationAccepted)
	if err != nil || ev.Custom || ev.Templates[0].Custom || strings.Join(ev.Rule.Channels, ",") != "sms,email" {
		t.Fatalf("reset = %+v %v", ev, err)
	}
	p, err := f.engine.Preview(ctx, f.org.ID, model.PreviewInput{Event: catalog.ReservationReminder, Channel: "sms", Body: "{{business_name}} {{nope}}"})
	if err != nil || p.Body != f.org.Name+" {{nope}}" || len(p.Unknown) != 1 {
		t.Fatalf("preview = %+v %v", p, err)
	}
}

// fakeLinker issues numbered panel links per phone.
type fakeLinker struct{ phones []string }

func (l *fakeLinker) PanelLink(_ context.Context, phone string) string {
	l.phones = append(l.phones, phone)
	return "https://takwim.link/panel/l#TOKEN" + strings.Repeat("x", len(l.phones))
}

// TestPersonalPanelLink: an SMS notice carries the phone's single-use panel
// link (TEC-910), put in at send time only; the stored text keeps the public
// address. A channel that is not ready never uses up a link.
func TestPersonalPanelLink(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	linker := &fakeLinker{}
	f.engine.SetLinkIssuer(adapters.PanelLinks(linker))
	if _, err := f.svc.Create(ctx, f.org.ID, f.input(at(9, 0), resmodel.SourcePanel, ""), none, f.owner.ID); err != nil {
		t.Fatal(err)
	}
	f.process(t)
	if f.sms.count() != 1 || len(linker.phones) != 1 || linker.phones[0] != "+905321234567" {
		t.Fatalf("sms %d links %v", f.sms.count(), linker.phones)
	}
	if text := f.sms.msgs[0].Text; !strings.Contains(text, "https://takwim.link/panel/l#TOKENx") {
		t.Fatalf("sms without the personal link: %q", text)
	}
	if n := f.count(`SELECT count(*) FROM customer_notification_deliveries WHERE organization_id = $1 AND body LIKE '%#TOKEN%'`, f.org.ID); n != 0 {
		t.Fatalf("stored texts with a link token = %d", n)
	}

	// The log sender is not ready: no link is issued for it.
	f.engine.Register(adapters.SMS{Sender: sms.NewLogSender(nil, false)})
	if _, err := f.svc.Create(ctx, f.org.ID, f.input(at(11, 0), resmodel.SourcePanel, ""), none, f.owner.ID); err != nil {
		t.Fatal(err)
	}
	f.process(t)
	if len(linker.phones) != 1 {
		t.Fatalf("links issued for a channel that is not ready: %v", linker.phones)
	}
}

// fakePublisher records the live events the engine publishes.
type fakePublisher struct {
	mu    sync.Mutex
	uuids []string
}

func (p *fakePublisher) Publish(_ context.Context, _, _ string, uuids ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.uuids = append(p.uuids, uuids...)
}

// TestStuckAttemptsClosed (TEC-974): an attempt left "sending" by a process
// that stopped between the claim and the record is closed after the stuck
// timeout as failed with an unknown outcome; it is never sent again, no next
// channel runs, and the live lists hear of it. A fresh attempt is left alone.
func TestStuckAttemptsClosed(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	pub := &fakePublisher{}
	f.engine.SetResourceEvents(pub, catalog.ReservationAccepted)

	// What a stopped process leaves behind: the claim committed (dispatch
	// "sending", attempt "sending"), the send result never recorded.
	stall := func(r resmodel.Reservation, age time.Duration) {
		t.Helper()
		var dispatchID int64
		if err := f.pool.QueryRow(ctx, `UPDATE customer_notification_dispatches d SET status = 'sending'
			FROM reservations r WHERE r.organization_id = d.organization_id AND r.id = d.reservation_id
			  AND d.organization_id = $1 AND r.uuid = $2 RETURNING d.id`, f.org.ID, r.UUID).Scan(&dispatchID); err != nil {
			t.Fatal(err)
		}
		f.exec(`INSERT INTO customer_notification_deliveries (organization_id, dispatch_id, channel, attempt_no, status, body, created_at)
			VALUES ($1, $2, 'sms', 1, 'sending', 'text', $3)`, f.org.ID, dispatchID, f.now.Add(-age))
	}
	stuck, err := f.svc.Create(ctx, f.org.ID, f.input(at(9, 0), resmodel.SourcePanel, ""), none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := f.svc.Create(ctx, f.org.ID, f.input(at(10, 0), resmodel.SourcePanel, ""), none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	stall(stuck, usecase.DefaultStuckTimeout+time.Minute)
	stall(fresh, time.Minute)

	if n, err := f.engine.CloseStuck(ctx); err != nil || n < 1 {
		t.Fatalf("close stuck = %d %v", n, err)
	}
	ds := f.dispatches(t, stuck)
	if events(ds) != "reservation.accepted=failed" || ds[0].lastError != "sms: "+usecase.ErrorOutcomeUnknown {
		t.Fatalf("stuck dispatch = %+v", ds)
	}
	if n := f.count(`SELECT count(*) FROM customer_notification_deliveries dl
		JOIN customer_notification_dispatches d ON d.organization_id = dl.organization_id AND d.id = dl.dispatch_id
		JOIN reservations r ON r.organization_id = d.organization_id AND r.id = d.reservation_id
		WHERE dl.organization_id = $1 AND r.uuid = $2 AND dl.status = 'failed' AND dl.error = $3 AND dl.finished_at IS NOT NULL`,
		f.org.ID, stuck.UUID, usecase.ErrorOutcomeUnknown); n != 1 {
		t.Fatalf("closed attempts = %d", n)
	}
	if got := events(f.dispatches(t, fresh)); got != "reservation.accepted=sending" {
		t.Fatalf("fresh dispatch = %s", got)
	}
	pub.mu.Lock()
	published := strings.Join(pub.uuids, ",")
	pub.mu.Unlock()
	if published != stuck.UUID.String() {
		t.Fatalf("live events = %q", published)
	}

	// Nothing is sent again, on that channel or the next one, and a second
	// sweep leaves the fresh attempt alone.
	f.process(t)
	if f.sms.count() != 0 || f.mail.count() != 0 {
		t.Fatalf("resent: sms %d mail %d", f.sms.count(), f.mail.count())
	}
	if _, err := f.engine.CloseStuck(ctx); err != nil {
		t.Fatal(err)
	}
	if got := events(f.dispatches(t, fresh)); got != "reservation.accepted=sending" {
		t.Fatalf("fresh dispatch after a second sweep = %s", got)
	}

	// Once the fresh one is old too, it is closed the same way.
	f.now = f.now.Add(usecase.DefaultStuckTimeout)
	if _, err := f.engine.CloseStuck(ctx); err != nil {
		t.Fatal(err)
	}
	if got := events(f.dispatches(t, fresh)); got != "reservation.accepted=failed" {
		t.Fatalf("fresh dispatch later = %s", got)
	}
}
