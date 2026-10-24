package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilitymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
)

// ReinstateInput takes a cancellation back (the rollback of a class
// session cancel, TEC-989). It is built by the code that cancelled the
// visit from what it read before the cancel; it is never read from a
// request body.
type ReinstateInput struct {
	// ExpectedVersion is the version the cancel left: a visit changed
	// since (a refund recorded, a note, another step) is not taken back
	// (ErrStaleVersion).
	ExpectedVersion int
	// Status, ApprovalStatus and PaymentStatus are the states before the
	// cancel. Only a request or an active booking is taken back.
	Status         string
	ApprovalStatus string
	PaymentStatus  string
	// Packages are the package sessions the visit held; each is held again
	// (the next free session of the same package for the same line).
	Packages []model.PackageSessionInput
	// Products are the product lines the cancel released; each is held
	// again with its price snapshot.
	Products []uuid.UUID
	Note     string
	// Prepare runs first inside the transaction, under the capacity lock
	// (the class module opens the cancelled session again there, so the
	// visit's start is offered again in the same transaction).
	Prepare func(ctx context.Context, q *db.Queries) error
}

// Reinstate turns a cancelled visit back into what it was: its time is
// taken again through the capacity engine (a booking stays a booking; a
// request gets a new hold for its approval or payment), its package
// sessions and products are held again and the payment state is restored.
// When the time is no longer free, a package has no session left or a
// product is out of stock, nothing changes. The history gets a
// "reinstated" entry and the notifier tells the customer.
func (s *Service) Reinstate(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, actorID int64, in ReinstateInput) (model.Reservation, error) {
	if in.Status != model.StatusRequested && in.Status != model.StatusActive {
		return model.Reservation{}, ErrInvalidTransition
	}
	if len(in.Note) > MaxNoteLength {
		ve := &ValidationError{}
		ve.add("note", "too_long", "note is too long")
		return model.Reservation{}, ve
	}
	err := s.holds.Run(ctx, orgID, func(tx *availabilityusecase.Tx) error {
		row, err := s.visible(ctx, tx.Q, orgID, id, caller, true)
		if err != nil {
			return err
		}
		if row.Status != model.StatusCancelled {
			return ErrInvalidTransition
		}
		if int(row.Version) != in.ExpectedVersion {
			return ErrStaleVersion
		}
		if in.Prepare != nil {
			if err := in.Prepare(ctx, tx.Q); err != nil {
				return err
			}
		}
		full, err := tx.Q.GetReservation(ctx, db.GetReservationParams{OrganizationID: orgID, Uuid: id})
		if err != nil {
			return err
		}
		current, err := tx.Q.ListReservationLines(ctx, db.ListReservationLinesParams{OrganizationID: orgID, ReservationID: row.ID})
		if err != nil {
			return err
		}
		lines := make([]availabilitymodel.LineInput, 0, len(current))
		for _, l := range current {
			if !l.ServiceUuid.Valid {
				ve := &ValidationError{}
				ve.add("lines", "required", "a service of the booking was deleted")
				return ve
			}
			line := availabilitymodel.LineInput{ServiceUUID: uuid.UUID(l.ServiceUuid.Bytes)}
			if l.StaffUuid.Valid {
				u := uuid.UUID(l.StaffUuid.Bytes)
				line.StaffUUID = &u
			}
			lines = append(lines, line)
		}
		// A booking is booked again; a request waits again for what it
		// waited for (the business's answer or the online payment).
		reason, booking := availabilitymodel.ReasonCheckout, in.Status == model.StatusActive
		if !booking {
			reason = availabilitymodel.ReasonApproval
			if in.ApprovalStatus == model.ApprovalAccepted {
				reason = availabilitymodel.ReasonPayment
			}
		}
		start := row.StartsAt.Time
		created, err := s.holds.CreateIn(ctx, tx, availabilitymodel.HoldInput{
			BranchUUID: full.BranchUuid, Lines: lines, Channel: availabilitymodel.ChannelPanel,
			StartsAt: &start, Reason: reason, PartySize: partyOf(full.PartySize), Zone: full.Zone,
		}, caller, actorID)
		if err != nil {
			return err
		}
		if booking {
			if err := s.holds.ConfirmIn(ctx, tx, created.ID, RefType, &row.Uuid); err != nil {
				return err
			}
		}
		if err := reinstateProducts(ctx, tx.Q, orgID, row, in.Products, actorID); err != nil {
			return err
		}
		for i, p := range in.Packages {
			if _, err := holdSession(ctx, tx.Q, orgID, row, p.CustomerPackageUUID, p.LineIndex, actorID, fmt.Sprintf("package_sessions.%d.", i)); err != nil {
				return err
			}
		}
		total, err := visitTotal(ctx, tx.Q, orgID, row.ID)
		if err != nil {
			return err
		}
		if _, err := tx.Q.ReplaceReservationSchedule(ctx, db.ReplaceReservationScheduleParams{
			OrganizationID: orgID, ID: row.ID, HoldID: pgInt8(created.ID), StartsAt: ts(created.Slot.StartsAt),
			EndsAt: ts(created.Slot.EndsAt), TotalPrice: total.Numeric(), PartySize: full.PartySize, Zone: full.Zone,
		}); err != nil {
			return err
		}
		updated, err := tx.Q.UpdateReservationState(ctx, db.UpdateReservationStateParams{
			OrganizationID: orgID, ID: row.ID, Status: in.Status, ApprovalStatus: in.ApprovalStatus,
			PaymentStatus: in.PaymentStatus, CancelReason: pgtype.Text{},
		})
		if err != nil {
			return err
		}
		return s.event(ctx, tx.Q, updated, "reinstated", row.Status, updated.Status, actorID, in.Note)
	})
	if err != nil {
		return model.Reservation{}, err
	}
	s.index(ctx, orgID, id)
	return s.get(ctx, orgID, id)
}

// reinstateProducts holds the released product lines of a visit again
// with their price snapshot, reserving their units (stock levels locked in
// product order). A line that is not released is left alone; too few
// available units is insufficient_stock.
func reinstateProducts(ctx context.Context, q *db.Queries, orgID int64, row db.Reservation, ids []uuid.UUID, actorID int64) error {
	copies := make([]db.ReservationProduct, 0, len(ids))
	for _, id := range ids {
		p, err := q.CopyReleasedReservationProduct(ctx, db.CopyReleasedReservationProductParams{
			OrganizationID: orgID, ReservationID: row.ID, Uuid: id, CreatedBy: pgtype.Int8{Int64: actorID, Valid: actorID > 0},
		})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		copies = append(copies, p)
	}
	sort.SliceStable(copies, func(i, j int) bool {
		if copies[i].ProductID != copies[j].ProductID {
			return copies[i].ProductID < copies[j].ProductID
		}
		return copies[i].VariantID.Int64 < copies[j].VariantID.Int64
	})
	ve := &ValidationError{}
	for _, p := range copies {
		level, err := lockStock(ctx, q, orgID, p.BranchID, p.ProductID, p.VariantID)
		if err != nil {
			return err
		}
		available := int64(level.OnHand) - int64(level.Reserved)
		if available < int64(p.Quantity) {
			ve.add("products", "insufficient_stock", fmt.Sprintf("only %d units of %s are available", max(available, 0), p.ProductName))
			continue
		}
		if err := q.SetStockLevelReserved(ctx, db.SetStockLevelReservedParams{
			OrganizationID: orgID, ID: level.ID, Reserved: level.Reserved + p.Quantity,
		}); err != nil {
			return err
		}
	}
	return ve.orNil()
}
