package cash_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash"
	cashadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/adapters"
	cashhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/handler"
	cashrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/repository"
	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

// TestCashStatementsOverHTTP drives statement reconciliation (TEC-965) over
// HTTP with the real SQL: a statement per imported document on a bank or
// online account, lines matched to book movements (auto by amount and
// date, by hand with a difference), posting of unmatched lines through the
// idempotent PostTx (payouts as transfers, provider fees as expenses),
// reversals, the period closing that refuses open lines and locks the
// statement, and the sheet import adapter.
func TestCashStatementsOverHTTP(t *testing.T) {
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
		Email: "st-" + suffix + "@example.test", PasswordHash: "x", Name: "Owner", Surname: "Test", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Statements " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM activity_events WHERE organization_id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", owner.ID)
	})

	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	svc := cashusecase.New(cashrepository.NewPostgres(pool, q), nil)
	mux := http.NewServeMux()
	cash.RegisterRoutes(mux, cashhandler.New(svc, activity.NewRecorder(q, nil)), cash.EngineHandlers{}, tokens, mapLoader{owner.Uuid: owner.ID}, orgSvc)

	call := func(method, path string, body any) result {
		t.Helper()
		access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: owner.Uuid})
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
	detail := func(r result, field, code, step string) {
		t.Helper()
		for _, raw := range r.details {
			d := raw.(map[string]any)
			if d["field"] == field && d["code"] == code {
				return
			}
		}
		t.Fatalf("%s: no detail %s.%s in %s", step, field, code, r.raw)
	}
	balance := func(id string) string {
		t.Helper()
		return want(call(http.MethodGet, "/v1/tenant/cash/accounts/"+id, nil), http.StatusOK, "get account").data["balance"].(string)
	}
	str := func(m map[string]any, key string) string {
		if v, ok := m[key].(string); ok {
			return v
		}
		return ""
	}
	ref := func(m map[string]any, key string) map[string]any {
		v, _ := m[key].(map[string]any)
		return v
	}

	today := svc.Today(orgctx.WithScope(ctx, orgctx.Scope{ID: org.ID, Timezone: org.Timezone}))
	day := func(offset int) string { return today.AddDate(0, 0, offset).Format("2006-01-02") }

	// Accounts: a till, a bank account and an online provider account.
	tillID := want(call(http.MethodPost, "/v1/tenant/cash/accounts", map[string]any{
		"kind": "cash", "name": "Merkez kasa", "opening_balance": "100", "opening_date": day(-10),
	}), http.StatusCreated, "till").data["uuid"].(string)
	bankID := want(call(http.MethodPost, "/v1/tenant/cash/accounts", map[string]any{
		"kind": "bank", "name": "Ziraat vadesiz", "opening_balance": "1000", "opening_date": day(-10),
	}), http.StatusCreated, "bank").data["uuid"].(string)
	onlineID := want(call(http.MethodPost, "/v1/tenant/cash/accounts", map[string]any{
		"kind": "online", "name": "iyzico", "provider": "iyzico", "opening_date": day(-10),
	}), http.StatusCreated, "online").data["uuid"].(string)

	// Book movements the bank statement should find.
	mIn := want(call(http.MethodPost, "/v1/tenant/cash/accounts/"+bankID+"/movements", map[string]any{
		"direction": "in", "amount": "250", "movement_date": day(-2), "description": "Havale Ayşe Yılmaz",
	}), http.StatusCreated, "book in").data["uuid"].(string)
	mOut := want(call(http.MethodPost, "/v1/tenant/cash/accounts/"+bankID+"/movements", map[string]any{
		"direction": "out", "amount": "80", "movement_date": day(-3), "description": "Kira",
	}), http.StatusCreated, "book out").data["uuid"].(string)

	// A statement belongs to a bank / online account; the source defaults
	// by kind.
	r := want(call(http.MethodPost, "/v1/tenant/cash/statements", map[string]any{
		"account_uuid": tillID, "name": "Kasa ekstresi",
	}), http.StatusBadRequest, "statement on a till")
	detail(r, "account_uuid", "not_reconcilable", "till statement")
	st := want(call(http.MethodPost, "/v1/tenant/cash/statements", map[string]any{
		"account_uuid": bankID, "name": "Haziran ekstresi", "period_from": day(-5), "period_to": day(-1), "closing_balance": "1290.00",
	}), http.StatusCreated, "bank statement").data
	stID := st["uuid"].(string)
	if st["source"] != "bank" || st["status"] != "open" || st["line_count"] != float64(0) {
		t.Fatalf("statement = %v", st)
	}
	lines := "/v1/tenant/cash/statements/" + stID + "/lines"
	lineURL := func(id string) string { return "/v1/tenant/cash/statement-lines/" + id }

	// Same amount within three days: matched on arrival, by the auto
	// method, to the one movement.
	l1 := want(call(http.MethodPost, lines, map[string]any{
		"line_date": day(-1), "direction": "in", "amount": "250", "description": "HAVALE AYSE YILMAZ", "reference": "R1",
	}), http.StatusCreated, "line 1").data
	if l1["status"] != "matched" || l1["match_method"] != "auto" || str(ref(l1, "matched_movement"), "uuid") != mIn || l1["line_no"] != float64(1) {
		t.Fatalf("line 1 = %v", l1)
	}
	l2 := want(call(http.MethodPost, lines, map[string]any{
		"line_date": day(-3), "direction": "out", "amount": "80", "description": "KIRA ODEMESI", "reference": "R2",
	}), http.StatusCreated, "line 2").data
	if l2["status"] != "matched" || str(ref(l2, "matched_movement"), "uuid") != mOut {
		t.Fatalf("line 2 = %v", l2)
	}
	// Nothing in the book: pending. A repeated reference on the account is
	// refused (the sheet was imported twice).
	l3 := want(call(http.MethodPost, lines, map[string]any{
		"line_date": day(-2), "direction": "in", "amount": "120", "description": "POS tahsilat", "reference": "R3",
	}), http.StatusCreated, "line 3").data
	if l3["status"] != "pending" {
		t.Fatalf("line 3 = %v", l3)
	}
	r = want(call(http.MethodPost, lines, map[string]any{
		"line_date": day(-2), "direction": "in", "amount": "120", "reference": "R3",
	}), http.StatusConflict, "duplicate reference")
	detail(r, "reference", "taken", "duplicate reference")
	l3ID := l3["uuid"].(string)

	// Candidates of a pending line: same account and direction, the
	// matched movement is not offered again.
	cands := want(call(http.MethodGet, lineURL(l3ID)+"/candidates", nil), http.StatusOK, "candidates").data["items"].([]any)
	for _, c := range cands {
		if c.(map[string]any)["uuid"] == mIn {
			t.Fatalf("matched movement offered again: %v", cands)
		}
	}

	// Post the pending line into the book; the second posting of the same
	// line is refused by state, the book gets one movement.
	posted := want(call(http.MethodPost, lineURL(l3ID)+"/post", map[string]any{"kind": "collection"}), http.StatusOK, "post line 3").data
	pm := ref(posted, "posted_movement")
	if posted["status"] != "posted" || pm["kind"] != "collection" || posted["post_count"] != float64(1) {
		t.Fatalf("posted = %v", posted)
	}
	if b := balance(bankID); b != "1290.00" {
		t.Fatalf("bank after posting = %s", b)
	}
	r = want(call(http.MethodPost, lineURL(l3ID)+"/post", map[string]any{}), http.StatusConflict, "post twice")
	detail(r, "line", "state", "post twice")
	// The reversal undoes the movement and frees the line; a new attempt
	// uses a new idempotency key and posts again.
	want(call(http.MethodPost, lineURL(l3ID)+"/reverse", map[string]any{}), http.StatusBadRequest, "reverse without reason")
	rev := want(call(http.MethodPost, lineURL(l3ID)+"/reverse", map[string]any{"reason": "Yanlış tür"}), http.StatusOK, "reverse posting").data
	if rev["status"] != "pending" || rev["posted_movement"] != nil {
		t.Fatalf("reversed = %v", rev)
	}
	if b := balance(bankID); b != "1170.00" {
		t.Fatalf("bank after reversal = %s", b)
	}
	posted = want(call(http.MethodPost, lineURL(l3ID)+"/post", map[string]any{"kind": "income", "description": "POS"}), http.StatusOK, "post again").data
	if posted["status"] != "posted" || posted["post_count"] != float64(2) || ref(posted, "posted_movement")["kind"] != "income" {
		t.Fatalf("second posting = %v", posted)
	}

	// Manual match with a different amount: the line is a difference of
	// line - movement. A movement matched elsewhere is refused until its
	// line is reset.
	l4 := want(call(http.MethodPost, lines, map[string]any{
		"line_date": day(-3), "direction": "out", "amount": "100", "description": "KIRA + masraf", "reference": "R4",
	}), http.StatusCreated, "line 4").data
	l4ID := l4["uuid"].(string)
	r = want(call(http.MethodPost, lineURL(l4ID)+"/match", map[string]any{"movement_uuid": mOut}), http.StatusConflict, "movement taken")
	detail(r, "movement_uuid", "matched", "movement taken")
	r = want(call(http.MethodPost, lineURL(l4ID)+"/match", map[string]any{"movement_uuid": mIn}), http.StatusBadRequest, "wrong direction")
	detail(r, "movement_uuid", "direction", "wrong direction")
	l2ID := l2["uuid"].(string)
	reset := want(call(http.MethodPost, lineURL(l2ID)+"/reset", map[string]any{}), http.StatusOK, "reset line 2").data
	if reset["status"] != "pending" || reset["matched_movement"] != nil {
		t.Fatalf("reset = %v", reset)
	}
	diff := want(call(http.MethodPost, lineURL(l4ID)+"/match", map[string]any{"movement_uuid": mOut, "note": "Masraf dahil"}), http.StatusOK, "match with difference").data
	if diff["status"] != "difference" || diff["difference_amount"] != "20.00" || diff["match_method"] != "manual" {
		t.Fatalf("difference = %v", diff)
	}

	// Summary: 1 matched, 1 pending, 1 difference, 1 posted; the book on the
	// period end against the stated balance.
	sum := want(call(http.MethodGet, "/v1/tenant/cash/statements/"+stID+"/summary", nil), http.StatusOK, "summary").data
	if sum["matched_count"] != float64(1) || sum["pending_count"] != float64(1) || sum["difference_count"] != float64(1) ||
		sum["posted_count"] != float64(1) || sum["reconciled"] != false || sum["book_balance"] != "1290.00" ||
		sum["closing_balance"] != "1290.00" || sum["balance_difference"] != "0.00" || sum["difference_total"] != "20.00" {
		t.Fatalf("summary = %v", sum)
	}
	facets := want(call(http.MethodGet, lines+"/facets", nil), http.StatusOK, "facets").data["status"].([]any)
	if len(facets) != 4 {
		t.Fatalf("facets = %v", facets)
	}
	filtered := want(call(http.MethodGet, "/v1/tenant/cash/statement-lines?statement="+stID+"&status=pending,difference", nil), http.StatusOK, "line list").data
	if filtered["total"] != float64(2) {
		t.Fatalf("filtered lines = %v", filtered)
	}

	// The period closing refuses while lines up to the day are open; the
	// preview counts them.
	pv := want(call(http.MethodGet, "/v1/tenant/cash/accounts/"+bankID+"/closings/preview?date="+day(-1), nil), http.StatusOK, "preview").data
	if pv["unreconciled_lines"] != float64(2) || pv["expected_amount"] != "1290.00" {
		t.Fatalf("preview = %v", pv)
	}
	r = want(call(http.MethodPost, "/v1/tenant/cash/accounts/"+bankID+"/closings", map[string]any{
		"business_date": day(-1), "counted_amount": "1290.00",
	}), http.StatusConflict, "close with open lines")
	detail(r, "business_date", "unreconciled_lines", "close with open lines")
	// Ignoring needs a reason and resolves the line without a posting.
	want(call(http.MethodPost, lineURL(l2ID)+"/ignore", map[string]any{}), http.StatusBadRequest, "ignore without reason")
	ign := want(call(http.MethodPost, lineURL(l2ID)+"/ignore", map[string]any{"reason": "Defterde var, farklı açıklama"}), http.StatusOK, "ignore line 2").data
	if ign["status"] != "ignored" || ign["note"] != "Defterde var, farklı açıklama" {
		t.Fatalf("ignored = %v", ign)
	}
	want(call(http.MethodPost, lineURL(l4ID)+"/reset", map[string]any{}), http.StatusOK, "reset difference")
	want(call(http.MethodPost, lineURL(l4ID)+"/ignore", map[string]any{"reason": "Banka masrafı ayrı girildi"}), http.StatusOK, "ignore line 4")
	sum = want(call(http.MethodGet, "/v1/tenant/cash/statements/"+stID+"/summary", nil), http.StatusOK, "summary reconciled").data
	if sum["reconciled"] != true || sum["ignored_count"] != float64(2) {
		t.Fatalf("summary = %v", sum)
	}

	// Closing the period locks the statement: no lines, no edits, no
	// delete; reopening frees it.
	cl := want(call(http.MethodPost, "/v1/tenant/cash/accounts/"+bankID+"/closings", map[string]any{
		"business_date": day(-1), "counted_amount": "1290.00",
	}), http.StatusCreated, "close period").data
	if cl["difference"] != "0.00" {
		t.Fatalf("closing = %v", cl)
	}
	st = want(call(http.MethodGet, "/v1/tenant/cash/statements/"+stID, nil), http.StatusOK, "closed statement").data
	if st["status"] != "closed" || st["closing_uuid"] != cl["uuid"] || st["open_count"] != float64(0) || st["line_count"] != float64(4) {
		t.Fatalf("closed statement = %v", st)
	}
	r = want(call(http.MethodPost, lines, map[string]any{"line_date": day(-1), "direction": "in", "amount": "1"}), http.StatusConflict, "line on closed")
	detail(r, "statement", "closed", "line on closed")
	want(call(http.MethodPatch, "/v1/tenant/cash/statements/"+stID, map[string]any{"name": "X"}), http.StatusConflict, "patch closed")
	want(call(http.MethodDelete, "/v1/tenant/cash/statements/"+stID, nil), http.StatusConflict, "delete closed")
	want(call(http.MethodPost, "/v1/tenant/cash/accounts/"+bankID+"/closings/"+cl["uuid"].(string)+"/reopen",
		map[string]any{"reason": "Ekstre düzeltmesi"}), http.StatusOK, "reopen period")
	st = want(call(http.MethodGet, "/v1/tenant/cash/statements/"+stID, nil), http.StatusOK, "reopened statement").data
	if st["status"] != "open" || st["closing_uuid"] != nil {
		t.Fatalf("reopened statement = %v", st)
	}
	// A statement whose line wrote into the book is kept.
	r = want(call(http.MethodDelete, "/v1/tenant/cash/statements/"+stID, nil), http.StatusConflict, "delete with postings")
	detail(r, "statement", "has_postings", "delete with postings")

	// Provider settlement: the payout to the bank is a transfer of both
	// accounts, the fee an expense of the provider account; the reversal
	// undoes all three legs.
	want(call(http.MethodPost, "/v1/tenant/cash/accounts/"+onlineID+"/movements", map[string]any{
		"direction": "in", "amount": "600", "movement_date": day(-4), "description": "Online tahsilat",
	}), http.StatusCreated, "online in")
	ps := want(call(http.MethodPost, "/v1/tenant/cash/statements", map[string]any{
		"account_uuid": onlineID, "name": "iyzico 24. hafta",
	}), http.StatusCreated, "provider statement").data
	if ps["source"] != "provider" {
		t.Fatalf("provider statement = %v", ps)
	}
	psID := ps["uuid"].(string)
	payout := want(call(http.MethodPost, "/v1/tenant/cash/statements/"+psID+"/lines", map[string]any{
		"line_date": day(-1), "direction": "out", "amount": "500", "fee": "10", "line_type": "payout", "description": "Hakediş", "reference": "H1",
	}), http.StatusCreated, "payout line").data
	payoutID := payout["uuid"].(string)
	if payout["status"] != "pending" || payout["fee"] != "10.00" {
		t.Fatalf("payout line = %v", payout)
	}
	r = want(call(http.MethodPost, lineURL(payoutID)+"/post", map[string]any{}), http.StatusBadRequest, "payout without counter")
	detail(r, "counter_account_uuid", "required", "payout without counter")
	r = want(call(http.MethodPost, lineURL(payoutID)+"/post", map[string]any{"counter_account_uuid": bankID, "kind": "expense"}), http.StatusBadRequest, "transfer with kind")
	detail(r, "kind", "invalid", "transfer with kind")
	tp := want(call(http.MethodPost, lineURL(payoutID)+"/post", map[string]any{"counter_account_uuid": bankID}), http.StatusOK, "post payout").data
	if tp["status"] != "posted" || ref(tp, "posted_movement")["kind"] != "transfer_out" || tp["fee_movement_uuid"] == nil {
		t.Fatalf("payout posted = %v", tp)
	}
	if o, b := balance(onlineID), balance(bankID); o != "90.00" || b != "1790.00" {
		t.Fatalf("after payout: online %s bank %s", o, b)
	}
	want(call(http.MethodPost, lineURL(payoutID)+"/reverse", map[string]any{"reason": "Yanlış hesap"}), http.StatusOK, "reverse payout")
	if o, b := balance(onlineID), balance(bankID); o != "600.00" || b != "1290.00" {
		t.Fatalf("after payout reversal: online %s bank %s", o, b)
	}

	// Sheet import through the adapter: a row lands as a line (matched when
	// the book has it), the rollback removes it, a posted line stays.
	scoped := orgctx.WithScope(ctx, orgctx.Scope{ID: org.ID, Currency: "TRY", Timezone: org.Timezone})
	io := cashadapters.NewStatementLinesIO(svc)
	defaults := map[string]any{"statement_uuid": psID}
	prev, err := io.PreviewRow(scoped, map[string]any{"line_date": day(-4), "credit": "600,00", "description": "Tahsilat", "reference": "H2"}, defaults)
	if err != nil || prev.Error != "" || prev.Op != "create" {
		t.Fatalf("preview row = %+v, %v", prev, err)
	}
	res, err := io.ApplyRow(scoped, map[string]any{"line_date": day(-4), "credit": "600,00", "description": "Tahsilat", "reference": "H2"}, defaults)
	if err != nil || !res.OK {
		t.Fatalf("apply row = %+v, %v", res, err)
	}
	imported := want(call(http.MethodGet, lineURL(res.EntityUUID), nil), http.StatusOK, "imported line").data
	if imported["status"] != "matched" || imported["amount"] != "600.00" || imported["direction"] != "in" {
		t.Fatalf("imported line = %v", imported)
	}
	bad, err := io.ApplyRow(scoped, map[string]any{"line_date": "bugün", "credit": "1"}, defaults)
	if err != nil || bad.OK {
		t.Fatalf("bad row = %+v, %v", bad, err)
	}
	if err := io.RevertRow(scoped, res.EntityType, res.EntityUUID, nil); err != nil {
		t.Fatalf("revert: %v", err)
	}
	want(call(http.MethodGet, lineURL(res.EntityUUID), nil), http.StatusNotFound, "reverted line")
	if err := io.RevertRow(scoped, "cash_statement_line", l3ID, nil); err == nil {
		t.Fatal("a posted line was removed by the import rollback")
	}

	// Auto-match over the statement and the clean delete.
	am := want(call(http.MethodPost, "/v1/tenant/cash/statements/"+psID+"/auto-match", map[string]any{}), http.StatusOK, "auto-match").data
	if am["checked"] != float64(1) || am["pending"] != float64(1) {
		t.Fatalf("auto-match = %v", am)
	}
	// A reversed posting still lives in the book, so that statement is kept
	// too; a statement that never posted goes with its lines.
	want(call(http.MethodDelete, "/v1/tenant/cash/statements/"+psID, nil), http.StatusConflict, "delete after reversal")
	tmp := want(call(http.MethodPost, "/v1/tenant/cash/statements", map[string]any{
		"account_uuid": onlineID, "name": "Yanlış yüklenen rapor",
	}), http.StatusCreated, "scratch statement").data["uuid"].(string)
	scratch := want(call(http.MethodPost, "/v1/tenant/cash/statements/"+tmp+"/lines", map[string]any{
		"line_date": day(-4), "direction": "in", "amount": "600", "reference": "H9",
	}), http.StatusCreated, "scratch line").data["uuid"].(string)
	want(call(http.MethodDelete, "/v1/tenant/cash/statements/"+tmp, nil), http.StatusNoContent, "delete scratch statement")
	want(call(http.MethodGet, lineURL(scratch), nil), http.StatusNotFound, "line of deleted statement")
	if o := balance(onlineID); o != "600.00" {
		t.Fatalf("delete changed the book: %s", o)
	}
	listed := want(call(http.MethodGet, "/v1/tenant/cash/statements?account="+onlineID, nil), http.StatusOK, "statements").data
	if listed["total"] != float64(1) {
		t.Fatalf("statements = %v", listed)
	}
}
