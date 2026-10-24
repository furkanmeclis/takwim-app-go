package orgoverview

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/entitlements"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type readySet map[string]bool

func (r readySet) Ready(ch string) bool { return r[ch] }

type payReady bool

func (p payReady) OnlineReady(context.Context, int64) bool { return bool(p) }

func TestOverview(t *testing.T) {
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
	newOrg := func(prefix string) db.Organization {
		suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
		org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: prefix + "-" + suffix, Name: prefix + " " + suffix})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", org.ID) })
		return org
	}
	org := newOrg("ovw")
	other := newOrg("ovx")
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	seed := func(orgID int64) {
		var branchID, customerID int64
		if err := pool.QueryRow(ctx, "INSERT INTO branches (organization_id, code, name) VALUES ($1, 'MRK', 'Merkez') RETURNING id", orgID).Scan(&branchID); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, "INSERT INTO customers (organization_id, first_name) VALUES ($1, 'Ayşe') RETURNING id", orgID).Scan(&customerID); err != nil {
			t.Fatal(err)
		}
		res := func(n int, status string, created, starts time.Time) {
			exec(`INSERT INTO reservations (organization_id, number, branch_id, customer_id, status, approval_mode, approval_status,
				payment_status, source, starts_at, ends_at, currency, created_at)
				VALUES ($1, $2, $3, $4, $5, 'auto', 'accepted', 'on_site', 'panel', $6, $6::timestamptz + interval '1 hour', 'TRY', $7)`,
				orgID, n, branchID, customerID, status, starts, created)
		}
		now := time.Now()
		res(1, "active", now, now.Add(48*time.Hour))
		res(2, "cancelled", now, now.Add(72*time.Hour))
		res(3, "completed", now.AddDate(0, -1, 0), now.AddDate(0, -1, 0))
		res(4, "completed", now.AddDate(-2, 0, 0), now.AddDate(-2, 0, 0))
	}
	seed(org.ID)
	seed(other.ID)
	exec(`INSERT INTO integration_accounts (organization_id, kind, provider, display_name, mode, status, credentials_ciphertext, last_check_status)
		VALUES ($1, 'payment', 'testpay', 'Test', 'sandbox', 'active', 'v1:x', 'ok'),
		       ($1, 'sms', 'netgsm', 'SMS', 'live', 'disabled', 'v1:y', 'failed')`, org.ID)

	ent := entitlements.New(q)
	scope := orgctx.Scope{ID: org.ID, Sector: "hairdresser", Timezone: "Europe/Istanbul", Plan: "starter", Addons: []string{"confirmation"}}
	if err := ent.AddUsage(ctx, scope, entitlements.MetricSMS, 4); err != nil {
		t.Fatal(err)
	}

	svc := New(q, ent, readySet{"email": true}, payReady(false))
	out, err := svc.Get(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	r := out.Reservations
	if r.Total != 4 || r.ThisMonth != 2 || r.PreviousMonth != 1 || r.CancelledThisMonth != 1 || r.Upcoming != 1 {
		t.Fatalf("reservations: %+v", r)
	}
	if len(r.Monthly) != volumeMonths || r.Monthly[volumeMonths-1].Period != out.Period || r.Monthly[volumeMonths-1].Reservations != 2 ||
		r.Monthly[volumeMonths-2].Period != out.PreviousPeriod || r.Monthly[volumeMonths-2].Reservations != 1 {
		t.Fatalf("monthly: %+v (period %s)", r.Monthly, out.Period)
	}
	if out.Catalogue.Customers != 1 || out.Entitlements.Used.Branches != 1 {
		t.Fatalf("counts: %+v %+v", out.Catalogue, out.Entitlements.Used)
	}
	if !out.Confirmation.Granted || !out.Confirmation.Active || !out.Confirmation.SectorOffers {
		t.Fatalf("confirmation: %+v", out.Confirmation)
	}
	want := map[string]string{"sms": ChannelNotReady, "whatsapp": ChannelNotReady, "email": ChannelReady, "voice_call": ChannelNotReady}
	if len(out.Channels) != len(want) {
		t.Fatalf("channels: %+v", out.Channels)
	}
	for _, c := range out.Channels {
		if want[c.Channel] != c.Status {
			t.Fatalf("channel %s: %s", c.Channel, c.Status)
		}
		if c.Channel == "sms" && (c.Usage != 4 || c.Account == nil || c.Account.Status != "disabled") {
			t.Fatalf("sms: %+v", c)
		}
	}
	if out.Payment.Status != PaymentNoCashAccount || out.Payment.Account == nil || out.Payment.Account.Provider != "testpay" {
		t.Fatalf("payment: %+v", out.Payment)
	}

	// Without the add-on the right is off; the other business has no
	// accounts.
	otherScope := orgctx.Scope{ID: other.ID, Sector: "hairdresser", Timezone: "Europe/Istanbul", Plan: "starter"}
	out, err = New(q, ent, nil, nil).Get(ctx, otherScope)
	if err != nil {
		t.Fatal(err)
	}
	if out.Confirmation.Granted || out.Confirmation.Active || out.Payment.Status != PaymentNotConnected || out.Reservations.Total != 4 {
		t.Fatalf("other: %+v %+v", out.Confirmation, out.Payment)
	}
	for _, c := range out.Channels {
		if c.Status == ChannelReady || c.Account != nil || c.Usage != 0 {
			t.Fatalf("other channel: %+v", c)
		}
	}
}
