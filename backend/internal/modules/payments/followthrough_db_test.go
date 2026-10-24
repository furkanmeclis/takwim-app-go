package payments_test

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
	cashrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/repository"
	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	customernotifycatalog "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/catalog"
	customernotifyusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/usecase"
	integrationsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	orgsettingsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	paymentsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments"
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

// TestRefundFollowThrough is TEC-978 on Postgres: a refund to make closed
// without paying it back (a reason, the history, off the follow-up list,
// back on it when a later refund fails), the refund notices to the
// customer (done / being processed, once per refund), and the worker's
// status queries of refunds whose provider result is unknown (found:
// posted; not done: failed; still pending: asked again later, further
// apart; never sent to the provider again).
func TestRefundFollowThrough(t *testing.T) {
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
		Email: "keep-owner-" + suffix + "@example.test", PasswordHash: "x", Name: "Owner", Surname: "Test", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Keep " + suffix, OwnerUserID: owner.ID})
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
	staffID, _ := idOf(`INSERT INTO staff (organization_id, name) VALUES ($1, 'Ayşe') RETURNING id, uuid`, org.ID)
	exec(`INSERT INTO staff_branches (organization_id, staff_id, branch_id, shifts) VALUES
		($1, $2, $3, '{"mon":[{"start":"09:00","end":"18:00","breaks":[]}]}')`, org.ID, staffID, branchID)
	cutID, cutUUID := idOf(`INSERT INTO services (organization_id, name, duration_minutes, price, payment_mode, show_online)
		VALUES ($1, 'Kesim', 30, 100, 'both', true) RETURNING id, uuid`, org.ID)
	exec(`INSERT INTO service_branches (organization_id, service_id, branch_id) VALUES ($1, $2, $3)`, org.ID, cutID, branchID)
	exec(`INSERT INTO service_staff (organization_id, service_id, staff_id) VALUES ($1, $2, $3)`, org.ID, cutID, staffID)
	_, customerUUID := idOf(`INSERT INTO customers (organization_id, first_name, last_name, email)
		VALUES ($1, 'Zeynep', 'Kaya', 'zeynep@example.test') RETURNING id, uuid`, org.ID)
	_, onlineAccUUID := idOf(`INSERT INTO cash_accounts (organization_id, branch_id, kind, name, currency, opening_date, provider)
		VALUES ($1, NULL, 'online', 'Online tahsilat', 'TRY', '2026-01-01', 'testpay') RETURNING id, uuid`, org.ID)

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

	now := at(8, 0)
	var clockMu sync.Mutex
	clock := func() time.Time { clockMu.Lock(); defer clockMu.Unlock(); return now }
	advanceClock := func(d time.Duration) { clockMu.Lock(); defer clockMu.Unlock(); now = now.Add(d) }
	cashSvc := cashusecase.New(cashrepository.NewPostgres(pool, q), nil)
	cashSvc.SetClock(clock)
	salesSvc := salesusecase.New(salesrepository.NewPostgres(pool, q), cashSvc)
	salesSvc.SetClock(clock)
	holds := availabilityusecase.NewHolds(pool, q, settings)
	holds.SetClock(clock)
	holds.SetExpiryHook(reservationsusecase.ExpireRequests)
	resSvc := reservationsusecase.New(holds, q, settings)
	resSvc.SetCompletionHook(salesSvc.RecordReservationTx)

	cipher, err := crypto.NewFieldCipher(strings.Repeat("k", 32))
	if err != nil {
		t.Fatal(err)
	}
	kit, err := integrationsmodule.New(q, cipher, "production", nil, testpay.New("http://app.test"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kit.Accounts.Connect(ctx, org.ID, accountsusecase.ConnectInput{
		Kind: integrations.KindPayment, Provider: testpay.ProviderCode, DisplayName: "Test", Mode: integrations.ModeSandbox,
		Credentials: map[string]string{"secret_key": "s3cret-" + suffix},
	}); err != nil {
		t.Fatal(err)
	}
	svc := usecase.New(q, resSvc, salesSvc, kit.Accounts, kit.Runner, usecase.Config{FrontendURL: "http://app.test/"}, nil)
	svc.SetClock(clock)
	engine := customernotifyusecase.New(pool, q, "http://app.test/panel")
	svc.SetRefundNotifier(paymentsmodule.RefundNotices(engine, q))
	none := availabilityusecase.Caller{}
	all := usecase.ViewOptions{Collect: true, Refund: true}

	view := func(id uuid.UUID) model.ReservationPayments {
		t.Helper()
		v, err := svc.ReservationPayments(ctx, org.ID, id, none, all)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	followUp := func(id uuid.UUID) (model.FollowUp, bool) {
		t.Helper()
		items, _, err := svc.ListFollowUps(ctx, org.ID, model.FollowUpFilter{Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range items {
			if f.UUID == id {
				return f, true
			}
		}
		return model.FollowUp{}, false
	}
	notices := func(event string) int {
		t.Helper()
		return count(`SELECT COUNT(*) FROM customer_notification_dispatches WHERE organization_id = $1 AND event_code = $2`, org.ID, event)
	}
	refundRow := func(id uuid.UUID) (status, provider string, checks int, next *time.Time) {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT status, provider_status, provider_checks, next_check_at FROM refunds
			WHERE organization_id = $1 AND uuid = $2`, org.ID, id).Scan(&status, &provider, &checks, &next); err != nil {
			t.Fatal(err)
		}
		return status, provider, checks, next
	}

	// A booking paid online in full (100), then cancelled: a refund to make.
	start := at(10, 0)
	r1, err := resSvc.Create(ctx, org.ID, reservationsmodel.CreateInput{
		BranchUUID: branchUUID, CustomerUUID: customerUUID, StartsAt: &start, Source: reservationsmodel.SourceOnline,
		Payment: reservationsmodel.MethodOnline, Lines: []availabilitymodel.LineInput{{ServiceUUID: cutUUID}},
	}, none, 0)
	if err != nil {
		t.Fatal(err)
	}
	page, err := svc.StartOnline(ctx, usecase.StartInput{OrganizationID: org.ID, Slug: org.Slug, ReservationUUID: r1.UUID})
	if err != nil {
		t.Fatal(err)
	}
	var ref string
	if err := pool.QueryRow(ctx, "SELECT order_ref FROM payments WHERE organization_id = $1 AND uuid = $2", org.ID, page.PaymentUUID).Scan(&ref); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.TestComplete(ctx, ref, payments.StatusSucceeded); err != nil {
		t.Fatal(err)
	}
	if _, err := resSvc.Cancel(ctx, org.ID, r1.UUID, none, owner.ID, reservationsusecase.ChangeInput{Reason: "business"}); err != nil {
		t.Fatal(err)
	}
	v := view(r1.UUID)
	if !v.CanClose || v.Kept != nil || v.PaymentStatus != reservationsmodel.PaymentRefundPending {
		t.Fatalf("before closing = can %v kept %+v status %s", v.CanClose, v.Kept, v.PaymentStatus)
	}
	col := v.RefundCollections[0].UUID

	// Closing needs a reason; then the money is kept, off the list.
	var ve *usecase.ValidationError
	if _, err := svc.CloseWithoutRefund(ctx, org.ID, r1.UUID, none, owner.ID, all, model.CloseInput{Reason: "  "}); !errors.As(err, &ve) ||
		ve.Details[0].Field != "reason" {
		t.Fatalf("no reason = %v", err)
	}
	closed, err := svc.CloseWithoutRefund(ctx, org.ID, r1.UUID, none, owner.ID, all, model.CloseInput{Reason: "bir sonraki ziyarete aktarıldı"})
	if err != nil {
		t.Fatal(err)
	}
	if closed.PaymentStatus != reservationsmodel.PaymentKept || closed.CanClose || closed.Kept == nil ||
		closed.Kept.Reason != "bir sonraki ziyarete aktarıldı" || closed.Kept.By == nil || !closed.Refundable.Equal(money.FromUnits(100)) {
		t.Fatalf("closed = %s can %v kept %+v refundable %s", closed.PaymentStatus, closed.CanClose, closed.Kept, closed.Refundable)
	}
	if _, ok := followUp(r1.UUID); ok {
		t.Fatal("a kept visit is still on the follow-up list")
	}
	if n := count(`SELECT COUNT(*) FROM reservation_events e JOIN reservations r ON r.organization_id = e.organization_id AND r.id = e.reservation_id
		WHERE e.organization_id = $1 AND r.uuid = $2 AND e.type = 'payment_kept' AND e.note = 'bir sonraki ziyarete aktarıldı'`, org.ID, r1.UUID); n != 1 {
		t.Fatalf("payment_kept events = %d", n)
	}
	if _, err := svc.CloseWithoutRefund(ctx, org.ID, r1.UUID, none, owner.ID, all, model.CloseInput{Reason: "again"}); !errors.Is(err, usecase.ErrNothingToClose) {
		t.Fatalf("close twice = %v", err)
	}
	if _, err := svc.CloseWithoutRefund(ctx, org.ID+100000, r1.UUID, none, owner.ID, all, model.CloseInput{Reason: "x"}); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("foreign close = %v", err)
	}

	refund := func(key, amount string) usecase.RefundResult {
		t.Helper()
		res, err := svc.Refund(ctx, org.ID, r1.UUID, none, owner.ID, all, model.RefundInput{
			CollectionUUID: col, Amount: money.MustParse(amount), Channel: model.ChannelProvider, IdempotencyKey: key,
		})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	// The money may still be paid back: a declined refund puts the visit
	// back on the list (and tells the customer it is being processed);
	// closing it again takes it off.
	declined := refund("k-decline", "10.13")
	if declined.Refund.Status != model.RefundFailed || !declined.Payments.CanClose {
		t.Fatalf("declined = %+v can close %v", declined.Refund, declined.Payments.CanClose)
	}
	if f, ok := followUp(r1.UUID); !ok || f.Kind != model.FollowRefundFailed {
		t.Fatalf("failed after kept = %+v %v", f, ok)
	}
	if notices(customernotifycatalog.RefundProcessing) != 1 || notices(customernotifycatalog.RefundCompleted) != 0 {
		t.Fatalf("notices after a decline = %d / %d", notices(customernotifycatalog.RefundProcessing), notices(customernotifycatalog.RefundCompleted))
	}
	advanceClock(time.Second)
	if _, err := svc.CloseWithoutRefund(ctx, org.ID, r1.UUID, none, owner.ID, all, model.CloseInput{Reason: "müşteri bıraktı"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := followUp(r1.UUID); ok {
		t.Fatal("closed again but still listed")
	}

	// An unknown result: being processed; closing waits for it; the
	// worker's query finds it refunded and posts it (done notice).
	found := refund("k-found", "10.99")
	if found.Refund.Status != model.RefundRequested || found.Refund.ProviderStatus != model.ProviderUncertain || !found.Refund.AutoCheck ||
		found.Payments.CanClose {
		t.Fatalf("unknown = %+v can close %v", found.Refund, found.Payments.CanClose)
	}
	if _, err := svc.CloseWithoutRefund(ctx, org.ID, r1.UUID, none, owner.ID, all, model.CloseInput{Reason: "x"}); !errors.Is(err, usecase.ErrRefundOpen) {
		t.Fatalf("close with an open refund = %v", err)
	}
	if notices(customernotifycatalog.RefundProcessing) != 2 {
		t.Fatalf("processing notices = %d", notices(customernotifycatalog.RefundProcessing))
	}
	// Not found and still pending ones too.
	notDone := refund("k-not-done", "10.98")
	pending := refund("k-pending", "10.97")
	if _, err := svc.CheckUncertainRefunds(ctx); err != nil {
		t.Fatal(err)
	}
	if st, ps, checks, _ := refundRow(found.Refund.UUID); st != model.RefundSucceeded || ps != model.ProviderSucceeded || checks != 1 {
		t.Fatalf("found = %s / %s / %d", st, ps, checks)
	}
	if st, ps, _, _ := refundRow(notDone.Refund.UUID); st != model.RefundFailed || ps != model.ProviderFailed {
		t.Fatalf("not done = %s / %s", st, ps)
	}
	st, ps, checks, next := refundRow(pending.Refund.UUID)
	if st != model.RefundRequested || ps != model.ProviderUncertain || checks != 1 || next == nil || !next.Equal(clock().Add(5*time.Minute)) {
		t.Fatalf("pending = %s / %s / %d / %v", st, ps, checks, next)
	}
	if notices(customernotifycatalog.RefundCompleted) != 1 || notices(customernotifycatalog.RefundProcessing) != 4 {
		t.Fatalf("notices after the checks = %d done / %d processing",
			notices(customernotifycatalog.RefundCompleted), notices(customernotifycatalog.RefundProcessing))
	}
	// Nothing was sent to the provider again: one refund ledger entry, for
	// the one found.
	if n := count(`SELECT COUNT(*) FROM ledger_entries WHERE organization_id = $1 AND kind = 'refund'`, org.ID); n != 1 {
		t.Fatalf("refund entries = %d", n)
	}
	v = view(r1.UUID)
	if v.PaymentStatus != reservationsmodel.PaymentPartiallyRefunded || !v.Refundable.Equal(money.MustParse("78.04")) {
		t.Fatalf("after the checks = %s / %s", v.PaymentStatus, v.Refundable)
	}
	// Not due yet: not asked; later it is, and the next wait is longer.
	if _, err := svc.CheckUncertainRefunds(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, checks, _ := refundRow(pending.Refund.UUID); checks != 1 {
		t.Fatalf("asked before due: %d", checks)
	}
	advanceClock(6 * time.Minute)
	if _, err := svc.CheckUncertainRefunds(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, checks, next := refundRow(pending.Refund.UUID); checks != 2 || next == nil || !next.Equal(clock().Add(15*time.Minute)) {
		t.Fatalf("second check = %d / %v", checks, next)
	}
	// The open one is settled by a person: the done notice goes once.
	if _, err := svc.Resolve(ctx, org.ID, pending.Refund.UUID, none, owner.ID, all, model.ResolveInput{Outcome: model.ResolveSucceeded}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refund(ctx, org.ID, r1.UUID, none, owner.ID, all, model.RefundInput{
		CollectionUUID: col, Amount: money.MustParse("10.97"), Channel: model.ChannelProvider, IdempotencyKey: "k-pending",
	}); err != nil {
		t.Fatal(err)
	}
	if notices(customernotifycatalog.RefundCompleted) != 2 {
		t.Fatalf("done notices = %d", notices(customernotifycatalog.RefundCompleted))
	}
	// The rest by hand: done notice, refunded.
	manual, err := svc.Refund(ctx, org.ID, r1.UUID, none, owner.ID, all, model.RefundInput{
		CollectionUUID: col, Amount: money.MustParse("78.04"), Channel: model.ChannelManual, Method: "online",
		CashAccountUUID: &onlineAccUUID, IdempotencyKey: "k-manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	if manual.Payments.PaymentStatus != reservationsmodel.PaymentRefunded || manual.Payments.CanClose || manual.Payments.Kept != nil {
		t.Fatalf("manual = %s can close %v", manual.Payments.PaymentStatus, manual.Payments.CanClose)
	}
	if notices(customernotifycatalog.RefundCompleted) != 3 {
		t.Fatalf("done notices = %d", notices(customernotifycatalog.RefundCompleted))
	}
}
