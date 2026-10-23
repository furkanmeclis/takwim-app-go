-- Staff invitations (TEC-671). Organization-side queries are scoped by
-- organization_id; the public token lookups match the sha256 token hash only.

-- name: CreateInvitation :one
INSERT INTO organization_invitations (
    organization_id, email, role_id, token_hash, expires_at, invited_by, branch_scope
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(email), sqlc.arg(role_id), sqlc.arg(token_hash),
    sqlc.arg(expires_at), sqlc.narg(invited_by), sqlc.arg(branch_scope)
)
RETURNING *;

-- name: AddInvitationRole :exec
INSERT INTO organization_invitation_roles (organization_id, invitation_id, role_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(invitation_id), sqlc.arg(role_id))
ON CONFLICT DO NOTHING;

-- name: AddInvitationBranch :exec
INSERT INTO organization_invitation_branches (organization_id, invitation_id, branch_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(invitation_id), sqlc.arg(branch_id))
ON CONFLICT DO NOTHING;

-- name: RevokeOpenInvitationsForEmail :execrows
-- Re-inviting an e-mail replaces its open (possibly expired) invitation.
UPDATE organization_invitations
SET revoked_at = NOW(), revoked_by = sqlc.narg(revoked_by)
WHERE organization_id = sqlc.arg(organization_id)
  AND email = sqlc.arg(email)
  AND accepted_at IS NULL AND revoked_at IS NULL;

-- name: GetInvitationByUUID :one
SELECT * FROM organization_invitations
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: GetInvitationPreviewByTokenHash :one
SELECT oi.id, oi.uuid, oi.organization_id, oi.email::text AS email, oi.expires_at,
       oi.accepted_at, oi.revoked_at, oi.created_at,
       o.uuid AS organization_uuid, o.slug AS organization_slug, o.name AS organization_name,
       o.logo_object_key, o.updated_at AS organization_updated_at,
       COALESCE(inv.name, '')::text AS inviter_name,
       COALESCE(inv.surname, '')::text AS inviter_surname
FROM organization_invitations oi
INNER JOIN organizations o ON o.id = oi.organization_id AND o.deleted_at IS NULL
LEFT JOIN users inv ON inv.id = oi.invited_by
WHERE oi.token_hash = sqlc.arg(token_hash);

-- name: LockInvitationByTokenHash :one
SELECT oi.*
FROM organization_invitations oi
INNER JOIN organizations o ON o.id = oi.organization_id AND o.deleted_at IS NULL
WHERE oi.token_hash = sqlc.arg(token_hash)
FOR UPDATE OF oi;

-- name: ListInvitations :many
-- status: 'pending' (open, not expired), 'expired' (open, past expires_at),
-- 'accepted', 'revoked'.
SELECT oi.id, oi.uuid, oi.email::text AS email, oi.branch_scope, oi.expires_at,
       oi.accepted_at, oi.revoked_at, oi.last_sent_at, oi.send_count, oi.created_at,
       (CASE
          WHEN oi.revoked_at IS NOT NULL THEN 'revoked'
          WHEN oi.accepted_at IS NOT NULL THEN 'accepted'
          WHEN oi.expires_at <= NOW() THEN 'expired'
          ELSE 'pending'
        END)::text AS status,
       inv.uuid AS inviter_uuid,
       COALESCE(inv.email, '')::text AS inviter_email,
       COALESCE(inv.name, '')::text AS inviter_name,
       COALESCE(inv.surname, '')::text AS inviter_surname
FROM organization_invitations oi
LEFT JOIN users inv ON inv.id = oi.invited_by
WHERE oi.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(q)::text IS NULL OR oi.email ILIKE '%' || sqlc.narg(q) || '%')
  AND (
    sqlc.narg(statuses)::text[] IS NULL
    OR (CASE
          WHEN oi.revoked_at IS NOT NULL THEN 'revoked'
          WHEN oi.accepted_at IS NOT NULL THEN 'accepted'
          WHEN oi.expires_at <= NOW() THEN 'expired'
          ELSE 'pending'
        END) = ANY(sqlc.narg(statuses)::text[])
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'email' AND NOT sqlc.arg(sort_desc)::boolean THEN oi.email END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'email' AND sqlc.arg(sort_desc)::boolean THEN oi.email END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'expires_at' AND NOT sqlc.arg(sort_desc)::boolean THEN oi.expires_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'expires_at' AND sqlc.arg(sort_desc)::boolean THEN oi.expires_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN oi.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN oi.created_at END DESC,
  oi.created_at DESC, oi.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountInvitations :one
SELECT COUNT(*)::bigint
FROM organization_invitations oi
WHERE oi.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(q)::text IS NULL OR oi.email ILIKE '%' || sqlc.narg(q) || '%')
  AND (
    sqlc.narg(statuses)::text[] IS NULL
    OR (CASE
          WHEN oi.revoked_at IS NOT NULL THEN 'revoked'
          WHEN oi.accepted_at IS NOT NULL THEN 'accepted'
          WHEN oi.expires_at <= NOW() THEN 'expired'
          ELSE 'pending'
        END) = ANY(sqlc.narg(statuses)::text[])
  );

-- name: ListInvitationRoles :many
SELECT oir.invitation_id, r.id, r.uuid, r.slug, r.name,
       (r.organization_id IS NULL)::boolean AS is_template
FROM organization_invitation_roles oir
INNER JOIN roles r ON r.id = oir.role_id AND r.scope = 'tenant'
WHERE oir.organization_id = sqlc.arg(organization_id)
  AND oir.invitation_id = ANY(sqlc.arg(invitation_ids)::bigint[])
ORDER BY oir.invitation_id, r.organization_id NULLS FIRST, r.id;

-- name: ListInvitationBranches :many
SELECT oib.invitation_id, b.id, b.uuid, b.code, b.name
FROM organization_invitation_branches oib
INNER JOIN branches b
    ON b.organization_id = oib.organization_id AND b.id = oib.branch_id AND b.deleted_at IS NULL
WHERE oib.organization_id = sqlc.arg(organization_id)
  AND oib.invitation_id = ANY(sqlc.arg(invitation_ids)::bigint[])
ORDER BY oib.invitation_id, b.code;

-- name: RevokeInvitation :one
UPDATE organization_invitations
SET revoked_at = NOW(), revoked_by = sqlc.narg(revoked_by)
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND accepted_at IS NULL AND revoked_at IS NULL
RETURNING *;

-- name: RotateInvitationToken :one
-- Resend: a new token (the old link stops working) and a fresh expiry.
UPDATE organization_invitations
SET token_hash = sqlc.arg(token_hash),
    expires_at = sqlc.arg(expires_at),
    last_sent_at = NOW(),
    send_count = send_count + 1
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND accepted_at IS NULL AND revoked_at IS NULL
RETURNING *;

-- name: MarkInvitationAccepted :execrows
UPDATE organization_invitations
SET accepted_at = NOW(), accepted_by = sqlc.arg(accepted_by)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND accepted_at IS NULL AND revoked_at IS NULL;
