package appleauth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func testKeyPEM(t *testing.T) (string, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), key
}

func TestNotConfigured(t *testing.T) {
	c, err := New(Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.Configured() {
		t.Fatal("expected unconfigured")
	}
	if _, err := c.GenerateClientSecret("com.example.app"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("got %v", err)
	}
}

func TestExchangeAndRevoke(t *testing.T) {
	pemText, key := testKeyPEM(t)
	var revoked string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		secret := r.PostForm.Get("client_secret")
		tok, err := jwt.Parse(secret, func(*jwt.Token) (any, error) { return &key.PublicKey, nil })
		if err != nil || !tok.Valid {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if sub, _ := tok.Claims.GetSubject(); sub != r.PostForm.Get("client_id") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/auth/token":
			_, _ = w.Write([]byte(`{"refresh_token":"rt-1","id_token":"x"}`))
		case "/auth/revoke":
			revoked = r.PostForm.Get("token")
		}
	}))
	defer srv.Close()

	c, err := New(Config{TeamID: "TEAM", KeyID: "KEY", PrivateKey: strings.ReplaceAll(pemText, "\n", `\n`)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	c.SetBaseURL(srv.URL)
	rt, err := c.ExchangeCode(context.Background(), "com.example.app", "code")
	if err != nil || rt != "rt-1" {
		t.Fatalf("exchange: %q %v", rt, err)
	}
	if err := c.Revoke(context.Background(), "com.example.app", "", rt); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if revoked != "rt-1" {
		t.Fatalf("revoked = %q", revoked)
	}
}
