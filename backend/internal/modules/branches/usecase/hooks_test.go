package usecase

import (
	"context"
	"os"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/repository"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestOrganizationHooksSeedDefaultBranch runs against a migrated database
// when TEST_DATABASE_URL is set (skipped otherwise).
func TestOrganizationHooksSeedDefaultBranch(t *testing.T) {
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

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	org, err := q.WithTx(tx).CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: "hook-" + uuid.NewString()[:8], Name: "Hook",
	})
	if err != nil {
		t.Fatal(err)
	}
	idx := &fakeIndexer{}
	hooks := NewOrganizationHooks(idx)
	if err := hooks.OnCreated(ctx, tx, orgmodel.Organization{ID: org.ID, UUID: org.Uuid}, orgmodel.User{}); err != nil {
		t.Fatalf("OnCreated: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", org.ID)
	})

	svc := New(repository.NewPostgres(q), nil)
	items, total, err := svc.List(ctx, org.ID, model.ListFilter{Limit: 20})
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("default branches = %d (total %d), err %v", len(items), total, err)
	}
	b := items[0]
	if b.Code != "MRK" || b.Name != "Merkez" || !b.IsActive || b.PrepMinutes != 60 || !b.WorkingHours.Sun.Closed {
		t.Fatalf("unexpected default branch: %+v", b)
	}
	if len(idx.upserts) != 1 {
		t.Fatalf("default branch must be queued for search indexing, got %v", idx.upserts)
	}
}
