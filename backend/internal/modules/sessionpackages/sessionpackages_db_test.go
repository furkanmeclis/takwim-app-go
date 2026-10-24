package sessionpackages_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilitymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	cashrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/repository"
	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	orgsettingsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	salesmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/model"
	salesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/repository"
	salesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sessionpackages/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sessionpackages/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sessionpackages/usecase"
	staffsharesmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/model"
	staffsharesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/repository"
	staffsharesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

var ist = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

func on(day, hour, minute int) time.Time {
	return time.Date(2030, 1, day, hour, minute, 0, 0, ist)
}

func ptr[T any](v T) *T { return &v }

func m(s string) money.Money { return money.MustParse(s) }

// noShowSettings turns no_show_uses_package_session on while on is set.
type noShowSettings struct {
	availabilityusecase.SettingsReader
	on bool
}

func (n *noShowSettings) Current(ctx context.Context, orgID int64) (orgsettingsmodel.Version, error) {
	v, err := n.SettingsReader.Current(ctx, orgID)
	if err == nil && n.on {
		v.Settings.Reservation.NoShowUsesPackageSession = true
	}
	return v, err
}

func TestSessionValue(t *testing.T) {
	// 5 × 2.500,00 = 500,00 each; 1.000,00 / 3 = 333,33 + 333,33 + 333,34.
	for no := 1; no <= 5; no++ {
		if v := reservationsmodel.SessionValue(m("2500"), 5, no); v.String() != "500.00" {
			t.Fatalf("session %d = %s", no, v)
		}
	}
	got := []string{}
	for no := 1; no <= 3; no++ {
		got = append(got, reservationsmodel.SessionValue(m("1000"), 3, no).String())
	}
	if got[0] != "333.33" || got[1] != "333.33" || got[2] != "333.34" {
		t.Fatalf("thirds = %v", got)
	}
}

