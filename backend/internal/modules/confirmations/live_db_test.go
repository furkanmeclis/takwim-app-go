package confirmations_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/catalog"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/channel"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// liveEvents records the live list events of the notification engine.
type liveEvents struct {
	mu   sync.Mutex
	org  int64
	seen []string
}

func (l *liveEvents) Publish(ctx context.Context, resource, action string, uuids ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if org, ok := orgctx.OrganizationID(ctx); !ok || org != l.org {
		l.seen = append(l.seen, "wrong organization")
		return
	}
	for _, id := range uuids {
		l.seen = append(l.seen, resource+" "+action+" "+id)
	}
}

func (l *liveEvents) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.seen
	l.seen = nil
	return out
}

// reportingSMS accepts every message and reports later (like WhatsApp).
type reportingSMS struct {
	mu  sync.Mutex
	ids []uuid.UUID
}

func (c *reportingSMS) Name() string                 { return channel.SMS }
func (c *reportingSMS) Ready() bool                  { return true }
func (c *reportingSMS) SetReporter(channel.Reporter) {}

func (c *reportingSMS) Send(_ context.Context, m channel.Message) (channel.Result, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ids = append(c.ids, m.DeliveryUUID)
	return channel.Result{Outcome: channel.OutcomeAccepted, Provider: "fake", ProviderRef: m.DeliveryUUID.String()}, nil
}

func expectLive(t *testing.T, l *liveEvents, r resmodel.Reservation, n int, what string) {
	t.Helper()
	got := l.take()
	want := resmodel.Slug + " " + resourceevents.ActionUpdated + " " + r.UUID.String()
	if len(got) != n {
		t.Fatalf("%s: events = %v, want %d of %q", what, got, n, want)
	}
	for _, g := range got {
		if g != want {
			t.Fatalf("%s: event %q, want %q", what, g, want)
		}
	}
}

// TestSendOutcomesAreLive (TEC-981): a confirmation request that could
// not be sent, and the delivery reports of one that was accepted, tell the
// reservations live channel; other notices do not.
func TestSendOutcomesAreLive(t *testing.T) {
	t.Run("skipped send", func(t *testing.T) {
		f := newFixture(t, true)
		live := &liveEvents{org: f.org.ID}
		f.notify.SetResourceEvents(live, catalog.ConfirmationRequest)
		f.sms.ready = false
		r := f.book(t, at(7, 14, 0))
		// The booking notice is skipped as well, but it is not a request.
		if _, err := f.notify.ProcessDueOrganization(context.Background(), f.org.ID); err != nil {
			t.Fatal(err)
		}
		expectLive(t, live, r, 0, "booking notice")
		f.tick(t, at(6, 14, 0))
		if f.requests(r) != 1 {
			t.Fatalf("requests = %d", f.requests(r))
		}
		if n := f.count(`SELECT count(*) FROM customer_notification_dispatches WHERE organization_id = $1
			AND event_code = $2 AND status = 'skipped'`, f.org.ID, catalog.ConfirmationRequest); n != 1 {
			t.Fatalf("skipped requests = %d", n)
		}
		expectLive(t, live, r, 1, "skipped request")
		// Nothing left to send: nothing published.
		f.tick(t, at(6, 14, 1))
		expectLive(t, live, r, 0, "idle tick")
	})

	t.Run("delivery reports", func(t *testing.T) {
		f := newFixture(t, true)
		live := &liveEvents{org: f.org.ID}
		f.notify.SetResourceEvents(live, catalog.ConfirmationRequest)
		ch := &reportingSMS{}
		f.notify.Register(ch)
		r := f.book(t, at(7, 14, 0))
		if _, err := f.notify.ProcessDueOrganization(context.Background(), f.org.ID); err != nil {
			t.Fatal(err)
		}
		live.take()
		f.tick(t, at(6, 14, 0))
		expectLive(t, live, r, 1, "accepted request")
		ch.mu.Lock()
		last := ch.ids[len(ch.ids)-1]
		ch.mu.Unlock()
		ctx := context.Background()
		if _, _, err := f.notify.RecordDelivery(ctx, channel.Report{
			Channel: channel.SMS, OrganizationID: f.org.ID, DeliveryUUID: &last, Error: "undeliverable",
		}); err != nil {
			t.Fatal(err)
		}
		expectLive(t, live, r, 1, "failed report")
		if n := f.count(`SELECT count(*) FROM customer_notification_deliveries WHERE organization_id = $1
			AND uuid = $2 AND status = 'failed'`, f.org.ID, last); n != 1 {
			t.Fatalf("failed attempts = %d", n)
		}
		// A late read receipt is shown on the attempt too.
		if _, _, err := f.notify.RecordDelivery(ctx, channel.Report{
			Channel: channel.SMS, OrganizationID: f.org.ID, DeliveryUUID: &last, Read: true,
		}); err != nil {
			t.Fatal(err)
		}
		expectLive(t, live, r, 1, "late receipt")
		// A report of an unknown message publishes nothing.
		unknown := uuid.New()
		if _, _, err := f.notify.RecordDelivery(ctx, channel.Report{
			Channel: channel.SMS, OrganizationID: f.org.ID, DeliveryUUID: &unknown,
		}); err != nil {
			t.Fatal(err)
		}
		expectLive(t, live, r, 0, "unknown report")
	})

	t.Run("report in the worker", func(t *testing.T) {
		f := newFixture(t, true)
		live := &liveEvents{org: f.org.ID}
		f.notify.SetResourceEvents(live, catalog.ConfirmationRequest)
		ch := &reportingSMS{}
		f.notify.Register(ch)
		r := f.book(t, at(7, 14, 0))
		f.tick(t, at(6, 14, 0))
		live.take()
		ch.mu.Lock()
		last := ch.ids[len(ch.ids)-1]
		ch.mu.Unlock()
		// The failure and the next channel's result are one event.
		f.notify.ReportDelivery(context.Background(), channel.Report{
			Channel: channel.SMS, OrganizationID: f.org.ID, DeliveryUUID: &last, Error: "no delivery report",
		})
		expectLive(t, live, r, 1, "reported failure")
	})
}
