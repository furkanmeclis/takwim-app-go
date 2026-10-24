package reservations_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	cashrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/repository"
	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	orgsettingsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	salesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/repository"
	salesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// productFixture adds a catalog to the reservation fixture: Wax (list 100,
// branch price 120, cost 40, 2 on hand), Şampuan with the variants 250 ml
// (own price 90, none on hand) and 500 ml (product price 150, 1 on hand),
// and Gizli (not shown online, 5 on hand). The sales module posts the
// revenue of completed visits.
type productFixture struct {
	*fixture
	wax, shampoo, hidden  uuid.UUID
	small, large          uuid.UUID
	waxID, shampooID      int64
	largeID               int64
	sales                 *salesusecase.Service
	stock                 func(productID int64, variantID *int64) (onHand, reserved int)
	productInput          func(product uuid.UUID, variant *uuid.UUID, qty int) model.ProductInput
	branchID, ayseStaffID int64
}

func newProductFixture(t *testing.T) *productFixture {
	t.Helper()
	f := newFixture(t)
	ctx := context.Background()
	p := &productFixture{fixture: f}
	scan := func(dst []any, sql string, args ...any) {
		t.Helper()
		if err := f.pool.QueryRow(ctx, sql, args...).Scan(dst...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	scan([]any{&p.branchID}, `SELECT id FROM branches WHERE organization_id = $1 AND uuid = $2`, f.org.ID, f.branch)
	scan([]any{&p.ayseStaffID}, `SELECT id FROM staff WHERE organization_id = $1 AND name = 'Ayşe'`, f.org.ID)
	scan([]any{&p.waxID, &p.wax}, `INSERT INTO products (organization_id, name, sku, price, cost) VALUES ($1, 'Wax', 'WAX', 100, 40) RETURNING id, uuid`, f.org.ID)
	scan([]any{&p.shampooID, &p.shampoo}, `INSERT INTO products (organization_id, name, sku, price, cost) VALUES ($1, 'Şampuan', 'SMP', 150, 50) RETURNING id, uuid`, f.org.ID)
	var hiddenID, smallID int64
	scan([]any{&hiddenID, &p.hidden}, `INSERT INTO products (organization_id, name, sku, price, show_online) VALUES ($1, 'Gizli', 'GZL', 10, false) RETURNING id, uuid`, f.org.ID)
	scan([]any{&smallID, &p.small}, `INSERT INTO product_variants (organization_id, product_id, name, sku, price) VALUES ($1, $2, '250 ml', 'SMP-250', 90) RETURNING id, uuid`, f.org.ID, p.shampooID)
	scan([]any{&p.largeID, &p.large}, `INSERT INTO product_variants (organization_id, product_id, name, sku) VALUES ($1, $2, '500 ml', 'SMP-500') RETURNING id, uuid`, f.org.ID, p.shampooID)
	f.exec(`INSERT INTO product_branches (organization_id, product_id, branch_id, price) VALUES ($1, $2, $3, 120), ($1, $4, $3, NULL), ($1, $5, $3, NULL)`,
		f.org.ID, p.waxID, p.branchID, p.shampooID, hiddenID)
	f.exec(`INSERT INTO stock_levels (organization_id, branch_id, product_id, variant_id, on_hand) VALUES
		($1, $2, $3, NULL, 2), ($1, $2, $4, $5, 1), ($1, $2, $6, NULL, 5)`, f.org.ID, p.branchID, p.waxID, p.shampooID, p.largeID, hiddenID)
	p.stock = func(productID int64, variantID *int64) (int, int) {
		t.Helper()
		var onHand, reserved int
		scan([]any{&onHand, &reserved}, `SELECT on_hand, reserved FROM stock_levels WHERE organization_id = $1 AND branch_id = $2
			AND product_id = $3 AND variant_id IS NOT DISTINCT FROM $4`, f.org.ID, p.branchID, productID, variantID)
		return onHand, reserved
	}
	p.productInput = func(product uuid.UUID, variant *uuid.UUID, qty int) model.ProductInput {
		return model.ProductInput{ProductUUID: product, VariantUUID: variant, Quantity: qty}
	}
	clock := func() time.Time { return f.now }
	cash := cashusecase.New(cashrepository.NewPostgres(f.pool, f.q), nil)
	cash.SetClock(clock)
	p.sales = salesusecase.New(salesrepository.NewPostgres(f.pool, f.q), cash)
	p.sales.SetClock(clock)
	f.svc.SetCompletionHook(p.sales.RecordReservationTx)
	return p
}

func detailCode(err error) (string, string) {
	var ve *usecase.ValidationError
	if errors.As(err, &ve) && len(ve.Details) > 0 {
		return ve.Details[0].Field, ve.Details[0].Code
	}
	return "", ""
}

// TestReservationProducts (A18): products added at booking and later
// reserve their units under a lock; the last unit is never reserved twice;
// removing, cancelling and no-show release them; completing hands them
// over once (one stock sale, one product sale line of the visit).
func TestReservationProducts(t *testing.T) {
	f := newProductFixture(t)
	ctx := context.Background()
	large := f.large

	// Booking with a product: Ayşe's cut 150 + Wax at the branch price 120.
	in := f.input(at(9, 0), "", "", f.cut)
	in.Products = []model.ProductInput{f.productInput(f.wax, nil, 1)}
	r, err := f.svc.Create(ctx, f.org.ID, in, none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !r.TotalPrice.Equal(money.MustParse("270")) || len(r.Products) != 1 || r.Version != 1 {
		t.Fatalf("booking = total %s, products %+v, version %d", r.TotalPrice, r.Products, r.Version)
	}
	wax := r.Products[0]
	if wax.Status != model.ProductHeld || !wax.UnitPrice.Equal(money.MustParse("120")) || wax.Staff == nil || wax.Staff.Name != "Ayşe" ||
		wax.SKU != "WAX" || wax.Source != model.SourcePanel {
		t.Fatalf("product = %+v", wax)
	}
	if on, res := f.stock(f.waxID, nil); on != 2 || res != 1 {
		t.Fatalf("wax stock = %d on hand, %d reserved", on, res)
	}

	// Validation: variants, quantities, the available units.
	add := func(target uuid.UUID, p model.ProductInput) (model.Reservation, error) {
		return f.svc.AddProduct(ctx, f.org.ID, target, none, f.owner.ID, model.AddProductInput{ProductInput: p})
	}
	if _, err := add(r.UUID, f.productInput(f.shampoo, nil, 1)); err == nil {
		t.Fatal("product with variants without a variant")
	} else if field, code := detailCode(err); field != "variant_uuid" || code != "required" {
		t.Fatalf("no variant = %s %s", field, code)
	}
	if _, err := add(r.UUID, f.productInput(f.wax, nil, 0)); err == nil {
		t.Fatal("quantity 0 accepted")
	}
	if _, err := add(r.UUID, f.productInput(f.wax, nil, 2)); err == nil {
		t.Fatal("more than available accepted")
	} else if field, code := detailCode(err); field != "quantity" || code != "insufficient_stock" {
		t.Fatalf("too many = %s %s", field, code)
	}
	if _, err := add(r.UUID, model.ProductInput{ProductUUID: uuid.New(), Quantity: 1}); err == nil {
		t.Fatal("unknown product accepted")
	}
	// Online bookings only see products shown online.
	f.saveSettings(t, orgsettingsmodel.ApprovalAuto)
	online := f.input(at(11, 0), model.SourceOnline, model.MethodOnSite, f.cut)
	online.Products = []model.ProductInput{f.productInput(f.hidden, nil, 1)}
	if _, err := f.svc.Create(ctx, f.org.ID, online, none, 0); err == nil {
		t.Fatal("hidden product booked online")
	} else if field, code := detailCode(err); field != "products[0].product_uuid" || code != "not_found" {
		t.Fatalf("hidden online = %s %s", field, code)
	}
	if n := f.reservationsOf(); n != 1 {
		t.Fatalf("a refused booking wrote a record: %d", n)
	}

	// Adding and removing move the reservation, the total and the version.
	r, err = add(r.UUID, f.productInput(f.wax, nil, 1))
	if err != nil {
		t.Fatal(err)
	}
	if !r.TotalPrice.Equal(money.MustParse("390")) || len(r.Products) != 2 || r.Version != 2 || r.Events[len(r.Events)-1].Type != "product_added" {
		t.Fatalf("after add = total %s, %d products, version %d", r.TotalPrice, len(r.Products), r.Version)
	}
	if on, res := f.stock(f.waxID, nil); on != 2 || res != 2 {
		t.Fatalf("wax stock = %d / %d", on, res)
	}
	// Every unit is reserved: a counter sale cannot take them either.
	if _, err := add(r.UUID, f.productInput(f.wax, nil, 1)); err == nil {
		t.Fatal("third wax reserved")
	}
	second := r.Products[1].UUID
	r, err = f.svc.RemoveProduct(ctx, f.org.ID, r.UUID, second, none, f.owner.ID, usecase.ChangeInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !r.TotalPrice.Equal(money.MustParse("270")) || r.Products[1].Status != model.ProductReleased ||
		r.Products[1].ReleaseReason == nil || *r.Products[1].ReleaseReason != "removed" {
		t.Fatalf("after remove = total %s, %+v", r.TotalPrice, r.Products[1])
	}
	if _, err := f.svc.RemoveProduct(ctx, f.org.ID, r.UUID, second, none, f.owner.ID, usecase.ChangeInput{}); !errors.Is(err, usecase.ErrInvalidTransition) {
		t.Fatalf("remove twice = %v", err)
	}
	if _, err := f.svc.RemoveProduct(ctx, f.org.ID, r.UUID, uuid.New(), none, f.owner.ID, usecase.ChangeInput{}); !errors.Is(err, usecase.ErrProductNotFound) {
		t.Fatalf("remove unknown = %v", err)
	}
	stale := 1
	if _, err := f.svc.AddProduct(ctx, f.org.ID, r.UUID, none, f.owner.ID, model.AddProductInput{
		ProductInput: f.productInput(f.wax, nil, 1), ExpectedVersion: &stale,
	}); !errors.Is(err, usecase.ErrStaleVersion) {
		t.Fatalf("stale add = %v", err)
	}
	if on, res := f.stock(f.waxID, nil); on != 2 || res != 1 {
		t.Fatalf("wax stock after remove = %d / %d", on, res)
	}

	// Rescheduling keeps the products in the total.
	r, err = f.svc.Reschedule(ctx, f.org.ID, r.UUID, none, f.owner.ID, model.RescheduleInput{StartsAt: ptr(at(9, 30))})
	if err != nil {
		t.Fatal(err)
	}
	if !r.TotalPrice.Equal(money.MustParse("270")) {
		t.Fatalf("rescheduled total = %s", r.TotalPrice)
	}

	// Completing hands the held wax over once.
	r, err = f.svc.Complete(ctx, f.org.ID, r.UUID, none, f.owner.ID, usecase.ChangeInput{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Products[0].Status != model.ProductDelivered || r.Products[0].DeliveredAt == nil || r.Products[1].Status != model.ProductReleased {
		t.Fatalf("completed products = %+v", r.Products)
	}
	if on, res := f.stock(f.waxID, nil); on != 1 || res != 0 {
		t.Fatalf("wax stock after completion = %d / %d", on, res)
	}
	if n := f.count(`SELECT count(*) FROM stock_movements WHERE organization_id = $1 AND source = 'reservation' AND source_uuid = $2
		AND kind = 'sale' AND quantity = -1 AND balance_after = 1 AND staff_id = $3 AND unit_price = 120`, f.org.ID, r.UUID, f.ayseStaffID); n != 1 {
		t.Fatalf("sale movements = %d", n)
	}
	var resID int64
	if err := f.pool.QueryRow(ctx, `SELECT id FROM reservations WHERE organization_id = $1 AND uuid = $2`, f.org.ID, r.UUID).Scan(&resID); err != nil {
		t.Fatal(err)
	}
	productLines := func() int {
		return f.count(`SELECT count(*) FROM sale_lines WHERE organization_id = $1 AND reservation_id = $2 AND item_type = 'product'
			AND kind = 'sale' AND net_amount = 120 AND staff_id IS NOT NULL AND customer_id IS NOT NULL`, f.org.ID, resID)
	}
	if n := productLines(); n != 1 {
		t.Fatalf("product sale lines = %d", n)
	}
	// Posting again (a retried hook) adds nothing: no double revenue.
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.sales.RecordReservationTx(ctx, f.q.WithTx(tx), f.org.ID, resID, f.owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if n := productLines(); n != 1 {
		t.Fatalf("product sale lines after a repost = %d", n)
	}
	balance, err := f.sales.ReservationBalance(ctx, f.org.ID, r.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if !balance.Expected.Equal(money.MustParse("270")) || !balance.Remaining.Equal(money.MustParse("270")) {
		t.Fatalf("balance = expected %s, remaining %s", balance.Expected, balance.Remaining)
	}
	// A finished visit keeps its products.
	if _, err := add(r.UUID, f.productInput(f.wax, nil, 1)); !errors.Is(err, usecase.ErrInvalidTransition) {
		t.Fatalf("add to a completed visit = %v", err)
	}

	// Cancel and no-show release what was not handed over.
	in = f.input(at(10, 0), "", "", f.cut)
	in.Products = []model.ProductInput{f.productInput(f.shampoo, &large, 1)}
	c, err := f.svc.Create(ctx, f.org.ID, in, none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	// 500 ml has no own price and no branch price: the catalog price.
	if !c.Products[0].UnitPrice.Equal(money.MustParse("150")) || c.Products[0].Variant == nil || c.Products[0].SKU != "SMP-500" {
		t.Fatalf("variant product = %+v", c.Products[0])
	}
	if _, res := f.stock(f.shampooID, &f.largeID); res != 1 {
		t.Fatalf("500 ml reserved = %d", res)
	}
	c, err = f.svc.Cancel(ctx, f.org.ID, c.UUID, none, f.owner.ID, usecase.ChangeInput{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Products[0].Status != model.ProductReleased || *c.Products[0].ReleaseReason != "cancelled" {
		t.Fatalf("cancelled product = %+v", c.Products[0])
	}
	if on, res := f.stock(f.shampooID, &f.largeID); on != 1 || res != 0 {
		t.Fatalf("500 ml after cancel = %d / %d", on, res)
	}
	in = f.input(at(10, 30), "", "", f.cut)
	in.Products = []model.ProductInput{f.productInput(f.shampoo, &large, 1)}
	n, err := f.svc.Create(ctx, f.org.ID, in, none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	n, err = f.svc.NoShow(ctx, f.org.ID, n.UUID, none, f.owner.ID, usecase.ChangeInput{})
	if err != nil {
		t.Fatal(err)
	}
	if n.Products[0].Status != model.ProductReleased || *n.Products[0].ReleaseReason != "no_show" {
		t.Fatalf("no-show product = %+v", n.Products[0])
	}
	if _, res := f.stock(f.shampooID, &f.largeID); res != 0 {
		t.Fatalf("500 ml after no-show reserved = %d", res)
	}
	if n := f.count(`SELECT count(*) FROM stock_movements WHERE organization_id = $1 AND product_id = $2`, f.org.ID, f.shampooID); n != 0 {
		t.Fatalf("released products moved the stock: %d", n)
	}
}

// TestReservationProductsExpiry: a request keeps its products for as long
// as its timed hold; when the review window lapses the request is cancelled
// and its products are released in the same transaction.
func TestReservationProductsExpiry(t *testing.T) {
	f := newProductFixture(t)
	ctx := context.Background()
	f.saveSettings(t, orgsettingsmodel.ApprovalStaff)
	in := f.input(at(9, 0), model.SourceOnline, model.MethodOnSite, f.cut)
	in.Products = []model.ProductInput{f.productInput(f.wax, nil, 2)}
	r, err := f.svc.Create(ctx, f.org.ID, in, none, 0)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != model.StatusRequested || r.Products[0].Source != model.SourceOnline {
		t.Fatalf("request = %s, %+v", r.Status, r.Products)
	}
	if _, res := f.stock(f.waxID, nil); res != 2 {
		t.Fatalf("held = %d", res)
	}
	// The window (30 minutes) lapses; the next writer expires the request.
	f.now = at(8, 31)
	if _, err := f.svc.Create(ctx, f.org.ID, f.input(at(11, 0), "", "", f.cut), none, f.owner.ID); err != nil {
		t.Fatal(err)
	}
	r, err = f.svc.Get(ctx, f.org.ID, r.UUID, none)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != model.StatusCancelled || r.Products[0].Status != model.ProductReleased || *r.Products[0].ReleaseReason != "expired" {
		t.Fatalf("expired = %s, %+v", r.Status, r.Products)
	}
	if on, res := f.stock(f.waxID, nil); on != 2 || res != 0 {
		t.Fatalf("after expiry = %d / %d", on, res)
	}
}

// TestReservationProductsLastUnit: two visits racing for the last unit of
// a product; exactly one reserves it.
func TestReservationProductsLastUnit(t *testing.T) {
	f := newProductFixture(t)
	ctx := context.Background()
	large := f.large
	var visits []uuid.UUID
	for _, start := range []time.Time{at(9, 0), at(9, 30), at(10, 0), at(10, 30)} {
		r, err := f.svc.Create(ctx, f.org.ID, f.input(start, "", "", f.cut), none, f.owner.ID)
		if err != nil {
			t.Fatal(err)
		}
		visits = append(visits, r.UUID)
	}
	var wg sync.WaitGroup
	errs := make([]error, len(visits))
	for i, v := range visits {
		wg.Add(1)
		go func(i int, v uuid.UUID) {
			defer wg.Done()
			_, errs[i] = f.svc.AddProduct(ctx, f.org.ID, v, none, f.owner.ID, model.AddProductInput{
				ProductInput: f.productInput(f.shampoo, &large, 1),
			})
		}(i, v)
	}
	wg.Wait()
	won := 0
	for _, err := range errs {
		switch field, code := detailCode(err); {
		case err == nil:
			won++
		case field == "quantity" && code == "insufficient_stock":
		default:
			t.Fatalf("racing add = %v", err)
		}
	}
	if won != 1 {
		t.Fatalf("%d visits reserved the last unit", won)
	}
	if on, res := f.stock(f.shampooID, &f.largeID); on != 1 || res != 1 {
		t.Fatalf("last unit = %d / %d", on, res)
	}
	if n := f.count(`SELECT count(*) FROM reservation_products WHERE organization_id = $1 AND status = 'held'`, f.org.ID); n != 1 {
		t.Fatalf("held lines = %d", n)
	}
}

// TestReservationProductOptions: the picker lists the branch's items with
// their available units; the online list keeps products shown online with
// stock.
func TestReservationProductOptions(t *testing.T) {
	f := newProductFixture(t)
	ctx := context.Background()
	r, err := f.svc.Create(ctx, f.org.ID, f.input(at(9, 0), "", "", f.cut), none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	items, err := f.svc.ProductOptions(ctx, f.org.ID, r.UUID, none, "")
	if err != nil {
		t.Fatal(err)
	}
	// Gizli, Şampuan 250 ml, Şampuan 500 ml, Wax.
	if len(items) != 4 {
		t.Fatalf("options = %+v", items)
	}
	byName := map[string]model.ProductOption{}
	for _, it := range items {
		name := it.ProductName
		if it.VariantName != nil {
			name += " " + *it.VariantName
		}
		byName[name] = it
	}
	if w := byName["Wax"]; w.Available != 2 || !w.Price.Equal(money.MustParse("120")) {
		t.Fatalf("wax option = %+v", w)
	}
	if s := byName["Şampuan 250 ml"]; s.Available != 0 || !s.Price.Equal(money.MustParse("90")) {
		t.Fatalf("250 ml option = %+v", s)
	}
	if items, err = f.svc.ProductOptions(ctx, f.org.ID, r.UUID, none, "smp-5"); err != nil || len(items) != 1 {
		t.Fatalf("search = %+v, %v", items, err)
	}
	online, err := usecase.ListProductOptions(ctx, f.q, f.org.ID, f.branchID, "", true, 100)
	if err != nil {
		t.Fatal(err)
	}
	// Gizli is hidden online, 250 ml has nothing available.
	if len(online) != 2 {
		t.Fatalf("online options = %+v", online)
	}
	if _, err := f.svc.ProductOptions(ctx, f.org2.ID, r.UUID, none, ""); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("other business = %v", err)
	}
}
