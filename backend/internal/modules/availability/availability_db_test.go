package availability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/engine"
	availabilityhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations"
	orghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/handler"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	orgsettingsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
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

// fakeBusy stands in for reservations and holds (TEC-898 / TEC-899).
type fakeBusy struct {
	staff map[int64][]engine.Interval
}

func (f *fakeBusy) Busy(_ context.Context, _ int64, _, _ []int64, _, _ time.Time) (map[int64][]engine.Interval, map[int64][]engine.Interval, error) {
	return f.staff, nil, nil
}

type result struct {
	code    int
	data    map[string]any
	details []any
	raw     string
}

// TestAvailabilityOverHTTP drives the slot engine (TEC-897) over HTTP with
// the real resolver and SQL: shifts and breaks, time off, special days,
// "any staff", personal durations, bundles with resources and closures, the
// one calendar of a person in two branches (A04), the online channel's
// booking window and visibility, the own calendar of a staff member,
// validation and isolation.
// Runs when TEST_DATABASE_URL points at a migrated database.
func TestAvailabilityOverHTTP(t *testing.T) {
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
	owner, carol, dave, emre, fatma, other := newUser("owner"), newUser("carol"), newUser("dave"), newUser("emre"), newUser("fatma"), newUser("other")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Availability " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	org2, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Availability other " + suffix, OwnerUserID: other.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		for _, id := range []int64{org.ID, org2.ID} {
			_, _ = pool.Exec(bg, "DELETE FROM activity_events WHERE organization_id = $1", id)
			_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", id)
		}
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{owner.ID, carol.ID, dave.ID, emre.ID, fatma.ID, other.ID})
	})
	for user, role := range map[db.User]string{carol: "sube_yoneticisi", dave: "resepsiyon", emre: "member", fatma: "member"} {
		if _, err := orgSvc.AddMember(ctx, org.UUID, user.Uuid, role); err != nil {
			t.Fatal(err)
		}
	}

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	idOf := func(sql string, args ...any) (int64, uuid.UUID) {
		t.Helper()
		var id int64
		var u uuid.UUID
		if err := pool.QueryRow(ctx, sql, args...).Scan(&id, &u); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return id, u
	}

	// Branches: A (default) and B, Monday–Saturday 09:00–18:00, A closed on
	// Tuesday 2030-01-08.
	hours := `{"mon":{"closed":false,"open":"09:00","close":"18:00"},"tue":{"closed":false,"open":"09:00","close":"18:00"},
		"wed":{"closed":false,"open":"09:00","close":"18:00"},"thu":{"closed":false,"open":"09:00","close":"18:00"},
		"fri":{"closed":false,"open":"09:00","close":"18:00"},"sat":{"closed":false,"open":"09:00","close":"18:00"},
		"sun":{"closed":true,"open":"","close":""}}`
	aID, aUUID := idOf("SELECT id, uuid FROM branches WHERE organization_id = $1 ORDER BY id LIMIT 1", org.ID)
	exec(`UPDATE branches SET working_hours = $3, timezone = NULL,
		holiday_exceptions = '[{"date":"2030-01-08","closed":true,"open":"","close":"","note":"Bayram"}]'
		WHERE organization_id = $1 AND id = $2`, org.ID, aID, hours)
	bID, bUUID := idOf(`INSERT INTO branches (organization_id, code, name, working_hours) VALUES ($1, 'B', 'Kadıköy', $2) RETURNING id, uuid`, org.ID, hours)
	_, foreignBranch := idOf("SELECT id, uuid FROM branches WHERE organization_id = $1 ORDER BY id LIMIT 1", org2.ID)

	// Ayşe works in both branches on Mondays (A 09–12 with a break, B
	// 13–17) and is emre's staff record; Burak works in A only, is hidden
	// online and needs an hour for a cut.
	ayseID, ayseUUID := idOf(`INSERT INTO staff (organization_id, name, user_id) VALUES ($1, 'Ayşe', $2) RETURNING id, uuid`, org.ID, emre.ID)
	burakID, burakUUID := idOf(`INSERT INTO staff (organization_id, name, show_online) VALUES ($1, 'Burak', FALSE) RETURNING id, uuid`, org.ID)
	exec(`INSERT INTO staff_branches (organization_id, staff_id, branch_id, shifts) VALUES
		($1, $2, $3, '{"mon":[{"start":"09:00","end":"12:00","breaks":[{"start":"10:00","end":"10:30"}]}]}'),
		($1, $2, $4, '{"mon":[{"start":"13:00","end":"17:00","breaks":[]}]}'),
		($1, $5, $3, '{"mon":[{"start":"09:00","end":"12:00","breaks":[]}]}')`, org.ID, ayseID, aID, bID, burakID)

	svcID := func(name string, duration int, staffMode string, kinds string) (int64, uuid.UUID) {
		return idOf(`INSERT INTO services (organization_id, name, duration_minutes, staff_mode, resource_kinds)
			VALUES ($1, $2, $3, $4, $5::TEXT[]) RETURNING id, uuid`, org.ID, name, duration, staffMode, kinds)
	}
	cutID, cutUUID := svcID("Kesim", 30, "required", "{}")
	_, colorUUID := svcID("Boya", 60, "none", "{room}")
	massageID, massageUUID := svcID("Masaj", 30, "required", "{}")
	colorID, _ := idOf("SELECT id, uuid FROM services WHERE organization_id = $1 AND uuid = $2", org.ID, colorUUID)
	exec(`INSERT INTO service_branches (organization_id, service_id, branch_id) VALUES ($1, $2, $3), ($1, $2, $4), ($1, $5, $3), ($1, $6, $3)`,
		org.ID, cutID, aID, bID, colorID, massageID)
	exec(`INSERT INTO service_staff (organization_id, service_id, staff_id, duration_minutes) VALUES ($1, $2, $3, NULL), ($1, $2, $4, 60)`,
		org.ID, cutID, ayseID, burakID)
	roomID, roomUUID := idOf(`INSERT INTO resources (organization_id, branch_id, kind, name) VALUES ($1, $2, 'room', 'Oda 1') RETURNING id, uuid`, org.ID, aID)
	exec(`INSERT INTO resource_closures (organization_id, resource_id, starts_at, ends_at) VALUES ($1, $2, '2030-01-07 09:00+03', '2030-01-07 10:00+03')`, org.ID, roomID)
	_, foreignService := idOf(`INSERT INTO services (organization_id, name, duration_minutes) VALUES ($1, 'Yabancı', 30) RETURNING id, uuid`, org2.ID)

	rec := activity.NewRecorder(q, nil)
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	loader := mapLoader{owner.Uuid: owner.ID, carol.Uuid: carol.ID, dave.Uuid: dave.ID, emre.Uuid: emre.ID, fatma.Uuid: fatma.ID, other.Uuid: other.ID}
	settings := orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))
	svc := availabilityusecase.New(q, settings)
	ist, _ := time.LoadLocation("Europe/Istanbul")
	now := time.Date(2030, 1, 7, 8, 0, 0, 0, ist) // Monday before opening
	svc.SetClock(func() time.Time { return now })
	busy := &fakeBusy{}
	svc.SetBusySource(busy)
	mux := http.NewServeMux()
	organizations.RegisterTenantRoutes(mux, orghandler.New(orgSvc, rec), tokens, loader, orgSvc)
	availability.RegisterRoutes(mux, availabilityhandler.New(svc), tokens, loader, orgSvc)

	send := func(user db.User, method, path string, body any) result {
		t.Helper()
		access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: user.Uuid})
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		_ = json.NewEncoder(&buf).Encode(body)
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Authorization", "Bearer "+access)
		if user.ID == other.ID {
			req.Header.Set(orgctx.HeaderName, org2.Slug)
		} else {
			req.Header.Set(orgctx.HeaderName, org.Slug)
		}
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
	call := func(user db.User, body any) result {
		t.Helper()
		return send(user, http.MethodPost, "/v1/tenant/availability/search", body)
	}
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
	days := func(r result) []map[string]any {
		var out []map[string]any
		for _, d := range r.data["days"].([]any) {
			out = append(out, d.(map[string]any))
		}
		return out
	}
	slotsOf := func(day map[string]any) []map[string]any {
		var out []map[string]any
		for _, s := range day["slots"].([]any) {
			out = append(out, s.(map[string]any))
		}
		return out
	}
	local := func(v any) string {
		ts, err := time.Parse(time.RFC3339, v.(string))
		if err != nil {
			t.Fatal(err)
		}
		return ts.In(ist).Format("15:04")
	}
	startsOf := func(day map[string]any) string {
		var parts []string
		for _, s := range slotsOf(day) {
			parts = append(parts, local(s["starts_at"]))
		}
		return strings.Join(parts, " ")
	}
	staffOf := func(slot map[string]any, line int) string {
		l := slot["lines"].([]any)[line].(map[string]any)
		if l["staff"] == nil {
			return ""
		}
		return l["staff"].(map[string]any)["uuid"].(string)
	}
	line := func(service uuid.UUID, staffUUID *uuid.UUID) map[string]any {
		m := map[string]any{"service_uuid": service}
		if staffUUID != nil {
			m["staff_uuid"] = staffUUID
		}
		return m
	}
	search := func(branch uuid.UUID, lines ...map[string]any) map[string]any {
		return map[string]any{"branch_uuid": branch, "lines": lines, "from": "2030-01-07", "to": "2030-01-07", "step_minutes": 30}
	}

	// "Fark etmez": Ayşe (break 10:00–10:30) and Burak (an hour) together.
	r := want(call(owner, search(aUUID, line(cutUUID, nil))), http.StatusOK, "any staff")
	monday := days(r)[0]
	if monday["status"] != "available" || startsOf(monday) != "09:00 09:30 10:00 10:30 11:00 11:30" {
		t.Fatalf("monday = %v %q", monday["status"], startsOf(monday))
	}
	for _, s := range slotsOf(monday) {
		start := local(s["starts_at"])
		if start == "10:00" && (staffOf(s, 0) != burakUUID.String() || local(s["ends_at"]) != "11:00") {
			t.Fatalf("10:00 = %v", s)
		}
		if start == "11:30" && staffOf(s, 0) != ayseUUID.String() {
			t.Fatalf("11:30 = %v", s)
		}
	}
	if lines := r.data["lines"].([]any); len(lines[0].(map[string]any)["staff"].([]any)) != 2 {
		t.Fatalf("lines = %v", lines)
	}
	if r.data["timezone"] != "Europe/Istanbul" || r.data["latest_date"] != nil {
		t.Fatalf("result = %v", r.data)
	}

	// A04: Ayşe's booking in branch B (11:00–12:00) is busy time in A.
	busy.staff = map[int64][]engine.Interval{ayseID: {{
		Start: time.Date(2030, 1, 7, 11, 0, 0, 0, ist), End: time.Date(2030, 1, 7, 12, 0, 0, 0, ist),
	}}}
	r = want(call(owner, search(aUUID, line(cutUUID, &ayseUUID))), http.StatusOK, "A04")
	if got := startsOf(days(r)[0]); got != "09:00 09:30 10:30" {
		t.Fatalf("A04 starts = %q", got)
	}
	busy.staff = nil

	// Time off blocks every branch of the person.
	exec(`INSERT INTO staff_time_off (organization_id, staff_id, starts_at, ends_at) VALUES ($1, $2, '2030-01-07 09:00+03', '2030-01-07 09:30+03')`, org.ID, ayseID)
	r = want(call(owner, search(aUUID, line(cutUUID, &ayseUUID))), http.StatusOK, "time off")
	if got := startsOf(days(r)[0]); got != "09:30 10:30 11:00 11:30" {
		t.Fatalf("time off starts = %q", got)
	}

	// The week: special day (Tuesday) closed, no shifts the rest of the
	// week, Sunday closed.
	week := search(aUUID, line(cutUUID, nil))
	week["to"] = "2030-01-13"
	r = want(call(owner, week), http.StatusOK, "week")
	var statuses []string
	for _, d := range days(r) {
		statuses = append(statuses, d["date"].(string)+"="+d["status"].(string))
	}
	if got := strings.Join(statuses, " "); got != "2030-01-07=available 2030-01-08=closed 2030-01-09=no_staff 2030-01-10=no_staff 2030-01-11=no_staff 2030-01-12=no_staff 2030-01-13=closed" {
		t.Fatalf("week = %s", got)
	}

	// Bundle: cut, then colour in the room (closed 09:00–10:00). At 09:00
	// only Burak's hour makes the colour start at 10:00.
	r = want(call(owner, search(aUUID, line(cutUUID, nil), line(colorUUID, nil))), http.StatusOK, "bundle")
	first := slotsOf(days(r)[0])[0]
	colour := first["lines"].([]any)[1].(map[string]any)
	if local(first["starts_at"]) != "09:00" || staffOf(first, 0) != burakUUID.String() || local(colour["starts_at"]) != "10:00" ||
		colour["staff"] != nil || colour["resources"].([]any)[0].(map[string]any)["uuid"] != roomUUID.String() || local(first["ends_at"]) != "11:00" {
		t.Fatalf("bundle first slot = %v", first)
	}

	// Online channel: refused until the booking window is configured.
	online := search(aUUID, line(cutUUID, nil))
	online["channel"] = "online"
	r = want(call(owner, online), http.StatusConflict, "online not ready")
	if !hasDetail(r, "channel", "online_not_ready") {
		t.Fatalf("details = %v", r.details)
	}
	lead, advance, window := 60, 3, 15
	s := orgsettingsmodel.Defaults()
	s.Reservation = orgsettingsmodel.Reservation{
		ApprovalMode: orgsettingsmodel.ApprovalAuto, MinLeadMinutes: &lead, MaxAdvanceDays: &advance, StaffReviewWindowMinutes: &window,
	}
	if _, err := settings.Save(ctx, org.ID, owner.ID, orgsettingsmodel.SaveInput{Settings: s}); err != nil {
		t.Fatal(err)
	}
	now = time.Date(2030, 1, 7, 9, 40, 0, 0, ist) // earliest start 10:40
	r = want(call(owner, online), http.StatusOK, "online")
	// Burak is not shown online; Ayşe's 10:30 is inside the notice.
	if got := startsOf(days(r)[0]); got != "11:00 11:30" {
		t.Fatalf("online starts = %q", got)
	}
	if r.data["latest_date"] != "2030-01-10" || local(r.data["earliest_start"]) != "10:40" {
		t.Fatalf("online window = %v / %v", r.data["latest_date"], r.data["earliest_start"])
	}
	online["to"] = "2030-01-11"
	r = want(call(owner, online), http.StatusOK, "online week")
	if d := days(r); d[len(d)-1]["status"] != "too_far" {
		t.Fatalf("2030-01-11 = %v", d[len(d)-1]["status"])
	}
	online["lines"] = []map[string]any{line(cutUUID, &burakUUID)}
	if r = want(call(owner, online), http.StatusBadRequest, "online hidden staff"); !hasDetail(r, "lines.0.staff_uuid", "not_found") {
		t.Fatalf("details = %v", r.details)
	}
	now = time.Date(2030, 1, 7, 8, 0, 0, 0, ist)

	// The panel never offers the past: at 10:40 the morning is gone.
	now = time.Date(2030, 1, 7, 10, 40, 0, 0, ist)
	r = want(call(owner, search(aUUID, line(cutUUID, &ayseUUID))), http.StatusOK, "past")
	if got := startsOf(days(r)[0]); got != "11:00 11:30" {
		t.Fatalf("past starts = %q", got)
	}
	now = time.Date(2030, 1, 7, 8, 0, 0, 0, ist)

	// Own calendar: emre (Çalışan, linked to Ayşe) sees Ayşe only.
	r = want(call(emre, search(aUUID, line(cutUUID, nil))), http.StatusOK, "own calendar")
	for _, s := range slotsOf(days(r)[0]) {
		if staffOf(s, 0) != ayseUUID.String() {
			t.Fatalf("own calendar slot = %v", s)
		}
	}
	want(call(emre, search(aUUID, line(cutUUID, &burakUUID))), http.StatusForbidden, "own calendar other person")
	want(call(fatma, search(aUUID, line(cutUUID, nil))), http.StatusForbidden, "member without staff record")
	want(call(dave, search(aUUID, line(cutUUID, nil))), http.StatusOK, "reception")

	// Branch scope: carol manages A only.
	want(send(owner, http.MethodPatch, "/v1/tenant/members/"+carol.Uuid.String(), map[string]any{
		"branch_scope": "selected", "branch_uuids": []string{aUUID.String()},
	}), http.StatusOK, "narrow carol")
	want(call(carol, search(aUUID, line(cutUUID, nil))), http.StatusOK, "carol A")
	want(call(carol, search(bUUID, line(cutUUID, nil))), http.StatusNotFound, "carol B")

	// Validation.
	r = want(call(owner, search(bUUID, line(massageUUID, nil), line(uuid.New(), nil), line(cutUUID, &uuid.UUID{}))), http.StatusBadRequest, "lines")
	for _, d := range [][2]string{{"lines.0.service_uuid", "not_offered"}, {"lines.1.service_uuid", "not_found"}, {"lines.2.staff_uuid", "not_found"}} {
		if !hasDetail(r, d[0], d[1]) {
			t.Fatalf("missing %v in %v", d, r.details)
		}
	}
	bad := search(aUUID, line(cutUUID, nil))
	bad["step_minutes"], bad["channel"], bad["to"] = 7, "fax", "2030-01-01"
	r = want(call(owner, bad), http.StatusBadRequest, "fields")
	if !hasDetail(r, "step_minutes", "invalid") || !hasDetail(r, "channel", "invalid") {
		t.Fatalf("details = %v", r.details)
	}
	bad = search(aUUID, line(cutUUID, nil))
	bad["to"] = "2030-01-01"
	if r = want(call(owner, bad), http.StatusBadRequest, "to before from"); !hasDetail(r, "to", "before_from") {
		t.Fatalf("details = %v", r.details)
	}
	bad["to"] = "2030-03-01"
	if r = want(call(owner, bad), http.StatusBadRequest, "too long"); !hasDetail(r, "to", "too_long") {
		t.Fatalf("details = %v", r.details)
	}
	if r = want(call(owner, map[string]any{"branch_uuid": aUUID, "lines": []any{}}), http.StatusBadRequest, "no lines"); !hasDetail(r, "lines", "required") {
		t.Fatalf("details = %v", r.details)
	}
	exec(`UPDATE services SET is_active = FALSE WHERE organization_id = $1 AND id = $2`, org.ID, massageID)
	if r = want(call(owner, search(aUUID, line(massageUUID, nil))), http.StatusBadRequest, "inactive"); !hasDetail(r, "lines.0.service_uuid", "inactive") {
		t.Fatalf("details = %v", r.details)
	}

	// Isolation: another business's branch and service are unknown here,
	// and this business's branch is unknown there.
	want(call(owner, search(foreignBranch, line(cutUUID, nil))), http.StatusNotFound, "foreign branch")
	if r = want(call(owner, search(aUUID, line(foreignService, nil))), http.StatusBadRequest, "foreign service"); !hasDetail(r, "lines.0.service_uuid", "not_found") {
		t.Fatalf("details = %v", r.details)
	}
	want(call(other, search(aUUID, line(cutUUID, nil))), http.StatusNotFound, "branch of A from B")

	// Check re-plans one start (the reservation flow's second look).
	in := model.SearchInput{BranchUUID: aUUID, Lines: []model.LineInput{{ServiceUUID: cutUUID, StaffUUID: &ayseUUID}}}
	if _, ok, err := svc.Check(ctx, org.ID, in, availabilityusecase.Caller{}, time.Date(2030, 1, 7, 10, 0, 0, 0, ist)); err != nil || ok {
		t.Fatalf("Check in the break = %v, %v", ok, err)
	}
	slot, ok, err := svc.Check(ctx, org.ID, in, availabilityusecase.Caller{}, time.Date(2030, 1, 7, 10, 30, 0, 0, ist))
	if err != nil || !ok || slot.Lines[0].Staff == nil || slot.Lines[0].Staff.UUID != ayseUUID {
		t.Fatalf("Check 10:30 = %+v, %v, %v", slot, ok, err)
	}
}
