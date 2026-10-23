package branches_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
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
	tenantmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tenant"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type livePublication struct {
	channel string
	event   resourceevents.Event
}

type fakeCentrifugo struct {
	mu   sync.Mutex
	pubs []livePublication
}

func (f *fakeCentrifugo) Publish(_ context.Context, channel string, data any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	ev, _ := data.(resourceevents.Event)
	f.pubs = append(f.pubs, livePublication{channel: channel, event: ev})
	return nil
}

func (f *fakeCentrifugo) take() []livePublication {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.pubs
	f.pubs = nil
	return out
}

// TestTenantBranchesLiveEvents checks that branch mutations and bulk jobs
// publish on the organization's channel only, and that only members with
// tenant.branches.read may subscribe to it. Runs with TEST_DATABASE_URL.
func TestTenantBranchesLiveEvents(t *testing.T) {
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
			Email: name + "-live-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	alice, bob := newUser("alice"), newUser("bob")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	orgA, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Live " + suffix, OwnerUserID: alice.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", orgA.ID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{alice.ID, bob.ID})
	})

	fake := &fakeCentrifugo{}
	events := resourceevents.New(fake, nil)
	events.AddResolver(tenantmodule.ChannelResolver(tenantmodule.QueriesLookup(q)))

	svc := branchusecase.New(branchrepository.NewPostgres(q), nil)
	bulkSvc := bulkusecase.New(q, bulkengine.NewRegistry(adapters.NewBulk(svc)), nil, nil, nil, config.BulkConfig{}, nil)
	bulkSvc.SetResourceEvents(events)
	h := branchhandler.New(svc, nil)
	h.SetResourceEvents(events)
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	branches.RegisterRoutes(mux, h, branches.EngineHandlers{
		Bulk: bulkhandler.New(bulkSvc).TenantExecute(branches.Resource),
	}, tokens, mapLoader{alice.Uuid: alice.ID, bob.Uuid: bob.ID}, orgSvc)

	call := func(method, path string, body any) (int, map[string]any) {
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
		req.Header.Set(orgctx.HeaderName, orgA.Slug)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		var env map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		data, _ := env["data"].(map[string]any)
		return rr.Code, data
	}
	wantChannel := tenantmodule.ResourceChannel(orgA.UUID, branches.Resource)
	expect := func(step, action string, uuids ...string) {
		t.Helper()
		events.Wait()
		pubs := fake.take()
		if len(pubs) != 1 {
			t.Fatalf("%s: publications = %+v, want 1", step, pubs)
		}
		got := pubs[0]
		if got.channel != wantChannel || got.event.Resource != branches.Resource || got.event.Action != action {
			t.Fatalf("%s: got %s %+v", step, got.channel, got.event)
		}
		if len(uuids) > 0 && (len(got.event.UUIDs) != len(uuids) || got.event.UUIDs[0] != uuids[0]) {
			t.Fatalf("%s: uuids = %v, want %v", step, got.event.UUIDs, uuids)
		}
	}

	code, created := call(http.MethodPost, "/v1/tenant/branches", map[string]any{"code": "LIVE", "name": "Live"})
	if code != http.StatusCreated {
		t.Fatalf("create = %d", code)
	}
	id := created["uuid"].(string)
	expect("create", resourceevents.ActionCreated, id)

	if code, _ := call(http.MethodPost, "/v1/tenant/branches/"+id+"/service-points", map[string]any{
		"type": "airport", "name": "SAW", "extra_fee": "10",
	}); code != http.StatusCreated {
		t.Fatalf("create point = %d", code)
	}
	expect("service point", resourceevents.ActionUpdated, id)

	if code, _ := call(http.MethodPost, "/v1/tenant/branches/bulk", map[string]any{
		"action": "deactivate", "target": map[string]any{"scope": "ids", "ids": []string{id}},
	}); code != http.StatusOK {
		t.Fatalf("bulk = %d", code)
	}
	expect("bulk", resourceevents.ActionBulk, id)

	// A background job only knows the organization id: the lookup fills in
	// the uuid.
	events.Publish(orgctx.WithOrganizationID(ctx, orgA.ID), branches.Resource, resourceevents.ActionBulk, id)
	expect("worker", resourceevents.ActionBulk, id)

	if code, _ := call(http.MethodDelete, "/v1/tenant/branches/"+id, nil); code != http.StatusOK {
		t.Fatalf("delete = %d", code)
	}
	expect("delete", resourceevents.ActionDeleted, id)

	// Subscription authorization for the organization channel.
	reg := resourceevents.NewRegistry()
	branches.RegisterLiveResources(reg)
	rule := tenantmodule.ChannelRule(reg, orgSvc)
	allowed := func(user db.User, channel string) bool {
		t.Helper()
		pctx := authctx.WithPrincipal(ctx, authctx.Principal{UserID: user.Uuid, UserInternal: user.ID})
		handled, ok, err := rule(pctx, user.Uuid, false, channel)
		if err != nil || !handled {
			t.Fatalf("rule(%s): handled=%v err=%v", channel, handled, err)
		}
		return ok
	}
	if !allowed(alice, wantChannel) {
		t.Fatal("owner denied the branches channel")
	}
	if allowed(bob, wantChannel) {
		t.Fatal("non-member allowed the branches channel")
	}
	if allowed(alice, tenantmodule.ResourceChannel(orgA.UUID, "tenant.secrets")) {
		t.Fatal("unregistered tenant resource allowed")
	}
	if allowed(alice, tenantmodule.ResourceChannel(uuid.New(), branches.Resource)) {
		t.Fatal("unknown organization allowed")
	}
}
