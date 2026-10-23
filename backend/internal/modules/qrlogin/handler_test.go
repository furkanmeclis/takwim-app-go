package qrlogin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerPublicFlow(t *testing.T) {
	f := newFixture(t)
	h := NewHandler(f.svc, nil, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/auth/qr/sessions", h.Create)
	mux.HandleFunc("POST /v1/auth/qr/sessions/{id}/state", h.State)
	mux.HandleFunc("POST /v1/auth/qr/exchange", h.Exchange)

	req := httptest.NewRequest("POST", "/v1/auth/qr/sessions", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0) Firefox/130.0")
	req.Header.Set("X-Forwarded-For", "203.0.113.5")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("create %d %s", rec.Code, rec.Body)
	}
	var created struct {
		Data Created `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	sess, err := f.svc.store.Get(req.Context(), created.Data.SessionID)
	if err != nil || sess.IP != "203.0.113.5" || !strings.Contains(sess.UserAgent, "Firefox") {
		t.Fatalf("stored %+v %v", sess, err)
	}

	body := `{"browser_secret":"` + created.Data.BrowserSecret + `"}`
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/auth/qr/sessions/"+created.Data.SessionID+"/state", strings.NewReader(body)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"pending"`) {
		t.Fatalf("state %d %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/auth/qr/exchange", strings.NewReader(`{"session_id":"x","browser_secret":"y","exchange_token":"z"}`)))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "QR_LOGIN_INVALID") {
		t.Fatalf("exchange %d %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/auth/qr/sessions/"+strings.Repeat("a", 43)+"/state", strings.NewReader(body)))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "QR_SESSION_NOT_FOUND") {
		t.Fatalf("unknown %d %s", rec.Code, rec.Body)
	}
}
