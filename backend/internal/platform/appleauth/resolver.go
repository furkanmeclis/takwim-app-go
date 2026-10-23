package appleauth

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Key sources reported by Resolver.Source.
const (
	SourceDB   = "db"
	SourceEnv  = "env"
	SourceNone = "none"
)

// DefaultResolverTTL bounds how long a resolved key is reused before the key
// source is read again. Invalidate drops it immediately in this process; the
// TTL lets other processes (separate API replicas / worker) pick up changes.
const DefaultResolverTTL = time.Minute

// KeySource returns key material uploaded by an admin (stored in the DB).
// ok=false means nothing is stored and the env fallback applies.
type KeySource interface {
	AppleSigningKey(ctx context.Context) (cfg Config, ok bool, err error)
}

// Resolver picks the Sign in with Apple signing key at runtime: the DB key
// first, the AUTH_APPLE_* env key as fallback. The parsed key is cached.
type Resolver struct {
	src     KeySource
	env     *Client
	http    *http.Client
	ttl     time.Duration
	baseURL string
	now     func() time.Time

	mu     sync.Mutex
	cached *resolvedKey
}

type resolvedKey struct {
	client   *Client
	source   string
	loadedAt time.Time
}

// NewResolver builds a resolver. An invalid env key is an error (as before).
func NewResolver(src KeySource, env Config, httpClient *http.Client) (*Resolver, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	envClient, err := New(trimConfig(env), httpClient)
	if err != nil {
		return nil, err
	}
	return &Resolver{src: src, env: envClient, http: httpClient, ttl: DefaultResolverTTL, now: time.Now}, nil
}

func trimConfig(cfg Config) Config {
	return Config{
		TeamID:     strings.TrimSpace(cfg.TeamID),
		KeyID:      strings.TrimSpace(cfg.KeyID),
		PrivateKey: cfg.PrivateKey,
	}
}

// SetTTL overrides the cache lifetime (tests).
func (r *Resolver) SetTTL(ttl time.Duration) { r.ttl = ttl }

// SetBaseURL overrides the Apple endpoint host for every resolved client (tests).
func (r *Resolver) SetBaseURL(u string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.baseURL = strings.TrimRight(u, "/")
	r.env.SetBaseURL(u)
	r.cached = nil
}

// Invalidate drops the cached key; the next call re-reads the key source.
func (r *Resolver) Invalidate() {
	r.mu.Lock()
	r.cached = nil
	r.mu.Unlock()
}

func (r *Resolver) envResolved(now time.Time) *resolvedKey {
	source := SourceNone
	if r.env.Configured() {
		source = SourceEnv
	}
	return &resolvedKey{client: r.env, source: source, loadedAt: now}
}

func (r *Resolver) current(ctx context.Context) *resolvedKey {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	if r.cached != nil && now.Sub(r.cached.loadedAt) < r.ttl {
		return r.cached
	}
	if r.src == nil {
		r.cached = r.envResolved(now)
		return r.cached
	}
	cfg, ok, err := r.src.AppleSigningKey(ctx)
	if err != nil {
		// Transient read failure: keep a previously resolved key, otherwise
		// use env without caching so the next call retries the source.
		if r.cached != nil {
			return r.cached
		}
		return r.envResolved(now)
	}
	res := r.envResolved(now)
	if ok {
		if c, err := New(trimConfig(cfg), r.http); err == nil && c.Configured() {
			if r.baseURL != "" {
				c.SetBaseURL(r.baseURL)
			}
			res = &resolvedKey{client: c, source: SourceDB, loadedAt: now}
		}
	}
	r.cached = res
	return res
}

// Source reports where the active key comes from: db, env or none.
func (r *Resolver) Source(ctx context.Context) string { return r.current(ctx).source }

// Configured reports whether client secrets can be generated.
func (r *Resolver) Configured(ctx context.Context) bool { return r.current(ctx).client.Configured() }

// GenerateClientSecretTTL builds a client_secret JWT for clientID with the active key.
func (r *Resolver) GenerateClientSecretTTL(ctx context.Context, clientID string, ttl time.Duration) (string, time.Time, error) {
	return r.current(ctx).client.GenerateClientSecretTTL(clientID, ttl)
}

// ExchangeCode trades a native authorization code for a refresh token.
func (r *Resolver) ExchangeCode(ctx context.Context, clientID, code string) (string, error) {
	return r.current(ctx).client.ExchangeCode(ctx, clientID, code)
}

// Revoke invalidates a refresh token (see Client.Revoke).
func (r *Resolver) Revoke(ctx context.Context, clientID, clientSecret, refreshToken string) error {
	return r.current(ctx).client.Revoke(ctx, clientID, clientSecret, refreshToken)
}

// EnvConfigured reports whether the AUTH_APPLE_* env fallback key is set.
func (r *Resolver) EnvConfigured() bool { return r.env.Configured() }
