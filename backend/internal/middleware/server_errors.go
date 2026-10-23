package middleware

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/observability"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/getsentry/sentry-go"
)

type statusWriter struct {
	http.ResponseWriter
	status        int
	headerWritten bool
	err           error
	publicMessage string
	recorded      bool
	errorCode     string
	failure       error
}

func (w *statusWriter) WriteHeader(status int) {
	if !w.headerWritten {
		w.status = status
		w.headerWritten = true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.headerWritten {
		w.status = http.StatusOK
		w.headerWritten = true
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) RecordServerError(err error, publicMessage string) {
	if err == nil {
		return
	}
	w.err = err
	w.publicMessage = publicMessage
	w.recorded = true
}

// RecordErrorCode keeps the envelope error code for the access log.
func (w *statusWriter) RecordErrorCode(code string) {
	if w.errorCode == "" {
		w.errorCode = code
	}
}

// RecordFailure keeps the cause of a non-5xx failure for the access log.
func (w *statusWriter) RecordFailure(err error) {
	if err != nil && w.failure == nil {
		w.failure = err
	}
}

func (w *statusWriter) StatusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *statusWriter) ErrorRecorded() (publicMessage string, ok bool, err error) {
	if !w.recorded {
		return "", false, nil
	}
	return w.publicMessage, true, w.err
}

// ServerErrors is the request observer: it recovers panics, writes one
// structured access log line per interesting request, logs 5xx responses at
// ERROR and reports them (and panics) to the error tracker.
//
// Requests the client cancelled are logged at INFO with status 499 and are
// never reported. Must run inside RequestID so request_id is available.
func ServerErrors(log *slog.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ctx := r.Context()
			var hub *sentry.Hub
			if observability.Enabled() {
				hub = sentry.CurrentHub().Clone()
				hub.Scope().SetTag("request_id", GetRequestID(ctx))
				ctx = sentry.SetHubOnContext(ctx, hub)
			}
			var tx *sentry.Span
			if observability.TracingEnabled() && !isHealthPath(r.URL.Path) {
				// Named after the route pattern once routed (raw paths can
				// carry tokens, e.g. DELETE /v1/push-devices/{token}).
				tx = sentry.StartTransaction(ctx, r.Method+" unmatched",
					sentry.ContinueFromRequest(r),
					sentry.WithOpName("http.server"),
					sentry.WithTransactionSource(sentry.SourceRoute),
				)
				ctx = tx.Context()
			}
			req := r.WithContext(ctx)
			if hub != nil {
				hub.Scope().AddEventProcessor(requestEventProcessor(req))
			}
			rec := &statusWriter{ResponseWriter: w, status: http.StatusOK}

			defer func() {
				if p := recover(); p != nil {
					if p == http.ErrAbortHandler { //nolint:errorlint // sentinel compared by identity, as net/http does
						panic(p)
					}
					handlePanic(log, rec, req, p, time.Since(start))
				} else {
					observe(log, rec, req, time.Since(start))
				}
				if tx != nil {
					tx.Name = routeOf(req)
					tx.Status = sentry.HTTPtoSpanStatus(rec.StatusCode())
					tx.SetData("http.response.status_code", rec.StatusCode())
					tx.Finish()
				}
			}()
			next.ServeHTTP(rec, req)
		})
	}
}

// routeOf returns the matched ServeMux pattern ("POST /v1/items/{id}"). The
// mux sets Request.Pattern on the request it routes, which is the one we
// passed down, so it is readable here after ServeHTTP returns.
func routeOf(r *http.Request) string {
	if r.Pattern != "" {
		return r.Pattern
	}
	return "unmatched"
}

// routePath is the path part of a pattern ("POST /v1/x" -> "/v1/x").
func routePath(route string) string {
	if _, p, ok := strings.Cut(route, " "); ok {
		return p
	}
	return route
}

func isHealthPath(p string) bool {
	return p == "/healthz" || p == "/readyz"
}

// isAuthRoute covers sign-in / session / NextAuth adapter endpoints whose
// requests are always access-logged (success and failure).
func isAuthRoute(path string) bool {
	return strings.HasPrefix(path, "/v1/auth/") || strings.HasPrefix(path, "/v1/internal/auth/")
}

// isPollRoute is a high-frequency poll that is only logged when it fails.
func isPollRoute(route string) bool {
	return route == "POST /v1/auth/qr/sessions/{id}/state"
}

func clientGone(r *http.Request) bool {
	return errors.Is(r.Context().Err(), context.Canceled)
}

