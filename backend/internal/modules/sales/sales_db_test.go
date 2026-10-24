package sales_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilitymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	cashmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/model"
	cashrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/repository"
	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	productsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/model"
	productsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/repository"
	productsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/usecase"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales"
	saleshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/model"
	salesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
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

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func ptr[T any](v T) *T { return &v }

type mapLoader map[uuid.UUID]int64

func (m mapLoader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, _ := uuid.Parse(c.Subject)
	return authctx.Principal{UserID: id, UserInternal: m[id]}, nil
}

// TestSalesLedger walks a visit paid online the day before through its
// completion, a correction, a refund and reversals, and checks that
// revenue, collections, transfers and staff payouts stay apart (A20):
// the prepayment is a collection on its day, the service revenue on the
// day it is delivered, never income twice; nothing is updated or deleted.
// Runs when TEST_DATABASE_URL points at a migrated database.
func TestSalesLedger(t *testing.T) {
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

	newUser := func(name string) db.User {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: name + "-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	owner, carol, dave, other := newUser("owner"), newUser("carol"), newUser("dave"), newUser("other")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Sales " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	org2, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Sales other " + suffix, OwnerUserID: other.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM activity_events WHERE organization_id = ANY($1)", []int64{org.ID, org2.ID})
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{org.ID, org2.ID})
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{owner.ID, carol.ID, dave.ID, other.ID})
	})
	if _, err := orgSvc.AddMember(ctx, org.UUID, carol.Uuid, "sube_yoneticisi"); err != nil {
		t.Fatal(err)
	}
	if _, err := orgSvc.AddMember(ctx, org.UUID, dave.Uuid, "resepsiyon"); err != nil {
		t.Fatal(err)
	}
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

	// Branch "Merkez", Ayşe (Mondays 09–12), a cut for 100, a customer, a
	// till and a bank account.
	branchID, branchUUID := idOf("SELECT id, uuid FROM branches WHERE organization_id = $1 ORDER BY id LIMIT 1", org.ID)
	exec(`UPDATE branches SET working_hours = $3, holiday_exceptions = '[]', timezone = NULL WHERE organization_id = $1 AND id = $2`,
		org.ID, branchID, `{"mon":{"closed":false,"open":"09:00","close":"18:00"}}`)
	ayseID, ayseUUID := idOf(`INSERT INTO staff (organization_id, name) VALUES ($1, 'Ayşe') RETURNING id, uuid`, org.ID)
	exec(`INSERT INTO staff_branches (organization_id, staff_id, branch_id, shifts) VALUES
		($1, $2, $3, '{"mon":[{"start":"09:00","end":"12:00","breaks":[]}]}')`, org.ID, ayseID, branchID)
	cutID, cutUUID := idOf(`INSERT INTO services (organization_id, name, duration_minutes, price, payment_mode)
		VALUES ($1, 'Kesim', 30, 100, 'both') RETURNING id, uuid`, org.ID)
	exec(`INSERT INTO service_branches (organization_id, service_id, branch_id) VALUES ($1, $2, $3)`, org.ID, cutID, branchID)
	exec(`INSERT INTO service_staff (organization_id, service_id, staff_id) VALUES ($1, $2, $3)`, org.ID, cutID, ayseID)
	_, customerUUID := idOf(`INSERT INTO customers (organization_id, first_name, last_name) VALUES ($1, 'Zeynep', 'Kaya') RETURNING id, uuid`, org.ID)
	_, tillUUID := idOf(`INSERT INTO cash_accounts (organization_id, branch_id, kind, name, currency, opening_date)
		VALUES ($1, $2, 'cash', 'Kasa', 'TRY', '2026-01-01') RETURNING id, uuid`, org.ID, branchID)
	_, bankUUID := idOf(`INSERT INTO cash_accounts (organization_id, branch_id, kind, name, currency, opening_date)
		VALUES ($1, NULL, 'bank', 'Banka', 'TRY', '2026-01-01') RETURNING id, uuid`, org.ID)

	now := at(12, 0)
	clock := func() time.Time { return now }
	cashSvc := cashusecase.New(cashrepository.NewPostgres(pool, q), nil)
	cashSvc.SetClock(clock)
	svc := usecase.New(salesrepository.NewPostgres(pool, q), cashSvc)
	svc.SetClock(clock)
	settings := orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))
	holds := availabilityusecase.NewHolds(pool, q, settings)
	holds.SetClock(func() time.Time { return at(8, 0) })
	holds.SetExpiryHook(reservationsusecase.ExpireRequests)
	resSvc := reservationsusecase.New(holds, q, settings)
	resSvc.SetCompletionHook(svc.RecordReservationTx)

	// A visit at 10:00 booked at the counter.
	start := at(10, 0)
	res, err := resSvc.Create(ctx, org.ID, reservationsmodel.CreateInput{
		BranchUUID: branchUUID, CustomerUUID: customerUUID, StartsAt: &start, Source: reservationsmodel.SourcePanel,
		Lines: []availabilitymodel.LineInput{{ServiceUUID: cutUUID, StaffUUID: &ayseUUID}},
	}, availabilityusecase.Caller{}, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !res.TotalPrice.Equal(money.FromUnits(100)) {
		t.Fatalf("total = %s", res.TotalPrice)
	}

	figures := func(from, to string) model.Figures {
		t.Helper()
		f, tt := day(from), day(to)
		s, err := svc.Summary(ctx, org.ID, &f, &tt, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(s.Totals) != 1 || s.Totals[0].Currency != "TRY" {
			t.Fatalf("totals = %+v", s.Totals)
		}
		return s.Totals[0]
	}
	balance := func(step string) model.ReservationBalance {
		t.Helper()
		b, err := svc.ReservationBalance(ctx, org.ID, res.UUID)
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		return b
	}
	eq := func(step string, got money.Money, want int64) {
		t.Helper()
		if !got.Equal(money.FromUnits(want)) {
			t.Fatalf("%s = %s, want %d", step, got, want)
		}
	}

	// 1. Paid online the day before: a collection on 01-06, no revenue.
	payKey := "payment:" + suffix
	col, err := svc.RecordCollection(ctx, org.ID, model.CollectionInput{
		ReservationUUID: &res.UUID, CashAccountUUID: tillUUID, Amount: money.FromUnits(100), Method: model.MethodOnline,
		Date: day("2030-01-06"), Description: "online payment", IdempotencyKey: payKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	if col.Kind != model.EntryCollection || col.Direction != model.Credit || col.EntryDate != "2030-01-06" ||
		col.Customer == nil || col.Customer.UUID != customerUUID || col.CashAccount == nil || col.CashMovementUUID == nil {
		t.Fatalf("collection = %+v", col)
	}
	// A repeated notification is the same collection (one cash movement).
	again, err := svc.RecordCollection(ctx, org.ID, model.CollectionInput{
		ReservationUUID: &res.UUID, CashAccountUUID: tillUUID, Amount: money.FromUnits(100), Method: model.MethodOnline,
		Date: day("2030-01-06"), IdempotencyKey: payKey,
	})
	if err != nil || again.UUID != col.UUID {
		t.Fatalf("replay = %v %v", again.UUID, err)
	}
	if _, err := svc.RecordCollection(ctx, org.ID, model.CollectionInput{
		ReservationUUID: &res.UUID, CashAccountUUID: tillUUID, Amount: money.FromUnits(90), Method: model.MethodOnline,
		IdempotencyKey: payKey,
	}); !errors.Is(err, usecase.ErrIdempotencyReused) {
		t.Fatalf("reused key = %v", err)
	}
	if n := count(`SELECT count(*) FROM cash_movements WHERE organization_id = $1 AND kind = 'collection'`, org.ID); n != 1 {
		t.Fatalf("cash collections = %d", n)
	}
	f := figures("2030-01-06", "2030-01-06")
	eq("01-06 revenue", f.Revenue.Net, 0)
	eq("01-06 collected", f.Collections.Net, 100)
	b := balance("before completion")
	if b.SalesPosted {
		t.Fatal("sales posted before completion")
	}
	eq("expected before", b.Expected, 100)
	eq("paid before", b.Paid, 100)
	eq("remaining before", b.Remaining, 0)

	// 2. Completed: one sale line on 01-07 with Ayşe, a ledger debit.
	if _, err := resSvc.Complete(ctx, org.ID, res.UUID, availabilityusecase.Caller{}, owner.ID, reservationsusecase.ChangeInput{}); err != nil {
		t.Fatal(err)
	}
	lines, total, err := svc.ListLines(ctx, org.ID, model.LineFilter{ReservationUUID: &res.UUID, Limit: 50})
	if err != nil || total != 1 {
		t.Fatalf("lines = %d %v", total, err)
	}
	line := lines[0]
	if line.Kind != model.LineSale || line.ItemType != model.ItemService || line.SaleDate != "2030-01-07" ||
		line.Staff == nil || line.Staff.UUID != ayseUUID || line.Description != "Kesim" || line.Customer == nil {
		t.Fatalf("line = %+v", line)
	}
	eq("line net", line.NetAmount, 100)
	// Running the posting again changes nothing.
	resID, _ := idOf(`SELECT id, uuid FROM reservations WHERE organization_id = $1 AND uuid = $2`, org.ID, res.UUID)
	if err := svc.RecordReservationTx(ctx, q, org.ID, resID, owner.ID); err != nil {
		t.Fatal(err)
	}
	if n := count(`SELECT count(*) FROM sale_lines WHERE organization_id = $1`, org.ID); n != 1 {
		t.Fatalf("sale lines after repost = %d", n)
	}
	f = figures("2030-01-07", "2030-01-07")
	eq("01-07 revenue", f.Revenue.Net, 100)
	eq("01-07 collected", f.Collections.Net, 0)
	f = figures("2030-01-06", "2030-01-07")
	eq("range revenue", f.Revenue.Net, 100)
	eq("range collected", f.Collections.Net, 100)
	b = balance("completed")
	if !b.SalesPosted || len(b.Lines) != 1 || len(b.Entries) != 2 {
		t.Fatalf("balance = %+v", b)
	}
	eq("expected", b.Expected, 100)
	eq("remaining", b.Remaining, 0)

	// The database refuses a second live sale of the same item, edits and
	// deletes.
	if _, err := pool.Exec(ctx, `INSERT INTO sale_lines (organization_id, branch_id, kind, item_type, source_type,
		reservation_id, line_index, description, quantity, unit_price, gross_amount, discount_amount, net_amount,
		currency, sale_date, accrual_key) SELECT organization_id, branch_id, kind, item_type, source_type, reservation_id,
		line_index, description, quantity, unit_price, gross_amount, discount_amount, net_amount, currency, sale_date,
		accrual_key FROM sale_lines WHERE organization_id = $1`, org.ID); err == nil {
		t.Fatal("second live sale line accepted")
	}
	for _, sql := range []string{
		`UPDATE sale_lines SET net_amount = 1 WHERE organization_id = $1`,
		`DELETE FROM sale_lines WHERE organization_id = $1`,
		`UPDATE ledger_entries SET amount = 1 WHERE organization_id = $1`,
		`DELETE FROM ledger_entries WHERE organization_id = $1`,
	} {
		if _, err := pool.Exec(ctx, sql, org.ID); err == nil {
			t.Fatalf("%s accepted", sql)
		}
	}

	// 3. Correction: a 20 discount. The old line is reversed and replaced,
	// both on the day of the correction; 20 is now held for the customer.
	if _, err := svc.CorrectLine(ctx, org.ID, line.UUID, &owner.ID, model.CorrectInput{Reason: "same"}); !errors.Is(err, usecase.ErrNothingChanged) {
		t.Fatalf("no-op correction = %v", err)
	}
	if _, err := svc.CorrectLine(ctx, org.ID, line.UUID, &owner.ID, model.CorrectInput{DiscountAmount: ptr(money.FromUnits(20))}); !usecase.IsValidation(err) {
		t.Fatalf("correction without reason = %v", err)
	}
	corr, err := svc.CorrectLine(ctx, org.ID, line.UUID, &owner.ID, model.CorrectInput{
		DiscountAmount: ptr(money.FromUnits(20)), Reason: "sadakat indirimi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if corr.Reversal.ReversesUUID == nil || *corr.Reversal.ReversesUUID != line.UUID || corr.Replacement.ReplacesUUID == nil ||
		*corr.Replacement.ReplacesUUID != line.UUID || corr.Replacement.Generation != 1 {
		t.Fatalf("correction = %+v", corr)
	}
	eq("reversal net", corr.Reversal.NetAmount, -100)
	eq("replacement net", corr.Replacement.NetAmount, 80)
	if _, err := svc.CorrectLine(ctx, org.ID, line.UUID, &owner.ID, model.CorrectInput{
		DiscountAmount: ptr(money.FromUnits(10)), Reason: "again",
	}); !errors.Is(err, usecase.ErrAlreadyReversed) {
		t.Fatalf("correcting a reversed line = %v", err)
	}
	if _, err := svc.ReverseLine(ctx, org.ID, corr.Reversal.UUID, &owner.ID, model.ReverseInput{Reason: "nope"}); !errors.Is(err, usecase.ErrNotReversible) {
		t.Fatalf("reversing a reversal = %v", err)
	}
	f = figures("2030-01-07", "2030-01-07")
	eq("corrected revenue", f.Revenue.Net, 80)
	eq("corrected gross", f.Revenue.Gross, 200)
	eq("corrected discount", f.Revenue.Discount, 20)
	eq("corrected reversed", f.Revenue.Reversed, -100)
	b = balance("corrected")
	eq("expected corrected", b.Expected, 80)
	eq("credit corrected", b.Credit, 20)
	eq("remaining corrected", b.Remaining, 0)

	// 4. The 20 is paid back; more than what is left is refused.
	refundID, err := svc.RecordRefund(ctx, org.ID, model.RefundInput{
		CollectionUUID: col.UUID, CashAccountUUID: tillUUID, Amount: money.FromUnits(20), Method: model.MethodCash,
		Description: "discount refund",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecordRefund(ctx, org.ID, model.RefundInput{
		CollectionUUID: col.UUID, CashAccountUUID: tillUUID, Amount: money.FromUnits(81), Method: model.MethodCash,
	}); !errors.Is(err, usecase.ErrRefundTooLarge) {
		t.Fatalf("refund too large = %v", err)
	}
	b = balance("refunded")
	eq("paid refunded", b.Paid, 80)
	eq("credit refunded", b.Credit, 0)
	f = figures("2030-01-07", "2030-01-07")
	eq("refunded on 01-07", f.Collections.Refunded, 20)
	eq("net collections 01-07", f.Collections.Net, -20)

	// 5. A collection with a live refund is not reversed; the refund is,
	// with its cash movement.
	if _, err := svc.ReverseEntry(ctx, org.ID, col.UUID, &owner.ID, model.ReverseInput{Reason: "wrong"}); !errors.Is(err, usecase.ErrHasRefunds) {
		t.Fatalf("reverse collection with refunds = %v", err)
	}
	sale, _, err := svc.ListEntries(ctx, org.ID, model.EntryFilter{ReservationUUID: &res.UUID, Kinds: []string{model.EntrySale}, Limit: 10})
	if err != nil || len(sale) != 2 {
		t.Fatalf("sale entries = %d %v", len(sale), err)
	}
	if _, err := svc.ReverseEntry(ctx, org.ID, sale[0].UUID, &owner.ID, model.ReverseInput{Reason: "wrong"}); !errors.Is(err, usecase.ErrReverseSaleLine) {
		t.Fatalf("reverse sale entry = %v", err)
	}
	rev, err := svc.ReverseEntry(ctx, org.ID, refundID.UUID, &owner.ID, model.ReverseInput{Reason: "kept as credit"})
	if err != nil {
		t.Fatal(err)
	}
	if rev.Kind != model.EntryReversal || rev.EffectiveKind != model.EntryRefund || rev.Direction != model.Credit {
		t.Fatalf("refund reversal = %+v", rev)
	}
	if _, err := svc.ReverseEntry(ctx, org.ID, refundID.UUID, &owner.ID, model.ReverseInput{Reason: "again"}); !errors.Is(err, usecase.ErrAlreadyReversed) {
		t.Fatalf("second reversal = %v", err)
	}
	if n := count(`SELECT count(*) FROM cash_movements WHERE organization_id = $1 AND kind = 'reversal'`, org.ID); n != 1 {
		t.Fatalf("cash reversals = %d", n)
	}
	b = balance("refund reversed")
	eq("paid after reversal", b.Paid, 100)
	eq("credit after reversal", b.Credit, 20)
	cb, err := svc.CustomerBalance(ctx, org.ID, customerUUID)
	if err != nil || len(cb.Totals) != 1 {
		t.Fatalf("customer balance = %+v %v", cb, err)
	}
	eq("customer charged", cb.Totals[0].Charged, 80)
	eq("customer balance", cb.Totals[0].Balance, -20)

	// 6. Transfers and staff payouts are reported apart; revenue and
	// collections do not move.
	if _, err := cashSvc.Transfer(ctx, org.ID, &owner.ID, cashmodel.TransferInput{
		FromAccountUUID: tillUUID, ToAccountUUID: bankUUID, Amount: money.FromUnits(50),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := cashSvc.Post(ctx, org.ID, cashusecase.PostInput{
		AccountUUID: bankUUID, Kind: cashmodel.MovementStaffPayment, Amount: money.FromUnits(30), Description: "Ayşe",
	}); err != nil {
		t.Fatal(err)
	}
	f = figures("2030-01-06", "2030-01-07")
	eq("A20 revenue", f.Revenue.Net, 80)
	eq("A20 collections", f.Collections.Net, 100)
	eq("A20 transfers", f.Transfers, 50)
	eq("A20 staff payments", f.StaffPayments, 30)

	// 7. A stock sale is a product line in the same transaction.
	productID, productUUID := idOf(`INSERT INTO products (organization_id, name, sku, price) VALUES ($1, 'Wax', 'WAX-1', 400) RETURNING id, uuid`, org.ID)
	exec(`INSERT INTO product_branches (organization_id, product_id, branch_id) VALUES ($1, $2, $3)`, org.ID, productID, branchID)
	products := productsusecase.New(productsrepository.NewPostgres(pool, q), nil)
	products.SetSaleRecorder(sales.ProductSaleRecorder(svc))
	if _, err := products.RecordMovement(ctx, org.ID, owner.ID, productsmodel.MovementInput{
		Kind: productsmodel.KindReceipt, BranchUUID: branchUUID, ProductUUID: productUUID, Quantity: ptr(int32(5)),
	}); err != nil {
		t.Fatal(err)
	}
	sold, err := products.RecordMovement(ctx, org.ID, owner.ID, productsmodel.MovementInput{
		Kind: productsmodel.KindSale, BranchUUID: branchUUID, ProductUUID: productUUID, Quantity: ptr(int32(2)),
		StaffUUID: &ayseUUID,
	})
	if err != nil {
		t.Fatal(err)
	}
	pl, n, err := svc.ListLines(ctx, org.ID, model.LineFilter{ItemTypes: []string{model.ItemProduct}, Limit: 10})
	if err != nil || n != 1 {
		t.Fatalf("product lines = %d %v", n, err)
	}
	if pl[0].SourceUUID == nil || *pl[0].SourceUUID != sold.Movements[0].UUID || pl[0].Quantity != 2 || pl[0].Customer != nil ||
		pl[0].Staff == nil || pl[0].Staff.UUID != ayseUUID {
		t.Fatalf("product line = %+v", pl[0])
	}
	eq("product net", pl[0].NetAmount, 800)
	if again, err := svc.RecordProductSaleTx(ctx, q, org.ID, usecase.ProductSale{
		MovementUUID: sold.Movements[0].UUID, BranchID: branchID, ProductID: productID, Quantity: 2, UnitPrice: money.FromUnits(400),
	}); err != nil || again != pl[0].UUID {
		t.Fatalf("product repost = %v %v", again, err)
	}

	// 8. Another business and a member limited to another branch see none
	// of it.
	if _, err := svc.GetLine(ctx, org2.ID, line.UUID); !errors.Is(err, usecase.ErrLineNotFound) {
		t.Fatalf("foreign line = %v", err)
	}
	if _, err := svc.ReservationBalance(ctx, org2.ID, res.UUID); !errors.Is(err, usecase.ErrReservationMissing) {
		t.Fatalf("foreign balance = %v", err)
	}
	if _, err := svc.ReverseEntry(ctx, org2.ID, col.UUID, &other.ID, model.ReverseInput{Reason: "steal"}); !errors.Is(err, usecase.ErrEntryNotFound) {
		t.Fatalf("foreign reverse = %v", err)
	}
	narrow := orgctx.WithScope(ctx, orgctx.Scope{ID: org.ID, BranchIDs: []int64{}})
	if _, err := svc.GetLine(narrow, org.ID, line.UUID); !errors.Is(err, usecase.ErrLineNotFound) {
		t.Fatalf("out of scope line = %v", err)
	}
	if _, n, err := svc.ListLines(narrow, org.ID, model.LineFilter{Limit: 10}); err != nil || n != 0 {
		t.Fatalf("out of scope lines = %d %v", n, err)
	}
	if s, err := svc.Summary(narrow, org.ID, ptr(day("2030-01-06")), ptr(day("2030-01-07")), nil); err != nil || !s.Totals[0].Revenue.Net.IsZero() {
		t.Fatalf("out of scope summary = %+v %v", s, err)
	}

	// 9. HTTP: finance roles read; reception does not; a price change needs
	// the price override permission, a discount the discount permission.
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	loader := mapLoader{owner.Uuid: owner.ID, carol.Uuid: carol.ID, dave.Uuid: dave.ID}
	sales.RegisterRoutes(mux, saleshandler.New(svc, nil), tokens, loader, orgSvc)
	call := func(user db.User, method, path string, body any) (int, map[string]any, string) {
		t.Helper()
		access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: user.Uuid})
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Authorization", "Bearer "+access)
		req.Header.Set(orgctx.HeaderName, org.Slug)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		var env struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		return rr.Code, env.Data, rr.Body.String()
	}
	code, data, raw := call(owner, http.MethodGet, "/v1/tenant/sales/summary?date_from=2030-01-06&date_to=2030-01-07", nil)
	if code != http.StatusOK {
		t.Fatalf("summary = %d %s", code, raw)
	}
	tot := data["totals"].([]any)[0].(map[string]any)
	if tot["revenue"].(map[string]any)["net"] != "80.00" || tot["collections"].(map[string]any)["net"] != "100.00" ||
		tot["transfers"] != "50.00" || tot["staff_payments"] != "30.00" {
		t.Fatalf("summary totals = %v", tot)
	}
	if code, _, raw := call(dave, http.MethodGet, "/v1/tenant/sales/lines", nil); code != http.StatusForbidden {
		t.Fatalf("reception lines = %d %s", code, raw)
	}
	if code, data, raw := call(carol, http.MethodGet, "/v1/tenant/sales/lines?reservation="+res.UUID.String()+"&kind=sale", nil); code != http.StatusOK || data["total"].(float64) != 2 {
		t.Fatalf("branch manager lines = %d %s", code, raw)
	}
	repl := corr.Replacement.UUID.String()
	if code, _, raw := call(carol, http.MethodPost, "/v1/tenant/sales/lines/"+repl+"/correct", map[string]any{
		"unit_price": "90.00", "reason": "fiyat",
	}); code != http.StatusForbidden {
		t.Fatalf("price change without override = %d %s", code, raw)
	}
	code, data, raw = call(carol, http.MethodPost, "/v1/tenant/sales/lines/"+repl+"/correct", map[string]any{
		"discount_amount": "30.00", "reason": "ek indirim",
	})
	if code != http.StatusCreated || data["replacement"].(map[string]any)["net_amount"] != "70.00" {
		t.Fatalf("discount correction = %d %s", code, raw)
	}
	if code, _, raw := call(owner, http.MethodGet, "/v1/tenant/sales/lines/"+uuid.NewString(), nil); code != http.StatusNotFound {
		t.Fatalf("unknown line = %d %s", code, raw)
	}
	if code, _, raw := call(owner, http.MethodGet, "/v1/tenant/sales/lines?kind=bogus", nil); code != http.StatusBadRequest {
		t.Fatalf("bad kind = %d %s", code, raw)
	}
	if code, data, raw := call(owner, http.MethodGet, "/v1/tenant/sales/reservations/"+res.UUID.String()+"/balance", nil); code != http.StatusOK ||
		data["expected"] != "70.00" || data["credit"] != "30.00" {
		t.Fatalf("reservation balance = %d %s", code, raw)
	}

	// 9. The manager takes the product back (TEC-913): the units return to
	// the branch and the sale line is reversed, once.
	products.SetSaleReverser(sales.ProductSaleReverser(svc))
	var pve *productsusecase.ValidationError
	if _, err := products.ReturnSale(ctx, org.ID, owner.ID, sold.Movements[0].UUID, productsmodel.ReturnInput{}); !errors.As(err, &pve) ||
		pve.Details[0].Field != "reason" {
		t.Fatalf("return without a reason = %v", err)
	}
	returned, err := products.ReturnSale(ctx, org.ID, owner.ID, sold.Movements[0].UUID, productsmodel.ReturnInput{Reason: "kusurlu"})
	if err != nil {
		t.Fatal(err)
	}
	if m := returned.Movements[0]; m.Kind != productsmodel.KindReturn || m.Quantity != 2 || m.BalanceAfter != 5 ||
		m.Source != productsmodel.SourceSaleReturn {
		t.Fatalf("return movement = %+v", m)
	}
	if got, err := products.GetMovement(ctx, org.ID, sold.Movements[0].UUID); err != nil || !got.Returned {
		t.Fatalf("returned sale = %+v %v", got, err)
	}
	if _, err := svc.LiveLineOfStockSale(ctx, q, org.ID, sold.Movements[0].UUID); !errors.Is(err, usecase.ErrLineNotFound) {
		t.Fatalf("live line after the return = %v", err)
	}
	if _, n, err := svc.ListLines(ctx, org.ID, model.LineFilter{ItemTypes: []string{model.ItemProduct}, Kinds: []string{model.LineReversal}, Limit: 10}); err != nil || n != 1 {
		t.Fatalf("product reversals = %d %v", n, err)
	}
	if _, err := products.ReturnSale(ctx, org.ID, owner.ID, sold.Movements[0].UUID, productsmodel.ReturnInput{Reason: "again"}); !errors.Is(err, productsusecase.ErrAlreadyReturned) {
		t.Fatalf("second return = %v", err)
	}
	if _, err := products.ReturnSale(ctx, org.ID, owner.ID, returned.Movements[0].UUID, productsmodel.ReturnInput{Reason: "x"}); !errors.Is(err, productsusecase.ErrNotReturnable) {
		t.Fatalf("return of a return = %v", err)
	}
	if _, err := products.ReturnSale(ctx, org2.ID, owner.ID, sold.Movements[0].UUID, productsmodel.ReturnInput{Reason: "x"}); !errors.Is(err, productsusecase.ErrMovementNotFound) {
		t.Fatalf("foreign return = %v", err)
	}
}
