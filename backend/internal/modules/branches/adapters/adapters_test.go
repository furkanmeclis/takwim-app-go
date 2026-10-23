package adapters

import (
	"context"
	"os"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAdaptersNeedOrganization(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	io := NewIO(usecase.New(nil, nil))
	if _, err := io.Export(ctx, ioengine.ExportQuery{}, "tr"); err == nil {
		t.Fatal("export without organization must fail")
	}
	if _, err := io.ApplyRow(ctx, map[string]any{"code": "X", "name": "Y"}, nil); err == nil {
		t.Fatal("import without organization must fail")
	}
	bulk := NewBulk(usecase.New(nil, nil))
	if _, err := bulk.ResolveTargets(ctx, "activate", bulkengine.BulkTarget{Scope: "ids", IDs: []string{uuid.NewString()}}); err == nil {
		t.Fatal("bulk without organization must fail")
	}
	if got := SplitCities(" İstanbul, ,Ankara "); len(got) != 2 || got[0] != "İstanbul" || got[1] != "Ankara" {
		t.Fatalf("SplitCities = %v", got)
	}
}

// TestIOAdapterRoundTrip runs against a migrated database when
// TEST_DATABASE_URL is set (skipped otherwise).
func TestIOAdapterRoundTrip(t *testing.T) {
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
	orgA, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "io-a-" + suffix, Name: "A"})
	if err != nil {
		t.Fatal(err)
	}
	orgB, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "io-b-" + suffix, Name: "B"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = ANY($1)", []int64{orgA.ID, orgB.ID})
	})

	io := NewIO(usecase.New(repository.NewPostgres(q), nil))
	ctxA := orgctx.WithOrganizationID(ctx, orgA.ID)
	ctxB := orgctx.WithOrganizationID(ctx, orgB.ID)

	created, err := io.ApplyRow(ctxA, map[string]any{
		"code": "ank", "name": "Ankara", "city": "Ankara", "latitude": "39,93", "longitude": "32.85", "is_active": "evet",
	}, map[string]any{"prep_minutes": "30"})
	if err != nil || !created.OK || created.Op != "create" {
		t.Fatalf("create row = %+v, %v", created, err)
	}
	if bad, _ := io.ApplyRow(ctxA, map[string]any{"code": "", "name": "No code"}, nil); bad.OK {
		t.Fatal("row without code must fail")
	}
	if _, err := io.ApplyRow(ctxB, map[string]any{"code": "ANK", "name": "B Ankara", "city": "Ankara"}, nil); err != nil {
		t.Fatal(err)
	}
	updated, err := io.ApplyRow(ctxA, map[string]any{"code": "ANK", "name": "Ankara Merkez"}, nil)
	if err != nil || !updated.OK || updated.Op != "update" || updated.EntityUUID != created.EntityUUID || updated.Previous["name"] != "Ankara" {
		t.Fatalf("update row = %+v, %v", updated, err)
	}

	ds, err := io.Export(ctxA, ioengine.ExportQuery{"city": "Ankara"}, "tr")
	if err != nil || len(ds.Rows) != 1 || ds.Rows[0]["name"] != "Ankara Merkez" || ds.Rows[0]["prep_minutes"] != "30" {
		t.Fatalf("export A = %+v, %v", ds.Rows, err)
	}

	if err := io.RevertRow(ctxA, "branch", updated.EntityUUID, updated.Previous); err != nil {
		t.Fatalf("revert update: %v", err)
	}
	ds, _ = io.Export(ctxA, ioengine.ExportQuery{}, "tr")
	if len(ds.Rows) != 1 || ds.Rows[0]["name"] != "Ankara" {
		t.Fatalf("after revert = %+v", ds.Rows)
	}
	// Reverting from another organization cannot touch A's branch.
	if err := io.RevertRow(ctxB, "branch", created.EntityUUID, nil); err == nil {
		t.Fatal("org B revert of org A branch must fail")
	}
	if err := io.RevertRow(ctxA, "branch", created.EntityUUID, nil); err != nil {
		t.Fatalf("revert create: %v", err)
	}
	ds, _ = io.Export(ctxA, ioengine.ExportQuery{}, "tr")
	if len(ds.Rows) != 0 {
		t.Fatalf("A must be empty after reverting the create, got %+v", ds.Rows)
	}
	ds, _ = io.Export(ctxB, ioengine.ExportQuery{}, "tr")
	if len(ds.Rows) != 1 || ds.Rows[0]["name"] != "B Ankara" {
		t.Fatalf("B untouched = %+v", ds.Rows)
	}
}
