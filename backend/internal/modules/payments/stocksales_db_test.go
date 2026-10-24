package payments_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	cashrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/repository"
	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	paymentsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
	productsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/model"
	productsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/repository"
	productsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/usecase"
	salesmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales"
	salesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/repository"
	salesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// TestStockSaleMoney is the money of walk-in product sales (TEC-992) on
// Postgres: a collection with the sale and after it (in parts, never more
// than what is left), a refund with the return (over several collections)
// and after it (idempotency key replayed and reused, never more than what
// was collected), a till that cannot go below zero rolling the return
// back, and the branch and organization limits.
func TestStockSaleMoney(t *testing.T) {
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
		Email: "stock-owner-" + suffix + "@example.test", PasswordHash: "x", Name: "Owner", Surname: "Test", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Stock " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	org2, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Stock other " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		for _, id := range []int64{org.ID, org2.ID} {
			_, _ = pool.Exec(bg, "DELETE FROM activity_events WHERE organization_id = $1", id)
			_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", id)
		}
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", owner.ID)
	})
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
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
	balance := func(account uuid.UUID) money.Money {
		t.Helper()
		var n string
		if err := pool.QueryRow(ctx, `SELECT (a.opening_balance + COALESCE((
			SELECT SUM(CASE WHEN m.direction = 'in' THEN m.amount ELSE -m.amount END) FROM cash_movements m
			WHERE m.organization_id = a.organization_id AND m.cash_account_id = a.id), 0))::text
			FROM cash_accounts a WHERE a.organization_id = $1 AND a.uuid = $2`, org.ID, account).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return money.MustParse(n)
	}
	eq := func(what string, got money.Money, want int64) {
		t.Helper()
		if !got.Equal(money.FromUnits(want)) {
			t.Fatalf("%s = %s, want %d", what, got, want)
		}
	}

	branchID, branchUUID := idOf("SELECT id, uuid FROM branches WHERE organization_id = $1 ORDER BY id LIMIT 1", org.ID)
	_, otherBranchUUID := idOf(`INSERT INTO branches (organization_id, name, code) VALUES ($1, 'İkinci', 'B2') RETURNING id, uuid`, org.ID)
	_, tillUUID := idOf(`INSERT INTO cash_accounts (organization_id, branch_id, kind, name, currency, opening_date)
		VALUES ($1, $2, 'cash', 'Kasa', 'TRY', '2026-01-01') RETURNING id, uuid`, org.ID, branchID)
	_, bankUUID := idOf(`INSERT INTO cash_accounts (organization_id, branch_id, kind, name, currency, opening_date)
		VALUES ($1, NULL, 'bank', 'Banka', 'TRY', '2026-01-01') RETURNING id, uuid`, org.ID)
	_, emptyTillUUID := idOf(`INSERT INTO cash_accounts (organization_id, branch_id, kind, name, currency, opening_date)
		VALUES ($1, $2, 'cash', 'Boş kasa', 'TRY', '2026-01-01') RETURNING id, uuid`, org.ID, branchID)
	otherBranchID, _ := idOf("SELECT id, uuid FROM branches WHERE organization_id = $1 AND uuid = $2", org.ID, otherBranchUUID)
	_, otherTillUUID := idOf(`INSERT INTO cash_accounts (organization_id, branch_id, kind, name, currency, opening_date)
		VALUES ($1, $2, 'cash', 'Diğer kasa', 'TRY', '2026-01-01') RETURNING id, uuid`, org.ID, otherBranchID)
	productID, productUUID := idOf(`INSERT INTO products (organization_id, name, sku, price) VALUES ($1, 'Şampuan', 'SH-1', 400) RETURNING id, uuid`, org.ID)
	exec(`INSERT INTO product_branches (organization_id, product_id, branch_id) VALUES ($1, $2, $3)`, org.ID, productID, branchID)

	cashSvc := cashusecase.New(cashrepository.NewPostgres(pool, q), nil)
	salesSvc := salesusecase.New(salesrepository.NewPostgres(pool, q), cashSvc)
	svc := usecase.New(q, nil, salesSvc, nil, nil, usecase.Config{}, nil)
	products := productsusecase.New(productsrepository.NewPostgres(pool, q), nil)
	products.SetSaleRecorder(salesmodule.ProductSaleRecorder(salesSvc))
	products.SetSaleReverser(salesmodule.ProductSaleReverser(salesSvc))
	products.SetSaleCollector(paymentsmodule.StockSaleCollector(svc))
	products.SetSaleRefunder(paymentsmodule.StockSaleRefunder(svc))
	all := usecase.ViewOptions{Collect: true, Refund: true}

	sell := func(units int32, collection *productsmodel.SaleCollectionInput) (uuid.UUID, error) {
		t.Helper()
		res, err := products.RecordMovement(ctx, org.ID, owner.ID, productsmodel.MovementInput{
			Kind: productsmodel.KindSale, BranchUUID: branchUUID, ProductUUID: productUUID, Quantity: &units, Collection: collection,
		})
		if err != nil {
			return uuid.Nil, err
		}
		return res.Movements[0].UUID, nil
	}
	view := func(id uuid.UUID) model.StockSalePayments {
		t.Helper()
		v, err := svc.StockSalePayments(ctx, org.ID, id, all)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	if _, err := products.RecordMovement(ctx, org.ID, owner.ID, productsmodel.MovementInput{
		Kind: productsmodel.KindReceipt, BranchUUID: branchUUID, ProductUUID: productUUID, Quantity: ptr(int32(20)),
	}); err != nil {
		t.Fatal(err)
	}

	// 1. A sale paid in full at the counter: one collection into the till,
	// in the sale's transaction.
	var pve *productsusecase.ValidationError
	if _, err := products.RecordMovement(ctx, org.ID, owner.ID, productsmodel.MovementInput{
		Kind: productsmodel.KindReceipt, BranchUUID: branchUUID, ProductUUID: productUUID, Quantity: ptr(int32(1)),
		Collection: &productsmodel.SaleCollectionInput{Method: "cash", CashAccountUUID: tillUUID},
	}); !errors.As(err, &pve) || pve.Details[0].Field != "collection" {
		t.Fatalf("collection on a receipt = %v", err)
	}
	if _, err := sell(1, &productsmodel.SaleCollectionInput{Method: "cash", CashAccountUUID: otherTillUUID}); !errors.As(err, &pve) ||
		pve.Details[0].Field != "collection.cash_account_uuid" || pve.Details[0].Code != "not_found" {
		t.Fatalf("account of another branch = %v", err)
	}
	if _, err := sell(1, &productsmodel.SaleCollectionInput{Method: "card", CashAccountUUID: tillUUID, Amount: money.FromUnits(900)}); !errors.As(err, &pve) ||
		pve.Details[0].Field != "collection.amount" || pve.Details[0].Code != "exceeds_remaining" {
		t.Fatalf("more than the sale = %v", err)
	}
	saleA, err := sell(2, &productsmodel.SaleCollectionInput{Method: "cash", CashAccountUUID: tillUUID, Note: "peşin"})
	if err != nil {
		t.Fatal(err)
	}
	a := view(saleA)
	eq("A expected", a.Expected, 800)
	eq("A collected", a.Collected, 800)
	eq("A remaining", a.Remaining, 0)
	eq("A refundable", a.Refundable, 800)
	if a.CanCollect || !a.CanRefund || len(a.Entries) != 1 || len(a.Collections) != 1 || a.Movement.Quantity != 2 || a.Currency != "TRY" {
		t.Fatalf("A view = %+v", a)
	}
	eq("till after A", balance(tillUUID), 800)
	if m, err := products.GetMovement(ctx, org.ID, saleA); err != nil || m.Collected == nil || !m.Collected.Equal(money.FromUnits(800)) || !m.Refunded.IsZero() {
		t.Fatalf("A movement money = %+v %v", m, err)
	}
	// The sale-time key: the same sale never collects twice.
	if n := count(t, pool, "SELECT COUNT(*) FROM ledger_entries WHERE organization_id = $1 AND idempotency_key = $2", org.ID, "stock_sale:"+saleA.String()); n != 1 {
		t.Fatalf("sale-time collections = %d", n)
	}

	// 2. A sale paid later, in parts, never more than what is left.
	saleB, err := sell(1, nil)
	if err != nil {
		t.Fatal(err)
	}
	b := view(saleB)
	eq("B remaining", b.Remaining, 400)
	if !b.CanCollect || b.CanRefund || len(b.CollectAccounts) != 3 {
		t.Fatalf("B view = %+v", b)
	}
	collect := func(id uuid.UUID, amount int64, method string, account uuid.UUID) (model.StockSalePayments, error) {
		return svc.CollectStockSale(ctx, org.ID, id, owner.ID, all, model.CollectInput{
			Amount: money.FromUnits(amount), Method: method, CashAccountUUID: account,
		})
	}
	var ve *usecase.ValidationError
	if _, err := collect(saleB, 500, "cash", tillUUID); !errors.As(err, &ve) || ve.Details[0].Code != "exceeds_remaining" {
		t.Fatalf("over remaining = %v", err)
	}
	if _, err := collect(saleB, 100, "cash", otherTillUUID); !errors.As(err, &ve) || ve.Details[0].Code != "not_found" {
		t.Fatalf("other branch till = %v", err)
	}
	if _, err := collect(saleB, 100, "online", tillUUID); !errors.As(err, &ve) || ve.Details[0].Field != "method" {
		t.Fatalf("online at the counter = %v", err)
	}
	b, err = collect(saleB, 150, "cash", tillUUID)
	if err != nil {
		t.Fatal(err)
	}
	eq("B remaining after 150", b.Remaining, 250)
	b, err = collect(saleB, 250, "transfer", bankUUID)
	if err != nil {
		t.Fatal(err)
	}
	eq("B remaining after all", b.Remaining, 0)
	eq("B refundable", b.Refundable, 400)
	if b.CanCollect || len(b.Collections) != 2 {
		t.Fatalf("B paid view = %+v", b)
	}
	eq("bank after B", balance(bankUUID), 250)

	// 3. The return pays everything back from the till, over both
	// collections, in the return's transaction.
	if _, err := products.ReturnSale(ctx, org.ID, owner.ID, saleB, productsmodel.ReturnInput{
		Reason: "kusurlu", Refund: &productsmodel.SaleRefundInput{Method: "cash", CashAccountUUID: tillUUID, Amount: money.FromUnits(500)},
	}); !errors.As(err, &pve) || pve.Details[0].Field != "refund.amount" || pve.Details[0].Code != "exceeds_refundable" {
		t.Fatalf("refund over the collections = %v", err)
	}
	if m, err := products.GetMovement(ctx, org.ID, saleB); err != nil || m.Returned {
		t.Fatalf("refused return left the sale returned: %+v %v", m, err)
	}
	if _, err := products.ReturnSale(ctx, org.ID, owner.ID, saleB, productsmodel.ReturnInput{
		Reason: "kusurlu", Refund: &productsmodel.SaleRefundInput{Method: "cash", CashAccountUUID: tillUUID},
	}); err != nil {
		t.Fatal(err)
	}
	b = view(saleB)
	eq("B refunded", b.Refunded, 400)
	eq("B refundable after return", b.Refundable, 0)
	eq("B remaining after return", b.Remaining, 0)
	if !b.Movement.Returned || b.CanRefund || b.CanCollect || len(b.Entries) != 4 {
		t.Fatalf("B returned view = %+v", b)
	}
	for _, c := range b.Collections {
		if !c.Refundable.IsZero() || !c.Refunded.Equal(c.Amount) {
			t.Fatalf("B collection after refund = %+v", c)
		}
	}
	eq("till after B refund", balance(tillUUID), 800+150-400)
	if m, err := products.GetMovement(ctx, org.ID, saleB); err != nil || !m.Returned || !m.Refunded.Equal(money.FromUnits(400)) {
		t.Fatalf("B movement = %+v %v", m, err)
	}
	if _, err := products.ReturnSale(ctx, org.ID, owner.ID, saleB, productsmodel.ReturnInput{Reason: "again"}); !errors.Is(err, productsusecase.ErrAlreadyReturned) {
		t.Fatalf("second return = %v", err)
	}
	if _, err := collect(saleB, 10, "cash", tillUUID); !errors.Is(err, usecase.ErrNotCollectable) {
		t.Fatalf("collect after the return = %v", err)
	}

	// 4. A return without money, then refunds after it: a key is replayed,
	// a key with another amount refused, never more than collected.
	saleC, err := sell(1, &productsmodel.SaleCollectionInput{Method: "card", CashAccountUUID: bankUUID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := products.ReturnSale(ctx, org.ID, owner.ID, saleC, productsmodel.ReturnInput{Reason: "beğenmedi"}); err != nil {
		t.Fatal(err)
	}
	refund := func(id uuid.UUID, key string, amount int64, account uuid.UUID) (model.StockSaleRefundResult, error) {
		return svc.RefundStockSale(ctx, org.ID, id, owner.ID, all, model.StockSaleRefundInput{
			Amount: money.FromUnits(amount), Method: "cash", CashAccountUUID: account, Reason: "iade", IdempotencyKey: key,
		})
	}
	if _, err := refund(saleC, "", 100, tillUUID); !errors.As(err, &ve) || ve.Details[0].Field != "idempotency_key" {
		t.Fatalf("refund without a key = %v", err)
	}
	if _, err := refund(saleC, "c-big", 500, tillUUID); !errors.Is(err, salesusecase.ErrRefundTooLarge) {
		t.Fatalf("refund over collected = %v", err)
	}
	r, err := refund(saleC, "c-1", 100, tillUUID)
	if err != nil || r.Replayed {
		t.Fatalf("first refund = %+v %v", r, err)
	}
	eq("C refundable after 100", r.Payments.Refundable, 300)
	if r, err := refund(saleC, "c-1", 100, tillUUID); err != nil || !r.Replayed || !r.Payments.Refundable.Equal(money.FromUnits(300)) {
		t.Fatalf("replayed refund = %+v %v", r, err)
	}
	if _, err := refund(saleC, "c-1", 50, tillUUID); !errors.Is(err, usecase.ErrIdempotencyReused) {
		t.Fatalf("reused key = %v", err)
	}
	if r, err := refund(saleC, "c-rest", 0, tillUUID); err != nil || !r.Payments.Refundable.IsZero() || r.Payments.CanRefund {
		t.Fatalf("refund of the rest = %+v %v", r, err)
	}
	if _, err := refund(saleC, "c-none", 0, tillUUID); !errors.As(err, &ve) || ve.Details[0].Code != "exceeds_refundable" {
		t.Fatalf("refund with nothing left = %v", err)
	}
	eq("till after C", balance(tillUUID), 550-400)
	if n := count(t, pool, "SELECT COUNT(*) FROM ledger_entries WHERE organization_id = $1 AND kind = 'refund' AND source_uuid = $2", org.ID, saleC); n != 2 {
		t.Fatalf("C refund entries = %d", n)
	}

	// 5. A till that cannot go below zero rolls the whole return back.
	saleD, err := sell(1, &productsmodel.SaleCollectionInput{Method: "transfer", CashAccountUUID: bankUUID})
	if err != nil {
		t.Fatal(err)
	}
	var mc *productsusecase.MoneyConflict
	if _, err := products.ReturnSale(ctx, org.ID, owner.ID, saleD, productsmodel.ReturnInput{
		Reason: "kusurlu", Refund: &productsmodel.SaleRefundInput{Method: "cash", CashAccountUUID: emptyTillUUID},
	}); !errors.As(err, &mc) || mc.Code != "insufficient_funds" || mc.Field != "refund.cash_account_uuid" {
		t.Fatalf("refund from an empty till = %v", err)
	}
	if m, err := products.GetMovement(ctx, org.ID, saleD); err != nil || m.Returned || !m.Refunded.IsZero() {
		t.Fatalf("D after the refused return = %+v %v", m, err)
	}
	if _, err := salesSvc.LiveLineOfStockSale(ctx, q, org.ID, saleD); err != nil {
		t.Fatalf("D sale line after the refused return = %v", err)
	}
	if _, err := refund(saleD, "d-1", 100, emptyTillUUID); !errors.Is(err, cashusecase.ErrInsufficientFunds) {
		t.Fatalf("refund after from an empty till = %v", err)
	}

	// 6. Other movements and other businesses are not found; the accounts
	// of a branch.
	if _, err := svc.StockSalePayments(ctx, org.ID, uuid.New(), all); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("unknown movement = %v", err)
	}
	if _, err := svc.StockSalePayments(ctx, org2.ID, saleA, all); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("foreign movement = %v", err)
	}
	if _, err := svc.CollectStockSale(ctx, org2.ID, saleB, owner.ID, all, model.CollectInput{Amount: money.FromUnits(1), Method: "cash", CashAccountUUID: tillUUID}); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("foreign collect = %v", err)
	}
	receipt, err := products.RecordMovement(ctx, org.ID, owner.ID, productsmodel.MovementInput{
		Kind: productsmodel.KindReceipt, BranchUUID: branchUUID, ProductUUID: productUUID, Quantity: ptr(int32(1)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StockSalePayments(ctx, org.ID, receipt.Movements[0].UUID, all); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("payments of a receipt = %v", err)
	}
	accounts, err := svc.StockPaymentAccounts(ctx, org.ID, branchUUID, all)
	if err != nil || accounts.Currency != "TRY" || len(accounts.CollectAccounts) != 3 || len(accounts.RefundAccounts) != 3 {
		t.Fatalf("branch accounts = %+v %v", accounts, err)
	}
	if _, err := svc.StockPaymentAccounts(ctx, org2.ID, branchUUID, all); !errors.As(err, &ve) || ve.Details[0].Field != "branch_uuid" {
		t.Fatalf("foreign branch accounts = %v", err)
	}
}

func count(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

func ptr[T any](v T) *T { return &v }
