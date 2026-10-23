package usecase_test

import (
	"context"
	"os"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/activity/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPlatformActivityExcludesOrganizationEvents: the platform audit feed
// (/v1/platform/activity and its export) lists platform events only; an
// organization's events stay in the tenant resource timelines. Runs with
// TEST_DATABASE_URL.
func TestPlatformActivityExcludesOrganizationEvents(t *testing.T) {
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
	user, err := q.CreateUser(ctx, db.CreateUserParams{
		Email: "activity-" + suffix + "@example.test", PasswordHash: "x", Name: "Act", Surname: "Test", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	var orgID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO organizations (slug, name) VALUES ($1, $2) RETURNING id`,
		"activity-"+suffix, "Activity "+suffix).Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM activity_events WHERE actor_user_id = $1", user.ID)
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", orgID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", user.ID)
	})

	rec := activity.NewRecorder(q, nil)
	uid := user.ID
	rec.Record(ctx, &uid, "platform.thing", "platform.users", nil, nil, nil)
	rec.Record(orgctx.WithOrganizationID(ctx, orgID), &uid, "branch.updated", "tenant.branches", nil, nil, nil)

	items, total, err := usecase.New(q).List(ctx, apiquery.Query{Limit: 50}, &uid, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 || items[0].Action != "platform.thing" {
		t.Fatalf("platform feed = %d items (total %d): %+v, want only platform.thing", len(items), total, items)
	}
	var stamped int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM activity_events WHERE actor_user_id = $1 AND organization_id = $2",
		user.ID, orgID).Scan(&stamped); err != nil || stamped != 1 {
		t.Fatalf("organization event stamped = %d, %v", stamped, err)
	}
}
