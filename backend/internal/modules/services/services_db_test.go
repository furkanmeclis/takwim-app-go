package services_test

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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services"
	servicesadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/adapters"
	serviceshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/handler"
	servicesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/repository"
	servicesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/usecase"
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

// TestServicesOverHTTP drives the service catalog (TEC-894) over HTTP with
// the real resolver and SQL: categories, validation, names unique per
// organization, branch offers limited to a branch manager's scope, staff
// links, read-only reception, filters, the import match and the activity
// trail.
// Runs when TEST_DATABASE_URL points at a migrated database.
func TestServicesOverHTTP(t *testing.T) {
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
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Services " + suffix, OwnerUserID: owner.ID})
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
	svc := servicesusecase.New(servicesrepository.NewPostgres(pool, q), nil)
	mux := http.NewServeMux()
	branches.RegisterRoutes(mux, branchhandler.New(branchusecase.New(branchrepository.NewPostgres(q), nil), rec), branches.EngineHandlers{}, tokens, loader, orgSvc)
	organizations.RegisterTenantRoutes(mux, orghandler.New(orgSvc, rec), tokens, loader, orgSvc)
	staff.RegisterRoutes(mux, staffhandler.New(staffusecase.New(staffrepository.NewPostgres(pool, q), nil), rec), staff.EngineHandlers{}, tokens, loader, orgSvc)
	services.RegisterRoutes(mux, serviceshandler.New(svc, rec), services.EngineHandlers{}, tokens, loader, orgSvc)

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

	// Branch A is the default "Merkez"; B is added; carol manages A only.
	list := want(call(owner, http.MethodGet, "/v1/tenant/branches", nil), http.StatusOK, "branches")
	aUUID := list.data["items"].([]any)[0].(map[string]any)["uuid"].(string)
	bUUID := want(call(owner, http.MethodPost, "/v1/tenant/branches", map[string]any{"code": "KDK", "name": "Kadıköy"}),
		http.StatusCreated, "create B").data["uuid"].(string)
	want(call(owner, http.MethodPatch, "/v1/tenant/members/"+carol.Uuid.String(), map[string]any{
		"branch_scope": "selected", "branch_uuids": []string{aUUID},
	}), http.StatusOK, "narrow carol")

	// Categories: names are unique, case-insensitive.
	catUUID := want(call(owner, http.MethodPost, "/v1/tenant/service-categories", map[string]any{"name": "Saç", "sort_order": 1}),
		http.StatusCreated, "create category").data["uuid"].(string)
	want(call(owner, http.MethodPost, "/v1/tenant/service-categories", map[string]any{"name": "SAÇ"}),
		http.StatusConflict, "duplicate category")

	// A 30 minute cut in both branches at the catalog price.
	s1 := want(call(owner, http.MethodPost, "/v1/tenant/services", map[string]any{
		"name": "Saç kesimi", "category_uuid": catUUID, "duration_minutes": 30, "buffer_after_minutes": 10,
		"price": "450.00", "resource_kinds": []string{"chair"}, "branch_uuids": []string{aUUID, bUUID},
	}), http.StatusCreated, "create service")
	s1UUID := s1.data["uuid"].(string)
	if s1.data["price"] != "450.00" || s1.data["payment_mode"] != "on_site" || s1.data["staff_mode"] == "" ||
		len(s1.data["offers"].([]any)) != 2 || s1.data["category"].(map[string]any)["uuid"] != catUUID {
		t.Fatalf("service = %s", s1.raw)
	}

	// Validation.
	r := want(call(owner, http.MethodPost, "/v1/tenant/services", map[string]any{
		"name": "X", "duration_minutes": 3, "price": "-1", "payment_mode": "cash", "resource_kinds": []string{"boat"},
	}), http.StatusBadRequest, "invalid service")
	for _, f := range []string{"duration_minutes", "price", "payment_mode", "resource_kinds.0"} {
		if !hasDetail(r, f, "") {
			t.Fatalf("no %s detail: %s", f, r.raw)
		}
	}
	want(call(owner, http.MethodPost, "/v1/tenant/services", map[string]any{"name": "saç KESİMİ", "duration_minutes": 30, "price": "1"}),
		http.StatusConflict, "duplicate name")
	s2UUID := want(call(owner, http.MethodPost, "/v1/tenant/services", map[string]any{
		"name": "Fön", "duration_minutes": 20, "price": "200", "payment_mode": "both",
	}), http.StatusCreated, "uncategorized service").data["uuid"].(string)

	// Carol reads the whole catalog but cannot edit it; she prices A only.
	r = want(call(carol, http.MethodGet, "/v1/tenant/services", nil), http.StatusOK, "carol list")
	if total(r) != 2 {
		t.Fatalf("carol sees %d services", total(r))
	}
	want(call(carol, http.MethodPatch, "/v1/tenant/services/"+s1UUID, map[string]any{"price": "1"}), http.StatusForbidden, "carol edits catalog")
	r = want(call(carol, http.MethodPut, "/v1/tenant/services/"+s1UUID+"/offers", map[string]any{
		"offers": []map[string]any{{"branch_uuid": bUUID}},
	}), http.StatusBadRequest, "carol prices B")
	if !hasDetail(r, "offers.0.branch_uuid", "not_found") {
		t.Fatalf("offer detail: %s", r.raw)
	}
	r = want(call(carol, http.MethodPut, "/v1/tenant/services/"+s1UUID+"/offers", map[string]any{
		"offers": []map[string]any{{"branch_uuid": aUUID, "price": "400", "duration_minutes": 45}},
	}), http.StatusOK, "carol prices A")
	offers := map[string]map[string]any{}
	for _, raw := range r.data["offers"].([]any) {
		o := raw.(map[string]any)
		offers[o["branch_uuid"].(string)] = o
	}
	if a, b := offers[aUUID], offers[bUUID]; a == nil || b == nil || a["effective_price"] != "400.00" ||
		a["effective_duration_minutes"].(float64) != 45 || a["editable"] != true || b["price"] != nil ||
		b["effective_price"] != "450.00" || b["editable"] != false {
		t.Fatalf("offers = %v", r.data["offers"])
	}
	// An empty list removes carol's branch only.
	r = want(call(carol, http.MethodPut, "/v1/tenant/services/"+s1UUID+"/offers", map[string]any{"offers": []any{}}),
		http.StatusOK, "carol removes A")
	if o := r.data["offers"].([]any); len(o) != 1 || o[0].(map[string]any)["branch_uuid"] != bUUID {
		t.Fatalf("offers after removal = %v", o)
	}
	want(call(owner, http.MethodPut, "/v1/tenant/services/"+s1UUID+"/offers", map[string]any{
		"offers": []map[string]any{{"branch_uuid": aUUID}, {"branch_uuid": bUUID, "is_active": false}},
	}), http.StatusOK, "owner sets offers")

	// Staff who may perform the service, with a personal price.
	stUUID := want(call(owner, http.MethodPost, "/v1/tenant/staff", map[string]any{"name": "Ayşe", "branch_uuids": []string{aUUID}}),
		http.StatusCreated, "create staff").data["uuid"].(string)
	r = want(call(owner, http.MethodPut, "/v1/tenant/services/"+s1UUID+"/staff", map[string]any{
		"staff": []map[string]any{{"staff_uuid": stUUID, "price": "500"}},
	}), http.StatusOK, "link staff")
	if st := r.data["staff"].([]any); len(st) != 1 || st[0].(map[string]any)["price"] != "500.00" {
		t.Fatalf("staff = %v", r.data["staff"])
	}
	r = want(call(owner, http.MethodPut, "/v1/tenant/services/"+s1UUID+"/staff", map[string]any{
		"staff": []map[string]any{{"staff_uuid": uuid.NewString()}},
	}), http.StatusBadRequest, "unknown staff")
	if !hasDetail(r, "staff.0.staff_uuid", "not_found") {
		t.Fatalf("staff detail: %s", r.raw)
	}

	// Reception reads, never writes.
	want(call(dave, http.MethodGet, "/v1/tenant/services/"+s1UUID, nil), http.StatusOK, "dave reads")
	want(call(dave, http.MethodPut, "/v1/tenant/services/"+s1UUID+"/offers", map[string]any{"offers": []any{}}),
		http.StatusForbidden, "dave prices")
	want(call(dave, http.MethodPost, "/v1/tenant/service-categories", map[string]any{"name": "K"}), http.StatusForbidden, "dave creates category")

	// Filters and facets.
	r = want(call(owner, http.MethodGet, "/v1/tenant/services?category=none", nil), http.StatusOK, "uncategorized")
	if total(r) != 1 || r.data["items"].([]any)[0].(map[string]any)["uuid"] != s2UUID {
		t.Fatalf("uncategorized = %s", r.raw)
	}
	r = want(call(owner, http.MethodGet, "/v1/tenant/services?branch="+aUUID+"&payment_mode=on_site", nil), http.StatusOK, "branch filter")
	if total(r) != 1 {
		t.Fatalf("branch filter total = %d", total(r))
	}
	r = want(call(owner, http.MethodGet, "/v1/tenant/services/facets", nil), http.StatusOK, "facets")
	if len(r.data["category"].([]any)) != 2 || len(r.data["payment_mode"].([]any)) != 2 {
		t.Fatalf("facets = %s", r.raw)
	}

	// Import: the name matches (case-insensitive); a new category is created.
	io := servicesadapters.NewIO(svc)
	scoped := orgctx.WithOrganizationID(ctx, org.ID)
	res, err := io.ApplyRow(scoped, map[string]any{"name": "fön", "duration_minutes": "25", "price": "220,50"}, nil)
	if err != nil || !res.OK || res.EntityUUID != s2UUID || res.Op != "update" {
		t.Fatalf("import update = %+v, %v", res, err)
	}
	res, err = io.ApplyRow(scoped, map[string]any{
		"category": "Sakal", "name": "Sakal tıraşı", "duration_minutes": "15", "price": "150", "branch_codes": "KDK",
	}, nil)
	if err != nil || !res.OK || res.Op != "create" {
		t.Fatalf("import create = %+v, %v", res, err)
	}
	r = want(call(owner, http.MethodGet, "/v1/tenant/services/"+res.EntityUUID, nil), http.StatusOK, "imported")
	if r.data["category"].(map[string]any)["name"] != "Sakal" || len(r.data["offers"].([]any)) != 1 {
		t.Fatalf("imported = %s", r.raw)
	}
	r = want(call(owner, http.MethodGet, "/v1/tenant/services/"+s2UUID, nil), http.StatusOK, "updated by import")
	if r.data["price"] != "220.50" || r.data["duration_minutes"].(float64) != 25 {
		t.Fatalf("import update = %s", r.raw)
	}

	// Deleting a category leaves its services uncategorized.
	want(call(owner, http.MethodDelete, "/v1/tenant/service-categories/"+catUUID, nil), http.StatusOK, "delete category")
	r = want(call(owner, http.MethodGet, "/v1/tenant/services/"+s1UUID, nil), http.StatusOK, "after category delete")
	if r.data["category"] != nil {
		t.Fatalf("category not cleared: %v", r.data["category"])
	}

	// Without storage the cover upload is unavailable, not a crash.
	want(call(owner, http.MethodDelete, "/v1/tenant/services/"+s1UUID+"/image", nil), http.StatusNotFound, "no image")

	// Activity, then delete.
	r = want(call(owner, http.MethodGet, "/v1/tenant/services/"+s1UUID+"/activity", nil), http.StatusOK, "activity")
	if total(r) < 3 {
		t.Fatalf("activity = %s", r.raw)
	}
	want(call(owner, http.MethodDelete, "/v1/tenant/services/"+s1UUID, nil), http.StatusOK, "delete")
	want(call(owner, http.MethodGet, "/v1/tenant/services/"+s1UUID, nil), http.StatusNotFound, "deleted")
}
