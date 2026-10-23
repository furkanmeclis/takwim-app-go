// Package observability wires error reporting to a Sentry-protocol endpoint
// (self-hosted tracker). Everything here is a no-op when SENTRY_DSN is empty.
//
// Privacy rules: no request bodies, cookies, query strings or auth headers are
// sent; free text is scrubbed of emails, phone numbers and tokens; the user is
// identified only by the internal user uuid.
package observability

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/getsentry/sentry-go"
)

var (
	enabled atomic.Bool
	tracing atomic.Bool
	// transport overrides the HTTP transport in tests (nil = SDK default).
	transport sentry.Transport
)

// Enabled reports whether events are being sent.
func Enabled() bool { return enabled.Load() }

// TracingEnabled reports whether HTTP transactions are sampled.
func TracingEnabled() bool { return tracing.Load() }

// Init configures the global client. component ("api", "worker") becomes the
// server name and a tag. The returned flush func must run before exit; it is
// safe to call when reporting is disabled.
func Init(cfg config.SentryConfig, component string) (flush func(), err error) {
	noop := func() {}
	if cfg.DSN == "" {
		return noop, nil
	}
	release := cfg.Release
	if release == "" {
		release = buildRevision()
	}
	err = sentry.Init(sentry.ClientOptions{
		Dsn:              cfg.DSN,
		Environment:      cfg.Environment,
		Release:          release,
		ServerName:       component,
		AttachStacktrace: true,
		// DataCollection left nil: the SDK then keeps its no-PII defaults
		// (same as the deprecated SendDefaultPII=false); scrubEvent runs on top.
		EnableTracing:         cfg.TracesSampleRate > 0,
		TracesSampleRate:      cfg.TracesSampleRate,
		MaxBreadcrumbs:        30,
		Tags:                  map[string]string{"component": component},
		Transport:             transport,
		BeforeSend:            func(e *sentry.Event, _ *sentry.EventHint) *sentry.Event { return scrubEvent(e) },
		BeforeSendTransaction: func(e *sentry.Event, _ *sentry.EventHint) *sentry.Event { return scrubEvent(e) },
	})
	if err != nil {
		return noop, fmt.Errorf("observability: sentry init: %w", err)
	}
	enabled.Store(true)
	tracing.Store(cfg.TracesSampleRate > 0)
	return func() { sentry.Flush(3 * time.Second) }, nil
}

func buildRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && s.Value != "" {
			if len(s.Value) > 12 {
				return s.Value[:12]
			}
			return s.Value
		}
	}
	return ""
}

// Options decorate a single captured event.
type Options struct {
	Level       sentry.Level
	Tags        map[string]string
	Extra       map[string]any
	Fingerprint []string
	// Transaction is shown as the issue culprit (e.g. the route pattern).
	Transaction string
}

// Hub returns the request-scoped hub from ctx, or the global hub.
func Hub(ctx context.Context) *sentry.Hub {
	if ctx != nil {
		if h := sentry.GetHubFromContext(ctx); h != nil {
			return h
		}
	}
	return sentry.CurrentHub()
}

func withOptions(ctx context.Context, opts Options, fn func(hub *sentry.Hub)) {
	hub := Hub(ctx)
	hub.WithScope(func(scope *sentry.Scope) {
		if opts.Level != "" {
			scope.SetLevel(opts.Level)
		}
		for k, v := range opts.Tags {
			scope.SetTag(k, v)
		}
		if len(opts.Extra) > 0 {
			scope.SetContext("details", scrubMap(opts.Extra))
		}
		if len(opts.Fingerprint) > 0 {
			scope.SetFingerprint(opts.Fingerprint)
		}
		if opts.Transaction != "" {
			tx := opts.Transaction
			scope.AddEventProcessor(func(e *sentry.Event, _ *sentry.EventHint) *sentry.Event {
				if e.Type != "transaction" {
					e.Transaction = tx
				}
				return e
			})
		}
		fn(hub)
	})
}

// CaptureError reports err (no-op when disabled or err is nil).
func CaptureError(ctx context.Context, err error, opts Options) {
	if !Enabled() || err == nil {
		return
	}
	withOptions(ctx, opts, func(hub *sentry.Hub) { hub.CaptureException(err) })
}

// CaptureMessage reports a message event (no-op when disabled).
func CaptureMessage(ctx context.Context, msg string, opts Options) {
	if !Enabled() || msg == "" {
		return
	}
	withOptions(ctx, opts, func(hub *sentry.Hub) { hub.CaptureMessage(msg) })
}

// SetUser attaches the internal user uuid to the request-scoped hub.
func SetUser(ctx context.Context, userID string) {
	if !Enabled() || userID == "" {
		return
	}
	if h := sentry.GetHubFromContext(ctx); h != nil {
		h.Scope().SetUser(sentry.User{ID: userID})
	}
}

var throttle sync.Map // key -> time.Time (last report)

// Allow returns true at most once per window for key. Used to keep noisy,
// repeated conditions (misconfiguration, error-log storms) from flooding the
// tracker.
func Allow(key string, window time.Duration) bool {
	now := time.Now()
	if v, ok := throttle.Load(key); ok {
		if last, _ := v.(time.Time); now.Sub(last) < window {
			return false
		}
	}
	throttle.Store(key, now)
	return true
}
