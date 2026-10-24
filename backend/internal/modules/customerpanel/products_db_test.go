package customerpanel_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	availabilitymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/model"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
)

// TestCustomerPanelProducts (TEC-977): the visit shows the products taken
// with it (name, variant, quantity, amounts, state) and nothing internal
// (stock code, cost, seller). A line the business removed is left out;
// cancelling the visit shows the rest as released. Another business's
// visit of the same person carries only its own products.
func TestCustomerPanelProducts(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	var branchID int64
	if err := f.pool.QueryRow(ctx, `SELECT id FROM branches WHERE organization_id = $1 AND uuid = $2`, f.orgA.ID, f.branchA).Scan(&branchID); err != nil {
		t.Fatal(err)
	}
	product := func(name, sku string) (int64, uuid.UUID) {
		t.Helper()
		var id int64
		var u uuid.UUID
		if err := f.pool.QueryRow(ctx, `INSERT INTO products (organization_id, name, sku, price, cost) VALUES ($1, $2, $3, 50, 20) RETURNING id, uuid`,
			f.orgA.ID, name, sku).Scan(&id, &u); err != nil {
			t.Fatal(err)
		}
		f.exec(`INSERT INTO product_branches (organization_id, product_id, branch_id) VALUES ($1, $2, $3)`, f.orgA.ID, id, branchID)
		f.exec(`INSERT INTO stock_levels (organization_id, branch_id, product_id, on_hand) VALUES ($1, $2, $3, 5)`, f.orgA.ID, branchID, id)
		return id, u
	}
	_, wax := product("Wax", "GIZLI-WAX-KODU")
	_, comb := product("Tarak", "GIZLI-TARAK-KODU")

	start := at(11, 0)
	staff := f.ayseA
	r, err := f.reservations.Create(ctx, f.orgA.ID, reservationsmodel.CreateInput{
		BranchUUID: f.branchA, CustomerUUID: f.huseyinA, StartsAt: &start,
		Lines: []availabilitymodel.LineInput{{ServiceUUID: f.cutA, StaffUUID: &staff}},
		Products: []reservationsmodel.ProductInput{
			{ProductUUID: wax, Quantity: 2}, {ProductUUID: comb, Quantity: 1},
		},
	}, availabilityusecase.Caller{}, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	var combLine uuid.UUID
	for _, p := range r.Products {
		if p.Product.UUID == comb {
			combLine = p.UUID
		}
	}
	if _, err := f.reservations.RemoveProduct(ctx, f.orgA.ID, r.UUID, combLine, availabilityusecase.Caller{}, f.owner.ID, reservationsusecase.ChangeInput{}); err != nil {
		t.Fatal(err)
	}
	other := f.book(t, f.orgB.ID, f.branchB, f.cutB, f.ayseB, f.huseyinB, at(11, 0))

	token := f.signIn(t, f.huseyinPhone, f.huseyinPhone).AccessToken
	res := f.do(t, http.MethodGet, "/v1/customer/reservations/"+r.UUID.String(), token, nil)
	if res.code != http.StatusOK {
		t.Fatalf("detail = %d %s", res.code, res.raw)
	}
	for _, secret := range []string{"GIZLI-WAX-KODU", "GIZLI-TARAK-KODU", "Tarak", `"cost"`, `"sku"`, `"release_reason"`} {
		if strings.Contains(res.raw, secret) {
			t.Fatalf("detail shows %q: %s", secret, res.raw)
		}
	}
	got := decode[model.Reservation](t, res)
	if len(got.Products) != 1 {
		t.Fatalf("products = %+v", got.Products)
	}
	p := got.Products[0]
	if p.Name != "Wax" || p.VariantName != nil || p.Quantity != 2 || p.UnitPrice.String() != "50.00" || p.Total.String() != "100.00" ||
		p.Status != reservationsmodel.ProductHeld {
		t.Fatalf("product = %+v", p)
	}
	// Kesim 100 + Wax 2 × 50.
	if got.Money.Total.String() != "200.00" {
		t.Fatalf("total = %s", got.Money.Total)
	}

	// The other business's visit carries none of A's products.
	if b := decode[model.Reservation](t, f.do(t, http.MethodGet, "/v1/customer/reservations/"+other.UUID.String(), token, nil)); len(b.Products) != 0 {
		t.Fatalf("other business products = %+v", b.Products)
	}

	// Cancelled by the customer: the products are released and shown so.
	cancel := f.do(t, http.MethodPost, "/v1/customer/reservations/"+r.UUID.String()+"/cancel", token, map[string]any{"expected_version": got.Version})
	if cancel.code != http.StatusOK {
		t.Fatalf("cancel = %d %s", cancel.code, cancel.raw)
	}
	cancelled := decode[model.Reservation](t, cancel)
	if len(cancelled.Products) != 1 || cancelled.Products[0].Status != reservationsmodel.ProductReleased {
		t.Fatalf("after cancel = %+v", cancelled.Products)
	}
}
