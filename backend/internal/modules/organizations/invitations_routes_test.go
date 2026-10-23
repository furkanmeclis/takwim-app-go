package organizations_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations"
	orghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/handler"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type linkNotifier struct {
	mu   sync.Mutex
	last string
}

func (n *linkNotifier) Enqueue(_ context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.last = in.TemplateVars["link"]
	return nil, nil
}

func (n *linkNotifier) token() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.last[strings.LastIndex(n.last, "/")+1:]
}

// permLoader gives each caller the listed platform permissions only.
type permLoader map[uuid.UUID]struct {
	id    int64
	perms []string
}

func (m permLoader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, _ := uuid.Parse(c.Subject)
	return authctx.Principal{UserID: id, UserInternal: m[id].id, Permissions: m[id].perms}, nil
}

// TestInvitationAndPlatformRoutePermissions checks the route guards of the
// platform organization screens, the tenant invitation / profile routes and
// the public accept flow over HTTP with the real resolver.
func TestInvitationAndPlatformRoutePermissions(t *testing.T) {
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
	var userIDs, orgIDs []int64
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", orgIDs)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", userIDs)
	})
	newUser := func(name string) db.User {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: name + "-invroute-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		userIDs = append(userIDs, u.ID)
		return u
	}
	operator, outsider, owner, member, invitee := newUser("operator"), newUser("outsider"), newUser("owner"), newUser("member"), newUser("invitee")

	repo := orgrepository.NewPostgres(pool, q)
	orgSvc := orgusecase.New(repo, branchusecase.NewOrganizationHooks(nil))
	mgmt := orgusecase.NewManagement(orgSvc, repo, orgusecase.ManagementConfig{FrontendURL: "https://app.example.test"})
	notifier := &linkNotifier{}
	mgmt.SetNotifier(notifier)
	h := orghandler.New(orgSvc, nil)
	h.SetManagement(mgmt)
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	platformPerms := []string{rbac.PermPlatformOrganizationsRead, rbac.PermPlatformOrganizationsWrite}
	loader := permLoader{}
	for _, u := range []db.User{operator, outsider, owner, member, invitee} {
		entry := loader[u.Uuid]
		entry.id = u.ID
		if u.ID == operator.ID {
			entry.perms = platformPerms
		}
		loader[u.Uuid] = entry
	}
	mux := http.NewServeMux()
	organizations.RegisterRoutes(mux, h, organizations.PlatformEngineHandlers{}, tokens, loader)
	organizations.RegisterTenantRoutes(mux, h, tokens, loader, orgSvc)

	call := func(as *db.User, method, path, orgSlug string, body any) (int, map[string]any, string) {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		if as != nil {
			access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: as.Uuid})
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+access)
		}
		if orgSlug != "" {
			req.Header.Set(orgctx.HeaderName, orgSlug)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		var env struct {
			Data  map[string]any `json:"data"`
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		return rr.Code, env.Data, env.Error.Code
	}
	expect := func(step string, want, got int, code string) {
		t.Helper()
		if got != want {
			t.Fatalf("%s = %d (%s), want %d", step, got, code, want)
		}
	}

	// Platform screens need platform permissions.
	code, _, ec := call(&outsider, http.MethodGet, "/v1/platform/organizations", "", nil)
	expect("outsider list", http.StatusForbidden, code, ec)
	code, _, ec = call(nil, http.MethodGet, "/v1/platform/organizations", "", nil)
	expect("anonymous list", http.StatusUnauthorized, code, ec)
	code, data, ec := call(&operator, http.MethodPost, "/v1/platform/organizations", "", map[string]any{
		"name": "İdes Car Rental " + suffix, "owner_user_uuid": owner.Uuid,
	})
	expect("operator create", http.StatusCreated, code, ec)
	created := data["organization"].(map[string]any)
	orgUUID, slug := created["uuid"].(string), created["slug"].(string)
	org, _ := orgSvc.GetOrganization(ctx, uuid.MustParse(orgUUID))
	orgIDs = append(orgIDs, org.ID)
	if created["branch_count"].(float64) != 1 || created["owner"].(map[string]any)["email"] != owner.Email {
		t.Fatalf("created summary = %v", created)
	}
	code, _, ec = call(&outsider, http.MethodPost, "/v1/platform/organizations/"+orgUUID+"/invitations", "", map[string]any{"email": "x@example.test"})
	expect("outsider platform invite", http.StatusForbidden, code, ec)
	code, _, ec = call(&operator, http.MethodPost, "/v1/platform/organizations/"+orgUUID+"/members", "", map[string]any{
		"user_uuid": member.Uuid, "role_key": "member",
	})
	expect("operator add member", http.StatusCreated, code, ec)
	code, _, ec = call(&operator, http.MethodDelete, "/v1/platform/organizations/"+orgUUID+"/members/"+owner.Uuid.String(), "", nil)
	if code != http.StatusConflict || ec != "LAST_ORGANIZATION_OWNER" {
		t.Fatalf("remove last owner = %d %s", code, ec)
	}

	// Tenant invitation routes: membership + tenant.members.write.
	roles, _ := orgSvc.ListTenantRoles(ctx, orgusecase.OperatorScope(org))
	var memberRole string
	for _, r := range roles {
		if r.Key == "member" {
			memberRole = r.UUID.String()
		}
	}
	body := map[string]any{"email": invitee.Email, "role_uuids": []string{memberRole}}
	code, _, ec = call(&owner, http.MethodPost, "/v1/tenant/invitations", "", body)
	expect("missing organization header", http.StatusBadRequest, code, ec)
	code, _, ec = call(&outsider, http.MethodPost, "/v1/tenant/invitations", slug, body)
	expect("non-member invite", http.StatusForbidden, code, ec)
	code, _, ec = call(&member, http.MethodPost, "/v1/tenant/invitations", slug, body)
	expect("member without members.write invite", http.StatusForbidden, code, ec)
	code, _, ec = call(&member, http.MethodGet, "/v1/tenant/invitations", slug, nil)
	expect("member lists invitations", http.StatusOK, code, ec)
	code, data, ec = call(&owner, http.MethodPost, "/v1/tenant/invitations", slug, body)
	expect("owner invite", http.StatusCreated, code, ec)
	if data["status"] != "pending" || data["email"] != invitee.Email {
		t.Fatalf("invitation = %v", data)
	}
	if _, ok := data["token"]; ok {
		t.Fatal("the API must never return the token")
	}
	token := notifier.token()

	// Public preview, signed-in accept.
	code, data, ec = call(nil, http.MethodGet, "/v1/invitations/"+token, "", nil)
	expect("public preview", http.StatusOK, code, ec)
	if data["email_masked"] == invitee.Email || data["organization"].(map[string]any)["slug"] != slug {
		t.Fatalf("preview = %v", data)
	}
	code, _, ec = call(nil, http.MethodGet, "/v1/invitations/unknown-token", "", nil)
	expect("unknown preview", http.StatusNotFound, code, ec)
	code, _, ec = call(nil, http.MethodPost, "/v1/invitations/"+token+"/accept", "", nil)
	expect("anonymous accept", http.StatusUnauthorized, code, ec)
	code, _, ec = call(&outsider, http.MethodPost, "/v1/invitations/"+token+"/accept", "", nil)
	if code != http.StatusForbidden || ec != "INVITATION_EMAIL_MISMATCH" {
		t.Fatalf("mismatch accept = %d %s", code, ec)
	}
	code, data, ec = call(&invitee, http.MethodPost, "/v1/invitations/"+token+"/accept", "", nil)
	expect("invitee accept", http.StatusOK, code, ec)
	if data["organization_slug"] != slug {
		t.Fatalf("accept = %v", data)
	}
	code, _, ec = call(&invitee, http.MethodPost, "/v1/invitations/"+token+"/accept", "", nil)
	if code != http.StatusGone || ec != "INVITATION_USED" {
		t.Fatalf("reuse = %d %s", code, ec)
	}
	code, _, ec = call(nil, http.MethodPost, "/v1/invitations/"+token+"/register", "", map[string]any{
		"name": "A", "surname": "B", "password": "long-enough-1",
	})
	if code != http.StatusForbidden && code != http.StatusGone {
		t.Fatalf("register on used invitation = %d %s", code, ec)
	}
	code, _, ec = call(&invitee, http.MethodGet, "/v1/tenant/members", slug, nil)
	expect("new member reads members", http.StatusOK, code, ec)

	// Organization profile: settings.read to view, settings.write to change.
	code, data, ec = call(&member, http.MethodGet, "/v1/tenant/organization", slug, nil)
	expect("member reads profile", http.StatusOK, code, ec)
	if data["name"] != org.Name {
		t.Fatalf("profile = %v", data)
	}
	code, _, ec = call(&member, http.MethodPatch, "/v1/tenant/organization", slug, map[string]any{"name": "Yeni Ad"})
	expect("member renames", http.StatusForbidden, code, ec)
	code, data, ec = call(&owner, http.MethodPatch, "/v1/tenant/organization", slug, map[string]any{"name": "İdes Rent " + suffix})
	expect("owner renames", http.StatusOK, code, ec)
	if data["name"] != "İdes Rent "+suffix || data["slug"] != slug {
		t.Fatalf("renamed = %v", data)
	}
	code, _, ec = call(&owner, http.MethodPatch, "/v1/tenant/organization", slug, map[string]any{"slug": "other"})
	expect("owner cannot change slug", http.StatusBadRequest, code, ec)

	// Platform owner invitation through the create screen.
	code, data, ec = call(&operator, http.MethodPost, "/v1/platform/organizations", "", map[string]any{
		"name": "Davetli Org " + suffix, "owner_email": "new-owner-" + suffix + "@example.test",
	})
	expect("create with owner invitation", http.StatusCreated, code, ec)
	invited := data["organization"].(map[string]any)
	invitedOrg, _ := orgSvc.GetOrganization(ctx, uuid.MustParse(invited["uuid"].(string)))
	orgIDs = append(orgIDs, invitedOrg.ID)
	if invited["member_count"].(float64) != 0 || data["owner_invitation"].(map[string]any)["status"] != "pending" {
		t.Fatalf("create with invitation = %v", data)
	}
}
