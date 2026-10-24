package confirmations_test

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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/confirmations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/confirmations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/catalog"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/channel"
	notifyusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	resusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
)

var ist = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

// at is a wall clock time in Istanbul in the week of Monday 2030-01-07.
func at(day, hour, minute int) time.Time {
	return time.Date(2030, 1, day, hour, minute, 0, 0, ist)
}

var none = availabilityusecase.Caller{}

// fakeSMS is an SMS provider; ready decides whether it really delivers.
type fakeSMS struct {
	mu    sync.Mutex
	msgs  []sms.Message
	ready bool
}

func (f *fakeSMS) Send(_ context.Context, m sms.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgs = append(f.msgs, m)
	return nil
}

func (f *fakeSMS) Provider() string { return "fake" }
func (f *fakeSMS) Ready() bool      { return f.ready }

func (f *fakeSMS) texts(part string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, m := range f.msgs {
		if strings.Contains(m.Text, part) {
			n++
		}
	}
	return n
}

type fixture struct {
	pool     *pgxpool.Pool
	q        *db.Queries
	org      orgmodel.Organization
	orgSvc   *orgusecase.Service
	owner    db.User
	branch   uuid.UUID
	branchID int64
	cut      uuid.UUID
	customer uuid.UUID
	now      time.Time
	svc      *resusecase.Service
	notify   *notifyusecase.Engine
	engine   *usecase.Engine
	sms      *fakeSMS
	count    func(sql string, args ...any) int
}

func newFixture(t *testing.T, addon bool) *fixture {
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
	f := &fixture{pool: pool, q: q, now: at(6, 10, 0), sms: &fakeSMS{ready: true}}
	f.owner, err = q.CreateUser(ctx, db.CreateUserParams{
		Email: "owner-" + suffix + "@example.test", PasswordHash: "x", Name: "Owner", Surname: "Test", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.orgSvc = orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	f.org, err = f.orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Kuaför " + suffix, OwnerUserID: f.owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", f.org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", f.owner.ID)
	})
	f.setAddon(t, addon)
	exec := func(sql string, args ...any) {
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
	f.branchID, f.branch = idOf("SELECT id, uuid FROM branches WHERE organization_id = $1 ORDER BY id LIMIT 1", f.org.ID)
	exec(`UPDATE branches SET working_hours = $3, holiday_exceptions = '[]', timezone = NULL WHERE organization_id = $1 AND id = $2`, f.org.ID, f.branchID, hours)
	ayseID, _ := idOf(`INSERT INTO staff (organization_id, name) VALUES ($1, 'Ayşe') RETURNING id, uuid`, f.org.ID)
	exec(`INSERT INTO staff_branches (organization_id, staff_id, branch_id, shifts) VALUES
		($1, $2, $3, '{"mon":[{"start":"09:00","end":"17:00","breaks":[]}]}')`, f.org.ID, ayseID, f.branchID)
	cutID, cut := idOf(`INSERT INTO services (organization_id, name, duration_minutes, price, payment_mode)
		VALUES ($1, 'Kesim', 30, 100, 'both') RETURNING id, uuid`, f.org.ID)
	f.cut = cut
	exec(`INSERT INTO service_branches (organization_id, service_id, branch_id) VALUES ($1, $2, $3)`, f.org.ID, cutID, f.branchID)
	exec(`INSERT INTO service_staff (organization_id, service_id, staff_id) VALUES ($1, $2, $3)`, f.org.ID, cutID, ayseID)
	_, f.customer = idOf(`INSERT INTO customers (organization_id, first_name, last_name, phone_e164, email)
		VALUES ($1, 'Zeynep', 'Kaya', '+905321234567', '') RETURNING id, uuid`, f.org.ID)

	settings := orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))
	clock := func() time.Time { return f.now }
	f.notify = notifyusecase.New(pool, q, "https://takwim.link/panel")
	f.notify.SetClock(clock)
	f.notify.Register(adapters.SMS{Sender: f.sms})
	f.notify.SetLinkIssuerFor(catalog.ConfirmationRequest, func(context.Context, string, channel.Recipient, int64, int64) (string, error) {
		return "https://takwim.link/panel/v#visit-token", nil
	})
	f.engine = usecase.New(pool, q, f.notify)
	f.engine.SetClock(clock)
	f.notify.SetDispatchGuard(f.engine.Guard)
	holds := availabilityusecase.NewHolds(pool, q, settings)
	holds.SetClock(clock)
	n := resusecase.Notifiers(f.notify, f.engine)
	holds.SetExpiryHook(resusecase.ExpireRequestsNotifying(n))
	f.svc = resusecase.New(holds, q, settings)
	f.svc.SetNotifier(n)
	f.engine.SetReservations(f.svc)
	return f
}

