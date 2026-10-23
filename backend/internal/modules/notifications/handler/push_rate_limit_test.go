package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/google/uuid"
)

// countingLimiter allows the first `limit` calls per action+subject.
type countingLimiter struct {
	hits map[string]int
}

func (l *countingLimiter) Allow(_ context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration) {
	key := action + "|" + subject
	l.hits[key]++
	if l.hits[key] > limit {
		return false, window
	}
	return true, 0
}

func pushRequest(method, path, body string, userID int64) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r.WithContext(authctx.WithPrincipal(r.Context(), authctx.Principal{UserID: uuid.New(), UserInternal: userID}))
}

func TestRegisterPushDeviceRateLimited(t *testing.T) {
	h := New(nil)
	h.SetRateLimiter(&countingLimiter{hits: map[string]int{}})

	// An invalid body is rejected before the service is touched (svc is nil),
	// so the first pushDeviceLimit calls answer 400 and the next one 429.
	for i := 0; i < pushDeviceLimit; i++ {
		w := httptest.NewRecorder()
		h.RegisterPushDevice(w, pushRequest(http.MethodPost, "/v1/push-devices", `{"token":"bad","platform":"ios"}`, 7))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("call %d: status %d, want 400", i+1, w.Code)
		}
	}

	w := httptest.NewRecorder()
	h.RegisterPushDevice(w, pushRequest(http.MethodPost, "/v1/push-devices", `{"token":"bad","platform":"ios"}`, 7))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("over limit: status %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("over limit: missing Retry-After")
	}
	if !strings.Contains(w.Body.String(), "RATE_LIMITED") {
		t.Fatalf("over limit: body %s, want RATE_LIMITED", w.Body.String())
	}

	// Another user has their own budget.
	w = httptest.NewRecorder()
	h.RegisterPushDevice(w, pushRequest(http.MethodPost, "/v1/push-devices", `{"token":"bad","platform":"ios"}`, 8))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("other user: status %d, want 400", w.Code)
	}
}

func TestPushDeviceNoLimiterConfigured(t *testing.T) {
	h := New(nil)
	for i := 0; i < pushDeviceLimit*3; i++ {
		w := httptest.NewRecorder()
		h.RegisterPushDevice(w, pushRequest(http.MethodPost, "/v1/push-devices", `{"token":"bad","platform":"ios"}`, 7))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("call %d: status %d, want 400 (no limiter)", i+1, w.Code)
		}
	}
}
