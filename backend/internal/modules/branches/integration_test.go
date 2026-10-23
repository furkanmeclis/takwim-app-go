package branches_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/adapters"
	branchhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/handler"
	branchrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/repository"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	bulkhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/bulk/handler"
	bulkusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/bulk/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type mapLoader map[uuid.UUID]int64

func (m mapLoader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, _ := uuid.Parse(c.Subject)
	return authctx.Principal{UserID: id, UserInternal: m[id]}, nil
}

// TestTenantBranchesEndToEnd drives /v1/tenant/branches over HTTP with the
// real organization resolver, SQL, activity recorder and bulk engine.
// Runs when TEST_DATABASE_URL points at a migrated database.
func TestTenantBranchesEndToEnd(t *testing.T) {
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
	alice, bob := newUser("alice"), newUser("bob")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	newOrg := func(name string, owner db.User) orgmodel.Organization {
		org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: name + " " + suffix, OwnerUserID: owner.ID})
		if err != nil {
			t.Fatal(err)
		}
		return org
	}
	orgA, orgB := newOrg("Acme", alice), newOrg("Beta", bob)
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{orgA.ID, orgB.ID})
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{alice.ID, bob.ID})
	})

	rec := activity.NewRecorder(q, nil)
	svc := branchusecase.New(branchrepository.NewPostgres(q), nil)
	bulkSvc := bulkusecase.New(q, bulkengine.NewRegistry(adapters.NewBulk(svc)), nil, nil, rec, config.BulkConfig{}, nil)
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	branches.RegisterRoutes(mux, branchhandler.New(svc, rec), branches.EngineHandlers{
		Bulk: bulkhandler.New(bulkSvc).TenantExecute(branches.Resource),
	}, tokens, mapLoader{alice.Uuid: alice.ID, bob.Uuid: bob.ID}, orgSvc)

	call := func(user db.User, org orgmodel.Organization, method, path string, body any) (int, map[string]any) {
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
		var env map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		data, _ := env["data"].(map[string]any)
		return rr.Code, data
	}

	// Every new organization starts with its default branch.
	code, page := call(alice, orgA, http.MethodGet, "/v1/tenant/branches", nil)
	if code != http.StatusOK || page["total"].(float64) != 1 {
		t.Fatalf("list A = %d %v", code, page)
	}
	defaultA := page["items"].([]any)[0].(map[string]any)
	if defaultA["code"] != "MRK" || defaultA["name"] != "Merkez" {
		t.Fatalf("default branch = %v", defaultA)
	}
	aUUID := defaultA["uuid"].(string)

	// Not a member of B.
	if code, _ := call(alice, orgB, http.MethodGet, "/v1/tenant/branches", nil); code != http.StatusForbidden {
		t.Fatalf("alice in org B = %d, want 403", code)
	}
	// B cannot see, change or bulk-touch A's branch.
	if code, _ := call(bob, orgB, http.MethodGet, "/v1/tenant/branches/"+aUUID, nil); code != http.StatusNotFound {
		t.Fatalf("bob reads A branch = %d, want 404", code)
	}
	if code, _ := call(bob, orgB, http.MethodPatch, "/v1/tenant/branches/"+aUUID, map[string]any{"name": "x"}); code != http.StatusNotFound {
		t.Fatalf("bob patches A branch = %d, want 404", code)
	}
	code, result := call(bob, orgB, http.MethodPost, "/v1/tenant/branches/bulk", map[string]any{
		"action": "deactivate", "target": map[string]any{"scope": "ids", "ids": []string{aUUID}},
	})
	if code != http.StatusOK || result["summary"].(map[string]any)["succeeded"].(float64) != 0 {
		t.Fatalf("bob bulk on A branch = %d %v", code, result)
	}

	// Same code in both organizations; duplicate within one is a 409.
	body := map[string]any{"code": "ist", "name": "İstanbul", "city": "İstanbul"}
	if code, _ := call(alice, orgA, http.MethodPost, "/v1/tenant/branches", body); code != http.StatusCreated {
		t.Fatalf("create IST in A = %d", code)
	}
	if code, _ := call(bob, orgB, http.MethodPost, "/v1/tenant/branches", body); code != http.StatusCreated {
		t.Fatalf("create IST in B = %d", code)
	}
	if code, _ := call(alice, orgA, http.MethodPost, "/v1/tenant/branches", body); code != http.StatusConflict {
		t.Fatalf("duplicate IST in A = %d, want 409", code)
	}

	// Bulk deactivate in A via the query scope, filtered by city.
	code, result = call(alice, orgA, http.MethodPost, "/v1/tenant/branches/bulk", map[string]any{
		"action": "deactivate", "target": map[string]any{"scope": "query", "query": map[string]string{"city": "İstanbul"}},
	})
	if code != http.StatusOK || result["summary"].(map[string]any)["succeeded"].(float64) != 1 {
		t.Fatalf("bulk deactivate in A = %d %v", code, result)
	}
	code, page = call(alice, orgA, http.MethodGet, "/v1/tenant/branches?is_active=false", nil)
	if code != http.StatusOK || page["total"].(float64) != 1 {
		t.Fatalf("inactive A branches = %d %v", code, page)
	}
	code, page = call(bob, orgB, http.MethodGet, "/v1/tenant/branches?is_active=false", nil)
	if code != http.StatusOK || page["total"].(float64) != 0 {
		t.Fatalf("B branches must stay active = %d %v", code, page)
	}

	// Service points and activity of A's default branch.
	code, point := call(alice, orgA, http.MethodPost, "/v1/tenant/branches/"+aUUID+"/service-points", map[string]any{
		"type": "airport", "name": "SAW", "extra_fee": "250",
	})
	if code != http.StatusCreated || point["extra_fee"] != "250.00" {
		t.Fatalf("create point = %d %v", code, point)
	}
	code, activityPage := call(alice, orgA, http.MethodGet, "/v1/tenant/branches/"+aUUID+"/activity", nil)
	if code != http.StatusOK || activityPage["total"].(float64) < 1 {
		t.Fatalf("activity = %d %v", code, activityPage)
	}
	var stamped int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM activity_events WHERE resource = 'tenant.branches' AND organization_id = $1", orgA.ID,
	).Scan(&stamped); err != nil || stamped < 2 {
		t.Fatalf("activity stamped with organization = %d, %v", stamped, err)
	}
	if code, _ := call(bob, orgB, http.MethodGet, "/v1/tenant/branches/"+aUUID+"/activity", nil); code != http.StatusNotFound {
		t.Fatalf("bob reads A activity = %d, want 404", code)
	}
}
