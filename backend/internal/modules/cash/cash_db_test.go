package cash_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash"
	cashhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/handler"
	cashmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/model"
	cashrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/repository"
	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations"
	orghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/handler"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
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

// TestCashOverHTTP drives the cash book (TEC-914) over HTTP with the real
// resolver and SQL: accounts per branch and business-wide, branch scope of
// a branch manager, balances (opening + in - out), the till floor,
// transfers (two linked legs, atomic), reversals, the daily closing with
// its difference, the closed-day lock, reopening and the daily summary.
// Runs when TEST_DATABASE_URL points at a migrated database.
func TestCashOverHTTP(t *testing.T) {
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
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Cash " + suffix, OwnerUserID: owner.ID})
	if err != nil {
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
	svc := cashusecase.New(cashrepository.NewPostgres(pool, q), nil)
	mux := http.NewServeMux()
	branches.RegisterRoutes(mux, branchhandler.New(branchusecase.New(branchrepository.NewPostgres(q), nil), rec), branches.EngineHandlers{}, tokens, loader, orgSvc)
	organizations.RegisterTenantRoutes(mux, orghandler.New(orgSvc, rec), tokens, loader, orgSvc)
	cash.RegisterRoutes(mux, cashhandler.New(svc, rec), cash.EngineHandlers{}, tokens, loader, orgSvc)

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
			if d["field"] == field && d["code"] == code {
				return true
			}
		}
		return false
	}
	balance := func(user db.User, id string) string {
		t.Helper()
		return want(call(user, http.MethodGet, "/v1/tenant/cash/accounts/"+id, nil), http.StatusOK, "get account").data["balance"].(string)
	}

	today := svc.Today(orgctx.WithScope(ctx, orgctx.Scope{ID: org.ID, Timezone: org.Timezone}))
	day := func(offset int) string { return today.AddDate(0, 0, offset).Format("2006-01-02") }

	// Branch A is the default "Merkez"; B is added; carol manages A only.
	list := want(call(owner, http.MethodGet, "/v1/tenant/branches", nil), http.StatusOK, "branches")
	aUUID := list.data["items"].([]any)[0].(map[string]any)["uuid"].(string)
	bUUID := want(call(owner, http.MethodPost, "/v1/tenant/branches", map[string]any{"code": "KDK", "name": "Kadıköy"}),
		http.StatusCreated, "create B").data["uuid"].(string)
	want(call(owner, http.MethodPatch, "/v1/tenant/members/"+carol.Uuid.String(), map[string]any{
		"branch_scope": "selected", "branch_uuids": []string{aUUID},
	}), http.StatusOK, "narrow carol")

	// Accounts: a till in A (opened three days ago with 100), a till in B,
	// a business-wide bank account and an online provider account.
	tillA := want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts", map[string]any{
		"branch_uuid": aUUID, "kind": "cash", "name": "Ana kasa", "opening_balance": "100.00", "opening_date": day(-3),
	}), http.StatusCreated, "till A").data
	tillAID := tillA["uuid"].(string)
	if tillA["balance"] != "100.00" || tillA["branch"].(map[string]any)["uuid"] != aUUID || tillA["currency"] != "TRY" {
		t.Fatalf("till A = %v", tillA)
	}
	tillBID := want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts", map[string]any{
		"branch_uuid": bUUID, "kind": "cash", "name": "Ana kasa", "opening_date": day(-3),
	}), http.StatusCreated, "till B (same name, other branch)").data["uuid"].(string)
	bankID := want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts", map[string]any{
		"kind": "bank", "name": "İş Bankası", "bank_name": "İş Bankası", "iban": "TR33 0006 1005 1978 6457 8413 26",
		"opening_date": day(-3),
	}), http.StatusCreated, "business-wide bank").data["uuid"].(string)
	onlineID := want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts", map[string]any{
		"kind": "online", "name": "iyzico alacağı", "provider": "iyzico", "opening_date": day(-3),
	}), http.StatusCreated, "online account").data["uuid"].(string)

	// Validation: duplicate name in the branch, negative till opening,
	// unknown kind, IBAN on a till.
	want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts", map[string]any{
		"branch_uuid": aUUID, "kind": "cash", "name": "ana KASA",
	}), http.StatusConflict, "duplicate name")
	r := want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts", map[string]any{
		"branch_uuid": aUUID, "kind": "cash", "name": "X", "opening_balance": "-5", "iban": "TR330006100519786457841326",
	}), http.StatusBadRequest, "invalid till")
	if !hasDetail(r, "opening_balance", "negative") || !hasDetail(r, "iban", "invalid") {
		t.Fatalf("validation details: %s", r.raw)
	}

	// Branch scope: carol sees A's till only; B's till and the
	// business-wide accounts read as not found; she cannot open a
	// business-wide account. Reception has no cash access by default.
	carolList := want(call(carol, http.MethodGet, "/v1/tenant/cash/accounts", nil), http.StatusOK, "carol list")
	if carolList.data["total"].(float64) != 1 {
		t.Fatalf("carol sees %v accounts: %s", carolList.data["total"], carolList.raw)
	}
	want(call(carol, http.MethodGet, "/v1/tenant/cash/accounts/"+tillBID, nil), http.StatusNotFound, "carol B till")
	want(call(carol, http.MethodGet, "/v1/tenant/cash/accounts/"+bankID, nil), http.StatusNotFound, "carol bank")
	r = want(call(carol, http.MethodPost, "/v1/tenant/cash/accounts", map[string]any{"kind": "bank", "name": "Y"}),
		http.StatusBadRequest, "carol business-wide")
	if !hasDetail(r, "branch_uuid", "required") {
		t.Fatalf("carol create details: %s", r.raw)
	}
	want(call(dave, http.MethodGet, "/v1/tenant/cash/accounts", nil), http.StatusForbidden, "reception")

	// Manual movements: balance = opening + in - out.
	want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts/"+tillAID+"/movements", map[string]any{
		"direction": "in", "amount": "1000", "movement_date": day(-1), "description": "Gün içi nakit",
	}), http.StatusCreated, "deposit")
	wd := want(call(carol, http.MethodPost, "/v1/tenant/cash/accounts/"+tillAID+"/movements", map[string]any{
		"direction": "out", "amount": "300.50", "movement_date": day(-1), "description": "Market alışverişi",
	}), http.StatusCreated, "withdrawal").data
	if wd["kind"] != "withdrawal" || wd["amount"] != "300.50" {
		t.Fatalf("withdrawal = %v", wd)
	}
	if b := balance(owner, tillAID); b != "799.50" {
		t.Fatalf("till A balance = %s, want 799.50", b)
	}
	// A till never goes below zero; a bank account may.
	r = want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts/"+tillAID+"/movements", map[string]any{
		"direction": "out", "amount": "800", "description": "Fazla",
	}), http.StatusConflict, "till below zero")
	if !hasDetail(r, "amount", "insufficient_funds") {
		t.Fatalf("insufficient details: %s", r.raw)
	}
	want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts/"+bankID+"/movements", map[string]any{
		"direction": "out", "amount": "50", "description": "Banka masrafı",
	}), http.StatusCreated, "bank overdraft")
	if b := balance(owner, bankID); b != "-50.00" {
		t.Fatalf("bank balance = %s", b)
	}
	// Dates: not before the opening, not in the future.
	want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts/"+tillAID+"/movements", map[string]any{
		"direction": "in", "amount": "1", "movement_date": day(-10), "description": "Eski",
	}), http.StatusBadRequest, "before opening")
	want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts/"+tillAID+"/movements", map[string]any{
		"direction": "in", "amount": "1", "movement_date": day(1), "description": "Yarın",
	}), http.StatusBadRequest, "future")
	// The opening balance is fixed once the account has movements.
	r = want(call(owner, http.MethodPatch, "/v1/tenant/cash/accounts/"+tillAID, map[string]any{"opening_balance": "5"}),
		http.StatusBadRequest, "opening locked")
	if !hasDetail(r, "opening_balance", "locked") {
		t.Fatalf("opening details: %s", r.raw)
	}

	// Transfer till A -> bank: two linked legs, not revenue; the total of
	// the two accounts is unchanged.
	tr := want(call(owner, http.MethodPost, "/v1/tenant/cash/transfers", map[string]any{
		"from_account_uuid": tillAID, "to_account_uuid": bankID, "amount": "200", "description": "Bankaya yatırıldı",
	}), http.StatusCreated, "transfer").data
	outLeg, inLeg := tr["out"].(map[string]any), tr["in"].(map[string]any)
	if outLeg["kind"] != "transfer_out" || inLeg["kind"] != "transfer_in" || outLeg["transfer_group"] != inLeg["transfer_group"] ||
		outLeg["counter_account"].(map[string]any)["uuid"] != bankID {
		t.Fatalf("transfer legs = %v", tr)
	}
	if a, b := balance(owner, tillAID), balance(owner, bankID); a != "599.50" || b != "150.00" {
		t.Fatalf("after transfer: till %s bank %s", a, b)
	}
	// Provider receivable to bank is a transfer of its own.
	want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts/"+onlineID+"/movements", map[string]any{
		"direction": "in", "amount": "500", "description": "Online tahsilat",
	}), http.StatusCreated, "online in")
	want(call(owner, http.MethodPost, "/v1/tenant/cash/transfers", map[string]any{
		"from_account_uuid": onlineID, "to_account_uuid": bankID, "amount": "500", "description": "iyzico aktarımı",
	}), http.StatusCreated, "provider payout")
	// Refused transfers write nothing.
	r = want(call(owner, http.MethodPost, "/v1/tenant/cash/transfers", map[string]any{
		"from_account_uuid": tillAID, "to_account_uuid": tillAID, "amount": "1",
	}), http.StatusBadRequest, "same account")
	if !hasDetail(r, "to_account_uuid", "same_account") {
		t.Fatalf("same account details: %s", r.raw)
	}
	want(call(owner, http.MethodPost, "/v1/tenant/cash/transfers", map[string]any{
		"from_account_uuid": tillAID, "to_account_uuid": tillBID, "amount": "10000",
	}), http.StatusConflict, "transfer over the till")
	if a, b := balance(owner, tillAID), balance(owner, tillBID); a != "599.50" || b != "0.00" {
		t.Fatalf("failed transfer left something: till A %s till B %s", a, b)
	}
	// Carol cannot move money to an account outside her scope.
	r = want(call(carol, http.MethodPost, "/v1/tenant/cash/transfers", map[string]any{
		"from_account_uuid": tillAID, "to_account_uuid": tillBID, "amount": "1",
	}), http.StatusBadRequest, "carol transfer to B")
	if !hasDetail(r, "to_account_uuid", "not_found") {
		t.Fatalf("carol transfer details: %s", r.raw)
	}

	// Reversal: the opposite movement, once; a transfer reverses both legs.
	dep := want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts/"+tillAID+"/movements", map[string]any{
		"direction": "in", "amount": "40", "description": "Hatalı giriş",
	}), http.StatusCreated, "wrong deposit").data["uuid"].(string)
	want(call(owner, http.MethodPost, "/v1/tenant/cash/movements/"+dep+"/reverse", map[string]any{"reason": "x"}),
		http.StatusBadRequest, "reason too short")
	rv := want(call(owner, http.MethodPost, "/v1/tenant/cash/movements/"+dep+"/reverse", map[string]any{"reason": "Yanlış tutar"}),
		http.StatusCreated, "reverse").data["items"].([]any)
	if len(rv) != 1 || rv[0].(map[string]any)["direction"] != "out" || rv[0].(map[string]any)["reverses_uuid"] != dep {
		t.Fatalf("reversal = %v", rv)
	}
	want(call(owner, http.MethodPost, "/v1/tenant/cash/movements/"+dep+"/reverse", map[string]any{"reason": "Tekrar"}),
		http.StatusConflict, "reverse twice")
	want(call(owner, http.MethodPost, "/v1/tenant/cash/movements/"+rv[0].(map[string]any)["uuid"].(string)+"/reverse",
		map[string]any{"reason": "Geri al"}), http.StatusConflict, "reverse a reversal")
	rv = want(call(owner, http.MethodPost, "/v1/tenant/cash/movements/"+inLeg["uuid"].(string)+"/reverse",
		map[string]any{"reason": "Yanlış hesap"}), http.StatusCreated, "reverse transfer").data["items"].([]any)
	if len(rv) != 2 {
		t.Fatalf("transfer reversal legs = %v", rv)
	}
	if a, b := balance(owner, tillAID), balance(owner, bankID); a != "799.50" || b != "450.00" {
		t.Fatalf("after reversals: till %s bank %s", a, b)
	}

	// Movement list of the account carries the running balance (newest first).
	ml := want(call(owner, http.MethodGet, "/v1/tenant/cash/accounts/"+tillAID+"/movements", nil), http.StatusOK, "account movements")
	first := ml.data["items"].([]any)[0].(map[string]any)
	if first["balance_after"] != "799.50" {
		t.Fatalf("running balance = %v", first["balance_after"])
	}
	// The cash book filters by account and direction.
	all := want(call(owner, http.MethodGet, "/v1/tenant/cash/movements?account="+tillAID+"&direction=out", nil), http.StatusOK, "book")
	for _, it := range all.data["items"].([]any) {
		m := it.(map[string]any)
		if m["direction"] != "out" || m["account"].(map[string]any)["uuid"] != tillAID {
			t.Fatalf("book filter leaked %v", m)
		}
	}
	if n := want(call(carol, http.MethodGet, "/v1/tenant/cash/movements", nil), http.StatusOK, "carol book").data["total"].(float64); n == 0 {
		t.Fatal("carol sees no movement of her till")
	} else {
		for _, it := range want(call(carol, http.MethodGet, "/v1/tenant/cash/movements?limit=100", nil), http.StatusOK, "carol book").data["items"].([]any) {
			if it.(map[string]any)["account"].(map[string]any)["uuid"] != tillAID {
				t.Fatalf("carol sees another account's movement: %v", it)
			}
		}
	}

	// Daily closing of yesterday: expected = opening + in - out of the day.
	pv := want(call(carol, http.MethodGet, "/v1/tenant/cash/accounts/"+tillAID+"/closings/preview?date="+day(-1), nil),
		http.StatusOK, "preview").data
	if pv["opening_amount"] != "100.00" || pv["total_in"] != "1000.00" || pv["total_out"] != "300.50" || pv["expected_amount"] != "799.50" {
		t.Fatalf("preview = %v", pv)
	}
	want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts/"+bankID+"/closings", map[string]any{"counted_amount": "1"}),
		http.StatusConflict, "bank is not closed daily")
	cl := want(call(carol, http.MethodPost, "/v1/tenant/cash/accounts/"+tillAID+"/closings", map[string]any{
		"business_date": day(-1), "counted_amount": "789.50", "note": "10 TL eksik",
	}), http.StatusCreated, "close yesterday").data
	if cl["expected_amount"] != "799.50" || cl["difference"] != "-10.00" || cl["status"] != "closed" {
		t.Fatalf("closing = %v", cl)
	}
	if b := balance(owner, tillAID); b != "789.50" {
		t.Fatalf("balance after closing = %s, want the count 789.50", b)
	}
	// The closed day (and the days before it) is locked; today is open.
	r = want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts/"+tillAID+"/movements", map[string]any{
		"direction": "in", "amount": "5", "movement_date": day(-1), "description": "Geç giriş",
	}), http.StatusConflict, "closed day")
	if !hasDetail(r, "movement_date", "day_closed") {
		t.Fatalf("closed day details: %s", r.raw)
	}
	want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts/"+tillAID+"/movements", map[string]any{
		"direction": "in", "amount": "5", "movement_date": day(-2), "description": "Daha eski",
	}), http.StatusConflict, "day before the closed day")
	want(call(owner, http.MethodPost, "/v1/tenant/cash/transfers", map[string]any{
		"from_account_uuid": bankID, "to_account_uuid": tillAID, "amount": "5", "movement_date": day(-1),
	}), http.StatusConflict, "transfer into a closed day")
	want(call(carol, http.MethodPost, "/v1/tenant/cash/accounts/"+tillAID+"/closings", map[string]any{
		"business_date": day(-1), "counted_amount": "789.50",
	}), http.StatusConflict, "close twice")
	want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts/"+tillAID+"/movements", map[string]any{
		"direction": "in", "amount": "10.50", "description": "Bugün",
	}), http.StatusCreated, "today is open")

	// The database refuses the same (backstop of the use case).
	var accID, orgID int64
	if err := pool.QueryRow(ctx, "SELECT id, organization_id FROM cash_accounts WHERE uuid = $1", tillAID).Scan(&accID, &orgID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO cash_movements (organization_id, cash_account_id, kind, direction, amount, currency, movement_date)
		VALUES ($1, $2, 'deposit', 'in', 1, 'TRY', $3)`, orgID, accID, day(-1)); err == nil {
		t.Fatal("insert on a closed day was accepted")
	}
	if _, err := pool.Exec(ctx, "UPDATE cash_movements SET amount = 1 WHERE cash_account_id = $1", accID); err == nil {
		t.Fatal("cash movements are not append-only")
	}
	if _, err := pool.Exec(ctx, "DELETE FROM cash_movements WHERE cash_account_id = $1", accID); err == nil {
		t.Fatal("cash movements can be deleted")
	}

	// Reopening needs tenant.cash.reopen (not granted to branch managers)
	// and a reason; it reverses the difference.
	closings := want(call(owner, http.MethodGet, "/v1/tenant/cash/accounts/"+tillAID+"/closings", nil), http.StatusOK, "closings")
	clID := closings.data["items"].([]any)[0].(map[string]any)["uuid"].(string)
	want(call(carol, http.MethodPost, "/v1/tenant/cash/accounts/"+tillAID+"/closings/"+clID+"/reopen", map[string]any{"reason": "Düzeltme"}),
		http.StatusForbidden, "carol reopen")
	want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts/"+tillAID+"/closings/"+clID+"/reopen", map[string]any{}),
		http.StatusBadRequest, "reopen without reason")
	ro := want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts/"+tillAID+"/closings/"+clID+"/reopen",
		map[string]any{"reason": "Sayım hatası"}), http.StatusOK, "reopen").data
	if ro["status"] != "reopened" || ro["reopened_by"] == nil {
		t.Fatalf("reopened = %v", ro)
	}
	if b := balance(owner, tillAID); b != "810.00" {
		t.Fatalf("balance after reopen = %s, want 810.00", b)
	}
	pv = want(call(owner, http.MethodGet, "/v1/tenant/cash/accounts/"+tillAID+"/closings/preview?date="+day(-1), nil),
		http.StatusOK, "preview after reopen").data
	if pv["expected_amount"] != "799.50" || pv["closed"] != false {
		t.Fatalf("preview after reopen = %v", pv)
	}
	want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts/"+tillAID+"/movements", map[string]any{
		"direction": "in", "amount": "0.50", "movement_date": day(-1), "description": "Geç giriş",
	}), http.StatusCreated, "reopened day takes movements")
	cl = want(call(carol, http.MethodPost, "/v1/tenant/cash/accounts/"+tillAID+"/closings", map[string]any{
		"business_date": day(-1), "counted_amount": "800.00",
	}), http.StatusCreated, "close again").data
	if cl["expected_amount"] != "800.00" || cl["difference"] != "0.00" {
		t.Fatalf("second closing = %v", cl)
	}
	want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts/"+tillAID+"/closings/"+clID+"/reopen",
		map[string]any{"reason": "Tekrar"}), http.StatusConflict, "reopen a reopened closing")

	// Daily summary of yesterday: transfers apart from money in / out.
	ds := want(call(owner, http.MethodGet, "/v1/tenant/cash/daily-summary?date="+day(-1), nil), http.StatusOK, "daily summary").data
	var tillRow map[string]any
	for _, it := range ds["accounts"].([]any) {
		if it.(map[string]any)["account"].(map[string]any)["uuid"] == tillAID {
			tillRow = it.(map[string]any)
		}
	}
	if tillRow == nil || tillRow["money_in"] != "1000.50" || tillRow["money_out"] != "300.50" || tillRow["closing_amount"] != "800.00" ||
		tillRow["closed"] != true {
		t.Fatalf("summary row = %v", tillRow)
	}
	today0 := want(call(owner, http.MethodGet, "/v1/tenant/cash/daily-summary", nil), http.StatusOK, "summary today").data
	for _, it := range today0["accounts"].([]any) {
		row := it.(map[string]any)
		if row["account"].(map[string]any)["uuid"] == bankID && (row["transfer_in"] != "700.00" || row["money_out"] != "250.00" || row["closing_amount"] != "450.00") {
			t.Fatalf("bank transfers today = %v", row)
		}
	}
	if n := len(want(call(carol, http.MethodGet, "/v1/tenant/cash/daily-summary", nil), http.StatusOK, "carol summary").data["accounts"].([]any)); n != 1 {
		t.Fatalf("carol summary rows = %d", n)
	}

	// Archive: only with a zero balance; an archived account takes no money.
	want(call(owner, http.MethodPatch, "/v1/tenant/cash/accounts/"+tillAID, map[string]any{"status": "archived"}),
		http.StatusConflict, "archive with balance")
	want(call(owner, http.MethodPatch, "/v1/tenant/cash/accounts/"+tillBID, map[string]any{"status": "archived"}),
		http.StatusOK, "archive empty till")
	want(call(owner, http.MethodPost, "/v1/tenant/cash/accounts/"+tillBID+"/movements", map[string]any{
		"direction": "in", "amount": "1", "description": "x",
	}), http.StatusConflict, "archived account")

	// Activity trail of the account.
	act := want(call(owner, http.MethodGet, "/v1/tenant/cash/accounts/"+tillAID+"/activity", nil), http.StatusOK, "activity")
	if act.data["total"].(float64) < 5 {
		t.Fatalf("activity = %s", act.raw)
	}

	// Module postings (payments, sales ...) are idempotent per key.
	scoped := orgctx.WithScope(ctx, orgctx.Scope{ID: org.ID, Currency: "TRY", Timezone: org.Timezone})
	payment := uuid.New()
	in := cashusecase.PostInput{
		AccountUUID: uuid.MustParse(tillAID), Kind: cashmodel.MovementCollection, Amount: money.MustParse("250"),
		SourceType: "payment", SourceUUID: &payment, IdempotencyKey: "payment:" + payment.String(),
	}
	m1, err := svc.Post(scoped, org.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	m2, err := svc.Post(scoped, org.ID, in)
	if err != nil || m1 != m2 {
		t.Fatalf("repeated posting = %v %v (first %v)", m2, err, m1)
	}
	in.Amount = money.MustParse("1")
	if _, err := svc.Post(scoped, org.ID, in); !errors.Is(err, cashusecase.ErrIdempotencyReused) {
		t.Fatalf("reused key with another amount = %v", err)
	}
	if b := balance(owner, tillAID); b != "1060.50" {
		t.Fatalf("balance after collection = %s", b)
	}
	// A module posting is reversed by its module, not from the cash panel.
	want(call(owner, http.MethodPost, "/v1/tenant/cash/movements/"+m1.String()+"/reverse", map[string]any{"reason": "Panelden"}),
		http.StatusConflict, "module movement")
}

// TestConcurrentTransfers runs opposite transfers between two accounts in
// parallel: no deadlock, the total is unchanged and the till never goes
// below zero.
func TestConcurrentTransfers(t *testing.T) {
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
		Email: "cc-" + suffix + "@example.test", PasswordHash: "x", Name: "C", Surname: "C", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Cash CC " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", owner.ID)
	})
	svc := cashusecase.New(cashrepository.NewPostgres(pool, q), nil)
	scoped := orgctx.WithScope(ctx, orgctx.Scope{ID: org.ID, Currency: "TRY", Timezone: org.Timezone})
	str := func(s string) *string { return &s }
	kind := func(k string) *string { return &k }
	opening := money.MustParse("100")
	till, err := svc.CreateAccount(scoped, org.ID, &owner.ID, cashmodel.AccountInput{Kind: kind("cash"), Name: str("Kasa"), OpeningBalance: &opening})
	if err != nil {
		t.Fatal(err)
	}
	bank, err := svc.CreateAccount(scoped, org.ID, &owner.ID, cashmodel.AccountInput{Kind: kind("bank"), Name: str("Banka"), OpeningBalance: &opening})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	failures := map[string]int{}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			in := cashmodel.TransferInput{FromAccountUUID: till.UUID, ToAccountUUID: bank.UUID, Amount: money.MustParse("30")}
			if i%2 == 1 {
				in.FromAccountUUID, in.ToAccountUUID = bank.UUID, till.UUID
				in.Amount = money.MustParse("10")
			}
			if _, err := svc.Transfer(scoped, org.ID, &owner.ID, in); err != nil {
				mu.Lock()
				failures[err.Error()]++
				mu.Unlock()
				if !errors.Is(err, cashusecase.ErrInsufficientFunds) {
					t.Errorf("transfer %d: %v", i, err)
				}
			}
		}(i)
	}
	wg.Wait()
	a, err := svc.GetAccount(scoped, org.ID, till.UUID)
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.GetAccount(scoped, org.ID, bank.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if got := a.Balance.Add(b.Balance); got.String() != "200.00" {
		t.Fatalf("total = %s, want 200.00 (failures %v)", got, failures)
	}
	if a.Balance.IsNegative() {
		t.Fatalf("till went negative: %s", a.Balance)
	}
}