// TestSessionPackages walks the package flow (requirements §13, A19): a
// definition, a sale that is a collection and not revenue, sessions held
// by visits and released by cancel / no-show / a move to another service,
// a session consumed once on completion with revenue and share base at the
// session's value, the last session never given twice, expiry, the
// report and the refund. Runs when TEST_DATABASE_URL points at a migrated
// database.
func TestSessionPackages(t *testing.T) {
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
	owner, other := newUser("owner"), newUser("other")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Packages " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	org2, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Packages other " + suffix, OwnerUserID: other.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM activity_events WHERE organization_id = ANY($1)", []int64{org.ID, org2.ID})
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{org.ID, org2.ID})
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{owner.ID, other.ID})
	})
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	count := func(sql string, args ...any) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return n
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
	hours := `{"mon":{"closed":false,"open":"09:00","close":"20:00"},"tue":{"closed":false,"open":"09:00","close":"20:00"},"wed":{"closed":false,"open":"09:00","close":"20:00"}}`
	shifts := `{"mon":[{"start":"09:00","end":"20:00","breaks":[]}],"tue":[{"start":"09:00","end":"20:00","breaks":[]}],"wed":[{"start":"09:00","end":"20:00","breaks":[]}]}`
	branchID, branchUUID := idOf("SELECT id, uuid FROM branches WHERE organization_id = $1 ORDER BY id LIMIT 1", org.ID)
	exec(`UPDATE branches SET working_hours = $3, holiday_exceptions = '[]', timezone = NULL WHERE organization_id = $1 AND id = $2`,
		org.ID, branchID, hours)
	branch2ID, branch2UUID := idOf(`INSERT INTO branches (organization_id, code, name, working_hours) VALUES ($1, 'B2', 'Şube 2', $2) RETURNING id, uuid`,
		org.ID, hours)
	staffID, staffUUID := idOf(`INSERT INTO staff (organization_id, name) VALUES ($1, 'Ayşe') RETURNING id, uuid`, org.ID)
	for _, b := range []int64{branchID, branch2ID} {
		exec(`INSERT INTO staff_branches (organization_id, staff_id, branch_id, shifts) VALUES ($1, $2, $3, $4)`, org.ID, staffID, b, shifts)
	}
	newService := func(name, price string) uuid.UUID {
		id, u := idOf(`INSERT INTO services (organization_id, name, duration_minutes, price, payment_mode)
			VALUES ($1, $2, 30, $3, 'both') RETURNING id, uuid`, org.ID, name, price)
		for _, b := range []int64{branchID, branch2ID} {
			exec(`INSERT INTO service_branches (organization_id, service_id, branch_id) VALUES ($1, $2, $3)`, org.ID, id, b)
		}
		exec(`INSERT INTO service_staff (organization_id, service_id, staff_id) VALUES ($1, $2, $3)`, org.ID, id, staffID)
		return u
	}
	laserUUID := newService("Lazer", "800")
	massageUUID := newService("Masaj", "600")
	customerID, customerUUID := idOf(`INSERT INTO customers (organization_id, first_name, last_name) VALUES ($1, 'Zeynep', 'Kaya') RETURNING id, uuid`, org.ID)
	_, otherCustomerUUID := idOf(`INSERT INTO customers (organization_id, first_name, last_name) VALUES ($1, 'Ali', 'Can') RETURNING id, uuid`, org.ID)
	_, tillUUID := idOf(`INSERT INTO cash_accounts (organization_id, branch_id, kind, name, currency, opening_date)
		VALUES ($1, $2, 'cash', 'Kasa', 'TRY', '2026-01-01') RETURNING id, uuid`, org.ID, branchID)
	_, foreignCustomer := idOf(`INSERT INTO customers (organization_id, first_name) VALUES ($1, 'Yabancı') RETURNING id, uuid`, org2.ID)

	now := on(7, 9, 0)
	clock := func() time.Time { return now }
	cashSvc := cashusecase.New(cashrepository.NewPostgres(pool, q), nil)
	cashSvc.SetClock(clock)
	sales := salesusecase.New(salesrepository.NewPostgres(pool, q), cashSvc)
	sales.SetClock(clock)
	shares := staffsharesusecase.New(staffsharesrepository.NewPostgres(pool, q), cashSvc)
	shares.SetClock(clock)
	sales.SetLineHook(shares.RecordLineTx)
	svc := usecase.New(repository.NewPostgres(pool, q), sales)
	svc.SetClock(clock)
	settings := orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))
	holds := availabilityusecase.NewHolds(pool, q, settings)
	holds.SetClock(func() time.Time { return on(7, 8, 0) })
	holds.SetExpiryHook(reservationsusecase.ExpireRequests)
	forfeit := &noShowSettings{SettingsReader: settings}
	resSvc := reservationsusecase.New(holds, q, forfeit)
	resSvc.SetCompletionHook(sales.RecordReservationTx)
	all := orgctx.WithScope(ctx, orgctx.Scope{ID: org.ID, Timezone: "Europe/Istanbul", Currency: "TRY"})
	none := availabilityusecase.Caller{}
	eq := func(step string, got money.Money, want string) {
		t.Helper()
		if got.String() != want {
			t.Fatalf("%s = %s, want %s", step, got, want)
		}
	}
	field := func(err error) string {
		var ve *reservationsusecase.ValidationError
		if errors.As(err, &ve) && len(ve.Details) > 0 {
			return ve.Details[0].Field + "/" + ve.Details[0].Code
		}
		return ""
	}

	// Staff share: Ayşe 40 % of services.
	if _, err := shares.CreateRule(all, org.ID, &owner.ID, staffsharesmodel.RuleInput{
		StaffUUID: &staffUUID, ItemType: ptr(staffsharesmodel.ItemService), Basis: ptr(staffsharesmodel.BasisPercent),
		Rate: ptr(money.Percent(40)), ValidFrom: ptr("2030-01-01"),
	}); err != nil {
		t.Fatal(err)
	}

	// --- Definitions.
	def := model.PackageInput{
		Name: "Lazer 5 seans", ServiceUUID: laserUUID, TotalSessions: 5, Price: m("2500"),
		ValidityDays: ptr(30), AllBranches: true,
	}
	narrow := orgctx.WithScope(ctx, orgctx.Scope{ID: org.ID, BranchIDs: []int64{branchID}})
	if _, err := svc.CreatePackage(narrow, org.ID, &owner.ID, def); !errors.Is(err, usecase.ErrAllBranches) {
		t.Fatalf("branch-limited create = %v", err)
	}
	if _, err := svc.CreatePackage(all, org.ID, &owner.ID, model.PackageInput{Name: "", TotalSessions: 0, Price: m("-1")}); err == nil {
		t.Fatal("invalid definition accepted")
	}
	laser, err := svc.CreatePackage(all, org.ID, &owner.ID, def)
	if err != nil {
		t.Fatal(err)
	}
	if laser.SessionValue.String() != "500.00" || laser.Service.UUID != laserUUID || !laser.AllBranches {
		t.Fatalf("package = %+v", laser)
	}
	if _, err := svc.CreatePackage(all, org.ID, &owner.ID, def); !errors.Is(err, usecase.ErrNameTaken) {
		t.Fatalf("same name = %v", err)
	}
	branchOnly, err := svc.CreatePackage(all, org.ID, &owner.ID, model.PackageInput{
		Name: "Lazer şube 1", ServiceUUID: laserUUID, TotalSessions: 2, Price: m("1000"),
		BranchUUIDs: []uuid.UUID{branchUUID},
	})
	if err != nil || branchOnly.AllBranches || len(branchOnly.Branches) != 1 {
		t.Fatalf("branch package = %+v %v", branchOnly, err)
	}

	// --- Sale: money taken (collection) and the package charged; no revenue.
	sell := func(pkg uuid.UUID, customer uuid.UUID, key string) (model.CustomerPackage, bool, error) {
		return svc.Sell(all, org.ID, &owner.ID, model.SellInput{
			PackageUUID: pkg, CustomerUUID: customer, BranchUUID: branchUUID, Method: "cash",
			CashAccountUUID: &tillUUID, IdempotencyKey: key,
		})
	}
	if _, _, err := svc.Sell(all, org.ID, &owner.ID, model.SellInput{
		PackageUUID: laser.UUID, CustomerUUID: customerUUID, BranchUUID: branchUUID, Method: "cash",
	}); err == nil {
		t.Fatal("sale without an account accepted")
	}
	if _, _, err := sell(laser.UUID, foreignCustomer, ""); err == nil {
		t.Fatal("another business's customer accepted")
	}
	cp, replayed, err := sell(laser.UUID, customerUUID, "sale-"+suffix)
	if err != nil || replayed {
		t.Fatalf("sell = %v %v", replayed, err)
	}
	if cp.State != model.StateActive || cp.Remaining != 5 || cp.SoldOn != "2030-01-07" || cp.ExpiresOn == nil || *cp.ExpiresOn != "2030-02-06" {
		t.Fatalf("sold = %+v", cp)
	}
	again, replayed, err := sell(laser.UUID, customerUUID, "sale-"+suffix)
	if err != nil || !replayed || again.UUID != cp.UUID {
		t.Fatalf("replay = %v %v %v", again.UUID, replayed, err)
	}
	if _, _, err := sell(branchOnly.UUID, customerUUID, "sale-"+suffix); !errors.Is(err, usecase.ErrIdempotencyReused) {
		t.Fatalf("reused key = %v", err)
	}
	if n := count(`SELECT count(*) FROM cash_movements WHERE organization_id = $1 AND kind = 'collection'`, org.ID); n != 1 {
		t.Fatalf("collections = %d", n)
	}
	balance := func(step, charged, paid, bal string) {
		t.Helper()
		b, err := sales.CustomerBalance(all, org.ID, customerUUID)
		if err != nil || len(b.Totals) != 1 {
			t.Fatalf("%s balance = %+v %v", step, b, err)
		}
		eq(step+" charged", b.Totals[0].Charged, charged)
		eq(step+" paid", b.Totals[0].Paid, paid)
		eq(step+" balance", b.Totals[0].Balance, bal)
	}
	balance("sold", "2500.00", "2500.00", "0.00")
	day := func(step, revenue, collected string) {
		t.Helper()
		from := time.Date(2030, 1, 7, 0, 0, 0, 0, time.UTC)
		s, err := sales.Summary(all, org.ID, &from, &from, nil)
		if err != nil {
			t.Fatal(err)
		}
		eq(step+" revenue", s.Totals[0].Revenue.Net, revenue)
		eq(step+" collections", s.Totals[0].Collections.Net, collected)
	}
	day("after sale", "0.00", "2500.00")

	// --- Sessions on visits.
	book := func(start time.Time, service uuid.UUID, branch uuid.UUID) reservationsmodel.Reservation {
		t.Helper()
		res, err := resSvc.Create(ctx, org.ID, reservationsmodel.CreateInput{
			BranchUUID: branch, CustomerUUID: customerUUID, StartsAt: &start, Source: reservationsmodel.SourcePanel,
			Lines: []availabilitymodel.LineInput{{ServiceUUID: service, StaffUUID: &staffUUID}},
		}, none, owner.ID)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	apply := func(res uuid.UUID, pkg uuid.UUID) (reservationsmodel.Reservation, error) {
		return resSvc.ApplyPackage(ctx, org.ID, res, none, owner.ID, reservationsmodel.ApplyPackageInput{CustomerPackageUUID: pkg})
	}
	v1 := book(on(7, 10, 0), laserUUID, branchUUID)
	eq("booked total", v1.TotalPrice, "800.00")
	opts, err := resSvc.PackageOptions(ctx, org.ID, v1.UUID, none)
	if err != nil || len(opts) != 1 || opts[0].CustomerPackageUUID != cp.UUID || len(opts[0].LineIndexes) != 1 || opts[0].NextValue.String() != "500.00" {
		t.Fatalf("options = %+v %v", opts, err)
	}
	v1, err = apply(v1.UUID, cp.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if len(v1.PackageSessions) != 1 || v1.PackageSessions[0].SessionNo != 1 || v1.PackageSessions[0].Amount.String() != "500.00" ||
		v1.PackageSessions[0].Status != reservationsmodel.SessionHeld {
		t.Fatalf("session = %+v", v1.PackageSessions)
	}
	eq("total on package", v1.TotalPrice, "0.00")
	if _, err := apply(v1.UUID, cp.UUID); field(err) != "line_index/already_on_package" {
		t.Fatalf("second session for the line = %v", err)
	}
	// Another service, another customer's package, another business's id.
	vm := book(on(7, 10, 30), massageUUID, branchUUID)
	if _, err := apply(vm.UUID, cp.UUID); field(err) != "line_index/service_mismatch" {
		t.Fatalf("other service = %v", err)
	}
	otherCP, _, err := sell(laser.UUID, otherCustomerUUID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := apply(vm.UUID, otherCP.UUID); field(err) != "customer_package_uuid/not_found" {
		t.Fatalf("other customer's package = %v", err)
	}
	if _, err := apply(vm.UUID, uuid.New()); field(err) != "customer_package_uuid/not_found" {
		t.Fatalf("unknown package = %v", err)
	}
	// A branch-limited package cannot be used in the other branch.
	bp, _, err := sell(branchOnly.UUID, customerUUID, "")
	if err != nil {
		t.Fatal(err)
	}
	vb2 := book(on(7, 11, 0), laserUUID, branch2UUID)
	if _, err := apply(vb2.UUID, bp.UUID); field(err) != "customer_package_uuid/branch_not_allowed" {
		t.Fatalf("branch-limited = %v", err)
	}
	if _, _, err := svc.Sell(all, org.ID, &owner.ID, model.SellInput{
		PackageUUID: branchOnly.UUID, CustomerUUID: customerUUID, BranchUUID: branch2UUID, Method: "cash", CashAccountUUID: &tillUUID,
	}); err == nil {
		t.Fatal("sale in a branch the package is not sold in accepted")
	}

	// Cancel releases the session; it can be given again.
	v2 := book(on(7, 12, 0), laserUUID, branchUUID)
	if _, err := apply(v2.UUID, cp.UUID); err != nil {
		t.Fatal(err)
	}
	v2, err = resSvc.Cancel(ctx, org.ID, v2.UUID, none, owner.ID, reservationsusecase.ChangeInput{})
	if err != nil || v2.PackageSessions[0].Status != reservationsmodel.SessionReleased || *v2.PackageSessions[0].ReleaseReason != "cancelled" {
		t.Fatalf("cancel = %+v %v", v2.PackageSessions, err)
	}
	// No-show releases too.
	v3 := book(on(7, 13, 0), laserUUID, branchUUID)
	if _, err := apply(v3.UUID, cp.UUID); err != nil {
		t.Fatal(err)
	}
	if v3, err = resSvc.NoShow(ctx, org.ID, v3.UUID, none, owner.ID, reservationsusecase.ChangeInput{}); err != nil ||
		v3.PackageSessions[0].Status != reservationsmodel.SessionReleased {
		t.Fatalf("no-show = %+v %v", v3.PackageSessions, err)
	}
	// A move keeping the service keeps the session; to another service
	// releases it and the line is charged again.
	v4 := book(on(7, 14, 0), laserUUID, branchUUID)
	if _, err := apply(v4.UUID, cp.UUID); err != nil {
		t.Fatal(err)
	}
	v4, err = resSvc.Reschedule(ctx, org.ID, v4.UUID, none, owner.ID, reservationsmodel.RescheduleInput{StartsAt: ptr(on(7, 15, 0))})
	if err != nil || v4.PackageSessions[0].Status != reservationsmodel.SessionHeld || !v4.TotalPrice.IsZero() {
		t.Fatalf("move = %+v %s %v", v4.PackageSessions, v4.TotalPrice, err)
	}
	v4, err = resSvc.Reschedule(ctx, org.ID, v4.UUID, none, owner.ID, reservationsmodel.RescheduleInput{
		StartsAt: ptr(on(7, 15, 0)), Lines: []availabilitymodel.LineInput{{ServiceUUID: massageUUID, StaffUUID: &staffUUID}},
	})
	if err != nil || v4.PackageSessions[0].Status != reservationsmodel.SessionReleased || *v4.PackageSessions[0].ReleaseReason != "rescheduled" {
		t.Fatalf("move to another service = %+v %v", v4.PackageSessions, err)
	}
	eq("total after move", v4.TotalPrice, "600.00")
	// Removing a held session charges the line again.
	v5 := book(on(7, 16, 0), laserUUID, branchUUID)
	if v5, err = apply(v5.UUID, cp.UUID); err != nil {
		t.Fatal(err)
	}
	if v5, err = resSvc.RemovePackageSession(ctx, org.ID, v5.UUID, v5.PackageSessions[0].UUID, none, owner.ID, reservationsusecase.ChangeInput{}); err != nil ||
		v5.PackageSessions[0].Status != reservationsmodel.SessionReleased || v5.TotalPrice.String() != "800.00" {
		t.Fatalf("remove = %+v %v", v5, err)
	}

	// --- Completion: consumed once; revenue and share base 500 (A19).
	v1, err = resSvc.Complete(ctx, org.ID, v1.UUID, none, owner.ID, reservationsusecase.ChangeInput{})
	if err != nil || v1.PackageSessions[0].Status != reservationsmodel.SessionConsumed {
		t.Fatalf("complete = %+v %v", v1.PackageSessions, err)
	}
	if _, err := resSvc.Complete(ctx, org.ID, v1.UUID, none, owner.ID, reservationsusecase.ChangeInput{}); err == nil {
		t.Fatal("second completion accepted")
	}
	if n := count(`SELECT count(*) FROM sale_lines WHERE organization_id = $1 AND reservation_id = (SELECT id FROM reservations WHERE uuid = $2) AND net_amount = 500`, org.ID, v1.UUID); n != 1 {
		t.Fatalf("session sale lines = %d", n)
	}
	if n := count(`SELECT count(*) FROM ledger_entries WHERE organization_id = $1 AND reservation_id = (SELECT id FROM reservations WHERE uuid = $2)`, org.ID, v1.UUID); n != 0 {
		t.Fatalf("visit ledger entries = %d (the package was charged at the sale)", n)
	}
	if n := count(`SELECT count(*) FROM staff_share_lines WHERE organization_id = $1 AND share_amount = 200 AND base_amount = 500`, org.ID); n != 1 {
		t.Fatalf("A19 share (40 %% of 500) = %d", n)
	}
	rb, err := sales.ReservationBalance(all, org.ID, v1.UUID)
	if err != nil {
		t.Fatal(err)
	}
	eq("visit expected", rb.Expected, "0.00")
	eq("visit remaining", rb.Remaining, "0.00")
	// Collections are the package sales (2 × 2.500 + 1.000); revenue is
	// the one session used, never the package price again.
	day("after completion", "500.00", "6000.00")
	balance("used", "3500.00", "3500.00", "0.00")
	got, err := svc.GetCustomerPackage(all, org.ID, cp.UUID)
	if err != nil || got.Used != 1 || got.Held != 0 || got.Remaining != 4 || got.UsedAmount.String() != "500.00" ||
		got.RemainingAmount.String() != "2000.00" || len(got.Sessions) != 5 {
		t.Fatalf("detail = %+v %v", got, err)
	}

	// --- The last session is never given to two visits.
	last, _, err := sell(branchOnly.UUID, customerUUID, "")
	if err != nil {
		t.Fatal(err)
	}
	a := book(on(8, 10, 0), laserUUID, branchUUID)
	if _, err := apply(a.UUID, last.UUID); err != nil {
		t.Fatal(err)
	}
	b := book(on(8, 11, 0), laserUUID, branchUUID)
	c := book(on(8, 12, 0), laserUUID, branchUUID)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, v := range []uuid.UUID{b.UUID, c.UUID} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = apply(v, last.UUID)
		}()
	}
	wg.Wait()
	if (errs[0] == nil) == (errs[1] == nil) {
		t.Fatalf("last session: %v / %v", errs[0], errs[1])
	}
	for _, e := range errs {
		if e != nil && field(e) != "customer_package_uuid/no_sessions_left" {
			t.Fatalf("loser = %v", e)
		}
	}
	if n := count(`SELECT count(*) FROM package_sessions WHERE organization_id = $1 AND customer_package_id = (SELECT id FROM customer_packages WHERE uuid = $2) AND status <> 'released'`, org.ID, last.UUID); n != 2 {
		t.Fatalf("live sessions = %d", n)
	}
	// The table refuses a third live session.
	if _, err := pool.Exec(ctx, `INSERT INTO package_sessions (organization_id, customer_package_id, reservation_id, line_index, session_no, amount)
		SELECT cp.organization_id, cp.id, r.id, 0, 3, 0 FROM customer_packages cp, reservations r WHERE cp.uuid = $1 AND r.uuid = $2`, last.UUID, v5.UUID); err == nil {
		t.Fatal("session beyond the count accepted")
	}

	// --- Expiry: a visit after the last day cannot use the package.
	exec(`UPDATE customer_packages SET expires_on = '2030-01-07' WHERE organization_id = $1 AND uuid = $2`, org.ID, bp.UUID)
	late := book(on(8, 14, 0), laserUUID, branchUUID)
	if _, err := apply(late.UUID, bp.UUID); field(err) != "customer_package_uuid/expired" {
		t.Fatalf("expired = %v", err)
	}
	now = on(8, 9, 0)
	if got, err := svc.GetCustomerPackage(all, org.ID, bp.UUID); err != nil || got.State != model.StateExpired {
		t.Fatalf("expired state = %+v %v", got.State, err)
	}

	// --- Report.
	from, to := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2030, 1, 31, 0, 0, 0, 0, time.UTC)
	rep, err := svc.Summary(all, org.ID, &from, &to, nil)
	if err != nil || len(rep.Totals) != 1 {
		t.Fatalf("summary = %+v %v", rep, err)
	}
	f := rep.Totals[0]
	if f.Sold.Count != 4 || f.Sold.Amount.String() != "7000.00" || f.Used.Count != 1 || f.Used.Amount.String() != "500.00" ||
		f.Expired.Count != 1 || f.Expired.Sessions != 2 || f.Expired.Amount.String() != "1000.00" || f.Held.Count != 2 {
		t.Fatalf("figures = %+v", f)
	}
	list, n, err := svc.ListCustomerPackages(all, org.ID, model.CustomerPackageFilter{Customer: &customerUUID, States: []string{model.StateExpired}, Limit: 10})
	if err != nil || n != 1 || list[0].UUID != bp.UUID {
		t.Fatalf("expired list = %d %v", n, err)
	}

	// --- Refund: sessions still booked block it; the rest is paid back.
	if _, err := svc.Refund(all, org.ID, last.UUID, &owner.ID, model.RefundInput{Amount: m("0"), Reason: "iptal"}); !errors.Is(err, usecase.ErrSessionsHeld) {
		t.Fatalf("refund with held sessions = %v", err)
	}
	refund := model.RefundInput{Amount: m("2000"), Method: "cash", CashAccountUUID: &tillUUID, Reason: "müşteri vazgeçti"}
	if _, err := svc.Refund(all, org.ID, cp.UUID, &owner.ID, model.RefundInput{Amount: m("3000"), Method: "cash", CashAccountUUID: &tillUUID, Reason: "fazla"}); err == nil {
		t.Fatal("refund above the price accepted")
	}
	got, err = svc.Refund(all, org.ID, cp.UUID, &owner.ID, refund)
	if err != nil || got.State != model.StateRefunded || got.Refund == nil || got.Refund.Amount.String() != "2000.00" || got.Remaining != 0 {
		t.Fatalf("refund = %+v %v", got, err)
	}
	if _, err := svc.Refund(all, org.ID, cp.UUID, &owner.ID, refund); !errors.Is(err, usecase.ErrAlreadyRefunded) {
		t.Fatalf("second refund = %v", err)
	}
	if n := count(`SELECT count(*) FROM cash_movements WHERE organization_id = $1 AND kind = 'refund'`, org.ID); n != 1 {
		t.Fatalf("refund movements = %d", n)
	}
	// Customer: 2.500 − 2.500 + 500 kept (cp) + 2.500 (other's is another
	// customer) + 1.000 + 1.000 charged; paid the same.
	balance("refunded", "2500.00", "2500.00", "0.00")
	v6 := book(on(8, 15, 0), laserUUID, branchUUID)
	if _, err := apply(v6.UUID, cp.UUID); field(err) != "customer_package_uuid/refunded" {
		t.Fatalf("refunded package = %v", err)
	}
	rep, err = svc.Summary(all, org.ID, &from, &to, nil)
	if err != nil || rep.Totals[0].Refunded.Count != 1 || rep.Totals[0].Refunded.Amount.String() != "2000.00" || rep.Totals[0].Refunded.Sessions != 4 {
		t.Fatalf("refunds in summary = %+v %v", rep.Totals[0].Refunded, err)
	}
	// A sold package keeps its terms.
	if _, err := pool.Exec(ctx, `UPDATE customer_packages SET price = 1 WHERE organization_id = $1 AND uuid = $2`, org.ID, otherCP.UUID); err == nil {
		t.Fatal("price change of a sold package accepted")
	}

	// --- Sessions chosen at booking (TEC-984).
	fresh, _, err := sell(laser.UUID, customerUUID, "")
	if err != nil {
		t.Fatal(err)
	}
	opts, err = resSvc.PackageOptionsFor(all, org.ID, customerUUID, branchUUID, on(9, 0, 0))
	if err != nil || len(opts) != 1 || opts[0].CustomerPackageUUID != fresh.UUID || opts[0].Remaining != 5 {
		t.Fatalf("options for a new visit = %+v %v", opts, err)
	}
	if _, err := resSvc.PackageOptionsFor(all, org.ID, uuid.New(), branchUUID, on(9, 0, 0)); field(err) != "customer_uuid/not_found" {
		t.Fatalf("options for an unknown customer = %v", err)
	}
	if _, err := resSvc.PackageOptionsFor(all, org.ID, customerUUID, uuid.New(), on(9, 0, 0)); field(err) != "branch_uuid/not_found" {
		t.Fatalf("options for an unknown branch = %v", err)
	}
	bookWith := func(start time.Time, sessions []reservationsmodel.PackageSessionInput, services ...uuid.UUID) (reservationsmodel.Reservation, error) {
		lines := []availabilitymodel.LineInput{}
		for _, s := range services {
			lines = append(lines, availabilitymodel.LineInput{ServiceUUID: s, StaffUUID: &staffUUID})
		}
		return resSvc.Create(ctx, org.ID, reservationsmodel.CreateInput{
			BranchUUID: branchUUID, CustomerUUID: customerUUID, StartsAt: &start, Source: reservationsmodel.SourcePanel,
			Lines: lines, PackageSessions: sessions,
		}, none, owner.ID)
	}
	on0 := func(pkg uuid.UUID) []reservationsmodel.PackageSessionInput {
		return []reservationsmodel.PackageSessionInput{{CustomerPackageUUID: pkg, LineIndex: 0}}
	}
	if _, err := bookWith(on(9, 10, 0), on0(fresh.UUID), massageUUID); field(err) != "package_sessions.0.line_index/service_mismatch" {
		t.Fatalf("booking on another service's package = %v", err)
	}
	if _, err := bookWith(on(9, 10, 0), on0(uuid.New()), laserUUID); field(err) != "package_sessions.0.customer_package_uuid/not_found" {
		t.Fatalf("booking on an unknown package = %v", err)
	}
	if _, err := bookWith(on(9, 10, 0), on0(bp.UUID), laserUUID); field(err) != "package_sessions.0.customer_package_uuid/expired" {
		t.Fatalf("booking on an expired package = %v", err)
	}
	if n := count(`SELECT count(*) FROM reservations WHERE organization_id = $1 AND starts_at = $2`, org.ID, on(9, 10, 0)); n != 0 {
		t.Fatalf("refused bookings left %d visits", n)
	}
	both, err := bookWith(on(9, 10, 0), on0(fresh.UUID), laserUUID, massageUUID)
	if err != nil {
		t.Fatal(err)
	}
	if len(both.PackageSessions) != 1 || both.PackageSessions[0].Status != reservationsmodel.SessionHeld || both.PackageSessions[0].LineIndex != 0 {
		t.Fatalf("booked session = %+v", both.PackageSessions)
	}
	eq("booked with a session", both.TotalPrice, "600.00")
	covered, err := bookWith(on(9, 12, 0), on0(fresh.UUID), laserUUID)
	if err != nil || covered.PackageSessions[0].SessionNo != 2 {
		t.Fatalf("second booked session = %+v %v", covered.PackageSessions, err)
	}
	eq("covered booking", covered.TotalPrice, "0.00")

	// --- No-show with no_show_uses_package_session: the session is used,
	// without revenue (it is forfeited, not served).
	forfeit.on = true
	covered, err = resSvc.NoShow(ctx, org.ID, covered.UUID, none, owner.ID, reservationsusecase.ChangeInput{})
	forfeit.on = false
	if err != nil || covered.PackageSessions[0].Status != reservationsmodel.SessionConsumed {
		t.Fatalf("forfeited no-show = %+v %v", covered.PackageSessions, err)
	}
	if n := count(`SELECT count(*) FROM sale_lines WHERE organization_id = $1 AND reservation_id = (SELECT id FROM reservations WHERE uuid = $2)`, org.ID, covered.UUID); n != 0 {
		t.Fatalf("forfeited no-show sale lines = %d", n)
	}
	if got, err := svc.GetCustomerPackage(all, org.ID, fresh.UUID); err != nil || got.Used != 1 || got.Held != 1 || got.Remaining != 3 {
		t.Fatalf("after forfeit = %+v %v", got, err)
	}

	// --- Engines: the definition switch and the export.
	if prev, err := svc.SetPackageActive(narrow, org.ID, laser.UUID, false); !errors.Is(err, usecase.ErrAllBranches) {
		t.Fatalf("branch-limited switch = %v %v", prev, err)
	}
	if prev, err := svc.SetPackageActive(all, org.ID, laser.UUID, false); err != nil || !prev {
		t.Fatalf("deactivate = %v %v", prev, err)
	}
	if prev, err := svc.SetPackageActive(all, org.ID, laser.UUID, true); err != nil || prev {
		t.Fatalf("activate = %v %v", prev, err)
	}
	if _, err := svc.SetPackageActive(orgctx.WithScope(ctx, orgctx.Scope{ID: org2.ID}), org2.ID, laser.UUID, false); !errors.Is(err, usecase.ErrPackageNotFound) {
		t.Fatalf("other business switch = %v", err)
	}

	// --- Isolation and branch scope.
	if _, err := svc.GetCustomerPackage(orgctx.WithScope(ctx, orgctx.Scope{ID: org2.ID}), org2.ID, cp.UUID); !errors.Is(err, usecase.ErrCustomerPackage) {
		t.Fatalf("other business = %v", err)
	}
	b2 := orgctx.WithScope(ctx, orgctx.Scope{ID: org.ID, BranchIDs: []int64{branch2ID}, Timezone: "Europe/Istanbul"})
	if _, err := svc.GetCustomerPackage(b2, org.ID, cp.UUID); !errors.Is(err, usecase.ErrCustomerPackage) {
		t.Fatalf("other branch = %v", err)
	}
	if list, n, err := svc.ListCustomerPackages(b2, org.ID, model.CustomerPackageFilter{Limit: 10}); err != nil || n != 0 || len(list) != 0 {
		t.Fatalf("other branch list = %d %v", n, err)
	}
	if _, err := svc.GetPackage(orgctx.WithScope(ctx, orgctx.Scope{ID: org2.ID}), org2.ID, laser.UUID); !errors.Is(err, usecase.ErrPackageNotFound) {
		t.Fatalf("other business definition = %v", err)
	}
	_ = customerID
	_ = salesmodel.MethodCash
	_ = http.StatusOK
}
