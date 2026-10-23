// Package qrlogin implements "sign in on the web by scanning a QR with the
// signed-in mobile app".
//
// Flow: the web login page creates a session (POST /v1/auth/qr/sessions) and
// gets an unguessable session id (encoded in the QR as
// {frontend}/login/qr/{id}), a browser secret that never leaves the tab, and
// an anonymous Centrifugo connection + subscription token for one private
// channel (qrlogin:{channel id}). The phone loads the details, then approves
// or rejects; the backend publishes the outcome to the channel. On approval
// the tab receives a one-time exchange token and trades it, together with
// its browser secret, for a normal session (NextAuth credentials provider →
// POST /v1/auth/qr/exchange). All state lives in Redis with short TTLs.
package qrlogin

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/model"
	authusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/realtime"
	"github.com/google/uuid"
)

const (
	// SessionTTL is how long a QR stays valid; the web page refreshes it.
	SessionTTL = 2 * time.Minute
	// ExchangeTTL is the window the tab has to redeem an approval.
	ExchangeTTL = time.Minute
	// realtimeGrace keeps the channel tokens valid slightly past the session
	// so a late approval still reaches the tab.
	realtimeGrace = 30 * time.Second
	// maxSecretAttempts burns the session after this many wrong secrets.
	maxSecretAttempts = 5
	secretBytes       = 32
)

// ErrRealtimeDisabled is returned when Centrifugo is not configured.
var ErrRealtimeDisabled = errors.New("realtime is disabled")

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// ValidID reports whether s has the shape of a generated session id / secret.
func ValidID(s string) bool { return idPattern.MatchString(s) }

// Accounts is the slice of the auth use case QR login needs.
type Accounts interface {
	CheckQRLoginApprover(ctx context.Context, userUUID uuid.UUID) error
	IssueQRLoginSession(ctx context.Context, userUUID uuid.UUID, meta model.SessionMeta) (model.Tokens, authusecase.QRLoginUser, error)
}

// TokenIssuer mints anonymous Centrifugo tokens.
type TokenIssuer interface {
	Enabled() bool
	WSURL() string
	AnonymousConnectionToken(exp time.Time) (string, error)
	AnonymousSubscriptionToken(channel string, exp time.Time) (string, error)
}

// Service orchestrates QR sign-in.
type Service struct {
	store       *Store
	accounts    Accounts
	issuer      TokenIssuer
	pub         realtime.Publisher
	frontendURL string
	log         *slog.Logger
	now         func() time.Time
}

// NewService wires the QR login service.
func NewService(store *Store, accounts Accounts, issuer TokenIssuer, pub realtime.Publisher, frontendURL string, log *slog.Logger) *Service {
	if pub == nil {
		pub = realtime.NoopPublisher{}
	}
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		store: store, accounts: accounts, issuer: issuer, pub: pub,
		frontendURL: strings.TrimRight(frontendURL, "/"), log: log, now: time.Now,
	}
}

// RealtimeGrant is everything the tab needs to listen on its channel.
type RealtimeGrant struct {
	WSURL             string `json:"ws_url"`
	ConnectionToken   string `json:"connection_token"`
	SubscriptionToken string `json:"subscription_token"`
	Channel           string `json:"channel"`
}

// Created is the response to the web tab.
type Created struct {
	SessionID     string        `json:"session_id"`
	QRURL         string        `json:"qr_url"`
	BrowserSecret string        `json:"browser_secret"`
	ExpiresAt     time.Time     `json:"expires_at"`
	ExpiresIn     int64         `json:"expires_in"`
	Realtime      RealtimeGrant `json:"realtime"`
}

// Create starts a QR session for the requesting browser.
func (s *Service) Create(ctx context.Context, userAgent, ip string, loc Location) (Created, error) {
	if s.issuer == nil || !s.issuer.Enabled() {
		return Created{}, ErrRealtimeDisabled
	}
	sessionID, err := randomToken()
	if err != nil {
		return Created{}, err
	}
	channelID, err := randomToken()
	if err != nil {
		return Created{}, err
	}
	secret, err := randomToken()
	if err != nil {
		return Created{}, err
	}
	now := s.now().UTC()
	expires := now.Add(SessionTTL)
	channel := realtime.QRLoginChannel(channelID)
	tokenExp := expires.Add(realtimeGrace)
	connToken, err := s.issuer.AnonymousConnectionToken(tokenExp)
	if err != nil {
		return Created{}, err
	}
	subToken, err := s.issuer.AnonymousSubscriptionToken(channel, tokenExp)
	if err != nil {
		return Created{}, err
	}
	if len(userAgent) > maxUserAgentLen {
		userAgent = userAgent[:maxUserAgentLen]
	}
	if err := s.store.Create(ctx, sessionID, Session{
		ChannelID: channelID, SecretHash: hashSecret(secret),
		UserAgent: userAgent, IP: ip, Location: loc,
		CreatedAt: now, ExpiresAt: expires,
	}, SessionTTL); err != nil {
		return Created{}, err
	}
	return Created{
		SessionID:     sessionID,
		QRURL:         fmt.Sprintf("%s/login/qr/%s", s.frontendURL, sessionID),
		BrowserSecret: secret,
		ExpiresAt:     expires,
		ExpiresIn:     int64(SessionTTL.Seconds()),
		Realtime: RealtimeGrant{
			WSURL: s.issuer.WSURL(), ConnectionToken: connToken,
			SubscriptionToken: subToken, Channel: channel,
		},
	}, nil
}

