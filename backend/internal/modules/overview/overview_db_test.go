package overview_test

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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/overview"
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

// TestOverviewAndBranchPicker drives the central panel and the X-Branch
// header (TEC-892) over HTTP with the real resolver and SQL: the overview
// shows branches one by one and in total, the header narrows lists to one
// branch inside the member's scope and is refused outside it.
// Runs when TEST_DATABASE_URL points at a migrated database.
func TestOverviewAndBranchPicker(t *testing.T) {
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
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Overview " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{owner.ID, carol.ID, dave.ID})
	})
	for _, m := range []struct {
		u    db.User
		role string
	}{{carol, "sube_yoneticisi"}, {dave, "member"}} {
		if _, err := orgSvc.AddMember(ctx, org.UUID, m.u.Uuid, m.role); err != nil {
			t.Fatalf("add %s: %v", m.u.Name, err)
		}
	}

	rec := activity.NewRecorder(q, nil)
	branchSvc := branchusecase.New(branchrepository.NewPostgres(q), nil)
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	loader := mapLoader{owner.Uuid: owner.ID, carol.Uuid: carol.ID, dave.Uuid: dave.ID}
	mux := http.NewServeMux()
	branches.RegisterRoutes(mux, branchhandler.New(branchSvc, rec), branches.EngineHandlers{}, tokens, loader, orgSvc)
	organizations.RegisterTenantRoutes(mux, orghandler.New(orgSvc, rec), tokens, loader, orgSvc)
	overview.RegisterRoutes(mux, overview.NewHandler(overview.New(branchSvc, q)), tokens, loader, orgSvc)

	call := func(user db.User, method, path, branch string, body any) (int, map[string]any, string) {
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
		code := ""
		if e, ok := env["error"].(map[string]any); ok {
			code, _ = e["code"].(string)
		}
		return rr.Code, data, code
	}

	// Branch A is the default "Merkez"; the owner adds B. Carol works in A,
	// dave in B; the owner sees every branch.
	_, page, _ := call(owner, http.MethodGet, "/v1/tenant/branches", "", nil)
	aUUID := page["items"].([]any)[0].(map[string]any)["uuid"].(string)
	code, branchB, _ := call(owner, http.MethodPost, "/v1/tenant/branches", "", map[string]any{"code": "KDK", "name": "Kadıköy", "city": "İstanbul"})
	if code != http.StatusCreated {
		t.Fatalf("create B = %d", code)
	}
	bUUID := branchB["uuid"].(string)
	for user, branch := range map[uuid.UUID]string{carol.Uuid: aUUID, dave.Uuid: bUUID} {
		if code, _, _ := call(owner, http.MethodPatch, "/v1/tenant/members/"+user.String(), "", map[string]any{
			"branch_scope": "selected", "branch_uuids": []string{branch},
		}); code != http.StatusOK {
			t.Fatalf("narrow %s = %d", user, code)
		}
	}

	memberCounts := func(ov map[string]any) map[string]float64 {
		out := map[string]float64{}
		for _, raw := range ov["branches"].([]any) {
			b := raw.(map[string]any)
			out[b["uuid"].(string)] = b["member_count"].(float64)
		}
		return out
	}

	// Owner, all branches: one by one and in total (each member once).
	code, ov, _ := call(owner, http.MethodGet, "/v1/tenant/overview", "", nil)
	totals := ov["totals"].(map[string]any)
	if code != http.StatusOK || totals["branches"].(float64) != 2 || totals["members"].(float64) != 3 {
		t.Fatalf("owner overview = %d %v", code, ov)
	}
	if counts := memberCounts(ov); counts[aUUID] != 2 || counts[bUUID] != 2 {
		t.Fatalf("member counts = %v", counts)
	}

	// Owner picks B: overview, branch list and members narrow to B.
	code, ov, _ = call(owner, http.MethodGet, "/v1/tenant/overview", bUUID, nil)
	totals = ov["totals"].(map[string]any)
	if code != http.StatusOK || totals["branches"].(float64) != 1 || totals["members"].(float64) != 2 {
		t.Fatalf("owner overview B = %d %v", code, ov)
	}
	if counts := memberCounts(ov); len(counts) != 1 || counts[bUUID] != 2 {
		t.Fatalf("member counts B = %v", counts)
	}
	code, page, _ = call(owner, http.MethodGet, "/v1/tenant/branches", bUUID, nil)
	if code != http.StatusOK || page["total"].(float64) != 1 || page["items"].([]any)[0].(map[string]any)["uuid"] != bUUID {
		t.Fatalf("owner list B = %d %v", code, page)
	}
	code, page, _ = call(owner, http.MethodGet, "/v1/tenant/members", aUUID, nil)
	if code != http.StatusOK || page["total"].(float64) != 2 {
		t.Fatalf("owner members A = %d %v", code, page)
	}
	// The picker filters lists only: a record of another branch still opens.
	if code, _, _ := call(owner, http.MethodGet, "/v1/tenant/branches/"+aUUID, bUUID, nil); code != http.StatusOK {
		t.Fatalf("owner get A while B picked = %d", code)
	}

	// Carol sees only A, and cannot pick B or an unknown branch.
	code, ov, _ = call(carol, http.MethodGet, "/v1/tenant/overview", "", nil)
	if counts := memberCounts(ov); code != http.StatusOK || len(counts) != 1 || counts[aUUID] != 2 {
		t.Fatalf("carol overview = %d %v", code, ov)
	}
	for _, branch := range []string{bUUID, uuid.NewString(), "not-a-uuid"} {
		if code, _, errCode := call(carol, http.MethodGet, "/v1/tenant/branches", branch, nil); code != http.StatusForbidden || errCode != "BRANCH_NOT_ALLOWED" {
			t.Fatalf("carol picks %s = %d %s", branch, code, errCode)
		}
	}
	if code, _, _ := call(carol, http.MethodGet, "/v1/tenant/overview", aUUID, nil); code != http.StatusOK {
		t.Fatalf("carol picks A = %d", code)
	}

	// A deleted branch stops matching at once.
	if code, _, _ := call(owner, http.MethodDelete, "/v1/tenant/branches/"+bUUID, "", nil); code != http.StatusNoContent && code != http.StatusOK {
		t.Fatalf("delete B = %d", code)
	}
	if code, _, errCode := call(owner, http.MethodGet, "/v1/tenant/overview", bUUID, nil); code != http.StatusForbidden || errCode != "BRANCH_NOT_ALLOWED" {
		t.Fatalf("owner picks deleted B = %d %s", code, errCode)
	}
}
