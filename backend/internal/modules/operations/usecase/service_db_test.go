package usecase

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
)

// TestOperationsQueries runs the platform lists and counts over real SQL:
// failed work of two organizations and a platform job shows up in one list,
// with the organization's name and nothing of the customer. Runs when
// TEST_DATABASE_URL points at a migrated database.
func TestOperationsQueries(t *testing.T) {
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
	s := New(q, nil, nil, Tracking{})
	suffix := uuid.NewString()[:8]

	before, err := s.Summary(ctx)
	if err != nil {
		t.Fatal(err)
	}

	newOrg := func(name string) int64 {
		var id int64
		if err := pool.QueryRow(ctx, "INSERT INTO organizations (slug, name) VALUES ($1, $2) RETURNING id",
			"ops-"+name+"-"+suffix, "Ops "+name+" "+suffix).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	orgA, orgB := newOrg("a"), newOrg("b")
	platformKey := "ops-platform-" + suffix
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM integration_jobs WHERE idempotency_key = $1", platformKey)
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{orgA, orgB})
	})

	job := func(org *int64, key, status, errText string) {
		if _, err := pool.Exec(ctx, `INSERT INTO integration_jobs (organization_id, direction, provider, kind, idempotency_key, status, last_error)
			VALUES ($1, 'outbound', 'log', 'sms.send', $2, $3, $4)`, org, key, status, errText); err != nil {
			t.Fatal(err)
		}
	}
	job(&orgA, "ops-a-failed-"+suffix, "failed", "provider said no "+suffix)
	job(&orgB, "ops-b-verify-"+suffix, "needs_verification", "timeout "+suffix)
	job(&orgB, "ops-b-ok-"+suffix, "succeeded", "")
	job(nil, platformKey, "blocked", "missing data "+suffix)

	// A failed notice send of B (customer, dispatch, delivery).
	var customerID, dispatchID int64
	if err := pool.QueryRow(ctx, "INSERT INTO customers (organization_id, first_name) VALUES ($1, 'Ayşe') RETURNING id", orgB).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO customer_notification_dispatches (organization_id, event_code, customer_id, idempotency_key, status, last_error)
		VALUES ($1, 'reservation.created', $2, $3, 'failed', 'sms: rejected') RETURNING id`, orgB, customerID, "ops-"+suffix).Scan(&dispatchID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO customer_notification_deliveries (organization_id, dispatch_id, channel, attempt_no, status, provider, recipient_masked, body, error)
		VALUES ($1, $2, 'sms', 1, 'failed', 'log', '+90 5** *** ** 12', 'secret text', $3)`, orgB, dispatchID, "rejected "+suffix); err != nil {
		t.Fatal(err)
	}

	jobs, total, err := s.IntegrationJobs(ctx, IntegrationJobFilter{Q: suffix, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(jobs) != 3 {
		t.Fatalf("jobs = %d %+v (succeeded jobs are not listed by default)", total, jobs)
	}
	var sawPlatform, sawA bool
	for _, j := range jobs {
		switch {
		case j.Organization == nil:
			sawPlatform = j.Status == "blocked"
		case j.Organization.Name == "Ops a "+suffix:
			sawA = j.Status == "failed" && j.Retryable
		}
	}
	if !sawPlatform || !sawA {
		t.Fatalf("platform %v, org A %v: %+v", sawPlatform, sawA, jobs)
	}
	ok, total, err := s.IntegrationJobs(ctx, IntegrationJobFilter{Statuses: []string{"succeeded"}, Q: suffix, Limit: 50})
	if err != nil || total != 1 || ok[0].Organization.Name != "Ops b "+suffix {
		t.Fatalf("succeeded = %d %+v %v", total, ok, err)
	}
	if _, total, _ := s.IntegrationJobs(ctx, IntegrationJobFilter{Providers: []string{"nope"}, Q: suffix, Limit: 50}); total != 0 {
		t.Fatalf("provider filter total = %d", total)
	}

	// The platform job's scope is 0, an organization job's its id.
	var platformUUID uuid.UUID
	if err := pool.QueryRow(ctx, "SELECT uuid FROM integration_jobs WHERE idempotency_key = $1", platformKey).Scan(&platformUUID); err != nil {
		t.Fatal(err)
	}
	if org, err := q.GetPlatformIntegrationJobScope(ctx, platformUUID); err != nil || org != 0 {
		t.Fatalf("platform scope = %d, %v", org, err)
	}

	notices, total, err := s.NoticeFailures(ctx, NoticeFailureFilter{Q: suffix, SortDesc: true, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || notices[0].Organization.Name != "Ops b "+suffix || notices[0].Channel != "sms" ||
		notices[0].EventCode != "reservation.created" || notices[0].Error != "rejected "+suffix {
		t.Fatalf("notices = %+v", notices)
	}
	if _, total, _ := s.NoticeFailures(ctx, NoticeFailureFilter{Channels: []string{"email"}, Q: suffix, Limit: 50}); total != 0 {
		t.Fatalf("channel filter total = %d", total)
	}

	after, err := s.Summary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Integrations.Failed-before.Integrations.Failed != 1 ||
		after.Integrations.NeedsVerification-before.Integrations.NeedsVerification != 1 ||
		after.Integrations.Blocked-before.Integrations.Blocked != 1 ||
		after.Notices.Failed24h-before.Notices.Failed24h != 1 || after.Status != "attention" {
		t.Fatalf("before %+v after %+v", before, after)
	}
}
