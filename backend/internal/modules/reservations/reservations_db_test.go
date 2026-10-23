package reservations_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilitymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	orgsettingsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
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

// fixture: one business (branch A, Ayşe 09:00–12:00 on Mondays, a cut with
// list price 100, branch price 120 and Ayşe's personal price 150, a wash
// without a personal price), a customer, and a second business.
type fixture struct {
	pool           *pgxpool.Pool
	q              *db.Queries
	org, org2      orgmodel.Organization
	owner, emre    db.User
	fatma          db.User
	branch         uuid.UUID
	cut, wash      uuid.UUID
	cutID          int64
	customer       uuid.UUID
	foreignCust    uuid.UUID
	settings       *orgsettingsusecase.Service
	now            time.Time
	holds          *availabilityusecase.Holds
	svc            *usecase.Service
	exec           func(sql string, args ...any)
	count          func(sql string, args ...any) int
	reservationsOf func() int
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
	f := &fixture{pool: pool, q: q, now: at(8, 0)}
	newUser := func(name string) db.User {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: name + "-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	f.owner, f.emre, f.fatma = newUser("owner"), newUser("emre"), newUser("fatma")
	other := newUser("other")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	f.org, err = orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Reservations " + suffix, OwnerUserID: f.owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	f.org2, err = orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Reservations other " + suffix, OwnerUserID: other.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{f.org.ID, f.org2.ID})
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{f.owner.ID, f.emre.ID, f.fatma.ID, other.ID})
	})
	for _, u := range []db.User{f.emre, f.fatma} {
		if _, err := orgSvc.AddMember(ctx, f.org.UUID, u.Uuid, "member"); err != nil {
			t.Fatal(err)
		}
	}
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
	f.reservationsOf = func() int {
		return f.count("SELECT count(*) FROM reservations WHERE organization_id = $1", f.org.ID)
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
	ayseID, _ := idOf(`INSERT INTO staff (organization_id, name, user_id) VALUES ($1, 'Ayşe', $2) RETURNING id, uuid`, f.org.ID, f.emre.ID)
	f.exec(`INSERT INTO staff_branches (organization_id, staff_id, branch_id, shifts) VALUES
		($1, $2, $3, '{"mon":[{"start":"09:00","end":"12:00","breaks":[]}]}')`, f.org.ID, ayseID, aID)
	f.cutID, f.cut = idOf(`INSERT INTO services (organization_id, name, duration_minutes, price, payment_mode)
		VALUES ($1, 'Kesim', 30, 100, 'both') RETURNING id, uuid`, f.org.ID)
	f.exec(`INSERT INTO service_branches (organization_id, service_id, branch_id, price) VALUES ($1, $2, $3, 120)`, f.org.ID, f.cutID, aID)
	f.exec(`INSERT INTO service_staff (organization_id, service_id, staff_id, price) VALUES ($1, $2, $3, 150)`, f.org.ID, f.cutID, ayseID)
	washID, wash := idOf(`INSERT INTO services (organization_id, name, duration_minutes, buffer_after_minutes, price, payment_mode)
		VALUES ($1, 'Yıkama', 30, 10, 80, 'on_site') RETURNING id, uuid`, f.org.ID)
	f.wash = wash
	f.exec(`INSERT INTO service_branches (organization_id, service_id, branch_id) VALUES ($1, $2, $3)`, f.org.ID, washID, aID)
	f.exec(`INSERT INTO service_staff (organization_id, service_id, staff_id) VALUES ($1, $2, $3)`, f.org.ID, washID, ayseID)
	_, f.customer = idOf(`INSERT INTO customers (organization_id, first_name, last_name) VALUES ($1, 'Zeynep', 'Kaya') RETURNING id, uuid`, f.org.ID)
	_, f.foreignCust = idOf(`INSERT INTO customers (organization_id, first_name) VALUES ($1, 'Yabancı') RETURNING id, uuid`, f.org2.ID)
	f.settings = orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))
	f.holds = availabilityusecase.NewHolds(pool, q, f.settings)
	f.holds.SetClock(func() time.Time { return f.now })
	f.holds.SetExpiryHook(usecase.ExpireRequests)
	f.svc = usecase.New(f.holds, q, f.settings)
	return f
}

