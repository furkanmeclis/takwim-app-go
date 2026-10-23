package activity_test

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
	activitymodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/activity"
	activityhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/activity/handler"
	activityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/activity/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches"
	branchhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/handler"
	branchrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/repository"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations"
	orghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/handler"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
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

// TestTenantActivityHistory drives the organization's activity history
// (TEC-893) over HTTP with the real resolver and SQL: branch events carry
// their branch and the before / after of the edit, the list follows the
// member's branch scope and the header branch picker, and staff without
// tenant.activity.read get 403.
// Runs when TEST_DATABASE_URL points at a migrated database.
func TestTenantActivityHistory(t *testing.T) {
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
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Activity " + suffix, OwnerUserID: owner.ID})
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
	mux := http.NewServeMux()
	branches.RegisterRoutes(mux, branchhandler.New(branchusecase.New(branchrepository.NewPostgres(q), nil), rec), branches.EngineHandlers{}, tokens, loader, orgSvc)
	organizations.RegisterTenantRoutes(mux, orghandler.New(orgSvc, rec), tokens, loader, orgSvc)
	activitymodule.RegisterTenantRoutes(mux, activityhandler.New(activityusecase.New(q)), tokens, loader, orgSvc)

	call := func(user db.User, method, path, branch string, body any) (int, map[string]any) {
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
		if branch != "" {
			req.Header.Set(orgctx.BranchHeaderName, branch)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		var env map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		data, _ := env["data"].(map[string]any)
		return rr.Code, data
	}
	items := func(page map[string]any) []map[string]any {
		out := []map[string]any{}
		for _, raw := range page["items"].([]any) {
			out = append(out, raw.(map[string]any))
		}
		return out
	}

	// Branch A is the default "Merkez"; the owner adds B, renames it, and
	// narrows carol to A (an event of the whole business).
	_, page := call(owner, http.MethodGet, "/v1/tenant/branches", "", nil)
	aUUID := page["items"].([]any)[0].(map[string]any)["uuid"].(string)
	code, created := call(owner, http.MethodPost, "/v1/tenant/branches", "", map[string]any{"code": "KDK", "name": "Kadıköy"})
	if code != http.StatusCreated {
		t.Fatalf("create B = %d", code)
	}
	bUUID := created["uuid"].(string)
	if code, _ := call(owner, http.MethodPatch, "/v1/tenant/branches/"+bUUID, "", map[string]any{"name": "Kadıköy Moda"}); code != http.StatusOK {
		t.Fatalf("rename B = %d", code)
	}
	if code, _ := call(owner, http.MethodPatch, "/v1/tenant/branches/"+aUUID, "", map[string]any{"phone": "+90 212 000 00 00"}); code != http.StatusOK {
		t.Fatalf("edit A = %d", code)
	}
	if code, _ := call(owner, http.MethodPatch, "/v1/tenant/members/"+carol.Uuid.String(), "", map[string]any{
		"branch_scope": "selected", "branch_uuids": []string{aUUID},
	}); code != http.StatusOK {
		t.Fatalf("narrow carol = %d", code)
	}

	// Owner: every event, newest first; the rename carries branch B and
	// the before / after of the name.
	code, page = call(owner, http.MethodGet, "/v1/tenant/activity", "", nil)
	all := items(page)
	if code != http.StatusOK || page["total"].(float64) != 4 || all[0]["action"] != "members.updated" {
		t.Fatalf("owner list = %d %v", code, page)
	}
	if all[0]["branch"] != nil {
		t.Fatalf("member event must belong to the whole business: %v", all[0])
	}
	changes := all[0]["payload"].(map[string]any)["changes"].(map[string]any)
	if changes["branch_scope"].(map[string]any)["after"] != "selected" {
		t.Fatalf("member changes = %v", changes)
	}
	code, page = call(owner, http.MethodGet, "/v1/tenant/activity?action=branches.updated&resource_uuid="+bUUID, "", nil)
	rename := items(page)
	if code != http.StatusOK || len(rename) != 1 || rename[0]["branch"].(map[string]any)["uuid"] != bUUID {
		t.Fatalf("rename event = %d %v", code, page)
	}
	name := rename[0]["payload"].(map[string]any)["changes"].(map[string]any)["name"].(map[string]any)
	if name["before"] != "Kadıköy" || name["after"] != "Kadıköy Moda" || rename[0]["actor"].(map[string]any)["email"] != owner.Email {
		t.Fatalf("rename changes = %v", rename[0])
	}

	// The picker narrows to branch B; carol only sees branch A events.
	code, page = call(owner, http.MethodGet, "/v1/tenant/activity", bUUID, nil)
	if code != http.StatusOK || page["total"].(float64) != 2 {
		t.Fatalf("owner list B = %d %v", code, page)
	}
	code, page = call(carol, http.MethodGet, "/v1/tenant/activity", "", nil)
	if code != http.StatusOK || page["total"].(float64) != 1 || items(page)[0]["branch"].(map[string]any)["uuid"] != aUUID {
		t.Fatalf("carol list = %d %v", code, page)
	}
	code, facets := call(carol, http.MethodGet, "/v1/tenant/activity/facets", "", nil)
	if code != http.StatusOK || len(facets["resources"].([]any)) != 1 {
		t.Fatalf("carol facets = %d %v", code, facets)
	}

	// Reception staff has no tenant.activity.read.
	if code, _ := call(dave, http.MethodGet, "/v1/tenant/activity", "", nil); code != http.StatusForbidden {
		t.Fatalf("dave list = %d, want 403", code)
	}
	// Bad filters answer 400.
	if code, _ := call(owner, http.MethodGet, "/v1/tenant/activity?from=yesterday", "", nil); code != http.StatusBadRequest {
		t.Fatalf("bad from = %d", code)
	}
}
