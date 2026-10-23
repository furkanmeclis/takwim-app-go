package jwt

import (
	"errors"
	"fmt"
	"strings"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var ErrInvalidToken = errors.New("invalid token")

// Token kinds (the "knd" claim). Staff tokens (web panel, staff mobile app)
// are the only kind the /v1 Bearer middleware accepts. Customer panel and
// single-appointment links get their own kinds and their own middleware
// (TEC-909 / TEC-910), so a customer token can never reach a staff route.
const (
	KindStaff = "staff"
	// KindCustomer is the customer panel session (takwim.link/panel,
	// /v1/customer/*): the subject is the platform customer identity, the
	// sid its customer_sessions row.
	KindCustomer = "customer"
)

// ErrWrongKind is returned when a token of another kind is presented.
var ErrWrongKind = errors.New("token kind not accepted")

// Claims are verified identity data carried by an access token.
type Claims struct {
	Roles          []string `json:"roles"`
	IsSuperAdmin   bool     `json:"is_super_admin"`
	ImpersonatorID *string  `json:"imp,omitempty"`
	SessionID      string   `json:"sid,omitempty"`
	// Kind is the token kind; empty reads as KindStaff (tokens issued
	// before the claim existed).
	Kind string `json:"knd,omitempty"`
	jwtlib.RegisteredClaims
}

// AccessInput is the payload used when issuing an access token.
type AccessInput struct {
	UserID         uuid.UUID
	Roles          []string
	IsSuperAdmin   bool
	ImpersonatorID *uuid.UUID
	SessionID      uuid.UUID
}

// Manager issues and validates access JWTs. Refresh tokens are opaque (not JWT).
type Manager struct {
	accessSecret []byte
	accessTTL    time.Duration
	refreshTTL   time.Duration
	now          func() time.Time
}

// NewManager creates a token manager from a non-empty access signing secret.
func NewManager(accessSecret string, accessTTL, refreshTTL time.Duration) (*Manager, error) {
	if accessSecret == "" {
		return nil, errors.New("jwt: access signing secret is required")
	}
	if accessTTL <= 0 || refreshTTL <= 0 {
		return nil, errors.New("jwt: token TTLs must be positive")
	}
	return &Manager{
		accessSecret: []byte(accessSecret),
		accessTTL:    accessTTL,
		refreshTTL:   refreshTTL,
		now:          time.Now,
	}, nil
}

// AccessTTL returns the configured access token lifetime.
func (m *Manager) AccessTTL() time.Duration { return m.accessTTL }

// RefreshTTL returns the configured opaque refresh token lifetime.
func (m *Manager) RefreshTTL() time.Duration { return m.refreshTTL }

// IssueAccess creates a short-lived access token.
func (m *Manager) IssueAccess(in AccessInput) (string, time.Time, error) {
	if in.UserID == uuid.Nil {
		return "", time.Time{}, errors.New("jwt: user id is required")
	}
	now := m.now().UTC()
	expiresAt := now.Add(m.accessTTL)
	roles := in.Roles
	if roles == nil {
		roles = []string{}
	}
	var imp *string
	if in.ImpersonatorID != nil && *in.ImpersonatorID != uuid.Nil {
		s := in.ImpersonatorID.String()
		imp = &s
	}
	var sid string
	if in.SessionID != uuid.Nil {
		sid = in.SessionID.String()
	}
	claims := Claims{
		Roles:          roles,
		IsSuperAdmin:   in.IsSuperAdmin,
		ImpersonatorID: imp,
		SessionID:      sid,
		Kind:           KindStaff,
		RegisteredClaims: jwtlib.RegisteredClaims{
			Subject:   in.UserID.String(),
			ExpiresAt: jwtlib.NewNumericDate(expiresAt),
			IssuedAt:  jwtlib.NewNumericDate(now),
			ID:        uuid.NewString(),
		},
	}
	signed, err := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims).SignedString(m.accessSecret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("jwt: sign access token: %w", err)
	}
	return signed, expiresAt, nil
}

