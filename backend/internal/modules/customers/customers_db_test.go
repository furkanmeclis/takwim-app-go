package customers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers"
	customersadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/adapters"
	customershandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/handler"
	customersmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	customersrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/repository"
	customersusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

type mapLoader map[uuid.UUID]int64

func (m mapLoader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, _ := uuid.Parse(c.Subject)
	return authctx.Principal{UserID: id, UserInternal: m[id]}, nil
}

type result struct {
	code    int
	data    map[string]any
	details []any
	raw     string
}

// TestCustomersOverHTTP drives customers (TEC-896) over HTTP with the real
// resolver and SQL, across two businesses that know the same phone:
// validation and E.164 normalization, one record per phone per business,
// no cross-business reach (A23), verified links that never leak another
// business's record or notes (A26), and unverified / imported phones that
// never link (A30). Also the import match, bulk tag / delete with rollback
// and the activity trail.
// Runs when TEST_DATABASE_URL points at a migrated database.
func TestCustomersOverHTTP(t *testing.T) {
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
	// A mobile number unlikely to clash with another run: 532 + 7 digits.
	phoneDigits := fmt.Sprintf("532%07d", time.Now().UnixNano()%10000000)
	localPhone := "0" + phoneDigits
	e164 := "+90" + phoneDigits

	newUser := func(name string) db.User {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: name + "-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	alice, bob, dave := newUser("alice"), newUser("bob"), newUser("dave")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	orgA, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Customers A " + suffix, OwnerUserID: alice.ID})
	if err != nil {
		t.Fatal(err)
	}
	orgB, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Customers B " + suffix, OwnerUserID: bob.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM activity_events WHERE organization_id = ANY($1)", []int64{orgA.ID, orgB.ID})
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{orgA.ID, orgB.ID})
		_, _ = pool.Exec(bg, "DELETE FROM customer_identities WHERE phone_e164 = $1", e164)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{alice.ID, bob.ID, dave.ID})
	})
	if _, err := orgSvc.AddMember(ctx, orgA.UUID, dave.Uuid, "resepsiyon"); err != nil {
		t.Fatal(err)
	}

	rec := activity.NewRecorder(q, nil)
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	loader := mapLoader{alice.Uuid: alice.ID, bob.Uuid: bob.ID, dave.Uuid: dave.ID}
	svc := customersusecase.New(customersrepository.NewPostgres(pool, q), nil)
	mux := http.NewServeMux()
	customers.RegisterRoutes(mux, customershandler.New(svc, rec), customers.EngineHandlers{}, tokens, loader, orgSvc)

	call := func(user db.User, slug, method, path string, body any) result {
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
		req.Header.Set(orgctx.HeaderName, slug)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		var env struct {
			Data  map[string]any `json:"data"`
			Error struct {
				Details []any `json:"details"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		return result{code: rr.Code, data: env.Data, details: env.Error.Details, raw: rr.Body.String()}
	}
	asA := func(method, path string, body any) result { return call(alice, orgA.Slug, method, path, body) }
	asB := func(method, path string, body any) result { return call(bob, orgB.Slug, method, path, body) }
	want := func(r result, code int, step string) result {
		t.Helper()
		if r.code != code {
			t.Fatalf("%s = %d, want %d: %s", step, r.code, code, r.raw)
		}
		return r
	}
	hasDetail := func(r result, field, code string) bool {
		for _, raw := range r.details {
			d := raw.(map[string]any)
			if d["field"] == field && (code == "" || d["code"] == code) {
				return true
			}
		}
		return false
	}
	total := func(r result) int {
		return int(r.data["total"].(float64))
	}

	// Create: the phone is stored as E.164, tags are trimmed and deduplicated
	// case-insensitively, consent gets its timestamp; nothing is verified.
	created := want(asA(http.MethodPost, "/v1/tenant/customers", map[string]any{
		"first_name": " Ayşe ", "last_name": "Yılmaz", "phone": localPhone, "email": "Ayse@Example.com",
		"birth_date": "1990-04-12", "tags": []string{"VIP", " vip ", "düzenli", ""}, "sms_consent": true,
		"notes": "A işletmesinin notu",
	}), http.StatusCreated, "create A")
	aUUID := created.data["uuid"].(string)
	if created.data["phone"] != e164 || created.data["first_name"] != "Ayşe" || created.data["email"] != "ayse@example.com" {
		t.Fatalf("create normalized = %v", created.data)
	}
	if tags := created.data["tags"].([]any); len(tags) != 2 || tags[0] != "VIP" || tags[1] != "düzenli" {
		t.Fatalf("tags = %v", tags)
	}
	if created.data["consent_updated_at"] == nil || created.data["verification"] != nil || created.data["source"] != "panel" {
		t.Fatalf("consent / verification / source = %v", created.data)
	}

	// Validation.
	bad := want(asA(http.MethodPost, "/v1/tenant/customers", map[string]any{
		"phone": "12", "email": "not-an-email", "birth_date": "2999-01-01",
	}), http.StatusBadRequest, "invalid create")
	for _, field := range []string{"first_name", "phone", "email", "birth_date"} {
		if !hasDetail(bad, field, "") {
			t.Fatalf("missing %s detail: %s", field, bad.raw)
		}
	}
	// One live record per phone in a business, whatever the spelling.
	dup := want(asA(http.MethodPost, "/v1/tenant/customers", map[string]any{
		"first_name": "Başka", "phone": "+90 " + phoneDigits[:3] + " " + phoneDigits[3:],
	}), http.StatusConflict, "duplicate phone")
	if !hasDetail(dup, "phone", "taken") {
		t.Fatalf("duplicate detail: %s", dup.raw)
	}

	// Business B keeps its own record of the same person.
	bRec := want(asB(http.MethodPost, "/v1/tenant/customers", map[string]any{
		"first_name": "Ayşe", "phone": e164, "notes": "B işletmesinin notu",
	}), http.StatusCreated, "create B")
	bUUID := bRec.data["uuid"].(string)

	// A23: no business reaches another business's record by uuid.
	want(asB(http.MethodGet, "/v1/tenant/customers/"+aUUID, nil), http.StatusNotFound, "B reads A")
	want(asB(http.MethodPatch, "/v1/tenant/customers/"+aUUID, map[string]any{"notes": "x"}), http.StatusNotFound, "B patches A")
	want(asB(http.MethodDelete, "/v1/tenant/customers/"+aUUID, nil), http.StatusNotFound, "B deletes A")
	want(asB(http.MethodGet, "/v1/tenant/customers/"+aUUID+"/activity", nil), http.StatusNotFound, "B reads A activity")
	if got := total(want(asB(http.MethodGet, "/v1/tenant/customers?q=Ay%C5%9Fe", nil), http.StatusOK, "B list")); got != 1 {
		t.Fatalf("B sees %d customers, want 1", got)
	}

	// Reception works with customers.
	want(call(dave, orgA.Slug, http.MethodPatch, "/v1/tenant/customers/"+aUUID, map[string]any{"notes": "Kısa kesim"}),
		http.StatusOK, "reception patch")

	// List search by name, by phone fragment (with the national 0), filters.
	for _, qs := range []string{"q=ay%C5%9Fe+y", "q=" + url.QueryEscape(localPhone[:5]+" "+localPhone[5:8]), "tag=vip", "tag=VIP", "verified=false", "sms_consent=true"} {
		r := want(asA(http.MethodGet, "/v1/tenant/customers?"+qs, nil), http.StatusOK, "list "+qs)
		if qs == "tag=vip" {
			if total(r) != 0 {
				t.Fatalf("tag filter is exact: %s", r.raw)
			}
			continue
		}
		if total(r) != 1 {
			t.Fatalf("list %s = %d, want 1", qs, total(r))
		}
	}
	facets := want(asA(http.MethodGet, "/v1/tenant/customers/facets", nil), http.StatusOK, "facets")
	if tags := facets.data["tag"].([]any); len(tags) != 2 {
		t.Fatalf("tag facets = %v", tags)
	}

	// A30: a typed or imported phone match links nothing, so B's lookup of
	// the person finds no record.
	scopedA := orgctx.WithOrganizationID(ctx, orgA.ID)
	scopedB := orgctx.WithOrganizationID(ctx, orgB.ID)
	if _, err := svc.LinkedCustomer(scopedA, orgA.ID, e164); !errors.Is(err, customersusecase.ErrNotFound) {
		t.Fatalf("unverified lookup = %v, want not found", err)
	}

	// The person proves the phone in business A only.
	aID, bID := uuid.MustParse(aUUID), uuid.MustParse(bUUID)
	if _, err := svc.LinkVerified(scopedA, orgA.ID, aID, "+90 "+phoneDigits, "otp", time.Now()); err != nil {
		t.Fatal(err)
	}
	if v := want(asA(http.MethodGet, "/v1/tenant/customers/"+aUUID, nil), http.StatusOK, "A after link").data["verification"]; v == nil {
		t.Fatal("A record is not verified after the link")
	}
	if v := want(asB(http.MethodGet, "/v1/tenant/customers/"+bUUID, nil), http.StatusOK, "B after A link").data["verification"]; v != nil {
		t.Fatal("A's verification leaked to B")
	}

	// TEC-991: a release hid the person's history at A; the reception
	// reopens it (write permission), once, with the trail; B cannot reach it.
	restorePath := "/v1/tenant/customers/" + aUUID + "/history/restore"
	if !hasDetail(want(asA(http.MethodPost, restorePath, nil), http.StatusConflict, "restore with open history"), "history", "open") {
		t.Fatal("open history conflict has no detail")
	}
	if _, err := pool.Exec(ctx, "UPDATE customer_identity_links SET visible_from = NOW(), history_requested_at = NOW() WHERE organization_id = $1", orgA.ID); err != nil {
		t.Fatal(err)
	}
	hidden := want(asA(http.MethodGet, "/v1/tenant/customers/"+aUUID, nil), http.StatusOK, "hidden history").data["verification"].(map[string]any)
	if hidden["history_hidden_from"] == nil || hidden["history_requested_at"] == nil || hidden["history_restored_at"] != nil {
		t.Fatalf("hidden verification = %v", hidden)
	}
	want(asB(http.MethodPost, restorePath, nil), http.StatusNotFound, "B restores A")
	reopened := want(call(dave, orgA.Slug, http.MethodPost, restorePath, nil), http.StatusOK, "reception restores").data["verification"].(map[string]any)
	if reopened["history_hidden_from"] != nil || reopened["history_requested_at"] != nil || reopened["history_restored_at"] == nil || reopened["history_restored_by"] != "dave Test" {
		t.Fatalf("reopened verification = %v", reopened)
	}
	want(asA(http.MethodPost, restorePath, nil), http.StatusConflict, "restore twice")
	restoredAct := want(asA(http.MethodGet, "/v1/tenant/customers/"+aUUID+"/activity", nil), http.StatusOK, "restore activity")
	var sawRestore bool
	for _, raw := range restoredAct.data["items"].([]any) {
		if raw.(map[string]any)["action"] == "customers.history_restored" {
			sawRestore = true
		}
	}
	if !sawRestore {
		t.Fatalf("no customers.history_restored entry: %s", restoredAct.raw)
	}
	if _, err := svc.LinkedCustomer(scopedB, orgB.ID, e164); !errors.Is(err, customersusecase.ErrNotFound) {
		t.Fatalf("B lookup after A link = %v, want not found", err)
	}
	got, err := svc.LinkedCustomer(scopedA, orgA.ID, e164)
	if err != nil || got.UUID != aID {
		t.Fatalf("A lookup = %v, %v", got.UUID, err)
	}

	// A26: after both businesses link the same person, each still reaches
	// only its own record, and notes stay local.
	if _, err := svc.LinkVerified(scopedB, orgB.ID, bID, e164, "panel_login", time.Now()); err != nil {
		t.Fatal(err)
	}
	if got, err := svc.LinkedCustomer(scopedB, orgB.ID, e164); err != nil || got.UUID != bID {
		t.Fatalf("B lookup = %v, %v", got.UUID, err)
	}
	want(asA(http.MethodPatch, "/v1/tenant/customers/"+aUUID, map[string]any{"notes": "Yalnızca A görür"}), http.StatusOK, "A notes")
	if notes := want(asB(http.MethodGet, "/v1/tenant/customers/"+bUUID, nil), http.StatusOK, "B notes").data["notes"]; notes != "B işletmesinin notu" {
		t.Fatalf("B notes = %v", notes)
	}
	// A link needs the very phone on file.
	if _, err := svc.LinkVerified(scopedA, orgA.ID, aID, "+4930901820", "otp", time.Now()); !errors.Is(err, customersusecase.ErrPhoneMismatch) {
		t.Fatalf("mismatched link = %v", err)
	}

	// Import: the same phone updates the record (it stays verified); a new
	// phone creates an unverified "import" record.
	first, last := "Ayşe", "Kaya"
	_, isNew, prev, err := svc.ImportRow(scopedA, orgA.ID, customersmodel.Input{
		FirstName: &first, LastName: &last, Phone: customersmodel.OptionalString{Set: true, Value: &localPhone},
	})
	if err != nil || isNew || prev == nil || prev.LastName != "Yılmaz" {
		t.Fatalf("import update = %v, new=%v, prev=%v", err, isNew, prev)
	}
	if v := want(asA(http.MethodGet, "/v1/tenant/customers/"+aUUID, nil), http.StatusOK, "A after import").data["verification"]; v == nil {
		t.Fatal("import with the same phone dropped the verification")
	}
	other := "0533 000 00 01"
	importedID, isNew, _, err := svc.ImportRow(scopedA, orgA.ID, customersmodel.Input{
		FirstName: &first, Phone: customersmodel.OptionalString{Set: true, Value: &other},
	})
	if err != nil || !isNew {
		t.Fatalf("import create = %v, new=%v", err, isNew)
	}
	imported := want(asA(http.MethodGet, "/v1/tenant/customers/"+importedID.String(), nil), http.StatusOK, "imported")
	if imported.data["source"] != "import" || imported.data["verification"] != nil {
		t.Fatalf("imported = %v", imported.data)
	}

	// Changing the phone of a verified record drops the link.
	changed := want(asA(http.MethodPatch, "/v1/tenant/customers/"+aUUID, map[string]any{"phone": "0533 000 00 02"}), http.StatusOK, "change phone")
	if changed.data["verification"] != nil {
		t.Fatal("verification survived a phone change")
	}
	if _, err := svc.LinkedCustomer(scopedA, orgA.ID, e164); !errors.Is(err, customersusecase.ErrNotFound) {
		t.Fatalf("A lookup after phone change = %v", err)
	}
	// Null clears the phone.
	if p := want(asA(http.MethodPatch, "/v1/tenant/customers/"+importedID.String(), map[string]any{"phone": nil}), http.StatusOK, "clear phone").data["phone"]; p != nil {
		t.Fatalf("phone = %v, want null", p)
	}

	// Bulk: add a tag and roll it back; delete and roll it back.
	bulk := customersadapters.NewBulk(svc)
	tagCtx := bulkengine.WithParams(scopedA, map[string]string{"tag": "Kampanya"})
	if _, err := bulk.ResolveTargets(scopedA, "add_tag", bulkengine.BulkTarget{Scope: "ids", IDs: []string{aUUID}}); err == nil {
		t.Fatal("add_tag without a tag resolved")
	}
	res, err := bulk.ApplyItem(tagCtx, "add_tag", aUUID)
	if err != nil || !res.OK {
		t.Fatalf("add_tag = %+v, %v", res, err)
	}
	if tags := want(asA(http.MethodGet, "/v1/tenant/customers/"+aUUID, nil), http.StatusOK, "tagged").data["tags"].([]any); len(tags) != 3 {
		t.Fatalf("tags after add = %v", tags)
	}
	prevJSON, _ := json.Marshal(res.Previous)
	var prevMap map[string]any
	_ = json.Unmarshal(prevJSON, &prevMap)
	if err := bulk.RevertItem(scopedA, "add_tag", aUUID, prevMap); err != nil {
		t.Fatal(err)
	}
	if tags := want(asA(http.MethodGet, "/v1/tenant/customers/"+aUUID, nil), http.StatusOK, "untagged").data["tags"].([]any); len(tags) != 2 {
		t.Fatalf("tags after revert = %v", tags)
	}
	if res, err := bulk.ApplyItem(scopedA, "delete", importedID.String()); err != nil || !res.OK {
		t.Fatalf("bulk delete = %+v, %v", res, err)
	}
	want(asA(http.MethodGet, "/v1/tenant/customers/"+importedID.String(), nil), http.StatusNotFound, "bulk deleted")
	if err := bulk.RevertItem(scopedA, "delete", importedID.String(), nil); err != nil {
		t.Fatal(err)
	}
	want(asA(http.MethodGet, "/v1/tenant/customers/"+importedID.String(), nil), http.StatusOK, "restored")
	// B's bulk cannot touch A's record.
	if res, err := bulk.ApplyItem(scopedB, "delete", aUUID); err != nil || res.OK {
		t.Fatalf("cross-business bulk delete = %+v, %v", res, err)
	}

	// Activity and delete.
	act := want(asA(http.MethodGet, "/v1/tenant/customers/"+aUUID+"/activity", nil), http.StatusOK, "activity")
	if total(act) < 3 {
		t.Fatalf("activity entries = %d", total(act))
	}
	want(asA(http.MethodDelete, "/v1/tenant/customers/"+aUUID, nil), http.StatusOK, "delete")
	want(asA(http.MethodGet, "/v1/tenant/customers/"+aUUID, nil), http.StatusNotFound, "deleted")
	// The phone is free again in A.
	want(asA(http.MethodPost, "/v1/tenant/customers", map[string]any{"first_name": "Yeni", "phone": "0533 000 00 02"}),
		http.StatusCreated, "reuse phone")
}
