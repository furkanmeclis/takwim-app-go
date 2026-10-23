// Package appleauth talks to Sign in with Apple's token endpoints: exchanging
// a native authorization code for a refresh token and revoking tokens when an
// account is deleted (App Store Review Guideline 5.1.1(v)).
package appleauth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	defaultBaseURL = "https://appleid.apple.com"
	audience       = "https://appleid.apple.com"
)

// ErrNotConfigured means no signing key is available to build client secrets.
var ErrNotConfigured = errors.New("appleauth: signing key is not configured")

// Config holds the Sign in with Apple private key (.p8) material.
type Config struct {
	TeamID     string
	KeyID      string
	PrivateKey string // PEM (PKCS#8); literal "\n" sequences are accepted
}

// Client performs Apple token calls. A zero-key client can still revoke with
// a caller-supplied client secret (e.g. the web client secret JWT).
type Client struct {
	cfg     Config
	key     *ecdsa.PrivateKey
	http    *http.Client
	baseURL string
	now     func() time.Time
}

// New parses the key (when present). An invalid key is an error; an empty
// config yields a client whose GenerateClientSecret returns ErrNotConfigured.
func New(cfg Config, httpClient *http.Client) (*Client, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	c := &Client{cfg: cfg, http: httpClient, baseURL: defaultBaseURL, now: time.Now}
	if strings.TrimSpace(cfg.PrivateKey) == "" || strings.TrimSpace(cfg.TeamID) == "" || strings.TrimSpace(cfg.KeyID) == "" {
		return c, nil
	}
	ec, err := ParsePrivateKey(cfg.PrivateKey)
	if err != nil {
		return nil, err
	}
	c.key = ec
	return c, nil
}

// ParsePrivateKey parses a Sign in with Apple .p8 key: PEM, PKCS#8, EC P-256.
// Literal "\n" sequences (single-line env values) are accepted.
func ParsePrivateKey(pemText string) (*ecdsa.PrivateKey, error) {
	pemText = strings.TrimSpace(strings.ReplaceAll(pemText, `\n`, "\n"))
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, errors.New("appleauth: private key is not PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("appleauth: parse private key: %w", err)
	}
	ec, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("appleauth: private key must be an EC (ES256) key")
	}
	if ec.Curve != elliptic.P256() {
		return nil, errors.New("appleauth: private key must use the P-256 curve")
	}
	return ec, nil
}

// SetBaseURL overrides the Apple endpoint host (tests).
func (c *Client) SetBaseURL(u string) { c.baseURL = strings.TrimRight(u, "/") }

// Configured reports whether client secrets can be generated.
func (c *Client) Configured() bool { return c != nil && c.key != nil }

// GenerateClientSecret builds the short-lived ES256 client_secret JWT for clientID.
func (c *Client) GenerateClientSecret(clientID string) (string, error) {
	secret, _, err := c.GenerateClientSecretTTL(clientID, 5*time.Minute)
	return secret, err
}

// MaxClientSecretTTL is Apple's cap on client_secret lifetime (6 months).
const MaxClientSecretTTL = 180 * 24 * time.Hour

// GenerateClientSecretTTL builds an ES256 client_secret JWT valid for ttl and
// returns its expiry.
func (c *Client) GenerateClientSecretTTL(clientID string, ttl time.Duration) (string, time.Time, error) {
	if !c.Configured() {
		return "", time.Time{}, ErrNotConfigured
	}
	if ttl <= 0 || ttl > MaxClientSecretTTL {
		return "", time.Time{}, fmt.Errorf("appleauth: client secret ttl %s out of range", ttl)
	}
	now := c.now().UTC()
	exp := now.Add(ttl)
	// aud is a plain string as in Apple's docs (ClaimStrings would encode an array).
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss": c.cfg.TeamID,
		"sub": clientID,
		"aud": audience,
		"iat": now.Unix(),
		"exp": exp.Unix(),
	})
	tok.Header["kid"] = c.cfg.KeyID
	signed, err := tok.SignedString(c.key)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, exp.Truncate(time.Second), nil
}

// ExchangeCode trades a native authorization code for a refresh token.
func (c *Client) ExchangeCode(ctx context.Context, clientID, code string) (string, error) {
	secret, err := c.GenerateClientSecret(clientID)
	if err != nil {
		return "", err
	}
	form := url.Values{
		"client_id":     {clientID},
		"client_secret": {secret},
		"code":          {strings.TrimSpace(code)},
		"grant_type":    {"authorization_code"},
	}
	body, err := c.post(ctx, "/auth/token", form)
	if err != nil {
		return "", err
	}
	var out struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("appleauth: decode token response: %w", err)
	}
	if out.RefreshToken == "" {
		return "", errors.New("appleauth: token response has no refresh_token")
	}
	return out.RefreshToken, nil
}

// Revoke invalidates a refresh token. clientSecret may be empty to use a
// generated secret for clientID.
func (c *Client) Revoke(ctx context.Context, clientID, clientSecret, refreshToken string) error {
	if strings.TrimSpace(clientSecret) == "" {
		s, err := c.GenerateClientSecret(clientID)
		if err != nil {
			return err
		}
		clientSecret = s
	}
	form := url.Values{
		"client_id":       {clientID},
		"client_secret":   {clientSecret},
		"token":           {refreshToken},
		"token_type_hint": {"refresh_token"},
	}
	_, err := c.post(ctx, "/auth/revoke", form)
	return err
}

func (c *Client) post(ctx context.Context, path string, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("appleauth: %s: %w", path, err)
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("appleauth: %s: status %d: %s", path, res.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}
