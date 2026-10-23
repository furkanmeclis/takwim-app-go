package branches_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations"
	orghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/handler"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestBranchScopeRTA02 is acceptance RT-A02 (TEC-689): a member whose branch
// scope is "selected [A]" gets 404 for branch B over the API and does not see
// B in the list, facets, export (request and worker), bulk or search.
// Runs when TEST_DATABASE_URL points at a migrated database.
func TestBranchScopeRTA02(t *testing.T) {
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
	owner, carol := newUser("owner"), newUser("carol")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Scope " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{owner.ID, carol.ID})
	})
	if _, err := orgSvc.AddMember(ctx, org.UUID, carol.Uuid, "sube_yoneticisi"); err != nil {
		t.Fatalf("add carol: %v", err)
	}

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
	}, tokens, mapLoader{owner.Uuid: owner.ID, carol.Uuid: carol.ID}, orgSvc)
	organizations.RegisterTenantRoutes(mux, orghandler.New(orgSvc, rec), tokens, mapLoader{owner.Uuid: owner.ID, carol.Uuid: carol.ID}, orgSvc)

	call := func(user db.User, method, path string, body any) (int, map[string]any) {
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

	// Branch A is the default "Merkez"; the owner adds branch B.
	_, page := call(owner, http.MethodGet, "/v1/tenant/branches", nil)
	branchA := page["items"].([]any)[0].(map[string]any)
	aUUID := branchA["uuid"].(string)
	code, branchB := call(owner, http.MethodPost, "/v1/tenant/branches", map[string]any{"code": "IST", "name": "İstanbul", "city": "İstanbul"})
	if code != http.StatusCreated {
		t.Fatalf("create B = %d", code)
	}
	bUUID := branchB["uuid"].(string)

	// The owner narrows carol to [A] through the members API.
	code, member := call(owner, http.MethodPatch, "/v1/tenant/members/"+carol.Uuid.String(), map[string]any{
		"branch_scope": "selected", "branch_uuids": []string{aUUID},
	})
	if code != http.StatusOK || member["branch_scope"] != "selected" || len(member["branches"].([]any)) != 1 {
		t.Fatalf("patch carol = %d %v", code, member)
	}

	// List / count / facets only show A.
	code, page = call(carol, http.MethodGet, "/v1/tenant/branches", nil)
	if code != http.StatusOK || page["total"].(float64) != 1 || page["items"].([]any)[0].(map[string]any)["uuid"] != aUUID {
		t.Fatalf("carol list = %d %v", code, page)
	}
	code, page = call(carol, http.MethodGet, "/v1/tenant/branches?q=IST", nil)
	if code != http.StatusOK || page["total"].(float64) != 0 {
		t.Fatalf("carol search IST = %d %v", code, page)
	}
	_, facets := call(carol, http.MethodGet, "/v1/tenant/branches/facets", nil)
	if cities := facets["city"].([]any); len(cities) != 0 {
		t.Fatalf("carol facets leak B's city: %v", cities)
	}

	// Branch B answers 404 for every route; A works.
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/v1/tenant/branches/" + bUUID},
		{http.MethodPatch, "/v1/tenant/branches/" + bUUID},
		{http.MethodDelete, "/v1/tenant/branches/" + bUUID},
		{http.MethodGet, "/v1/tenant/branches/" + bUUID + "/activity"},
		{http.MethodGet, "/v1/tenant/branches/" + bUUID + "/service-points"},
		{http.MethodPost, "/v1/tenant/branches/" + bUUID + "/service-points"},
	} {
		var body any
		if tc.method == http.MethodPatch {
			body = map[string]any{"name": "x"}
		}
		if tc.method == http.MethodPost {
			body = map[string]any{"type": "office", "name": "x"}
		}
		if code, _ := call(carol, tc.method, tc.path, body); code != http.StatusNotFound {
			t.Fatalf("carol %s %s = %d, want 404", tc.method, tc.path, code)
		}
	}
	if code, _ := call(carol, http.MethodGet, "/v1/tenant/branches/"+aUUID, nil); code != http.StatusOK {
		t.Fatalf("carol reads A = %d", code)
	}
	// Creating a branch needs every branch.
	if code, _ := call(carol, http.MethodPost, "/v1/tenant/branches", map[string]any{"code": "ANK", "name": "Ankara"}); code != http.StatusForbidden {
		t.Fatalf("carol creates a branch = %d, want 403", code)
	}

	// Bulk: B by id fails, "all matching" only reaches A.
	code, result := call(carol, http.MethodPost, "/v1/tenant/branches/bulk", map[string]any{
		"action": "deactivate", "target": map[string]any{"scope": "ids", "ids": []string{bUUID}},
	})
	if code != http.StatusOK || result["summary"].(map[string]any)["succeeded"].(float64) != 0 {
		t.Fatalf("carol bulk on B = %d %v", code, result)
	}
	code, result = call(carol, http.MethodPost, "/v1/tenant/branches/bulk", map[string]any{
		"action": "deactivate", "target": map[string]any{"scope": "query", "query": map[string]string{}},
	})
	if code != http.StatusOK || result["summary"].(map[string]any)["total"].(float64) != 1 {
		t.Fatalf("carol bulk all matching = %d %v", code, result)
	}
	if code, b := call(owner, http.MethodGet, "/v1/tenant/branches/"+bUUID, nil); code != http.StatusOK || b["is_active"] != true {
		t.Fatalf("B must stay active = %d %v", code, b)
	}

	// Export in the request scope and in a worker job of carol.
	carolScope, err := orgSvc.Resolve(ctx, org.Slug, carol.ID)
	if err != nil {
		t.Fatal(err)
	}
	io := adapters.NewIO(svc)
	assertExportOnlyA := func(label string, ctx context.Context) {
		t.Helper()
		ds, err := io.Export(ctx, map[string]string{}, i18n.Normalize("tr"))
		if err != nil {
			t.Fatalf("%s export: %v", label, err)
		}
		if len(ds.Rows) != 1 || ds.Rows[0]["uuid"] != aUUID {
			t.Fatalf("%s export rows = %v", label, ds.Rows)
		}
	}
	assertExportOnlyA("request", orgctx.WithScope(ctx, carolScope))
	jobCtx, err := orgctx.WithJobScope(context.Background(), orgSvc, org.ID, carol.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertExportOnlyA("worker", jobCtx)

	// Search: B's document carries its branch and carol's filter excludes it.
	search := adapters.NewSearch(q)
	var bID int64
	if err := pool.QueryRow(ctx, "SELECT id FROM branches WHERE uuid = $1", bUUID).Scan(&bID); err != nil {
		t.Fatal(err)
	}
	doc, err := search.Document(ctx, searchengine.TenantDocumentID(org.ID, bUUID))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.BranchIDs) != 1 || doc.BranchIDs[0] != bID {
		t.Fatalf("B document branch ids = %v", doc.BranchIDs)
	}
	filter, ok := searchengine.TenantFilter(search.Spec(), org.ID, carolScope.BranchIDs)
	if !ok || !strings.Contains(filter, "branch_ids IN [") || strings.Contains(filter, strconv.FormatInt(bID, 10)+"]") {
		t.Fatalf("carol search filter = %q", filter)
	}
	if filter, _ := searchengine.TenantFilter(search.Spec(), org.ID, nil); strings.Contains(filter, "branch_ids") {
		t.Fatalf("all-branch filter = %q", filter)
	}

	// Back to every branch: B becomes visible again (resolver re-reads).
	if code, _ := call(owner, http.MethodPatch, "/v1/tenant/members/"+carol.Uuid.String(), map[string]any{"branch_scope": "all"}); code != http.StatusOK {
		t.Fatalf("reset carol = %d", code)
	}
	if code, page := call(carol, http.MethodGet, "/v1/tenant/branches", nil); code != http.StatusOK || page["total"].(float64) != 2 {
		t.Fatalf("carol list after reset = %d %v", code, page)
	}
	if code, _ := call(carol, http.MethodGet, "/v1/tenant/branches/"+bUUID, nil); code != http.StatusOK {
		t.Fatalf("carol reads B after reset = %d", code)
	}
}
