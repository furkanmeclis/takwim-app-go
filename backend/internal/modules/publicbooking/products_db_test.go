package publicbooking_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicbooking/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// TestPublicBookingProducts (TEC-919): the product step lists active
// products shown online and sold in the branch with units available; a
// booking reserves the products it names; the last unit is not booked
// twice; hidden products are refused.
func TestPublicBookingProducts(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	base := f.base(f.org.Slug)
	var branchID, waxID, hiddenID, emptyID int64
	var wax, hidden uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT id FROM branches WHERE organization_id = $1 AND uuid = $2`, f.org.ID, f.branch).Scan(&branchID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO products (organization_id, name, sku, price) VALUES ($1, 'Wax', 'WAX', 100) RETURNING id, uuid`, f.org.ID).Scan(&waxID, &wax); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO products (organization_id, name, sku, price, show_online) VALUES ($1, 'Gizli', 'GZL', 10, false) RETURNING id, uuid`, f.org.ID).Scan(&hiddenID, &hidden); err != nil {
		t.Fatal(err)
	}
	var empty uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO products (organization_id, name, sku, price) VALUES ($1, 'Tükendi', 'TKN', 10) RETURNING id, uuid`, f.org.ID).Scan(&emptyID, &empty); err != nil {
		t.Fatal(err)
	}
	f.exec(`INSERT INTO product_branches (organization_id, product_id, branch_id, price) VALUES ($1, $2, $3, 80), ($1, $4, $3, NULL), ($1, $5, $3, NULL)`,
		f.org.ID, waxID, branchID, hiddenID, emptyID)
	f.exec(`INSERT INTO stock_levels (organization_id, branch_id, product_id, on_hand) VALUES ($1, $2, $3, 1), ($1, $2, $4, 3)`,
		f.org.ID, branchID, waxID, hiddenID)

	r := f.do(t, http.MethodGet, base+"/products?branch_uuid="+f.branch.String(), nil, nil)
	if r.code != http.StatusOK {
		t.Fatalf("products = %d %+v", r.code, r.env.Error)
	}
	list := decode[[]model.Product](t, r)
	if len(list) != 1 || list[0].ProductUUID != wax || !list[0].Price.Equal(money.MustParse("80")) || list[0].MaxQuantity != 1 {
		t.Fatalf("products = %+v", list)
	}
	for name, path := range map[string]string{
		"no branch":      base + "/products",
		"hidden branch":  base + "/products?branch_uuid=" + f.hidden.String(),
		"foreign branch": base + "/products?branch_uuid=" + f.branch2.String(),
	} {
		if r := f.do(t, http.MethodGet, path, nil, nil); r.code != http.StatusBadRequest {
			t.Fatalf("%s = %d", name, r.code)
		}
	}

	book := func(phone string, products []map[string]any) result {
		co := f.checkout(t, f.org.Slug, at(9, 0), nil)
		if co.code != http.StatusCreated {
			t.Fatalf("checkout = %d %+v", co.code, co.env.Error)
		}
		token := decode[model.Checkout](t, co).Token
		r := f.do(t, http.MethodPost, base+"/reservations", map[string]any{
			"checkout_token": token, "customer": customer(phone), "products": products,
		}, nil)
		if r.code != http.StatusCreated {
			// Free the held time for the next try.
			f.do(t, http.MethodDelete, base+"/checkouts/"+token.String(), nil, nil)
		}
		return r
	}
	if r := book("05321234567", []map[string]any{{"product_uuid": hidden, "quantity": 1}}); r.code != http.StatusBadRequest || r.detail() != "products[0].product_uuid/not_found" {
		t.Fatalf("hidden product = %d %s", r.code, r.detail())
	}
	if r := book("05321234567", []map[string]any{{"product_uuid": wax, "quantity": 2}}); r.code != http.StatusBadRequest || r.detail() != "products[0].quantity/insufficient_stock" {
		t.Fatalf("too many = %d %s", r.code, r.detail())
	}
	if n := f.count(`SELECT count(*) FROM reservations WHERE organization_id = $1`, f.org.ID); n != 0 {
		t.Fatalf("refused bookings wrote %d records", n)
	}
	r = book("05321234567", []map[string]any{{"product_uuid": wax, "quantity": 1}})
	if r.code != http.StatusCreated {
		t.Fatalf("booking = %d %+v", r.code, r.env.Error)
	}
	res := decode[model.Reservation](t, r)
	// Ayşe's cut 150 + the wax 80.
	if len(res.Products) != 1 || res.Products[0].Name != "Wax" || res.Products[0].Status != "held" || !res.TotalPrice.Equal(money.MustParse("230")) {
		t.Fatalf("booked = total %s, %+v", res.TotalPrice, res.Products)
	}
	if n := f.count(`SELECT reserved FROM stock_levels WHERE organization_id = $1 AND product_id = $2`, f.org.ID, waxID); n != 1 {
		t.Fatalf("reserved = %d", n)
	}
	// The last unit is gone from the product step.
	r = f.do(t, http.MethodGet, base+"/products?branch_uuid="+f.branch.String(), nil, nil)
	if list := decode[[]model.Product](t, r); len(list) != 0 {
		t.Fatalf("products after booking = %+v", list)
	}
}
