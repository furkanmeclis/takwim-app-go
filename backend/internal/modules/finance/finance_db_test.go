package finance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash"
	cashhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/handler"
	cashmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/model"
	cashrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/repository"
	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance"
	financehandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/handler"
	financerepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/repository"
	financeusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations"
	orghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/handler"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales"
	saleshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/handler"
	salesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/repository"
	salesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

type mapLoader map[uuid.UUID]int64

func (m mapLoader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, _ := uuid.Parse(c.Subject)
	return authctx.Principal{UserID: id, UserInternal: m[id]}, nil
}

// memDriver serves the documents from the in-memory store.
type memDriver struct {
	storage.Driver
	m *storage.Memory
}

func (d memDriver) Upload(ctx context.Context, f storage.File, path string) error {
	return d.m.Upload(ctx, f, path)
}

func (d memDriver) Download(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	return d.m.Download(ctx, path)
}

func (d memDriver) Delete(ctx context.Context, path string) error { return d.m.Delete(ctx, path) }

type result struct {
	code    int
	data    map[string]any
	details []any
	raw     string
	body    []byte
	header  http.Header
}

// pdf is the smallest body the content sniffer takes for a PDF.
var pdf = []byte("%PDF-1.4\n1 0 obj << >> endobj\ntrailer << >>\n%%EOF\n")

