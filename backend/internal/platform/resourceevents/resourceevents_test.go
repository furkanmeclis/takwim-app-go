package resourceevents_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/realtime"
)

type published struct {
	channel string
	event   resourceevents.Event
}

type fakePublisher struct {
	mu    sync.Mutex
	calls []published
	err   error
}

func (f *fakePublisher) Publish(_ context.Context, channel string, data any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	ev, _ := data.(resourceevents.Event)
	f.calls = append(f.calls, published{channel: channel, event: ev})
	return f.err
}

func (f *fakePublisher) snapshot() []published {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]published(nil), f.calls...)
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestPublishSendsEventOnDefaultChannel(t *testing.T) {
	t.Parallel()
	fake := &fakePublisher{}
	b := resourceevents.New(fake, quietLog())
	if !b.Enabled() {
		t.Fatal("broadcaster should be enabled with a real publisher")
	}
	b.Publish(context.Background(), "platform.users", resourceevents.ActionCreated, "u-1")
	b.Wait()

	calls := fake.snapshot()
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}
	got := calls[0]
	if got.channel != "resource:platform.users" {
		t.Fatalf("channel = %q", got.channel)
	}
	if got.event.Type != resourceevents.EventType || got.event.Resource != "platform.users" ||
		got.event.Action != resourceevents.ActionCreated {
		t.Fatalf("event = %+v", got.event)
	}
	if len(got.event.UUIDs) != 1 || got.event.UUIDs[0] != "u-1" {
		t.Fatalf("uuids = %v", got.event.UUIDs)
	}
	if got.event.At.IsZero() {
		t.Fatal("at must be set")
	}
}

func TestPublishIsNoopWhenRealtimeDisabled(t *testing.T) {
	t.Parallel()
	disabled := []realtime.Publisher{
		nil,
		realtime.NoopPublisher{},
		realtime.NewCentrifugoClient(config.CentrifugoConfig{Enabled: false}),
	}
	for i, pub := range disabled {
		b := resourceevents.New(pub, quietLog())
		if b.Enabled() {
			t.Fatalf("case %d: broadcaster should be disabled", i)
		}
		// Must not panic or block.
		b.Publish(context.Background(), "platform.users", resourceevents.ActionUpdated, "u-1")
		b.Wait()
	}

	var nilB *resourceevents.Broadcaster
	nilB.Publish(context.Background(), "platform.users", resourceevents.ActionDeleted, "u-1")
	nilB.Wait()
	resourceevents.Nop{}.Publish(context.Background(), "x", "y")
}

func TestPublishErrorIsSwallowed(t *testing.T) {
	t.Parallel()
	fake := &fakePublisher{err: errors.New("centrifugo down")}
	b := resourceevents.New(fake, quietLog())
	b.Publish(context.Background(), "platform.roles", resourceevents.ActionDeleted, "r-1")
	b.Wait()
	if n := len(fake.snapshot()); n != 1 {
		t.Fatalf("calls = %d, want 1", n)
	}
}

func TestPublishSurvivesCanceledRequestContext(t *testing.T) {
	t.Parallel()
	fake := &fakePublisher{}
	b := resourceevents.New(fake, quietLog())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b.Publish(ctx, "platform.users", resourceevents.ActionUpdated, "u-1")
	b.Wait()
	if n := len(fake.snapshot()); n != 1 {
		t.Fatalf("calls = %d, want 1", n)
	}
}

func TestBulkEventCapsAndDedupesUUIDs(t *testing.T) {
	t.Parallel()
	fake := &fakePublisher{}
	b := resourceevents.New(fake, quietLog())
	ids := []string{"dup", "dup", ""}
	for i := 0; i < resourceevents.MaxUUIDs+50; i++ {
		ids = append(ids, fmt.Sprintf("id-%d", i))
	}
	b.Publish(context.Background(), "platform.users", resourceevents.ActionBulk, ids...)
	b.Wait()
	ev := fake.snapshot()[0].event
	if len(ev.UUIDs) != resourceevents.MaxUUIDs {
		t.Fatalf("uuids = %d, want %d", len(ev.UUIDs), resourceevents.MaxUUIDs)
	}
	if !ev.Truncated {
		t.Fatal("truncated should be set")
	}
	if ev.UUIDs[0] != "dup" || ev.UUIDs[1] != "id-0" {
		t.Fatalf("unexpected head %v", ev.UUIDs[:2])
	}
}

func TestChannelResolverHooks(t *testing.T) {
	t.Parallel()
	fake := &fakePublisher{}
	b := resourceevents.New(fake, quietLog())
	b.AddResolver(func(_ context.Context, resource string) (string, bool, error) {
		switch resource {
		case "scoped.things":
			return "acct:abc:resource:scoped.things", true, nil
		case "scoped.skip":
			return "", true, nil
		case "scoped.err":
			return "", true, errors.New("boom")
		}
		return "", false, nil
	})
	ctx := context.Background()
	b.Publish(ctx, "scoped.things", resourceevents.ActionUpdated, "a")
	b.Publish(ctx, "scoped.skip", resourceevents.ActionUpdated, "b")
	b.Publish(ctx, "scoped.err", resourceevents.ActionUpdated, "c")
	b.Publish(ctx, "platform.roles", resourceevents.ActionUpdated, "d")
	b.Wait()

	channels := map[string]bool{}
	for _, c := range fake.snapshot() {
		channels[c.channel] = true
	}
	if len(channels) != 2 || !channels["acct:abc:resource:scoped.things"] || !channels["resource:platform.roles"] {
		t.Fatalf("channels = %v", channels)
	}
}

func TestParseChannel(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"resource:platform.users": true,
		"resource:":               false,
		"resource:Platform":       false,
		"resource:a b":            false,
		"user:123":                false,
	}
	for ch, want := range cases {
		if _, ok := resourceevents.ParseChannel(ch); ok != want {
			t.Errorf("ParseChannel(%q) = %v, want %v", ch, ok, want)
		}
	}
}
