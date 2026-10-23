-- Central panel (TEC-892): per-branch and total counts of the branches the
-- caller sees (branch scope, narrowed by the X-Branch header).

-- name: CountMembersByBranch :many
SELECT b.uuid AS branch_uuid, COUNT(om.id)::bigint AS member_count
FROM branches b
LEFT JOIN organization_members om
  ON om.organization_id = b.organization_id
 AND EXISTS (SELECT 1 FROM users u WHERE u.id = om.user_id AND u.deleted_at IS NULL)
 AND (
    om.branch_scope = 'all'
    OR EXISTS (
        SELECT 1 FROM organization_member_branches omb
        WHERE omb.organization_id = om.organization_id
          AND omb.member_id = om.id
          AND omb.branch_id = b.id
    )
 )
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.deleted_at IS NULL
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR b.id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY b.uuid;

-- name: CountMembersInBranches :one
SELECT COUNT(*)::bigint
FROM organization_members om
INNER JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL
WHERE om.organization_id = sqlc.arg(organization_id)
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR om.branch_scope = 'all'
    OR EXISTS (
        SELECT 1 FROM organization_member_branches omb
        WHERE omb.organization_id = om.organization_id
          AND omb.member_id = om.id
          AND omb.branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    )
  );
