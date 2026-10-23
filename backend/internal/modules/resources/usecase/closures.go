package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ListClosures pages the closed periods of a resource, newest first.
func (s *Service) ListClosures(ctx context.Context, orgID int64, id uuid.UUID, f model.ClosureFilter) ([]model.Closure, int64, error) {
	row, err := s.load(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return nil, 0, err
	}
	if f.Limit <= 0 || f.Limit > closurePageLimit {
		f.Limit = closurePageLimit
	}
	from, to := tsArg(f.From), tsArg(f.To)
	rows, err := s.repo.Q().ListResourceClosures(ctx, db.ListResourceClosuresParams{
		OrganizationID: orgID, ResourceID: row.ID, RangeFrom: from, RangeTo: to, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.repo.Q().CountResourceClosures(ctx, db.CountResourceClosuresParams{
		OrganizationID: orgID, ResourceID: row.ID, RangeFrom: from, RangeTo: to,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Closure, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapClosure(r))
	}
	return out, total, nil
}

// CreateClosure records a closed period of a resource.
func (s *Service) CreateClosure(ctx context.Context, orgID int64, id uuid.UUID, actorID *int64, in model.ClosureInput) (model.Resource, model.Closure, error) {
	row, err := s.load(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return model.Resource{}, model.Closure{}, err
	}
	var f closureFields
	if err := f.apply(in, true); err != nil {
		return model.Resource{}, model.Closure{}, err
	}
	created := pgtype.Int8{}
	if actorID != nil {
		created = pgtype.Int8{Int64: *actorID, Valid: true}
	}
	r, err := s.repo.Q().CreateResourceClosure(ctx, db.CreateResourceClosureParams{
		OrganizationID: orgID, ResourceID: row.ID, StartsAt: ts(f.StartsAt), EndsAt: ts(f.EndsAt),
		Reason: f.Reason, CreatedBy: created,
	})
	if err != nil {
		return model.Resource{}, model.Closure{}, err
	}
	return MapResource(row), mapClosure(r), nil
}

// PatchClosure updates a closed period. It returns the resource, the
// previous and the new state.
func (s *Service) PatchClosure(ctx context.Context, orgID int64, id, closureID uuid.UUID, in model.ClosureInput) (model.Resource, model.Closure, model.Closure, error) {
	row, err := s.load(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return model.Resource{}, model.Closure{}, model.Closure{}, err
	}
	current, err := s.repo.Q().GetResourceClosure(ctx, db.GetResourceClosureParams{OrganizationID: orgID, ResourceID: row.ID, Uuid: closureID})
	if err != nil {
		return model.Resource{}, model.Closure{}, model.Closure{}, mapNotFound(repository.MapErr(err), ErrClosureNotFound)
	}
	f := closureFields{StartsAt: current.StartsAt.Time, EndsAt: current.EndsAt.Time, Reason: current.Reason}
	if err := f.apply(in, false); err != nil {
		return model.Resource{}, model.Closure{}, model.Closure{}, err
	}
	r, err := s.repo.Q().UpdateResourceClosure(ctx, db.UpdateResourceClosureParams{
		OrganizationID: orgID, ResourceID: row.ID, Uuid: closureID,
		StartsAt: ts(f.StartsAt), EndsAt: ts(f.EndsAt), Reason: f.Reason,
	})
	if err != nil {
		return model.Resource{}, model.Closure{}, model.Closure{}, mapNotFound(repository.MapErr(err), ErrClosureNotFound)
	}
	return MapResource(row), mapClosure(current), mapClosure(r), nil
}

// DeleteClosure removes a closed period and returns it.
func (s *Service) DeleteClosure(ctx context.Context, orgID int64, id, closureID uuid.UUID) (model.Resource, model.Closure, error) {
	row, err := s.load(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return model.Resource{}, model.Closure{}, err
	}
	current, err := s.repo.Q().GetResourceClosure(ctx, db.GetResourceClosureParams{OrganizationID: orgID, ResourceID: row.ID, Uuid: closureID})
	if err != nil {
		return model.Resource{}, model.Closure{}, mapNotFound(repository.MapErr(err), ErrClosureNotFound)
	}
	if _, err := s.repo.Q().DeleteResourceClosure(ctx, db.DeleteResourceClosureParams{OrganizationID: orgID, ResourceID: row.ID, Uuid: closureID}); err != nil {
		return model.Resource{}, model.Closure{}, err
	}
	return MapResource(row), mapClosure(current), nil
}

type closureFields struct {
	StartsAt, EndsAt time.Time
	Reason           string
}

func (f *closureFields) apply(in model.ClosureInput, create bool) error {
	ve := &ValidationError{}
	if in.StartsAt != nil {
		f.StartsAt = in.StartsAt.UTC()
	} else if create {
		ve.add("starts_at", "required", "starts_at is required")
	}
	if in.EndsAt != nil {
		f.EndsAt = in.EndsAt.UTC()
	} else if create {
		ve.add("ends_at", "required", "ends_at is required")
	}
	if in.Reason != nil {
		f.Reason = strings.TrimSpace(*in.Reason)
		if utf8.RuneCountInString(f.Reason) > maxClosureReason {
			ve.add("reason", "too_long", fmt.Sprintf("reason must be at most %d characters", maxClosureReason))
		}
	}
	if len(ve.Details) == 0 {
		switch {
		case !f.EndsAt.After(f.StartsAt):
			ve.add("ends_at", "invalid", "ends_at must be after starts_at")
		case f.EndsAt.Sub(f.StartsAt) > maxClosureSpan:
			ve.add("ends_at", "too_long", "a closed period spans at most 366 days")
		}
	}
	return ve.orNil()
}

func mapClosure(r db.ResourceClosure) model.Closure {
	return model.Closure{
		UUID: r.Uuid, StartsAt: r.StartsAt.Time, EndsAt: r.EndsAt.Time, Reason: r.Reason, CreatedAt: r.CreatedAt.Time,
	}
}

func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func tsArg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return ts(*t)
}
