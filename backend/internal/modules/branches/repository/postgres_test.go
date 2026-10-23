package repository

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPostgresIsolation runs against a migrated database when
// TEST_DATABASE_URL is set (skipped otherwise). It proves the SQL itself
// keeps organizations apart, not only the use case.
func TestPostgresIsolation(t *testing.T) {
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
	orgA, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "branches-a-" + suffix, Name: "A"})
	if err != nil {
		t.Fatal(err)
	}
	orgB, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "branches-b-" + suffix, Name: "B"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = ANY($1)", []int64{orgA.ID, orgB.ID})
	})

	repo := NewPostgres(q)
	fields := BranchFields{Code: "MRK", Name: "Merkez", WorkingHours: []byte("{}"), HolidayExceptions: []byte("[]"), PrepMinutes: 60, IsActive: true}
	a, err := repo.CreateBranch(ctx, orgA.ID, fields)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateBranch(ctx, orgA.ID, fields); !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("duplicate code in org A: err = %v, want ErrCodeTaken", err)
	}
	b, err := repo.CreateBranch(ctx, orgB.ID, fields)
	if err != nil {
		t.Fatalf("same code in org B must be allowed: %v", err)
	}

	if _, err := repo.GetBranch(ctx, orgB.ID, a.Uuid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("org B reads org A branch: err = %v", err)
	}
	fields.Name = "hijack"
	if _, err := repo.UpdateBranch(ctx, orgB.ID, a.Uuid, fields); !errors.Is(err, ErrNotFound) {
		t.Fatalf("org B updates org A branch: err = %v", err)
	}
	if err := repo.DeleteBranch(ctx, orgB.ID, a.Uuid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("org B deletes org A branch: err = %v", err)
	}
	rows, total, err := repo.ListBranches(ctx, orgA.ID, model.ListFilter{Limit: 50, SortField: "code"})
	if err != nil || total != 1 || len(rows) != 1 || rows[0].Uuid != a.Uuid {
		t.Fatalf("org A list = %d rows (total %d), err %v", len(rows), total, err)
	}

	point, err := repo.CreateServicePoint(ctx, orgA.ID, a.ID, ServicePointFields{Type: "office", Name: "Desk", OnlineSelectable: true, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	// The composite foreign key refuses a point that pairs org B with org A's branch.
	if _, err := repo.CreateServicePoint(ctx, orgB.ID, a.ID, ServicePointFields{Type: "office", Name: "Cross"}); err == nil {
		t.Fatal("service point across organizations must be refused by the database")
	}
	if _, err := repo.GetServicePoint(ctx, orgA.ID, b.ID, point.Uuid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("point through another branch id: err = %v", err)
	}
	if rows, _, _ := repo.ListBranches(ctx, orgA.ID, model.ListFilter{Limit: 50}); len(rows) != 1 || rows[0].ServicePointsCount != 1 {
		t.Fatalf("service point count = %+v", rows)
	}
}