// TestFinanceOverHTTP drives the income / expense module (TEC-915) over
// HTTP with the real resolver and SQL: seeded categories and their CRUD,
// records posting their cash movement, idempotency, the branch scope of a
// branch manager, classification edits with their activity diff,
// reversal and correction (history kept, nothing deleted), the document
// attachment, the combined list with sale lines as automatic income, its
// filters and facets, and the summary that keeps expenses apart from
// staff payouts. Runs when TEST_DATABASE_URL points at a migrated database.
func TestFinanceOverHTTP(t *testing.T) {
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
	owner, carol, other := newUser("owner"), newUser("carol"), newUser("other")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Finance " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	otherOrg, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Finance other " + suffix, OwnerUserID: other.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		for _, id := range []int64{org.ID, otherOrg.ID} {
			_, _ = pool.Exec(bg, "DELETE FROM activity_events WHERE organization_id = $1", id)
			_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", id)
		}
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{owner.ID, carol.ID, other.ID})
	})
	if _, err := orgSvc.AddMember(ctx, org.UUID, carol.Uuid, "sube_yoneticisi"); err != nil {
		t.Fatal(err)
	}

	rec := activity.NewRecorder(q, nil)
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	loader := mapLoader{owner.Uuid: owner.ID, carol.Uuid: carol.ID, other.Uuid: other.ID}
	cashSvc := cashusecase.New(cashrepository.NewPostgres(pool, q), nil)
	salesSvc := salesusecase.New(salesrepository.NewPostgres(pool, q), cashSvc)
	svc := financeusecase.New(financerepository.NewPostgres(pool, q), cashSvc, salesSvc)
	svc.SetStorage(memDriver{m: storage.NewMemory()})
	mux := http.NewServeMux()
	branches.RegisterRoutes(mux, branchhandler.New(branchusecase.New(branchrepository.NewPostgres(q), nil), rec), branches.EngineHandlers{}, tokens, loader, orgSvc)
	organizations.RegisterTenantRoutes(mux, orghandler.New(orgSvc, rec), tokens, loader, orgSvc)
	cash.RegisterRoutes(mux, cashhandler.New(cashSvc, rec), cash.EngineHandlers{}, tokens, loader, orgSvc)
	sales.RegisterRoutes(mux, saleshandler.New(salesSvc, rec), tokens, loader, orgSvc)
	finance.RegisterRoutes(mux, financehandler.New(svc, rec), finance.EngineHandlers{}, tokens, loader, orgSvc)

	send := func(user db.User, slug, method, path, contentType string, body *bytes.Buffer) result {
		t.Helper()
		access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: user.Uuid})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, body)
		req.Header.Set("Authorization", "Bearer "+access)
		req.Header.Set(orgctx.HeaderName, slug)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		var env struct {
			Data  map[string]any `json:"data"`
			Error struct {
				Details []any `json:"details"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		return result{code: rr.Code, data: env.Data, details: env.Error.Details, raw: rr.Body.String(), body: rr.Body.Bytes(), header: rr.Header()}
	}
	callIn := func(user db.User, slug, method, path string, body any) result {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		return send(user, slug, method, path, "application/json", &buf)
	}
	call := func(user db.User, method, path string, body any) result {
		t.Helper()
		return callIn(user, org.Slug, method, path, body)
	}
	upload := func(user db.User, path, name string, content []byte) result {
		t.Helper()
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", name)
		_, _ = fw.Write(content)
		_ = mw.Close()
		return send(user, org.Slug, http.MethodPut, path, mw.FormDataContentType(), &buf)
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
			if d["field"] == field && d["code"] == code {
				return true
			}
		}
		return false
	}
	items := func(r result) []map[string]any {
		var out []map[string]any
		for _, it := range r.data["items"].([]any) {
			out = append(out, it.(map[string]any))
		}
		return out
	}
	total := func(r result) int {
		return int(r.data["total"].(float64))
	}
	balance := func(id string) string {
		t.Helper()
		return want(call(owner, http.MethodGet, "/v1/tenant/cash/accounts/"+id, nil), http.StatusOK, "account").data["balance"].(string)
	}
	scoped := orgctx.WithScope(ctx, orgctx.Scope{ID: org.ID, Timezone: org.Timezone})
	today := cashSvc.Today(scoped)
	day := func(offset int) string { return today.AddDate(0, 0, offset).Format("2006-01-02") }

	// Branch A ("Merkez") and B; carol manages A only.
	list := want(call(owner, http.MethodGet, "/v1/tenant/branches", nil), http.StatusOK, "branches")
	aUUID := list.data["items"].([]any)[0].(map[string]any)["uuid"].(string)
	bUUID := want(call(owner, http.MethodPost, "/v1/tenant/branches", map[string]any{"code": "KDK", "name": "Kadıköy"}),
		http.StatusCreated, "create B").data["uuid"].(string)
	want(call(owner, http.MethodPatch, "/v1/tenant/members/"+carol.Uuid.String(), map[string]any{
		"branch_scope": "selected", "branch_uuids": []string{aUUID},
	}), http.StatusOK, "narrow carol")
	var aID int64
	if err := pool.QueryRow(ctx, "SELECT id FROM branches WHERE uuid = $1", aUUID).Scan(&aID); err != nil {
		t.Fatal(err)
	}

	// Accounts: a till in A with 1000, a till in B, a business-wide bank.
	tillA := want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts", map[string]any{
		"branch_uuid": aUUID, "kind": "cash", "name": "Ana kasa", "opening_balance": "1000.00", "opening_date": day(-5),
	}), http.StatusCreated, "till A").data["uuid"].(string)
	tillB := want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts", map[string]any{
		"branch_uuid": bUUID, "kind": "cash", "name": "Ana kasa", "opening_balance": "500.00", "opening_date": day(-5),
	}), http.StatusCreated, "till B").data["uuid"].(string)
	bank := want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts", map[string]any{
		"kind": "bank", "name": "Banka", "opening_balance": "5000.00", "opening_date": day(-5),
	}), http.StatusCreated, "bank").data["uuid"].(string)

	// A staff member working in A.
	var ayseID int64
	var ayseUUID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO staff (organization_id, name) VALUES ($1, 'Ayşe') RETURNING id, uuid`, org.ID).Scan(&ayseID, &ayseUUID); err != nil {
		t.Fatal(err)
	}

	// 1. Seeded categories: ten expense and two income defaults per
	// business, system ones cannot be deleted.
	cats := want(call(owner, http.MethodGet, "/v1/tenant/finance/categories?limit=100", nil), http.StatusOK, "categories")
	if total(cats) != 12 {
		t.Fatalf("seeded categories = %d: %s", total(cats), cats.raw)
	}
	byCode := map[string]string{}
	for _, c := range items(cats) {
		if code, ok := c["code"].(string); ok {
			byCode[code] = c["uuid"].(string)
		}
	}
	rent, supplies, otherIncome := byCode["rent"], byCode["supplies"], byCode["other_income"]
	if rent == "" || supplies == "" || otherIncome == "" || byCode["staff_costs"] == "" {
		t.Fatalf("default codes = %v", byCode)
	}
	if exp := want(call(owner, http.MethodGet, "/v1/tenant/finance/categories?kind=expense&limit=100", nil), http.StatusOK, "expense cats"); total(exp) != 10 {
		t.Fatalf("expense categories = %d", total(exp))
	}
	want(call(owner, http.MethodDelete, "/v1/tenant/finance/categories/"+rent, nil), http.StatusConflict, "delete system")
	// The other business has its own defaults.
	if oc := want(callIn(other, otherOrg.Slug, http.MethodGet, "/v1/tenant/finance/categories?limit=100", nil), http.StatusOK, "other cats"); total(oc) != 12 {
		t.Fatalf("other org categories = %d", total(oc))
	}

	// Category CRUD: create, duplicate name, kind fixed, archive, delete.
	cleaning := want(call(owner, http.MethodPost, "/v1/tenant/finance/categories", map[string]any{"kind": "expense", "name": "Temizlik"}),
		http.StatusCreated, "create category").data
	cleaningID := cleaning["uuid"].(string)
	if cleaning["is_system"] != false || cleaning["status"] != "active" {
		t.Fatalf("category = %v", cleaning)
	}
	want(call(owner, http.MethodPost, "/v1/tenant/finance/categories", map[string]any{"kind": "expense", "name": "temizlik"}),
		http.StatusConflict, "duplicate category")
	want(call(owner, http.MethodPost, "/v1/tenant/finance/categories", map[string]any{"kind": "income", "name": "Temizlik"}),
		http.StatusCreated, "same name other kind")
	r := want(call(owner, http.MethodPost, "/v1/tenant/finance/categories", map[string]any{"kind": "x", "name": ""}),
		http.StatusBadRequest, "invalid category")
	if !hasDetail(r, "kind", "invalid") || !hasDetail(r, "name", "required") {
		t.Fatalf("category validation: %s", r.raw)
	}
	want(call(carol, http.MethodPost, "/v1/tenant/finance/categories", map[string]any{"kind": "expense", "name": "Şube"}),
		http.StatusForbidden, "branch manager category")
	tmp := want(call(owner, http.MethodPost, "/v1/tenant/finance/categories", map[string]any{"kind": "expense", "name": "Geçici"}),
		http.StatusCreated, "tmp category").data["uuid"].(string)
	want(call(owner, http.MethodDelete, "/v1/tenant/finance/categories/"+tmp, nil), http.StatusNoContent, "delete unused")
	want(call(owner, http.MethodGet, "/v1/tenant/finance/categories/"+tmp, nil), http.StatusNotFound, "deleted gone")

	// 2. An expense paid from till A: the till goes down in the same
	// transaction and the record carries its movement.
	key := "exp-" + suffix
	exp := want(call(owner, http.MethodPost, "/v1/tenant/finance/entries", map[string]any{
		"kind": "expense", "category_uuid": supplies, "cash_account_uuid": tillA, "amount": "150.00",
		"entry_date": day(-1), "description": "Şampuan", "idempotency_key": key,
	}), http.StatusCreated, "expense").data
	expID := exp["uuid"].(string)
	if exp["entry_type"] != "entry" || exp["amount"] != "150.00" || exp["branch"].(map[string]any)["uuid"] != aUUID ||
		exp["entry_date"] != day(-1) || exp["cash_movement_uuid"] == nil || exp["created_by"] == nil {
		t.Fatalf("expense = %v", exp)
	}
	if b := balance(tillA); b != "850.00" {
		t.Fatalf("till A after expense = %s", b)
	}
	// The same submit again writes nothing; the key with another body is refused.
	again := want(call(owner, http.MethodPost, "/v1/tenant/finance/entries", map[string]any{
		"kind": "expense", "category_uuid": supplies, "cash_account_uuid": tillA, "amount": "150.00",
		"entry_date": day(-1), "description": "Şampuan", "idempotency_key": key,
	}), http.StatusCreated, "replay").data
	if again["uuid"] != expID || balance(tillA) != "850.00" {
		t.Fatalf("replay = %v", again)
	}
	want(call(owner, http.MethodPost, "/v1/tenant/finance/entries", map[string]any{
		"kind": "expense", "category_uuid": supplies, "cash_account_uuid": tillA, "amount": "151.00", "idempotency_key": key,
	}), http.StatusConflict, "reused key")

	// Validation: kind / category mismatch, future date, zero amount, an
	// account of branch B for branch A, a till that would go negative.
	r = want(call(owner, http.MethodPost, "/v1/tenant/finance/entries", map[string]any{
		"kind": "income", "category_uuid": supplies, "cash_account_uuid": tillA, "amount": "10",
	}), http.StatusBadRequest, "kind mismatch")
	if !hasDetail(r, "category_uuid", "kind_mismatch") {
		t.Fatalf("kind mismatch: %s", r.raw)
	}
	r = want(call(owner, http.MethodPost, "/v1/tenant/finance/entries", map[string]any{
		"kind": "expense", "category_uuid": supplies, "cash_account_uuid": tillA, "amount": "0", "entry_date": day(1),
	}), http.StatusBadRequest, "amount and date")
	if !hasDetail(r, "amount", "invalid") || !hasDetail(r, "entry_date", "future") {
		t.Fatalf("amount/date: %s", r.raw)
	}
	r = want(call(owner, http.MethodPost, "/v1/tenant/finance/entries", map[string]any{
		"kind": "expense", "category_uuid": supplies, "cash_account_uuid": tillB, "branch_uuid": aUUID, "amount": "10",
	}), http.StatusBadRequest, "account of other branch")
	if !hasDetail(r, "cash_account_uuid", "branch_mismatch") {
		t.Fatalf("branch mismatch: %s", r.raw)
	}
	want(call(owner, http.MethodPost, "/v1/tenant/finance/entries", map[string]any{
		"kind": "expense", "category_uuid": supplies, "cash_account_uuid": tillB, "amount": "9999",
	}), http.StatusConflict, "till floor")

	// A business-wide rent from the bank, an expense of branch B paid from
	// the bank (the branch is given), an income into till A with staff.
	rentRec := want(call(owner, http.MethodPost, "/v1/tenant/finance/entries", map[string]any{
		"kind": "expense", "category_uuid": rent, "cash_account_uuid": bank, "amount": "2000.00", "description": "Ekim kirası",
	}), http.StatusCreated, "rent").data
	if rentRec["branch"] != nil || rentRec["entry_date"] != day(0) {
		t.Fatalf("rent = %v", rentRec)
	}
	bExp := want(call(owner, http.MethodPost, "/v1/tenant/finance/entries", map[string]any{
		"kind": "expense", "category_uuid": cleaningID, "cash_account_uuid": bank, "branch_uuid": bUUID, "amount": "80.00",
	}), http.StatusCreated, "B expense via bank").data
	if bExp["branch"].(map[string]any)["uuid"] != bUUID {
		t.Fatalf("B expense = %v", bExp)
	}
	inc := want(call(owner, http.MethodPost, "/v1/tenant/finance/entries", map[string]any{
		"kind": "income", "category_uuid": otherIncome, "cash_account_uuid": tillA, "staff_uuid": ayseUUID.String(),
		"amount": "40.00", "description": "Eğitim ücreti",
	}), http.StatusCreated, "income").data
	if inc["staff"].(map[string]any)["uuid"] != ayseUUID.String() || balance(tillA) != "890.00" || balance(bank) != "2920.00" {
		t.Fatalf("income = %v, till A %s, bank %s", inc, balance(tillA), balance(bank))
	}
	// The used category cannot be deleted.
	want(call(owner, http.MethodDelete, "/v1/tenant/finance/categories/"+cleaningID, nil), http.StatusConflict, "delete used")

	// 3. Branch scope: carol sees A only; she cannot read B's record or the
	// business-wide rent, cannot write business-wide or use till B.
	cl := want(call(carol, http.MethodGet, "/v1/tenant/finance/entries?source=manual", nil), http.StatusOK, "carol list")
	for _, it := range items(cl) {
		if b, _ := it["branch"].(map[string]any); b == nil || b["uuid"] != aUUID {
			t.Fatalf("carol sees %v", it)
		}
	}
	if total(cl) != 2 {
		t.Fatalf("carol manual rows = %d: %s", total(cl), cl.raw)
	}
	want(call(carol, http.MethodGet, "/v1/tenant/finance/entries/"+bExp["uuid"].(string), nil), http.StatusNotFound, "carol B record")
	want(call(carol, http.MethodGet, "/v1/tenant/finance/entries/"+rentRec["uuid"].(string), nil), http.StatusNotFound, "carol rent")
	r = want(call(carol, http.MethodPost, "/v1/tenant/finance/entries", map[string]any{
		"kind": "expense", "category_uuid": rent, "cash_account_uuid": bank, "amount": "1",
	}), http.StatusBadRequest, "carol business-wide")
	if !hasDetail(r, "cash_account_uuid", "not_found") {
		t.Fatalf("carol business-wide: %s", r.raw)
	}
	want(call(carol, http.MethodPost, "/v1/tenant/finance/entries", map[string]any{
		"kind": "expense", "category_uuid": supplies, "cash_account_uuid": tillA, "amount": "5.00",
	}), http.StatusCreated, "carol A")
	want(call(carol, http.MethodPost, "/v1/tenant/finance/entries", map[string]any{
		"kind": "expense", "category_uuid": supplies, "cash_account_uuid": tillB, "amount": "1",
	}), http.StatusBadRequest, "carol till B")

	// 4. Classification edit: category and description, with the change in
	// the activity log. Money stays.
	patched := want(call(owner, http.MethodPatch, "/v1/tenant/finance/entries/"+expID, map[string]any{
		"category_uuid": cleaningID, "description": "Şampuan ve havlu", "staff_uuid": ayseUUID.String(),
	}), http.StatusOK, "patch").data
	if patched["category"].(map[string]any)["uuid"] != cleaningID || patched["amount"] != "150.00" || patched["staff"] == nil {
		t.Fatalf("patched = %v", patched)
	}
	r = want(call(owner, http.MethodPatch, "/v1/tenant/finance/entries/"+expID, map[string]any{"category_uuid": otherIncome}),
		http.StatusBadRequest, "patch other kind")
	if !hasDetail(r, "category_uuid", "kind_mismatch") {
		t.Fatalf("patch kind: %s", r.raw)
	}
	act := want(call(owner, http.MethodGet, "/v1/tenant/finance/entries/"+expID+"/activity", nil), http.StatusOK, "activity")
	actions := map[string]bool{}
	for _, it := range items(act) {
		actions[it["action"].(string)] = true
	}
	if !actions["finance_entries.created"] || !actions["finance_entries.updated"] {
		t.Fatalf("activity = %s", act.raw)
	}

	// 5. The database keeps the history: money fields cannot change and a
	// record cannot be deleted.
	for _, sql := range []string{
		`UPDATE finance_entries SET amount = 1 WHERE organization_id = $1`,
		`UPDATE finance_entries SET entry_date = entry_date - 1 WHERE organization_id = $1`,
		`DELETE FROM finance_entries WHERE organization_id = $1`,
	} {
		if _, err := pool.Exec(ctx, sql, org.ID); err == nil {
			t.Fatalf("%s accepted", sql)
		}
	}

	// 6. Document: upload a PDF, read it back through the API, refuse a
	// non-document, remove it (the record stays).
	want(upload(owner, "/v1/tenant/finance/entries/"+expID+"/document", "fis.exe", []byte("MZ not a document")),
		http.StatusBadRequest, "upload exe")
	doc := want(upload(owner, "/v1/tenant/finance/entries/"+expID+"/document", "fiş.pdf", pdf), http.StatusOK, "upload").data
	d := doc["document"].(map[string]any)
	if d["content_type"] != "application/pdf" || d["url"] != "/v1/tenant/finance/entries/"+expID+"/document" {
		t.Fatalf("document = %v", d)
	}
	got := send(owner, org.Slug, http.MethodGet, "/v1/tenant/finance/entries/"+expID+"/document", "", &bytes.Buffer{})
	if got.code != http.StatusOK || !bytes.Equal(got.body, pdf) || got.header.Get("Content-Type") != "application/pdf" {
		t.Fatalf("download = %d %q", got.code, got.header)
	}
	var docKey string
	if err := pool.QueryRow(ctx, "SELECT document_key FROM finance_entries WHERE uuid = $1", expID).Scan(&docKey); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(docKey, "organizations/"+org.UUID.String()+"/finance/"+expID+"/") {
		t.Fatalf("document key = %s", docKey)
	}
	// The other business cannot reach it.
	want(callIn(other, otherOrg.Slug, http.MethodGet, "/v1/tenant/finance/entries/"+expID+"/document", nil), http.StatusNotFound, "other org document")
	want(callIn(other, otherOrg.Slug, http.MethodGet, "/v1/tenant/finance/entries/"+expID, nil), http.StatusNotFound, "other org record")

	// 7. Reversal: a negative record dated today, the money back in the
	// till, the original kept and linked; a second reversal is refused.
	rev := want(call(owner, http.MethodPost, "/v1/tenant/finance/entries/"+inc["uuid"].(string)+"/reverse", map[string]any{
		"reason": "Yanlış kayıt",
	}), http.StatusCreated, "reverse").data
	if rev["entry_type"] != "reversal" || rev["amount"] != "-40.00" || rev["entry_date"] != day(0) || rev["reverses_uuid"] != inc["uuid"] {
		t.Fatalf("reversal = %v", rev)
	}
	if b := balance(tillA); b != "845.00" {
		t.Fatalf("till A after reversal = %s", b)
	}
	want(call(owner, http.MethodPost, "/v1/tenant/finance/entries/"+inc["uuid"].(string)+"/reverse", map[string]any{"reason": "Tekrar"}),
		http.StatusConflict, "second reversal")
	want(call(owner, http.MethodPost, "/v1/tenant/finance/entries/"+rev["uuid"].(string)+"/reverse", map[string]any{"reason": "Geri"}),
		http.StatusConflict, "reverse a reversal")
	want(call(owner, http.MethodPost, "/v1/tenant/finance/entries/"+expID+"/reverse", map[string]any{"reason": ""}),
		http.StatusBadRequest, "reason required")
	orig := want(call(owner, http.MethodGet, "/v1/tenant/finance/entries/"+inc["uuid"].(string), nil), http.StatusOK, "reversed original").data
	if orig["reversed_by_uuid"] != rev["uuid"] || orig["amount"] != "40.00" {
		t.Fatalf("reversed original = %v", orig)
	}
	want(call(owner, http.MethodPatch, "/v1/tenant/finance/entries/"+inc["uuid"].(string), map[string]any{"description": "x"}),
		http.StatusConflict, "patch reversed")

	// 8. Correction: the expense was 120 not 150. The old record is
	// reversed and replaced in one step; the replacement keeps the date,
	// the category and the document.
	corr := want(call(owner, http.MethodPost, "/v1/tenant/finance/entries/"+expID+"/correct", map[string]any{
		"amount": "120.00", "reason": "Fiş tutarı",
	}), http.StatusCreated, "correct").data
	if corr["replaces_uuid"] != expID || corr["amount"] != "120.00" || corr["entry_date"] != day(-1) ||
		corr["category"].(map[string]any)["uuid"] != cleaningID || corr["document"] == nil {
		t.Fatalf("correction = %v", corr)
	}
	if b := balance(tillA); b != "875.00" {
		t.Fatalf("till A after correction = %s", b)
	}
	old := want(call(owner, http.MethodGet, "/v1/tenant/finance/entries/"+expID, nil), http.StatusOK, "corrected original").data
	if old["reversed_by_uuid"] == nil || old["replaced_by_uuid"] != corr["uuid"] {
		t.Fatalf("corrected original = %v", old)
	}
	want(call(owner, http.MethodPost, "/v1/tenant/finance/entries/"+expID+"/correct", map[string]any{"amount": "1", "reason": "Yine"}),
		http.StatusConflict, "correct twice")
	// The replacement's document streams too; removing it keeps the record.
	if g := send(owner, org.Slug, http.MethodGet, "/v1/tenant/finance/entries/"+corr["uuid"].(string)+"/document", "", &bytes.Buffer{}); g.code != http.StatusOK {
		t.Fatalf("replacement document = %d", g.code)
	}
	rm := want(call(owner, http.MethodDelete, "/v1/tenant/finance/entries/"+corr["uuid"].(string)+"/document", nil), http.StatusOK, "remove document").data
	if rm["document"] != nil {
		t.Fatalf("after remove = %v", rm)
	}
	want(call(owner, http.MethodGet, "/v1/tenant/finance/entries/"+corr["uuid"].(string)+"/document", nil), http.StatusNotFound, "removed document")

	// 9. Sale lines show as automatic income in the list; a staff payout
	// stays out of the expenses.
	if _, err := pool.Exec(ctx, `INSERT INTO sale_lines (organization_id, branch_id, kind, item_type, source_type, source_uuid,
		staff_id, description, quantity, unit_price, gross_amount, discount_amount, net_amount, currency, sale_date, accrual_key)
		VALUES ($1, $2, 'sale', 'product', 'stock_movement', gen_random_uuid(), $3, 'Wax', 1, 300, 300, 0, 300, 'TRY', $4,
		'stock_movement:' || gen_random_uuid()::text)`, org.ID, aID, ayseID, today); err != nil {
		t.Fatal(err)
	}
	payout := uuid.New()
	if _, err := cashSvc.Post(scoped, org.ID, cashusecase.PostInput{
		AccountUUID: uuid.MustParse(bank), Kind: cashmodel.MovementStaffPayment, Amount: money.FromUnits(70),
		Description: "Ayşe payı", SourceType: "staff_payout", SourceUUID: &payout,
	}); err != nil {
		t.Fatal(err)
	}

	all := want(call(owner, http.MethodGet, "/v1/tenant/finance/entries?limit=100", nil), http.StatusOK, "list")
	// expense, its reversal, its replacement, rent, B expense, income, its
	// reversal, carol's expense, the sale line.
	if total(all) != 9 {
		t.Fatalf("list total = %d: %s", total(all), all.raw)
	}
	sale := 0
	for _, it := range items(all) {
		if it["source"] == "sale" {
			sale++
			if it["kind"] != "income" || it["amount"] != "300.00" || it["item_type"] != "product" {
				t.Fatalf("sale row = %v", it)
			}
		}
	}
	if sale != 1 {
		t.Fatalf("sale rows = %d", sale)
	}
	count := func(query string) int {
		t.Helper()
		return total(want(call(owner, http.MethodGet, "/v1/tenant/finance/entries?limit=100&"+query, nil), http.StatusOK, query))
	}
	for query, n := range map[string]int{
		"source=sale":                                  1,
		"source=manual":                                8,
		"kind=income":                                  3,
		"kind=expense":                                 6,
		"category=sale_product":                        1,
		"category=sale_service":                        0,
		"category=" + cleaningID:                       4,
		"category=sale_product," + rent:                2,
		"branch=head_office":                           1,
		"branch=" + bUUID:                              1,
		"branch=" + aUUID:                              7,
		"staff=" + ayseUUID.String():                   6,
		"account=" + bank:                              2,
		"date_from=" + day(-1) + "&date_to=" + day(-1): 2,
		"q=kirası":                                     1,
		"category=" + uuid.NewString():                 0,
	} {
		if got := count(query); got != n {
			t.Errorf("filter %s = %d, want %d", query, got, n)
		}
	}
	want(call(owner, http.MethodGet, "/v1/tenant/finance/entries?kind=nope", nil), http.StatusBadRequest, "bad kind filter")
	want(call(owner, http.MethodGet, "/v1/tenant/finance/entries?sort=-amount", nil), http.StatusOK, "sort amount")
	want(call(owner, http.MethodGet, "/v1/tenant/finance/entries?sort=-secret", nil), http.StatusBadRequest, "bad sort")
	want(call(owner, http.MethodGet, "/v1/tenant/finance/entries/meta", nil), http.StatusOK, "meta")

	facets := want(call(owner, http.MethodGet, "/v1/tenant/finance/entries/facets", nil), http.StatusOK, "facets").data
	if len(facets["category"].([]any)) == 0 || len(facets["branch"].([]any)) == 0 || len(facets["staff"].([]any)) == 0 {
		t.Fatalf("facets = %v", facets)
	}

	// 10. Summary: revenue 300 from the sale line, income 0 (40 reversed),
	// expenses 120 + 2000 + 80 + 5 = 2205 (150 reversed), staff payouts 70
	// apart; net = 300 + 0 - 2205.
	sum := want(call(owner, http.MethodGet, "/v1/tenant/finance/summary?date_from="+day(-5)+"&date_to="+day(0), nil), http.StatusOK, "summary").data
	tot := sum["totals"].([]any)
	if len(tot) != 1 {
		t.Fatalf("summary totals = %v", sum)
	}
	st := tot[0].(map[string]any)
	if st["revenue"] != "300.00" || st["income"] != "0.00" || st["expenses"] != "2205.00" || st["staff_payments"] != "70.00" || st["net"] != "-1905.00" {
		t.Fatalf("summary = %v", st)
	}
	foundRent := false
	for _, raw := range sum["categories"].([]any) {
		c := raw.(map[string]any)
		if c["category"].(map[string]any)["uuid"] == rent && c["total"] == "2000.00" {
			foundRent = true
		}
	}
	if !foundRent {
		t.Fatalf("summary categories = %v", sum["categories"])
	}
	// Branch B only: the 80 of B.
	sb := want(call(owner, http.MethodGet, "/v1/tenant/finance/summary?branch="+bUUID, nil), http.StatusOK, "summary B").data
	if e := sb["totals"].([]any)[0].(map[string]any)["expenses"]; e != "80.00" {
		t.Fatalf("summary B = %v", sb)
	}
	// The sales summary carries income and expenses too, apart from the
	// staff payouts.
	ss := want(call(owner, http.MethodGet, "/v1/tenant/sales/summary?date_from="+day(-5)+"&date_to="+day(0), nil), http.StatusOK, "sales summary").data
	sst := ss["totals"].([]any)[0].(map[string]any)
	if sst["expenses"] != "2205.00" || sst["income"] != "0.00" || sst["staff_payments"] != "70.00" {
		t.Fatalf("sales summary = %v", sst)
	}
	// Carol's summary covers branch A only (no rent).
	cs := want(call(carol, http.MethodGet, "/v1/tenant/finance/summary?date_from="+day(-5), nil), http.StatusOK, "carol summary").data
	if e := cs["totals"].([]any)[0].(map[string]any)["expenses"]; e != "125.00" {
		t.Fatalf("carol summary = %v", cs)
	}

	// 11. The other business sees none of it.
	ol := want(callIn(other, otherOrg.Slug, http.MethodGet, "/v1/tenant/finance/entries", nil), http.StatusOK, "other list")
	if total(ol) != 0 {
		t.Fatalf("other org list = %s", ol.raw)
	}
	want(callIn(other, otherOrg.Slug, http.MethodPatch, "/v1/tenant/finance/categories/"+cleaningID, map[string]any{"name": "x"}),
		http.StatusNotFound, "other org category")
}
