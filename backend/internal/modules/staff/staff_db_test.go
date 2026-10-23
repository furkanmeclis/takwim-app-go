package staff_test

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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches"
	branchhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/handler"
	branchrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/repository"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	entitlementsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/entitlements"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations"
	orghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/handler"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
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

// TestStaffOverHTTP drives the staff resource (TEC-890) over HTTP with the
// real resolver and SQL: several branches per person, shifts that never
// overlap across branches (A04), breaks, branch scope of a branch manager,
// account links, time off, seat limits and the activity trail.
// Runs when TEST_DATABASE_URL points at a migrated database.
func TestStaffOverHTTP(t *testing.T) {
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
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Staff " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM activity_events WHERE organization_id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{owner.ID, carol.ID, dave.ID})
	})
	// Room for the people below; the seat check is tested at the end.
	if _, err := pool.Exec(ctx, "UPDATE organizations SET staff_limit = 50 WHERE id = $1", org.ID); err != nil {
		t.Fatal(err)
	}
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
	staffSvc := staffusecase.New(staffrepository.NewPostgres(pool, q), nil)
	staffSvc.SetLimiter(entitlementsmodule.New(q))
	mux := http.NewServeMux()
	branches.RegisterRoutes(mux, branchhandler.New(branchusecase.New(branchrepository.NewPostgres(q), nil), rec), branches.EngineHandlers{}, tokens, loader, orgSvc)
	organizations.RegisterTenantRoutes(mux, orghandler.New(orgSvc, rec), tokens, loader, orgSvc)
	staff.RegisterRoutes(mux, staffhandler.New(staffSvc, rec), staff.EngineHandlers{}, tokens, loader, orgSvc)

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

	// Branch A is the default "Merkez"; B is added; carol manages A only.
	list := want(call(owner, http.MethodGet, "/v1/tenant/branches", nil), http.StatusOK, "branches")
	aUUID := list.data["items"].([]any)[0].(map[string]any)["uuid"].(string)
	bUUID := want(call(owner, http.MethodPost, "/v1/tenant/branches", map[string]any{"code": "KDK", "name": "Kadıköy"}),
		http.StatusCreated, "create B").data["uuid"].(string)
	want(call(owner, http.MethodPatch, "/v1/tenant/members/"+carol.Uuid.String(), map[string]any{
		"branch_scope": "selected", "branch_uuids": []string{aUUID},
	}), http.StatusOK, "narrow carol")

	// One person in two branches.
	ayse := want(call(owner, http.MethodPost, "/v1/tenant/staff", map[string]any{
		"name": "Ayşe Yılmaz", "title": "Kuaför", "color": "#ff0000", "branch_uuids": []string{aUUID, bUUID},
	}), http.StatusCreated, "create ayse")
	ayseUUID := ayse.data["uuid"].(string)
	if len(ayse.data["branches"].([]any)) != 2 || ayse.data["account"] != nil || ayse.data["color"] != "#FF0000" {
		t.Fatalf("ayse = %v", ayse.data)
	}
	shiftsPath := func(staffUUID, branchUUID string) string {
		return "/v1/tenant/staff/" + staffUUID + "/branches/" + branchUUID + "/shifts"
	}
	want(call(owner, http.MethodPut, shiftsPath(ayseUUID, aUUID), map[string]any{
		"mon": []any{map[string]any{"start": "09:00", "end": "13:00", "breaks": []any{map[string]any{"start": "11:00", "end": "11:15"}}}},
	}), http.StatusOK, "shifts A")

	// A04: the same hours in B overlap A; adjacent hours do not.
	r := want(call(owner, http.MethodPut, shiftsPath(ayseUUID, bUUID), map[string]any{
		"mon": []any{map[string]any{"start": "12:00", "end": "18:00"}},
	}), http.StatusBadRequest, "overlapping shifts B")
	if !hasDetail(r, "shifts.mon.0.start", "branch_overlap") {
		t.Fatalf("overlap detail missing: %s", r.raw)
	}
	r = want(call(owner, http.MethodPut, shiftsPath(ayseUUID, bUUID), map[string]any{
		"mon": []any{map[string]any{"start": "13:00", "end": "18:00"}},
		"tue": []any{map[string]any{"start": "14:00", "end": "18:00"}, map[string]any{"start": "09:00", "end": "12:00"}},
	}), http.StatusOK, "shifts B")
	for _, a := range r.data["assignments"].([]any) {
		as := a.(map[string]any)
		if as["branch_uuid"] == bUUID {
			tue := as["shifts"].(map[string]any)["tue"].([]any)
			if tue[0].(map[string]any)["start"] != "09:00" {
				t.Fatalf("shifts are not sorted: %v", tue)
			}
		}
	}
	r = want(call(owner, http.MethodPut, shiftsPath(ayseUUID, aUUID), map[string]any{
		"wed": []any{map[string]any{"start": "09:00", "end": "12:00", "breaks": []any{map[string]any{"start": "12:00", "end": "12:30"}}}},
		"thu": []any{map[string]any{"start": "10:00", "end": "09:00"}},
		"fri": []any{map[string]any{"start": "09:00", "end": "12:00"}, map[string]any{"start": "11:00", "end": "13:00"}},
	}), http.StatusBadRequest, "invalid shifts")
	if !hasDetail(r, "shifts.wed.0.breaks.0.start", "outside") || !hasDetail(r, "shifts.thu.0.end", "invalid") ||
		!hasDetail(r, "shifts.fri.1.start", "overlap") {
		t.Fatalf("invalid shift details: %s", r.raw)
	}

	// Branch manager of A: sees ayse, cannot reach B's assignment, keeps
	// B when replacing the branches, cannot delete a person of B too.
	want(call(carol, http.MethodGet, "/v1/tenant/staff/"+ayseUUID, nil), http.StatusOK, "carol get ayse")
	want(call(carol, http.MethodPut, shiftsPath(ayseUUID, bUUID), map[string]any{}), http.StatusNotFound, "carol shifts B")
	r = want(call(carol, http.MethodPatch, "/v1/tenant/staff/"+ayseUUID, map[string]any{"branch_uuids": []string{aUUID}}),
		http.StatusOK, "carol patch ayse")
	if len(r.data["branches"].([]any)) != 2 {
		t.Fatalf("out-of-scope branch dropped: %v", r.data["branches"])
	}
	want(call(carol, http.MethodDelete, "/v1/tenant/staff/"+ayseUUID, nil), http.StatusForbidden, "carol delete ayse")
	r = want(call(carol, http.MethodPost, "/v1/tenant/staff", map[string]any{"name": "Mehmet", "branch_uuids": []string{bUUID}}),
		http.StatusBadRequest, "carol create in B")
	if !hasDetail(r, "branch_uuids.0", "not_found") {
		t.Fatalf("carol create in B: %s", r.raw)
	}
	want(call(carol, http.MethodPost, "/v1/tenant/staff", map[string]any{"name": "Mehmet"}), http.StatusBadRequest, "carol create without branch")
	mehmet := want(call(carol, http.MethodPost, "/v1/tenant/staff", map[string]any{"name": "Mehmet", "branch_uuids": []string{aUUID}}),
		http.StatusCreated, "carol create in A").data["uuid"].(string)
	zeynep := want(call(owner, http.MethodPost, "/v1/tenant/staff", map[string]any{"name": "Zeynep", "branch_uuids": []string{bUUID}}),
		http.StatusCreated, "create zeynep").data["uuid"].(string)
	want(call(carol, http.MethodGet, "/v1/tenant/staff/"+zeynep, nil), http.StatusNotFound, "carol get zeynep")
	r = want(call(carol, http.MethodGet, "/v1/tenant/staff", nil), http.StatusOK, "carol list")
	if r.data["total"].(float64) != 2 {
		t.Fatalf("carol list = %v", r.data)
	}
	r = want(call(owner, http.MethodGet, "/v1/tenant/staff?branch="+bUUID, nil), http.StatusOK, "owner list B")
	if r.data["total"].(float64) != 2 {
		t.Fatalf("branch filter = %v", r.data)
	}
	want(call(carol, http.MethodDelete, "/v1/tenant/staff/"+mehmet, nil), http.StatusOK, "carol delete mehmet")

	// Reception reads, never writes.
	want(call(dave, http.MethodGet, "/v1/tenant/staff", nil), http.StatusOK, "dave list")
	want(call(dave, http.MethodPost, "/v1/tenant/staff", map[string]any{"name": "X", "branch_uuids": []string{aUUID}}),
		http.StatusForbidden, "dave create")

	// Account links: one staff record per member.
	r = want(call(owner, http.MethodPatch, "/v1/tenant/staff/"+ayseUUID, map[string]any{"account_uuid": carol.Uuid.String()}),
		http.StatusOK, "link carol")
	if r.data["account"].(map[string]any)["email"] != carol.Email {
		t.Fatalf("account = %v", r.data["account"])
	}
	want(call(owner, http.MethodPatch, "/v1/tenant/staff/"+zeynep, map[string]any{"account_uuid": carol.Uuid.String()}),
		http.StatusConflict, "link carol twice")
	r = want(call(owner, http.MethodGet, "/v1/tenant/staff/linkable-members", nil), http.StatusOK, "linkable")
	for _, raw := range r.data["items"].([]any) {
		if raw.(map[string]any)["uuid"] == carol.Uuid.String() {
			t.Fatal("a linked member is still linkable")
		}
	}

	// Time off.
	off := want(call(owner, http.MethodPost, "/v1/tenant/staff/"+ayseUUID+"/time-off", map[string]any{
		"kind": "sick", "starts_at": "2030-03-01T06:00:00Z", "ends_at": "2030-03-02T06:00:00Z", "note": "grip",
	}), http.StatusCreated, "time off").data["uuid"].(string)
	want(call(owner, http.MethodPost, "/v1/tenant/staff/"+ayseUUID+"/time-off", map[string]any{
		"starts_at": "2030-03-02T06:00:00Z", "ends_at": "2030-03-01T06:00:00Z",
	}), http.StatusBadRequest, "reversed time off")
	want(call(owner, http.MethodPatch, "/v1/tenant/staff/"+ayseUUID+"/time-off/"+off, map[string]any{"note": "grip, rapor"}),
		http.StatusOK, "patch time off")
	r = want(call(owner, http.MethodGet, "/v1/tenant/staff/"+ayseUUID+"/time-off?from=2030-03-01T12:00:00Z", nil), http.StatusOK, "list time off")
	if r.data["total"].(float64) != 1 {
		t.Fatalf("time off = %v", r.data)
	}
	want(call(owner, http.MethodPatch, "/v1/tenant/staff/"+zeynep+"/time-off/"+off, map[string]any{"note": "x"}),
		http.StatusNotFound, "time off through another person")
	want(call(dave, http.MethodDelete, "/v1/tenant/staff/"+ayseUUID+"/time-off/"+off, nil), http.StatusForbidden, "dave delete time off")
	want(call(owner, http.MethodDelete, "/v1/tenant/staff/"+ayseUUID+"/time-off/"+off, nil), http.StatusOK, "delete time off")

	// Seats: owner, carol, dave + zeynep (no account) = 4.
	if _, err := pool.Exec(ctx, "UPDATE organizations SET staff_limit = 4 WHERE id = $1", org.ID); err != nil {
		t.Fatal(err)
	}
	want(call(owner, http.MethodPost, "/v1/tenant/staff", map[string]any{"name": "Ali", "branch_uuids": []string{aUUID}}),
		http.StatusConflict, "seat limit")
	want(call(owner, http.MethodPost, "/v1/tenant/staff", map[string]any{"name": "Owner", "account_uuid": owner.Uuid.String()}),
		http.StatusCreated, "owner as staff uses its own seat")

	// History: the shift edit of B carries its branch and the changes.
	r = want(call(owner, http.MethodGet, "/v1/tenant/staff/"+ayseUUID+"/activity", nil), http.StatusOK, "activity")
	found := false
	for _, raw := range r.data["items"].([]any) {
		e := raw.(map[string]any)
		if e["action"] == "staff.shifts_updated" && e["payload"].(map[string]any)["branch"] == "Kadıköy" {
			changes := e["payload"].(map[string]any)["changes"].(map[string]any)
			if _, ok := changes["mon"]; ok {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("shift history = %s", r.raw)
	}
}