func (f *fixture) input(start time.Time, source, payment string, services ...uuid.UUID) model.CreateInput {
	in := model.CreateInput{BranchUUID: f.branch, CustomerUUID: f.customer, StartsAt: &start, Source: source, Payment: payment}
	for _, s := range services {
		in.Lines = append(in.Lines, availabilitymodel.LineInput{ServiceUUID: s})
	}
	return in
}

// saveSettings sets the online rules: approval mode, review window 30,
// payment hold 15, both payment methods.
func (f *fixture) saveSettings(t *testing.T, approval string) {
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

func (f *fixture) hold(t *testing.T, r model.Reservation) (kind, status, reason string) {
	t.Helper()
	var rs *string
	err := f.pool.QueryRow(context.Background(), `SELECT h.kind, h.status, h.reason FROM reservations r
		JOIN capacity_holds h ON h.organization_id = r.organization_id AND h.id = r.hold_id
		WHERE r.organization_id = $1 AND r.uuid = $2`, f.org.ID, r.UUID).Scan(&kind, &status, &rs)
	if err != nil {
		t.Fatal(err)
	}
	if rs != nil {
		reason = *rs
	}
	return kind, status, reason
}

func check(t *testing.T, step string, r model.Reservation, status, approval, payment string) {
	t.Helper()
	if r.Status != status || r.ApprovalStatus != approval || r.PaymentStatus != payment {
		t.Fatalf("%s: status %s / approval %s / payment %s, want %s / %s / %s", step, r.Status, r.ApprovalStatus, r.PaymentStatus, status, approval, payment)
	}
}

// TestPanelReservation: a panel booking is active at once and consumes the
// capacity once; lines keep the price and times of the moment; the
// lifecycle moves through in service and completed; versions, history,
// stale versions, cancelling frees the time.
func TestPanelReservation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	in := f.input(at(9, 0), "", "", f.cut, f.wash)
	in.CustomFields = map[string]any{"allergy": "yok"}
	in.InternalNote = "VIP"
	r, err := f.svc.Create(ctx, f.org.ID, in, none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	check(t, "panel", r, model.StatusActive, model.ApprovalAccepted, model.PaymentOnSite)
	if r.Number != 1 || r.Source != model.SourcePanel || r.ApprovalMode != model.ApprovalModePanel || r.Version != 1 || r.ConfirmationStatus != model.ConfirmationNotRequired {
		t.Fatalf("record = %+v", r)
	}
	if r.Customer.Name != "Zeynep Kaya" || r.Currency != "TRY" || r.CustomFields["allergy"] != "yok" || r.HoldExpiresAt != nil {
		t.Fatalf("record = %+v", r)
	}
	// Ayşe's personal price for the cut, the list price for the wash.
	if len(r.Lines) != 2 || !r.Lines[0].UnitPrice.Equal(money.MustParse("150")) || !r.Lines[1].UnitPrice.Equal(money.MustParse("80")) ||
		!r.TotalPrice.Equal(money.MustParse("230")) {
		t.Fatalf("prices = %+v, total %s", r.Lines, r.TotalPrice)
	}
	l := r.Lines[1]
	if l.ServiceName != "Yıkama" || l.Staff == nil || l.Staff.Name != "Ayşe" || l.DurationMinutes != 30 || l.BufferAfterMinutes != 10 ||
		!l.StartsAt.Equal(at(9, 30)) || !r.EndsAt.Equal(at(10, 0)) {
		t.Fatalf("line = %+v", l)
	}
	if kind, status, _ := f.hold(t, r); kind != availabilitymodel.KindBooking || status != availabilitymodel.StatusActive {
		t.Fatalf("hold = %s %s", kind, status)
	}
	// The record names its commitment and the commitment its record.
	if f.count(`SELECT count(*) FROM capacity_holds WHERE organization_id = $1 AND ref_type = 'reservation' AND ref_uuid = $2`, f.org.ID, r.UUID) != 1 {
		t.Fatal("hold does not name the reservation")
	}

	// The time is taken; nothing is written for the loser.
	if _, err := f.svc.Create(ctx, f.org.ID, f.input(at(9, 0), "", "", f.cut), none, f.owner.ID); !errors.Is(err, availabilityusecase.ErrSlotTaken) {
		t.Fatalf("double booking = %v", err)
	}
	if n := f.reservationsOf(); n != 1 {
		t.Fatalf("reservations = %d", n)
	}
	if n := f.count("SELECT count(*) FROM capacity_holds WHERE organization_id = $1", f.org.ID); n != 1 {
		t.Fatalf("holds = %d", n)
	}

	// Later catalog changes do not rewrite the booking.
	f.exec(`UPDATE services SET name = 'Saç kesimi', price = 300 WHERE id = $1`, f.cutID)
	f.exec(`UPDATE service_staff SET price = 400 WHERE organization_id = $1 AND service_id = $2`, f.org.ID, f.cutID)
	got, err := f.svc.Get(ctx, f.org.ID, r.UUID, none)
	if err != nil || got.Lines[0].ServiceName != "Kesim" || !got.Lines[0].UnitPrice.Equal(money.MustParse("150")) {
		t.Fatalf("snapshot = %+v, %v", got.Lines, err)
	}

	stale := 5
	if _, err := f.svc.Start(ctx, f.org.ID, r.UUID, none, f.owner.ID, usecase.ChangeInput{ExpectedVersion: &stale}); !errors.Is(err, usecase.ErrStaleVersion) {
		t.Fatalf("stale = %v", err)
	}
	v := 1
	r, err = f.svc.Start(ctx, f.org.ID, r.UUID, none, f.owner.ID, usecase.ChangeInput{ExpectedVersion: &v})
	if err != nil || r.Status != model.StatusInService || r.Version != 2 {
		t.Fatalf("start = %+v, %v", r, err)
	}
	if _, err := f.svc.Cancel(ctx, f.org.ID, r.UUID, none, f.owner.ID, usecase.ChangeInput{}); !errors.Is(err, usecase.ErrInvalidTransition) {
		t.Fatalf("cancel in service = %v", err)
	}
	if _, err := f.svc.NoShow(ctx, f.org.ID, r.UUID, none, f.owner.ID, usecase.ChangeInput{}); !errors.Is(err, usecase.ErrInvalidTransition) {
		t.Fatalf("no-show in service = %v", err)
	}
	r, err = f.svc.Complete(ctx, f.org.ID, r.UUID, none, f.owner.ID, usecase.ChangeInput{Note: "memnun"})
	if err != nil || r.Status != model.StatusCompleted || r.Version != 3 {
		t.Fatalf("complete = %+v, %v", r, err)
	}
	if len(r.Events) != 3 || r.Events[0].Type != "created" || r.Events[2].Type != "completed" || r.Events[2].Note != "memnun" ||
		*r.Events[2].FromStatus != model.StatusInService || r.Events[2].Version != 3 || r.Events[0].Actor == nil || *r.Events[0].Actor != "owner Test" {
		t.Fatalf("events = %+v", r.Events)
	}
	// A finished visit keeps its time (history).
	if kind, status, _ := f.hold(t, r); kind != availabilitymodel.KindBooking || status != availabilitymodel.StatusActive {
		t.Fatalf("completed hold = %s %s", kind, status)
	}

	// Cancelling frees the time; the next number follows.
	second, err := f.svc.Create(ctx, f.org.ID, f.input(at(10, 30), "", "", f.cut), none, f.owner.ID)
	if err != nil || second.Number != 2 {
		t.Fatalf("second = %+v, %v", second, err)
	}
	second, err = f.svc.Cancel(ctx, f.org.ID, second.UUID, none, f.owner.ID, usecase.ChangeInput{Reason: "customer_request"})
	if err != nil {
		t.Fatal(err)
	}
	check(t, "cancelled", second, model.StatusCancelled, model.ApprovalAccepted, model.PaymentOnSite)
	if second.CancelReason == nil || *second.CancelReason != "customer_request" {
		t.Fatalf("cancel reason = %v", second.CancelReason)
	}
	if _, status, _ := f.hold(t, second); status != availabilitymodel.StatusReleased {
		t.Fatalf("cancelled hold = %s", status)
	}
	if _, err := f.svc.Cancel(ctx, f.org.ID, second.UUID, none, f.owner.ID, usecase.ChangeInput{}); !errors.Is(err, usecase.ErrInvalidTransition) {
		t.Fatalf("cancel twice = %v", err)
	}
	third, err := f.svc.Create(ctx, f.org.ID, f.input(at(10, 30), "", "", f.cut), none, f.owner.ID)
	if err != nil {
		t.Fatalf("time not freed: %v", err)
	}
	if third, err = f.svc.NoShow(ctx, f.org.ID, third.UUID, none, f.owner.ID, usecase.ChangeInput{}); err != nil || third.Status != model.StatusNoShow {
		t.Fatalf("no-show = %+v, %v", third, err)
	}

	// Validation: customer of another business, missing customer, panel
	// asking for online payment.
	bad := f.input(at(11, 0), "", "", f.cut)
	bad.CustomerUUID = f.foreignCust
	var ve *usecase.ValidationError
	if _, err := f.svc.Create(ctx, f.org.ID, bad, none, f.owner.ID); !errors.As(err, &ve) || ve.Details[0].Field != "customer_uuid" {
		t.Fatalf("foreign customer = %v", err)
	}
	bad.CustomerUUID = uuid.Nil
	if _, err := f.svc.Create(ctx, f.org.ID, bad, none, f.owner.ID); !errors.As(err, &ve) {
		t.Fatalf("no customer = %v", err)
	}
	if _, err := f.svc.Create(ctx, f.org.ID, f.input(at(11, 0), "", model.MethodOnline, f.cut), none, f.owner.ID); !errors.As(err, &ve) || ve.Details[0].Field != "payment" {
		t.Fatalf("panel online payment = %v", err)
	}
	if n := f.count("SELECT count(*) FROM capacity_holds WHERE organization_id = $1 AND status = 'active'", f.org.ID); n != 2 {
		t.Fatalf("active holds after failures = %d", n)
	}
}

// TestOnlineFlows: requests under manual approval (accept on site, accept
// then pay online, reject), automatic approval with online payment, and the
// timeouts that cancel a request when its hold lapses.
func TestOnlineFlows(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.Create(ctx, f.org.ID, f.input(at(9, 0), model.SourceOnline, "", f.cut), none, 0); !errors.Is(err, usecase.ErrOnlineNotReady) {
		t.Fatalf("online without settings = %v", err)
	}
	f.saveSettings(t, orgsettingsmodel.ApprovalStaff)

	// Request paid on site: held for the review window, then booked.
	r, err := f.svc.Create(ctx, f.org.ID, f.input(at(9, 0), model.SourceOnline, model.MethodOnSite, f.cut), none, 0)
	if err != nil {
		t.Fatal(err)
	}
	check(t, "request", r, model.StatusRequested, model.ApprovalPending, model.PaymentOnSite)
	if r.ApprovalMode != model.ApprovalModeStaff || r.HoldExpiresAt == nil || !r.HoldExpiresAt.Equal(at(8, 30)) {
		t.Fatalf("request = %+v", r)
	}
	if _, err := f.svc.Start(ctx, f.org.ID, r.UUID, none, f.owner.ID, usecase.ChangeInput{}); !errors.Is(err, usecase.ErrInvalidTransition) {
		t.Fatalf("start a request = %v", err)
	}
	r, err = f.svc.Accept(ctx, f.org.ID, r.UUID, none, f.owner.ID, usecase.ChangeInput{})
	if err != nil {
		t.Fatal(err)
	}
	check(t, "accepted", r, model.StatusActive, model.ApprovalAccepted, model.PaymentOnSite)
	if kind, _, _ := f.hold(t, r); kind != availabilitymodel.KindBooking || r.HoldExpiresAt != nil {
		t.Fatalf("accepted hold = %s, expires %v", kind, r.HoldExpiresAt)
	}
	if _, err := f.svc.Accept(ctx, f.org.ID, r.UUID, none, f.owner.ID, usecase.ChangeInput{}); !errors.Is(err, usecase.ErrInvalidTransition) {
		t.Fatalf("accept twice = %v", err)
	}

	// Request with online payment: accepting asks for the payment (the hold
	// now waits for it), the payment books it.
	r, err = f.svc.Create(ctx, f.org.ID, f.input(at(9, 30), model.SourceEmbed, model.MethodOnline, f.cut), none, 0)
	if err != nil {
		t.Fatal(err)
	}
	check(t, "online request", r, model.StatusRequested, model.ApprovalPending, model.PaymentOnlinePending)
	// Payment is asked for only after the business accepts.
	if _, err := f.svc.MarkPaid(ctx, f.org.ID, r.UUID, 0, usecase.ChangeInput{}); !errors.Is(err, usecase.ErrInvalidTransition) {
		t.Fatalf("payment before acceptance = %v", err)
	}
	f.now = at(8, 5)
	r, err = f.svc.Accept(ctx, f.org.ID, r.UUID, none, f.owner.ID, usecase.ChangeInput{})
	if err != nil {
		t.Fatal(err)
	}
	check(t, "accepted, unpaid", r, model.StatusRequested, model.ApprovalAccepted, model.PaymentOnlinePending)
	if kind, _, reason := f.hold(t, r); kind != availabilitymodel.KindHold || reason != availabilitymodel.ReasonPayment || !r.HoldExpiresAt.Equal(at(8, 20)) {
		t.Fatalf("payment hold = %s %s %v", kind, reason, r.HoldExpiresAt)
	}
	r, err = f.svc.MarkPaid(ctx, f.org.ID, r.UUID, 0, usecase.ChangeInput{})
	if err != nil {
		t.Fatal(err)
	}
	check(t, "paid", r, model.StatusActive, model.ApprovalAccepted, model.PaymentPaid)
	// Cancelling a paid booking leaves a refund to the payment flow.
	r, err = f.svc.Cancel(ctx, f.org.ID, r.UUID, none, f.owner.ID, usecase.ChangeInput{})
	if err != nil {
		t.Fatal(err)
	}
	check(t, "paid cancelled", r, model.StatusCancelled, model.ApprovalAccepted, model.PaymentRefundPending)

	// Rejecting frees the time at once.
	r, err = f.svc.Create(ctx, f.org.ID, f.input(at(10, 0), model.SourceOnline, "", f.cut), none, 0)
	if err != nil {
		t.Fatal(err)
	}
	r, err = f.svc.Reject(ctx, f.org.ID, r.UUID, none, f.owner.ID, usecase.ChangeInput{Note: "dolu"})
	if err != nil {
		t.Fatal(err)
	}
	check(t, "rejected", r, model.StatusRejected, model.ApprovalRejected, model.PaymentOnSite)
	if _, status, _ := f.hold(t, r); status != availabilitymodel.StatusReleased {
		t.Fatalf("rejected hold = %s", status)
	}

	// Nobody decides within 30 minutes: the next writer expires the hold
	// and cancels the request in the same transaction.
	waiting, err := f.svc.Create(ctx, f.org.ID, f.input(at(10, 0), model.SourceOnline, "", f.cut), none, 0)
	if err != nil {
		t.Fatalf("rejected time not freed: %v", err)
	}
	f.now = at(8, 36)
	if _, err := f.svc.Accept(ctx, f.org.ID, waiting.UUID, none, f.owner.ID, usecase.ChangeInput{}); !errors.Is(err, usecase.ErrInvalidTransition) {
		t.Fatalf("accept after the window = %v", err)
	}
	waiting, _ = f.svc.Get(ctx, f.org.ID, waiting.UUID, none)
	check(t, "approval timeout", waiting, model.StatusCancelled, model.ApprovalPending, model.PaymentOnSite)
	if waiting.CancelReason == nil || *waiting.CancelReason != model.ReasonApprovalTimeout || waiting.Events[len(waiting.Events)-1].Type != "expired" {
		t.Fatalf("approval timeout = %+v", waiting)
	}

	// Automatic approval with online payment: accepted, waiting for the
	// payment; unpaid after 15 minutes, the sweep cancels it.
	f.saveSettings(t, orgsettingsmodel.ApprovalAuto)
	r, err = f.svc.Create(ctx, f.org.ID, f.input(at(11, 0), model.SourceOnline, model.MethodOnline, f.cut), none, 0)
	if err != nil {
		t.Fatal(err)
	}
	check(t, "auto online", r, model.StatusRequested, model.ApprovalAccepted, model.PaymentOnlinePending)
	if r.ApprovalMode != model.ApprovalModeAuto {
		t.Fatalf("approval mode = %s", r.ApprovalMode)
	}
	f.now = at(8, 52)
	// The sweep covers every business (other tests may leave lapsed holds).
	if n, err := f.holds.ExpireDue(ctx); err != nil || n < 1 {
		t.Fatalf("sweep = %d, %v", n, err)
	}
	r, _ = f.svc.Get(ctx, f.org.ID, r.UUID, none)
	check(t, "payment timeout", r, model.StatusCancelled, model.ApprovalAccepted, model.PaymentFailed)
	if *r.CancelReason != model.ReasonPaymentTimeout {
		t.Fatalf("cancel reason = %s", *r.CancelReason)
	}
	if _, err := f.svc.MarkPaid(ctx, f.org.ID, r.UUID, 0, usecase.ChangeInput{}); !errors.Is(err, usecase.ErrInvalidTransition) {
		t.Fatalf("late payment = %v", err)
	}

	// Automatic approval paid on site: active at once (the time the unpaid
	// booking let go).
	r, err = f.svc.Create(ctx, f.org.ID, f.input(at(11, 0), model.SourceOnline, "", f.wash), none, 0)
	if err != nil {
		t.Fatal(err)
	}
	check(t, "auto on site", r, model.StatusActive, model.ApprovalAccepted, model.PaymentOnSite)
	// The wash is paid on site only.
	var ve *usecase.ValidationError
	if _, err := f.svc.Create(ctx, f.org.ID, f.input(at(10, 0), model.SourceOnline, model.MethodOnline, f.wash), none, 0); !errors.As(err, &ve) || ve.Details[0].Code != "not_allowed" {
		t.Fatalf("online payment for an on-site service = %v", err)
	}
	if _, err := f.svc.Create(ctx, f.org.ID, f.input(at(10, 0), model.SourceOnline, "card", f.cut), none, 0); !errors.As(err, &ve) {
		t.Fatalf("unknown payment = %v", err)
	}
}

// TestVisibilityAndIsolation: an own calendar member sees the bookings it
// serves or created; another business's uuid is not found.
func TestVisibilityAndIsolation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	r, err := f.svc.Create(ctx, f.org.ID, f.input(at(9, 0), "", "", f.cut), none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Get(ctx, f.org.ID, r.UUID, availabilityusecase.Caller{OwnUserID: f.emre.ID}); err != nil {
		t.Fatalf("served by emre = %v", err)
	}
	if _, err := f.svc.Get(ctx, f.org.ID, r.UUID, availabilityusecase.Caller{OwnUserID: f.fatma.ID}); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("fatma = %v", err)
	}
	if _, err := f.svc.Cancel(ctx, f.org.ID, r.UUID, availabilityusecase.Caller{OwnUserID: f.fatma.ID}, f.fatma.ID, usecase.ChangeInput{}); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("fatma cancels = %v", err)
	}
	if _, err := f.svc.Get(ctx, f.org2.ID, r.UUID, none); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("other business = %v", err)
	}
	if _, err := f.svc.Cancel(ctx, f.org2.ID, r.UUID, none, f.owner.ID, usecase.ChangeInput{}); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("other business cancels = %v", err)
	}
	if _, err := f.svc.Create(ctx, f.org2.ID, f.input(at(9, 30), "", "", f.cut), none, f.owner.ID); err == nil {
		t.Fatal("another business booked our branch")
	}
}

// TestLastSlotRecords: parallel bookings of the last free start leave one
// reservation and one commitment (A03 at the record level).
func TestLastSlotRecords(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.exec(`UPDATE staff_branches SET shifts = '{"mon":[{"start":"09:00","end":"09:30","breaks":[]}]}' WHERE organization_id = $1`, f.org.ID)
	const writers = 8
	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = f.svc.Create(ctx, f.org.ID, f.input(at(9, 0), "", "", f.cut), none, f.owner.ID)
		}(i)
	}
	wg.Wait()
	won := 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case !errors.Is(err, availabilityusecase.ErrSlotTaken):
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if won != 1 || f.reservationsOf() != 1 || f.count("SELECT count(*) FROM capacity_holds WHERE organization_id = $1", f.org.ID) != 1 {
		t.Fatalf("won = %d, reservations = %d", won, f.reservationsOf())
	}
}
