package middleware

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/idempotency"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/google/uuid"
)

func idemSetup(t *testing.T, status int) (http.Handler, *atomic.Int32, string, *jwt.Manager) {
	t.Helper()
	tokens, err := jwt.NewManager("secret", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	h := Idempotency(idempotency.NewMemoryStore(), tokens, slog.New(slog.NewTextHandler(io.Discard, nil)))(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := calls.Add(1)
			b, _ := io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = fmt.Fprintf(w, `{"call":%d,"body":%s}`, n, b)
		}))
	return h, &calls, tok, tokens
}

func idemDo(h http.Handler, tok, method, path, key, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if tok != "" {
		r.Header.Set("Authorization", "Bearer "+tok)
	}
	if key != "" {
		r.Header.Set(IdempotencyKeyHeader, key)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestIdempotencyReplaysFirstOutcome(t *testing.T) {
	t.Parallel()
	h, calls, tok, _ := idemSetup(t, http.StatusCreated)
	first := idemDo(h, tok, http.MethodPost, "/v1/x", "k1", `{"a":1}`)
	second := idemDo(h, tok, http.MethodPost, "/v1/x", "k1", `{"a":1}`)
	if calls.Load() != 1 {
		t.Fatalf("handler ran %d times", calls.Load())
	}
	if second.Code != http.StatusCreated || second.Body.String() != first.Body.String() {
		t.Fatalf("replay = %d %s", second.Code, second.Body)
	}
	if second.Header().Get(IdempotentReplayedHeader) != "true" || first.Header().Get(IdempotentReplayedHeader) != "" {
		t.Fatal("replay header")
	}
	if second.Header().Get("Content-Type") != "application/json" {
		t.Fatal("content type not replayed")
	}

	if w := idemDo(h, tok, http.MethodPost, "/v1/x", "k1", `{"a":2}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("reused key with another body = %d", w.Code)
	}
	if w := idemDo(h, tok, http.MethodPost, "/v1/y", "k1", `{"a":1}`); w.Code != http.StatusCreated || calls.Load() != 2 {
		t.Fatalf("other path = %d calls=%d", w.Code, calls.Load())
	}
}

func TestIdempotencyScopesAndPassThrough(t *testing.T) {
	t.Parallel()
	h, calls, tok, tokens := idemSetup(t, http.StatusOK)
	other, _, _ := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New()})
	idemDo(h, tok, http.MethodPost, "/v1/x", "k", `{}`)
	idemDo(h, other, http.MethodPost, "/v1/x", "k", `{}`)
	if calls.Load() != 2 {
		t.Fatalf("keys are not scoped per user: %d", calls.Load())
	}
	idemDo(h, tok, http.MethodPost, "/v1/x", "", `{}`)
	idemDo(h, tok, http.MethodGet, "/v1/x", "k", ``)
	idemDo(h, "", http.MethodPost, "/v1/x", "k", `{}`)
	idemDo(h, "", http.MethodPost, "/v1/x", "k", `{}`)
	if calls.Load() != 6 {
		t.Fatalf("pass-through calls = %d", calls.Load())
	}
	if w := idemDo(h, tok, http.MethodPost, "/v1/x", "bad key", `{}`); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid key = %d", w.Code)
	}
}

func TestIdempotencyReleasesOnServerError(t *testing.T) {
	t.Parallel()
	h, calls, tok, _ := idemSetup(t, http.StatusInternalServerError)
	idemDo(h, tok, http.MethodPost, "/v1/x", "k", `{}`)
	idemDo(h, tok, http.MethodPost, "/v1/x", "k", `{}`)
	if calls.Load() != 2 {
		t.Fatalf("5xx was replayed: calls=%d", calls.Load())
	}
}

func TestIdempotencyInProgress(t *testing.T) {
	t.Parallel()
	tokens, _ := jwt.NewManager("secret", time.Minute, time.Hour)
	tok, _, _ := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New()})
	store := idempotency.NewMemoryStore()
	release := make(chan struct{})
	started := make(chan struct{})
	h := Idempotency(store, tokens, slog.New(slog.NewTextHandler(io.Discard, nil)))(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			close(started)
			<-release
			w.WriteHeader(http.StatusNoContent)
		}))
	done := make(chan struct{})
	go func() {
		idemDo(h, tok, http.MethodDelete, "/v1/x/1", "k", ``)
		close(done)
	}()
	<-started
	if w := idemDo(h, tok, http.MethodDelete, "/v1/x/1", "k", ``); w.Code != http.StatusConflict {
		t.Fatalf("concurrent retry = %d", w.Code)
	}
	close(release)
	<-done
	if w := idemDo(h, tok, http.MethodDelete, "/v1/x/1", "k", ``); w.Code != http.StatusNoContent || w.Header().Get(IdempotentReplayedHeader) != "true" {
		t.Fatalf("replay after finish = %d", w.Code)
	}
}
