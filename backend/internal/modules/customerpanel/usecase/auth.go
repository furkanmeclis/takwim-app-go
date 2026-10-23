// Package usecase is the shared customer panel of Takwim (TEC-909,
// takwim.link/panel): sign-in by a one-time code sent to the phone, a
// customer session (token kind customer) and the person's reservations and
// payments across every business that linked the verified phone.
//
// Ownership always comes from the session: the identity's verified links
// (customer_identity_links) select the records, never a phone or id in the
// request. Reservation changes go through the reservations use cases with a
// guard on the locked record, so capacity and history work exactly as on
// the business panel.
package usecase

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/model"
	customersusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/phone"
)

// otpPurpose separates the code hashes from every other keyed hash of the
// field cipher.
const otpPurpose = "customer.panel_otp"

// linkSource is how a panel sign-in proves the phone (customers.LinkSources).
const linkSource = "panel_login"

var (
	// ErrInvalidCode covers every failed verification: unknown phone, no
	// open code, wrong, expired, used or locked code. One answer for all.
	ErrInvalidCode = errors.New("the code is invalid or expired")
	// ErrUnauthorized: missing, invalid, expired or revoked session.
	ErrUnauthorized = errors.New("customer session is not valid")
	// ErrNotFound: no record of the session's identity with the id.
	ErrNotFound = errors.New("not found")
)

// Detail is one field-level validation problem.
type Detail struct {
	Field   string
	Message string
	Code    string
}

// ValidationError carries field details; handlers answer 400.
type ValidationError struct {
	Details []Detail
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Details) == 0 {
		return "validation failed"
	}
	return e.Details[0].Field + ": " + e.Details[0].Message
}

func invalid(field, code, msg string) error {
	return &ValidationError{Details: []Detail{{Field: field, Code: code, Message: msg}}}
}

// Config holds the sign-in and session rules.
type Config struct {
	// CodeTTL is how long a code works; MaxAttempts how many tries it gets.
	CodeTTL     time.Duration
	MaxAttempts int
	// ResendCooldown is the wait between two codes of a phone; CodesPerHour
	// caps the codes of a phone (both counted in the database, so they hold
	// without Redis).
	ResendCooldown time.Duration
	CodesPerHour   int
	// SessionIdle is the sliding idle expiry, SessionMax the hard cap.
	SessionIdle time.Duration
	SessionMax  time.Duration
	// PanelURL is the panel address sent after an online booking.
	PanelURL string
}

// DefaultConfig are the production rules.
var DefaultConfig = Config{
	CodeTTL:        5 * time.Minute,
	MaxAttempts:    5,
	ResendCooldown: 60 * time.Second,
	CodesPerHour:   5,
	SessionIdle:    7 * 24 * time.Hour,
	SessionMax:     30 * 24 * time.Hour,
	PanelURL:       "https://takwim.link/panel",
}

// TokenSigner computes keyed hashes (crypto.FieldCipher.BlindIndex).
type TokenSigner interface {
	BlindIndex(purpose, scope, normalized string) string
}

// Principal is the signed-in person of a request.
type Principal struct {
	SessionID    int64
	SessionUUID  uuid.UUID
	IdentityID   int64
	IdentityUUID uuid.UUID
	Phone        string
	VerifiedAt   time.Time
	ExpiresAt    time.Time
}

// Meta is the client of a sign-in (audit fields of the session).
type Meta struct {
	IP        string
	UserAgent string
}

// Service implements the customer panel.
type Service struct {
	q         *db.Queries
	tokens    *jwt.Manager
	signer    TokenSigner
	sender    sms.Sender
	customers *customersusecase.Service
	deps      PanelDeps
	cfg       Config
	log       *slog.Logger
	now       func() time.Time
}

// New builds the service. sender may be nil (codes are then never sent and
// sign-in answers like an unknown phone).
func New(q *db.Queries, tokens *jwt.Manager, signer TokenSigner, sender sms.Sender, customers *customersusecase.Service, deps PanelDeps, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		q: q, tokens: tokens, signer: signer, sender: sender, customers: customers,
		deps: deps, cfg: DefaultConfig, log: log, now: time.Now,
	}
}

// SetConfig replaces the rules (tests, environment).
func (s *Service) SetConfig(c Config) {
	s.cfg = c
}

