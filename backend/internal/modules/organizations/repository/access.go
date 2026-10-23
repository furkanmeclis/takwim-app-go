package repository

import (
	"context"
	"errors"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrInUse is returned when a role is still referenced (members, invitations).
var ErrInUse = errors.New("in use")

// TenantRoleRow is a tenant role as listed for one organization.
type TenantRoleRow = db.ListTenantRolesForOrganizationRow

func (r *Postgres) queries(q *db.Queries) *db.Queries {
	if q != nil {
		return q
	}
	return r.q
}

// ListMemberPermissionSlugs returns the union of the permissions of every role
// of the member.
func (r *Postgres) ListMemberPermissionSlugs(ctx context.Context, orgID, memberID int64) ([]string, error) {
	rows, err := r.q.ListMemberPermissionSlugs(ctx, db.ListMemberPermissionSlugsParams{
		OrganizationID: orgID, MemberID: memberID,
	})
	if rows == nil {
		rows = []string{}
	}
	return rows, err
}

func (r *Postgres) ListMemberRoles(ctx context.Context, q *db.Queries, orgID int64, memberIDs []int64) ([]db.ListMemberRolesRow, error) {
	return r.queries(q).ListMemberRoles(ctx, db.ListMemberRolesParams{OrganizationID: orgID, MemberIds: memberIDs})
}

// ListMemberBranchIDs returns the live branches granted to a member; never nil.
func (r *Postgres) ListMemberBranchIDs(ctx context.Context, orgID, memberID int64) ([]int64, error) {
	rows, err := r.q.ListMemberBranchIDs(ctx, db.ListMemberBranchIDsParams{OrganizationID: orgID, MemberID: memberID})
	if rows == nil {
		rows = []int64{}
	}
	return rows, err
}

func (r *Postgres) ListMemberBranches(ctx context.Context, orgID int64, memberIDs []int64) ([]db.ListMemberBranchesRow, error) {
	return r.q.ListMemberBranches(ctx, db.ListMemberBranchesParams{OrganizationID: orgID, MemberIds: memberIDs})
}

func (r *Postgres) ListBranchIDsByUUIDs(ctx context.Context, orgID int64, ids []uuid.UUID) ([]db.ListBranchIDsByUUIDsRow, error) {
	return r.q.ListBranchIDsByUUIDs(ctx, db.ListBranchIDsByUUIDsParams{OrganizationID: orgID, Uuids: ids})
}

// ReplaceMemberRoles sets the member's roles and its primary role.
func (r *Postgres) ReplaceMemberRoles(ctx context.Context, q *db.Queries, orgID, memberID int64, roleIDs []int64, primaryRoleID int64) error {
	q = r.queries(q)
	if err := q.SetMemberPrimaryRole(ctx, db.SetMemberPrimaryRoleParams{
		OrganizationID: orgID, MemberID: memberID, RoleID: primaryRoleID,
	}); err != nil {
		return err
	}
	if err := q.DeleteMemberRoles(ctx, db.DeleteMemberRolesParams{OrganizationID: orgID, MemberID: memberID}); err != nil {
		return err
	}
	for _, id := range roleIDs {
		if err := q.AddMemberRole(ctx, db.AddMemberRoleParams{OrganizationID: orgID, MemberID: memberID, RoleID: id}); err != nil {
			return err
		}
	}
	return nil
}

// ReplaceMemberBranches sets the branch scope and, for 'selected', the branches.
func (r *Postgres) ReplaceMemberBranches(ctx context.Context, q *db.Queries, orgID, memberID int64, scope string, branchIDs []int64) error {
	q = r.queries(q)
	if err := q.SetMemberBranchScope(ctx, db.SetMemberBranchScopeParams{
		OrganizationID: orgID, MemberID: memberID, BranchScope: scope,
	}); err != nil {
		return err
	}
	if err := q.DeleteMemberBranches(ctx, db.DeleteMemberBranchesParams{OrganizationID: orgID, MemberID: memberID}); err != nil {
		return err
	}
	for _, id := range branchIDs {
		if err := q.AddMemberBranch(ctx, db.AddMemberBranchParams{OrganizationID: orgID, MemberID: memberID, BranchID: id}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Postgres) ListTenantMembers(ctx context.Context, params db.ListTenantMembersParams) ([]db.ListTenantMembersRow, int64, error) {
	rows, err := r.q.ListTenantMembers(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountTenantMembers(ctx, db.CountTenantMembersParams{
		OrganizationID: params.OrganizationID, Q: params.Q, BranchScope: params.BranchScope, RoleIds: params.RoleIds,
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *Postgres) GetTenantMember(ctx context.Context, orgID int64, userUUID uuid.UUID) (db.ListTenantMembersRow, error) {
	row, err := r.q.GetTenantMemberByUserUUID(ctx, db.GetTenantMemberByUserUUIDParams{
		OrganizationID: orgID, UserUuid: userUUID,
	})
	return db.ListTenantMembersRow(row), mapErr(err)
}

func (r *Postgres) ListTenantRoles(ctx context.Context, orgID int64) ([]TenantRoleRow, error) {
	return r.q.ListTenantRolesForOrganization(ctx, orgID)
}

func (r *Postgres) GetTenantRoleByUUID(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (TenantRoleRow, error) {
	row, err := r.queries(q).GetTenantRoleByUUIDForOrganization(ctx, db.GetTenantRoleByUUIDForOrganizationParams{
		OrganizationID: orgID, Uuid: id,
	})
	return TenantRoleRow(row), mapErr(err)
}

// ListRolePermissionSlugs returns the permission slugs of each role id.
func (r *Postgres) ListRolePermissionSlugs(ctx context.Context, roleIDs []int64) (map[int64][]string, error) {
	rows, err := r.q.ListRolePermissionSlugsByRoleIDs(ctx, roleIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[int64][]string, len(roleIDs))
	for _, row := range rows {
		out[row.RoleID] = append(out[row.RoleID], row.Slug)
	}
	return out, nil
}

func (r *Postgres) ListTenantPermissions(ctx context.Context) ([]db.ListTenantPermissionsRow, error) {
	return r.q.ListTenantPermissions(ctx)
}

func (r *Postgres) TenantRoleSlugTaken(ctx context.Context, orgID int64, slug string) (bool, error) {
	return r.q.TenantRoleSlugTaken(ctx, db.TenantRoleSlugTakenParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, Slug: slug,
	})
}

func (r *Postgres) CreateTenantRole(ctx context.Context, q *db.Queries, orgID int64, name, slug, description string) (int64, uuid.UUID, error) {
	row, err := r.queries(q).CreateTenantRole(ctx, db.CreateTenantRoleParams{
		Name: name, Slug: slug, Description: optText(description),
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true},
	})
	return row.ID, row.Uuid, err
}

func (r *Postgres) UpdateTenantRole(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID, name, description string) (int64, error) {
	roleID, err := r.queries(q).UpdateTenantRole(ctx, db.UpdateTenantRoleParams{
		Name: name, Description: optText(description), Uuid: id,
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true},
	})
	return roleID, mapErr(err)
}

func (r *Postgres) DeleteTenantRole(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) error {
	n, err := r.queries(q).DeleteTenantRole(ctx, db.DeleteTenantRoleParams{
		Uuid: id, OrganizationID: pgtype.Int8{Int64: orgID, Valid: true},
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ErrInUse
		}
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetRolePermissions replaces the permissions of a role by slug.
func (r *Postgres) SetRolePermissions(ctx context.Context, q *db.Queries, roleID int64, slugs []string) error {
	q = r.queries(q)
	if err := q.SetRolePermissions(ctx, roleID); err != nil {
		return err
	}
	for _, slug := range slugs {
		perm, err := q.GetPermissionBySlug(ctx, slug)
		if err != nil {
			return mapErr(err)
		}
		if err := q.InsertRolePermission(ctx, db.InsertRolePermissionParams{RoleID: roleID, PermissionID: perm.ID}); err != nil {
			return err
		}
	}
	return nil
}

func optText(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}
