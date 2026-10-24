package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/providers"
	notifusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RT-A01: cross-cutting organization isolation over the real router.
//
// Two organizations get data in every tenant resource. Then, for every
// /v1/tenant route registered on the server (enumerated from the route
// literals in the module sources, cross-checked against the mux and
// openapi.yaml, so a new route is covered automatically):
//
//   - a member of A naming organization B (X-Organization: B) gets 403;
//   - a member of A calling a route with path parameters filled with B's
//     uuids (and A's parent with B's child) gets 404 and never B's data;
//   - every GET without path parameters, called in A, returns none of B's
//     identifiers;
//   - mutations without path parameters have an explicit case (a body or
//     query naming B's records) or an allowlist entry with the reason.
//
// Afterwards B's records read the same as before. A route without a case
// fails the test (TestTenantIsolationRT_A01 → "uncovered route").

// isolationAllowlist lists tenant routes without path parameters whose body
// names no record of the organization (nothing to swap for B's uuids), with
// the test that covers them otherwise.
var isolationAllowlist = map[string]string{
	"POST /v1/tenant/branches":               "body has no foreign references (code, name)",
	"POST /v1/tenant/roles":                  "body names permission slugs only; role isolation is the {uuid} cases",
	"PUT /v1/tenant/settings":                "settings document of the caller's organization; versions are the {version} cases",
	"POST /v1/tenant/notifications/read-all": "marks the caller's rows of the current organization (notifications scope test)",
	"POST /v1/tenant/branches/import":        "imports match natural keys inside the organization (adapter tests); the job is the {uuid} cases",
	"POST /v1/tenant/staff/import":           "imports match uuids and branch codes inside the organization (staff DB tests); the job is the {uuid} cases",
	"POST /v1/tenant/resources/import":       "imports match branch codes and names inside the organization (resources DB tests); the job is the {uuid} cases",
	"POST /v1/tenant/services/import":        "imports match names, category names and branch codes inside the organization (services DB tests); the job is the {uuid} cases",
	"POST /v1/tenant/service-categories":     "body has no foreign references (name, description, sort order)",
	"POST /v1/tenant/products/import":        "imports match stock codes and branch codes inside the organization (products DB tests); the job is the {uuid} cases",
	"POST /v1/tenant/customers":              "body has no foreign references (name, contact, notes, tags, consent)",
	"POST /v1/tenant/customers/import":       "imports match phones inside the organization (customers DB tests); the job is the {uuid} cases",
	"PATCH /v1/tenant/organization":          "profile of the caller's organization (name only); no foreign references",
	"PUT /v1/tenant/organization/logo":       "logo of the caller's organization; the key is built from the scope (storage prefix check)",
	"DELETE /v1/tenant/organization/logo":    "logo of the caller's organization; no ids",
	"POST /v1/tenant/integrations":           "body has no foreign references (kind, provider code, display name, mode, the organization's own credentials)",
}

type isoOrg struct {
	org    orgmodel.Organization
	owner  db.User
	marker string
	ids    map[string]string // key → uuid (or version number)
}