// SetClock replaces time.Now (tests).
func (s *Service) SetClock(now func() time.Time) {
	s.now = now
}

// NormalizePhone returns the E.164 form of raw, or "".
func NormalizePhone(raw string) string {
	p, err := phone.Normalize(raw, "")
	if err != nil {
		return ""
	}
	return p
}

func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// Start sends a sign-in code to the phone when a business knows it. The
// answer is the same for every valid phone, whether a code went out, a
// cooldown or the hourly cap applied, or the phone is unknown.
func (s *Service) Start(ctx context.Context, in model.StartInput, meta Meta) (model.StartResult, error) {
	p := NormalizePhone(in.Phone)
	if p == "" {
		return model.StartResult{}, invalid("phone", "invalid", "a valid phone number is required")
	}
	now := s.now()
	out := model.StartResult{
		Status: model.StartStatus, PhoneMasked: sms.Mask(p),
		ResendAfterSeconds: int(s.cfg.ResendCooldown.Seconds()), CodeTTLSeconds: int(s.cfg.CodeTTL.Seconds()),
	}
	latest, err := s.q.GetLatestCustomerOTPChallenge(ctx, p)
	switch {
	case err == nil:
		if wait := latest.CreatedAt.Time.Add(s.cfg.ResendCooldown).Sub(now); wait > 0 {
			out.ResendAfterSeconds = int(wait.Seconds()) + 1
			return out, nil
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return model.StartResult{}, err
	}
	n, err := s.q.CountCustomerOTPChallengesSince(ctx, db.CountCustomerOTPChallengesSinceParams{PhoneE164: p, Since: ts(now.Add(-time.Hour))})
	if err != nil {
		return model.StartResult{}, err
	}
	if n >= int64(s.cfg.CodesPerHour) {
		return out, nil
	}
	known, err := s.q.CustomerPhoneKnown(ctx, pgtype.Text{String: p, Valid: true})
	if err != nil {
		return model.StartResult{}, err
	}
	code, err := newCode()
	if err != nil {
		return model.StartResult{}, err
	}
	id := uuid.New()
	if err := s.q.CloseOpenCustomerOTPChallenges(ctx, db.CloseOpenCustomerOTPChallengesParams{Now: ts(now), PhoneE164: p}); err != nil {
		return model.StartResult{}, err
	}
	// A phone no business knows gets a row too (same cooldown and cap as a
	// known one) but no message: its code never verifies.
	send := known && s.sender != nil
	if _, err := s.q.CreateCustomerOTPChallenge(ctx, db.CreateCustomerOTPChallengeParams{
		Uuid: id, PhoneE164: p, CodeHash: s.codeHash(id, code), Sent: send,
		MaxAttempts: int32(s.cfg.MaxAttempts), ExpiresAt: ts(now.Add(s.cfg.CodeTTL)), Ip: truncate(meta.IP, 64),
		CreatedAt: ts(now),
	}); err != nil {
		return model.StartResult{}, err
	}
	// Codes older than a day serve neither cooldowns nor caps: the table
	// keeps one day of rows.
	if _, err := s.CleanupChallenges(ctx); err != nil {
		s.log.WarnContext(ctx, "customer_panel_code_cleanup_failed", "error", err)
	}
	if send {
		text := fmt.Sprintf("Takwim giriş kodunuz: %s. Kod %d dakika geçerlidir; kimseyle paylaşmayın.", code, int(s.cfg.CodeTTL.Minutes()))
		if err := s.sender.Send(ctx, sms.Message{To: p, Text: text, Purpose: sms.PurposeCustomerOTP}); err != nil {
			s.log.WarnContext(ctx, "customer_panel_code_not_sent", "to", sms.Mask(p), "error", err)
		}
	}
	return out, nil
}

func (s *Service) codeHash(challenge uuid.UUID, code string) string {
	return s.signer.BlindIndex(otpPurpose, challenge.String(), code)
}

// newCode returns six random digits.
func newCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func validCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Verify exchanges the phone's open code for a panel session. The verified
// phone links the person's records in every business (customers
// LinkVerified, source panel_login); the session token names the session
// row.
func (s *Service) Verify(ctx context.Context, in model.VerifyInput, meta Meta) (model.Session, error) {
	p := NormalizePhone(in.Phone)
	if p == "" {
		return model.Session{}, invalid("phone", "invalid", "a valid phone number is required")
	}
	code := strings.TrimSpace(in.Code)
	if !validCode(code) {
		return model.Session{}, ErrInvalidCode
	}
	now := s.now()
	ch, err := s.q.GetOpenCustomerOTPChallenge(ctx, db.GetOpenCustomerOTPChallengeParams{PhoneE164: p, Now: ts(now)})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Session{}, ErrInvalidCode
	}
	if err != nil {
		return model.Session{}, err
	}
	att, err := s.q.CountCustomerOTPAttempt(ctx, ch.ID)
	if err != nil {
		return model.Session{}, err
	}
	match := subtle.ConstantTimeCompare([]byte(s.codeHash(ch.Uuid, code)), []byte(ch.CodeHash)) == 1
	if att.Attempts > att.MaxAttempts || !ch.Sent || !match {
		return model.Session{}, ErrInvalidCode
	}
	if n, err := s.q.ConsumeCustomerOTPChallenge(ctx, db.ConsumeCustomerOTPChallengeParams{Now: ts(now), ID: ch.ID}); err != nil {
		return model.Session{}, err
	} else if n == 0 {
		return model.Session{}, ErrInvalidCode
	}
	identity, err := s.q.UpsertCustomerIdentity(ctx, db.UpsertCustomerIdentityParams{PhoneE164: p, VerifiedAt: ts(now)})
	if err != nil {
		return model.Session{}, err
	}
	s.linkRecords(ctx, p, now)
	row, err := s.q.CreateCustomerSession(ctx, db.CreateCustomerSessionParams{
		IdentityID: identity.ID, PhoneE164: p, VerifiedAt: ts(now), Ip: truncate(meta.IP, 64),
		UserAgent: truncate(meta.UserAgent, 512), IdleExpiresAt: ts(now.Add(s.cfg.SessionIdle)), ExpiresAt: ts(now.Add(s.cfg.SessionMax)),
	})
	if err != nil {
		return model.Session{}, err
	}
	token, err := s.tokens.IssueCustomer(jwt.CustomerInput{IdentityID: identity.Uuid, SessionID: row.Uuid, ExpiresAt: row.ExpiresAt.Time})
	if err != nil {
		return model.Session{}, err
	}
	principal := Principal{
		SessionID: row.ID, SessionUUID: row.Uuid, IdentityID: identity.ID, IdentityUUID: identity.Uuid,
		Phone: p, VerifiedAt: now, ExpiresAt: row.ExpiresAt.Time,
	}
	profile, err := s.profile(ctx, principal)
	if err != nil {
		return model.Session{}, err
	}
	return model.Session{AccessToken: token, TokenType: "Bearer", ExpiresAt: row.ExpiresAt.Time, Profile: profile}, nil
}

