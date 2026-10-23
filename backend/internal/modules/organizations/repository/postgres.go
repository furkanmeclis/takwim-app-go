package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type TxFunc func(ctx context.Context, q *db.Queries, tx pgx.Tx) error

type Postgres struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func NewPostgres(pool *pgxpool.Pool, q *db.Queries) *Postgres {
	return &Postgres{pool: pool, q: q}
}

func (r *Postgres) WithTx(ctx context.Context, fn TxFunc) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(ctx, r.q.WithTx(tx), tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Postgres) CreateOrganization(ctx context.Context, q *db.Queries, params db.CreateOrganizationParams) (db.Organization, error) {
	return q.CreateOrganization(ctx, params)
}

func (r *Postgres) AddOrganizationMember(ctx context.Context, q *db.Queries, params db.AddOrganizationMemberParams) error {
	_, err := q.AddOrganizationMember(ctx, params)
	return err
}

func (r *Postgres) GetUserByID(ctx context.Context, id int64) (db.User, error) {
	row, err := r.q.GetUserByID(ctx, id)
	return row, mapErr(err)
}

func (r *Postgres) GetUserByUUID(ctx context.Context, id uuid.UUID) (db.User, error) {
	row, err := r.q.GetUserByUUID(ctx, id)
	return row, mapErr(err)
}

func (r *Postgres) GetTenantRoleByKey(ctx context.Context, q *db.Queries, orgID int64, key string) (db.Role, error) {
	row, err := q.GetTenantRoleByKey(ctx, db.GetTenantRoleByKeyParams{
		RoleKey: key, OrganizationID: pgtype.Int8{Int64: orgID, Valid: true},
	})
	return row, mapErr(err)
}

func (r *Postgres) GetOrganizationByUUID(ctx context.Context, id uuid.UUID) (db.Organization, error) {
	row, err := r.q.GetOrganizationByUUID(ctx, id)
	return row, mapErr(err)
}

func (r *Postgres) GetOrganizationByID(ctx context.Context, id int64) (db.Organization, error) {
	row, err := r.q.GetOrganizationByID(ctx, id)
	return row, mapErr(err)
}

func (r *Postgres) SlugExists(ctx context.Context, slug string) (bool, error) {
	return r.q.SlugExists(ctx, slug)
}

func (r *Postgres) ListOrganizations(ctx context.Context, params db.ListOrganizationsWithStatsParams) ([]db.ListOrganizationsWithStatsRow, int64, error) {
	rows, err := r.q.ListOrganizationsWithStats(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountOrganizationsWithStats(ctx, db.CountOrganizationsWithStatsParams{
		OrganizationUuid: params.OrganizationUuid, Statuses: params.Statuses, AccessStates: params.AccessStates, Windows: params.Windows, Q: params.Q,
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *Postgres) UpdateOrganization(ctx context.Context, params db.UpdateOrganizationParams) (db.Organization, error) {
	row, err := r.q.UpdateOrganization(ctx, params)
	return row, mapErr(err)
}

func (r *Postgres) UpdateOrganizationEntitlements(ctx context.Context, params db.UpdateOrganizationEntitlementsParams) (db.Organization, error) {
	row, err := r.q.UpdateOrganizationEntitlements(ctx, params)
	return row, mapErr(err)
}

func (r *Postgres) SetOrganizationStatus(ctx context.Context, params db.SetOrganizationStatusParams) (db.Organization, error) {
	row, err := r.q.SetOrganizationStatus(ctx, params)
	return row, mapErr(err)
}

func (r *Postgres) SetOrganizationAccessWindow(ctx context.Context, params db.SetOrganizationAccessWindowParams) (db.Organization, error) {
	row, err := r.q.SetOrganizationAccessWindow(ctx, params)
	return row, mapErr(err)
}

func (r *Postgres) SoftDeleteOrganization(ctx context.Context, id uuid.UUID) (db.Organization, error) {
	row, err := r.q.SoftDeleteOrganization(ctx, id)
	return row, mapErr(err)
}

func (r *Postgres) LockOrganizationRow(ctx context.Context, q *db.Queries, orgID int64) error {
	_, err := q.LockOrganizationRow(ctx, orgID)
	return mapErr(err)
}

func (r *Postgres) GetOrganizationMemberByUserUUID(ctx context.Context, orgID int64, userUUID uuid.UUID) (db.GetOrganizationMemberByUserUUIDRow, error) {
	row, err := r.q.GetOrganizationMemberByUserUUID(ctx, db.GetOrganizationMemberByUserUUIDParams{
		OrganizationID: orgID, Uuid: userUUID,
	})
	return row, mapErr(err)
}

func (r *Postgres) GetOrganizationMember(ctx context.Context, q *db.Queries, orgID, userID int64) (db.GetOrganizationMemberRow, error) {
	row, err := q.GetOrganizationMember(ctx, db.GetOrganizationMemberParams{OrganizationID: orgID, UserID: userID})
	return row, mapErr(err)
}

func (r *Postgres) ListOrganizationMembers(ctx context.Context, params db.ListOrganizationMembersByOrgParams) ([]db.ListOrganizationMembersByOrgRow, int64, error) {
	rows, err := r.q.ListOrganizationMembersByOrg(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountOrganizationMembersByOrg(ctx, db.CountOrganizationMembersByOrgParams{
		OrganizationID: params.OrganizationID, Q: params.Q,
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *Postgres) ListOrganizationMembershipsByUser(ctx context.Context, userID int64) ([]db.ListOrganizationMembershipsByUserRow, error) {
	return r.q.ListOrganizationMembershipsByUser(ctx, userID)
}

func (r *Postgres) UpdateOrganizationMemberRole(ctx context.Context, q *db.Queries, params db.UpdateOrganizationMemberRoleParams) error {
	_, err := q.UpdateOrganizationMemberRole(ctx, params)
	return mapErr(err)
}

func (r *Postgres) DeleteOrganizationMember(ctx context.Context, q *db.Queries, orgID, userID int64) error {
	n, err := q.DeleteOrganizationMember(ctx, db.DeleteOrganizationMemberParams{OrganizationID: orgID, UserID: userID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Postgres) CountOrganizationOwners(ctx context.Context, q *db.Queries, orgID int64) (int64, error) {
	return q.CountOrganizationOwners(ctx, orgID)
}

func (r *Postgres) ResolveOrganizationScopeBySlug(ctx context.Context, slug string, userID int64) (db.ResolveOrganizationScopeBySlugRow, error) {
	row, err := r.q.ResolveOrganizationScopeBySlug(ctx, db.ResolveOrganizationScopeBySlugParams{Slug: slug, UserID: userID})
	return row, mapErr(err)
}

func (r *Postgres) ResolveOrganizationScopeByUUID(ctx context.Context, id uuid.UUID, userID int64) (db.ResolveOrganizationScopeByUUIDRow, error) {
	row, err := r.q.ResolveOrganizationScopeByUUID(ctx, db.ResolveOrganizationScopeByUUIDParams{Uuid: id, UserID: userID})
	return row, mapErr(err)
}

func (r *Postgres) ListTenantRolePermissionSlugsByRoleID(ctx context.Context, roleID int64) ([]string, error) {
	rows, err := r.q.ListTenantRolePermissionSlugsByRoleID(ctx, roleID)
	if rows == nil {
		rows = []string{}
	}
	return rows, err
}

func mapErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
