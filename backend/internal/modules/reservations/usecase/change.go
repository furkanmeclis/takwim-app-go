package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"math"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilitymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Reschedule moves a request or an active booking to a new start (and,
// when given, new services / staff). The old commitment is released and
// every line planned again on the same transaction's data, so a move into
// a time the booking itself occupied works; when the new time is not free
// nothing changes (ErrSlotTaken). A booking stays a booking; a waiting
// request keeps its hold reason and expiry. Lines with the same service and
// staff member keep the price they were booked at; other lines take today's
// price. The version rises, so old confirmation jobs no longer apply.
func (s *Service) Reschedule(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, actorID int64, in model.RescheduleInput) (model.Reservation, error) {
	ve := &ValidationError{}
	if in.StartsAt == nil {
		ve.add("starts_at", "required", "start is required")
	}
	if len(in.Note) > MaxNoteLength {
		ve.add("note", "too_long", "note is too long")
	}
	if err := ve.orNil(); err != nil {
		// Another business's uuid is 404 before the body is judged.
		if _, verr := s.visible(ctx, s.q, orgID, id, caller, false); verr != nil {
			return model.Reservation{}, verr
		}
		return model.Reservation{}, err
	}
	err := s.holds.Run(ctx, orgID, func(tx *availabilityusecase.Tx) error {
		row, err := s.visible(ctx, tx.Q, orgID, id, caller, true)
		if err != nil {
			return err
		}
		if in.ExpectedVersion != nil && *in.ExpectedVersion != int(row.Version) {
			return ErrStaleVersion
		}
		if row.Status != model.StatusRequested && row.Status != model.StatusActive {
			return ErrInvalidTransition
		}
		full, err := tx.Q.GetReservation(ctx, db.GetReservationParams{OrganizationID: orgID, Uuid: id})
		if err != nil {
			return err
		}
		current, err := tx.Q.ListReservationLines(ctx, db.ListReservationLinesParams{OrganizationID: orgID, ReservationID: row.ID})
		if err != nil {
			return err
		}
		// An own calendar member moves a shared visit as a whole: the
		// colleagues' lines keep their service and person and are checked
		// again with the rest; it changes only its own lines.
		if caller.OwnUserID > 0 {
			own, err := s.ownStaff(ctx, tx.Q, orgID, caller)
			if err != nil {
				return err
			}
			if err := keepsOthers(current, in.Lines, own); err != nil {
				return err
			}
			caller.KeepOthers = true
		}
		lines := in.Lines
		if len(lines) == 0 {
			for _, l := range current {
				if !l.ServiceUuid.Valid {
					ve.add("lines", "required", "a service of the booking was deleted; choose the services again")
					return ve
				}
				line := availabilitymodel.LineInput{ServiceUUID: uuid.UUID(l.ServiceUuid.Bytes)}
				if l.StaffUuid.Valid {
					u := uuid.UUID(l.StaffUuid.Bytes)
					line.StaffUUID = &u
				}
				lines = append(lines, line)
			}
		}
		// The current commitment: a booking stays one; a hold keeps its
		// reason and the time it has left.
		reason, booking := availabilitymodel.ReasonCheckout, true
		var minutes *int
		if oldID := holdID(row); oldID != 0 {
			old, err := tx.Q.GetCapacityHoldByIDForUpdate(ctx, db.GetCapacityHoldByIDForUpdateParams{OrganizationID: orgID, ID: oldID})
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if err == nil && old.Status == availabilitymodel.StatusActive && old.Kind == availabilitymodel.KindHold {
				booking = false
				if old.Reason.Valid {
					reason = old.Reason.String
				}
				left := int(math.Ceil(old.ExpiresAt.Time.Sub(tx.Now).Minutes()))
				minutes = &left
				if left < 1 {
					one := 1
					minutes = &one
				}
			}
			if err := s.release(ctx, tx, row, "rescheduled"); err != nil {
				return err
			}
		}
		created, err := s.holds.CreateIn(ctx, tx, availabilitymodel.HoldInput{
			BranchUUID: full.BranchUuid, Lines: lines, Channel: availabilitymodel.ChannelPanel,
			StartsAt: in.StartsAt, Reason: reason, Minutes: minutes,
		}, caller, actorID)
		if err != nil {
			return err
		}
		if booking {
			if err := s.holds.ConfirmIn(ctx, tx, created.ID, RefType, &row.Uuid); err != nil {
				return err
			}
		}
		priced, _, err := s.priceLines(ctx, tx, created.Slot, "")
		if err != nil {
			return err
		}
		before, err := tx.Q.ListReservationLinePrices(ctx, db.ListReservationLinePricesParams{OrganizationID: orgID, ReservationID: row.ID})
		if err != nil {
			return err
		}
		total := money.Zero()
		for i := range priced {
			if i < len(before) && before[i].ServiceID == priced[i].ServiceID && before[i].StaffID == priced[i].StaffID {
				priced[i].UnitPrice = before[i].UnitPrice
			}
			total = total.Add(money.FromNumericOrZero(priced[i].UnitPrice))
		}
		if err := tx.Q.DeleteReservationLines(ctx, db.DeleteReservationLinesParams{OrganizationID: orgID, ReservationID: row.ID}); err != nil {
			return err
		}
		for _, l := range priced {
			l.OrganizationID, l.ReservationID = orgID, row.ID
			if err := tx.Q.CreateReservationLine(ctx, l); err != nil {
				return err
			}
		}
		updated, err := tx.Q.ReplaceReservationSchedule(ctx, db.ReplaceReservationScheduleParams{
			OrganizationID: orgID, ID: row.ID, HoldID: pgInt8(created.ID), StartsAt: ts(created.Slot.StartsAt),
			EndsAt: ts(created.Slot.EndsAt), TotalPrice: total.Numeric(),
		})
		if err != nil {
			return err
		}
		return event(ctx, tx.Q, updated, "rescheduled", row.Status, updated.Status, actorID, in.Note)
	})
	if err != nil {
		return model.Reservation{}, err
	}
	s.index(ctx, orgID, id)
	return s.get(ctx, orgID, id)
}

// UpdateNotes edits the notes and the form answers of any reservation.
func (s *Service) UpdateNotes(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, actorID int64, in model.NotesInput) (model.Reservation, error) {
	ve := &ValidationError{}
	if len(in.CustomerNote) > MaxNoteLength {
		ve.add("customer_note", "too_long", "note is too long")
	}
	if len(in.InternalNote) > MaxNoteLength {
		ve.add("internal_note", "too_long", "note is too long")
	}
	if in.CustomFields == nil {
		in.CustomFields = map[string]any{}
	}
	fields, err := json.Marshal(in.CustomFields)
	if err != nil {
		ve.add("custom_fields", "invalid", "custom fields must be an object")
	}
	if err := ve.orNil(); err != nil {
		return model.Reservation{}, err
	}
	err = s.holds.Run(ctx, orgID, func(tx *availabilityusecase.Tx) error {
		row, err := s.visible(ctx, tx.Q, orgID, id, caller, true)
		if err != nil {
			return err
		}
		if in.ExpectedVersion != nil && *in.ExpectedVersion != int(row.Version) {
			return ErrStaleVersion
		}
		updated, err := tx.Q.UpdateReservationNotes(ctx, db.UpdateReservationNotesParams{
			OrganizationID: orgID, ID: row.ID, CustomerNote: in.CustomerNote, InternalNote: in.InternalNote, CustomFields: fields,
		})
		if err != nil {
			return err
		}
		return event(ctx, tx.Q, updated, "notes_updated", "", "", actorID, "")
	})
	if err != nil {
		return model.Reservation{}, err
	}
	return s.get(ctx, orgID, id)
}

func pgInt8(v int64) pgtype.Int8 {
	return pgtype.Int8{Int64: v, Valid: v != 0}
}

// keepsOthers checks that new lines leave the colleagues' lines (served by
// someone other than own) as they are: same position, service and person.
func keepsOthers(current []db.ListReservationLinesRow, next []availabilitymodel.LineInput, own int64) error {
	if len(next) == 0 {
		return nil
	}
	shared := false
	for _, l := range current {
		if l.StaffID.Valid && l.StaffID.Int64 != own {
			shared = true
			break
		}
	}
	if !shared {
		return nil
	}
	if len(next) != len(current) {
		return ErrOtherStaff
	}
	for i, l := range current {
		if !l.StaffID.Valid || l.StaffID.Int64 == own {
			continue
		}
		n := next[i]
		if !l.ServiceUuid.Valid || n.ServiceUUID != uuid.UUID(l.ServiceUuid.Bytes) ||
			n.StaffUUID == nil || !l.StaffUuid.Valid || *n.StaffUUID != uuid.UUID(l.StaffUuid.Bytes) {
			return ErrOtherStaff
		}
	}
	return nil
}
