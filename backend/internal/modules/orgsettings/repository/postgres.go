// Package repository stores organization settings versions. Every method
// takes the organization id explicitly; rows are insert-only.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned for a missing version.
var ErrNotFound = errors.New("not found")

// VersionRow is one stored version with its author.
type VersionRow = db.GetCurrentOrganizationSettingsRow

// Postgres implements the settings repository.
type Postgres struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewPostgres builds the repository.
func NewPostgres(pool *pgxpool.Pool, q *db.Queries) *Postgres {
	return &Postgres{pool: pool, q: q}
}

// Current returns the highest version (ErrNotFound when none was saved).
func (r *Postgres) Current(ctx context.Context, orgID int64) (VersionRow, error) {
	row, err := r.q.GetCurrentOrganizationSettings(ctx, orgID)
	return row, mapErr(err)
}

// Get returns one version.
func (r *Postgres) Get(ctx context.Context, orgID int64, version int32) (VersionRow, error) {
	row, err := r.q.GetOrganizationSettingsVersion(ctx, db.GetOrganizationSettingsVersionParams{
		OrganizationID: orgID, Version: version,
	})
	return VersionRow(row), mapErr(err)
}

// List returns one page of versions.
func (r *Postgres) List(ctx context.Context, orgID int64, desc bool, limit, offset int32) ([]VersionRow, int64, error) {
	rows, err := r.q.ListOrganizationSettingsVersions(ctx, db.ListOrganizationSettingsVersionsParams{
		OrganizationID: orgID, SortDesc: desc, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountOrganizationSettingsVersions(ctx, orgID)
	if err != nil {
		return nil, 0, err
	}
	out := make([]VersionRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, VersionRow(row))
	}
	return out, total, nil
}

// ErrVersionConflict is returned by Insert when check rejects the current
// version.
var ErrVersionConflict = errors.New("settings version conflict")

// Insert appends version current+1 under a lock on the organization row, so
// concurrent saves get consecutive versions. check receives the current
// version (0 when none) and may return ErrVersionConflict.
func (r *Postgres) Insert(ctx context.Context, orgID int64, data []byte, createdBy int64, check func(current int32) error) (int32, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)
	if _, err := q.LockOrganizationRow(ctx, orgID); err != nil {
		return 0, mapErr(err)
	}
	current, err := q.LatestOrganizationSettingsVersion(ctx, orgID)
	if err != nil {
		return 0, err
	}
	if check != nil {
		if err := check(current); err != nil {
			return 0, err
		}
	}
	var by pgtype.Int8
	if createdBy > 0 {
		by = pgtype.Int8{Int64: createdBy, Valid: true}
	}
	row, err := q.InsertOrganizationSettingsVersion(ctx, db.InsertOrganizationSettingsVersionParams{
		OrganizationID: orgID, Version: current + 1, Data: data, CreatedBy: by,
	})
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return row.Version, nil
}

func mapErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
