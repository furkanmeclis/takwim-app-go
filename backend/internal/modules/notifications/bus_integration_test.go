package notifications_test

import (
	"context"
	"os"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	notifmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/providers"
	notifusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestBusEventOrganizationStampsNotification: an event stamped with an
// organization (events.Event.WithOrganization) produces an
// organization-stamped notification, so it shows in that organization's
// panel only. Runs with TEST_DATABASE_URL.
func TestBusEventOrganizationStampsNotification(t *testing.T) {
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
		Email: "bus-" + suffix + "@example.test", PasswordHash: "x", Name: "Bus", Surname: "Test", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	var orgID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, $2) RETURNING id`,
		"bus-"+suffix, "Bus "+suffix).Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM notifications WHERE user_id = $1", user.ID)
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", orgID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", user.ID)
	})

	bus := events.NewBus(nil)
	svc := notifusecase.New(q, nil, []providers.Provider{providers.InappProvider{}}, nil)
	notifmodule.RegisterEventHandlers(bus, svc, nil)
	publish := func(ev events.Event) {
		t.Helper()
		if err := bus.Publish(ctx, ev.WithPayload(map[string]any{"assigned_user_id": user.ID})); err != nil {
			t.Fatal(err)
		}
	}
	publish(events.New(events.ConversationsAssigned).WithOrganization(orgID))
	publish(events.New(events.ConversationsAssigned))

	rows, err := pool.Query(ctx, "SELECT organization_id FROM notifications WHERE user_id = $1 ORDER BY id", user.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []*int64
	for rows.Next() {
		var v *int64
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		got = append(got, v)
	}
	if len(got) != 2 || got[0] == nil || *got[0] != orgID || got[1] != nil {
		t.Fatalf("notification organizations = %v, want [%d, nil]", got, orgID)
	}
}
