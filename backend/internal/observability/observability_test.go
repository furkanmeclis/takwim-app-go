package observability

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/getsentry/sentry-go"
)

func TestScrubString(t *testing.T) {
	cases := map[string]string{
		"user ali.veli@example.com not found":           "[email]",
		"Authorization: Bearer abc.def-123":             "[token]",
		"jwt eyJhbGciOi.eyJzdWIiOiIx.c2lnbmF0dXJl here": "[token]",
		"call +90 555 123 45 67 now":                    "[phone]",
		"call 0555 123 45 67 now":                       "[phone]",
		"refresh_token=abcdef123&x=1":                   "refresh_token=[redacted]",
		`{"password":"hunter2"}`:                        `"password":"[redacted]"`,
		"link ticket=Zm9vYmFy":                          "ticket=[redacted]",
	}
	for in, want := range cases {
		got := ScrubString(in)
		if !strings.Contains(got, want) {
			t.Errorf("ScrubString(%q) = %q, want it to contain %q", in, got, want)
		}
	}
	// Ordinary diagnostics survive.
	keep := "oidc: invalid id token: audience (request 7f1c2d3e-0000-4000-8000-000000000001, id 1234567)"
	if got := ScrubString(keep); got != keep {
		t.Errorf("ScrubString changed a harmless message: %q", got)
	}
}

func TestScrubEventDropsPII(t *testing.T) {
	e := &sentry.Event{
		Message: "failed for a@b.co",
		User:    sentry.User{ID: "u-1", Email: "a@b.co", IPAddress: "1.2.3.4", Username: "ali"},
		Request: &sentry.Request{
			URL:         "https://api.example.com/v1/auth/login?email=a@b.co",
			Method:      "POST",
			Data:        `{"password":"x"}`,
			Cookies:     "session=abc",
			QueryString: "email=a@b.co",
			Headers: map[string]string{
				"Authorization": "Bearer x", "Cookie": "a=b", "user-agent": "ios", "X-Auth-Adapter-Key": "k",
			},
			Env: map[string]string{"REMOTE_ADDR": "1.2.3.4"},
		},
		Tags:      map[string]string{"access_token": "x", "route": "POST /v1/auth/login"},
		Contexts:  map[string]sentry.Context{"details": {"email": "a@b.co", "count": 3}},
		Exception: []sentry.Exception{{Type: "*errors.errorString", Value: "user a@b.co"}},
	}
	out := scrubEvent(e)
	if out.User.ID != "u-1" || out.User.Email != "" || out.User.IPAddress != "" || out.User.Username != "" {
		t.Fatalf("user not reduced to id: %+v", out.User)
	}
	r := out.Request
	if r.Data != "" || r.Cookies != "" || r.QueryString != "" || r.Env != nil {
		t.Fatalf("request not scrubbed: %+v", r)
	}
	if strings.Contains(r.URL, "?") {
		t.Fatalf("query kept in url: %s", r.URL)
	}
	if _, ok := r.Headers["Authorization"]; ok || len(r.Headers) != 1 || r.Headers["User-Agent"] != "ios" {
		t.Fatalf("headers not allowlisted: %v", r.Headers)
	}
	if out.Tags["access_token"] != redacted || out.Tags["route"] != "POST /v1/auth/login" {
		t.Fatalf("tags: %v", out.Tags)
	}
	if out.Contexts["details"]["email"] != redacted || out.Contexts["details"]["count"] != 3 {
		t.Fatalf("contexts: %v", out.Contexts)
	}
	if strings.Contains(out.Message, "@") || strings.Contains(out.Exception[0].Value, "@") {
		t.Fatalf("free text not scrubbed: %q / %q", out.Message, out.Exception[0].Value)
	}
}

func TestDisabledWithoutDSN(t *testing.T) {
	flush, err := Init(config.SentryConfig{}, "api")
	if err != nil {
		t.Fatal(err)
	}
	flush()
	if Enabled() {
		t.Fatal("reporting enabled without DSN")
	}
	// Must be safe no-ops.
	CaptureError(context.Background(), errors.New("x"), Options{})
	CaptureMessage(context.Background(), "x", Options{})
	SetUser(context.Background(), "u")
}

func TestCaptureAndSlogHandler(t *testing.T) {
	mock := &sentry.MockTransport{}
	transport = mock
	t.Cleanup(func() {
		transport = nil
		enabled.Store(false)
		tracing.Store(false)
		sentry.CurrentHub().BindClient(nil)
	})
	if _, err := Init(config.SentryConfig{DSN: "https://public@sentry.example.test/1", Environment: "test"}, "api"); err != nil {
		t.Fatal(err)
	}
	if !Enabled() {
		t.Fatal("expected enabled")
	}
	CaptureError(context.Background(), errors.New("boom for x@y.io"), Options{
		Tags: map[string]string{"task_type": "export:process"}, Fingerprint: []string{"queue-task", "export:process"},
	})

	log := slog.New(NewSlogHandler(slog.NewTextHandler(io.Discard, nil)))
	log.Error("billing_lifecycle_boot_failed", "error", errors.New("db down"), "email", "x@y.io")
	log.Error("billing_lifecycle_boot_failed", "error", errors.New("db down again")) // throttled
	log.Error("http_handler_failed", "error", errors.New("already reported"))        // skipped
	log.Warn("just_a_warning")                                                       // not an error

	events := mock.Events()
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if v := events[0].Exception[len(events[0].Exception)-1].Value; strings.Contains(v, "@") {
		t.Fatalf("exception value not scrubbed: %q", v)
	}
	if events[0].Tags["task_type"] != "export:process" || events[0].Tags["component"] != "api" {
		t.Fatalf("tags: %v", events[0].Tags)
	}
	if events[1].Contexts["details"]["email"] != redacted {
		t.Fatalf("slog attrs not scrubbed: %v", events[1].Contexts["details"])
	}
	if events[1].Transaction != "billing_lifecycle_boot_failed" {
		t.Fatalf("transaction = %q", events[1].Transaction)
	}
}

func TestScrubPushToken(t *testing.T) {
	for _, in := range []string{
		"device ExponentPushToken[xxxxABC] gone",
		"/v1/push-devices/ExponentPushToken%5Bab12%5D",
	} {
		if got := ScrubString(in); !strings.Contains(got, "[push-token]") || strings.Contains(got, "ab12") || strings.Contains(got, "xxxxABC") {
			t.Errorf("ScrubString(%q) = %q", in, got)
		}
	}
}
