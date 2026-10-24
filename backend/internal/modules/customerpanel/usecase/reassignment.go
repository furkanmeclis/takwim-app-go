package usecase

// Numbers that changed hands without the person reporting it (TEC-972):
// reassignment notices processed from Takwim management, and the ownership
// check of numbers nobody proved for a long time. See
// docs/design/customer-panel.md, "Reassignments from Takwim management".

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
)

// ErrAlreadyReleased: nobody proved the number since its last release, so a
// new notice would close nothing.
var ErrAlreadyReleased = errors.New("the number was released and nobody proved it since")

// noteMax is the longest operator note of a release.
const noteMax = 500

// ProcessReassignment handles a reassignment notice of a number from Takwim
// management: the same release as the person's own report (reassigned),
// without anything kept. Every business's earlier history of the number's
// profile is hidden (visible_from = now), every session of the profile ends
// (nobody keeps a device, the operator is not the holder), and records of
// the number found later open only what is booked from now on. A number
// without a profile gets the release too, so its records never open older
// history. actorID is the platform user.
func (s *Service) ProcessReassignment(ctx context.Context, in model.ReassignmentInput, actorID int64) (model.PhoneRelease, error) {
	p := NormalizePhone(in.Phone)
	if p == "" {
		return model.PhoneRelease{}, invalid("phone", "invalid", "a valid phone number is required")
	}
	note := strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(note) > noteMax {
		return model.PhoneRelease{}, invalid("note", "too_long", "the note is too long")
	}
	now := s.now()
	var row db.CustomerPhoneRelease
	err := s.inTx(ctx, func(q *db.Queries) error {
		var identity pgtype.Int8
		var hidden, revoked int64
		found, err := q.GetCustomerIdentityByPhone(ctx, p)
		switch {
		case err == nil:
			identity = pgtype.Int8{Int64: found.ID, Valid: true}
			if released, err := q.LatestCustomerPhoneRelease(ctx, p); err == nil && !released.Time.Before(found.PhoneVerifiedAt.Time) {
				return ErrAlreadyReleased
			} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if hidden, err = q.HideCustomerIdentityHistory(ctx, db.HideCustomerIdentityHistoryParams{IdentityID: found.ID, Keep: keepNone()}); err != nil {
				return err
			}
			if revoked, err = q.RevokeCustomerSessionsOfIdentity(ctx, db.RevokeCustomerSessionsOfIdentityParams{Now: ts(now), IdentityID: found.ID}); err != nil {
				return err
			}
		case errors.Is(err, pgx.ErrNoRows):
			// No profile: a second notice of the same number adds nothing.
			if _, err := q.LatestCustomerPhoneRelease(ctx, p); err == nil {
				return ErrAlreadyReleased
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		default:
			return err
		}
		var by pgtype.Int8
		if actorID != 0 {
			by = pgtype.Int8{Int64: actorID, Valid: true}
		}
		row, err = q.CreateCustomerPhoneRelease(ctx, db.CreateCustomerPhoneReleaseParams{
			PhoneE164: p, IdentityID: identity, Reason: releaseReassigned, Source: model.ReleaseSourcePlatform,
			Note: note, CreatedBy: by, BusinessesHidden: clampInt32(hidden), SessionsRevoked: clampInt32(revoked),
		})
		return err
	})
	if err != nil {
		return model.PhoneRelease{}, err
	}
	var identityID int64
	if row.IdentityID.Valid {
		identityID = row.IdentityID.Int64
	}
	s.event(ctx, eventPlatformReassigned, p, identityID, "")
	s.log.WarnContext(ctx, "customer_number_reassigned_by_platform", "source", logSource, "phone", sms.Mask(p),
		"profile", row.IdentityID.Valid, "businesses_hidden", row.BusinessesHidden, "sessions_revoked", row.SessionsRevoked)
	return releaseFromRow(row), nil
}

// ListReleases is the platform list of numbers that changed hands.
func (s *Service) ListReleases(ctx context.Context, f model.ReleaseFilter) ([]model.PhoneRelease, int64, error) {
	for _, v := range f.Reasons {
		if !slices.Contains(model.ReleaseReasons, v) {
			return nil, 0, invalid("reason", "invalid", "unknown reason")
		}
	}
	for _, v := range f.Sources {
		if !slices.Contains(model.ReleaseSources, v) {
			return nil, 0, invalid("source", "invalid", "unknown source")
		}
	}
	// NULL (no filter) for an empty list.
	orNil := func(v []string) []string {
		if len(v) == 0 {
			return nil
		}
		return v
	}
	var phone, note pgtype.Text
	if q := strings.TrimSpace(f.Q); q != "" {
		if p := NormalizePhone(q); p != "" {
			phone = pgtype.Text{String: p, Valid: true}
		} else {
			note = pgtype.Text{String: escapeLike(q), Valid: true}
		}
	}
	rows, err := s.q.ListCustomerPhoneReleases(ctx, db.ListCustomerPhoneReleasesParams{
		Phone: phone, Note: note, Reasons: orNil(f.Reasons), Sources: orNil(f.Sources),
		OldestFirst: f.OldestFirst, RowLimit: f.Limit, RowOffset: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountCustomerPhoneReleases(ctx, db.CountCustomerPhoneReleasesParams{
		Phone: phone, Note: note, Reasons: orNil(f.Reasons), Sources: orNil(f.Sources),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.PhoneRelease, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.PhoneRelease{
			UUID: r.Uuid, PhoneMasked: sms.Mask(r.PhoneE164), Reason: r.Reason, Source: r.Source, Note: r.Note,
			CreatedByName: r.CreatedByName, HasProfile: r.HasProfile, BusinessesHidden: int(r.BusinessesHidden),
			SessionsRevoked: int(r.SessionsRevoked), ReleasedAt: r.ReleasedAt.Time,
		})
	}
	return out, total, nil
}

// closeDormant is the ownership check of a number nobody proved for
// DormantAfter (TEC-972): a recycled number is often given out unreported,
// and its next holder must not open the previous holder's visits by
// signing in. Before the sign-in of such a number goes on, the profile's
// earlier history is closed (visible_from = now) with a dormant release
// (records found later open only new bookings too) and its stale sessions
// end. The person sees every visit booked from now on; the businesses keep
// every record. A number released after its last proof (a change, a report
// or a notice already closed it) is left alone. A failure fails the
// sign-in, so history never opens unchecked.
func (s *Service) closeDormant(ctx context.Context, p string, now time.Time) error {
	if s.cfg.DormantAfter <= 0 {
		return nil
	}
	identity, err := s.q.GetCustomerIdentityByPhone(ctx, p)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if identity.PhoneVerifiedAt.Time.After(now.Add(-s.cfg.DormantAfter)) {
		return nil
	}
	switch released, err := s.q.LatestCustomerPhoneRelease(ctx, p); {
	case err == nil && !released.Time.Before(identity.PhoneVerifiedAt.Time):
		return nil
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		return err
	}
	var hidden, revoked int64
	err = s.inTx(ctx, func(q *db.Queries) error {
		var err error
		if hidden, err = q.HideCustomerIdentityHistory(ctx, db.HideCustomerIdentityHistoryParams{IdentityID: identity.ID, Keep: keepNone()}); err != nil {
			return err
		}
		if revoked, err = q.RevokeCustomerSessionsOfIdentity(ctx, db.RevokeCustomerSessionsOfIdentityParams{Now: ts(now), IdentityID: identity.ID}); err != nil {
			return err
		}
		_, err = q.CreateCustomerPhoneRelease(ctx, db.CreateCustomerPhoneReleaseParams{
			PhoneE164: p, IdentityID: pgtype.Int8{Int64: identity.ID, Valid: true}, Reason: releaseDormant,
			Source: model.ReleaseSourceSystem, BusinessesHidden: clampInt32(hidden), SessionsRevoked: clampInt32(revoked),
		})
		return err
	})
	if err != nil {
		return err
	}
	s.event(ctx, eventDormantClosed, p, identity.ID, "")
	s.log.WarnContext(ctx, "customer_dormant_history_closed", "source", logSource, "phone", sms.Mask(p),
		"last_verified_at", identity.PhoneVerifiedAt.Time, "businesses_hidden", hidden)
	return nil
}

// HistoryRequest is a history request the person made to one business:
// the business (for its activity trail) and the record it concerns.
type HistoryRequest struct {
	Scope        orgctx.Scope
	CustomerUUID uuid.UUID
	RequestedAt  time.Time
}

// RequestHistory (TEC-991) records that the signed-in person asks the
// business named by slug to reopen the history a release closed
// (visible_from on the link): the receptionist sees the request on the
// record and reopens it after checking who the person is. The panel never
// reopens anything itself: the answer to "are these visits yours?" is no
// proof. A business that does not know the person is ErrNotFound; one whose
// history is open answers an ActionError (history_open). Asking again keeps
// the first request's time.
func (s *Service) RequestHistory(ctx context.Context, p Principal, slug string, meta Meta) (HistoryRequest, model.Profile, error) {
	row, err := s.q.RequestCustomerHistory(ctx, db.RequestCustomerHistoryParams{Slug: slug, IdentityID: p.IdentityID})
	if errors.Is(err, pgx.ErrNoRows) {
		return HistoryRequest{}, model.Profile{}, ErrNotFound
	}
	if err != nil {
		return HistoryRequest{}, model.Profile{}, err
	}
	if !row.VisibleFrom.Valid {
		return HistoryRequest{}, model.Profile{}, &ActionError{Action: "history_request", Reason: model.ReasonHistoryOpen}
	}
	s.event(ctx, eventHistoryRequested, p.Phone, p.IdentityID, meta.IP)
	profile, err := s.profile(ctx, p)
	if err != nil {
		return HistoryRequest{}, model.Profile{}, err
	}
	req := HistoryRequest{
		Scope:        orgctx.Scope{ID: row.OrganizationID, UUID: row.OrganizationUuid, Slug: row.OrganizationSlug, Name: row.OrganizationName},
		CustomerUUID: row.CustomerUuid,
		RequestedAt:  row.HistoryRequestedAt.Time,
	}
	return req, profile, nil
}

// historyNotice is the closing release of the profile's number, if any.
func (s *Service) historyNotice(ctx context.Context, p Principal) *model.HistoryNotice {
	row, err := s.q.LatestClosingCustomerPhoneRelease(ctx, db.LatestClosingCustomerPhoneReleaseParams{
		IdentityID: pgtype.Int8{Int64: p.IdentityID, Valid: true}, PhoneE164: p.Phone,
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			s.log.WarnContext(ctx, "customer_history_notice_failed", "error", err)
		}
		return nil
	}
	return &model.HistoryNotice{Reason: row.Reason, ClosedAt: row.ReleasedAt.Time}
}

func releaseFromRow(r db.CustomerPhoneRelease) model.PhoneRelease {
	return model.PhoneRelease{
		UUID: r.Uuid, PhoneMasked: sms.Mask(r.PhoneE164), Reason: r.Reason, Source: r.Source, Note: r.Note,
		HasProfile: r.IdentityID.Valid, BusinessesHidden: int(r.BusinessesHidden),
		SessionsRevoked: int(r.SessionsRevoked), ReleasedAt: r.ReleasedAt.Time,
	}
}

// keepNone keeps no business: an empty array, never NULL (NOT (x = ANY(NULL))
// would match nothing).
func keepNone() []uuid.UUID {
	return []uuid.UUID{}
}

func clampInt32(n int64) int32 {
	if n > 1<<31-1 {
		return 1<<31 - 1
	}
	if n < 0 {
		return 0
	}
	return int32(n)
}

// escapeLike makes a search text literal inside ILIKE '%…%'.
func escapeLike(v string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(v)
}
