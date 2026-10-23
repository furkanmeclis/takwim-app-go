package usecase

// Access security of the customer panel (TEC-910): single-use panel links,
// the device list, number changes, reassigned numbers and the abuse trail.
// See docs/design/customer-panel.md, "Access security".

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
)

// logSource groups the security warnings in the platform logs.
const logSource = "customer_auth"

// Keyed hash purposes of the link tokens: a panel link and a visit link
// never hash alike, and they live in different tables.
const (
	panelLinkPurpose = "customer.panel_link"
	visitLinkPurpose = "customer.visit_link"
)

// Kinds of customer_auth_events.
const (
	eventSignedIn           = "signed_in"
	eventCodeFailed         = "code_failed"
	eventLinkFailed         = "link_failed"
	eventVisitLinkFailed    = "visit_link_failed"
	eventIPThrottled        = "ip_throttled"
	eventPhoneChanged       = "phone_changed"
	eventPhoneChangeRefused = "phone_change_refused"
	eventReassigned         = "reassigned"
	eventSessionRevoked     = "session_revoked"
)

// Reasons of customer_phone_releases.
const (
	releaseNumberChanged = "number_changed"
	releaseReassigned    = "reassigned"
)

var (
	// ErrLinkInvalid: an unknown, expired, used or revoked panel or visit
	// link. One answer for all; the page offers a new sign-in.
	ErrLinkInvalid = errors.New("the link is invalid or expired")
	// ErrPhoneInUse: the new number has its own Takwim profile. Two profiles
	// are never merged on a number alone.
	ErrPhoneInUse = errors.New("the number has its own profile")
)

// tokenBytes is the entropy of a link token (192 bits, 32 characters).
const tokenBytes = 24

func newToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// validToken accepts only the shape newToken makes (no lookup otherwise).
func validToken(t string) bool {
	if len(t) != base64.RawURLEncoding.EncodedLen(tokenBytes) {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(t)
	return err == nil
}

func (s *Service) tokenHash(purpose, token string) string {
	return s.signer.BlindIndex(purpose, "v1", token)
}

// event records one row of the abuse trail; a failure to record never
// fails the request.
func (s *Service) event(ctx context.Context, kind, p string, identityID int64, ip string) {
	params := db.CreateCustomerAuthEventParams{Kind: kind, Ip: truncate(ip, 64), CreatedAt: ts(s.now())}
	if p != "" {
		params.PhoneE164 = pgtype.Text{String: p, Valid: true}
	}
	if identityID != 0 {
		params.IdentityID = pgtype.Int8{Int64: identityID, Valid: true}
	}
	if err := s.q.CreateCustomerAuthEvent(ctx, params); err != nil {
		s.log.WarnContext(ctx, "customer_auth_event_failed", "source", logSource, "kind", kind, "error", err)
	}
}

// failure records a failed code or link and writes one abuse warning when
// an address reaches FailureAlert failures of that kind in an hour.
func (s *Service) failure(ctx context.Context, kind, p string, identityID int64, ip string) {
	s.event(ctx, kind, p, identityID, ip)
	ip = truncate(ip, 64)
	if ip == "" || s.cfg.FailureAlert <= 0 {
		return
	}
	n, err := s.q.CountCustomerAuthEventsFromIPSince(ctx, db.CountCustomerAuthEventsFromIPSinceParams{
		Ip: ip, Kind: kind, Since: ts(s.now().Add(-time.Hour)),
	})
	if err == nil && n == int64(s.cfg.FailureAlert) {
		s.log.WarnContext(ctx, "customer_auth_failures", "source", logSource, "kind", kind, "ip", ip, "count", n)
	}
}

// issuePanelLink creates a single-use panel link for the phone; "" when the
// phone's daily link cap is reached.
func (s *Service) issuePanelLink(ctx context.Context, p string) (string, error) {
	now := s.now()
	n, err := s.q.CountCustomerAccessLinksSince(ctx, db.CountCustomerAccessLinksSinceParams{PhoneE164: p, Since: ts(now.Add(-24 * time.Hour))})
	if err != nil {
		return "", err
	}
	if n >= int64(s.cfg.LinksPerDay) {
		return "", nil
	}
	token, err := newToken()
	if err != nil {
		return "", err
	}
	if _, err := s.q.CreateCustomerAccessLink(ctx, db.CreateCustomerAccessLinkParams{
		TokenHash: s.tokenHash(panelLinkPurpose, token), PhoneE164: p,
		ExpiresAt: ts(now.Add(s.cfg.LinkTTL)), CreatedAt: ts(now),
	}); err != nil {
		return "", err
	}
	return strings.TrimRight(s.cfg.PanelURL, "/") + "/l#" + token, nil
}

// RedeemLink exchanges a panel link for a panel session. The link works
// once and only within its lifetime; anything else is ErrLinkInvalid and
// the person signs in with a code instead.
func (s *Service) RedeemLink(ctx context.Context, in model.LinkInput, meta Meta) (model.Session, error) {
	token := strings.TrimSpace(in.Token)
	if !validToken(token) {
		s.failure(ctx, eventLinkFailed, "", 0, meta.IP)
		return model.Session{}, ErrLinkInvalid
	}
	p, err := s.q.ConsumeCustomerAccessLink(ctx, db.ConsumeCustomerAccessLinkParams{
		Now: ts(s.now()), TokenHash: s.tokenHash(panelLinkPurpose, token),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		s.failure(ctx, eventLinkFailed, "", 0, meta.IP)
		return model.Session{}, ErrLinkInvalid
	}
	if err != nil {
		return model.Session{}, err
	}
	return s.openSession(ctx, p, model.MethodLink, meta)
}

// Sessions lists the person's live panel sessions (devices).
func (s *Service) Sessions(ctx context.Context, p Principal) (model.DeviceList, error) {
	rows, err := s.q.ListActiveCustomerSessions(ctx, db.ListActiveCustomerSessionsParams{IdentityID: p.IdentityID, Now: ts(s.now())})
	if err != nil {
		return model.DeviceList{}, err
	}
	out := model.DeviceList{Items: make([]model.Device, 0, len(rows))}
	for _, r := range rows {
		expires := r.ExpiresAt.Time
		if r.IdleExpiresAt.Time.Before(expires) {
			expires = r.IdleExpiresAt.Time
		}
		out.Items = append(out.Items, model.Device{
			UUID: r.Uuid, Current: r.Uuid == p.SessionUUID, Method: r.Method, Device: DeviceLabel(r.UserAgent),
			IP: MaskIP(r.Ip), CreatedAt: r.CreatedAt.Time, LastSeenAt: r.LastSeenAt.Time, ExpiresAt: expires,
		})
	}
	return out, nil
}

// RevokeSession signs one device of the person out (ErrNotFound for any
// other session).
func (s *Service) RevokeSession(ctx context.Context, p Principal, id uuid.UUID, meta Meta) error {
	n, err := s.q.RevokeCustomerSessionOfIdentity(ctx, db.RevokeCustomerSessionOfIdentityParams{
		Now: ts(s.now()), Uuid: id, IdentityID: p.IdentityID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	s.event(ctx, eventSessionRevoked, p.Phone, p.IdentityID, meta.IP)
	return nil
}

// StartPhoneChange sends a code to the new number of the signed-in person.
// The code is bound to the person's identity and purpose; the usual
// cooldown and caps of the target number apply, plus a daily cap per
// person.
func (s *Service) StartPhoneChange(ctx context.Context, p Principal, in model.PhoneStartInput, meta Meta) (model.StartResult, error) {
	np := NormalizePhone(in.Phone)
	if np == "" {
		return model.StartResult{}, invalid("phone", "invalid", "a valid phone number is required")
	}
	if np == p.Phone {
		return model.StartResult{}, invalid("phone", "same", "this is already the profile's number")
	}
	n, err := s.q.CountCustomerOTPChallengesOfIdentitySince(ctx, db.CountCustomerOTPChallengesOfIdentitySinceParams{
		IdentityID: pgtype.Int8{Int64: p.IdentityID, Valid: true}, Since: ts(s.now().Add(-24 * time.Hour)),
	})
	if err != nil {
		return model.StartResult{}, err
	}
	deliver := n < int64(s.cfg.PhoneChangesPerDay)
	id := p.IdentityID
	return s.sendCode(ctx, np, purposePhoneChange, &id, deliver, meta)
}

// VerifyPhoneChange moves the person's profile to the new number once its
// code is confirmed. The identity (and every visit it sees) stays; the old
// number is released, so its next holder starts empty; the links proven
// with the old number stop showing later bookings made with it; every other
// session ends. A number that has its own profile is refused
// (ErrPhoneInUse): profiles are never merged on a number.
func (s *Service) VerifyPhoneChange(ctx context.Context, p Principal, in model.VerifyInput, meta Meta) (model.Profile, error) {
	np := NormalizePhone(in.Phone)
	if np == "" {
		return model.Profile{}, invalid("phone", "invalid", "a valid phone number is required")
	}
	id := p.IdentityID
	if err := s.checkCode(ctx, np, purposePhoneChange, &id, in.Code, meta); err != nil {
		return model.Profile{}, err
	}
	now := s.now()
	err := s.inTx(ctx, func(q *db.Queries) error {
		other, err := q.GetCustomerIdentityByPhone(ctx, np)
		switch {
		case err == nil && other.ID != p.IdentityID:
			return ErrPhoneInUse
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		n, err := q.SetCustomerIdentityPhone(ctx, db.SetCustomerIdentityPhoneParams{
			PhoneE164: np, VerifiedAt: ts(now), ID: p.IdentityID, OldPhoneE164: p.Phone,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrUnauthorized
		}
		if err := q.CreateCustomerPhoneRelease(ctx, db.CreateCustomerPhoneReleaseParams{
			PhoneE164: p.Phone, IdentityID: pgtype.Int8{Int64: p.IdentityID, Valid: true},
			Reason: releaseNumberChanged,
		}); err != nil {
			return err
		}
		if _, err := q.CloseCustomerIdentityLinksOfPhone(ctx, db.CloseCustomerIdentityLinksOfPhoneParams{
			IdentityID: p.IdentityID, PhoneE164: p.Phone,
		}); err != nil {
			return err
		}
		if err := q.SetCustomerSessionPhone(ctx, db.SetCustomerSessionPhoneParams{PhoneE164: np, VerifiedAt: ts(now), ID: p.SessionID}); err != nil {
			return err
		}
		_, err = q.RevokeOtherCustomerSessions(ctx, db.RevokeOtherCustomerSessionsParams{Now: ts(now), IdentityID: p.IdentityID, KeepID: p.SessionID})
		return err
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		err = ErrPhoneInUse
	}
	if errors.Is(err, ErrPhoneInUse) {
		s.event(ctx, eventPhoneChangeRefused, np, p.IdentityID, meta.IP)
		return model.Profile{}, err
	}
	if err != nil {
		return model.Profile{}, err
	}
	s.event(ctx, eventPhoneChanged, np, p.IdentityID, meta.IP)
	s.log.InfoContext(ctx, "customer_phone_changed", "source", logSource, "from", sms.Mask(p.Phone), "to", sms.Mask(np))
	p.Phone, p.VerifiedAt = np, now
	s.linkRecords(ctx, np, now, sourcePhoneChange)
	return s.profile(ctx, p)
}

// ReportReassigned processes the person's report that the signed-in number
// was recently given to them: the earlier history of every business they do
// not keep is hidden from the profile (it belongs to the previous holder),
// records of the number found later open only what is booked from now on,
// and every other session of the profile (the previous holder's devices)
// ends.
func (s *Service) ReportReassigned(ctx context.Context, p Principal, in model.ReassignedInput, meta Meta) (model.Profile, error) {
	businesses, err := s.q.ListCustomerPanelBusinesses(ctx, p.IdentityID)
	if err != nil {
		return model.Profile{}, err
	}
	keep := make([]uuid.UUID, 0, len(in.KeepBusinesses))
	for _, id := range in.KeepBusinesses {
		if !slices.ContainsFunc(businesses, func(b db.ListCustomerPanelBusinessesRow) bool { return b.Uuid == id }) {
			return model.Profile{}, invalid("keep_businesses", "unknown", "not a business of this profile")
		}
		keep = append(keep, id)
	}
	now := s.now()
	err = s.inTx(ctx, func(q *db.Queries) error {
		if _, err := q.HideCustomerIdentityHistory(ctx, db.HideCustomerIdentityHistoryParams{IdentityID: p.IdentityID, Keep: keep}); err != nil {
			return err
		}
		if err := q.CreateCustomerPhoneRelease(ctx, db.CreateCustomerPhoneReleaseParams{
			PhoneE164: p.Phone, IdentityID: pgtype.Int8{Int64: p.IdentityID, Valid: true},
			Reason: releaseReassigned,
		}); err != nil {
			return err
		}
		_, err := q.RevokeOtherCustomerSessions(ctx, db.RevokeOtherCustomerSessionsParams{Now: ts(now), IdentityID: p.IdentityID, KeepID: p.SessionID})
		return err
	})
	if err != nil {
		return model.Profile{}, err
	}
	s.event(ctx, eventReassigned, p.Phone, p.IdentityID, meta.IP)
	s.log.WarnContext(ctx, "customer_number_reassigned", "source", logSource, "phone", sms.Mask(p.Phone), "kept", len(keep))
	return s.profile(ctx, p)
}

// MaskIP hides the host part of an address for the device list: the first
// two IPv4 octets, the first two IPv6 groups.
func MaskIP(raw string) string {
	ip := net.ParseIP(strings.TrimSpace(raw))
	if ip == nil {
		return ""
	}
	if v4 := ip.To4(); v4 != nil {
		parts := strings.Split(v4.String(), ".")
		return parts[0] + "." + parts[1] + ".*.*"
	}
	parts := strings.Split(ip.String(), ":")
	if len(parts) < 2 {
		return "*"
	}
	return parts[0] + ":" + parts[1] + ":*"
}

// DeviceLabel names a device from its user agent ("Chrome · Android");
// "" when nothing is recognised. Mobile apps send their own name first.
func DeviceLabel(ua string) string {
	if ua == "" {
		return ""
	}
	browser := ""
	for _, b := range []struct{ token, name string }{
		{"TakwimApp", "Takwim"}, {"Edg/", "Edge"}, {"OPR/", "Opera"}, {"SamsungBrowser", "Samsung Internet"},
		{"YaBrowser", "Yandex"}, {"Firefox/", "Firefox"}, {"FxiOS", "Firefox"}, {"CriOS", "Chrome"},
		{"Chrome/", "Chrome"}, {"Safari/", "Safari"},
	} {
		if strings.Contains(ua, b.token) {
			browser = b.name
			break
		}
	}
	os := ""
	for _, o := range []struct{ token, name string }{
		{"iPhone", "iPhone"}, {"iPad", "iPad"}, {"Android", "Android"}, {"Windows", "Windows"},
		{"Mac OS X", "macOS"}, {"CrOS", "ChromeOS"}, {"Linux", "Linux"},
	} {
		if strings.Contains(ua, o.token) {
			os = o.name
			break
		}
	}
	switch {
	case browser != "" && os != "":
		return browser + " · " + os
	case browser != "":
		return browser
	default:
		return os
	}
}
