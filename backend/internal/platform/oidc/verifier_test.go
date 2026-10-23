package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type testIssuer struct {
	key  *rsa.PrivateKey
	kid  string
	srv  *httptest.Server
	hits atomic.Int32
}

func newTestIssuer(t *testing.T) *testIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ti := &testIssuer{key: key, kid: "k1"}
	ti.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		ti.hits.Add(1)
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": ti.kid, "alg": "RS256", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	t.Cleanup(ti.srv.Close)
	return ti
}

func (ti *testIssuer) sign(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = ti.kid
	raw, err := tok.SignedString(ti.key)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func baseClaims(now time.Time) jwt.MapClaims {
	return jwt.MapClaims{
		"iss":              "https://appleid.apple.com",
		"aud":              "com.example.app",
		"sub":              "001234.abc",
		"email":            "X@privaterelay.appleid.com",
		"email_verified":   "true",
		"is_private_email": "true",
		"iat":              now.Unix(),
		"exp":              now.Add(10 * time.Minute).Unix(),
		"nonce":            "raw-nonce",
	}
}

func TestVerifyApple(t *testing.T) {
	ti := newTestIssuer(t)
	now := time.Now()
	v := New(map[string]ProviderConfig{ProviderApple: {Issuers: []string{"https://appleid.apple.com"}, JWKSURL: ti.srv.URL}}, nil)
	ctx := context.Background()
	aud := []string{"com.example.app"}

	claims, err := v.Verify(ctx, ProviderApple, ti.sign(t, baseClaims(now)), aud, "raw-nonce")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Subject != "001234.abc" || claims.Email != "x@privaterelay.appleid.com" || !claims.EmailVerified || !claims.IsPrivateEmail {
		t.Fatalf("claims = %+v", claims)
	}

	// Hashed nonce in token, raw nonce from client.
	c := baseClaims(now)
	sum := sha256.Sum256([]byte("raw-nonce"))
	c["nonce"] = hex.EncodeToString(sum[:])
	if _, err := v.Verify(ctx, ProviderApple, ti.sign(t, c), aud, "raw-nonce"); err != nil {
		t.Fatalf("hashed nonce: %v", err)
	}

	cases := map[string]func(jwt.MapClaims){
		"wrong aud":     func(c jwt.MapClaims) { c["aud"] = "com.other.app" },
		"wrong iss":     func(c jwt.MapClaims) { c["iss"] = "https://evil.example" },
		"expired":       func(c jwt.MapClaims) { c["exp"] = now.Add(-10 * time.Minute).Unix() },
		"wrong nonce":   func(c jwt.MapClaims) { c["nonce"] = "other" },
		"missing sub":   func(c jwt.MapClaims) { delete(c, "sub") },
		"missing exp":   func(c jwt.MapClaims) { delete(c, "exp") },
		"missing nonce": func(c jwt.MapClaims) { delete(c, "nonce") },
	}
	for name, mutate := range cases {
		c := baseClaims(now)
		mutate(c)
		if _, err := v.Verify(ctx, ProviderApple, ti.sign(t, c), aud, "raw-nonce"); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("%s: expected ErrInvalidToken, got %v", name, err)
		}
	}

	// Signature from another key is rejected.
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, baseClaims(now))
	tok.Header["kid"] = ti.kid
	forged, _ := tok.SignedString(other)
	if _, err := v.Verify(ctx, ProviderApple, forged, aud, ""); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("forged: expected ErrInvalidToken, got %v", err)
	}

	// Keys are cached across calls.
	if hits := ti.hits.Load(); hits != 1 {
		t.Fatalf("jwks fetched %d times, want 1", hits)
	}
}

func TestVerifyUnknownKidBackoff(t *testing.T) {
	ti := newTestIssuer(t)
	now := time.Now()
	v := New(map[string]ProviderConfig{ProviderGoogle: {Issuers: []string{"accounts.google.com"}, JWKSURL: ti.srv.URL}}, nil)
	c := baseClaims(now)
	c["iss"] = "accounts.google.com"
	c["aud"] = "client.apps.googleusercontent.com"
	if _, err := v.Verify(context.Background(), ProviderGoogle, ti.sign(t, c), []string{"client.apps.googleusercontent.com"}, ""); err != nil {
		t.Fatalf("verify: %v", err)
	}
	ti.kid = "rotated"
	for range 3 {
		if _, err := v.Verify(context.Background(), ProviderGoogle, ti.sign(t, c), []string{"client.apps.googleusercontent.com"}, ""); err == nil {
			t.Fatal("expected unknown kid failure within backoff")
		}
	}
	if hits := ti.hits.Load(); hits != 1 {
		t.Fatalf("jwks refetched within backoff: %d", hits)
	}
	v.SetClock(func() time.Time { return now.Add(2 * time.Minute) })
	if _, err := v.Verify(context.Background(), ProviderGoogle, ti.sign(t, c), []string{"client.apps.googleusercontent.com"}, ""); err != nil {
		t.Fatalf("after backoff, rotated key should verify: %v", err)
	}
}

func TestVerifyUnknownProvider(t *testing.T) {
	v := New(DefaultProviders(), nil)
	if _, err := v.Verify(context.Background(), "github", "x", []string{"a"}, ""); !errors.Is(err, ErrUnknownProvider) {
		t.Fatalf("got %v", err)
	}
}
