package resources_test

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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations"
	orghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/handler"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources"
	resourcesadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/adapters"
	resourceshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/handler"
	resourcesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/repository"
	resourcesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/usecase"
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

// TestResourcesOverHTTP drives the resource catalog (TEC-891) over HTTP with
// the real resolver and SQL: capacity ranges, names unique per branch, own
// working hours, branch scope of a branch manager, read-only reception,
// closures, the import match and the activity trail.
// Runs when TEST_DATABASE_URL points at a migrated database.
func TestResourcesOverHTTP(t *testing.T) {
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
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Resources " + suffix, OwnerUserID: owner.ID})
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
	svc := resourcesusecase.New(resourcesrepository.NewPostgres(pool, q), nil)
	mux := http.NewServeMux()
	branches.RegisterRoutes(mux, branchhandler.New(branchusecase.New(branchrepository.NewPostgres(q), nil), rec), branches.EngineHandlers{}, tokens, loader, orgSvc)
	organizations.RegisterTenantRoutes(mux, orghandler.New(orgSvc, rec), tokens, loader, orgSvc)
	resources.RegisterRoutes(mux, resourceshandler.New(svc, rec), resources.EngineHandlers{}, tokens, loader, orgSvc)

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
	total := func(r result) int {
		return int(r.data["total"].(float64))
	}

	// Branch A is the default "Merkez"; B is added; carol manages A only.
	list := want(call(owner, http.MethodGet, "/v1/tenant/branches", nil), http.StatusOK, "branches")
	aUUID := list.data["items"].([]any)[0].(map[string]any)["uuid"].(string)
	bUUID := want(call(owner, http.MethodPost, "/v1/tenant/branches", map[string]any{"code": "KDK", "name": "Kadıköy"}),
		http.StatusCreated, "create B").data["uuid"].(string)
	want(call(owner, http.MethodPatch, "/v1/tenant/members/"+carol.Uuid.String(), map[string]any{
		"branch_scope": "selected", "branch_uuids": []string{aUUID},
	}), http.StatusOK, "narrow carol")

	// A table for 2–4 people in the garden of A.
	t1 := want(call(owner, http.MethodPost, "/v1/tenant/resources", map[string]any{
		"branch_uuid": aUUID, "kind": "table", "name": "Masa 1", "zone": "Bahçe",
		"min_capacity": 2, "capacity": 4, "combinable": true,
	}), http.StatusCreated, "create table")
	t1UUID := t1.data["uuid"].(string)
	if t1.data["working_hours"] != nil || t1.data["branch"].(map[string]any)["uuid"] != aUUID || t1.data["is_active"] != true {
		t.Fatalf("table = %v", t1.data)
	}
	got := want(call(owner, http.MethodGet, "/v1/tenant/resources/"+t1UUID, nil), http.StatusOK, "get table")
	if got.data["branch_hours"] == nil {
		t.Fatalf("detail misses the branch hours: %s", got.raw)
	}

	// Validation: unknown kind, inverted range, missing branch.
	r := want(call(owner, http.MethodPost, "/v1/tenant/resources", map[string]any{
		"branch_uuid": aUUID, "kind": "boat", "name": "X", "min_capacity": 5, "capacity": 2,
	}), http.StatusBadRequest, "invalid resource")
	if !hasDetail(r, "kind", "invalid") || !hasDetail(r, "capacity", "below_min") {
		t.Fatalf("validation details: %s", r.raw)
	}
	r = want(call(owner, http.MethodPost, "/v1/tenant/resources", map[string]any{"kind": "chair", "name": "X"}),
		http.StatusBadRequest, "no branch")
	if !hasDetail(r, "branch_uuid", "required") {
		t.Fatalf("branch detail: %s", r.raw)
	}

	// Names are unique per branch (case-insensitive), not across branches.
	want(call(owner, http.MethodPost, "/v1/tenant/resources", map[string]any{
		"branch_uuid": aUUID, "kind": "table", "name": "masa 1",
	}), http.StatusConflict, "duplicate name")
	t2UUID := want(call(owner, http.MethodPost, "/v1/tenant/resources", map[string]any{
		"branch_uuid": bUUID, "kind": "table", "name": "Masa 1",
	}), http.StatusCreated, "same name in B").data["uuid"].(string)

	// Own working hours replace the branch's; a bad day is rejected.
	r = want(call(owner, http.MethodPatch, "/v1/tenant/resources/"+t1UUID, map[string]any{
		"working_hours": map[string]any{
			"mon": map[string]any{"open": "10:00", "close": "09:00"},
		},
	}), http.StatusBadRequest, "bad hours")
	if !hasDetail(r, "working_hours.mon.close", "before_open") {
		t.Fatalf("hours detail: %s", r.raw)
	}
	day := map[string]any{"open": "12:00", "close": "23:00"}
	closed := map[string]any{"closed": true}
	r = want(call(owner, http.MethodPatch, "/v1/tenant/resources/"+t1UUID, map[string]any{
		"working_hours": map[string]any{"mon": closed, "tue": day, "wed": day, "thu": day, "fri": day, "sat": day, "sun": day},
	}), http.StatusOK, "own hours")
	if r.data["working_hours"].(map[string]any)["tue"].(map[string]any)["open"] != "12:00" {
		t.Fatalf("hours = %v", r.data["working_hours"])
	}
	r = want(call(owner, http.MethodPatch, "/v1/tenant/resources/"+t1UUID, map[string]any{"working_hours": nil}),
		http.StatusOK, "back to branch hours")
	if r.data["working_hours"] != nil {
		t.Fatalf("hours not cleared: %v", r.data["working_hours"])
	}

	// List filters and facets.
	r = want(call(owner, http.MethodGet, "/v1/tenant/resources?zone=Bahçe", nil), http.StatusOK, "zone filter")
	if total(r) != 1 {
		t.Fatalf("zone filter total = %d", total(r))
	}
	r = want(call(owner, http.MethodGet, "/v1/tenant/resources/facets", nil), http.StatusOK, "facets")
	if kinds := r.data["kind"].([]any); len(kinds) != 1 || kinds[0].(map[string]any)["count"].(float64) != 2 {
		t.Fatalf("kind facets = %v", r.data["kind"])
	}

	// Carol sees and edits A only; B is 404 and moving into B is refused.
	r = want(call(carol, http.MethodGet, "/v1/tenant/resources", nil), http.StatusOK, "carol list")
	if total(r) != 1 {
		t.Fatalf("carol sees %d resources", total(r))
	}
	want(call(carol, http.MethodGet, "/v1/tenant/resources/"+t2UUID, nil), http.StatusNotFound, "carol reads B")
	want(call(carol, http.MethodPatch, "/v1/tenant/resources/"+t2UUID, map[string]any{"name": "X"}), http.StatusNotFound, "carol edits B")
	r = want(call(carol, http.MethodPatch, "/v1/tenant/resources/"+t1UUID, map[string]any{"branch_uuid": bUUID}),
		http.StatusBadRequest, "carol moves to B")
	if !hasDetail(r, "branch_uuid", "not_found") {
		t.Fatalf("move detail: %s", r.raw)
	}
	want(call(carol, http.MethodPatch, "/v1/tenant/resources/"+t1UUID, map[string]any{"capacity": 6}), http.StatusOK, "carol edits A")

	// Reception reads, never writes.
	want(call(dave, http.MethodGet, "/v1/tenant/resources/"+t1UUID, nil), http.StatusOK, "dave reads")
	want(call(dave, http.MethodPost, "/v1/tenant/resources", map[string]any{"branch_uuid": aUUID, "kind": "chair", "name": "K"}),
		http.StatusForbidden, "dave creates")
	want(call(dave, http.MethodPost, "/v1/tenant/resources/"+t1UUID+"/closures", map[string]any{
		"starts_at": "2030-01-01T09:00:00Z", "ends_at": "2030-01-01T18:00:00Z",
	}), http.StatusForbidden, "dave closes")

	// Closures.
	closuresPath := "/v1/tenant/resources/" + t1UUID + "/closures"
	r = want(call(owner, http.MethodPost, closuresPath, map[string]any{
		"starts_at": "2030-01-02T09:00:00Z", "ends_at": "2030-01-01T09:00:00Z",
	}), http.StatusBadRequest, "inverted closure")
	if !hasDetail(r, "ends_at", "invalid") {
		t.Fatalf("closure detail: %s", r.raw)
	}
	c := want(call(owner, http.MethodPost, closuresPath, map[string]any{
		"starts_at": "2030-01-01T09:00:00Z", "ends_at": "2030-01-01T18:00:00Z", "reason": "Boya",
	}), http.StatusCreated, "create closure")
	cUUID := c.data["uuid"].(string)
	r = want(call(owner, http.MethodPatch, closuresPath+"/"+cUUID, map[string]any{"reason": "Bakım"}), http.StatusOK, "patch closure")
	if r.data["reason"] != "Bakım" {
		t.Fatalf("closure = %v", r.data)
	}
	r = want(call(dave, http.MethodGet, closuresPath, nil), http.StatusOK, "list closures")
	if total(r) != 1 {
		t.Fatalf("closures total = %d", total(r))
	}
	want(call(owner, http.MethodPatch, "/v1/tenant/resources/"+t2UUID+"/closures/"+cUUID, map[string]any{"reason": "X"}),
		http.StatusNotFound, "closure under another resource")
	want(call(owner, http.MethodDelete, closuresPath+"/"+cUUID, nil), http.StatusOK, "delete closure")

	// Import: branch code + name updates the existing row, else creates.
	io := resourcesadapters.NewIO(svc)
	scoped := orgctx.WithOrganizationID(ctx, org.ID)
	res, err := io.ApplyRow(scoped, map[string]any{"branch_code": "KDK", "kind": "table", "name": "masa 1", "capacity": "8"}, nil)
	if err != nil || res.EntityUUID != t2UUID {
		t.Fatalf("import update = %+v, %v", res, err)
	}
	res, err = io.ApplyRow(scoped, map[string]any{"branch_code": "KDK", "kind": "bay", "name": "Yıkama 1"}, nil)
	if err != nil || res.EntityUUID == "" || res.EntityUUID == t2UUID {
		t.Fatalf("import create = %+v, %v", res, err)
	}

	// Delete, then the activity trail of a deleted resource is gone with it.
	want(call(owner, http.MethodDelete, "/v1/tenant/resources/"+t1UUID, nil), http.StatusOK, "delete")
	want(call(owner, http.MethodGet, "/v1/tenant/resources/"+t1UUID, nil), http.StatusNotFound, "deleted")
	r = want(call(owner, http.MethodGet, "/v1/tenant/resources/"+t2UUID+"/activity", nil), http.StatusOK, "activity")
	if total(r) < 1 {
		t.Fatalf("activity = %s", r.raw)
	}
}
