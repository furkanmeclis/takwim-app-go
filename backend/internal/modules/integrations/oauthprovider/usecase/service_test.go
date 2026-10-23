package usecase

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/appleauth"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// memStore mirrors the UpdateOAuthProviderSettings SQL semantics.
type memStore struct {
	rows map[string]db.OauthProviderSetting
}

func newMemStore() *memStore {
	s := &memStore{rows: map[string]db.OauthProviderSetting{}}
	for _, p := range []string{"google", "facebook", "apple"} {
		s.rows[p] = db.OauthProviderSetting{Provider: p}
	}
	return s
}

func (m *memStore) GetOAuthProviderSettings(_ context.Context, p string) (db.OauthProviderSetting, error) {
	return m.rows[p], nil
}

func (m *memStore) ListOAuthProviderSettings(context.Context) ([]db.OauthProviderSetting, error) {
	return nil, nil
}

func (m *memStore) UpdateOAuthProviderSettings(_ context.Context, a db.UpdateOAuthProviderSettingsParams) (db.OauthProviderSetting, error) {
	r := m.rows[a.Provider]
	if a.LoginEnabled.Valid {
		r.LoginEnabled = a.LoginEnabled.Bool
	}
	if a.RegisterEnabled.Valid {
		r.RegisterEnabled = a.RegisterEnabled.Bool
	}
	if a.ClientID.Valid {
		r.ClientID = a.ClientID.String
	}
	if a.ClientSecretEnc.Valid {
		r.ClientSecretEnc = a.ClientSecretEnc
	}
	if a.AppleTeamID.Valid {
		r.AppleTeamID = a.AppleTeamID.String
	}
	if a.AppleKeyID.Valid {
		r.AppleKeyID = a.AppleKeyID.String
	}
	switch {
	case a.ClearApplePrivateKey:
		r.ApplePrivateKeyEnc = pgtype.Text{}
	case a.ApplePrivateKeyEnc.Valid:
		r.ApplePrivateKeyEnc = a.ApplePrivateKeyEnc
	}
	m.rows[a.Provider] = r
	return r, nil
}

func testBox(t *testing.T) *crypto.SecretBox {
	t.Helper()
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i + 3)
	}
	box, err := crypto.NewSecretBox(base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatal(err)
	}
	return box
}

func p8(t *testing.T, key any) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func newKey(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return k, p8(t, k)
}

// newSvc wires the service with a real resolver reading keys back from it.
func newSvc(t *testing.T, env appleauth.Config) (*Service, *memStore) {
	t.Helper()
	store := newMemStore()
	svc := New(store, testBox(t))
	r, err := appleauth.NewResolver(svc, env, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.SetAppleKeys(r)
	return svc, store
}

func str(s string) *string { return &s }
func boolp(b bool) *bool   { return &b }

func TestPatchAppleKeyValidation(t *testing.T) {
	ctx := context.Background()
	svc, _ := newSvc(t, appleauth.Config{})
	_, goodPEM := newKey(t)
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	p384, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)

	cases := map[string]PatchInput{
		"not pem":        {TeamID: str("ABCDE12345"), KeyID: str("KEY1234567"), PrivateKey: str("nope")},
		"rsa key":        {TeamID: str("ABCDE12345"), KeyID: str("KEY1234567"), PrivateKey: str(p8(t, rsaKey))},
		"p384 key":       {TeamID: str("ABCDE12345"), KeyID: str("KEY1234567"), PrivateKey: str(p8(t, p384))},
		"short team":     {TeamID: str("ABC"), KeyID: str("KEY1234567"), PrivateKey: str(goodPEM)},
		"long key id":    {TeamID: str("ABCDE12345"), KeyID: str("KEY12345678"), PrivateKey: str(goodPEM)},
		"missing key id": {TeamID: str("ABCDE12345"), PrivateKey: str(goodPEM)},
		"missing team":   {KeyID: str("KEY1234567"), PrivateKey: str(goodPEM)},
		"remove and set": {TeamID: str("ABCDE12345"), KeyID: str("KEY1234567"), PrivateKey: str(goodPEM), RemovePrivateKey: boolp(true)},
		"bad chars":      {TeamID: str("ABCDE-1234"), KeyID: str("KEY1234567"), PrivateKey: str(goodPEM)},
	}
	for name, in := range cases {
		if _, err := svc.Patch(ctx, "apple", in); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("%s: expected ErrInvalidRequest, got %v", name, err)
		}
	}
	if _, err := svc.Patch(ctx, "google", PatchInput{PrivateKey: str(goodPEM)}); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("google with apple key fields: %v", err)
	}

	got, err := svc.Patch(ctx, "apple", PatchInput{TeamID: str("abcde12345"), KeyID: str(" KEY1234567 "), PrivateKey: str(goodPEM)})
	if err != nil {
		t.Fatalf("valid key: %v", err)
	}
	if *got.TeamID != "ABCDE12345" || *got.KeyID != "KEY1234567" || !*got.PrivateKeyConfigured || *got.PrivateKeySource != appleauth.SourceDB {
		t.Fatalf("unexpected settings: %+v", got)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "PRIVATE KEY") || strings.Contains(string(raw), "private_key\"") {
		t.Fatalf("response leaks key: %s", raw)
	}
	key, ok, err := svc.AppleSigningKey(ctx)
	if err != nil || !ok || strings.TrimSpace(key.PrivateKey) != strings.TrimSpace(goodPEM) {
		t.Fatalf("stored key not readable: ok=%v err=%v", ok, err)
	}

	got, err = svc.Patch(ctx, "apple", PatchInput{RemovePrivateKey: boolp(true)})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if *got.PrivateKeyConfigured || *got.KeyID != "" || *got.PrivateKeySource != appleauth.SourceNone {
		t.Fatalf("key not removed: %+v", got)
	}
	if _, ok, _ := svc.AppleSigningKey(ctx); ok {
		t.Fatal("key still stored")
	}
}

