package usecase

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/appleauth"
	"github.com/golang-jwt/jwt/v5"
)

type dbKeySource struct{ cfg appleauth.Config }

func (s dbKeySource) AppleSigningKey(context.Context) (appleauth.Config, bool, error) {
	return s.cfg, true, nil
}

func ecPEM(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(k)
	return k, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

// Deactivation revokes stored Apple tokens with a client secret signed by the
// admin-uploaded (DB) key, not the env key.
func TestDeactivateRevokesAppleTokensWithDBKey(t *testing.T) {
	_, envPEM := ecPEM(t)
	dbKey, dbPEM := ecPEM(t)

	var mu sync.Mutex
	revoked := map[string]string{} // token -> client_id
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		claims := jwt.MapClaims{}
		tok, err := jwt.ParseWithClaims(r.PostForm.Get("client_secret"), claims,
			func(*jwt.Token) (any, error) { return &dbKey.PublicKey, nil })
		sub, _ := claims.GetSubject()
		if err != nil || !tok.Valid || tok.Header["kid"] != "DBKEY00001" || sub != r.PostForm.Get("client_id") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		revoked[r.PostForm.Get("token")] = r.PostForm.Get("client_id")
		mu.Unlock()
	}))
	defer srv.Close()

	resolver, err := appleauth.NewResolver(
		dbKeySource{cfg: appleauth.Config{TeamID: "DBTEAM0001", KeyID: "DBKEY00001", PrivateKey: dbPEM}},
		appleauth.Config{TeamID: "ENVTEAM001", KeyID: "ENVKEY0001", PrivateKey: envPEM}, nil)
	if err != nil {
		t.Fatal(err)
	}
	resolver.SetBaseURL(srv.URL)

	e := newFlowEnv(t)
	e.uc.SetNativeOAuth(NativeOAuthConfig{
		Verifier: e.verify, Apple: resolver, Clients: fakeClients{id: "com.example.web"},
		AppleClientIDs: []string{"com.example.app"},
	})
	u := e.user(t, "ada@example.com")
	nativeRT, _ := e.uc.box.Encrypt("native-rt")
	webRT, _ := e.uc.box.Encrypt("web-rt")
	cid := "com.example.app"
	e.repo.accounts = append(e.repo.accounts,
		model.OAuthAccountRecord{ID: 1, UserID: u.ID, UserUUID: u.UUID, Provider: "apple", ProviderAccountID: "a1", RefreshTokenEnc: &nativeRT, ClientID: &cid},
		model.OAuthAccountRecord{ID: 2, UserID: u.ID, UserUUID: u.UUID, Provider: "apple", ProviderAccountID: "a2", RefreshTokenEnc: &webRT},
	)

	if err := e.uc.DeactivateAccount(context.Background(), u.UUID, "", true); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if revoked["native-rt"] != "com.example.app" || revoked["web-rt"] != "com.example.web" {
		t.Fatalf("tokens not revoked with DB key: %v", revoked)
	}
	if !e.repo.cleared[1] || !e.repo.cleared[2] {
		t.Fatal("revoked tokens must be cleared")
	}
}
