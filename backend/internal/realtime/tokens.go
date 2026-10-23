package realtime

import (
	"fmt"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/golang-jwt/jwt/v5"
)

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"

// TokenIssuer mints Centrifugo connection and subscription JWTs.
// Connection JWT sub is the user UUID string (not a numeric id).
type TokenIssuer struct {
	secret []byte
	ttl    time.Duration
	wsURL  string
}

// NewTokenIssuer creates a JWT issuer from Centrifugo config.
// Returns (nil, nil) when Centrifugo is disabled.
func NewTokenIssuer(cfg config.CentrifugoConfig) (*TokenIssuer, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if cfg.TokenHMAC == "" {
		return nil, fmt.Errorf("realtime: CENTRIFUGO_TOKEN_HMAC_SECRET is required when enabled")
	}
	ttl := cfg.TokenTTL
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &TokenIssuer{
		secret: []byte(cfg.TokenHMAC),
		ttl:    ttl,
		wsURL:  cfg.WSURL,
	}, nil
}

// Enabled reports whether the issuer is active.
func (i *TokenIssuer) Enabled() bool {
	return i != nil
}

// WSURL returns the public WebSocket URL for clients.
func (i *TokenIssuer) WSURL() string {
	if i == nil {
		return ""
	}
	return i.wsURL
}

// ConnectionToken issues a Centrifugo connection JWT for sub=userUUID.
func (i *TokenIssuer) ConnectionToken(userUUID string) (string, time.Time, error) {
	if i == nil {
		return "", time.Time{}, fmt.Errorf("realtime: token issuer disabled")
	}
	if userUUID == "" {
		return "", time.Time{}, fmt.Errorf("realtime: user uuid is required")
	}
	now := time.Now().UTC()
	exp := now.Add(i.ttl)
	claims := jwt.MapClaims{
		"sub": userUUID,
		"exp": exp.Unix(),
		"iat": now.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(i.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("realtime: sign connection token: %w", err)
	}
	return signed, exp, nil
}

// SubscriptionToken issues a channel subscription JWT.
func (i *TokenIssuer) SubscriptionToken(userUUID, channel string) (string, time.Time, error) {
	if i == nil {
		return "", time.Time{}, fmt.Errorf("realtime: token issuer disabled")
	}
	if userUUID == "" {
		return "", time.Time{}, fmt.Errorf("realtime: user uuid is required")
	}
	if channel == "" {
		return "", time.Time{}, fmt.Errorf("realtime: channel is required")
	}
	now := time.Now().UTC()
	exp := now.Add(i.ttl)
	claims := jwt.MapClaims{
		"sub":     userUUID,
		"channel": channel,
		"exp":     exp.Unix(),
		"iat":     now.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(i.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("realtime: sign subscription token: %w", err)
	}
	return signed, exp, nil
}

// AnonymousConnectionToken issues a Centrifugo connection JWT with an empty
// sub (anonymous user) that expires at exp. It grants no channel by itself:
// every namespace keeps allow_subscribe_for_client=false, so the holder can
// only join channels it also has a subscription token for.
func (i *TokenIssuer) AnonymousConnectionToken(exp time.Time) (string, error) {
	if i == nil {
		return "", fmt.Errorf("realtime: token issuer disabled")
	}
	claims := jwt.MapClaims{
		"sub": "",
		"exp": exp.Unix(),
		"iat": time.Now().UTC().Unix(),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(i.secret)
	if err != nil {
		return "", fmt.Errorf("realtime: sign anonymous connection token: %w", err)
	}
	return signed, nil
}

// AnonymousSubscriptionToken issues a subscription JWT for one channel to an
// anonymous connection (sub must match the connection's empty sub).
func (i *TokenIssuer) AnonymousSubscriptionToken(channel string, exp time.Time) (string, error) {
	if i == nil {
		return "", fmt.Errorf("realtime: token issuer disabled")
	}
	if channel == "" {
		return "", fmt.Errorf("realtime: channel is required")
	}
	claims := jwt.MapClaims{
		"sub":     "",
		"channel": channel,
		"exp":     exp.Unix(),
		"iat":     time.Now().UTC().Unix(),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(i.secret)
	if err != nil {
		return "", fmt.Errorf("realtime: sign anonymous subscription token: %w", err)
	}
	return signed, nil
}
