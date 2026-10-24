package usecase

// Confirmation answers (TEC-921): the customer's "Geleceğim" and change
// request for the business's confirmation request, in the signed-in panel
// and through a visit link. A cancel is the ordinary cancel under the
// business's cancel rules (the confirmation module records it as
// declined). The answer itself is written by the confirmation module on the
// locked reservation; the panel only decides whether to offer it.

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	confirmationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/confirmations/model"
	confirmationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/confirmations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/model"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// Confirmer records confirmation answers (the confirmation module).
type Confirmer interface {
	Answer(ctx context.Context, orgID int64, id uuid.UUID, in confirmationsusecase.AnswerInput) error
}

// SetConfirmations plugs in the confirmation module (nil offers no
// confirmation answers).
func (s *Service) SetConfirmations(c Confirmer) {
	s.deps.Confirmations = c
}

// confirmActions decides the confirm and change request actions of a visit.
func (o owned) confirmActions(now time.Time) (model.Action, model.Action) {
	r := o.full
	c := r.Confirmation
	decide := func(change bool) model.Action {
		switch {
		case !o.live:
			return model.Action{Reason: reason(model.ReasonBusinessUnavailable)}
		case c == nil || c.Status == confirmationsmodel.TaskDropped || !o.modules.Allows(sector.ModuleConfirmation):
			return model.Action{Reason: reason(model.ReasonNotRequested)}
		case r.Status != reservationsmodel.StatusActive || !now.Before(r.StartsAt):
			return model.Action{Reason: reason(model.ReasonStatus)}
		case !confirmationsusecase.CanAnswer(c), change && c.ChangeRequestedAt != nil:
			return model.Action{Reason: reason(model.ReasonAnswered)}
		}
		until := c.DeadlineAt
		if c.Status == confirmationsmodel.TaskExpired || !now.Before(until) {
			// Flagged after the deadline: a late answer still counts until
			// the visit starts.
			until = r.StartsAt
		}
		return model.Action{Allowed: true, Until: &until}
	}
	return decide(false), decide(true)
}

// confirmationView is the customer's view of the confirmation request.
func confirmationView(r reservationsmodel.Reservation) *model.Confirmation {
	c := r.Confirmation
	if c == nil || c.Status == confirmationsmodel.TaskDropped {
		return nil
	}
	return &model.Confirmation{
		Status: r.ConfirmationStatus, DeadlineAt: c.DeadlineAt, AnsweredAt: c.AnsweredAt, ChangeRequestedAt: c.ChangeRequestedAt,
	}
}

// answer checks the action and hands the answer to the confirmation
// module, with the ownership guard on the locked record.
func (s *Service) answer(ctx context.Context, o owned, answer, source, note string) error {
	confirm, change := o.confirmActions(s.now())
	action, name := confirm, "confirm"
	if answer == confirmationsmodel.AnswerChange {
		action, name = change, "request_change"
		note = strings.TrimSpace(note)
		if utf8.RuneCountInString(note) > confirmationsmodel.MaxChangeNote {
			return invalid("note", "too_long", "note is at most 500 characters")
		}
	}
	if !action.Allowed {
		return &ActionError{Action: name, Reason: *action.Reason}
	}
	if s.deps.Confirmations == nil {
		return &ActionError{Action: name, Reason: model.ReasonNotRequested}
	}
	err := s.deps.Confirmations.Answer(ctx, o.row.OrganizationID, o.row.Uuid, confirmationsusecase.AnswerInput{
		Answer: answer, Source: source, Note: note,
		Guard: func(l reservationsmodel.Locked) error {
			if l.CustomerID != o.row.CustomerID {
				return ErrNotFound
			}
			return nil
		},
	})
	switch {
	case errors.Is(err, confirmationsusecase.ErrNotOpen):
		// Changed since the visit was read (answered, cancelled, started).
		return reservationsusecase.ErrInvalidTransition
	case errors.Is(err, confirmationsusecase.ErrNotFound):
		return ErrNotFound
	}
	return err
}

// Confirm records the person's "Geleceğim" for a visit.
func (s *Service) Confirm(ctx context.Context, p Principal, id uuid.UUID) (Changed, error) {
	o, err := s.load(ctx, p, id)
	if err != nil {
		return Changed{}, err
	}
	if err := s.answer(ctx, o, confirmationsmodel.AnswerConfirm, confirmationsmodel.SourcePanel, ""); err != nil {
		return Changed{}, err
	}
	return s.reloaded(ctx, p, o)
}

// RequestChange asks the business for another time; the visit stays as it
// is until the business moves it.
func (s *Service) RequestChange(ctx context.Context, p Principal, id uuid.UUID, in model.ChangeRequestInput) (Changed, error) {
	o, err := s.load(ctx, p, id)
	if err != nil {
		return Changed{}, err
	}
	if err := s.answer(ctx, o, confirmationsmodel.AnswerChange, confirmationsmodel.SourcePanel, in.Note); err != nil {
		return Changed{}, err
	}
	return s.reloaded(ctx, p, o)
}

// visitAnswer answers through a visit link. Opening the link changes
// nothing; only this explicit request does (A21).
func (s *Service) visitAnswer(ctx context.Context, token string, meta Meta, answer, note string) (Changed, error) {
	row, err := s.visitRow(ctx, token, meta)
	if err != nil {
		return Changed{}, err
	}
	o, err := s.loadRow(ctx, row)
	if err != nil {
		return Changed{}, err
	}
	if err := s.answer(ctx, o, answer, confirmationsmodel.SourceVisitLink, note); err != nil {
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

// ChannelAnswer applies a customer's reply on a message channel (a
// WhatsApp button, TEC-922) to the visit the message was about, with the
// rules of the visit link: "Geleceğim", a change request (no note) or the
// cancel under the business's cancel rules. The caller has checked that the
// reply came from the customer's number and answers the current request.
func (s *Service) ChannelAnswer(ctx context.Context, orgID, reservationID int64, answer, source string) (Changed, error) {
	row, err := s.visitReservation(ctx, orgID, reservationID)
	if err != nil {
		return Changed{}, ErrNotFound
	}
	o, err := s.loadRow(ctx, row)
	if err != nil {
		return Changed{}, err
	}
	switch answer {
	case confirmationsmodel.AnswerConfirm, confirmationsmodel.AnswerChange:
		err = s.answer(ctx, o, answer, source, "")
	case confirmationsmodel.AnswerCancel:
		err = s.cancel(ctx, o, nil)
	default:
		return Changed{}, &ActionError{Action: "confirm", Reason: model.ReasonNotRequested}
	}
	if err != nil {
		return Changed{}, err
	}
	fresh, err := s.visitReservation(ctx, orgID, reservationID)
	if err != nil {
		return Changed{}, err
	}
	reloaded, err := s.loadRow(ctx, fresh)
	if err != nil {
		return Changed{}, err
	}
	return Changed{Reservation: s.visitView(reloaded), Scope: scopeOf(reloaded)}, nil
}

// VisitConfirm confirms the visit of a visit link.
func (s *Service) VisitConfirm(ctx context.Context, in model.VisitConfirmInput, meta Meta) (Changed, error) {
	return s.visitAnswer(ctx, in.Token, meta, confirmationsmodel.AnswerConfirm, "")
}

// VisitRequestChange asks for a change through a visit link.
func (s *Service) VisitRequestChange(ctx context.Context, in model.VisitChangeRequestInput, meta Meta) (Changed, error) {
	return s.visitAnswer(ctx, in.Token, meta, confirmationsmodel.AnswerChange, in.Note)
}
