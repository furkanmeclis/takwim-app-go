package demo_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/demo"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/phone"
)

// TestSeed writes the six demo businesses through the use cases and checks
// the accounts, the shared customer of the panel and the access limits
// between businesses and branches.
func TestSeed(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)

	// Four days ahead: a run on a Saturday afternoon still reaches an open
	// day of every business.
	res, err := demo.New(pool).Run(ctx, demo.Options{Password: "DemoTest1", Reset: true, HistoryDays: 3, FutureDays: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE slug LIKE 'demo-%'")
	})
	if len(res.Businesses) != 6 {
		t.Fatalf("businesses = %d, want 6", len(res.Businesses))
	}
	sectors := map[string]bool{}
	for _, b := range res.Businesses {
		sectors[b.Sector] = true
		if b.Branches != 2 || b.Customers < 14 {
			t.Errorf("%s: %d branches, %d customers", b.Slug, b.Branches, b.Customers)
		}
		if b.Completed == 0 || b.Upcoming == 0 {
			t.Errorf("%s: %d completed, %d upcoming visits", b.Slug, b.Completed, b.Upcoming)
		}
	}
	if len(sectors) != 6 {
		t.Errorf("sectors = %v", sectors)
	}
	// Platform + per business: owner, manager, reception, staff; the
	// shared account in two businesses.
	if want := 1 + 6*4 + 2; len(res.Accounts) != want {
		t.Errorf("accounts = %d, want %d", len(res.Accounts), want)
	}

	scalar := func(sql string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return n
	}
	// Revenue, collections and staff shares came with the completed visits.
	for _, b := range res.Businesses {
		org, err := q.GetOrganizationBySlug(ctx, b.Slug)
		if err != nil {
			t.Fatal(err)
		}
		if n := scalar(`SELECT count(*) FROM sale_lines WHERE organization_id = $1`, org.ID); n == 0 && b.Key != "restoran" {
			t.Errorf("%s: no sale lines", b.Slug)
		}
		if n := scalar(`SELECT count(*) FROM ledger_entries WHERE organization_id = $1 AND kind = 'collection'`, org.ID); n == 0 && b.Key != "restoran" {
			t.Errorf("%s: no collections", b.Slug)
		}
	}

	// The panel customer is known to the hairdresser and the clinic, each
	// with visits; the other businesses do not know her.
	p, err := phone.Normalize(demo.PanelPhone, "")
	if err != nil {
		t.Fatal(err)
	}
	known := scalar(`SELECT count(DISTINCT o.slug) FROM customers c JOIN organizations o ON o.id = c.organization_id
		WHERE c.phone_e164 = $1 AND o.slug LIKE 'demo-%' AND o.deleted_at IS NULL AND c.deleted_at IS NULL`, p)
	if known != 2 {
		t.Errorf("panel phone known to %d businesses, want 2", known)
	}
	for _, slug := range []string{demo.Slug("kuafor"), demo.Slug("klinik")} {
		n := scalar(`SELECT count(*) FROM reservations r JOIN customers c ON c.id = r.customer_id
			JOIN organizations o ON o.id = r.organization_id WHERE o.slug = $1 AND o.deleted_at IS NULL AND c.phone_e164 = $2`, slug, p)
		if n == 0 {
			t.Errorf("%s: the panel customer has no visits", slug)
		}
	}

	// Access limits between businesses and branches.
	orgs := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	user := func(local string) int64 {
		t.Helper()
		u, err := q.GetUserByEmail(ctx, demo.Email(local))
		if err != nil {
			t.Fatalf("%s: %v", local, err)
		}
		return u.ID
	}
	if _, err := orgs.Resolve(ctx, demo.Slug("klinik"), user("kuafor.sahip")); err == nil {
		t.Error("the hairdresser's owner reached the clinic")
	}
	if _, err := orgs.Resolve(ctx, demo.Slug("kuafor"), user("platform")); err == nil {
		t.Error("the platform account reached a business without a membership")
	}
	owner, err := orgs.Resolve(ctx, demo.Slug("kuafor"), user("kuafor.sahip"))
	if err != nil || owner.BranchIDs != nil {
		t.Errorf("owner scope = %+v, %v", owner.BranchIDs, err)
	}
	manager, err := orgs.Resolve(ctx, demo.Slug("kuafor"), user("kuafor.yonetici"))
	if err != nil || len(manager.BranchIDs) != 1 {
		t.Errorf("manager branches = %v, %v", manager.BranchIDs, err)
	}
	shared, err := orgs.Resolve(ctx, demo.Slug("kuafor"), user("coklu"))
	if err != nil || len(shared.BranchIDs) != 1 || shared.BranchIDs[0] == manager.BranchIDs[0] {
		t.Errorf("shared account in the hairdresser = %v, %v", shared.BranchIDs, err)
	}
	if !shared.Has("tenant.reservations.write") {
		t.Error("reception may not book")
	}
	staff, err := orgs.Resolve(ctx, demo.Slug("guzellik"), user("coklu"))
	if err != nil {
		t.Fatal(err)
	}
	if staff.Has("tenant.reservations.write") || !staff.Has("tenant.reservations.own") {
		t.Errorf("shared account in the beauty salon has %v", staff.Permissions)
	}

	// A second run without reset refuses; with reset it writes again.
	if _, err := demo.New(pool).Run(ctx, demo.Options{Password: "DemoTest1", HistoryDays: 1, FutureDays: 1, Only: []string{"kuafor"}}); !errors.Is(err, demo.ErrExists) {
		t.Errorf("second run err = %v, want ErrExists", err)
	}
	again, err := demo.New(pool).Run(ctx, demo.Options{Password: "DemoTest2", Reset: true, HistoryDays: 1, FutureDays: 1, Only: []string{"kuafor"}})
	if err != nil || len(again.Businesses) != 1 {
		t.Fatalf("reset run = %+v, %v", again.Businesses, err)
	}
}
