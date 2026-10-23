// Package resourceevents publishes "a list resource changed" events to
// Centrifugo so open DataTables can refresh themselves.
//
// Modules call Publisher.Publish after a successful mutation. Publishing is
// fail-soft and asynchronous: it never fails or slows the request, and it is
// a no-op when realtime is disabled.
//
// Channels default to "resource:{name}". A host app scopes channels (for
// example per account) by adding a ChannelResolver; subscriptions are
// authorized by Registry.ChannelRule against the read permission each module
// registers for its resource.
package resourceevents

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/realtime"
)

// EventType is the `type` field of every resource-change publication.
const EventType = "resource.changed"

// Actions carried by an Event.
const (
	ActionCreated = "created"
	ActionUpdated = "updated"
	ActionDeleted = "deleted"
	// ActionBulk is one event for a whole bulk or import job. Without uuids
	// it means "an unknown set of rows changed" (clients refetch).
	ActionBulk = "bulk"
)

// ChannelPrefix is the Centrifugo namespace of unscoped resource channels.
const ChannelPrefix = "resource:"

// MaxUUIDs caps the uuids carried by one event. Larger changes set
// Truncated; clients then treat every visible row as changed.
const MaxUUIDs = 200

// publishTimeout bounds one Centrifugo call made in the background.
const publishTimeout = 5 * time.Second

// Event is the payload published on a resource channel.
type Event struct {
	Type      string    `json:"type"`
	Resource  string    `json:"resource"`
	Action    string    `json:"action"`
	UUIDs     []string  `json:"uuids"`
	Truncated bool      `json:"truncated,omitempty"`
	At        time.Time `json:"at"`
}

// Publisher is what modules depend on. Implementations must not block the
// caller on network I/O and must swallow (log) every error.
type Publisher interface {
	Publish(ctx context.Context, resource, action string, uuids ...string)
}

// Nop discards every event (tests, realtime disabled).
type Nop struct{}

// Publish implements Publisher.
func (Nop) Publish(context.Context, string, string, ...string) {}

// DefaultChannel returns the unscoped channel of a resource.
func DefaultChannel(resource string) string {
	return ChannelPrefix + resource
}

// ParseChannel returns the resource of a "resource:{name}" channel.
func ParseChannel(channel string) (string, bool) {
	if !strings.HasPrefix(channel, ChannelPrefix) {
		return "", false
	}
	name := strings.TrimPrefix(channel, ChannelPrefix)
	if !ValidResourceName(name) {
		return "", false
	}
	return name, true
}

// ValidResourceName accepts lowercase slugs such as "platform.users".
func ValidResourceName(name string) bool {
	if name == "" || len(name) > 100 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

// ChannelResolver lets a host app pick the channel of an event. It returns
// handled=false to pass the resource on to the next resolver (and finally to
// DefaultChannel). handled=true with an empty channel skips publishing.
type ChannelResolver func(ctx context.Context, resource string) (channel string, handled bool, err error)

// Broadcaster publishes events through a realtime.Publisher.
type Broadcaster struct {
	pub       realtime.Publisher
	log       *slog.Logger
	now       func() time.Time
	mu        sync.RWMutex
	resolvers []ChannelResolver
	inflight  sync.WaitGroup
}

// New returns a Broadcaster. A nil or Noop realtime publisher yields a
// Broadcaster whose Publish does nothing.
func New(pub realtime.Publisher, log *slog.Logger) *Broadcaster {
	if log == nil {
		log = slog.Default()
	}
	if !realtimeEnabled(pub) {
		pub = nil
	}
	return &Broadcaster{pub: pub, log: log, now: time.Now}
}

func realtimeEnabled(pub realtime.Publisher) bool {
	switch p := pub.(type) {
	case nil:
		return false
	case realtime.NoopPublisher, *realtime.NoopPublisher:
		return false
	case *realtime.CentrifugoClient:
		return p != nil
	default:
		return true
	}
}

// Enabled reports whether events reach Centrifugo.
func (b *Broadcaster) Enabled() bool {
	return b != nil && b.pub != nil
}

// AddResolver registers a channel resolver hook. Resolvers run in the order
// they were added, before DefaultChannel.
func (b *Broadcaster) AddResolver(r ChannelResolver) {
	if b == nil || r == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.resolvers = append(b.resolvers, r)
}

// Publish sends one event in the background. Empty uuids are dropped and the
// list is capped at MaxUUIDs.
func (b *Broadcaster) Publish(ctx context.Context, resource, action string, uuids ...string) {
	if !b.Enabled() || resource == "" || action == "" {
		return
	}
	channel, ok := b.channel(ctx, resource)
	if !ok {
		return
	}
	ev := b.event(resource, action, uuids)
	// Detach from the request: the response must not wait for Centrifugo
	// and a finished request must not cancel the publish.
	bg := context.WithoutCancel(ctx)
	b.inflight.Add(1)
	go func() {
		defer b.inflight.Done()
		pctx, cancel := context.WithTimeout(bg, publishTimeout)
		defer cancel()
		if err := b.pub.Publish(pctx, channel, ev); err != nil {
			b.log.Warn("resource_event_publish_failed",
				"resource", resource, "action", action, "channel", channel, "error", err)
		}
	}()
}

// Wait blocks until background publishes finish (tests, graceful shutdown).
func (b *Broadcaster) Wait() {
	if b == nil {
		return
	}
	b.inflight.Wait()
}

func (b *Broadcaster) channel(ctx context.Context, resource string) (string, bool) {
	b.mu.RLock()
	resolvers := append([]ChannelResolver(nil), b.resolvers...)
	b.mu.RUnlock()
	for _, resolve := range resolvers {
		channel, handled, err := resolve(ctx, resource)
		if err != nil {
			b.log.Warn("resource_event_channel_failed", "resource", resource, "error", err)
			return "", false
		}
		if handled {
			return channel, channel != ""
		}
	}
	return DefaultChannel(resource), true
}

func (b *Broadcaster) event(resource, action string, uuids []string) Event {
	ids := make([]string, 0, min(len(uuids), MaxUUIDs))
	seen := make(map[string]struct{}, len(uuids))
	truncated := false
	for _, id := range uuids {
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		if len(ids) == MaxUUIDs {
			truncated = true
			break
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return Event{
		Type:      EventType,
		Resource:  resource,
		Action:    action,
		UUIDs:     ids,
		Truncated: truncated,
		At:        b.now().UTC(),
	}
}

var (
	_ Publisher = Nop{}
	_ Publisher = (*Broadcaster)(nil)
)