func TestAppleEnableRequiresKeyOrSecret(t *testing.T) {
	ctx := context.Background()
	svc, _ := newSvc(t, appleauth.Config{})
	_, pemText := newKey(t)

	if _, err := svc.Patch(ctx, "apple", PatchInput{ClientID: str("com.example.web"), LoginEnabled: boolp(true)}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("apple without key/secret: %v", err)
	}
	if _, err := svc.Patch(ctx, "google", PatchInput{ClientID: str("g"), LoginEnabled: boolp(true)}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("google without secret: %v", err)
	}
	if _, err := svc.Patch(ctx, "apple", PatchInput{
		ClientID: str("com.example.web"), LoginEnabled: boolp(true),
		TeamID: str("ABCDE12345"), KeyID: str("KEY1234567"), PrivateKey: str(pemText),
	}); err != nil {
		t.Fatalf("apple with key, no secret: %v", err)
	}
	if ok, _ := svc.IsLoginEnabled(ctx, "apple"); !ok {
		t.Fatal("apple login should be enabled with a key")
	}
	// Removing the only credential while enabled is refused.
	if _, err := svc.Patch(ctx, "apple", PatchInput{RemovePrivateKey: boolp(true)}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("remove last credential: %v", err)
	}
}

func TestAdapterConfigGeneratesAppleSecret(t *testing.T) {
	ctx := context.Background()
	envKey, envPEM := newKey(t)
	svc, _ := newSvc(t, appleauth.Config{TeamID: "ENVTEAM001", KeyID: "ENVKEY0001", PrivateKey: envPEM})
	dbKey, dbPEM := newKey(t)
	if _, err := svc.Patch(ctx, "apple", PatchInput{
		ClientID: str("com.example.web"), ClientSecret: str("stale-pasted-jwt"), LoginEnabled: boolp(true),
		TeamID: str("ABCDE12345"), KeyID: str("KEY1234567"), PrivateKey: str(dbPEM),
	}); err != nil {
		t.Fatal(err)
	}
	before := time.Now()
	cfg, err := svc.GetAdapterOAuthConfig(ctx, "apple")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Enabled || cfg.ClientID != "com.example.web" || cfg.ClientSecret == "stale-pasted-jwt" || cfg.ClientSecretExpiresAt == nil {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if _, err := jwt.Parse(cfg.ClientSecret, func(*jwt.Token) (any, error) { return &envKey.PublicKey, nil }); err == nil {
		t.Fatal("secret must be signed with the DB key, not env")
	}
	claims := jwt.MapClaims{}
	tok, err := jwt.ParseWithClaims(cfg.ClientSecret, claims, func(*jwt.Token) (any, error) { return &dbKey.PublicKey, nil },
		jwt.WithValidMethods([]string{"ES256"}))
	if err != nil || !tok.Valid {
		t.Fatalf("secret does not verify with DB key: %v", err)
	}
	if tok.Header["kid"] != "KEY1234567" {
		t.Fatalf("kid = %v", tok.Header["kid"])
	}
	if iss, _ := claims.GetIssuer(); iss != "ABCDE12345" {
		t.Fatalf("iss = %q", iss)
	}
	if sub, _ := claims.GetSubject(); sub != "com.example.web" {
		t.Fatalf("sub = %q", sub)
	}
	if aud, _ := claims.GetAudience(); len(aud) != 1 || aud[0] != "https://appleid.apple.com" {
		t.Fatalf("aud = %v", aud)
	}
	exp, _ := claims.GetExpirationTime()
	want := before.Add(AppleWebClientSecretTTL)
	if exp == nil || exp.Before(want.Add(-time.Minute)) || exp.After(want.Add(time.Minute)) {
		t.Fatalf("exp = %v, want ~%v", exp, want)
	}
	if !cfg.ClientSecretExpiresAt.Equal(exp.Time) {
		t.Fatalf("client_secret_expires_at %v != exp %v", cfg.ClientSecretExpiresAt, exp.Time)
	}
	if AppleWebClientSecretTTL >= appleauth.MaxClientSecretTTL || AppleWebClientSecretTTL < time.Hour {
		t.Fatal("web secret TTL must sit well between the frontend cache TTL and Apple's cap")
	}
}

func TestAdapterConfigFallsBackToPastedSecret(t *testing.T) {
	ctx := context.Background()
	svc, _ := newSvc(t, appleauth.Config{})
	if _, err := svc.Patch(ctx, "apple", PatchInput{
		ClientID: str("com.example.web"), ClientSecret: str("pasted-jwt"), LoginEnabled: boolp(true),
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := svc.GetAdapterOAuthConfig(ctx, "apple")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Enabled || cfg.ClientSecret != "pasted-jwt" || cfg.ClientSecretExpiresAt != nil {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	got, _ := svc.Get(ctx, "google")
	if got.TeamID != nil || got.PrivateKeyConfigured != nil {
		t.Fatal("apple fields must be omitted for other providers")
	}
}
