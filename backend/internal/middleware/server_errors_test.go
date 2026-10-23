package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

type logLine map[string]any

func observedServer(t *testing.T) (http.Handler, func() []logLine) {
	t.Helper()
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /v1/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		response.JSON(w, r, http.StatusOK, map[string]string{"id": r.PathValue("id")})
	})
	mux.HandleFunc("POST /v1/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		response.InternalErr(w, r, errors.New("db exploded for a@b.co"), "Unexpected server error")
	})
	mux.HandleFunc("POST /v1/push-devices", func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // client goes away mid-request
		response.InternalErr(w, r, r.Context().Err(), "Unexpected server error")
	})
	mux.HandleFunc("POST /v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		response.RecordFailure(w, errors.New("invalid credentials"))
		response.Error(w, r, http.StatusUnauthorized, response.CodeInvalidCredentials, "nope")
	})
	mux.HandleFunc("GET /v1/boom/{id}", func(http.ResponseWriter, *http.Request) { panic("kaboom") })
	h := RequestID(ServerErrors(log)(mux))
	return h, func() []logLine {
		var out []logLine
		for _, raw := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
			if len(raw) == 0 {
				continue
			}
			var l logLine
			if err := json.Unmarshal(raw, &l); err != nil {
				t.Fatalf("bad log line %q: %v", raw, err)
			}
			out = append(out, l)
		}
		return out
	}
}

func TestServerErrorLogsRoutePatternAndRequestID(t *testing.T) {
	h, logs := observedServer(t)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v1/items/9f8e7d", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rr.Code)
	}
	lines := logs()
	if len(lines) != 1 {
		t.Fatalf("got %d log lines: %v", len(lines), lines)
	}
	l := lines[0]
	if l["msg"] != "http_handler_failed" || l["level"] != "ERROR" {
		t.Fatalf("unexpected line: %v", l)
	}
	if l["route"] != "POST /v1/items/{id}" || l["request_id"] == "" || l["error_code"] != response.CodeInternalError {
		t.Fatalf("missing route/request_id/error_code: %v", l)
	}
	if strings.Contains(l["error"].(string), "@") {
		t.Fatalf("email not scrubbed from log: %v", l["error"])
	}
}

func TestClientCancelIs499AtInfo(t *testing.T) {
	h, logs := observedServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v1/push-devices", nil).WithContext(ctx))
	if rr.Code != response.StatusClientClosedRequest {
		t.Fatalf("status = %d, want 499", rr.Code)
	}
	lines := logs()
	if len(lines) != 1 || lines[0]["msg"] != "http_request_canceled" || lines[0]["level"] != "INFO" {
		t.Fatalf("want one INFO http_request_canceled, got %v", lines)
	}
	if lines[0]["status"] != float64(499) {
		t.Fatalf("status attr = %v", lines[0]["status"])
	}
}

func TestPanicRecoveredAs500(t *testing.T) {
	h, logs := observedServer(t)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/boom/1", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rr.Code)
	}
	lines := logs()
	if len(lines) != 1 || lines[0]["msg"] != "http_panic" || lines[0]["route"] != "GET /v1/boom/{id}" {
		t.Fatalf("unexpected logs: %v", lines)
	}
}

func TestAccessLogAuthFailuresOnly(t *testing.T) {
	h, logs := observedServer(t)
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/healthz", nil),
		httptest.NewRequest(http.MethodGet, "/v1/items/1", nil),
		httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil),
		httptest.NewRequest(http.MethodGet, "/nope", nil),
	} {
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	lines := logs()
	if len(lines) != 2 {
		t.Fatalf("want 2 access lines (auth 401 + 404), got %v", lines)
	}
	auth := lines[0]
	if auth["msg"] != "http_request" || auth["level"] != "INFO" || auth["route"] != "POST /v1/auth/login" ||
		auth["status"] != float64(401) || auth["error_code"] != response.CodeInvalidCredentials ||
		auth["reason"] != "invalid credentials" {
		t.Fatalf("auth line: %v", auth)
	}
	if lines[1]["route"] != "unmatched" || lines[1]["status"] != float64(404) {
		t.Fatalf("404 line: %v", lines[1])
	}
}