func TestTenantIsolationRT_A01(t *testing.T) {
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

	const secret = "isolation-test-access-secret-0123456789"
	cfg := config.Config{
		App:        config.AppConfig{Name: "Isolation", Env: "test"},
		JWT:        config.JWTConfig{AccessSecret: secret, RefreshSecret: secret, AccessTTL: time.Hour, RefreshTTL: time.Hour},
		Encryption: config.EncryptionConfig{Key: strings.Repeat("k", 32)},
		Tenancy:    config.TenancyConfig{Enabled: true},
		Bulk:       config.BulkConfig{SyncMax: 1, RollbackHours: 24},
	}
	if host := os.Getenv("TEST_MEILI_HOST"); host != "" {
		cfg.Search = config.SearchConfig{
			Enabled: true, Driver: "meilisearch", MeiliHost: host, MeiliKey: os.Getenv("TEST_MEILI_KEY"),
			IndexPrefix: "iso" + strings.ReplaceAll(uuid.NewString()[:8], "-", ""),
		}
	}
	mem := &memStorage{m: storage.NewMemory()}
	// Server errors are printed with -v (the defence-in-depth checks below
	// log expected errors).
	logOut := io.Discard
	if testing.Verbose() {
		logOut = os.Stderr
	}
	log := slog.New(slog.NewTextHandler(logOut, &slog.HandlerOptions{Level: slog.LevelError}))
	srv, err := New(cfg, log, Deps{DB: pool, Queries: q, Storage: mem})
	if err != nil {
		t.Fatal(err)
	}
	handler := srv.http.Handler
	tokens, err := jwt.NewManager(secret, time.Hour, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	// --- Organizations, users ---------------------------------------------
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	var users []int64
	newUser := func(name string) db.User {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: name + "-iso-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Iso", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		users = append(users, u.ID)
		return u
	}
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	alice, bob := newUser("alice"), newUser("bob")
	newOrg := func(name string, owner db.User, marker string) *isoOrg {
		org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: name + " " + suffix, OwnerUserID: owner.ID})
		if err != nil {
			t.Fatal(err)
		}
		// The seed opens a second branch: above the starter plan (TEC-883).
		if org, err = orgSvc.SetEntitlements(ctx, org.UUID, orgmodel.SetEntitlementsInput{Plan: "growth"}); err != nil {
			t.Fatal(err)
		}
		return &isoOrg{org: org, owner: owner, marker: marker, ids: map[string]string{}}
	}
	orgA := newOrg("Iso A", alice, "AMARK"+suffix)
	orgB := newOrg("Iso B", bob, "BMARK"+suffix)
	t.Cleanup(func() {
		bg := context.Background()
		for _, stmt := range []string{
			"DELETE FROM notifications WHERE user_id = ANY($1)",
			"DELETE FROM export_jobs WHERE actor_id = ANY($1)",
			"DELETE FROM import_jobs WHERE actor_id = ANY($1)",
			"DELETE FROM bulk_jobs WHERE actor_id = ANY($1)",
			"DELETE FROM activity_events WHERE actor_user_id = ANY($1)",
		} {
			_, _ = pool.Exec(bg, stmt, users)
		}
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{orgA.org.ID, orgB.org.ID})
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", users)
	})
	// A second member of B (a user of B only; never a member of A).
	carol := newUser("carol")
	if _, err := orgSvc.AddMember(ctx, orgB.org.UUID, carol.Uuid, orgctx.RoleMember); err != nil {
		t.Fatal(err)
	}
	orgB.ids["member"] = carol.Uuid.String()
	orgA.ids["member"] = alice.Uuid.String()

	// --- HTTP helpers -----------------------------------------------------
	type resp struct {
		code int
		raw  []byte
		data any
	}
	do := func(user db.User, orgRef, method, path, contentType string, body io.Reader) resp {
		t.Helper()
		// cms_user: the seeded platform role of signed-in users that grants
		// notifications.read (the tenant inbox routes need it). Tenant
		// permissions still come from the membership only.
		access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: user.Uuid, Roles: []string{"cms_user"}})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, body)
		req.Header.Set("Authorization", "Bearer "+access)
		if orgRef != "" {
			req.Header.Set(orgctx.HeaderName, orgRef)
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		out := resp{code: rr.Code, raw: rr.Body.Bytes()}
		var env map[string]any
		if json.Unmarshal(out.raw, &env) == nil {
			out.data = env["data"]
		}
		return out
	}
	call := func(o *isoOrg, method, path string, body any) resp {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		return do(o.owner, o.org.Slug, method, path, "application/json", &buf)
	}
	upload := func(o *isoOrg, method, path, field, filename string, content []byte, extra map[string]string) resp {
		t.Helper()
		var form bytes.Buffer
		mw := multipart.NewWriter(&form)
		for k, v := range extra {
			_ = mw.WriteField(k, v)
		}
		fw, _ := mw.CreateFormFile(field, filename)
		_, _ = fw.Write(content)
		_ = mw.Close()
		return do(o.owner, o.org.Slug, method, path, mw.FormDataContentType(), &form)
	}
	str := func(r resp, keys ...string) string {
		cur := r.data
		for _, k := range keys {
			m, _ := cur.(map[string]any)
			cur = m[k]
		}
		s, _ := cur.(string)
		return s
	}
	must := func(r resp, want int, step string) resp {
		t.Helper()
		if r.code != want {
			t.Fatalf("%s = %d, want %d: %s", step, r.code, want, r.raw)
		}
		return r
	}
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 64)...)

	// --- Seed both organizations through the API ---------------------------
	seed := func(o *isoOrg) {
		ids := o.ids
		m := o.marker
		branches := must(call(o, http.MethodGet, "/v1/tenant/branches?q=MRK", nil), http.StatusOK, "list branches")
		items, _ := branches.data.(map[string]any)["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("default branch: %s", branches.raw)
		}
		ids["home"] = items[0].(map[string]any)["uuid"].(string)
		ids["branch"] = str(must(call(o, http.MethodPost, "/v1/tenant/branches", map[string]any{
			"code": "ISO", "name": m + " Şube", "city": m,
		}), http.StatusCreated, "create branch"), "uuid")
		ids["point"] = str(must(call(o, http.MethodPost, "/v1/tenant/branches/"+ids["branch"]+"/service-points", map[string]any{
			"type": "airport", "name": m + " SAW",
		}), http.StatusCreated, "create service point"), "uuid")
		ids["branchImage"] = str(must(upload(o, http.MethodPost, "/v1/tenant/branches/"+ids["branch"]+"/images", "file", "photo.png",
			png, nil), http.StatusCreated, "upload branch image"), "uuid")
		ids["role"] = str(must(call(o, http.MethodPost, "/v1/tenant/roles", map[string]any{
			"key": "iso_role", "name": m + " Rol", "permissions": []string{rbac.PermTenantMembersRead},
		}), http.StatusCreated, "create role"), "uuid")
		ids["invitation"] = str(must(call(o, http.MethodPost, "/v1/tenant/invitations", map[string]any{
			"email": strings.ToLower(m) + "-invitee@iso.test", "role_uuids": []string{ids["role"]},
			"branch_scope": "selected", "branch_uuids": []string{ids["branch"]},
		}), http.StatusCreated, "create invitation"), "uuid")
		settings := must(call(o, http.MethodGet, "/v1/tenant/settings", nil), http.StatusOK, "settings")
		doc := settings.data.(map[string]any)["settings"].(map[string]any)
		doc["company"] = map[string]any{"legal_name": m + " A.Ş.", "tax_number": "4540536920", "tax_office": "Kadıköy", "address": "", "phone": "", "email": "info@iso.test"}
		must(call(o, http.MethodPut, "/v1/tenant/settings", map[string]any{"settings": doc, "base_version": 0}), http.StatusOK, "settings v1")
		ids["staff"] = str(must(call(o, http.MethodPost, "/v1/tenant/staff", map[string]any{
			"name": m + " Personel", "branch_uuids": []string{ids["branch"]}, "account_uuid": o.owner.Uuid.String(),
		}), http.StatusCreated, "create staff"), "uuid")
		ids["timeOff"] = str(must(call(o, http.MethodPost, "/v1/tenant/staff/"+ids["staff"]+"/time-off", map[string]any{
			"kind": "leave", "starts_at": "2030-01-01T09:00:00Z", "ends_at": "2030-01-02T09:00:00Z", "note": m,
		}), http.StatusCreated, "create time off"), "uuid")
		ids["resource"] = str(must(call(o, http.MethodPost, "/v1/tenant/resources", map[string]any{
			"branch_uuid": ids["branch"], "kind": "chair", "name": m + " Koltuk 1",
		}), http.StatusCreated, "create resource"), "uuid")
		ids["closure"] = str(must(call(o, http.MethodPost, "/v1/tenant/resources/"+ids["resource"]+"/closures", map[string]any{
			"starts_at": "2030-01-01T09:00:00Z", "ends_at": "2030-01-02T09:00:00Z", "reason": m,
		}), http.StatusCreated, "create closure"), "uuid")
		ids["serviceCategory"] = str(must(call(o, http.MethodPost, "/v1/tenant/service-categories", map[string]any{
			"name": m + " Kategori",
		}), http.StatusCreated, "create service category"), "uuid")
		ids["service"] = str(must(call(o, http.MethodPost, "/v1/tenant/services", map[string]any{
			"name": m + " Hizmet", "category_uuid": ids["serviceCategory"], "duration_minutes": 30, "price": "450.00",
			"branch_uuids": []string{ids["branch"]},
		}), http.StatusCreated, "create service"), "uuid")
		must(call(o, http.MethodPut, "/v1/tenant/services/"+ids["service"]+"/staff", map[string]any{
			"staff": []map[string]any{{"staff_uuid": ids["staff"]}},
		}), http.StatusOK, "service staff")
		cover := must(upload(o, http.MethodPut, "/v1/tenant/services/"+ids["service"]+"/image", "file", "cover.png",
			png, nil), http.StatusOK, "upload service image")
		ids["serviceImageURL"] = str(cover, "image_url")
		ids["product"] = str(must(call(o, http.MethodPost, "/v1/tenant/products", map[string]any{
			"name": m + " Ürün", "sku": m + "-SKU", "barcode": m + "-BAR", "price": "120.00", "cost": "60.00",
			"branch_uuids": []string{ids["branch"]},
		}), http.StatusCreated, "create product"), "uuid")
		ids["productSKU"] = m + "-SKU"
		variant := must(call(o, http.MethodPost, "/v1/tenant/products/"+ids["product"]+"/variants", map[string]any{
			"name": m + " Varyant", "sku": m + "-VAR",
		}), http.StatusCreated, "create variant")
		ids["variant"] = variant.data.(map[string]any)["variants"].([]any)[0].(map[string]any)["uuid"].(string)
		must(call(o, http.MethodPost, "/v1/tenant/stock-movements", map[string]any{
			"kind": "receipt", "branch_uuid": ids["branch"], "product_uuid": ids["product"], "variant_uuid": ids["variant"],
			"quantity": 5, "reason": m,
		}), http.StatusCreated, "stock receipt")
		must(call(o, http.MethodPost, "/v1/tenant/stock-movements", map[string]any{
			"kind": "sale", "branch_uuid": ids["branch"], "product_uuid": ids["product"], "variant_uuid": ids["variant"],
			"quantity": 1, "staff_uuid": ids["staff"], "reason": m,
		}), http.StatusCreated, "stock sale")
		ids["customerPhone"] = map[bool]string{true: "+905320000001", false: "+905320000002"}[o == orgA]
		ids["customer"] = str(must(call(o, http.MethodPost, "/v1/tenant/customers", map[string]any{
			"first_name": m, "last_name": "Müşteri", "phone": ids["customerPhone"], "notes": m, "tags": []string{m},
		}), http.StatusCreated, "create customer"), "uuid")
		must(call(o, http.MethodPut, "/v1/tenant/staff/"+ids["staff"]+"/branches/"+ids["branch"]+"/shifts", map[string]any{
			"mon": []map[string]any{{"start": "09:00", "end": "18:00"}},
		}), http.StatusOK, "staff shifts")
		ids["hold"] = str(must(call(o, http.MethodPost, "/v1/tenant/availability/holds", map[string]any{
			"branch_uuid": ids["branch"], "lines": []map[string]any{{"service_uuid": ids["service"]}},
			"starts_at": "2030-01-07T09:00:00+03:00", "reason": "approval", "minutes": 1440,
		}), http.StatusCreated, "create hold"), "uuid")
		ids["reservation"] = str(must(call(o, http.MethodPost, "/v1/tenant/reservations", map[string]any{
			"branch_uuid": ids["branch"], "customer_uuid": ids["customer"], "lines": []map[string]any{{"service_uuid": ids["service"]}},
			"starts_at": "2030-01-07T10:00:00+03:00", "internal_note": m,
		}), http.StatusCreated, "create reservation"), "uuid")
		ids["cashAccount"] = str(must(call(o, http.MethodPost, "/v1/tenant/cash/accounts", map[string]any{
			"branch_uuid": ids["branch"], "kind": "cash", "name": m + " Kasa", "opening_balance": "100",
		}), http.StatusCreated, "create cash account"), "uuid")
		ids["cashMovement"] = str(must(call(o, http.MethodPost, "/v1/tenant/cash/accounts/"+ids["cashAccount"]+"/movements", map[string]any{
			"direction": "in", "amount": "50", "description": m + " giriş",
		}), http.StatusCreated, "create cash movement"), "uuid")
		ids["cashClosing"] = str(must(call(o, http.MethodPost, "/v1/tenant/cash/accounts/"+ids["cashAccount"]+"/closings", map[string]any{
			"counted_amount": "150", "note": m,
		}), http.StatusCreated, "close cash day"), "uuid")
		// The stock sale above is a product sale line with its ledger debit.
		saleLines := must(call(o, http.MethodGet, "/v1/tenant/sales/lines", nil), http.StatusOK, "sale lines")
		ids["saleLine"] = saleLines.data.(map[string]any)["items"].([]any)[0].(map[string]any)["uuid"].(string)
		entries := must(call(o, http.MethodGet, "/v1/tenant/sales/entries", nil), http.StatusOK, "ledger entries")
		ids["ledgerEntry"] = entries.data.(map[string]any)["items"].([]any)[0].(map[string]any)["uuid"].(string)
		ids["exportJob"] = str(must(call(o, http.MethodPost, "/v1/tenant/branches/export", map[string]any{"format": "csv"}),
			http.StatusAccepted, "export"), "uuid")
		ids["importJob"] = str(must(upload(o, http.MethodPost, "/v1/tenant/branches/import", "file", "branches.csv",
			[]byte("code,name\nIMP,"+m+" Import\n"), map[string]string{"format": "csv"}), http.StatusCreated, "import upload"), "uuid")
		// Two targets > Bulk.SyncMax: a bulk job row (run inline without a queue).
		branch2 := str(must(call(o, http.MethodPost, "/v1/tenant/branches", map[string]any{
			"code": "ISO2", "name": m + " Şube 2", "city": m,
		}), http.StatusCreated, "create branch 2"), "uuid")
		ids["bulkJob"] = str(must(call(o, http.MethodPost, "/v1/tenant/branches/bulk", map[string]any{
			"action": "deactivate", "target": map[string]any{"scope": "ids", "ids": []string{ids["branch"], branch2}},
		}), http.StatusAccepted, "bulk"), "uuid")
		// Integration account (log provider, no network) and a failed job
		// of it: the {uuid} of /v1/tenant/integration-jobs.
		ids["integrationAccount"] = str(must(call(o, http.MethodPost, "/v1/tenant/integrations", map[string]any{
			"kind": "sms", "provider": "log", "display_name": m + " sms", "credentials": map[string]any{"sender": "TAKWIM"},
		}), http.StatusCreated, "connect integration"), "uuid")
		var jobUUID uuid.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO integration_jobs (organization_id, integration_account_id, direction, provider,
			kind, idempotency_key, status, attempts, last_error)
			SELECT a.organization_id, a.id, 'outbound', 'log', 'sms.send', $2, 'failed', 1, $3
			FROM integration_accounts a WHERE a.uuid = $1 RETURNING uuid`,
			ids["integrationAccount"], "iso:"+m, m+" failed").Scan(&jobUUID); err != nil {
			t.Fatal(err)
		}
		ids["integrationJob"] = jobUUID.String()
	}
	seed(orgA)
	seed(orgB)
	// B saves two more settings versions, so versions 2 and 3 exist in B only.
	for i := 1; i <= 2; i++ {
		settings := must(call(orgB, http.MethodGet, "/v1/tenant/settings", nil), http.StatusOK, "settings")
		doc := settings.data.(map[string]any)["settings"].(map[string]any)
		doc["company"].(map[string]any)["address"] = fmt.Sprintf("%s adres %d", orgB.marker, i)
		must(call(orgB, http.MethodPut, "/v1/tenant/settings", map[string]any{"settings": doc, "base_version": i}), http.StatusOK, "settings B")
	}
	orgA.ids["version"] = "1"
	orgB.ids["version"] = "3"
	// Org-stamped notification of B (bob's inbox in B).
	notif := notifusecase.New(q, nil, []providers.Provider{providers.InappProvider{}}, nil)
	bobID := bob.ID
	if _, err := notif.Enqueue(ctx, notifmodel.EnqueueInput{
		UserID: &bobID, OrganizationID: &orgB.org.ID, Channels: []string{notifmodel.ChannelInapp},
		Title: orgB.marker + " bildirim", Body: orgB.marker, SourceEvent: "test",
	}); err != nil {
		t.Fatal(err)
	}

	// B's identifiers: none of them may appear in a response to A.
	leaks := []string{orgB.marker, orgB.org.UUID.String(), orgB.org.Slug, bob.Uuid.String(), carol.Uuid.String()}
	for k, v := range orgB.ids {
		if k != "version" {
			leaks = append(leaks, v)
		}
	}
	leaked := func(raw []byte) string {
		s := string(raw)
		for _, l := range leaks {
			if strings.Contains(s, l) {
				return l
			}
		}
		return ""
	}
	// Sanity: B's own list shows its marker (the leak check is meaningful).
	if r := call(orgB, http.MethodGet, "/v1/tenant/branches", nil); leaked(r.raw) == "" {
		t.Fatalf("B's list does not contain B's identifiers: %s", r.raw)
	}

	// --- Route table ------------------------------------------------------
	routes := tenantRoutes(t, srv.mux)
	paramRE := regexp.MustCompile(`\{([^}]+)\}`)
	fill := func(pattern string, pick func(key string) string) (string, bool) {
		_, path, _ := strings.Cut(pattern, " ")
		segs := strings.Split(path, "/")
		for i, s := range segs {
			m := paramRE.FindStringSubmatch(s)
			if m == nil {
				continue
			}
			key := paramKey(segs[i-1], m[1])
			if key == "" {
				return "", false
			}
			segs[i] = pick(key)
		}
		return strings.Join(segs, "/"), true
	}
	bodyFor := func(pattern string) (string, io.Reader) {
		method, path, _ := strings.Cut(pattern, " ")
		if strings.HasSuffix(path, "/file") && method == http.MethodPut || strings.HasSuffix(path, "/images") && method == http.MethodPost ||
			strings.HasSuffix(path, "/image") && method == http.MethodPut {
			var form bytes.Buffer
			mw := multipart.NewWriter(&form)
			fw, _ := mw.CreateFormFile("file", "x.png")
			_, _ = fw.Write(png)
			_ = mw.Close()
			return mw.FormDataContentType(), &form
		}
		if method == http.MethodGet || method == http.MethodDelete {
			return "", nil
		}
		// Strict decoders refuse unknown fields: an empty object reaches the
		// lookup (handlers that validate first would answer 400 here, which
		// the 404 check reports).
		return "application/json", strings.NewReader("{}")
	}

	// Snapshot of B's records (as bob) before the attacks.
	snapshot := func() map[string]string {
		out := map[string]string{}
		for _, rt := range routes {
			if !strings.HasPrefix(rt, "GET ") || !strings.Contains(rt, "{") || strings.HasSuffix(rt, "/download") || strings.HasSuffix(rt, "/file") {
				continue
			}
			path, ok := fill(rt, func(k string) string { return orgB.ids[k] })
			if !ok {
				continue
			}
			r := call(orgB, http.MethodGet, path, nil)
			if r.code != http.StatusOK {
				t.Fatalf("B reads its own %s = %d: %s", path, r.code, r.raw)
			}
			b, _ := json.Marshal(r.data)
			out[path] = string(b)
		}
		return out
	}
	before := snapshot()

	covered := map[string]string{}
	// 1. Not a member of B: every route answers 403 with X-Organization: B.
	for _, rt := range routes {
		method, _, _ := strings.Cut(rt, " ")
		path, ok := fill(rt, func(k string) string { return orgB.ids[k] })
		if !ok {
			path, _ = fill(rt, func(string) string { return uuid.NewString() })
		}
		ct, body := bodyFor(rt)
		r := do(alice, orgB.org.Slug, method, path, ct, body)
		if r.code != http.StatusForbidden {
			t.Errorf("non-member %s (X-Organization: B) = %d, want 403: %s", rt, r.code, r.raw)
		}
		if l := leaked(r.raw); l != "" {
			t.Errorf("non-member %s leaked %q", rt, l)
		}
	}

	// 2. Path parameters with B's uuids (and A's parent with B's child).
	for _, rt := range routes {
		if !strings.Contains(rt, "{") {
			continue
		}
		method, _, _ := strings.Cut(rt, " ")
		path, ok := fill(rt, func(k string) string { return orgB.ids[k] })
		if !ok {
			t.Errorf("uncovered route %s: no isolation value for its path parameters (extend paramKey and the seed)", rt)
			continue
		}
		variants := []string{path}
		if strings.Count(rt, "{") > 1 {
			n := 0
			mixed, _ := fill(rt, func(k string) string {
				n++
				if n == 1 {
					return orgA.ids[k]
				}
				return orgB.ids[k]
			})
			variants = append(variants, mixed)
		}
		for _, p := range variants {
			ct, body := bodyFor(rt)
			r := do(alice, orgA.org.Slug, method, p, ct, body)
			if r.code != http.StatusNotFound {
				t.Errorf("%s %s (B's ids, member of A) = %d, want 404: %s", method, p, r.code, r.raw)
			}
			if l := leaked(r.raw); l != "" {
				t.Errorf("%s %s leaked %q: %s", method, p, l, r.raw)
			}
		}
		covered[rt] = "path parameters"
	}

	// 3. GET routes without path parameters: no B identifier in A's answer
	// (with and without filters naming B's records).
	probes := map[string][]string{
		"GET /v1/tenant/search":           {"?q=" + orgB.marker},
		"GET /v1/tenant/integrations":     {"?q=" + orgB.marker},
		"GET /v1/tenant/integration-jobs": {"?status=failed"},
		"GET /v1/tenant/services":         {"?category=" + orgB.ids["serviceCategory"], "?branch=" + orgB.ids["branch"]},
		"GET /v1/tenant/products":         {"?branch=" + orgB.ids["branch"]},
		"GET /v1/tenant/products/lookup":  {"?code=" + orgB.ids["productSKU"]},
		"GET /v1/tenant/stock-movements": {"?branch=" + orgB.ids["branch"], "?product=" + orgB.ids["product"],
			"?staff=" + orgB.ids["staff"]},
		"GET /v1/tenant/customers":          {"?q=" + orgB.ids["customerPhone"][3:], "?tag=" + orgB.marker},
		"GET /v1/tenant/cash/accounts":      {"?branch=" + orgB.ids["branch"], "?q=" + orgB.marker},
		"GET /v1/tenant/cash/movements":     {"?account=" + orgB.ids["cashAccount"], "?q=" + orgB.marker},
		"GET /v1/tenant/cash/daily-summary": {"?branch=" + orgB.ids["branch"]},
		"GET /v1/tenant/reservations": {"?q=" + orgB.ids["customerPhone"][3:], "?branch=" + orgB.ids["branch"],
			"?staff=" + orgB.ids["staff"], "?customer_uuid=" + orgB.ids["customer"]},
		"GET /v1/tenant/reservations/calendar": {"?from=2030-01-07&by=branch", "?from=2030-01-07&branch=" + orgB.ids["branch"],
			"?from=2030-01-07&by=resource&branch=" + orgB.ids["branch"], "?from=2030-01-07&by=branch&branch=" + orgB.ids["branch"]},
		"GET /v1/tenant/reservations/agenda": {"?from=2030-01-07&to=2030-01-13", "?from=2030-01-07&include_cancelled=true"},
		"GET /v1/tenant/sales/lines": {"?branch=" + orgB.ids["branch"], "?reservation=" + orgB.ids["reservation"],
			"?customer=" + orgB.ids["customer"], "?staff=" + orgB.ids["staff"], "?q=" + orgB.marker},
		"GET /v1/tenant/sales/entries": {"?branch=" + orgB.ids["branch"], "?reservation=" + orgB.ids["reservation"],
			"?customer=" + orgB.ids["customer"], "?q=" + orgB.marker},
		"GET /v1/tenant/sales/summary": {"?branch=" + orgB.ids["branch"]},
	}
	for _, rt := range routes {
		method, path, _ := strings.Cut(rt, " ")
		if method != http.MethodGet || strings.Contains(rt, "{") {
			continue
		}
		for _, query := range append([]string{""}, probes[rt]...) {
			r := call(orgA, http.MethodGet, path+query, nil)
			if r.code >= 500 {
				t.Errorf("%s%s = %d: %s", rt, query, r.code, r.raw)
			}
			if l := leaked(r.raw); l != "" {
				t.Errorf("%s%s (member of A) leaked %q: %s", rt, query, l, r.raw)
			}
			// A probe must reach the filter: 200, or the record named by the
			// query is unknown in A ("not_found").
			if query != "" && r.code != http.StatusOK && (r.code >= 500 || !bytes.Contains(r.raw, []byte(`"not_found"`))) {
				t.Errorf("probe %s%s = %d (the probe must reach the query): %s", rt, query, r.code, r.raw)
			}
		}
		covered[rt] = "GET leak scan"
	}

	// 4. Mutations without path parameters: bodies naming B's records.
	type bodyCase struct {
		route string
		body  map[string]any
		want  []int
	}
	reject := []int{http.StatusBadRequest, http.StatusNotFound, http.StatusUnprocessableEntity}
	bulkIDs := func(key string) map[string]any {
		return map[string]any{"action": "deactivate", "target": map[string]any{"scope": "ids", "ids": []string{orgB.ids[key]}}}
	}
	cases := []bodyCase{
		{"POST /v1/tenant/invitations", map[string]any{"email": "x1@iso.test", "role_uuids": []string{orgB.ids["role"]}, "branch_scope": "all"}, reject},
		{"POST /v1/tenant/invitations", map[string]any{"email": "x2@iso.test", "role_uuids": []string{orgA.ids["role"]},
			"branch_scope": "selected", "branch_uuids": []string{orgB.ids["branch"]}}, reject},
		{"POST /v1/tenant/branches/bulk", bulkIDs("branch"), nil},
		{"POST /v1/tenant/staff", map[string]any{"name": "X", "branch_uuids": []string{orgB.ids["branch"]}}, reject},
		{"POST /v1/tenant/staff", map[string]any{"name": "X", "branch_uuids": []string{orgA.ids["branch"]}, "account_uuid": orgB.ids["member"]}, reject},
		{"POST /v1/tenant/staff/bulk", bulkIDs("staff"), nil},
		{"POST /v1/tenant/resources", map[string]any{"branch_uuid": orgB.ids["branch"], "kind": "chair", "name": "X"}, reject},
		{"POST /v1/tenant/resources/bulk", bulkIDs("resource"), nil},
		{"POST /v1/tenant/services", map[string]any{"name": "X1", "duration_minutes": 30, "price": "1", "category_uuid": orgB.ids["serviceCategory"]}, reject},
		{"POST /v1/tenant/services", map[string]any{"name": "X2", "duration_minutes": 30, "price": "1", "branch_uuids": []string{orgB.ids["branch"]}}, reject},
		{"POST /v1/tenant/services/bulk", bulkIDs("service"), nil},
		{"POST /v1/tenant/products", map[string]any{"name": "X", "sku": "ISO-X1", "price": "1", "branch_uuids": []string{orgB.ids["branch"]}}, reject},
		{"POST /v1/tenant/products/bulk", bulkIDs("product"), nil},
		{"POST /v1/tenant/stock-movements", map[string]any{"kind": "receipt", "branch_uuid": orgA.ids["branch"],
			"product_uuid": orgB.ids["product"], "variant_uuid": orgB.ids["variant"], "quantity": 1}, reject},
		{"POST /v1/tenant/stock-movements", map[string]any{"kind": "receipt", "branch_uuid": orgB.ids["branch"],
			"product_uuid": orgA.ids["product"], "variant_uuid": orgA.ids["variant"], "quantity": 1}, reject},
		{"POST /v1/tenant/stock-movements", map[string]any{"kind": "receipt", "branch_uuid": orgA.ids["branch"],
			"product_uuid": orgA.ids["product"], "variant_uuid": orgB.ids["variant"], "quantity": 1}, reject},
		{"POST /v1/tenant/stock-movements", map[string]any{"kind": "transfer", "branch_uuid": orgA.ids["branch"], "to_branch_uuid": orgB.ids["branch"],
			"product_uuid": orgA.ids["product"], "variant_uuid": orgA.ids["variant"], "quantity": 1}, reject},
		{"POST /v1/tenant/stock-movements", map[string]any{"kind": "sale", "branch_uuid": orgA.ids["branch"], "staff_uuid": orgB.ids["staff"],
			"product_uuid": orgA.ids["product"], "variant_uuid": orgA.ids["variant"], "quantity": 1}, reject},
		{"POST /v1/tenant/cash/accounts", map[string]any{"branch_uuid": orgB.ids["branch"], "kind": "cash", "name": "X"}, reject},
		{"POST /v1/tenant/cash/accounts/bulk", map[string]any{"action": "archive", "target": map[string]any{"scope": "ids", "ids": []string{orgB.ids["cashAccount"]}}}, nil},
		{"POST /v1/tenant/cash/transfers", map[string]any{"from_account_uuid": orgA.ids["cashAccount"], "to_account_uuid": orgB.ids["cashAccount"], "amount": "1"}, reject},
		{"POST /v1/tenant/cash/transfers", map[string]any{"from_account_uuid": orgB.ids["cashAccount"], "to_account_uuid": orgA.ids["cashAccount"], "amount": "1"}, reject},
		{"POST /v1/tenant/customers/bulk", map[string]any{"action": "delete", "target": map[string]any{"scope": "ids", "ids": []string{orgB.ids["customer"]}}}, nil},
		{"POST /v1/tenant/reservations/bulk", map[string]any{"action": "cancel", "target": map[string]any{"scope": "ids", "ids": []string{orgB.ids["reservation"]}}}, nil},
		{"POST /v1/tenant/availability/holds", map[string]any{"branch_uuid": orgB.ids["branch"], "lines": []map[string]any{{"service_uuid": orgA.ids["service"]}},
			"starts_at": "2030-01-07T10:00:00+03:00", "reason": "checkout"}, reject},
		{"POST /v1/tenant/availability/holds", map[string]any{"branch_uuid": orgA.ids["branch"], "lines": []map[string]any{
			{"service_uuid": orgA.ids["service"], "staff_uuid": orgB.ids["staff"]}}, "starts_at": "2030-01-07T10:00:00+03:00", "reason": "checkout"}, reject},
		{"POST /v1/tenant/availability/search", map[string]any{"branch_uuid": orgB.ids["branch"], "lines": []map[string]any{{"service_uuid": orgA.ids["service"]}}}, reject},
		{"POST /v1/tenant/reservations", map[string]any{"branch_uuid": orgB.ids["branch"], "customer_uuid": orgA.ids["customer"],
			"lines": []map[string]any{{"service_uuid": orgA.ids["service"]}}, "starts_at": "2030-01-07T11:00:00+03:00"}, reject},
		{"POST /v1/tenant/reservations", map[string]any{"branch_uuid": orgA.ids["branch"], "customer_uuid": orgB.ids["customer"],
			"lines": []map[string]any{{"service_uuid": orgA.ids["service"]}}, "starts_at": "2030-01-07T11:00:00+03:00"}, reject},
		{"POST /v1/tenant/reservations", map[string]any{"branch_uuid": orgA.ids["branch"], "customer_uuid": orgA.ids["customer"],
			"lines": []map[string]any{{"service_uuid": orgB.ids["service"]}}, "starts_at": "2030-01-07T11:00:00+03:00"}, reject},
		{"POST /v1/tenant/reservations", map[string]any{"branch_uuid": orgA.ids["branch"], "customer_uuid": orgA.ids["customer"],
			"lines": []map[string]any{{"service_uuid": orgA.ids["service"], "staff_uuid": orgB.ids["staff"]}}, "starts_at": "2030-01-07T11:00:00+03:00"}, reject},
		{"POST /v1/tenant/availability/search", map[string]any{"branch_uuid": orgA.ids["branch"], "lines": []map[string]any{{"service_uuid": orgB.ids["service"]}}}, reject},
		{"POST /v1/tenant/availability/search", map[string]any{"branch_uuid": orgA.ids["branch"], "lines": []map[string]any{
			{"service_uuid": orgA.ids["service"], "staff_uuid": orgB.ids["staff"]}}}, reject},
	}
	for _, c := range cases {
		method, path, _ := strings.Cut(c.route, " ")
		r := call(orgA, method, path, c.body)
		if l := leaked(r.raw); l != "" {
			t.Errorf("%s leaked %q: %s", c.route, l, r.raw)
		}
		if c.want == nil { // bulk: refused, or run with nothing succeeding
			summary, _ := asMap(r.data)["summary"].(map[string]any)
			if r.code != http.StatusBadRequest && (r.code != http.StatusOK || summary == nil || summary["succeeded"].(float64) != 0) {
				t.Errorf("%s with B's ids = %d, want 400 or 200 with 0 succeeded: %s", c.route, r.code, r.raw)
			}
		} else if !containsInt(c.want, r.code) {
			t.Errorf("%s naming B's records = %d, want one of %v: %s", c.route, r.code, c.want, r.raw)
		}
		covered[c.route] = "body names B's records"
	}
	// Mutations on A's own records whose body names B's records.
	ownCases := []bodyCase{
		{"PATCH /v1/tenant/members/" + alice.Uuid.String(), map[string]any{"role_uuids": []string{orgB.ids["role"]}}, reject},
		{"PATCH /v1/tenant/members/" + alice.Uuid.String(), map[string]any{"branch_scope": "selected", "branch_uuids": []string{orgB.ids["home"]}}, reject},
		{"PATCH /v1/tenant/staff/" + orgA.ids["staff"], map[string]any{"branch_uuids": []string{orgB.ids["home"]}}, reject},
		{"PATCH /v1/tenant/staff/" + orgA.ids["staff"], map[string]any{"account_uuid": orgB.ids["member"]}, reject},
		{"PATCH /v1/tenant/resources/" + orgA.ids["resource"], map[string]any{"branch_uuid": orgB.ids["home"]}, reject},
		{"PATCH /v1/tenant/cash/accounts/" + orgA.ids["cashAccount"], map[string]any{"branch_uuid": orgB.ids["home"]}, reject},
		{"PATCH /v1/tenant/services/" + orgA.ids["service"], map[string]any{"category_uuid": orgB.ids["serviceCategory"]}, reject},
		{"PUT /v1/tenant/services/" + orgA.ids["service"] + "/offers", map[string]any{"offers": []map[string]any{{"branch_uuid": orgB.ids["home"]}}}, reject},
		{"PUT /v1/tenant/services/" + orgA.ids["service"] + "/staff", map[string]any{"staff": []map[string]any{{"staff_uuid": orgB.ids["staff"]}}}, reject},
		{"PUT /v1/tenant/products/" + orgA.ids["product"] + "/offers", map[string]any{"offers": []map[string]any{{"branch_uuid": orgB.ids["home"]}}}, reject},
		{"POST /v1/tenant/reservations/" + orgA.ids["reservation"] + "/reschedule", map[string]any{"starts_at": "2030-01-07T11:00:00+03:00",
			"lines": []map[string]any{{"service_uuid": orgB.ids["service"]}}}, reject},
		{"POST /v1/tenant/reservations/" + orgA.ids["reservation"] + "/reschedule", map[string]any{"starts_at": "2030-01-07T11:00:00+03:00",
			"lines": []map[string]any{{"service_uuid": orgA.ids["service"], "staff_uuid": orgB.ids["staff"]}}}, reject},
	}
	for _, c := range ownCases {
		method, path, _ := strings.Cut(c.route, " ")
		r := call(orgA, method, path, c.body)
		if l := leaked(r.raw); l != "" {
			t.Errorf("%s leaked %q: %s", c.route, l, r.raw)
		}
		if !containsInt(c.want, r.code) {
			t.Errorf("%s naming B's records = %d, want one of %v: %s", c.route, r.code, c.want, r.raw)
		}
	}

	// 5. Exports of A contain A's rows only (the worker context is scoped
	// to the job's organization).
	for _, rt := range routes {
		method, path, _ := strings.Cut(rt, " ")
		if method != http.MethodPost || !strings.HasSuffix(path, "/export") {
			continue
		}
		r := call(orgA, http.MethodPost, path, map[string]any{"format": "csv"})
		if r.code != http.StatusAccepted || str(r, "status") != "completed" {
			t.Errorf("%s in A = %d: %s", rt, r.code, r.raw)
			continue
		}
		file := do(alice, orgA.org.Slug, http.MethodGet, "/v1/tenant/exports/"+str(r, "uuid")+"/download", "", nil)
		if file.code != http.StatusOK {
			t.Errorf("download %s = %d: %s", rt, file.code, file.raw)
			continue
		}
		if l := leaked(file.raw); l != "" {
			t.Errorf("%s: A's export file contains %q", rt, l)
		}
		if bytes.Count(file.raw, []byte("\n")) < 1 {
			t.Errorf("%s: empty export", rt)
		}
		covered[rt] = "export file scan"
	}
	// A's own export download stays A's (B cannot read it either).
	if r := do(bob, orgB.org.Slug, http.MethodGet, "/v1/tenant/exports/"+orgA.ids["exportJob"]+"/download", "", nil); r.code != http.StatusNotFound {
		t.Errorf("B downloads A's export = %d, want 404", r.code)
	}

	// Public service covers are bound to their organization: B's image
	// under A's organization uuid is not served.
	bImage := orgB.ids["serviceImageURL"][strings.LastIndex(orgB.ids["serviceImageURL"], "/")+1:]
	if r := do(alice, "", http.MethodGet, "/v1/public/media/services/"+orgA.org.UUID.String()+"/images/"+bImage, "", nil); r.code != http.StatusNotFound {
		t.Errorf("B's service cover under A's organization = %d, want 404", r.code)
	}
	if r := do(bob, "", http.MethodGet, orgB.ids["serviceImageURL"], "", nil); r.code != http.StatusOK {
		t.Errorf("B's own service cover = %d, want 200: %s", r.code, r.raw)
	}

	// B is unchanged by everything A did.
	after := snapshot()
	for path, want := range before {
		if after[path] != want {
			t.Errorf("B's %s changed:\n before %s\n after  %s", path, want, after[path])
		}
	}

	// Engine files live under the organization prefix.
	keyOf := func(table, id string) string {
		t.Helper()
		var key string
		if err := pool.QueryRow(ctx, "SELECT file_key FROM "+table+" WHERE uuid = $1", id).Scan(&key); err != nil {
			t.Fatal(err)
		}
		return key
	}
	for _, c := range []struct{ table, id, kind string }{
		{"export_jobs", orgB.ids["exportJob"], "exports"}, {"import_jobs", orgB.ids["importJob"], "imports"},
	} {
		if key := keyOf(c.table, c.id); !strings.HasPrefix(key, storage.OrganizationPrefix(orgB.org.UUID.String())+c.kind+"/") {
			t.Errorf("%s file key %q is not under organizations/{B}/%s/", c.table, key, c.kind)
		}
	}
	// Defence in depth: a row pointing at another organization's object is
	// not streamed (export download).
	aExportKey := keyOf("export_jobs", orgA.ids["exportJob"])
	bExportKey := keyOf("export_jobs", orgB.ids["exportJob"])
	if _, err := pool.Exec(ctx, "UPDATE export_jobs SET file_key = $1 WHERE uuid = $2", aExportKey, orgB.ids["exportJob"]); err != nil {
		t.Fatal(err)
	}
	if r := do(bob, orgB.org.Slug, http.MethodGet, "/v1/tenant/exports/"+orgB.ids["exportJob"]+"/download", "", nil); r.code != http.StatusNotFound {
		t.Errorf("export row pointing at A's file = %d, want 404", r.code)
	}
	if _, err := pool.Exec(ctx, "UPDATE export_jobs SET file_key = $1 WHERE uuid = $2", bExportKey, orgB.ids["exportJob"]); err != nil {
		t.Fatal(err)
	}

	// 6. Search documents (Meilisearch only).
	if cfg.Search.Enabled {
		deadline := time.Now().Add(10 * time.Second)
		for {
			r := call(orgB, http.MethodGet, "/v1/tenant/search?q="+orgB.marker, nil)
			if strings.Contains(string(r.raw), orgB.ids["branch"]) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("B's documents never indexed: %s", r.raw)
			}
			time.Sleep(200 * time.Millisecond)
		}
		if r := call(orgA, http.MethodGet, "/v1/tenant/search?q="+orgB.marker, nil); leaked(r.raw) != "" {
			t.Errorf("A's search returns B's documents: %s", r.raw)
		}
	} else {
		t.Log("search: TEST_MEILI_HOST not set, Meilisearch part skipped")
	}

	// Coverage: every tenant route has an isolation case or a reason.
	for _, rt := range routes {
		if covered[rt] != "" {
			continue
		}
		if reason := isolationAllowlist[rt]; reason != "" {
			continue
		}
		t.Errorf("uncovered route %s: add an isolation case (body naming another organization's records) or an allowlist entry with the reason", rt)
	}
	for rt, reason := range isolationAllowlist {
		if reason == "" {
			continue
		}
		if !containsString(routes, rt) {
			t.Errorf("isolationAllowlist: %s is not a registered route", rt)
		}
	}
}

// paramKey maps a path parameter (with the segment before it) to the seed
// key of the record it names. "" = unknown: the route needs a case.
func paramKey(prev, name string) string {
	switch prev + "/" + name {
	case "branches/uuid":
		return "branch"
	case "service-points/pointUuid":
		return "point"
	case "images/imageUuid":
		return "branchImage"
	case "invitations/uuid":
		return "invitation"
	case "roles/uuid":
		return "role"
	case "members/userUuid":
		return "member"
	case "exports/uuid":
		return "exportJob"
	case "imports/uuid":
		return "importJob"
	case "bulk/uuid":
		return "bulkJob"
	case "versions/version":
		return "version"
	case "integrations/uuid":
		return "integrationAccount"
	case "integration-jobs/uuid":
		return "integrationJob"
	case "staff/uuid":
		return "staff"
	case "branches/branchUuid":
		return "branch"
	case "time-off/timeOffUuid":
		return "timeOff"
	case "resources/uuid":
		return "resource"
	case "closures/closureUuid":
		return "closure"
	case "services/uuid":
		return "service"
	case "service-categories/uuid":
		return "serviceCategory"
	case "customers/uuid":
		return "customer"
	case "products/uuid":
		return "product"
	case "variants/variantUuid":
		return "variant"
	case "holds/uuid":
		return "hold"
	case "reservations/uuid":
		return "reservation"
	case "accounts/uuid":
		return "cashAccount"
	case "closings/closingUuid":
		return "cashClosing"
	case "movements/uuid":
		return "cashMovement"
	case "lines/uuid":
		return "saleLine"
	case "entries/uuid":
		return "ledgerEntry"
	}
	return ""
}

var tenantRouteRE = regexp.MustCompile(`^(GET|POST|PUT|PATCH|DELETE) /v1/tenant(/\S*)?$`)

// tenantRoutes enumerates the /v1/tenant routes: every route pattern
// literal in the backend sources, each checked to be registered on mux, plus
// a cross-check that every /v1/tenant operation of openapi.yaml resolves to
// one of them (a pattern built at runtime would show up there).
func tenantRoutes(t *testing.T, mux *http.ServeMux) []string {
	t.Helper()
	root := filepath.Join("..", "..")
	set := map[string]bool{}
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if s, err := strconv.Unquote(lit.Value); err == nil && tenantRouteRE.MatchString(s) {
					set[s] = true
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	routes := make([]string, 0, len(set))
	for rt := range set {
		method, path, _ := strings.Cut(rt, " ")
		concrete := regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(path, "00000000-0000-0000-0000-000000000000")
		req := httptest.NewRequest(method, concrete, nil)
		if _, pattern := mux.Handler(req); pattern != rt {
			t.Errorf("route literal %q is not registered on the server (mux pattern %q)", rt, pattern)
			continue
		}
		routes = append(routes, rt)
	}
	sort.Strings(routes)
	if len(routes) < 40 {
		t.Fatalf("found only %d tenant routes", len(routes))
	}

	spec, err := os.ReadFile(filepath.Join(root, "docs", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	pathRE := regexp.MustCompile(`^  (/v1/tenant[^:]*):\s*$`)
	opRE := regexp.MustCompile(`^    (get|post|put|patch|delete):\s*$`)
	current := ""
	for _, line := range strings.Split(string(spec), "\n") {
		if m := pathRE.FindStringSubmatch(line); m != nil {
			current = m[1]
			continue
		}
		if strings.HasPrefix(line, "  /") || (len(line) > 0 && line[0] != ' ') {
			current = ""
			continue
		}
		if m := opRE.FindStringSubmatch(line); m != nil && current != "" {
			method := strings.ToUpper(m[1])
			concrete := regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(current, "00000000-0000-0000-0000-000000000000")
			_, pattern := mux.Handler(httptest.NewRequest(method, concrete, nil))
			if !containsString(routes, pattern) {
				t.Errorf("openapi %s %s resolves to %q, which is not an enumerated tenant route", method, current, pattern)
			}
		}
	}
	return routes
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func containsString(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// memStorage is an in-memory storage.Driver for the isolation test.
type memStorage struct {
	storage.Driver
	m *storage.Memory
}

func (d *memStorage) Ping(ctx context.Context) error { return d.m.Ping(ctx) }

func (d *memStorage) Upload(ctx context.Context, f storage.File, path string) error {
	return d.m.Upload(ctx, f, path)
}

func (d *memStorage) Download(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	return d.m.Download(ctx, path)
}

func (d *memStorage) Delete(ctx context.Context, path string) error { return d.m.Delete(ctx, path) }

func (d *memStorage) Exists(ctx context.Context, path string) (bool, error) {
	return d.m.Exists(ctx, path)
}

func (d *memStorage) Bucket() string { return "isolation" }