func (f *fixture) setAddon(t *testing.T, on bool) {
	t.Helper()
	addons := []string{}
	if on {
		addons = []string{"confirmation"}
	}
	org, err := f.orgSvc.SetEntitlements(context.Background(), f.org.UUID, orgmodel.SetEntitlementsInput{Plan: "enterprise", Addons: addons})
	if err != nil {
		t.Fatal(err)
	}
	f.org = org
}

func (f *fixture) scope() orgctx.Scope {
	return orgctx.Scope{ID: f.org.ID, UUID: f.org.UUID}
}

func (f *fixture) book(t *testing.T, start time.Time) resmodel.Reservation {
	t.Helper()
	r, err := f.svc.Create(context.Background(), f.org.ID, resmodel.CreateInput{
		BranchUUID: f.branch, CustomerUUID: f.customer, StartsAt: &start, Source: resmodel.SourcePanel,
		Lines: []availabilitymodel.LineInput{{ServiceUUID: f.cut}},
	}, none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (f *fixture) get(t *testing.T, r resmodel.Reservation) resmodel.Reservation {
	t.Helper()
	out, err := f.svc.Get(context.Background(), f.org.ID, r.UUID, none)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// tick moves the clock and runs both workers (confirmation tasks, then
// the notices they wrote).
func (f *fixture) tick(t *testing.T, now time.Time) {
	t.Helper()
	f.now = now
	ctx := context.Background()
	if _, err := f.engine.RunDueOrganization(ctx, f.org.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.notify.ProcessDueOrganization(ctx, f.org.ID); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) requests(r resmodel.Reservation) int {
	return f.count(`SELECT count(*) FROM customer_notification_dispatches d JOIN reservations r
		ON r.organization_id = d.organization_id AND r.id = d.reservation_id
		WHERE d.organization_id = $1 AND r.uuid = $2 AND d.event_code = $3`, f.org.ID, r.UUID, catalog.ConfirmationRequest)
}

func eventTypes(r resmodel.Reservation) string {
	var out []string
	for _, e := range r.Events {
		if strings.HasPrefix(e.Type, "confirmation_") || e.Type == "cancelled" {
			out = append(out, e.Type)
		}
	}
	return strings.Join(out, ",")
}

// TestModuleOff (A08): without the add-on no task is planned and the
// booking needs no confirmation.
func TestModuleOff(t *testing.T) {
	f := newFixture(t, false)
	r := f.book(t, at(7, 14, 0))
	if r.ConfirmationStatus != resmodel.ConfirmationNotRequired || r.Confirmation != nil {
		t.Fatalf("status %s, task %+v", r.ConfirmationStatus, r.Confirmation)
	}
	f.tick(t, at(7, 13, 0))
	if f.requests(r) != 0 || f.get(t, r).Status != resmodel.StatusActive {
		t.Fatal("a booking without the add-on was asked or cancelled")
	}
}

// TestConfirmStopsRepeats (A12): the template plans 24 h / 3 h / 2 h; the
// request carries the visit link and the deadline; a confirmation stops
// the follow-up and the cancel.
func TestConfirmStopsRepeats(t *testing.T) {
	f := newFixture(t, true)
	ctx := context.Background()
	r := f.book(t, at(7, 14, 0))
	c := r.Confirmation
	if r.ConfirmationStatus != resmodel.ConfirmationScheduled || c == nil || c.Status != model.TaskScheduled {
		t.Fatalf("status %s task %+v", r.ConfirmationStatus, c)
	}
	if len(c.AttemptTimes) != 2 || !c.AttemptTimes[0].Equal(at(6, 14, 0)) || !c.AttemptTimes[1].Equal(at(7, 11, 0)) ||
		!c.DeadlineAt.Equal(at(7, 12, 0)) {
		t.Fatalf("plan %+v", c)
	}
	f.tick(t, at(6, 13, 59))
	if f.requests(r) != 0 {
		t.Fatal("request before its time")
	}
	f.tick(t, at(6, 14, 0))
	if f.requests(r) != 1 || f.sms.texts("panel/v#visit-token") != 1 || f.sms.texts("07.01.2030 12:00") != 1 {
		t.Fatalf("requests %d, sms %+v", f.requests(r), f.sms.msgs)
	}
	got := f.get(t, r)
	if got.ConfirmationStatus != resmodel.ConfirmationPending || got.Confirmation.AttemptsSent != 1 {
		t.Fatalf("after the request: %s %+v", got.ConfirmationStatus, got.Confirmation)
	}
	// Running again sends nothing twice.
	f.tick(t, at(6, 14, 1))
	if f.requests(r) != 1 {
		t.Fatal("request sent twice")
	}
	if err := f.engine.Answer(ctx, f.org.ID, r.UUID, usecase.AnswerInput{Answer: model.AnswerConfirm, Source: model.SourceVisitLink}); err != nil {
		t.Fatal(err)
	}
	f.tick(t, at(7, 11, 0))
	f.tick(t, at(7, 12, 30))
	got = f.get(t, r)
	if f.requests(r) != 1 || got.Status != resmodel.StatusActive || got.ConfirmationStatus != resmodel.ConfirmationConfirmed ||
		got.Confirmation.Status != model.TaskConfirmed || *got.Confirmation.AnswerChannel != model.SourceVisitLink {
		t.Fatalf("after confirm: requests %d, %s / %s %+v", f.requests(r), got.Status, got.ConfirmationStatus, got.Confirmation)
	}
	if ev := eventTypes(got); ev != "confirmation_scheduled,confirmation_requested,confirmation_confirmed" {
		t.Fatalf("history %s", ev)
	}
	// A second answer is refused.
	if err := f.engine.Answer(ctx, f.org.ID, r.UUID, usecase.AnswerInput{Answer: model.AnswerConfirm}); !errors.Is(err, usecase.ErrNotOpen) {
		t.Fatalf("second answer = %v", err)
	}
}

// TestNoAnswerCancels (A11): no answer by the deadline cancels the booking
// and frees its time; a late answer does not reopen it.
func TestNoAnswerCancels(t *testing.T) {
	f := newFixture(t, true)
	ctx := context.Background()
	r := f.book(t, at(7, 14, 0))
	f.tick(t, at(6, 14, 0))
	f.tick(t, at(7, 11, 0))
	if f.requests(r) != 2 {
		t.Fatalf("requests %d, want first + follow-up", f.requests(r))
	}
	f.tick(t, at(7, 11, 59))
	if f.get(t, r).Status != resmodel.StatusActive {
		t.Fatal("cancelled before the deadline")
	}
	f.tick(t, at(7, 12, 0))
	got := f.get(t, r)
	if got.Status != resmodel.StatusCancelled || *got.CancelReason != resmodel.ReasonConfirmationTimeout ||
		got.ConfirmationStatus != resmodel.ConfirmationExpired || got.Confirmation.Status != model.TaskExpired ||
		*got.Confirmation.Outcome != model.OutcomeCancelled {
		t.Fatalf("after the deadline: %s %v %s %+v", got.Status, got.CancelReason, got.ConfirmationStatus, got.Confirmation)
	}
	// The time is free again.
	f.book(t, at(7, 14, 0))
	if err := f.engine.Answer(ctx, f.org.ID, r.UUID, usecase.AnswerInput{Answer: model.AnswerConfirm}); !errors.Is(err, usecase.ErrNotOpen) {
		t.Fatalf("late answer = %v", err)
	}
	if f.get(t, r).Status != resmodel.StatusCancelled {
		t.Fatal("late answer reopened the booking")
	}
}

// TestUnreachableAndFlag: a booking whose requests never left is flagged,
// never cancelled; the flag rule keeps the booking and a late answer still
// confirms it.
func TestUnreachableAndFlag(t *testing.T) {
	f := newFixture(t, true)
	ctx := context.Background()
	f.sms.ready = false
	r := f.book(t, at(7, 14, 0))
	f.tick(t, at(6, 14, 0))
	f.tick(t, at(7, 11, 0))
	f.tick(t, at(7, 12, 0))
	got := f.get(t, r)
	if got.Status != resmodel.StatusActive || got.ConfirmationStatus != resmodel.ConfirmationExpired ||
		*got.Confirmation.Outcome != model.OutcomeUnreachable {
		t.Fatalf("unreachable: %s %s %+v", got.Status, got.ConfirmationStatus, got.Confirmation)
	}

	f.sms.ready = true
	rule := model.DefaultRule()
	rule.OnNoAnswer = model.OnNoAnswerFlag
	if _, err := f.engine.Save(ctx, f.scope(), f.owner.ID, model.SaveInput{BranchUUID: &f.branch, Rule: rule}); err != nil {
		t.Fatal(err)
	}
	f.now = at(6, 10, 0)
	r2 := f.book(t, at(7, 15, 0))
	f.tick(t, at(6, 15, 0))
	f.tick(t, at(7, 13, 0))
	got = f.get(t, r2)
	if got.Status != resmodel.StatusActive || got.ConfirmationStatus != resmodel.ConfirmationExpired ||
		*got.Confirmation.Outcome != model.OutcomeFlagged {
		t.Fatalf("flagged: %s %s %+v", got.Status, got.ConfirmationStatus, got.Confirmation)
	}
	if err := f.engine.Answer(ctx, f.org.ID, r2.UUID, usecase.AnswerInput{Answer: model.AnswerConfirm}); err != nil {
		t.Fatalf("late confirm of a flagged booking = %v", err)
	}
	if got = f.get(t, r2); got.ConfirmationStatus != resmodel.ConfirmationConfirmed {
		t.Fatalf("late confirm: %s", got.ConfirmationStatus)
	}
}

// TestStaleTasks (A12): a move plans again and drops the queued request
// of the old time; a visit in service is never cancelled by an old task;
// a change request stops the repeats and the cancel.
func TestStaleTasks(t *testing.T) {
	f := newFixture(t, true)
	ctx := context.Background()
	r := f.book(t, at(7, 14, 0))
	// The first request is written but not sent yet; then the visit moves.
	f.now = at(6, 14, 0)
	if _, err := f.engine.RunDueOrganization(ctx, f.org.ID); err != nil {
		t.Fatal(err)
	}
	start := at(7, 15, 0)
	moved, err := f.svc.Reschedule(ctx, f.org.ID, r.UUID, none, f.owner.ID, resmodel.RescheduleInput{StartsAt: &start})
	if err != nil {
		t.Fatal(err)
	}
	if moved.ConfirmationStatus != resmodel.ConfirmationScheduled || !moved.Confirmation.DeadlineAt.Equal(at(7, 13, 0)) {
		t.Fatalf("after the move: %s %+v", moved.ConfirmationStatus, moved.Confirmation)
	}
	if _, err := f.notify.ProcessDueOrganization(ctx, f.org.ID); err != nil {
		t.Fatal(err)
	}
	if f.sms.texts("panel/v#") != 0 {
		t.Fatal("the request of the old time went out")
	}
	// Started before its deadline: the deadline does nothing.
	f.tick(t, at(6, 15, 0))
	if _, err := f.svc.Start(ctx, f.org.ID, r.UUID, none, f.owner.ID, resusecase.ChangeInput{}); err != nil {
		t.Fatal(err)
	}
	f.tick(t, at(7, 13, 0))
	got := f.get(t, r)
	if got.Status != resmodel.StatusInService || got.Confirmation.Status != model.TaskDropped {
		t.Fatalf("in service: %s %+v", got.Status, got.Confirmation)
	}

	// A change request: no follow-up, no cancel; the booking stays.
	f.now = at(6, 10, 0)
	r2 := f.book(t, at(7, 16, 0))
	f.tick(t, at(6, 16, 0))
	if err := f.engine.Answer(ctx, f.org.ID, r2.UUID, usecase.AnswerInput{Answer: model.AnswerChange, Note: "Salı olur mu?"}); err != nil {
		t.Fatal(err)
	}
	f.tick(t, at(7, 13, 0))
	f.tick(t, at(7, 14, 30))
	got = f.get(t, r2)
	if f.requests(r2) != 1 || got.Status != resmodel.StatusActive || got.Confirmation.ChangeRequestedAt == nil ||
		got.Confirmation.ChangeNote != "Salı olur mu?" {
		t.Fatalf("change request: requests %d %s %+v", f.requests(r2), got.Status, got.Confirmation)
	}
}

// TestCustomerCancelDeclines: a cancel by the customer while the request
// is open is "Gelmeyeceğim".
func TestCustomerCancelDeclines(t *testing.T) {
	f := newFixture(t, true)
	ctx := context.Background()
	r := f.book(t, at(7, 14, 0))
	f.tick(t, at(6, 14, 0))
	if _, err := f.svc.Cancel(ctx, f.org.ID, r.UUID, none, 0, resusecase.ChangeInput{Reason: resmodel.ReasonCustomerCancelled}); err != nil {
		t.Fatal(err)
	}
	got := f.get(t, r)
	if got.ConfirmationStatus != resmodel.ConfirmationDeclined || got.Confirmation.Status != model.TaskDeclined {
		t.Fatalf("declined: %s %+v", got.ConfirmationStatus, got.Confirmation)
	}
	f.tick(t, at(7, 11, 0))
	if f.requests(r) != 1 {
		t.Fatal("follow-up after the cancel")
	}
}

// TestSettings: the template, a branch override, invalid settings refused,
// a branch rule turned off (no task), and reset.
func TestSettings(t *testing.T) {
	f := newFixture(t, true)
	ctx := context.Background()
	s, err := f.engine.Settings(ctx, f.scope())
	if err != nil {
		t.Fatal(err)
	}
	if s.DefaultCustom || len(s.Branches) == 0 || s.Branches[0].Custom || s.Default.FirstRequestMinutes != 1440 {
		t.Fatalf("settings %+v", s)
	}
	bad := model.DefaultRule()
	bad.DeadlineMinutes = 1440
	if _, err := f.engine.Save(ctx, f.scope(), f.owner.ID, model.SaveInput{Rule: bad}); err == nil {
		t.Fatal("deadline after the first request saved")
	}
	off := model.DefaultRule()
	off.Enabled = false
	if s, err = f.engine.Save(ctx, f.scope(), f.owner.ID, model.SaveInput{BranchUUID: &f.branch, Rule: off}); err != nil {
		t.Fatal(err)
	}
	var branch model.BranchRule
	for _, b := range s.Branches {
		if b.Branch.UUID == f.branch {
			branch = b
		}
	}
	if !branch.Custom || branch.Rule.Enabled {
		t.Fatalf("branch rule %+v", branch)
	}
	if r := f.book(t, at(7, 14, 0)); r.Confirmation != nil || r.ConfirmationStatus != resmodel.ConfirmationNotRequired {
		t.Fatalf("branch off but planned: %+v", r.Confirmation)
	}
	if s, err = f.engine.Reset(ctx, f.scope(), model.ResetInput{BranchUUID: &f.branch}); err != nil {
		t.Fatal(err)
	}
	for _, b := range s.Branches {
		if b.Branch.UUID == f.branch && b.Custom {
			t.Fatal("reset kept the branch rule")
		}
	}
	other := uuid.New()
	if _, err := f.engine.Reset(ctx, f.scope(), model.ResetInput{BranchUUID: &other}); !errors.Is(err, usecase.ErrBranchNotFound) {
		t.Fatalf("unknown branch = %v", err)
	}
	// The add-on turned off after planning: the task is dropped, nothing
	// is cancelled.
	r := f.book(t, at(7, 15, 0))
	f.setAddon(t, false)
	f.tick(t, at(6, 15, 0))
	got := f.get(t, r)
	if got.Confirmation.Status != model.TaskDropped || got.ConfirmationStatus != resmodel.ConfirmationNotRequired || f.requests(r) != 0 {
		t.Fatalf("add-on off: %s %+v", got.ConfirmationStatus, got.Confirmation)
	}
}
