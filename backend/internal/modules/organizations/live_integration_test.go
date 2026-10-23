package organizations_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations"
	orghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/handler"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	tenantmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tenant"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeCentrifugo struct {
	mu   sync.Mutex
	pubs []string
}

// Publish records "channel resource action uuids…" lines.
func (f *fakeCentrifugo) Publish(_ context.Context, channel string, data any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	ev, _ := data.(resourceevents.Event)
	f.pubs = append(f.pubs, strings.TrimSpace(strings.Join(append([]string{channel, ev.Resource, ev.Action}, ev.UUIDs...), " ")))
	return nil
}

func (f *fakeCentrifugo) take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.pubs
	f.pubs = nil
	sort.Strings(out)
	return out
}

// operatorLoader gives every caller the platform organization permissions;
// tenant routes ignore them and use the membership.
type operatorLoader map[uuid.UUID]int64

func (m operatorLoader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, _ := uuid.Parse(c.Subject)
	return authctx.Principal{
		UserID: id, UserInternal: m[id],
		Permissions: []string{rbac.PermPlatformOrganizationsRead, rbac.PermPlatformOrganizationsWrite},
	}, nil
}

// TestTenantMembersRolesLiveEvents checks that member and role changes (from
// the tenant shell and from the platform operator) publish on the
// organization's tenant.members / tenant.roles channels, and that only
// members with the read permission may subscribe. Runs with TEST_DATABASE_URL.
func TestTenantMembersRolesLiveEvents(t *testing.T) {
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
			Email: name + "-orglive-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	alice, bob := newUser("alice"), newUser("bob")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Org live " + suffix, OwnerUserID: alice.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{alice.ID, bob.ID})
	})

	fake := &fakeCentrifugo{}
	events := resourceevents.New(fake, nil)
	events.AddResolver(tenantmodule.ChannelResolver(tenantmodule.QueriesLookup(q)))
	h := orghandler.New(orgSvc, nil)
	h.SetResourceEvents(events)
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	loader := operatorLoader{alice.Uuid: alice.ID, bob.Uuid: bob.ID}
	mux := http.NewServeMux()
	organizations.RegisterRoutes(mux, h, organizations.PlatformEngineHandlers{}, tokens, loader)
	organizations.RegisterTenantRoutes(mux, h, tokens, loader, orgSvc)

	call := func(method, path string, tenant bool, body any) (int, map[string]any) {
		t.Helper()
		access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: alice.Uuid})
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Authorization", "Bearer "+access)
		if tenant {
			req.Header.Set(orgctx.HeaderName, org.Slug)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code >= 300 {
			t.Logf("%s %s = %d %s", method, path, rr.Code, rr.Body.String())
		}
		var env map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		data, _ := env["data"].(map[string]any)
		return rr.Code, data
	}
	must := func(want, got int, step string) {
		t.Helper()
		if got != want {
			t.Fatalf("%s = %d, want %d", step, got, want)
		}
	}
	members := orghandler.ResourceTenantMembers
	roles := orghandler.ResourceTenantRoles
	channel := func(resource string) string { return tenantmodule.ResourceChannel(org.UUID, resource) }
	pub := func(resource, action string, uuids ...string) string {
		return strings.TrimSpace(strings.Join(append([]string{channel(resource), resource, action}, uuids...), " "))
	}
	expect := func(step string, want ...string) {
		t.Helper()
		events.Wait()
		got := fake.take()
		sort.Strings(want)
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("%s:\n got  %q\n want %q", step, got, want)
		}
	}
	bobID := bob.Uuid.String()

	// A platform operator adds a member: the tenant lists refresh too.
	code, _ := call(http.MethodPost, "/v1/platform/organizations/"+org.UUID.String()+"/members", false, map[string]any{
		"user_uuid": bobID, "role_key": orgctx.RoleMember,
	})
	must(http.StatusCreated, code, "platform add member")
	platformList := strings.Join([]string{
		resourceevents.DefaultChannel(orghandler.ResourcePlatformOrganizations), orghandler.ResourcePlatformOrganizations,
		resourceevents.ActionUpdated, org.UUID.String(),
	}, " ")
	expect("platform add member", pub(members, resourceevents.ActionCreated, bobID), pub(roles, resourceevents.ActionBulk), platformList)

	code, _ = call(http.MethodPatch, "/v1/tenant/members/"+bobID, true, map[string]any{"branch_scope": "all"})
	must(http.StatusOK, code, "patch member")
	expect("patch member", pub(members, resourceevents.ActionUpdated, bobID), pub(roles, resourceevents.ActionBulk))

	code, role := call(http.MethodPost, "/v1/tenant/roles", true, map[string]any{
		"key": "live_viewer", "name": "Live viewer", "permissions": []string{rbac.PermTenantMembersRead},
	})
	must(http.StatusCreated, code, "create role")
	roleID := role["uuid"].(string)
	expect("create role", pub(roles, resourceevents.ActionCreated, roleID))

	code, _ = call(http.MethodPatch, "/v1/tenant/roles/"+roleID, true, map[string]any{"name": "Live viewers"})
	must(http.StatusOK, code, "patch role")
	expect("patch role", pub(roles, resourceevents.ActionUpdated, roleID), pub(members, resourceevents.ActionBulk))

	code, _ = call(http.MethodDelete, "/v1/tenant/roles/"+roleID, true, nil)
	must(http.StatusOK, code, "delete role")
	expect("delete role", pub(roles, resourceevents.ActionDeleted, roleID))

	code, _ = call(http.MethodDelete, "/v1/tenant/members/"+bobID, true, nil)
	must(http.StatusOK, code, "remove member")
	expect("remove member", pub(members, resourceevents.ActionDeleted, bobID), pub(roles, resourceevents.ActionBulk))

	// Subscription authorization.
	reg := resourceevents.NewRegistry()
	organizations.RegisterLiveResources(reg)
	rule := tenantmodule.ChannelRule(reg, orgSvc)
	allowed := func(user db.User, ch string) bool {
		t.Helper()
		pctx := authctx.WithPrincipal(ctx, authctx.Principal{UserID: user.Uuid, UserInternal: user.ID})
		handled, ok, err := rule(pctx, user.Uuid, false, ch)
		if err != nil || !handled {
			t.Fatalf("rule(%s): handled=%v err=%v", ch, handled, err)
		}
		return ok
	}
	for _, resource := range []string{members, roles} {
		if !allowed(alice, channel(resource)) {
			t.Fatalf("owner denied %s", resource)
		}
		if allowed(bob, channel(resource)) {
			t.Fatalf("former member allowed %s", resource)
		}
	}
}
