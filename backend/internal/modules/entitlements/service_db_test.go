package entitlements

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLimitsAndUsage(t *testing.T) {
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
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "ent-" + suffix, Name: "Ent " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", org.ID) })
	if org.Plan != "starter" || len(org.Addons) != 0 {
		t.Fatalf("new business defaults: %q %v", org.Plan, org.Addons)
	}

	svc := New(q)
	// 2026-10-31 22:30 UTC is already November in Istanbul.
	svc.now = func() time.Time { return time.Date(2026, 10, 31, 22, 30, 0, 0, time.UTC) }
	scope := orgctx.Scope{ID: org.ID, Sector: "hairdresser", Timezone: "Europe/Istanbul", Plan: org.Plan}

	if err := svc.CheckBranchAdd(ctx, scope); err != nil {
		t.Fatalf("first branch fits the starter plan: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO branches (organization_id, code, name) VALUES ($1, 'MRK', 'Merkez')", org.ID); err != nil {
		t.Fatal(err)
	}
	var le *entitlement.LimitError
	if err := svc.CheckBranchAdd(ctx, scope); !errors.As(err, &le) || le.Kind != entitlement.LimitBranches || le.Used != 1 {
		t.Fatalf("second branch over the starter plan: %v", err)
	}
	two := int32(2)
	scope.BranchLimit = &two
	if err := svc.CheckBranchAdd(ctx, scope); err != nil {
		t.Fatalf("override raises the limit: %v", err)
	}
	if err := svc.CheckStaffAdd(ctx, nil, scope); err != nil {
		t.Fatalf("no staff yet: %v", err)
	}

	if err := svc.AddUsage(ctx, scope, MetricSMS, 3); err != nil {
		t.Fatal(err)
	}
	if err := svc.AddUsage(ctx, scope, MetricSMS, 2); err != nil {
		t.Fatal(err)
	}
	if err := svc.AddUsage(ctx, scope, "fax", 1); !errors.Is(err, ErrUnknownMetric) {
		t.Fatalf("unknown metric: %v", err)
	}
	sum, err := svc.Summary(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Period != "2026-11" || sum.Used.Branches != 1 || len(sum.Usage) != len(Metrics) || sum.Usage[0].Metric != MetricSMS || sum.Usage[0].Quantity != 5 {
		t.Fatalf("summary: %+v", sum)
	}
	if sum.BranchLimit == nil || *sum.BranchLimit != 2 || sum.Allows("confirmation") {
		t.Fatalf("effective: %+v", sum.Effective)
	}
}
