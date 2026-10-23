package orgsettings_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
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

// TestSettingsVersioning drives /v1/tenant/settings over HTTP: every save is
// version +1, old versions stay unchanged (and the table refuses updates),
// base_version conflicts answer 409, validation answers 400 with field
// paths, readiness follows the reservation settings, and members without
// tenant.settings.write cannot save. Runs with TEST_DATABASE_URL.
func TestSettingsVersioning(t *testing.T) {
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
	owner, clerk := newUser("owner"), newUser("clerk")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), nil)
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Settings " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{owner.ID, clerk.ID})
	})
	if _, err := orgSvc.AddMember(ctx, org.UUID, clerk.Uuid, "resepsiyon"); err != nil {
		t.Fatal(err)
	}

	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	svc := usecase.New(repository.NewPostgres(pool, q))
	orgsettings.RegisterRoutes(mux, handler.New(svc, nil), tokens, mapLoader{owner.Uuid: owner.ID, clerk.Uuid: clerk.ID}, orgSvc)
	call := func(user db.User, method, path string, body any) (int, map[string]any, map[string]any) {
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
		errBody, _ := env["error"].(map[string]any)
		return rr.Code, data, errBody
	}

	// Nothing saved yet: version 0, defaults, not ready.
	code, current, _ := call(owner, http.MethodGet, "/v1/tenant/settings", nil)
	if code != http.StatusOK || current["version"].(float64) != 0 || current["ready_for_online_sales"] != false {
		t.Fatalf("initial = %d %v", code, current)
	}
	settings := current["settings"].(map[string]any)
	if settings["tax"].(map[string]any)["default_vat_rate"] != "" {
		t.Fatalf("default VAT must be empty: %v", settings["tax"])
	}

	// Save 1: company only.
	settings["company"] = map[string]any{"legal_name": "Acme Kuaför Ltd. Şti.", "tax_number": "4540536920", "tax_office": "Kadıköy", "address": "", "phone": "", "email": "info@acme.test"}
	code, v1, _ := call(owner, http.MethodPut, "/v1/tenant/settings", map[string]any{"settings": settings, "base_version": 0})
	if code != http.StatusOK || v1["version"].(float64) != 1 || v1["ready_for_online_sales"] != false {
		t.Fatalf("save 1 = %d %v", code, v1)
	}
	if missing := v1["missing"].([]any); len(missing) != 3 {
		t.Fatalf("missing after save 1 = %v", missing)
	}

	// Save 2: reservation settings complete -> ready.
	settings["reservation"] = map[string]any{
		"approval_mode": "staff", "staff_review_window_minutes": 30, "min_lead_minutes": 60, "max_advance_days": 30,
	}
	code, v2, _ := call(owner, http.MethodPut, "/v1/tenant/settings", map[string]any{"settings": settings, "base_version": 1})
	if code != http.StatusOK || v2["version"].(float64) != 2 || v2["ready_for_online_sales"] != true {
		t.Fatalf("save 2 = %d %v", code, v2)
	}
	if v2["created_by"].(map[string]any)["email"] != owner.Email {
		t.Fatalf("created_by = %v", v2["created_by"])
	}

	// Stale base version -> 409, nothing stored.
	if code, _, _ := call(owner, http.MethodPut, "/v1/tenant/settings", map[string]any{"settings": settings, "base_version": 1}); code != http.StatusConflict {
		t.Fatalf("stale save = %d, want 409", code)
	}
	// Invalid VKN -> 400 with the field path.
	bad := map[string]any{}
	raw, _ := json.Marshal(settings)
	_ = json.Unmarshal(raw, &bad)
	bad["company"].(map[string]any)["tax_number"] = "4540536921"
	code, _, errBody := call(owner, http.MethodPut, "/v1/tenant/settings", map[string]any{"settings": bad})
	if code != http.StatusBadRequest {
		t.Fatalf("bad VKN = %d", code)
	}
	if details := errBody["details"].([]any); details[0].(map[string]any)["field"] != "company.tax_number" {
		t.Fatalf("bad VKN details = %v", details)
	}

	// Save 3 without base_version (last write wins).
	settings["policies"] = map[string]any{"cancellation": "Randevudan 2 saat öncesine kadar ücretsiz iptal", "no_show": "", "change": ""}
	code, v3, _ := call(owner, http.MethodPut, "/v1/tenant/settings", map[string]any{"settings": settings})
	if code != http.StatusOK || v3["version"].(float64) != 3 {
		t.Fatalf("save 3 = %d %v", code, v3)
	}

	// Old versions are unchanged.
	code, old, _ := call(owner, http.MethodGet, "/v1/tenant/settings/versions/1", nil)
	if code != http.StatusOK || old["version"].(float64) != 1 || old["ready_for_online_sales"] != false {
		t.Fatalf("version 1 = %d %v", code, old)
	}
	if old["settings"].(map[string]any)["reservation"].(map[string]any)["approval_mode"] != "" {
		t.Fatalf("version 1 changed: %v", old["settings"])
	}
	if code, _, _ := call(owner, http.MethodGet, "/v1/tenant/settings/versions/9", nil); code != http.StatusNotFound {
		t.Fatalf("missing version = %d", code)
	}
	// History list, newest first.
	code, page, _ := call(owner, http.MethodGet, "/v1/tenant/settings/versions", nil)
	items := page["items"].([]any)
	if code != http.StatusOK || page["total"].(float64) != 3 || items[0].(map[string]any)["version"].(float64) != 3 {
		t.Fatalf("history = %d %v", code, page)
	}
	// The table itself refuses edits of a stored version.
	if _, err := pool.Exec(ctx, "UPDATE organization_settings_versions SET data = '{}' WHERE organization_id = $1 AND version = 1", org.ID); err == nil {
		t.Fatal("updating a stored version must fail")
	}

	// A member with tenant.settings.read only can read but not save.
	if code, _, _ := call(clerk, http.MethodGet, "/v1/tenant/settings", nil); code != http.StatusOK {
		t.Fatalf("clerk read = %d", code)
	}
	if code, _, _ := call(clerk, http.MethodPut, "/v1/tenant/settings", map[string]any{"settings": settings}); code != http.StatusForbidden {
		t.Fatalf("clerk save = %d, want 403", code)
	}
}
