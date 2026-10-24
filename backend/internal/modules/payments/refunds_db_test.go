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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/testpay"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// TestRefundsFlow is A14 and the refund flow on Postgres: a paid booking
// cancelled waits for its refund on the follow-up list; refunds through the
// provider in parts (a confirmed one is posted once, a declined one fails,
// an unknown one waits for a person and is never sent again), the same
// idempotency key twice, more than what is left, a manual refund of the
// rest (refunded), a visit paid at the business and cancelled, and a
// finished visit not paid in full.
func TestRefundsFlow(t *testing.T) {
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
		Email: "refund-owner-" + suffix + "@example.test", PasswordHash: "x", Name: "Owner", Surname: "Test", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Refunds " + suffix, OwnerUserID: owner.ID})
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

	holdNow := at(8, 0)
	var clockMu sync.Mutex
	clock := func() time.Time { clockMu.Lock(); defer clockMu.Unlock(); return holdNow }
	cashSvc := cashusecase.New(cashrepository.NewPostgres(pool, q), nil)
	cashSvc.SetClock(clock)
	salesSvc := salesusecase.New(salesrepository.NewPostgres(pool, q), cashSvc)
	salesSvc.SetClock(clock)
	holds := availabilityusecase.NewHolds(pool, q, settings)
	holds.SetClock(clock)
	holds.SetExpiryHook(reservationsusecase.ExpireRequests)
	resSvc := reservationsusecase.New(holds, q, settings)
	resSvc.SetCompletionHook(salesSvc.RecordReservationTx)

	cipher, err := crypto.NewFieldCipher(strings.Repeat("r", 32))
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
	none := availabilityusecase.Caller{}
	all := usecase.ViewOptions{Collect: true, Refund: true}

	get := func(id uuid.UUID) reservationsmodel.Reservation {
		t.Helper()
		r, err := resSvc.Get(ctx, org.ID, id, none)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	view := func(id uuid.UUID) model.ReservationPayments {
		t.Helper()
		v, err := svc.ReservationPayments(ctx, org.ID, id, none, all)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	followUps := func(kinds ...string) map[uuid.UUID]model.FollowUp {
		t.Helper()
		items, total, err := svc.ListFollowUps(ctx, org.ID, model.FollowUpFilter{Kinds: kinds, Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		if int(total) != len(items) {
			t.Fatalf("follow-up total %d, items %d", total, len(items))
		}
		out := map[uuid.UUID]model.FollowUp{}
		for _, f := range items {
			out[f.UUID] = f
		}
		return out
	}

	// A booking paid online in full, then cancelled by the business.
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
	if got := get(r1.UUID); got.Status != reservationsmodel.StatusCancelled || got.PaymentStatus != reservationsmodel.PaymentRefundPending {
		t.Fatalf("cancelled paid booking = %s / %s", got.Status, got.PaymentStatus)
	}
	v := view(r1.UUID)
	if !v.CanRefund || !v.Refundable.Equal(money.FromUnits(100)) || len(v.RefundCollections) != 1 ||
		!v.RefundCollections[0].ProviderRefund || v.RefundCollections[0].Provider != testpay.ProviderCode {
		t.Fatalf("refund view = %+v", v)
	}
	col := v.RefundCollections[0].UUID
	if f, ok := followUps()[r1.UUID]; !ok || f.Kind != model.FollowRefundPending || !f.Amount.Equal(money.FromUnits(100)) {
		t.Fatalf("follow-up = %+v (%v)", f, ok)
	}
	// Without the refund option the view lists no refund targets.
	if plain, err := svc.ReservationPayments(ctx, org.ID, r1.UUID, none, usecase.ViewOptions{}); err != nil || plain.CanRefund ||
		len(plain.RefundCollections) != 0 || !plain.Refundable.Equal(money.FromUnits(100)) {
		t.Fatalf("view without refund = %+v %v", plain, err)
	}

	refund := func(key, amount, channel, method string, account *uuid.UUID) (usecase.RefundResult, error) {
		return svc.Refund(ctx, org.ID, r1.UUID, none, owner.ID, all, model.RefundInput{
			CollectionUUID: col, Amount: money.MustParse(amount), Channel: channel, Method: method, CashAccountUUID: account,
			Reason: "iptal", IdempotencyKey: key,
		})
	}
	var ve *usecase.ValidationError
	if _, err := refund("", "40", model.ChannelProvider, "", nil); !errors.As(err, &ve) || ve.Details[0].Field != "idempotency_key" {
		t.Fatalf("no key = %v", err)
	}
	if _, err := refund("k-big", "120", model.ChannelProvider, "", nil); !errors.As(err, &ve) || ve.Details[0].Code != "exceeds_refundable" {
		t.Fatalf("more than paid = %v", err)
	}

	// A partial refund through the provider: requested, answered, posted.
	res, err := refund("k-40", "40", model.ChannelProvider, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Refund.Status != model.RefundSucceeded || res.Refund.ProviderStatus != model.ProviderSucceeded ||
		res.Refund.LedgerEntryUUID == nil || res.Refund.ReflectedAt == nil || res.Refund.ProviderRefundID == "" ||
		res.Refund.Method != "online" || res.Refund.CashAccount.UUID != onlineAccUUID {
		t.Fatalf("provider refund = %+v", res.Refund)
	}
	if res.Payments.PaymentStatus != reservationsmodel.PaymentPartiallyRefunded || !res.Payments.Refundable.Equal(money.FromUnits(60)) {
		t.Fatalf("after a part = %s, %s", res.Payments.PaymentStatus, res.Payments.Refundable)
	}
	// The same key again: the same refund, nothing new.
	again, err := refund("k-40", "40", model.ChannelProvider, "", nil)
	if err != nil || !again.Replayed || again.Refund.UUID != res.Refund.UUID {
		t.Fatalf("replay = %+v %v", again.Refund, err)
	}
	if _, err := refund("k-40", "41", model.ChannelProvider, "", nil); !errors.Is(err, usecase.ErrIdempotencyReused) {
		t.Fatalf("key reused = %v", err)
	}
	if n := count(`SELECT COUNT(*) FROM ledger_entries WHERE organization_id = $1 AND kind = 'refund'`, org.ID); n != 1 {
		t.Fatalf("refund entries = %d, want 1", n)
	}
	if n := count(`SELECT COUNT(*) FROM cash_movements m JOIN cash_accounts a ON a.organization_id = m.organization_id AND a.id = m.cash_account_id
		WHERE m.organization_id = $1 AND a.uuid = $2 AND m.kind = 'refund' AND m.amount = 40`, org.ID, onlineAccUUID); n != 1 {
		t.Fatalf("refund movements = %d, want 1", n)
	}
	if _, err := refund("k-70", "70", model.ChannelProvider, "", nil); !errors.As(err, &ve) || ve.Details[0].Code != "exceeds_refundable" {
		t.Fatalf("more than left = %v", err)
	}

	// The provider declines: failed, nothing posted, on the list.
	declined, err := refund("k-decline", "10.13", model.ChannelProvider, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if declined.Refund.Status != model.RefundFailed || declined.Refund.ProviderStatus != model.ProviderFailed ||
		!strings.Contains(declined.Refund.FailureReason, "test_declined") || !declined.Payments.Refundable.Equal(money.FromUnits(60)) {
		t.Fatalf("declined = %+v", declined.Refund)
	}
	if f := followUps(model.FollowRefundFailed)[r1.UUID]; f.Kind != model.FollowRefundFailed || !f.Amount.Equal(money.FromUnits(60)) {
		t.Fatalf("failed follow-up = %+v", f)
	}

	// An unknown result: requested, the amount is held, a person settles it.
	unknown, err := refund("k-unknown", "10.99", model.ChannelProvider, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if unknown.Refund.Status != model.RefundRequested || unknown.Refund.ProviderStatus != model.ProviderUncertain ||
		!unknown.Refund.NeedsAction || !unknown.Payments.Refundable.Equal(money.MustParse("49.01")) {
		t.Fatalf("unknown = %+v, refundable %s", unknown.Refund, unknown.Payments.Refundable)
	}
	if f := followUps()[r1.UUID]; f.Kind != model.FollowRefundUnverified || f.OpenRefunds != 1 {
		t.Fatalf("unverified follow-up = %+v", f)
	}
	// The same key does not ask the provider again.
	if again, err := refund("k-unknown", "10.99", model.ChannelProvider, "", nil); err != nil || again.Refund.Status != model.RefundRequested {
		t.Fatalf("unknown replay = %+v %v", again.Refund, err)
	}
	if _, err := svc.Resolve(ctx, org.ID, unknown.Refund.UUID, none, owner.ID, all, model.ResolveInput{Outcome: "maybe"}); !errors.As(err, &ve) {
		t.Fatalf("bad outcome = %v", err)
	}
	resolved, err := svc.Resolve(ctx, org.ID, unknown.Refund.UUID, none, owner.ID, all, model.ResolveInput{Outcome: model.ResolveFailed, Note: "panelde yok"})
	if err != nil || resolved.Refund.Status != model.RefundFailed || !resolved.Payments.Refundable.Equal(money.FromUnits(60)) {
		t.Fatalf("resolved failed = %+v %v", resolved.Refund, err)
	}
	if _, err := svc.Resolve(ctx, org.ID, unknown.Refund.UUID, none, owner.ID, all, model.ResolveInput{Outcome: model.ResolveSucceeded}); !errors.Is(err, usecase.ErrRefundNotOpen) {
		t.Fatalf("resolve twice = %v", err)
	}
	// Another unknown one that the provider did refund: posted on resolve.
	unknown2, err := refund("k-unknown-2", "20.99", model.ChannelProvider, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err = svc.Resolve(ctx, org.ID, unknown2.Refund.UUID, none, owner.ID, all, model.ResolveInput{Outcome: model.ResolveSucceeded})
	if err != nil || resolved.Refund.Status != model.RefundSucceeded || resolved.Refund.LedgerEntryUUID == nil ||
		!resolved.Payments.Refundable.Equal(money.MustParse("39.01")) {
		t.Fatalf("resolved succeeded = %+v %v", resolved.Refund, err)
	}
	// Another business does not find it.
	if _, err := svc.Resolve(ctx, org.ID+100000, unknown2.Refund.UUID, none, owner.ID, all, model.ResolveInput{Outcome: model.ResolveFailed}); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("foreign resolve = %v", err)
	}

	// The rest refunded in the provider's own panel and recorded by hand:
	// refunded, off the list. The empty till cannot pay anything back.
	if _, err := refund("k-manual-bad", "39.01", model.ChannelManual, "cheque", &tillUUID); !errors.As(err, &ve) || ve.Details[0].Field != "method" {
		t.Fatalf("bad method = %v", err)
	}
	if _, err := refund("k-manual-till", "39.01", model.ChannelManual, "cash", &tillUUID); !errors.Is(err, cashusecase.ErrInsufficientFunds) {
		t.Fatalf("refund from an empty till = %v", err)
	}
	manual, err := refund("k-manual", "39.01", model.ChannelManual, "online", &onlineAccUUID)
	if err != nil {
		t.Fatal(err)
	}
	if manual.Refund.Status != model.RefundSucceeded || manual.Refund.ProviderStatus != "" || manual.Refund.CashAccount.UUID != onlineAccUUID ||
		manual.Payments.PaymentStatus != reservationsmodel.PaymentRefunded || manual.Payments.Refundable.IsPositive() || manual.Payments.CanRefund {
		t.Fatalf("manual = %+v / %s", manual.Refund, manual.Payments.PaymentStatus)
	}
	if _, ok := followUps()[r1.UUID]; ok {
		t.Fatal("a refunded visit is still on the follow-up list")
	}
	if got := get(r1.UUID); got.Status != reservationsmodel.StatusCancelled {
		t.Fatalf("a refund changed the visit: %s", got.Status)
	}
	if n := count(`SELECT COUNT(*) FROM reservation_events e JOIN reservations r ON r.organization_id = e.organization_id AND r.id = e.reservation_id
		WHERE e.organization_id = $1 AND r.uuid = $2 AND e.type = 'refund_recorded'`, org.ID, r1.UUID); n != 3 {
		t.Fatalf("refund events = %d, want 3", n)
	}

	// Paid at the business, then cancelled: the money waits for a refund.
	start2 := at(14, 0)
	visit, err := resSvc.Create(ctx, org.ID, reservationsmodel.CreateInput{
		BranchUUID: branchUUID, CustomerUUID: customerUUID, StartsAt: &start2, Source: reservationsmodel.SourcePanel,
		Lines: []availabilitymodel.LineInput{{ServiceUUID: cutUUID}},
	}, none, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Collect(ctx, org.ID, visit.UUID, none, owner.ID, all, model.CollectInput{
		Amount: money.FromUnits(30), Method: "cash", CashAccountUUID: tillUUID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := resSvc.Cancel(ctx, org.ID, visit.UUID, none, owner.ID, reservationsusecase.ChangeInput{}); err != nil {
		t.Fatal(err)
	}
	if got := get(visit.UUID); got.PaymentStatus != reservationsmodel.PaymentRefundPending {
		t.Fatalf("cancelled after a part paid on site = %s", got.PaymentStatus)
	}
	vv := view(visit.UUID)
	if vv.RefundCollections[0].ProviderRefund {
		t.Fatal("a cash collection offered a provider refund")
	}
	if _, err := svc.Refund(ctx, org.ID, visit.UUID, none, owner.ID, all, model.RefundInput{
		CollectionUUID: vv.RefundCollections[0].UUID, Amount: money.FromUnits(30), Channel: model.ChannelProvider, IdempotencyKey: "k-cash-provider",
	}); !errors.As(err, &ve) || ve.Details[0].Code != "unsupported" {
		t.Fatalf("provider refund of cash = %v", err)
	}
	if f := followUps()[visit.UUID]; f.Kind != model.FollowRefundPending || !f.Amount.Equal(money.FromUnits(30)) {
		t.Fatalf("on-site follow-up = %+v", f)
	}
	// Part of it back from the till: partially refunded, off the list.
	part, err := svc.Refund(ctx, org.ID, visit.UUID, none, owner.ID, all, model.RefundInput{
		CollectionUUID: vv.RefundCollections[0].UUID, Amount: money.FromUnits(10), Channel: model.ChannelManual, Method: "cash",
		CashAccountUUID: &tillUUID, IdempotencyKey: "k-cash-part",
	})
	if err != nil || part.Payments.PaymentStatus != reservationsmodel.PaymentPartiallyRefunded || !part.Payments.Refundable.Equal(money.FromUnits(20)) {
		t.Fatalf("part from the till = %+v %v", part.Payments, err)
	}
	if _, ok := followUps()[visit.UUID]; ok {
		t.Fatal("a partially refunded visit is still on the list")
	}

	// A finished visit not paid in full is on the list as unpaid.
	start3 := at(15, 0)
	done, err := resSvc.Create(ctx, org.ID, reservationsmodel.CreateInput{
		BranchUUID: branchUUID, CustomerUUID: customerUUID, StartsAt: &start3, Source: reservationsmodel.SourcePanel,
		Lines: []availabilitymodel.LineInput{{ServiceUUID: cutUUID}},
	}, none, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resSvc.Complete(ctx, org.ID, done.UUID, none, owner.ID, reservationsusecase.ChangeInput{}); err != nil {
		t.Fatal(err)
	}
	if f := followUps(model.FollowUnpaid)[done.UUID]; f.Kind != model.FollowUnpaid || !f.Amount.Equal(money.FromUnits(100)) {
		t.Fatalf("unpaid follow-up = %+v", f)
	}
	facets, err := svc.FollowUpFacets(ctx, org.ID, nil, "")
	if err != nil || len(facets.Kind) != 1 || facets.Kind[0].Value != model.FollowUnpaid {
		t.Fatalf("facets = %+v %v", facets, err)
	}
	if items, _, err := svc.ListFollowUps(ctx, org.ID, model.FollowUpFilter{Q: "Zeynep", Limit: 10}); err != nil || len(items) != 1 {
		t.Fatalf("search = %d %v", len(items), err)
	}
	// A member limited to no branch sees none of it.
	narrow := orgctx.WithScope(ctx, orgctx.Scope{ID: org.ID, BranchIDs: []int64{}})
	if items, total, err := svc.ListFollowUps(narrow, org.ID, model.FollowUpFilter{Limit: 10}); err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("out of scope = %d %d %v", len(items), total, err)
	}
	if _, err := svc.Resolve(narrow, org.ID, unknown.Refund.UUID, none, owner.ID, all, model.ResolveInput{Outcome: model.ResolveFailed}); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("out of scope resolve = %v", err)
	}
}
