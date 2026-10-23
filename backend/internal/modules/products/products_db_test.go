package products_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches"
	branchhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/handler"
	branchrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/repository"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations"
	orghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/handler"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products"
	productsadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/adapters"
	productshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/handler"
	productsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/repository"
	productsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff"
	staffhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/handler"
	staffrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/repository"
	staffusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

type mapLoader map[uuid.UUID]int64

func (m mapLoader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, _ := uuid.Parse(c.Subject)
	return authctx.Principal{UserID: id, UserInternal: m[id]}, nil
}

type result struct {
	code    int
	data    map[string]any
	details []any
	raw     string
}

// TestProductsAndStockOverHTTP drives the product catalog and the stock
// ledger (TEC-918) over HTTP with the real resolver and SQL: stock codes
// unique across products and variants, variants and the has_stock rule,
// offers in a branch manager's scope, receipts, sales (never below zero,
// only where the product is sold, credited to staff), counts, transfers,
// branch-scoped stock and ledger, reception selling but not receiving,
// concurrent sales of the last units, the append-only ledger and the
// import match.
// Runs when TEST_DATABASE_URL points at a migrated database.
func TestProductsAndStockOverHTTP(t *testing.T) {
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
	owner, carol, dave := newUser("owner"), newUser("carol"), newUser("dave")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Products " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	if org, err = orgSvc.SetEntitlements(ctx, org.UUID, orgmodel.SetEntitlementsInput{Plan: "growth"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM activity_events WHERE organization_id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{owner.ID, carol.ID, dave.ID})
	})
	if _, err := orgSvc.AddMember(ctx, org.UUID, carol.Uuid, "sube_yoneticisi"); err != nil {
		t.Fatal(err)
	}
	if _, err := orgSvc.AddMember(ctx, org.UUID, dave.Uuid, "resepsiyon"); err != nil {
		t.Fatal(err)
	}

	rec := activity.NewRecorder(q, nil)
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	loader := mapLoader{owner.Uuid: owner.ID, carol.Uuid: carol.ID, dave.Uuid: dave.ID}
	svc := productsusecase.New(productsrepository.NewPostgres(pool, q), nil)
	mux := http.NewServeMux()
	branches.RegisterRoutes(mux, branchhandler.New(branchusecase.New(branchrepository.NewPostgres(q), nil), rec), branches.EngineHandlers{}, tokens, loader, orgSvc)
	organizations.RegisterTenantRoutes(mux, orghandler.New(orgSvc, rec), tokens, loader, orgSvc)
	staff.RegisterRoutes(mux, staffhandler.New(staffusecase.New(staffrepository.NewPostgres(pool, q), nil), rec), staff.EngineHandlers{}, tokens, loader, orgSvc)
	products.RegisterRoutes(mux, productshandler.New(svc, rec), products.EngineHandlers{}, tokens, loader, orgSvc)

	call := func(user db.User, method, path string, body any) result {
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
			Data  map[string]any `json:"data"`
			Error struct {
				Details []any `json:"details"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		return result{code: rr.Code, data: env.Data, details: env.Error.Details, raw: rr.Body.String()}
	}
	want := func(r result, code int, step string) result {
		t.Helper()
		if r.code != code {
			t.Fatalf("%s = %d, want %d: %s", step, r.code, code, r.raw)
		}
		return r
	}
	hasDetail := func(r result, field, code string) bool {
		for _, raw := range r.details {
			d := raw.(map[string]any)
			if d["field"] == field && (code == "" || d["code"] == code) {
				return true
			}
		}
		return false
	}
	total := func(r result) int {
		return int(r.data["total"].(float64))
	}
	stock := func(r result) map[string]any {
		return r.data["stock"].(map[string]any)
	}
	move := func(user db.User, body map[string]any) result {
		t.Helper()
		return call(user, http.MethodPost, "/v1/tenant/stock-movements", body)
	}

	// Branch A is the default "Merkez"; B is added; carol manages A only.
	list := want(call(owner, http.MethodGet, "/v1/tenant/branches", nil), http.StatusOK, "branches")
	aUUID := list.data["items"].([]any)[0].(map[string]any)["uuid"].(string)
	bUUID := want(call(owner, http.MethodPost, "/v1/tenant/branches", map[string]any{"code": "KDK", "name": "Kadıköy"}),
		http.StatusCreated, "create B").data["uuid"].(string)
	want(call(owner, http.MethodPatch, "/v1/tenant/members/"+carol.Uuid.String(), map[string]any{
		"branch_scope": "selected", "branch_uuids": []string{aUUID},
	}), http.StatusOK, "narrow carol")
	stUUID := want(call(owner, http.MethodPost, "/v1/tenant/staff", map[string]any{"name": "Ayşe", "branch_uuids": []string{aUUID}}),
		http.StatusCreated, "create staff").data["uuid"].(string)

	// A shampoo sold in A only.
	p := want(call(owner, http.MethodPost, "/v1/tenant/products", map[string]any{
		"name": "Şampuan", "sku": "SMP-1", "barcode": "8690001", "unit": "bottle", "price": "300.00", "cost": "150",
		"low_stock_threshold": 2, "branch_uuids": []string{aUUID},
	}), http.StatusCreated, "create product")
	pUUID := p.data["uuid"].(string)
	if p.data["price"] != "300.00" || p.data["cost"] != "150.00" || stock(p)["on_hand"].(float64) != 0 ||
		stock(p)["status"] != "out" || len(p.data["offers"].([]any)) != 1 {
		t.Fatalf("product = %s", p.raw)
	}

	// Validation and stock codes unique (case-insensitive).
	r := want(call(owner, http.MethodPost, "/v1/tenant/products", map[string]any{
		"name": "X", "sku": "A B", "price": "-1", "unit": "kg",
	}), http.StatusBadRequest, "invalid product")
	for _, f := range []string{"sku", "price", "unit"} {
		if !hasDetail(r, f, "") {
			t.Fatalf("no %s detail: %s", f, r.raw)
		}
	}
	want(call(owner, http.MethodPost, "/v1/tenant/products", map[string]any{"name": "Y", "sku": "smp-1", "price": "1"}),
		http.StatusConflict, "duplicate sku")

	// Receipt in A, sales never below zero.
	r = want(move(owner, map[string]any{"kind": "receipt", "branch_uuid": aUUID, "product_uuid": pUUID, "quantity": 5, "unit_cost": "140", "reason": "Tedarikçi"}),
		http.StatusCreated, "receipt")
	if m := r.data["items"].([]any)[0].(map[string]any); m["quantity"].(float64) != 5 || m["balance_after"].(float64) != 5 || m["unit_cost"] != "140.00" {
		t.Fatalf("receipt = %s", r.raw)
	}
	r = want(move(owner, map[string]any{"kind": "sale", "branch_uuid": aUUID, "product_uuid": pUUID, "quantity": 6}),
		http.StatusBadRequest, "oversell")
	if !hasDetail(r, "quantity", "insufficient_stock") {
		t.Fatalf("oversell detail: %s", r.raw)
	}
	r = want(move(owner, map[string]any{"kind": "sale", "branch_uuid": aUUID, "product_uuid": pUUID, "quantity": 2, "staff_uuid": stUUID}),
		http.StatusCreated, "sale")
	m := r.data["items"].([]any)[0].(map[string]any)
	if m["quantity"].(float64) != -2 || m["balance_after"].(float64) != 3 || m["unit_price"] != "300.00" ||
		m["staff"].(map[string]any)["uuid"] != stUUID || m["created_by"] == nil {
		t.Fatalf("sale = %s", r.raw)
	}
	// Not sold in B: the sale is refused there; receipts are fine.
	want(move(owner, map[string]any{"kind": "receipt", "branch_uuid": bUUID, "product_uuid": pUUID, "quantity": 1}),
		http.StatusCreated, "receipt B")
	r = want(move(owner, map[string]any{"kind": "sale", "branch_uuid": bUUID, "product_uuid": pUUID, "quantity": 1}),
		http.StatusBadRequest, "sale B")
	if !hasDetail(r, "branch_uuid", "not_offered") {
		t.Fatalf("not offered detail: %s", r.raw)
	}

	// Transfer A → B (both legs), then a count in B.
	r = want(move(owner, map[string]any{"kind": "transfer", "branch_uuid": aUUID, "to_branch_uuid": bUUID, "product_uuid": pUUID, "quantity": 2}),
		http.StatusCreated, "transfer")
	if items := r.data["items"].([]any); len(items) != 2 || items[0].(map[string]any)["kind"] != "transfer_out" ||
		items[1].(map[string]any)["balance_after"].(float64) != 3 || items[0].(map[string]any)["transfer_uuid"] != items[1].(map[string]any)["transfer_uuid"] {
		t.Fatalf("transfer = %s", r.raw)
	}
	r = want(move(owner, map[string]any{"kind": "adjustment", "branch_uuid": bUUID, "product_uuid": pUUID, "counted_quantity": 1, "reason": "Sayım"}),
		http.StatusCreated, "count")
	if m := r.data["items"].([]any)[0].(map[string]any); m["quantity"].(float64) != -2 || m["balance_after"].(float64) != 1 {
		t.Fatalf("count = %s", r.raw)
	}
	r = want(call(owner, http.MethodGet, "/v1/tenant/products/"+pUUID, nil), http.StatusOK, "detail")
	if st := stock(r); st["on_hand"].(float64) != 2 || st["status"] != "low" || len(r.data["levels"].([]any)) != 2 {
		t.Fatalf("detail stock = %s", r.raw)
	}

	// Carol (A only) sees A's stock and ledger, cannot record in B.
	r = want(call(carol, http.MethodGet, "/v1/tenant/products/"+pUUID, nil), http.StatusOK, "carol detail")
	if st := stock(r); st["on_hand"].(float64) != 1 || len(r.data["levels"].([]any)) != 1 {
		t.Fatalf("carol stock = %s", r.raw)
	}
	r = want(call(carol, http.MethodGet, "/v1/tenant/stock-movements", nil), http.StatusOK, "carol ledger")
	if total(r) != 3 { // receipt, sale, transfer_out in A
		t.Fatalf("carol ledger = %d: %s", total(r), r.raw)
	}
	r = want(move(carol, map[string]any{"kind": "receipt", "branch_uuid": bUUID, "product_uuid": pUUID, "quantity": 1}),
		http.StatusBadRequest, "carol in B")
	if !hasDetail(r, "branch_uuid", "not_found") {
		t.Fatalf("carol B detail: %s", r.raw)
	}
	want(move(carol, map[string]any{"kind": "transfer", "branch_uuid": aUUID, "to_branch_uuid": bUUID, "product_uuid": pUUID, "quantity": 1}),
		http.StatusBadRequest, "carol transfer to B")

	// Reception sells but does not receive or count.
	want(move(dave, map[string]any{"kind": "receipt", "branch_uuid": aUUID, "product_uuid": pUUID, "quantity": 1}),
		http.StatusForbidden, "dave receipt")
	want(move(dave, map[string]any{"kind": "sale", "branch_uuid": aUUID, "product_uuid": pUUID, "quantity": 1}),
		http.StatusCreated, "dave sale")

	// Two concurrent sales of the last two units: at most two succeed.
	want(move(owner, map[string]any{"kind": "receipt", "branch_uuid": aUUID, "product_uuid": pUUID, "quantity": 2}),
		http.StatusCreated, "restock")
	var wg sync.WaitGroup
	codes := make(chan int, 6)
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- move(owner, map[string]any{"kind": "sale", "branch_uuid": aUUID, "product_uuid": pUUID, "quantity": 1}).code
		}()
	}
	wg.Wait()
	close(codes)
	ok := 0
	for c := range codes {
		if c == http.StatusCreated {
			ok++
		}
	}
	if ok != 2 {
		t.Fatalf("concurrent sales succeeded %d times, want 2", ok)
	}
	r = want(call(owner, http.MethodGet, "/v1/tenant/stock-movements?product="+pUUID+"&branch="+aUUID+"&limit=1", nil), http.StatusOK, "last")
	if m := r.data["items"].([]any)[0].(map[string]any); m["balance_after"].(float64) != 0 {
		t.Fatalf("last balance = %s", r.raw)
	}

	// Stock at zero in A, still 1 in B: the product cannot be deleted.
	want(call(owner, http.MethodDelete, "/v1/tenant/products/"+pUUID, nil), http.StatusConflict, "delete with stock")

	// Variants: the first one needs the product's own stock at zero.
	want(call(owner, http.MethodPost, "/v1/tenant/products/"+pUUID+"/variants", map[string]any{"name": "500 ml", "sku": "SMP-1-500"}),
		http.StatusConflict, "variant with product stock")
	p2 := want(call(owner, http.MethodPost, "/v1/tenant/products", map[string]any{
		"name": "Boya", "sku": "BOYA", "price": "500", "branch_uuids": []string{aUUID, bUUID},
	}), http.StatusCreated, "create p2")
	p2UUID := p2.data["uuid"].(string)
	want(call(owner, http.MethodPost, "/v1/tenant/products/"+p2UUID+"/variants", map[string]any{"name": "Kızıl", "sku": "smp-1"}),
		http.StatusConflict, "variant sku taken by product")
	r = want(call(owner, http.MethodPost, "/v1/tenant/products/"+p2UUID+"/variants", map[string]any{
		"name": "Kızıl", "sku": "BOYA-KZL", "barcode": "8690002", "price": "550",
	}), http.StatusCreated, "variant")
	v := r.data["variants"].([]any)[0].(map[string]any)
	vUUID := v["uuid"].(string)
	if v["effective_price"] != "550.00" || v["effective_cost"] != "0.00" {
		t.Fatalf("variant = %s", r.raw)
	}
	r = want(move(owner, map[string]any{"kind": "receipt", "branch_uuid": aUUID, "product_uuid": p2UUID, "quantity": 3}),
		http.StatusBadRequest, "receipt without variant")
	if !hasDetail(r, "variant_uuid", "required") {
		t.Fatalf("variant required detail: %s", r.raw)
	}
	want(move(owner, map[string]any{"kind": "receipt", "branch_uuid": aUUID, "product_uuid": p2UUID, "variant_uuid": vUUID, "quantity": 3}),
		http.StatusCreated, "variant receipt")
	r = want(move(owner, map[string]any{"kind": "sale", "branch_uuid": aUUID, "product_uuid": p2UUID, "variant_uuid": vUUID, "quantity": 1}),
		http.StatusCreated, "variant sale")
	if m := r.data["items"].([]any)[0].(map[string]any); m["unit_price"] != "550.00" || m["variant"].(map[string]any)["uuid"] != vUUID {
		t.Fatalf("variant sale = %s", r.raw)
	}
	want(call(owner, http.MethodDelete, "/v1/tenant/products/"+p2UUID+"/variants/"+vUUID, nil), http.StatusConflict, "delete variant with stock")
	r = want(call(owner, http.MethodPatch, "/v1/tenant/products/"+p2UUID+"/variants/"+vUUID, map[string]any{"price": nil, "name": "Kızıl kahve"}),
		http.StatusOK, "patch variant")
	if v := r.data["variants"].([]any)[0].(map[string]any); v["price"] != nil || v["effective_price"] != "500.00" || v["name"] != "Kızıl kahve" {
		t.Fatalf("patched variant = %s", r.raw)
	}

	// Barcode lookup finds the variant.
	r = want(call(owner, http.MethodGet, "/v1/tenant/products/lookup?code=8690002", nil), http.StatusOK, "lookup")
	if items := r.data["items"].([]any); len(items) != 1 || items[0].(map[string]any)["variant"].(map[string]any)["uuid"] != vUUID {
		t.Fatalf("lookup = %s", r.raw)
	}

	// Offers: carol prices A only.
	r = want(call(carol, http.MethodPut, "/v1/tenant/products/"+p2UUID+"/offers", map[string]any{
		"offers": []map[string]any{{"branch_uuid": bUUID}},
	}), http.StatusBadRequest, "carol offers B")
	if !hasDetail(r, "offers.0.branch_uuid", "not_found") {
		t.Fatalf("offer detail: %s", r.raw)
	}
	r = want(call(carol, http.MethodPut, "/v1/tenant/products/"+p2UUID+"/offers", map[string]any{
		"offers": []map[string]any{{"branch_uuid": aUUID, "price": "480"}},
	}), http.StatusOK, "carol offers A")
	if len(r.data["offers"].([]any)) != 2 {
		t.Fatalf("offers = %s", r.raw)
	}

	// Filters.
	r = want(call(owner, http.MethodGet, "/v1/tenant/products?stock_status=low", nil), http.StatusOK, "low filter")
	if total(r) != 1 || r.data["items"].([]any)[0].(map[string]any)["uuid"] != pUUID {
		t.Fatalf("low = %s", r.raw)
	}
	r = want(call(owner, http.MethodGet, "/v1/tenant/products?q=BOYA-KZL", nil), http.StatusOK, "variant search")
	if total(r) != 1 {
		t.Fatalf("variant search = %s", r.raw)
	}
	r = want(call(owner, http.MethodGet, "/v1/tenant/stock-movements?kind=sale&staff="+stUUID, nil), http.StatusOK, "staff sales")
	if total(r) != 1 {
		t.Fatalf("staff sales = %s", r.raw)
	}
	r = want(call(owner, http.MethodGet, "/v1/tenant/stock-movements/facets", nil), http.StatusOK, "movement facets")
	if len(r.data["kind"].([]any)) != 5 {
		t.Fatalf("movement facets = %s", r.raw)
	}

	// The ledger is append-only.
	if _, err := pool.Exec(ctx, "UPDATE stock_movements SET quantity = 99 WHERE organization_id = $1", org.ID); err == nil {
		t.Fatal("ledger update succeeded")
	}

	// Import: the stock code matches (case-insensitive).
	io := productsadapters.NewIO(svc)
	scoped := orgctx.WithOrganizationID(ctx, org.ID)
	res, err := io.ApplyRow(scoped, map[string]any{"sku": "boya", "name": "Saç boyası", "price": "520,50"}, nil)
	if err != nil || !res.OK || res.EntityUUID != p2UUID || res.Op != "update" {
		t.Fatalf("import update = %+v, %v", res, err)
	}
	res, err = io.ApplyRow(scoped, map[string]any{"sku": "TRK-1", "name": "Tarak", "price": "40", "branch_codes": "KDK"}, nil)
	if err != nil || !res.OK || res.Op != "create" {
		t.Fatalf("import create = %+v, %v", res, err)
	}
	r = want(call(owner, http.MethodGet, "/v1/tenant/products/"+p2UUID, nil), http.StatusOK, "updated by import")
	if r.data["price"] != "520.50" || r.data["name"] != "Saç boyası" {
		t.Fatalf("import update = %s", r.raw)
	}

	// Activity holds catalog changes and movements; a product without
	// stock can be deleted.
	r = want(call(owner, http.MethodGet, "/v1/tenant/products/"+pUUID+"/activity?limit=100", nil), http.StatusOK, "activity")
	if total(r) < 8 {
		t.Fatalf("activity = %d", total(r))
	}
	want(call(owner, http.MethodDelete, "/v1/tenant/products/"+res.EntityUUID, nil), http.StatusOK, "delete")
	want(call(owner, http.MethodGet, "/v1/tenant/products/"+res.EntityUUID, nil), http.StatusNotFound, "deleted")
}
