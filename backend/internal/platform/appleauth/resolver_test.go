package appleauth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type fakeSource struct {
	cfg   Config
	ok    bool
	err   error
	calls atomic.Int32
}

func (f *fakeSource) AppleSigningKey(context.Context) (Config, bool, error) {
	f.calls.Add(1)
	return f.cfg, f.ok, f.err
}

func pkcs8PEM(t *testing.T, key any) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestParsePrivateKey(t *testing.T) {
	good, _ := testKeyPEM(t)
	if _, err := ParsePrivateKey(good); err != nil {
		t.Fatalf("valid p8: %v", err)
	}
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	p384, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	p256, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	sec1, _ := x509.MarshalECPrivateKey(p256)
	bad := map[string]string{
		"not pem": "hello",
		"rsa":     pkcs8PEM(t, rsaKey),
		"p384":    pkcs8PEM(t, p384),
		"sec1":    string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: sec1})),
	}
	for name, text := range bad {
		if _, err := ParsePrivateKey(text); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func parseSecret(t *testing.T, secret string, pub *ecdsa.PublicKey) (*jwt.Token, jwt.MapClaims) {
	t.Helper()
	claims := jwt.MapClaims{}
	tok, err := jwt.ParseWithClaims(secret, claims, func(*jwt.Token) (any, error) { return pub, nil },
		jwt.WithValidMethods([]string{"ES256"}))
	if err != nil || !tok.Valid {
		t.Fatalf("secret does not verify: %v", err)
	}
	return tok, claims
}

func TestResolverPrefersDBOverEnv(t *testing.T) {
	envPEM, envKey := testKeyPEM(t)
	dbPEM, dbKey := testKeyPEM(t)
	src := &fakeSource{cfg: Config{TeamID: "DBTEAM0001", KeyID: "DBKEY00001", PrivateKey: dbPEM}, ok: true}
	r, err := NewResolver(src, Config{TeamID: "ENVTEAM001", KeyID: "ENVKEY0001", PrivateKey: envPEM}, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	r.now = func() time.Time { return now }
	ctx := context.Background()

	if got := r.Source(ctx); got != SourceDB {
		t.Fatalf("source = %q, want db", got)
	}
	secret, _, err := r.GenerateClientSecretTTL(ctx, "com.example.web", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	tok, claims := parseSecret(t, secret, &dbKey.PublicKey)
	if tok.Header["kid"] != "DBKEY00001" || claims["iss"] != "DBTEAM0001" || claims["aud"] != "https://appleid.apple.com" {
		t.Fatalf("unexpected header/claims: %v %v", tok.Header, claims)
	}

	// Cached: a source change is not seen until TTL or Invalidate.
	src.ok = false
	if r.Source(ctx) != SourceDB || src.calls.Load() != 1 {
		t.Fatalf("expected cached db key (calls=%d)", src.calls.Load())
	}
	now = now.Add(DefaultResolverTTL + time.Second)
	if got := r.Source(ctx); got != SourceEnv {
		t.Fatalf("after ttl source = %q, want env", got)
	}
	secret, _, err = r.GenerateClientSecretTTL(ctx, "com.example.web", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	parseSecret(t, secret, &envKey.PublicKey)

	src.ok = true
	r.Invalidate()
	if got := r.Source(ctx); got != SourceDB {
		t.Fatalf("after invalidate source = %q, want db", got)
	}

	// Read errors keep the last resolved key.
	src.err = errors.New("db down")
	now = now.Add(DefaultResolverTTL + time.Second)
	if got := r.Source(ctx); got != SourceDB {
		t.Fatalf("on source error source = %q, want cached db", got)
	}
}

func TestResolverNoKey(t *testing.T) {
	r, err := NewResolver(&fakeSource{}, Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if r.Configured(ctx) || r.EnvConfigured() || r.Source(ctx) != SourceNone {
		t.Fatal("expected no key")
	}
	if _, _, err := r.GenerateClientSecretTTL(ctx, "x", time.Hour); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("got %v", err)
	}
}

func TestResolverInvalidEnvKeyFails(t *testing.T) {
	if _, err := NewResolver(nil, Config{TeamID: "T", KeyID: "K", PrivateKey: "junk"}, nil); err == nil {
		t.Fatal("expected error for invalid env key")
	}
}

func TestResolverRevokeUsesDBKey(t *testing.T) {
	envPEM, _ := testKeyPEM(t)
	dbPEM, dbKey := testKeyPEM(t)
	var revoked string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		tok, err := jwt.Parse(r.PostForm.Get("client_secret"), func(*jwt.Token) (any, error) { return &dbKey.PublicKey, nil })
		if err != nil || !tok.Valid {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		revoked = r.PostForm.Get("token")
	}))
	defer srv.Close()
	src := &fakeSource{cfg: Config{TeamID: "DBTEAM0001", KeyID: "DBKEY00001", PrivateKey: dbPEM}, ok: true}
	r, err := NewResolver(src, Config{TeamID: "ENVTEAM001", KeyID: "ENVKEY0001", PrivateKey: envPEM}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.SetBaseURL(srv.URL)
	if err := r.Revoke(context.Background(), "com.example.app", "", "rt-db"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if revoked != "rt-db" {
		t.Fatalf("revoked = %q", revoked)
	}
}

func TestGenerateClientSecretTTLBounds(t *testing.T) {
	pemText, _ := testKeyPEM(t)
	c, err := New(Config{TeamID: "T", KeyID: "K", PrivateKey: pemText}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.GenerateClientSecretTTL("x", MaxClientSecretTTL+time.Hour); err == nil {
		t.Fatal("expected error above Apple's 6-month cap")
	}
}
