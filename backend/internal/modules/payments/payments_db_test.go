package payments_test

import (
	"context"
	"errors"
	"net/http"
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
	cashrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/repository"
	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	integrationsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	orgsettingsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	salesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/repository"
	salesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/testpay"
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

// TestPaymentsFlow is A13 / A14 end to end on Postgres: an online full
// payment recorded only from the provider's signed notification (a forged
// one changes nothing, the same notification three times is one
// collection), a failed payment, a payment after the payment window
// (the visit is not reopened, refund pending) and money taken at the
// business in parts. Runs when TEST_DATABASE_URL points at a migrated
// database.
func TestPaymentsFlow(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := db.New(pool)
	suffix := uuid.NewString()[:8]

	owner, err := q.CreateUser(ctx, db.CreateUserParams{
		Email: "pay-owner-" + suffix + "@example.test", PasswordHash: "x", Name: "Owner", Surname: "Test", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Payments " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM integration_jobs WHERE organization_id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM activity_events WHERE organization_id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", owner.ID)
	})
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	count := func(sql string, args ...any) int {
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

	branchID, branchUUID := idOf("SELECT id, uuid FROM branches WHERE organization_id = $1 ORDER BY id LIMIT 1", org.ID)
	exec(`UPDATE branches SET working_hours = $3, holiday_exceptions = '[]', timezone = NULL WHERE organization_id = $1 AND id = $2`,
		org.ID, branchID, `{"mon":{"closed":false,"open":"09:00","close":"18:00"}}`)
	ayseID, _ := idOf(`INSERT INTO staff (organization_id, name) VALUES ($1, 'Ayşe') RETURNING id, uuid`, org.ID)
	exec(`INSERT INTO staff_branches (organization_id, staff_id, branch_id, shifts) VALUES
		($1, $2, $3, '{"mon":[{"start":"09:00","end":"18:00","breaks":[]}]}')`, org.ID, ayseID, branchID)
	cutID, cutUUID := idOf(`INSERT INTO services (organization_id, name, duration_minutes, price, payment_mode, show_online)
		VALUES ($1, 'Kesim', 30, 100, 'both', true) RETURNING id, uuid`, org.ID)
	exec(`INSERT INTO service_branches (organization_id, service_id, branch_id) VALUES ($1, $2, $3)`, org.ID, cutID, branchID)
	exec(`INSERT INTO service_staff (organization_id, service_id, staff_id) VALUES ($1, $2, $3)`, org.ID, cutID, ayseID)
	_, customerUUID := idOf(`INSERT INTO customers (organization_id, first_name, last_name) VALUES ($1, 'Zeynep', 'Kaya') RETURNING id, uuid`, org.ID)
	_, tillUUID := idOf(`INSERT INTO cash_accounts (organization_id, branch_id, kind, name, currency, opening_date)
		VALUES ($1, $2, 'cash', 'Kasa', 'TRY', '2026-01-01') RETURNING id, uuid`, org.ID, branchID)
	_, bankUUID := idOf(`INSERT INTO cash_accounts (organization_id, branch_id, kind, name, currency, opening_date)
		VALUES ($1, NULL, 'bank', 'Banka', 'TRY', '2026-01-01') RETURNING id, uuid`, org.ID)

	settings := orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))
	lead, advance, window, hold := 0, 30, 30, 15
	s := orgsettingsmodel.Defaults()
	s.Reservation = orgsettingsmodel.Reservation{
		ApprovalMode: orgsettingsmodel.ApprovalAuto, StaffReviewWindowMinutes: &window, PaymentHoldMinutes: &hold,
		MinLeadMinutes: &lead, MaxAdvanceDays: &advance,
	}
	s.Payment = orgsettingsmodel.Payment{PayAtVenue: true, PayOnline: true}
	if _, err := settings.Save(ctx, org.ID, owner.ID, orgsettingsmodel.SaveInput{Settings: s}); err != nil {
		t.Fatal(err)
	}

	holdNow := at(8, 0)
	var clockMu sync.Mutex
	holdClock := func() time.Time { clockMu.Lock(); defer clockMu.Unlock(); return holdNow }
	setNow := func(v time.Time) { clockMu.Lock(); holdNow = v; clockMu.Unlock() }
	cashSvc := cashusecase.New(cashrepository.NewPostgres(pool, q), nil)
	cashSvc.SetClock(holdClock)
	salesSvc := salesusecase.New(salesrepository.NewPostgres(pool, q), cashSvc)
	salesSvc.SetClock(holdClock)
	holds := availabilityusecase.NewHolds(pool, q, settings)
	holds.SetClock(holdClock)
	holds.SetExpiryHook(reservationsusecase.ExpireRequests)
	resSvc := reservationsusecase.New(holds, q, settings)

	cipher, err := crypto.NewFieldCipher(strings.Repeat("p", 32))
	if err != nil {
		t.Fatal(err)
	}
	kit, err := integrationsmodule.New(q, cipher, "production", nil, testpay.New("http://app.test"))
	if err != nil {
		t.Fatal(err)
	}
	svc := usecase.New(q, resSvc, salesSvc, kit.Accounts, kit.Runner, usecase.Config{FrontendURL: "http://app.test/"}, nil)
	svc.SetClock(holdClock)
	none := availabilityusecase.Caller{}

	online := func(hour int) reservationsmodel.Reservation {
		t.Helper()
		start := at(hour, 0)
		r, err := resSvc.Create(ctx, org.ID, reservationsmodel.CreateInput{
			BranchUUID: branchUUID, CustomerUUID: customerUUID, StartsAt: &start, Source: reservationsmodel.SourceOnline,
			Payment: reservationsmodel.MethodOnline, Lines: []availabilitymodel.LineInput{{ServiceUUID: cutUUID}},
		}, none, 0)
		if err != nil {
			t.Fatal(err)
		}
		if r.Status != reservationsmodel.StatusRequested || r.PaymentStatus != reservationsmodel.PaymentOnlinePending {
			t.Fatalf("online booking = %s / %s", r.Status, r.PaymentStatus)
		}
		return r
	}
	start := func(r reservationsmodel.Reservation) model.OnlineStart {
		t.Helper()
		out, err := svc.StartOnline(ctx, usecase.StartInput{OrganizationID: org.ID, Slug: org.Slug, ReservationUUID: r.UUID, IP: "10.0.0.1"})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	refOf := func(p uuid.UUID) string {
		t.Helper()
		var ref string
		if err := pool.QueryRow(ctx, "SELECT order_ref FROM payments WHERE organization_id = $1 AND uuid = $2", org.ID, p).Scan(&ref); err != nil {
			t.Fatal(err)
		}
		return ref
	}
	get := func(id uuid.UUID) reservationsmodel.Reservation {
		t.Helper()
		r, err := resSvc.Get(ctx, org.ID, id, none)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}

	// Not ready without a provider account and an online cash account.
	r1 := online(10)
	if svc.OnlineReady(ctx, org.ID) {
		t.Fatal("ready without a provider")
	}
	if _, err := svc.StartOnline(ctx, usecase.StartInput{OrganizationID: org.ID, Slug: org.Slug, ReservationUUID: r1.UUID}); !errors.Is(err, usecase.ErrNotReady) {
		t.Fatalf("start without a provider = %v", err)
	}
	if _, err := kit.Accounts.Connect(ctx, org.ID, accountsusecase.ConnectInput{
		Kind: integrations.KindPayment, Provider: testpay.ProviderCode, DisplayName: "Test", Mode: integrations.ModeSandbox,
		Credentials: map[string]string{"secret_key": "s3cret-" + suffix},
	}); err != nil {
		t.Fatal(err)
	}
	if svc.OnlineReady(ctx, org.ID) {
		t.Fatal("ready without an online cash account")
	}
	_, onlineAccUUID := idOf(`INSERT INTO cash_accounts (organization_id, branch_id, kind, name, currency, opening_date, provider)
		VALUES ($1, NULL, 'online', 'Online tahsilat', 'TRY', '2026-01-01', 'testpay') RETURNING id, uuid`, org.ID)
	if !svc.OnlineReady(ctx, org.ID) {
		t.Fatal("not ready with provider and online account")
	}

	// The page; asking again reopens the same attempt.
	setNow(at(8, 1))
	p1 := start(r1)
	if !strings.HasPrefix(p1.RedirectURL, "http://app.test/pay/test/") || !p1.Amount.Equal(money.FromUnits(100)) {
		t.Fatalf("start = %+v", p1)
	}
	if again := start(r1); again.PaymentUUID != p1.PaymentUUID {
		t.Fatalf("second start opened another attempt")
	}
	ref1 := refOf(p1.PaymentUUID)

	// A forged notification (another secret) is refused and changes nothing.
	forged := integrations.Account{Credentials: integrations.Credentials{"secret_key": "attacker"}}
	h, body, _ := testpay.Notify(forged, ref1, payments.StatusSucceeded, money.FromUnits(100), "TRY")
	if _, err := svc.HandleNotification(ctx, testpay.ProviderCode, h, body); !errors.Is(err, usecase.ErrSignature) {
		t.Fatalf("forged = %v", err)
	}
	// A notification for an unknown reference.
	h, body, _ = testpay.Notify(forged, "nope", payments.StatusSucceeded, money.FromUnits(100), "TRY")
	if _, err := svc.HandleNotification(ctx, testpay.ProviderCode, h, body); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("unknown ref = %v", err)
	}
	if _, err := svc.HandleNotification(ctx, testpay.ProviderCode, http.Header{}, []byte("{")); !errors.Is(err, usecase.ErrMalformed) {
		t.Fatalf("malformed = %v", err)
	}
	if n := count("SELECT COUNT(*) FROM ledger_entries WHERE organization_id = $1", org.ID); n != 0 {
		t.Fatalf("forged notification wrote %d entries", n)
	}
	if get(r1.UUID).PaymentStatus != reservationsmodel.PaymentOnlinePending {
		t.Fatal("forged notification changed the reservation")
	}

	// The same verified notification three times, two of them at once:
	// one collection, one confirmation, one job with three deliveries.
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.TestComplete(ctx, ref1, payments.StatusSucceeded)
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	n3, err := svc.TestComplete(ctx, ref1, payments.StatusSucceeded)
	if err != nil || !n3.Result.Duplicate {
		t.Fatalf("third delivery = %+v, %v", n3.Result, err)
	}
	r1 = get(r1.UUID)
	if r1.Status != reservationsmodel.StatusActive || r1.PaymentStatus != reservationsmodel.PaymentPaid ||
		r1.ConfirmationStatus != "not_required" {
		t.Fatalf("paid reservation = %s / %s / %s", r1.Status, r1.PaymentStatus, r1.ConfirmationStatus)
	}
	if n := count(`SELECT COUNT(*) FROM ledger_entries WHERE organization_id = $1 AND kind = 'collection' AND method = 'online'
		AND source_type = 'payment' AND source_uuid = $2`, org.ID, p1.PaymentUUID); n != 1 {
		t.Fatalf("collections = %d, want 1", n)
	}
	if n := count(`SELECT COUNT(*) FROM cash_movements m JOIN cash_accounts a ON a.organization_id = m.organization_id AND a.id = m.cash_account_id
		WHERE m.organization_id = $1 AND a.uuid = $2 AND m.kind = 'collection' AND m.amount = 100`, org.ID, onlineAccUUID); n != 1 {
		t.Fatalf("online account movements = %d, want 1", n)
	}
	if n := count(`SELECT deliveries FROM integration_jobs WHERE organization_id = $1 AND ref_uuid = $2`, org.ID, p1.PaymentUUID); n != 3 {
		t.Fatalf("deliveries = %d, want 3", n)
	}
	if n := count(`SELECT COUNT(*) FROM reservation_events e JOIN reservations r ON r.organization_id = e.organization_id AND r.id = e.reservation_id
		WHERE e.organization_id = $1 AND r.uuid = $2 AND e.type = 'paid'`, org.ID, r1.UUID); n != 1 {
		t.Fatalf("paid events = %d, want 1", n)
	}
	if _, status, _ := holdOf(t, pool, org.ID, r1.UUID); status != availabilitymodel.StatusActive {
		t.Fatalf("paid hold = %s", status)
	}
	view, err := svc.ReservationPayments(ctx, org.ID, r1.UUID, none, usecase.ViewOptions{Collect: true})
	if err != nil {
		t.Fatal(err)
	}
	if !view.Paid.Equal(money.FromUnits(100)) || view.Remaining.IsPositive() || view.CanCollect || len(view.Online) != 1 ||
		view.Online[0].Status != model.StatusSucceeded || view.Online[0].Outcome != reservationsusecase.PaymentConfirmed || len(view.Entries) != 1 {
		t.Fatalf("view = %+v", view)
	}
	// A paid booking is not offered again.
	if _, err := svc.StartOnline(ctx, usecase.StartInput{OrganizationID: org.ID, Slug: org.Slug, ReservationUUID: r1.UUID}); !errors.Is(err, usecase.ErrNotPayable) {
		t.Fatalf("start paid = %v", err)
	}

	// A declined payment: the attempt fails, the booking still waits.
	r2 := online(11)
	p2 := start(r2)
	if _, err := svc.TestComplete(ctx, refOf(p2.PaymentUUID), payments.StatusFailed); err != nil {
		t.Fatal(err)
	}
	if got := get(r2.UUID); got.PaymentStatus != reservationsmodel.PaymentOnlinePending || got.Status != reservationsmodel.StatusRequested {
		t.Fatalf("declined = %s / %s", got.Status, got.PaymentStatus)
	}
	// The customer tries again: a new attempt.
	if p2b := start(r2); p2b.PaymentUUID == p2.PaymentUUID {
		t.Fatal("a failed attempt was reopened")
	}

	// A payment after the window: the request was cancelled, its time is
	// not taken back, the money is recorded and waits for its refund.
	r3 := online(12)
	p3 := start(r3)
	setNow(at(8, 40))
	if _, err := holds.ExpireDue(ctx); err != nil {
		t.Fatal(err)
	}
	if got := get(r3.UUID); got.Status != reservationsmodel.StatusCancelled {
		t.Fatalf("expired = %s", got.Status)
	}
	if _, err := svc.StartOnline(ctx, usecase.StartInput{OrganizationID: org.ID, Slug: org.Slug, ReservationUUID: r3.UUID}); !errors.Is(err, usecase.ErrNotPayable) {
		t.Fatalf("start after the window = %v", err)
	}
	if _, err := svc.TestComplete(ctx, refOf(p3.PaymentUUID), payments.StatusSucceeded); err != nil {
		t.Fatal(err)
	}
	r3 = get(r3.UUID)
	if r3.Status != reservationsmodel.StatusCancelled || r3.PaymentStatus != reservationsmodel.PaymentRefundPending {
		t.Fatalf("late payment = %s / %s", r3.Status, r3.PaymentStatus)
	}
	if n := count(`SELECT COUNT(*) FROM payments WHERE organization_id = $1 AND uuid = $2 AND status = 'succeeded' AND outcome = 'late'`,
		org.ID, p3.PaymentUUID); n != 1 {
		t.Fatal("late attempt not recorded as late")
	}
	if _, status, _ := holdOf(t, pool, org.ID, r3.UUID); status == availabilitymodel.StatusActive {
		t.Fatal("late payment took the time back")
	}

	// Money at the business, in parts.
	visitStart := at(14, 0)
	visit, err := resSvc.Create(ctx, org.ID, reservationsmodel.CreateInput{
		BranchUUID: branchUUID, CustomerUUID: customerUUID, StartsAt: &visitStart, Source: reservationsmodel.SourcePanel,
		Lines: []availabilitymodel.LineInput{{ServiceUUID: cutUUID}},
	}, none, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	view, err = svc.ReservationPayments(ctx, org.ID, visit.UUID, none, usecase.ViewOptions{Collect: true})
	if err != nil || !view.CanCollect || len(view.CollectAccounts) != 2 {
		t.Fatalf("collect view = %+v, %v", view, err)
	}
	collect := func(amount string, method string, account uuid.UUID) (model.ReservationPayments, error) {
		return svc.Collect(ctx, org.ID, visit.UUID, none, owner.ID, model.CollectInput{
			Amount: money.MustParse(amount), Method: method, CashAccountUUID: account,
		})
	}
	var ve *usecase.ValidationError
	if _, err := collect("40", "online", tillUUID); !errors.As(err, &ve) || ve.Details[0].Field != "method" {
		t.Fatalf("online method at the business = %v", err)
	}
	if _, err := collect("40", "cash", onlineAccUUID); !errors.As(err, &ve) || ve.Details[0].Field != "cash_account_uuid" {
		t.Fatalf("online account at the business = %v", err)
	}
	view, err = collect("40", "cash", tillUUID)
	if err != nil {
		t.Fatal(err)
	}
	if !view.Remaining.Equal(money.FromUnits(60)) || view.PaymentStatus != reservationsmodel.PaymentOnSite {
		t.Fatalf("after a part = %+v", view)
	}
	if _, err := collect("70", "card", bankUUID); !errors.As(err, &ve) || ve.Details[0].Code != "exceeds_remaining" {
		t.Fatalf("more than the rest = %v", err)
	}
	view, err = collect("60", "card", bankUUID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Remaining.IsPositive() || view.PaymentStatus != reservationsmodel.PaymentPaid || len(view.Entries) != 2 {
		t.Fatalf("paid at the business = %+v", view)
	}
	// Nothing is taken for a cancelled visit (r2 lapsed with r3).
	if got := get(r2.UUID); got.Status != reservationsmodel.StatusCancelled || got.PaymentStatus != reservationsmodel.PaymentFailed {
		t.Fatalf("unpaid lapsed = %s / %s", got.Status, got.PaymentStatus)
	}
	if _, err := svc.Collect(ctx, org.ID, r2.UUID, none, owner.ID, model.CollectInput{
		Amount: money.FromUnits(10), Method: "cash", CashAccountUUID: tillUUID,
	}); !errors.Is(err, usecase.ErrNotCollectable) {
		t.Fatalf("collect cancelled = %v", err)
	}
	// The test provider's page is unknown when the provider is not
	// registered (production).
	bare, err := integrationsmodule.New(q, cipher, "production", nil)
	if err != nil {
		t.Fatal(err)
	}
	prod := usecase.New(q, resSvc, salesSvc, bare.Accounts, bare.Runner, usecase.Config{}, nil)
	if _, err := prod.TestPage(ctx, ref1); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("test page in production = %v", err)
	}
}

func holdOf(t *testing.T, pool *pgxpool.Pool, orgID int64, id uuid.UUID) (kind, status string, err error) {
	t.Helper()
	err = pool.QueryRow(context.Background(), `SELECT h.kind, h.status FROM reservations r
		JOIN capacity_holds h ON h.organization_id = r.organization_id AND h.id = r.hold_id
		WHERE r.organization_id = $1 AND r.uuid = $2`, orgID, id).Scan(&kind, &status)
	if err != nil {
		t.Fatal(err)
	}
	return kind, status, nil
}