// linkRecords links every live, unlinked record with the proven phone to
// the person's identity. A business whose identity link already sits on an
// older (deleted) record keeps it (customers.md: links are not reused).
func (s *Service) linkRecords(ctx context.Context, p string, verifiedAt time.Time) {
	rows, err := s.q.ListUnlinkedCustomersByPhone(ctx, pgtype.Text{String: p, Valid: true})
	if err != nil {
		s.log.WarnContext(ctx, "customer_panel_link_failed", "error", err)
		return
	}
	for _, r := range rows {
		_, err := s.customers.LinkVerified(ctx, r.OrganizationID, r.Uuid, p, linkSource, verifiedAt)
		if err != nil && !errors.Is(err, customersusecase.ErrIdentityLinked) {
			s.log.WarnContext(ctx, "customer_panel_link_failed", "error", err)
		}
	}
}

// Authenticate resolves a customer token to its live session. The session
// slides its idle expiry (at most once a minute) inside the hard cap.
func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	claims, err := s.tokens.ParseCustomer(token)
	if err != nil {
		return Principal{}, ErrUnauthorized
	}
	row, err := s.q.GetCustomerSession(ctx, claims.SessionUUID())
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrUnauthorized
	}
	if err != nil {
		return Principal{}, err
	}
	now := s.now()
	if row.IdentityUuid.String() != claims.Subject || row.RevokedAt.Valid ||
		!row.ExpiresAt.Time.After(now) || !row.IdleExpiresAt.Time.After(now) {
		return Principal{}, ErrUnauthorized
	}
	if now.Sub(row.LastSeenAt.Time) >= time.Minute {
		idle := now.Add(s.cfg.SessionIdle)
		if idle.After(row.ExpiresAt.Time) {
			idle = row.ExpiresAt.Time
		}
		if err := s.q.TouchCustomerSession(ctx, db.TouchCustomerSessionParams{Now: ts(now), IdleExpiresAt: ts(idle), ID: row.ID}); err != nil {
			s.log.WarnContext(ctx, "customer_session_touch_failed", "error", err)
		}
	}
	return Principal{
		SessionID: row.ID, SessionUUID: row.Uuid, IdentityID: row.IdentityID, IdentityUUID: row.IdentityUuid,
		Phone: row.PhoneE164, VerifiedAt: row.VerifiedAt.Time, ExpiresAt: row.ExpiresAt.Time,
	}, nil
}

