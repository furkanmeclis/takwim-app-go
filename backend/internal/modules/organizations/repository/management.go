package repository

import (
	"context"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Staff invitations (organization_invitations.sql) and platform organization
// management (organization_management.sql).

func (r *Postgres) CreateInvitation(ctx context.Context, q *db.Queries, params db.CreateInvitationParams) (db.OrganizationInvitation, error) {
	return r.queries(q).CreateInvitation(ctx, params)
}

func (r *Postgres) AddInvitationRoles(ctx context.Context, q *db.Queries, orgID, invitationID int64, roleIDs []int64) error {
	q = r.queries(q)
	for _, id := range roleIDs {
		if err := q.AddInvitationRole(ctx, db.AddInvitationRoleParams{
			OrganizationID: orgID, InvitationID: invitationID, RoleID: id,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Postgres) AddInvitationBranches(ctx context.Context, q *db.Queries, orgID, invitationID int64, branchIDs []int64) error {
	q = r.queries(q)
	for _, id := range branchIDs {
		if err := q.AddInvitationBranch(ctx, db.AddInvitationBranchParams{
			OrganizationID: orgID, InvitationID: invitationID, BranchID: id,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Postgres) RevokeOpenInvitationsForEmail(ctx context.Context, q *db.Queries, orgID int64, email string, by *int64) error {
	_, err := r.queries(q).RevokeOpenInvitationsForEmail(ctx, db.RevokeOpenInvitationsForEmailParams{
		OrganizationID: orgID, Email: email, RevokedBy: int8Ptr(by),
	})
	return err
}

func (r *Postgres) GetInvitationByUUID(ctx context.Context, orgID int64, id uuid.UUID) (db.OrganizationInvitation, error) {
	row, err := r.q.GetInvitationByUUID(ctx, db.GetInvitationByUUIDParams{OrganizationID: orgID, Uuid: id})
	return row, mapErr(err)
}

func (r *Postgres) GetInvitationPreview(ctx context.Context, tokenHash string) (db.GetInvitationPreviewByTokenHashRow, error) {
	row, err := r.q.GetInvitationPreviewByTokenHash(ctx, tokenHash)
	return row, mapErr(err)
}

func (r *Postgres) LockInvitationByTokenHash(ctx context.Context, q *db.Queries, tokenHash string) (db.OrganizationInvitation, error) {
	row, err := r.queries(q).LockInvitationByTokenHash(ctx, tokenHash)
	return row, mapErr(err)
}

func (r *Postgres) ListInvitations(ctx context.Context, params db.ListInvitationsParams) ([]db.ListInvitationsRow, int64, error) {
	rows, err := r.q.ListInvitations(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountInvitations(ctx, db.CountInvitationsParams{
		OrganizationID: params.OrganizationID, Q: params.Q, Statuses: params.Statuses,
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *Postgres) ListInvitationRoles(ctx context.Context, q *db.Queries, orgID int64, ids []int64) ([]db.ListInvitationRolesRow, error) {
	return r.queries(q).ListInvitationRoles(ctx, db.ListInvitationRolesParams{OrganizationID: orgID, InvitationIds: ids})
}

func (r *Postgres) ListInvitationBranches(ctx context.Context, q *db.Queries, orgID int64, ids []int64) ([]db.ListInvitationBranchesRow, error) {
	return r.queries(q).ListInvitationBranches(ctx, db.ListInvitationBranchesParams{OrganizationID: orgID, InvitationIds: ids})
}

func (r *Postgres) RevokeInvitation(ctx context.Context, orgID int64, id uuid.UUID, by *int64) (db.OrganizationInvitation, error) {
	row, err := r.q.RevokeInvitation(ctx, db.RevokeInvitationParams{OrganizationID: orgID, Uuid: id, RevokedBy: int8Ptr(by)})
	return row, mapErr(err)
}

func (r *Postgres) RotateInvitationToken(ctx context.Context, params db.RotateInvitationTokenParams) (db.OrganizationInvitation, error) {
	row, err := r.q.RotateInvitationToken(ctx, params)
	return row, mapErr(err)
}

func (r *Postgres) MarkInvitationAccepted(ctx context.Context, q *db.Queries, orgID, invitationID, userID int64) error {
	n, err := r.queries(q).MarkInvitationAccepted(ctx, db.MarkInvitationAcceptedParams{
		OrganizationID: orgID, ID: invitationID, AcceptedBy: pgtype.Int8{Int64: userID, Valid: true},
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Postgres) FindUserByEmail(ctx context.Context, email string) (db.User, error) {
	row, err := r.q.GetUserByEmail(ctx, email)
	return row, mapErr(err)
}

func (r *Postgres) ListOrganizationBranchOptions(ctx context.Context, orgID int64) ([]db.ListOrganizationBranchOptionsRow, error) {
	return r.q.ListOrganizationBranchOptions(ctx, orgID)
}

func (r *Postgres) ListOrganizationActivity(ctx context.Context, orgID int64, orgUUID uuid.UUID, limit, offset int32) ([]db.ListPlatformOrganizationActivityRow, int64, error) {
	rows, err := r.q.ListPlatformOrganizationActivity(ctx, db.ListPlatformOrganizationActivityParams{
		OrganizationUuid: orgUUID, OrganizationID: orgID, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountPlatformOrganizationActivity(ctx, db.CountPlatformOrganizationActivityParams{
		OrganizationUuid: orgUUID, OrganizationID: orgID,
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *Postgres) SetOrganizationLogo(ctx context.Context, orgID int64, key *string) (db.Organization, error) {
	v := pgtype.Text{}
	if key != nil {
		v = pgtype.Text{String: *key, Valid: true}
	}
	row, err := r.q.SetOrganizationLogo(ctx, db.SetOrganizationLogoParams{ID: orgID, LogoObjectKey: v})
	return row, mapErr(err)
}

func int8Ptr(v *int64) pgtype.Int8 {
	if v == nil || *v <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}
