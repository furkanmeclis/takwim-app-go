// Package usecase holds the organization settings rules (TEC-690): typed
// validation, immutable versioning and the online sales readiness flag.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	// ErrVersionNotFound is returned for an unknown version number (404).
	ErrVersionNotFound = errors.New("settings version not found")
	// ErrVersionConflict is returned when base_version is not the current
	// version (409): someone else saved in between.
	ErrVersionConflict = errors.New("settings were changed by someone else")
)

// Repository is the persistence port; every method is scoped by orgID.
type Repository interface {
	Current(ctx context.Context, orgID int64) (repository.VersionRow, error)
	Get(ctx context.Context, orgID int64, version int32) (repository.VersionRow, error)
	List(ctx context.Context, orgID int64, desc bool, limit, offset int32) ([]repository.VersionRow, int64, error)
	Insert(ctx context.Context, orgID int64, data []byte, createdBy int64, check func(current int32) error) (int32, error)
}

// Service implements the settings use cases.
type Service struct {
	repo Repository
}

// New builds the service.
func New(repo Repository) *Service {
	return &Service{repo: repo}
}

// Current returns the current settings (version 0 with defaults when the
// organization never saved).
func (s *Service) Current(ctx context.Context, orgID int64) (model.Version, error) {
	row, err := s.repo.Current(ctx, orgID)
	if errors.Is(err, repository.ErrNotFound) {
		settings := model.Defaults()
		ready, missing := model.Readiness(settings)
		return model.Version{Version: 0, Settings: settings, ReadyForOnlineSales: ready, Missing: missing}, nil
	}
	if err != nil {
		return model.Version{}, err
	}
	return mapVersion(row)
}

// Get returns one stored version.
func (s *Service) Get(ctx context.Context, orgID int64, version int) (model.Version, error) {
	if version <= 0 {
		return model.Version{}, ErrVersionNotFound
	}
	row, err := s.repo.Get(ctx, orgID, int32(version))
	if errors.Is(err, repository.ErrNotFound) {
		return model.Version{}, ErrVersionNotFound
	}
	if err != nil {
		return model.Version{}, err
	}
	return mapVersion(row)
}

// List returns one page of the version history.
func (s *Service) List(ctx context.Context, orgID int64, desc bool, limit, offset int32) ([]model.VersionSummary, int64, error) {
	rows, total, err := s.repo.List(ctx, orgID, desc, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.VersionSummary, 0, len(rows))
	for _, row := range rows {
		v, err := mapVersion(row)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, model.VersionSummary{
			Version: v.Version, ReadyForOnlineSales: v.ReadyForOnlineSales,
			CreatedAt: *v.CreatedAt, CreatedBy: v.CreatedBy,
		})
	}
	return out, total, nil
}

// Save validates the settings and stores them as a new version.
func (s *Service) Save(ctx context.Context, orgID, actorID int64, in model.SaveInput) (model.Version, error) {
	settings, err := Normalize(in.Settings)
	if err != nil {
		return model.Version{}, err
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return model.Version{}, err
	}
	version, err := s.repo.Insert(ctx, orgID, data, actorID, func(current int32) error {
		if in.BaseVersion != nil && int32(*in.BaseVersion) != current {
			return fmt.Errorf("%w (current version %d)", ErrVersionConflict, current)
		}
		return nil
	})
	if err != nil {
		return model.Version{}, err
	}
	return s.Get(ctx, orgID, int(version))
}

func mapVersion(row repository.VersionRow) (model.Version, error) {
	settings := model.Defaults()
	if len(row.Data) > 0 {
		if err := json.Unmarshal(row.Data, &settings); err != nil {
			return model.Version{}, fmt.Errorf("decode settings version %d: %w", row.Version, err)
		}
	}
	if settings.Embed.AllowedOrigins == nil {
		settings.Embed.AllowedOrigins = []string{}
	}
	ready, missing := model.Readiness(settings)
	created := row.CreatedAt.Time.UTC()
	return model.Version{
		Version: int(row.Version), Settings: settings, ReadyForOnlineSales: ready, Missing: missing,
		CreatedAt: &created, CreatedBy: actor(row.CreatedByUuid, row.CreatedByName, row.CreatedBySurname, row.CreatedByEmail),
	}, nil
}

func actor(id pgtype.UUID, name, surname, email pgtype.Text) *model.Actor {
	if !id.Valid {
		return nil
	}
	return &model.Actor{UUID: id.Bytes, Name: name.String, Surname: surname.String, Email: email.String}
}
