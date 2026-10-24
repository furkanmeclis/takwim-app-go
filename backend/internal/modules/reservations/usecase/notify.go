package usecase

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
)

// Change is one written reservation change (a reservation_events row) as a
// notifier sees it.
type Change struct {
	OrganizationID int64
	ReservationID  int64
	// Version is the reservation's version after the change.
	Version int32
	// Type is the event type: created, accepted, rejected, cancelled,
	// expired, rescheduled, paid, started, completed, no_show,
	// notes_updated.
	Type           string
	From, To       string
	ApprovalStatus string
	CancelReason   string
}

// Notifier reacts to reservation changes inside their transaction (the
// customer notifications of TEC-911): what it writes commits with the
// change, and an error rolls the change back.
type Notifier interface {
	ReservationChanged(ctx context.Context, q *db.Queries, c Change) error
}

// SetNotifier registers the notifier of reservation changes.
func (s *Service) SetNotifier(n Notifier) {
	s.notifier = n
}

// ExpireRequestsNotifying is ExpireRequests that also tells n about the
// requests it cancels (the capacity writers' expiry hook).
func ExpireRequestsNotifying(n Notifier) availabilityusecase.ExpiryHook {
	return func(ctx context.Context, q *db.Queries, orgID int64, holdIDs []int64, now time.Time) error {
		return expireRequests(ctx, q, n, orgID, holdIDs, now)
	}
}

// event writes the history row of a change and tells the notifier.
func (s *Service) event(ctx context.Context, q *db.Queries, row db.Reservation, typ, from, to string, actorID int64, note string) error {
	return recordEvent(ctx, q, s.notifier, row, typ, from, to, actorID, note)
}

func recordEvent(ctx context.Context, q *db.Queries, n Notifier, row db.Reservation, typ, from, to string, actorID int64, note string) error {
	if err := q.CreateReservationEvent(ctx, db.CreateReservationEventParams{
		OrganizationID: row.OrganizationID, ReservationID: row.ID, Version: row.Version, Type: typ,
		FromStatus: pgtype.Text{String: from, Valid: from != ""}, ToStatus: pgtype.Text{String: to, Valid: to != ""},
		ActorID: pgtype.Int8{Int64: actorID, Valid: actorID > 0}, Note: note,
	}); err != nil {
		return err
	}
	if n == nil {
		return nil
	}
	return n.ReservationChanged(ctx, q, Change{
		OrganizationID: row.OrganizationID, ReservationID: row.ID, Version: row.Version, Type: typ, From: from, To: to,
		ApprovalStatus: row.ApprovalStatus, CancelReason: row.CancelReason.String,
	})
}
