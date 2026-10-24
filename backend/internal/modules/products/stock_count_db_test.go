package products_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

// TestStockCountImportAndExport drives the stock ledger engines (TEC-964)
// with the real SQL: a count row (branch code, stock code, counted units)
// writes one adjustment of the requester, stock codes match SKUs and
// barcodes of products and variants, the requester's branch scope and the
// count limits hold, the dry run shows the stock change, a rollback writes
// the opposite adjustment (refused once the units have left the stock)
// and the export honours the list filters.
// Runs when TEST_DATABASE_URL points at a migrated database.
func TestStockCountImportAndExport(t *testing.T) {
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
			Email: name + "-count-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	owner, carol := newUser("owner"), newUser("carol")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Counts " + suffix, OwnerUserID: owner.ID})
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
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{owner.ID, carol.ID})
	})
	if _, err := orgSvc.AddMember(ctx, org.UUID, carol.Uuid, "sube_yoneticisi"); err != nil {
		t.Fatal(err)
	}

	rec := activity.NewRecorder(q, nil)
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	loader := mapLoader{owner.Uuid: owner.ID, carol.Uuid: carol.ID}
	svc := productsusecase.New(productsrepository.NewPostgres(pool, q), nil)
	mux := http.NewServeMux()
	branches.RegisterRoutes(mux, branchhandler.New(branchusecase.New(branchrepository.NewPostgres(q), nil), rec), branches.EngineHandlers{}, tokens, loader, orgSvc)
	organizations.RegisterTenantRoutes(mux, orghandler.New(orgSvc, rec), tokens, loader, orgSvc)
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
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		return result{code: rr.Code, data: env.Data, raw: rr.Body.String()}
	}
	want := func(r result, code int, step string) result {
		t.Helper()
		if r.code != code {
			t.Fatalf("%s = %d, want %d: %s", step, r.code, code, r.raw)
		}
		return r
	}

	// Branch A (default) and B; carol manages A only.
	list := want(call(owner, http.MethodGet, "/v1/tenant/branches", nil), http.StatusOK, "branches")
	branchA := list.data["items"].([]any)[0].(map[string]any)
	aUUID, aCode := branchA["uuid"].(string), branchA["code"].(string)
	bUUID := want(call(owner, http.MethodPost, "/v1/tenant/branches", map[string]any{"code": "SYM", "name": "Sayım B"}),
		http.StatusCreated, "create B").data["uuid"].(string)
	want(call(owner, http.MethodPatch, "/v1/tenant/members/"+carol.Uuid.String(), map[string]any{
		"branch_scope": "selected", "branch_uuids": []string{aUUID},
	}), http.StatusOK, "narrow carol")

	// A plain product with 5 in A, a product with one variant.
	pUUID := want(call(owner, http.MethodPost, "/v1/tenant/products", map[string]any{
		"name": "Krem", "sku": "CNT-1", "price": "100", "branch_uuids": []string{aUUID, bUUID},
	}), http.StatusCreated, "product").data["uuid"].(string)
	want(call(owner, http.MethodPost, "/v1/tenant/stock-movements", map[string]any{
		"kind": "receipt", "branch_uuid": aUUID, "product_uuid": pUUID, "quantity": 5,
	}), http.StatusCreated, "receipt")
	p2UUID := want(call(owner, http.MethodPost, "/v1/tenant/products", map[string]any{
		"name": "Boya", "sku": "CNT-V", "price": "200", "branch_uuids": []string{aUUID},
	}), http.StatusCreated, "variant product").data["uuid"].(string)
	v := want(call(owner, http.MethodPost, "/v1/tenant/products/"+p2UUID+"/variants", map[string]any{
		"name": "Kızıl", "sku": "CNT-V-R", "barcode": "8691964",
	}), http.StatusCreated, "variant")
	vUUID := v.data["variants"].([]any)[0].(map[string]any)["uuid"].(string)

	io := productsadapters.NewMovementsIO(svc)
	ownerCtx, err := orgctx.WithJobScope(ctx, orgSvc, org.ID, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	carolCtx, err := orgctx.WithJobScope(ctx, orgSvc, org.ID, carol.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Dry run: 5 on hand become 3 (stock code case-insensitive).
	diff, err := io.PreviewRow(ownerCtx, map[string]any{"branch_code": strings.ToLower(aCode), "stock_code": "cnt-1", "counted_quantity": "3"}, nil)
	if err != nil || diff.Error != "" || len(diff.Changes) != 1 || diff.Changes[0].From != "5" || diff.Changes[0].To != "3" {
		t.Fatalf("preview = %+v, %v", diff, err)
	}

	// Apply: one adjustment of -2, recorded for the requester.
	res, err := io.ApplyRow(ownerCtx, map[string]any{"branch_code": aCode, "stock_code": "cnt-1", "counted_quantity": "3"},
		map[string]any{"reason": "Ay sonu"})
	if err != nil || !res.OK || res.EntityType != "stock_movement" || res.Op != "create" {
		t.Fatalf("apply = %+v, %v", res, err)
	}
	countUUID := res.EntityUUID
	m := want(call(owner, http.MethodGet, "/v1/tenant/stock-movements?product="+pUUID+"&kind=adjustment", nil), http.StatusOK, "count row")
	row := m.data["items"].([]any)[0].(map[string]any)
	if row["uuid"] != countUUID || row["quantity"].(float64) != -2 || row["balance_after"].(float64) != 3 ||
		row["source"] != "import" || row["reason"] != "Ay sonu" || row["created_by"].(map[string]any)["uuid"] != owner.Uuid.String() {
		t.Fatalf("count row = %s", m.raw)
	}

	// Invalid rows are row errors, not job failures.
	for name, bad := range map[string]map[string]any{
		"unknown branch":   {"branch_code": "YOK", "stock_code": "CNT-1", "counted_quantity": "1"},
		"unknown code":     {"branch_code": aCode, "stock_code": "NOPE", "counted_quantity": "1"},
		"product variants": {"branch_code": aCode, "stock_code": "CNT-V", "counted_quantity": "1"},
		"negative":         {"branch_code": aCode, "stock_code": "CNT-1", "counted_quantity": "-1"},
		"not a number":     {"branch_code": aCode, "stock_code": "CNT-1", "counted_quantity": "abc"},
		"too many":         {"branch_code": aCode, "stock_code": "CNT-1", "counted_quantity": "100001"},
		"no count":         {"branch_code": aCode, "stock_code": "CNT-1"},
	} {
		res, err := io.ApplyRow(ownerCtx, bad, nil)
		if err != nil || res.OK || res.Error == "" {
			t.Fatalf("%s = %+v, %v", name, res, err)
		}
		if diff, err := io.PreviewRow(ownerCtx, bad, nil); err != nil || diff.Error == "" {
			t.Fatalf("%s preview = %+v, %v", name, diff, err)
		}
	}
	// Carol counts in A only: B reads as unknown.
	if res, err := io.ApplyRow(carolCtx, map[string]any{"branch_code": "SYM", "stock_code": "CNT-1", "counted_quantity": "1"}, nil); err != nil || res.OK {
		t.Fatalf("carol in B = %+v, %v", res, err)
	}

	// The variant by its barcode (0 → 4), "4.0" from a spreadsheet.
	res, err = io.ApplyRow(carolCtx, map[string]any{"branch_code": aCode, "stock_code": "8691964", "counted_quantity": "4.0"}, nil)
	if err != nil || !res.OK {
		t.Fatalf("variant count = %+v, %v", res, err)
	}
	variantCount := res.EntityUUID

	// Rollback of the first count brings the 2 units back.
	if err := io.RevertRow(ownerCtx, "stock_movement", countUUID, res.Previous); err != nil {
		t.Fatalf("revert = %v", err)
	}
	m = want(call(owner, http.MethodGet, "/v1/tenant/stock-movements?product="+pUUID+"&limit=1", nil), http.StatusOK, "after revert")
	row = m.data["items"].([]any)[0].(map[string]any)
	if row["kind"] != "adjustment" || row["quantity"].(float64) != 2 || row["balance_after"].(float64) != 5 || row["source"] != "import_rollback" {
		t.Fatalf("rollback row = %s", m.raw)
	}
	// Only imported counts are rolled back here.
	if err := io.RevertRow(ownerCtx, "stock_movement", row["uuid"].(string), nil); err == nil {
		t.Fatal("rollback of a rollback succeeded")
	}

	// Three of the four counted units are sold: the rollback would take
	// the stock below zero and is refused.
	want(call(owner, http.MethodPost, "/v1/tenant/stock-movements", map[string]any{
		"kind": "sale", "branch_uuid": aUUID, "product_uuid": p2UUID, "variant_uuid": vUUID, "quantity": 3,
	}), http.StatusCreated, "variant sale")
	if err := io.RevertRow(ownerCtx, "stock_movement", variantCount, nil); err == nil || !productsusecase.IsValidation(err) {
		t.Fatalf("revert after sale = %v", err)
	}

	// Export: list filters apply (kind, product, branch scope).
	ds, err := io.Export(ownerCtx, ioengine.ExportQuery{"kind": "adjustment", "product": pUUID, "sort": "occurred_at"}, "tr")
	if err != nil || len(ds.Rows) != 2 || ds.Rows[0]["source"] != "import" || ds.Rows[1]["source"] != "import_rollback" ||
		ds.Rows[0]["stock_code"] != "CNT-1" || ds.Rows[0]["branch"] == "" {
		t.Fatalf("export = %+v, %v", ds.Rows, err)
	}
	ds, err = io.Export(ownerCtx, ioengine.ExportQuery{"branch": bUUID}, "tr")
	if err != nil || len(ds.Rows) != 0 {
		t.Fatalf("export B = %+v, %v", ds.Rows, err)
	}
	ds, err = io.Export(carolCtx, ioengine.ExportQuery{}, "tr")
	if err != nil || len(ds.Rows) != 5 { // receipt, count, rollback, variant count, sale in A
		t.Fatalf("carol export = %d rows, %v", len(ds.Rows), err)
	}
	ds, err = io.Export(ownerCtx, ioengine.ExportQuery{"product": p2UUID, "kind": "adjustment"}, "tr")
	if err != nil || len(ds.Rows) != 1 || ds.Rows[0]["stock_code"] != "CNT-V-R" || ds.Rows[0]["variant"] != "Kızıl" {
		t.Fatalf("variant export = %+v, %v", ds.Rows, err)
	}
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	if ds, err = io.Export(ownerCtx, ioengine.ExportQuery{"from": future}, "tr"); err != nil || len(ds.Rows) != 0 {
		t.Fatalf("export from future = %d rows, %v", len(ds.Rows), err)
	}
}
