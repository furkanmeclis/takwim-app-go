package reservations_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// TestBranchProductOptions (TEC-977): the new reservation form lists a
// branch's stock items before the booking exists; another business's
// branch, an unknown one and one outside the branch scope are not found.
// The picked items are reserved with the booking in one transaction.
func TestBranchProductOptions(t *testing.T) {
	f := newProductFixture(t)
	ctx := context.Background()
	items, err := f.svc.BranchProductOptions(ctx, f.org.ID, f.branch, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("options = %+v", items)
	}
	if items, err = f.svc.BranchProductOptions(ctx, f.org.ID, f.branch, "wax"); err != nil || len(items) != 1 || items[0].Available != 2 {
		t.Fatalf("search = %+v, %v", items, err)
	}
	if _, err := f.svc.BranchProductOptions(ctx, f.org2.ID, f.branch, ""); !errors.Is(err, availabilityusecase.ErrNotFound) {
		t.Fatalf("other business = %v", err)
	}
	if _, err := f.svc.BranchProductOptions(ctx, f.org.ID, uuid.New(), ""); !errors.Is(err, availabilityusecase.ErrNotFound) {
		t.Fatalf("unknown branch = %v", err)
	}
	scoped := orgctx.WithScope(ctx, orgctx.Scope{ID: f.org.ID, BranchIDs: []int64{f.branchID + 1000}})
	if _, err := f.svc.BranchProductOptions(scoped, f.org.ID, f.branch, ""); !errors.Is(err, availabilityusecase.ErrNotFound) {
		t.Fatalf("outside the branch scope = %v", err)
	}

	// The form's products are reserved with the booking; not enough units
	// write nothing (no booking, no reservation of the other line).
	large := f.large
	in := f.input(at(9, 0), "", "", f.cut)
	in.Products = []model.ProductInput{f.productInput(f.wax, nil, 1), f.productInput(f.shampoo, &large, 2)}
	if _, err := f.svc.Create(ctx, f.org.ID, in, none, f.owner.ID); err == nil {
		t.Fatal("more than available accepted")
	} else if field, code := detailCode(err); field != "products[1].quantity" || code != "insufficient_stock" {
		t.Fatalf("too many = %s %s", field, code)
	}
	if on, res := f.stock(f.waxID, nil); on != 2 || res != 0 {
		t.Fatalf("wax stock after the refused booking = %d on hand, %d reserved", on, res)
	}
	in.Products[1].Quantity = 1
	r, err := f.svc.Create(ctx, f.org.ID, in, none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Cut 150 + Wax 120 + Şampuan 500 ml 150.
	if len(r.Products) != 2 || !r.TotalPrice.Equal(money.MustParse("420")) {
		t.Fatalf("booking = total %s, products %+v", r.TotalPrice, r.Products)
	}
	if items, err = f.svc.BranchProductOptions(ctx, f.org.ID, f.branch, "wax"); err != nil || len(items) != 1 || items[0].Available != 1 {
		t.Fatalf("wax after the booking = %+v, %v", items, err)
	}
}