// Details is what the phone shows before approving.
type Details struct {
	SessionID string     `json:"session_id"`
	Status    string     `json:"status"`
	Client    ClientInfo `json:"client"`
	IP        string     `json:"ip"`
	Location  *Location  `json:"location"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
}

// Approver is the signed-in phone user.
type Approver struct {
	UserUUID      uuid.UUID
	Impersonating bool
}

// ErrImpersonating blocks approvals from impersonation sessions.
var ErrImpersonating = errors.New("qr login cannot be approved while impersonating")

// Get returns session details and claims the session for the viewer.
func (s *Service) Get(ctx context.Context, sessionID string, viewer Approver) (Details, error) {
	if !ValidID(sessionID) {
		return Details{}, ErrNotFound
	}
	first, err := s.store.Claim(ctx, sessionID, viewer.UserUUID.String())
	if err != nil {
		return Details{}, err
	}
	sess, err := s.store.Get(ctx, sessionID)
	if err != nil {
		return Details{}, err
	}
	if first {
		s.publish(ctx, sess.ChannelID, map[string]any{"type": "scanned"})
	}
	status := sess.Status
	if status == StatusScanned {
		status = StatusPending
	}
	d := Details{
		SessionID: sessionID, Status: status, Client: ParseUserAgent(sess.UserAgent),
		IP: sess.IP, CreatedAt: sess.CreatedAt, ExpiresAt: sess.ExpiresAt,
	}
	if !sess.Location.Empty() {
		loc := sess.Location
		d.Location = &loc
	}
	return d, nil
}

// Approve signs the waiting tab in as the approver.
func (s *Service) Approve(ctx context.Context, sessionID string, approver Approver) error {
	if !ValidID(sessionID) {
		return ErrNotFound
	}
	if approver.Impersonating {
		return ErrImpersonating
	}
	if err := s.accounts.CheckQRLoginApprover(ctx, approver.UserUUID); err != nil {
		return err
	}
	sess, err := s.store.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	if !sess.ExpiresAt.IsZero() && s.now().UTC().After(sess.ExpiresAt) {
		return ErrNotFound
	}
	exchange, err := randomToken()
	if err != nil {
		return err
	}
	if err := s.store.Resolve(ctx, sessionID, approver.UserUUID.String(), StatusApproved, exchange, ExchangeTTL); err != nil {
		return err
	}
	s.publish(ctx, sess.ChannelID, map[string]any{"type": "approved", "exchange_token": exchange})
	return nil
}

// Reject tells the waiting tab the sign-in was refused.
func (s *Service) Reject(ctx context.Context, sessionID string, viewer Approver) error {
	if !ValidID(sessionID) {
		return ErrNotFound
	}
	sess, err := s.store.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	if err := s.store.Resolve(ctx, sessionID, viewer.UserUUID.String(), StatusRejected, "", ExchangeTTL); err != nil {
		return err
	}
	s.publish(ctx, sess.ChannelID, map[string]any{"type": "rejected"})
	return nil
}

// State is the tab's view, used once after (re)subscribing so an event
// published while the socket was down is not lost. It is not polled.
type State struct {
	Status        string `json:"status"`
	ExchangeToken string `json:"exchange_token,omitempty"`
}

// State returns the session status to the tab holding browserSecret.
func (s *Service) State(ctx context.Context, sessionID, browserSecret string) (State, error) {
	if !ValidID(sessionID) || !ValidID(browserSecret) {
		return State{}, ErrNotFound
	}
	status, token, err := s.store.State(ctx, sessionID, hashSecret(browserSecret), maxSecretAttempts)
	if err != nil {
		return State{}, err
	}
	out := State{Status: status}
	if status == StatusApproved {
		out.ExchangeToken = token
	}
	return out, nil
}

// Exchanged is the web session issued for an approved QR.
type Exchanged struct {
	model.Tokens
	User ExchangedUser `json:"user"`
}

// ExchangedUser identifies the signed-in account for NextAuth.
type ExchangedUser struct {
	UUID  string `json:"uuid"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// Exchange redeems an approval exactly once.
func (s *Service) Exchange(ctx context.Context, sessionID, browserSecret, exchangeToken string) (Exchanged, error) {
	if !ValidID(sessionID) || !ValidID(browserSecret) || !ValidID(exchangeToken) {
		return Exchanged{}, ErrInvalidSecret
	}
	consumed, err := s.store.Exchange(ctx, sessionID, hashSecret(browserSecret), exchangeToken, maxSecretAttempts)
	if err != nil {
		return Exchanged{}, err
	}
	userUUID, err := uuid.Parse(consumed.UserUUID)
	if err != nil {
		return Exchanged{}, ErrNotFound
	}
	tokens, user, err := s.accounts.IssueQRLoginSession(ctx, userUUID, model.SessionMeta{
		UserAgent: consumed.UserAgent, IP: consumed.IP,
	})
	if err != nil {
		return Exchanged{}, err
	}
	return Exchanged{
		Tokens: tokens,
		User:   ExchangedUser{UUID: user.UUID.String(), Email: user.Email, Name: user.Name},
	}, nil
}

// publish is best effort: the tab also reads State after subscribing.
func (s *Service) publish(ctx context.Context, channelID string, data map[string]any) {
	if channelID == "" {
		return
	}
	if err := s.pub.Publish(ctx, realtime.QRLoginChannel(channelID), data); err != nil {
		s.log.Warn("qrlogin_publish_failed", "error", err, "event", data["type"])
	}
}

func randomToken() (string, error) {
	buf := make([]byte, secretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("qrlogin: random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
