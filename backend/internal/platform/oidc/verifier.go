// Package oidc verifies OpenID Connect id_tokens issued to native (mobile)
// clients by Apple and Google. Signatures are checked against the provider's
// JWKS (cached), together with iss, exp, aud and an optional nonce.
package oidc

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	ProviderApple  = "apple"
	ProviderGoogle = "google"

	defaultJWKSTTL    = time.Hour
	minRefetchBackoff = time.Minute
	clockLeeway       = time.Minute
)

var (
	// ErrInvalidToken is returned for any signature / claim validation failure.
	ErrInvalidToken = errors.New("oidc: invalid id token")
	// ErrUnknownProvider is returned for providers without a configuration.
	ErrUnknownProvider = errors.New("oidc: unknown provider")
)

// ProviderConfig describes where to fetch keys and which issuers are valid.
type ProviderConfig struct {
	Issuers []string
	JWKSURL string
}

// DefaultProviders returns the Apple and Google production endpoints.
func DefaultProviders() map[string]ProviderConfig {
	return map[string]ProviderConfig{
		ProviderApple: {
			Issuers: []string{"https://appleid.apple.com"},
			JWKSURL: "https://appleid.apple.com/auth/keys",
		},
		ProviderGoogle: {
			Issuers: []string{"https://accounts.google.com", "accounts.google.com"},
			JWKSURL: "https://www.googleapis.com/oauth2/v3/certs",
		},
	}
}

// Claims is the verified subset of id_token claims used for sign-in.
type Claims struct {
	Provider       string
	Subject        string
	Email          string
	EmailVerified  bool
	IsPrivateEmail bool
	GivenName      string
	FamilyName     string
	Audience       string
}

// Verifier validates id_tokens. Safe for concurrent use.
type Verifier struct {
	client    *http.Client
	providers map[string]ProviderConfig
	now       func() time.Time

	mu    sync.Mutex
	cache map[string]*keySet
}

type keySet struct {
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
	expiresAt time.Time
}

// New creates a verifier. A nil client uses a 10s-timeout default.
func New(providers map[string]ProviderConfig, client *http.Client) *Verifier {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Verifier{client: client, providers: providers, now: time.Now, cache: map[string]*keySet{}}
}

// SetClock overrides time (tests).
func (v *Verifier) SetClock(now func() time.Time) { v.now = now }

type rawClaims struct {
	jwt.RegisteredClaims
	Email          string `json:"email"`
	EmailVerified  any    `json:"email_verified"`
	IsPrivateEmail any    `json:"is_private_email"`
	Nonce          string `json:"nonce"`
	GivenName      string `json:"given_name"`
	FamilyName     string `json:"family_name"`
}

// Verify checks raw against provider keys. audiences must contain the token's
// aud. When nonce is non-empty the token nonce must equal it, or equal its
// hex SHA-256 (clients that hash the nonce before handing it to the SDK).
func (v *Verifier) Verify(ctx context.Context, provider, raw string, audiences []string, nonce string) (Claims, error) {
	cfg, ok := v.providers[provider]
	if !ok {
		return Claims{}, ErrUnknownProvider
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || len(audiences) == 0 {
		return Claims{}, ErrInvalidToken
	}
	var claims rawClaims
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(clockLeeway),
		jwt.WithTimeFunc(v.now),
	)
	_, err := parser.ParseWithClaims(raw, &claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		return v.key(ctx, provider, cfg, kid)
	})
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if !containsString(cfg.Issuers, claims.Issuer) {
		return Claims{}, fmt.Errorf("%w: issuer", ErrInvalidToken)
	}
	aud := matchAudience(claims.Audience, audiences)
	if aud == "" {
		return Claims{}, fmt.Errorf("%w: audience", ErrInvalidToken)
	}
	if strings.TrimSpace(claims.Subject) == "" {
		return Claims{}, fmt.Errorf("%w: subject", ErrInvalidToken)
	}
	if nonce = strings.TrimSpace(nonce); nonce != "" && !nonceMatches(claims.Nonce, nonce) {
		return Claims{}, fmt.Errorf("%w: nonce", ErrInvalidToken)
	}
	return Claims{
		Provider:       provider,
		Subject:        claims.Subject,
		Email:          strings.ToLower(strings.TrimSpace(claims.Email)),
		EmailVerified:  truthy(claims.EmailVerified),
		IsPrivateEmail: truthy(claims.IsPrivateEmail),
		GivenName:      strings.TrimSpace(claims.GivenName),
		FamilyName:     strings.TrimSpace(claims.FamilyName),
		Audience:       aud,
	}, nil
}

func (v *Verifier) key(ctx context.Context, provider string, cfg ProviderConfig, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	now := v.now()
	set := v.cache[provider]
	if set != nil && now.Before(set.expiresAt) {
		if k, ok := set.keys[kid]; ok {
			return k, nil
		}
	}
	// Unknown kid (key rotation) or expired cache: refetch, but not more
	// often than minRefetchBackoff so bogus kids cannot hammer the provider.
	if set != nil && now.Sub(set.fetchedAt) < minRefetchBackoff {
		if k, ok := set.keys[kid]; ok {
			return k, nil
		}
		return nil, fmt.Errorf("unknown key id %q", kid)
	}
	fresh, err := v.fetch(ctx, cfg.JWKSURL, now)
	if err != nil {
		if set != nil {
			if k, ok := set.keys[kid]; ok {
				return k, nil
			}
		}
		return nil, err
	}
	v.cache[provider] = fresh
	if k, ok := fresh.keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("unknown key id %q", kid)
}

type jwks struct {
	Keys []struct {
		Kty string `json:"kty"`
		Kid string `json:"kid"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

func (v *Verifier) fetch(ctx context.Context, url string, now time.Time) (*keySet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch jwks: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch jwks: status %d", res.StatusCode)
	}
	var body jwks
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode jwks: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey, len(body.Keys))
	for _, k := range body.Keys {
		if k.Kty != "RSA" {
			continue
		}
		nb, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eb, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: int(new(big.Int).SetBytes(eb).Int64())}
	}
	if len(keys) == 0 {
		return nil, errors.New("jwks has no RSA keys")
	}
	return &keySet{keys: keys, fetchedAt: now, expiresAt: now.Add(cacheTTL(res.Header.Get("Cache-Control")))}, nil
}

func cacheTTL(header string) time.Duration {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if v, ok := strings.CutPrefix(part, "max-age="); ok {
			var secs int
			if _, err := fmt.Sscanf(v, "%d", &secs); err == nil && secs > 0 {
				ttl := time.Duration(secs) * time.Second
				if ttl > 24*time.Hour {
					ttl = 24 * time.Hour
				}
				return ttl
			}
		}
	}
	return defaultJWKSTTL
}

func matchAudience(tokenAud jwt.ClaimStrings, allowed []string) string {
	for _, a := range tokenAud {
		if containsString(allowed, a) {
			return a
		}
	}
	return ""
}

func containsString(list []string, v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	for _, item := range list {
		if strings.TrimSpace(item) == v {
			return true
		}
	}
	return false
}

func nonceMatches(tokenNonce, nonce string) bool {
	if tokenNonce == "" {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(tokenNonce), []byte(nonce)) == 1 {
		return true
	}
	sum := sha256.Sum256([]byte(nonce))
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(tokenNonce)), []byte(hex.EncodeToString(sum[:]))) == 1
}

// truthy handles Apple's string-encoded booleans ("true") as well as JSON bools.
func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(strings.TrimSpace(t), "true")
	default:
		return false
	}
}