func observe(log *slog.Logger, rec *statusWriter, r *http.Request, dur time.Duration) {
	ctx := r.Context()
	status := rec.StatusCode()
	route := routeOf(r)
	attrs := []any{
		"method", r.Method,
		"route", route,
		"status", status,
		"duration_ms", dur.Milliseconds(),
		"request_id", GetRequestID(ctx),
	}

	// Client went away: not a server fault. Record as 499, never report.
	if status == response.StatusClientClosedRequest || (status >= http.StatusInternalServerError && clientGone(r)) {
		attrs[5] = response.StatusClientClosedRequest
		if _, ok, err := rec.ErrorRecorded(); ok {
			attrs = append(attrs, "error", observability.ScrubString(err.Error()))
		} else if rec.failure != nil {
			attrs = append(attrs, "error", observability.ScrubString(rec.failure.Error()))
		}
		log.InfoContext(ctx, "http_request_canceled", attrs...)
		return
	}

	if status >= http.StatusInternalServerError {
		if rec.errorCode != "" {
			attrs = append(attrs, "error_code", rec.errorCode)
		}
		opts := observability.Options{
			Level:       sentry.LevelError,
			Transaction: route,
			Fingerprint: []string{"{{ default }}", route, fmt.Sprint(status)},
			Tags: map[string]string{
				"http.route":       route,
				"http.method":      r.Method,
				"http.status_code": fmt.Sprint(status),
			},
		}
		if msg, ok, err := rec.ErrorRecorded(); ok {
			attrs = append(attrs, "error", observability.ScrubString(err.Error()), "message", msg)
			log.ErrorContext(ctx, "http_handler_failed", attrs...)
			observability.CaptureError(ctx, err, opts)
			return
		}
		log.ErrorContext(ctx, "http_server_error", attrs...)
		code := rec.errorCode
		if code == "" {
			code = http.StatusText(status)
		}
		observability.CaptureMessage(ctx, fmt.Sprintf("HTTP %d %s (%s)", status, route, code), opts)
		return
	}

	if isHealthPath(r.URL.Path) {
		return
	}
	auth := isAuthRoute(routePath(route))
	failed := status >= http.StatusBadRequest
	if !failed && (!auth || isPollRoute(route)) {
		return
	}
	if rec.errorCode != "" {
		attrs = append(attrs, "error_code", rec.errorCode)
	}
	reason := ""
	if rec.failure != nil {
		reason = observability.ScrubString(rec.failure.Error())
		attrs = append(attrs, "reason", reason)
	}
	log.InfoContext(ctx, "http_request", attrs...)
	if auth && failed {
		reportAuthMisconfig(ctx, route, status, rec.errorCode, reason)
	}
}

// reportAuthMisconfig sends a warning for auth failures that point at server
// configuration rather than at the user (wrong adapter secret, provider not
// enabled, client id / issuer mismatch). Throttled per route + code.
func reportAuthMisconfig(ctx context.Context, route string, status int, code, reason string) {
	var what string
	switch {
	case strings.HasPrefix(routePath(route), "/v1/internal/auth/") && status == http.StatusUnauthorized:
		what = "NextAuth adapter rejected: adapter secret mismatch"
	case code == response.CodeOAuthProviderDisabled:
		what = "Sign-in provider is not enabled"
	case code == response.CodeInvalidIDToken &&
		(strings.Contains(reason, "audience") || strings.Contains(reason, "issuer") || strings.Contains(reason, "unknown key id")):
		what = "Provider id_token rejected: " + reason
	default:
		return
	}
	if !observability.Enabled() || !observability.Allow("auth:"+route+":"+what, 10*time.Minute) {
		return
	}
	observability.CaptureMessage(ctx, what, observability.Options{
		Level:       sentry.LevelWarning,
		Transaction: route,
		Fingerprint: []string{"auth-misconfig", route, code},
		Tags: map[string]string{
			"http.route":       route,
			"http.status_code": fmt.Sprint(status),
			"error_code":       code,
		},
	})
}

func handlePanic(log *slog.Logger, rec *statusWriter, r *http.Request, p any, dur time.Duration) {
	ctx := r.Context()
	route := routeOf(r)
	log.ErrorContext(ctx, "http_panic",
		"method", r.Method,
		"route", route,
		"duration_ms", dur.Milliseconds(),
		"request_id", GetRequestID(ctx),
		"panic", observability.ScrubString(fmt.Sprint(p)),
		"stack", string(debug.Stack()),
	)
	if observability.Enabled() {
		hub := observability.Hub(ctx)
		hub.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("http.route", route)
			scope.SetTag("http.method", r.Method)
			scope.AddEventProcessor(func(e *sentry.Event, _ *sentry.EventHint) *sentry.Event {
				e.Transaction = route
				return e
			})
			hub.RecoverWithContext(ctx, p)
		})
	}
	if !rec.headerWritten {
		response.Internal(rec, r, "Unexpected server error")
	}
}

// requestEventProcessor attaches a minimal request description: method, the
// route pattern as URL (never the raw path or query string: they can carry
// tokens and emails), and an allowlist of headers. Bodies, cookies and auth
// headers are never included. r is the routed request; its Pattern is read
// when the event is built.
func requestEventProcessor(r *http.Request) sentry.EventProcessor {
	headers := map[string]string{}
	for _, h := range []string{"User-Agent", "Content-Type", "Content-Length", "Accept"} {
		if v := r.Header.Get(h); v != "" {
			headers[h] = v
		}
	}
	if id := GetRequestID(r.Context()); id != "" {
		headers[RequestIDHeader] = id
	}
	return func(e *sentry.Event, _ *sentry.EventHint) *sentry.Event {
		if e.Request == nil {
			e.Request = &sentry.Request{Method: r.Method, URL: routePath(routeOf(r)), Headers: headers}
		}
		return e
	}
}
