package servicebundles_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/servicebundles/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/servicebundles/usecase"
	servicesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
)

// TestServiceBundleEngines (TEC-985): the import creates a bundle from
// service names and branch codes or updates the one with the same name and
// can be rolled back; the export lists the bundles with their contents; the
// bulk actions switch is_active and revert; the Cmd+K document names the
// bundle and its services. Everything stays inside the organization.
func TestServiceBundleEngines(t *testing.T) {
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
	owner, err := q.CreateUser(ctx, db.CreateUserParams{Email: "bundles-" + suffix + "@example.test", PasswordHash: "x", Name: "Owner", Surname: "Test", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Bundles " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", owner.ID)
	})
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	var branchID int64
	var branchCode string
	if err := pool.QueryRow(ctx, `SELECT id, code FROM branches WHERE organization_id = $1 ORDER BY id LIMIT 1`, org.ID).Scan(&branchID, &branchCode); err != nil {
		t.Fatal(err)
	}
	var colorID, blowID int64
	for _, s := range []struct {
		name  string
		price int
		dst   *int64
	}{{"Boya", 800, &colorID}, {"Fön", 400, &blowID}} {
		if err := pool.QueryRow(ctx, `INSERT INTO services (organization_id, name, duration_minutes, price) VALUES ($1, $2, 30, $3) RETURNING id`, org.ID, s.name, s.price).Scan(s.dst); err != nil {
			t.Fatal(err)
		}
		exec(`INSERT INTO service_branches (organization_id, service_id, branch_id) VALUES ($1, $2, $3)`, org.ID, *s.dst, branchID)
	}

	svc := usecase.New(servicesrepository.NewPostgres(pool, q))
	scoped := orgctx.WithOrganizationID(ctx, org.ID)
	io := adapters.NewIO(svc)

	// Import: create from names and codes.
	res, err := io.ApplyRow(scoped, map[string]any{
		"name": "İlkbahar paketi", "services": "Boya + Fön", "price": "1000", "branch_codes": strings.ToLower(branchCode), "sort_order": "2",
	}, nil)
	if err != nil || !res.OK || res.Op != "create" || res.EntityType != "service_bundle" {
		t.Fatalf("import create = %+v, %v", res, err)
	}
	id, err := uuid.Parse(res.EntityUUID)
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Get(ctx, org.ID, id)
	if err != nil || len(b.Items) != 2 || b.Items[0].ServiceName != "Boya" || b.Items[1].ServiceName != "Fön" ||
		len(b.Branches) != 1 || b.Price.String() != "1000.00" || b.SortOrder != 2 {
		t.Fatalf("imported = %+v, %v", b, err)
	}
	// Unknown names and codes are row errors, not crashes.
	if res, err := io.ApplyRow(scoped, map[string]any{"name": "X", "services": "Boya; Yok", "price": "1"}, nil); err != nil || res.OK || !strings.Contains(res.Error, "Yok") {
		t.Fatalf("unknown service = %+v, %v", res, err)
	}
	if res, err := io.ApplyRow(scoped, map[string]any{"name": "X", "services": "Boya;Fön", "price": "1", "branch_codes": "NOPE"}, nil); err != nil || res.OK || !strings.Contains(res.Error, "NOPE") {
		t.Fatalf("unknown branch = %+v, %v", res, err)
	}
	if res, err := io.ApplyRow(scoped, map[string]any{"name": "X", "services": "Boya", "price": "1"}, nil); err != nil || res.OK {
		t.Fatalf("one service = %+v, %v", res, err)
	}
	// Update by name (case-insensitive): the price changes, the contents
	// stay; the revert restores the price.
	res, err = io.ApplyRow(scoped, map[string]any{"name": "ilkbahar PAKETİ", "price": "900,50"}, nil)
	if err != nil || !res.OK || res.Op != "update" || res.EntityUUID != id.String() || res.Previous == nil {
		t.Fatalf("import update = %+v, %v", res, err)
	}
	if b, _ = svc.Get(ctx, org.ID, id); b.Price.String() != "900.50" || len(b.Items) != 2 {
		t.Fatalf("updated = %+v", b)
	}
	if err := io.RevertRow(scoped, res.EntityType, res.EntityUUID, res.Previous); err != nil {
		t.Fatalf("revert update: %v", err)
	}
	if b, _ = svc.Get(ctx, org.ID, id); b.Price.String() != "1000.00" || len(b.Items) != 2 || len(b.Branches) != 1 {
		t.Fatalf("reverted = %+v", b)
	}

	// Export honours the filters.
	ds, err := io.Export(scoped, ioengine.ExportQuery{"is_active": "true"}, "tr")
	if err != nil || len(ds.Rows) != 1 || ds.Rows[0]["services"] != "Boya + Fön" || ds.Rows[0]["price"] != "1000.00" {
		t.Fatalf("export = %+v, %v", ds.Rows, err)
	}
	if ds, err := io.Export(scoped, ioengine.ExportQuery{"show_online": "false"}, "tr"); err != nil || len(ds.Rows) != 0 {
		t.Fatalf("export filtered = %+v, %v", ds.Rows, err)
	}
	// Without an organization scope every call fails.
	if _, err := io.Export(ctx, ioengine.ExportQuery{}, "tr"); err == nil {
		t.Fatal("export without organization accepted")
	}

	// Bulk deactivate / revert, by ids and by query.
	bulk := adapters.NewBulk(svc)
	item, err := bulk.ApplyItem(scoped, "deactivate", id.String())
	if err != nil || !item.OK || item.Previous["is_active"] != true {
		t.Fatalf("deactivate = %+v, %v", item, err)
	}
	if b, _ = svc.Get(ctx, org.ID, id); b.IsActive {
		t.Fatal("still active")
	}
	targets, err := bulk.ResolveTargets(scoped, "activate", bulkengine.BulkTarget{Scope: "query", Query: map[string]string{"is_active": "false"}})
	if err != nil || len(targets) != 1 || targets[0] != id.String() {
		t.Fatalf("targets = %v, %v", targets, err)
	}
	if err := bulk.RevertItem(scoped, "deactivate", id.String(), item.Previous); err != nil {
		t.Fatalf("revert: %v", err)
	}
	if b, _ = svc.Get(ctx, org.ID, id); !b.IsActive {
		t.Fatal("not restored")
	}
	if item, err := bulk.ApplyItem(scoped, "deactivate", uuid.NewString()); err != nil || item.OK {
		t.Fatalf("unknown target = %+v, %v", item, err)
	}

	// Search document.
	search := adapters.NewSearch(q)
	doc, err := search.Document(ctx, searchengine.TenantDocumentID(org.ID, id.String()))
	if err != nil || doc.Title != "İlkbahar paketi" || doc.Subtitle != "Boya + Fön" || doc.OrganizationID != org.ID ||
		!strings.Contains(doc.Href, "/service-bundles/"+id.String()) {
		t.Fatalf("document = %+v, %v", doc, err)
	}
	if _, err := search.Document(ctx, searchengine.TenantDocumentID(org.ID+1, id.String())); err == nil {
		t.Fatal("document of another organization answered")
	}

	// Revert of an imported create deletes the bundle.
	if err := io.RevertRow(scoped, "service_bundle", id.String(), nil); err != nil {
		t.Fatalf("revert create: %v", err)
	}
	if _, err := svc.Get(ctx, org.ID, id); err == nil {
		t.Fatal("bundle still there after revert")
	}
}
