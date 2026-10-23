package reservations_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilitymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations"
	reservationshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

type mapLoader map[uuid.UUID]int64

func (m mapLoader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, _ := uuid.Parse(c.Subject)
	return authctx.Principal{UserID: id, UserInternal: m[id]}, nil
}

// TestReschedule: moving a booking re-plans every line after letting its
// own capacity go (a move into its own time works), keeps the booked price
// of unchanged lines, refuses a taken time without changing anything, and
// keeps a waiting request's hold reason and expiry.
func TestReschedule(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	r, err := f.svc.Create(ctx, f.org.ID, f.input(at(9, 0), "", "", f.cut), none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(`UPDATE service_staff SET price = 400 WHERE organization_id = $1 AND service_id = $2`, f.org.ID, f.cutID)
	moved, err := f.svc.Reschedule(ctx, f.org.ID, r.UUID, none, f.owner.ID, model.RescheduleInput{StartsAt: ptr(at(9, 15)), Note: "müşteri istedi"})
	if err != nil {
		t.Fatal(err)
	}
	if !moved.StartsAt.Equal(at(9, 15)) || moved.Version != 2 || !moved.Lines[0].UnitPrice.Equal(money.MustParse("150")) ||
		moved.Status != model.StatusActive || moved.Events[1].Type != "rescheduled" || moved.Events[1].Note != "müşteri istedi" {
		t.Fatalf("moved = %+v", moved)
	}
	if kind, status, _ := f.hold(t, moved); kind != availabilitymodel.KindBooking || status != availabilitymodel.StatusActive {
		t.Fatalf("moved hold = %s %s", kind, status)
	}
	if n := f.count("SELECT count(*) FROM capacity_holds WHERE organization_id = $1 AND status = 'active'", f.org.ID); n != 1 {
		t.Fatalf("active holds = %d", n)
	}
	// A new service takes today's price; the total follows.
	moved, err = f.svc.Reschedule(ctx, f.org.ID, r.UUID, none, f.owner.ID, model.RescheduleInput{
		StartsAt: ptr(at(9, 15)), Lines: []availabilitymodel.LineInput{{ServiceUUID: f.cut}, {ServiceUUID: f.wash}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(moved.Lines) != 2 || !moved.TotalPrice.Equal(money.MustParse("230")) || !moved.EndsAt.Equal(at(10, 15)) {
		t.Fatalf("with wash = %+v", moved)
	}

	// A taken time changes nothing.
	other, err := f.svc.Create(ctx, f.org.ID, f.input(at(11, 0), "", "", f.cut), none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Reschedule(ctx, f.org.ID, r.UUID, none, f.owner.ID, model.RescheduleInput{StartsAt: ptr(at(11, 0))}); !errors.Is(err, availabilityusecase.ErrSlotTaken) {
		t.Fatalf("into a taken time = %v", err)
	}
	still, _ := f.svc.Get(ctx, f.org.ID, r.UUID, none)
	if !still.StartsAt.Equal(at(9, 15)) || still.Version != 3 || len(still.Lines) != 2 {
		t.Fatalf("after a refused move = %+v", still)
	}
	if _, status, _ := f.hold(t, still); status != availabilitymodel.StatusActive {
		t.Fatalf("hold after a refused move = %s", status)
	}
	stale := 1
	if _, err := f.svc.Reschedule(ctx, f.org.ID, r.UUID, none, f.owner.ID, model.RescheduleInput{StartsAt: ptr(at(10, 30)), ExpectedVersion: &stale}); !errors.Is(err, usecase.ErrStaleVersion) {
		t.Fatalf("stale move = %v", err)
	}
	if _, err := f.svc.Complete(ctx, f.org.ID, other.UUID, none, f.owner.ID, usecase.ChangeInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Reschedule(ctx, f.org.ID, other.UUID, none, f.owner.ID, model.RescheduleInput{StartsAt: ptr(at(11, 30))}); !errors.Is(err, usecase.ErrInvalidTransition) {
		t.Fatalf("move a completed visit = %v", err)
	}

	// A request keeps waiting with the same expiry.
	f.saveSettings(t, "staff")
	req, err := f.svc.Create(ctx, f.org.ID, f.input(at(10, 30), model.SourceOnline, "", f.cut), none, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.now = at(8, 10)
	req, err = f.svc.Reschedule(ctx, f.org.ID, req.UUID, none, f.owner.ID, model.RescheduleInput{StartsAt: ptr(at(11, 30))})
	if err != nil {
		t.Fatal(err)
	}
	if kind, _, reason := f.hold(t, req); kind != availabilitymodel.KindHold || reason != availabilitymodel.ReasonApproval || !req.HoldExpiresAt.Equal(at(8, 30)) ||
		req.Status != model.StatusRequested {
		t.Fatalf("moved request = %s %s %v %s", kind, reason, req.HoldExpiresAt, req.Status)
	}

	// Notes and form answers.
	v := req.Version
	req, err = f.svc.UpdateNotes(ctx, f.org.ID, req.UUID, none, f.owner.ID, model.NotesInput{InternalNote: "aradık", CustomFields: map[string]any{"kat": 2}, ExpectedVersion: &v})
	if err != nil || req.InternalNote != "aradık" || req.CustomFields["kat"] != float64(2) || req.Version != v+1 {
		t.Fatalf("notes = %+v, %v", req, err)
	}
}

func ptr[T any](v T) *T { return &v }

type result struct {
	code    int
	data    map[string]any
	details []any
	raw     string
}

// TestReservationsOverHTTP: the panel routes, list filters and facets, the
// own calendar member, and the 409 details.
func TestReservationsOverHTTP(t *testing.T) {
	f := newFixture(t)
	orgSvc := orgusecase.New(orgrepository.NewPostgres(f.pool, f.q), branchusecase.NewOrganizationHooks(nil))
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	loader := mapLoader{f.owner.Uuid: f.owner.ID, f.emre.Uuid: f.emre.ID, f.fatma.Uuid: f.fatma.ID}
	mux := http.NewServeMux()
	reservations.RegisterRoutes(mux, reservationshandler.New(f.svc, nil), tokens, loader, orgSvc)
	call := func(user db.User, method, path string, body any) result {
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
		req.Header.Set(orgctx.HeaderName, f.org.Slug)
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
	want := func(r result, code int, step string) result {
		t.Helper()
		if r.code != code {
			t.Fatalf("%s = %d, want %d: %s", step, r.code, code, r.raw)
		}
		return r
	}
	detail := func(r result) string {
		if len(r.details) == 0 {
			return ""
		}
		d := r.details[0].(map[string]any)
		return d["field"].(string) + "/" + d["code"].(string)
	}
	items := func(r result) []map[string]any {
		var out []map[string]any
		for _, it := range r.data["items"].([]any) {
			out = append(out, it.(map[string]any))
		}
		return out
	}
	body := func(start time.Time) map[string]any {
		return map[string]any{"branch_uuid": f.branch, "customer_uuid": f.customer, "lines": []map[string]any{{"service_uuid": f.cut}}, "starts_at": start}
	}

	created := want(call(f.owner, http.MethodPost, "/v1/tenant/reservations", body(at(9, 0))), http.StatusCreated, "create")
	id := created.data["uuid"].(string)
	if created.data["status"] != "active" || created.data["total_price"] != "150.00" || created.data["source"] != "panel" {
		t.Fatalf("created = %v", created.data)
	}
	if r := want(call(f.owner, http.MethodPost, "/v1/tenant/reservations", body(at(9, 0))), http.StatusConflict, "taken"); detail(r) != "starts_at/taken" {
		t.Fatalf("detail = %s", detail(r))
	}
	bad := body(at(10, 0))
	bad["customer_uuid"] = uuid.New()
	if r := want(call(f.owner, http.MethodPost, "/v1/tenant/reservations", bad), http.StatusBadRequest, "unknown customer"); detail(r) != "customer_uuid/not_found" {
		t.Fatalf("detail = %s", detail(r))
	}
	bad = body(at(10, 0))
	bad["branch_uuid"] = uuid.New()
	if r := want(call(f.owner, http.MethodPost, "/v1/tenant/reservations", bad), http.StatusBadRequest, "unknown branch"); detail(r) != "branch_uuid/not_found" {
		t.Fatalf("detail = %s", detail(r))
	}
	want(call(f.owner, http.MethodGet, "/v1/tenant/reservations/"+id, nil), http.StatusOK, "get")
	want(call(f.owner, http.MethodGet, "/v1/tenant/reservations/"+uuid.NewString(), nil), http.StatusNotFound, "unknown")
	want(call(f.owner, http.MethodGet, "/v1/tenant/reservations/meta", nil), http.StatusOK, "meta")

	// Actions: an empty body is fine; a wrong state and a stale version are
	// 409 with details.
	want(call(f.owner, http.MethodPost, "/v1/tenant/reservations/"+id+"/start", nil), http.StatusOK, "start")
	if r := want(call(f.owner, http.MethodPost, "/v1/tenant/reservations/"+id+"/cancel", map[string]any{}), http.StatusConflict, "cancel in service"); detail(r) != "status/invalid_transition" {
		t.Fatalf("detail = %s", detail(r))
	}
	if r := want(call(f.owner, http.MethodPost, "/v1/tenant/reservations/"+id+"/complete", map[string]any{"expected_version": 1}), http.StatusConflict, "stale"); detail(r) != "expected_version/stale" {
		t.Fatalf("detail = %s", detail(r))
	}
	done := want(call(f.owner, http.MethodPost, "/v1/tenant/reservations/"+id+"/complete", map[string]any{"expected_version": 2, "note": "ok"}), http.StatusOK, "complete")
	if done.data["status"] != "completed" {
		t.Fatalf("completed = %v", done.data)
	}

	// Emre (Ayşe's account, member role = own calendar) books his own time
	// and sees only what he serves or created; fatma has no staff record.
	mine := want(call(f.emre, http.MethodPost, "/v1/tenant/reservations", body(at(10, 0))), http.StatusCreated, "own create")
	mineID := mine.data["uuid"].(string)
	moved := want(call(f.emre, http.MethodPost, "/v1/tenant/reservations/"+mineID+"/reschedule", map[string]any{"starts_at": at(10, 30)}), http.StatusOK, "own move")
	if moved.data["version"] != float64(2) {
		t.Fatalf("moved = %v", moved.data)
	}
	if r := want(call(f.emre, http.MethodGet, "/v1/tenant/reservations", nil), http.StatusOK, "own list"); len(items(r)) != 2 {
		t.Fatalf("own list = %d", len(items(r)))
	}
	if r := want(call(f.fatma, http.MethodGet, "/v1/tenant/reservations", nil), http.StatusOK, "fatma list"); len(items(r)) != 0 {
		t.Fatalf("fatma list = %d", len(items(r)))
	}
	want(call(f.fatma, http.MethodGet, "/v1/tenant/reservations/"+mineID, nil), http.StatusNotFound, "fatma get")
	want(call(f.fatma, http.MethodPost, "/v1/tenant/reservations/"+mineID+"/cancel", nil), http.StatusNotFound, "fatma cancel")
	want(call(f.fatma, http.MethodPost, "/v1/tenant/reservations", body(at(11, 0))), http.StatusForbidden, "fatma create")
	want(call(f.owner, http.MethodPatch, "/v1/tenant/reservations/"+mineID, map[string]any{"internal_note": "x"}), http.StatusOK, "notes")
	want(call(f.owner, http.MethodPost, "/v1/tenant/reservations/"+mineID+"/cancel", map[string]any{"reason": "customer_request"}), http.StatusOK, "cancel")

	// List filters, search and facets.
	list := func(query string) []map[string]any {
		t.Helper()
		return items(want(call(f.owner, http.MethodGet, "/v1/tenant/reservations"+query, nil), http.StatusOK, "list "+query))
	}
	if all := list(""); len(all) != 2 || all[0]["uuid"] != mineID || all[0]["services"] != "Kesim" || all[0]["staff"] != "Ayşe" {
		t.Fatalf("list = %v", all)
	}
	if got := list("?status=cancelled"); len(got) != 1 || got[0]["uuid"] != mineID {
		t.Fatalf("status filter = %v", got)
	}
	if got := list("?q=%231"); len(got) != 1 || got[0]["uuid"] != id {
		t.Fatalf("number search = %v", got)
	}
	if got := list("?q=zeynep&sort=starts_at"); len(got) != 2 || got[0]["uuid"] != id {
		t.Fatalf("name search = %v", got)
	}
	window := "?starts_from=" + url.QueryEscape(at(10, 0).Format(time.RFC3339)) + "&starts_to=" + url.QueryEscape(at(12, 0).Format(time.RFC3339))
	if got := list(window); len(got) != 1 || got[0]["uuid"] != mineID {
		t.Fatalf("window = %v", got)
	}
	if got := list("?branch=" + uuid.NewString()); len(got) != 0 {
		t.Fatalf("unknown branch = %v", got)
	}
	want(call(f.owner, http.MethodGet, "/v1/tenant/reservations?status=lost", nil), http.StatusBadRequest, "bad status")
	want(call(f.owner, http.MethodGet, "/v1/tenant/reservations?sort=price", nil), http.StatusBadRequest, "bad sort")
	facets := want(call(f.owner, http.MethodGet, "/v1/tenant/reservations/facets", nil), http.StatusOK, "facets")
	if staff := facets.data["staff"].([]any); len(staff) != 1 || staff[0].(map[string]any)["label"] != "Ayşe" {
		t.Fatalf("facets = %v", facets.data)
	}
}
