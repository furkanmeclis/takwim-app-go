package usecase

// Visit links (TEC-910): a link to one reservation (takwim.link/panel/v#…)
// for confirmation and cancel messages. Its token is a separate type: it is
// hashed with its own purpose, lives in its own table, names one
// reservation, and never opens the panel, a session or another visit.

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/model"
)

// IssueVisitLink creates a link to one reservation of the business. It
// works until VisitLinkGrace after the visit ends (at least an hour). The
// confirmation module (TEC-921 / TEC-922) puts it in its messages.
func (s *Service) IssueVisitLink(ctx context.Context, orgID int64, reservation uuid.UUID) (model.VisitLink, error) {
	r, err := s.q.GetReservationForVisitLink(ctx, db.GetReservationForVisitLinkParams{OrganizationID: orgID, Uuid: reservation})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.VisitLink{}, ErrNotFound
	}
	if err != nil {
		return model.VisitLink{}, err
	}
	now := s.now()
	expires := r.EndsAt.Time.Add(s.cfg.VisitLinkGrace)
	if min := now.Add(time.Hour); expires.Before(min) {
		expires = min
	}
	token, err := newToken()
	if err != nil {
		return model.VisitLink{}, err
	}
	if _, err := s.q.CreateCustomerVisitLink(ctx, db.CreateCustomerVisitLinkParams{
		OrganizationID: orgID, ReservationID: r.ID, TokenHash: s.tokenHash(visitLinkPurpose, token),
		ExpiresAt: ts(expires), CreatedAt: ts(now),
	}); err != nil {
		return model.VisitLink{}, err
	}
	return model.VisitLink{URL: strings.TrimRight(s.cfg.PanelURL, "/") + "/v#" + token, ExpiresAt: expires}, nil
}

// IssueVisitLinkFor is IssueVisitLink by the reservation's id (the
// confirmation request puts it in its text at send time).
func (s *Service) IssueVisitLinkFor(ctx context.Context, orgID, reservationID int64) (model.VisitLink, error) {
	r, err := s.q.GetReservationForVisitLinkByID(ctx, db.GetReservationForVisitLinkByIDParams{OrganizationID: orgID, ID: reservationID})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.VisitLink{}, ErrNotFound
	}
	if err != nil {
		return model.VisitLink{}, err
	}
	return s.IssueVisitLink(ctx, orgID, r.Uuid)
}

// visitRow resolves a visit link to its reservation row; every failure is
// ErrLinkInvalid (recorded as visit_link_failed).
func (s *Service) visitRow(ctx context.Context, token string, meta Meta) (db.GetCustomerPanelReservationRow, error) {
	token = strings.TrimSpace(token)
	fail := func() (db.GetCustomerPanelReservationRow, error) {
		s.failure(ctx, eventVisitLinkFailed, "", 0, meta.IP)
		return db.GetCustomerPanelReservationRow{}, ErrLinkInvalid
	}
	if !validToken(token) {
		return fail()
	}
	link, err := s.q.UseCustomerVisitLink(ctx, db.UseCustomerVisitLinkParams{Now: ts(s.now()), TokenHash: s.tokenHash(visitLinkPurpose, token)})
	if errors.Is(err, pgx.ErrNoRows) {
		return fail()
	}
	if err != nil {
		return db.GetCustomerPanelReservationRow{}, err
	}
	return s.visitReservation(ctx, link.OrganizationID, link.ReservationID)
}

func (s *Service) visitReservation(ctx context.Context, orgID, reservationID int64) (db.GetCustomerPanelReservationRow, error) {
	row, err := s.q.GetCustomerVisitReservation(ctx, db.GetCustomerVisitReservationParams{OrganizationID: orgID, ID: reservationID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.GetCustomerPanelReservationRow{}, ErrLinkInvalid
	}
	if err != nil {
		return db.GetCustomerPanelReservationRow{}, err
	}
	return db.GetCustomerPanelReservationRow(row), nil
}

// visitView is the panel detail of the visit; moving it needs the signed-in
// panel, so a visit link offers only the cancel.
func (s *Service) visitView(o owned) model.Reservation {
	out := s.view(o)
	out.Actions.Reschedule = model.Action{Reason: reason(model.ReasonSignInRequired)}
	return out
}

// Visit opens the reservation of a visit link.
func (s *Service) Visit(ctx context.Context, in model.VisitInput, meta Meta) (model.Reservation, error) {
	row, err := s.visitRow(ctx, in.Token, meta)
	if err != nil {
		return model.Reservation{}, err
	}
	o, err := s.loadRow(ctx, row)
	if err != nil {
		return model.Reservation{}, err
	}
	return s.visitView(o), nil
}

// VisitCancel cancels the reservation of a visit link under the business's
// cancel rules, exactly as the panel does.
func (s *Service) VisitCancel(ctx context.Context, in model.VisitCancelInput, meta Meta) (Changed, error) {
	row, err := s.visitRow(ctx, in.Token, meta)
	if err != nil {
		return Changed{}, err
	}
	o, err := s.loadRow(ctx, row)
	if err != nil {
		return Changed{}, err
	}
	if err := s.cancel(ctx, o, in.ExpectedVersion); err != nil {
		return Changed{}, err
	}
	fresh, err := s.visitReservation(ctx, row.OrganizationID, row.ID)
	if err != nil {
		return Changed{}, err
	}
	reloaded, err := s.loadRow(ctx, fresh)
	if err != nil {
		return Changed{}, err
	}
	return Changed{Reservation: s.visitView(reloaded), Scope: scopeOf(reloaded)}, nil
}