// Logout ends the session; everywhere ends every session of the person.
func (s *Service) Logout(ctx context.Context, p Principal, everywhere bool) error {
	now := ts(s.now())
	if everywhere {
		_, err := s.q.RevokeCustomerSessionsOfIdentity(ctx, db.RevokeCustomerSessionsOfIdentityParams{Now: now, IdentityID: p.IdentityID})
		return err
	}
	_, err := s.q.RevokeCustomerSession(ctx, db.RevokeCustomerSessionParams{Now: now, ID: p.SessionID})
	return err
}

// Profile is the signed-in person: phone, linked businesses, sessions.
func (s *Service) Profile(ctx context.Context, p Principal) (model.Profile, error) {
	s.linkRecords(ctx, p.Phone, p.VerifiedAt)
	return s.profile(ctx, p)
}

func (s *Service) profile(ctx context.Context, p Principal) (model.Profile, error) {
	rows, err := s.q.ListCustomerPanelBusinesses(ctx, p.IdentityID)
	if err != nil {
		return model.Profile{}, err
	}
	active, err := s.q.CountActiveCustomerSessions(ctx, db.CountActiveCustomerSessionsParams{IdentityID: p.IdentityID, Now: ts(s.now())})
	if err != nil {
		return model.Profile{}, err
	}
	out := model.Profile{
		Phone: p.Phone, PhoneDisplay: phone.Display(p.Phone, ""), VerifiedAt: p.VerifiedAt,
		SessionExpiresAt: p.ExpiresAt, ActiveSessions: active, Businesses: make([]model.Business, 0, len(rows)),
	}
	for _, r := range rows {
		out.Businesses = append(out.Businesses, model.Business{
			UUID: r.Uuid, Slug: r.Slug, Name: r.Name, FirstName: r.FirstName, LastName: r.LastName,
			LinkedAt: r.VerifiedAt.Time, ReservationCount: r.ReservationCount,
		})
	}
	return out, nil
}

// PanelLinkNotice sends the panel address to the phone of a new online
// booking (requirements §14A step 2). It needs no confirmation module and
// opens nothing by itself: the panel still asks for a code.
func (s *Service) PanelLinkNotice(ctx context.Context, phoneE164, businessName string) {
	if s.sender == nil || phoneE164 == "" {
		return
	}
	text := fmt.Sprintf("%s randevunuz alındı. Tüm randevularınızı görmek ve yönetmek için: %s", businessName, s.cfg.PanelURL)
	if err := s.sender.Send(ctx, sms.Message{To: phoneE164, Text: text, Purpose: sms.PurposeCustomerPanelLink}); err != nil {
		s.log.WarnContext(ctx, "customer_panel_link_not_sent", "to", sms.Mask(phoneE164), "error", err)
	}
}

// CleanupChallenges deletes codes older than a day (run on every code
// request).
func (s *Service) CleanupChallenges(ctx context.Context) (int64, error) {
	return s.q.DeleteStaleCustomerOTPChallenges(ctx, ts(s.now().Add(-24*time.Hour)))
}

func truncate(v string, n int) string {
	v = strings.TrimSpace(v)
	if len(v) <= n {
		return v
	}
	for n > 0 && !utf8.RuneStart(v[n]) {
		n--
	}
	return v[:n]
}
