package observability

import (
	"context"
	"log/slog"
	"time"

	"github.com/getsentry/sentry-go"
)

// alreadyReported are error log messages whose events are captured with
// richer context at the call site (HTTP middleware, queue error handlers).
var alreadyReported = map[string]bool{
	"http_handler_failed":    true,
	"http_server_error":      true,
	"http_panic":             true,
	"queue_task_failed":      true,
	"messaging_task_failed":  true,
	"app_log_persist_failed": true,
}

// slogWindow throttles identical error-log messages to one event per window.
const slogWindow = time.Minute

// NewSlogHandler wraps next so ERROR records are also reported as Sentry
// events (one per message per minute). Attributes are scrubbed and attached
// as context; the log message is the issue title and fingerprint.
func NewSlogHandler(next slog.Handler) slog.Handler {
	return &slogHandler{next: next}
}

type slogHandler struct {
	next   slog.Handler
	attrs  []slog.Attr
	groups string
}

func (h *slogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *slogHandler) Handle(ctx context.Context, rec slog.Record) error {
	if rec.Level >= slog.LevelError && Enabled() && !alreadyReported[rec.Message] &&
		Allow("slog:"+rec.Message, slogWindow) {
		extra := make(map[string]any, len(h.attrs)+rec.NumAttrs())
		for _, a := range h.attrs {
			addAttr(extra, h.groups, a)
		}
		rec.Attrs(func(a slog.Attr) bool {
			addAttr(extra, h.groups, a)
			return true
		})
		var err error
		if v, ok := extra["error"]; ok {
			if e, ok := v.(error); ok {
				err = e
			}
		}
		opts := Options{
			Level:       sentry.LevelError,
			Extra:       extra,
			Fingerprint: []string{"log", rec.Message},
			Tags:        map[string]string{"log_message": rec.Message},
		}
		if err != nil {
			opts.Transaction = rec.Message
			CaptureError(ctx, err, opts)
		} else {
			CaptureMessage(ctx, rec.Message, opts)
		}
	}
	return h.next.Handle(ctx, rec)
}

func addAttr(dst map[string]any, prefix string, a slog.Attr) {
	a.Value = a.Value.Resolve()
	key := a.Key
	if prefix != "" {
		key = prefix + "." + key
	}
	if a.Value.Kind() == slog.KindGroup {
		for _, ga := range a.Value.Group() {
			addAttr(dst, key, ga)
		}
		return
	}
	dst[key] = a.Value.Any()
}

func (h *slogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	next = append(next, h.attrs...)
	next = append(next, attrs...)
	return &slogHandler{next: h.next.WithAttrs(attrs), attrs: next, groups: h.groups}
}

func (h *slogHandler) WithGroup(name string) slog.Handler {
	g := name
	if h.groups != "" {
		g = h.groups + "." + name
	}
	return &slogHandler{next: h.next.WithGroup(name), attrs: h.attrs, groups: g}
}