// ParseAccess validates an access token and returns claims.
func (m *Manager) ParseAccess(token string) (Claims, error) {
	var claims Claims
	parsed, err := jwtlib.ParseWithClaims(token, &claims, func(token *jwtlib.Token) (any, error) {
		if token.Method != jwtlib.SigningMethodHS256 {
			return nil, ErrInvalidToken
		}
		return m.accessSecret, nil
	})
	if err != nil || !parsed.Valid || claims.Subject == "" {
		return Claims{}, ErrInvalidToken
	}
	if _, err := uuid.Parse(claims.Subject); err != nil {
		return Claims{}, ErrInvalidToken
	}
	if claims.Kind == "" {
		claims.Kind = KindStaff
	}
	if claims.Kind != KindStaff {
		return Claims{}, ErrWrongKind
	}
	if claims.Roles == nil {
		claims.Roles = []string{}
	}
	return claims, nil
}

// CustomerInput is the payload of a customer panel token.
type CustomerInput struct {
	IdentityID uuid.UUID
	SessionID  uuid.UUID
	ExpiresAt  time.Time
}

// IssueCustomer signs a customer panel token (kind customer). It expires
// with its session's hard cap; the session row decides revocation and idle
// expiry on every request.
func (m *Manager) IssueCustomer(in CustomerInput) (string, error) {
	if in.IdentityID == uuid.Nil || in.SessionID == uuid.Nil {
		return "", errors.New("jwt: identity and session are required")
	}
	now := m.now().UTC()
	if !in.ExpiresAt.After(now) {
		return "", errors.New("jwt: expiry must be in the future")
	}
	claims := Claims{
		Roles:     []string{},
		SessionID: in.SessionID.String(),
		Kind:      KindCustomer,
		RegisteredClaims: jwtlib.RegisteredClaims{
			Subject:   in.IdentityID.String(),
			ExpiresAt: jwtlib.NewNumericDate(in.ExpiresAt.UTC()),
			IssuedAt:  jwtlib.NewNumericDate(now),
			ID:        uuid.NewString(),
		},
	}
	signed, err := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims).SignedString(m.accessSecret)
	if err != nil {
		return "", fmt.Errorf("jwt: sign customer token: %w", err)
	}
	return signed, nil
}

// ParseCustomer validates a customer panel token. Staff tokens (and tokens
// without a kind, which read as staff) answer ErrWrongKind, so a staff
// token never opens a customer's history either.
func (m *Manager) ParseCustomer(token string) (Claims, error) {
	var claims Claims
	parsed, err := jwtlib.ParseWithClaims(token, &claims, func(token *jwtlib.Token) (any, error) {
		if token.Method != jwtlib.SigningMethodHS256 {
			return nil, ErrInvalidToken
		}
		return m.accessSecret, nil
	})
	if err != nil || !parsed.Valid || claims.Subject == "" {
		return Claims{}, ErrInvalidToken
	}
	if claims.Kind != KindCustomer {
		return Claims{}, ErrWrongKind
	}
	if _, err := uuid.Parse(claims.Subject); err != nil {
		return Claims{}, ErrInvalidToken
	}
	if claims.SessionUUID() == uuid.Nil {
		return Claims{}, ErrInvalidToken
	}
	return claims, nil
}

// UserUUID returns the subject as a UUID.
func (c Claims) UserUUID() (uuid.UUID, error) {
	return uuid.Parse(c.Subject)
}

// ImpersonatorUUID returns the optional impersonator claim as a UUID.
func (c Claims) ImpersonatorUUID() (*uuid.UUID, error) {
	if c.ImpersonatorID == nil || strings.TrimSpace(*c.ImpersonatorID) == "" {
		return nil, nil
	}
	id, err := uuid.Parse(*c.ImpersonatorID)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// SessionUUID returns the optional refresh-session id bound to this access token.
func (c Claims) SessionUUID() uuid.UUID {
	if strings.TrimSpace(c.SessionID) == "" {
		return uuid.Nil
	}
	id, err := uuid.Parse(c.SessionID)
	if err != nil {
		return uuid.Nil
	}
	return id
}
