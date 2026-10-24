package reports_test

import (
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
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

// TestDailySummary seeds a day with the owner serving visits itself, a
// staff member, a visit waiting for approval, sales, shares and a
// collection; then checks the owner's own day and the business day, the
// branch scope, the reception (visits but no money, no own day), the
// staff member (only its own day and earnings), another business and the
// date check. Runs when TEST_DATABASE_URL points at a migrated database.
func TestDailySummary(t *testing.T) {
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
			Email: name + "-daily-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	owner, desk, worker, other := newUser("owner"), newUser("desk"), newUser("worker"), newUser("other")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Daily " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	org2, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Daily other " + suffix, OwnerUserID: other.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM activity_events WHERE organization_id = ANY($1)", []int64{org.ID, org2.ID})
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{org.ID, org2.ID})
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{owner.ID, desk.ID, worker.ID, other.ID})
	})
	if _, err := orgSvc.AddMember(ctx, org.UUID, desk.Uuid, "resepsiyon"); err != nil {
		t.Fatal(err)
	}
	if _, err := orgSvc.AddMember(ctx, org.UUID, worker.Uuid, "member"); err != nil {
		t.Fatal(err)
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

	xID, xUUID := idOf("SELECT id, uuid FROM branches WHERE organization_id = $1 ORDER BY id LIMIT 1", org.ID)
	exec("UPDATE branches SET timezone = NULL WHERE organization_id = $1", org.ID)
	yID, yUUID := idOf("INSERT INTO branches (organization_id, code, name) VALUES ($1, 'YY', 'Yalı') RETURNING id, uuid", org.ID)
	ownerStaff, ownerStaffUUID := idOf("INSERT INTO staff (organization_id, name, user_id) VALUES ($1, 'Sahip', $2) RETURNING id, uuid", org.ID, owner.ID)
	workerStaff, _ := idOf("INSERT INTO staff (organization_id, name, user_id) VALUES ($1, 'Ece', $2) RETURNING id, uuid", org.ID, worker.ID)
	customerID, _ := idOf("INSERT INTO customers (organization_id, first_name, last_name) VALUES ($1, 'Zeynep', 'Kaya') RETURNING id, uuid", org.ID)
	// The other business has a visit the same day.
	b2, _ := idOf("SELECT id, uuid FROM branches WHERE organization_id = $1 ORDER BY id LIMIT 1", org2.ID)
	c2, _ := idOf("INSERT INTO customers (organization_id, first_name, last_name) VALUES ($1, 'Başka', 'Müşteri') RETURNING id, uuid", org2.ID)

	number := int64(0)
	reservation := func(orgID, branch, customer int64, status, approval string, start time.Time, staff int64) (int64, uuid.UUID) {
		number++
		id, u := idOf(`INSERT INTO reservations (organization_id, number, branch_id, customer_id, status, approval_mode, approval_status,
			payment_status, source, starts_at, ends_at, total_price, currency)
			VALUES ($1, $2, $3, $4, $5, 'panel', $6, 'on_site', 'panel', $7, $8, 1000, 'TRY') RETURNING id, uuid`,
			orgID, number, branch, customer, status, approval, start, start.Add(time.Hour))
		exec(`INSERT INTO reservation_lines (organization_id, reservation_id, line_index, service_name, staff_id, starts_at, ends_at,
			duration_minutes, unit_price) VALUES ($1, $2, 0, 'Kesim', $3, $4, $5, 60, 1000)`, orgID, id, staff, start, start.Add(time.Hour))
		return id, u
	}
	r1, r1UUID := reservation(org.ID, xID, customerID, "completed", "accepted", at(14, 10), ownerStaff)
	r2, _ := reservation(org.ID, yID, customerID, "active", "accepted", at(14, 12), workerStaff)
	reservation(org.ID, xID, customerID, "requested", "pending", at(14, 15), workerStaff)
	reservation(org.ID, xID, customerID, "cancelled", "accepted", at(14, 17), ownerStaff)
	// The owner's visit of the next day is not on the 14th.
	reservation(org.ID, xID, customerID, "active", "accepted", at(15, 9), ownerStaff)
	otherStaff, _ := idOf("INSERT INTO staff (organization_id, name) VALUES ($1, 'Diğer') RETURNING id, uuid", org2.ID)
	reservation(org2.ID, b2, c2, "requested", "pending", at(14, 11), otherStaff)

	saleLine := func(branch, reservationID, staff int64, net, key string) int64 {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO sale_lines (organization_id, branch_id, customer_id, kind, item_type, source_type,
			reservation_id, line_index, staff_id, description, quantity, unit_price, gross_amount, discount_amount, net_amount,
			currency, sale_date, accrual_key) VALUES ($1, $2, $3, 'sale', 'service', 'reservation', $4, 0, $5, 'x', 1, $6::numeric,
			$6::numeric, 0, $6::numeric, 'TRY', '2030-01-14', $7) RETURNING id`,
			org.ID, branch, customerID, reservationID, staff, net, key).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	s1 := saleLine(xID, r1, ownerStaff, "900", "reservation:"+r1UUID.String()+":line:0")
	s2 := saleLine(yID, r2, workerStaff, "500", "reservation:r2-"+suffix+":line:0")
	share := func(branch, staff, sale, res int64, base, amount string) {
		exec(`INSERT INTO staff_share_lines (organization_id, branch_id, staff_id, sale_line_id, reservation_id, kind, item_type,
			rule_scope, basis, rate, base_amount, share_amount, currency, share_date) VALUES ($1, $2, $3, $4, $5, 'share', 'service',
			'default', 'percent', 40, $6::numeric, $7::numeric, 'TRY', '2030-01-14')`, org.ID, branch, staff, sale, res, base, amount)
	}
	share(xID, ownerStaff, s1, r1, "900", "360")
	share(yID, workerStaff, s2, r2, "500", "200")
	tillID, _ := idOf(`INSERT INTO cash_accounts (organization_id, branch_id, kind, name, opening_date) VALUES ($1, $2, 'cash', 'Kasa', '2030-01-01') RETURNING id, uuid`, org.ID, xID)
	var moveID int64
	if err := pool.QueryRow(ctx, `INSERT INTO cash_movements (organization_id, cash_account_id, branch_id, kind, direction, amount,
		currency, movement_date) VALUES ($1, $2, $3, 'collection', 'in', 1000, 'TRY', '2030-01-14') RETURNING id`,
		org.ID, tillID, xID).Scan(&moveID); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO ledger_entries (organization_id, branch_id, customer_id, reservation_id, kind, direction, amount, currency,
		entry_date, method, cash_movement_id) VALUES ($1, $2, $3, $4, 'collection', 'credit', 1000, 'TRY', '2030-01-14', 'cash', $5)`,
		org.ID, xID, customerID, r1, moveID)

	svc := usecase.New(q)
	svc.SetClock(func() time.Time { return at(14, 18) })
	full := model.DailyAccess{OwnReservations: true, BusinessReservations: true, Money: true, Earnings: true}
	all := orgctx.WithScope(ctx, orgctx.Scope{ID: org.ID, Timezone: "Europe/Istanbul", Currency: "TRY"})

	d, err := svc.Daily(all, org.ID, owner.ID, nil, full)
	if err != nil {
		t.Fatal(err)
	}
	if d.Date != "2030-01-14" || d.Staff == nil || d.Staff.UUID != ownerStaffUUID || d.Own == nil || d.Business == nil || d.Pending == nil {
		t.Fatalf("owner daily = %+v", d)
	}
	if r := d.Own.Reservations; r.Total != 2 || r.Completed != 1 || r.Cancelled != 1 {
		t.Errorf("own reservations = %+v", r)
	}
	if len(d.Own.Visits) != 1 || d.Own.Visits[0].ReservationUUID != r1UUID || d.Own.Visits[0].CustomerName != "Zeynep Kaya" ||
		d.Own.Visits[0].BranchUUID != xUUID {
		t.Errorf("own visits = %+v", d.Own.Visits)
	}
	if len(d.Own.Earnings) != 1 || d.Own.Earnings[0].Sales.String() != "900.00" || d.Own.Earnings[0].Share.String() != "360.00" {
		t.Errorf("own earnings = %+v", d.Own.Earnings)
	}
	if r := d.Business.Reservations; r == nil || r.Total != 4 || r.Requested != 1 || r.Active != 1 {
		t.Errorf("business reservations = %+v", r)
	}
	if len(d.Business.Money) != 1 {
		t.Fatalf("business money = %+v", d.Business.Money)
	}
	for name, pair := range map[string][2]string{
		"revenue":        {d.Business.Money[0].Revenue.String(), "1400.00"},
		"collections":    {d.Business.Money[0].Collections.String(), "1000.00"},
		"staff_share":    {d.Business.Money[0].StaffShare.String(), "560.00"},
		"business_share": {d.Business.Money[0].BusinessShare.String(), "840.00"},
	} {
		if pair[0] != pair[1] {
			t.Errorf("business %s = %s, want %s", name, pair[0], pair[1])
		}
	}
	if len(d.Business.Branches) != 2 || d.Business.Branches[0].Reservations == nil {
		t.Errorf("business branches = %+v", d.Business.Branches)
	}
	if d.Pending.Approvals != 1 {
		t.Errorf("pending = %+v", d.Pending)
	}

	// The header branch picker (branch scope Y): one branch, no approval.
	onlyY := orgctx.WithScope(ctx, orgctx.Scope{ID: org.ID, Timezone: "Europe/Istanbul", Currency: "TRY", BranchIDs: []int64{yID}})
	dy, err := svc.Daily(onlyY, org.ID, owner.ID, nil, full)
	if err != nil || dy.Business.Money[0].Revenue.String() != "500.00" || dy.Business.Reservations.Total != 1 ||
		len(dy.Business.Branches) != 1 || dy.Business.Branches[0].Branch.UUID != yUUID || dy.Pending.Approvals != 0 ||
		len(dy.Own.Visits) != 0 || !dy.Own.Earnings[0].Share.IsZero() {
		t.Errorf("branch Y daily = %+v %v", dy, err)
	}
	// Another day: the owner's next visit.
	next, err := svc.Daily(all, org.ID, owner.ID, date("2030-01-15"), full)
	if err != nil || len(next.Own.Visits) != 1 || next.Own.Reservations.Total != 1 || !next.Business.Money[0].Revenue.IsZero() {
		t.Errorf("next day = %+v %v", next, err)
	}
	// Another business sees none of it.
	foreign, err := svc.Daily(orgctx.WithScope(ctx, orgctx.Scope{ID: org2.ID}), org2.ID, other.ID, date("2030-01-14"), full)
	if err != nil || foreign.Own != nil || foreign.Business.Reservations.Total != 1 || foreign.Pending.Approvals != 1 ||
		!foreign.Business.Money[0].Revenue.IsZero() {
		t.Errorf("foreign = %+v %v", foreign, err)
	}

	// HTTP: each member gets the sections its permissions allow.
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	reports.RegisterRoutes(mux, handler.New(svc, nil), tokens, mapLoader{owner.Uuid: owner.ID, desk.Uuid: desk.ID, worker.Uuid: worker.ID}, orgSvc)
	call := func(user db.User, path string) (int, model.Daily, string) {
		t.Helper()
		access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: user.Uuid})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+access)
		req.Header.Set(orgctx.HeaderName, org.Slug)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		var env struct {
			Data model.Daily `json:"data"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		return rr.Code, env.Data, rr.Body.String()
	}
	if code, out, raw := call(owner, "/v1/tenant/daily-summary"); code != http.StatusOK || out.Own == nil || out.Business == nil ||
		out.Business.Money == nil || out.Pending == nil || out.Pending.Approvals != 1 {
		t.Errorf("owner = %d %s", code, raw)
	}
	if code, out, raw := call(desk, "/v1/tenant/daily-summary?date=2030-01-14"); code != http.StatusOK || out.Staff != nil ||
		out.Own != nil || out.Business == nil || out.Business.Reservations == nil || out.Business.Reservations.Total != 4 ||
		out.Business.Money != nil || out.Business.Branches != nil || out.Pending == nil || out.Pending.Approvals != 1 {
		t.Errorf("reception = %d %s", code, raw)
	}
	code, out, raw := call(worker, "/v1/tenant/daily-summary?date=2030-01-14")
	if code != http.StatusOK || out.Business != nil || out.Pending != nil || out.Own == nil || len(out.Own.Visits) != 2 ||
		out.Own.Reservations.Total != 2 || out.Own.Reservations.Requested != 1 {
		t.Errorf("staff member = %d %s", code, raw)
	}
	if code == http.StatusOK && out.Own != nil &&
		(len(out.Own.Earnings) != 1 || out.Own.Earnings[0].Share.String() != "200.00" || out.Own.Earnings[0].Sales.String() != "500.00") {
		t.Errorf("staff member earnings = %s", raw)
	}
	if code, _, raw := call(owner, "/v1/tenant/daily-summary?date=14.01.2030"); code != http.StatusBadRequest {
		t.Errorf("bad date = %d %s", code, raw)
	}
}
