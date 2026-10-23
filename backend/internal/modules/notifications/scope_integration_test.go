package notifications_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/adapters"
	branchrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/repository"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	exportusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/exports/usecase"
	notifmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications"
	notifhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/handler"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/providers"
	notifusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// memDriver serves the export upload from the in-memory store.
type memDriver struct {
	storage.Driver
	m *storage.Memory
}

func (d memDriver) Upload(ctx context.Context, f storage.File, path string) error {
	return d.m.Upload(ctx, f, path)
}

func (d memDriver) Download(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	return d.m.Download(ctx, path)
}

func (d memDriver) Exists(ctx context.Context, path string) (bool, error) {
	return d.m.Exists(ctx, path)
}

type readerLoader map[uuid.UUID]int64

func (m readerLoader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, _ := uuid.Parse(c.Subject)
	return authctx.Principal{
		UserID: id, UserInternal: m[id], Permissions: []string{rbac.PermNotificationsRead},
	}, nil
}

// TestNotificationOrganizationScope checks the TEC-748 inbox scope: a user
// who belongs to organizations A and B sees A's job notifications only in A's
// panel, B's only in B's, and neither in the platform shell, where only
// un-stamped notifications appear. A tenant export stamps its organization
// and links to the tenant download page. Runs with TEST_DATABASE_URL.
func TestNotificationOrganizationScope(t *testing.T) {
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
			Email: name + "-notif-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
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
	orgA, orgB := newOrg("Notif A", alice), newOrg("Notif B", bob)
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM notifications WHERE user_id = ANY($1)", []int64{alice.ID, bob.ID})
		_, _ = pool.Exec(bg, "DELETE FROM export_jobs WHERE actor_id = ANY($1)", []int64{alice.ID, bob.ID})
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{orgA.ID, orgB.ID})
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{alice.ID, bob.ID})
	})
	// Alice belongs to both organizations.
	if _, err := orgSvc.AddMember(ctx, orgB.UUID, alice.Uuid, orgctx.RoleMember); err != nil {
		t.Fatal(err)
	}

	svc := notifusecase.New(q, nil, []providers.Provider{providers.InappProvider{}}, nil)
	enqueue := func(title string, orgID *int64) {
		t.Helper()
		uid := alice.ID
		if _, err := svc.Enqueue(ctx, notifmodel.EnqueueInput{
			UserID: &uid, OrganizationID: orgID, Channels: []string{notifmodel.ChannelInapp},
			Title: title, Body: title, SourceEvent: "test",
		}); err != nil {
			t.Fatal(err)
		}
	}
	enqueue("platform", nil)
	enqueue("org-b", &orgB.ID)

	// A tenant export of organization A stamps A and links to the tenant
	// download page.
	branchSvc := branchusecase.New(branchrepository.NewPostgres(q), nil)
	exports := exportusecase.New(q, memDriver{m: storage.NewMemory()}, ioengine.NewRegistry(branchadapters.NewIO(branchSvc)), nil, svc, nil, nil)
	job, err := exports.RequestExport(orgctx.WithOrganizationID(ctx, orgA.ID), alice.ID, branchadapters.Resource, ioengine.ExportCSV, ioengine.ExportQuery{}, "en")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "completed" {
		t.Fatalf("export status = %s", job.Status)
	}

	titles := func(orgID *int64) []string {
		t.Helper()
		page, err := svc.ListInbox(ctx, alice.ID, orgID, apiquery.Query{Limit: 50}, "", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, 0, len(page.Items))
		for _, n := range page.Items {
			out = append(out, n.Title)
			if n.SourceEvent != nil && *n.SourceEvent == "exports.ready" {
				want := exportusecase.TenantDownloadPath(orgA.Slug, job.UUID)
				if n.ActionURL == nil || *n.ActionURL != want {
					t.Fatalf("export action url = %v, want %s", n.ActionURL, want)
				}
				if n.OrganizationUUID == nil || *n.OrganizationUUID != orgA.UUID {
					t.Fatalf("export organization = %v, want %s", n.OrganizationUUID, orgA.UUID)
				}
			}
		}
		sort.Strings(out)
		if int64(len(out)) != page.Total {
			t.Fatalf("total %d != items %d", page.Total, len(out))
		}
		return out
	}
	exportTitle := func() string {
		t.Helper()
		var title string
		if err := pool.QueryRow(ctx,
			"SELECT title FROM notifications WHERE user_id = $1 AND organization_id = $2 AND source_event = 'exports.ready'",
			alice.ID, orgA.ID).Scan(&title); err != nil {
			t.Fatalf("stamped export notification: %v", err)
		}
		return title
	}()
	assertTitles := func(step string, got []string, want ...string) {
		t.Helper()
		sort.Strings(want)
		if len(got) != len(want) {
			t.Fatalf("%s: titles = %v, want %v", step, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("%s: titles = %v, want %v", step, got, want)
			}
		}
	}
	assertTitles("org A", titles(&orgA.ID), exportTitle, "platform")
	assertTitles("org B", titles(&orgB.ID), "org-b", "platform")
	assertTitles("platform", titles(nil), "platform")

	unread := func(orgID *int64) int64 {
		t.Helper()
		n, err := svc.UnreadCount(ctx, alice.ID, orgID)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if a, b, p := unread(&orgA.ID), unread(&orgB.ID), unread(nil); a != 2 || b != 2 || p != 1 {
		t.Fatalf("unread A=%d B=%d platform=%d, want 2 2 1", a, b, p)
	}

	// The platform notification log (operators, own rows without read_all)
	// lists platform rows only: organization-stamped rows belong to their
	// organization's panel.
	for _, actor := range []authctx.Principal{
		{UserID: alice.Uuid, UserInternal: alice.ID},
		{UserID: alice.Uuid, UserInternal: alice.ID, Permissions: []string{rbac.PermPlatformNotificationsReadAll}},
	} {
		page, err := svc.ListPlatform(ctx, actor, apiquery.Query{Limit: 100}, "", "", "", alice.Uuid.String())
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range page.Items {
			if n.OrganizationUUID != nil {
				t.Fatalf("platform log lists an organization notification: %q (%v)", n.Title, *n.OrganizationUUID)
			}
		}
		if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Title != "platform" {
			t.Fatalf("platform log = %d items, total %d, want only \"platform\"", len(page.Items), page.Total)
		}
	}

	// HTTP: /v1/tenant/notifications with X-Organization, /v1/notifications
	// without.
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h := notifhandler.New(svc)
	loader := readerLoader{alice.Uuid: alice.ID, bob.Uuid: bob.ID}
	notifmodule.RegisterRoutes(mux, h, tokens, loader)
	notifmodule.RegisterTenantRoutes(mux, h, tokens, loader, orgSvc)
	call := func(user db.User, org, method, path string) (int, map[string]any) {
		t.Helper()
		access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: user.Uuid})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+access)
		if org != "" {
			req.Header.Set(orgctx.HeaderName, org)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		var env map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		data, _ := env["data"].(map[string]any)
		return rr.Code, data
	}
	total := func(org, path string) float64 {
		t.Helper()
		code, data := call(alice, org, http.MethodGet, path)
		if code != http.StatusOK {
			t.Fatalf("GET %s (%s) = %d", path, org, code)
		}
		if v, ok := data["total"].(float64); ok {
			return v
		}
		return data["count"].(float64)
	}
	if got := total(orgA.Slug, "/v1/tenant/notifications"); got != 2 {
		t.Fatalf("tenant A list total = %v, want 2", got)
	}
	if got := total(orgB.Slug, "/v1/tenant/notifications"); got != 2 {
		t.Fatalf("tenant B list total = %v, want 2", got)
	}
	if got := total("", "/v1/notifications"); got != 1 {
		t.Fatalf("platform list total = %v, want 1", got)
	}
	// The organization header does nothing on the platform routes.
	if got := total(orgA.Slug, "/v1/notifications"); got != 1 {
		t.Fatalf("platform list with header total = %v, want 1", got)
	}
	if got := total(orgA.Slug, "/v1/tenant/notifications/unread-count"); got != 2 {
		t.Fatalf("tenant A unread = %v, want 2", got)
	}
	if code, _ := call(bob, orgA.Slug, http.MethodGet, "/v1/tenant/notifications"); code != http.StatusForbidden {
		t.Fatalf("non-member list = %d, want 403", code)
	}
	if code, _ := call(alice, "", http.MethodGet, "/v1/tenant/notifications"); code != http.StatusBadRequest {
		t.Fatalf("missing header = %d, want 400", code)
	}

	// Mark all read in A: A's rows and the un-stamped one, never B's.
	if code, _ := call(alice, orgA.Slug, http.MethodPost, "/v1/tenant/notifications/read-all"); code != http.StatusOK {
		t.Fatalf("read-all A = %d", code)
	}
	if a, b, p := unread(&orgA.ID), unread(&orgB.ID), unread(nil); a != 0 || b != 1 || p != 0 {
		t.Fatalf("after read-all A: unread A=%d B=%d platform=%d, want 0 1 0", a, b, p)
	}
}
