-- name: CreateOrganization :one
INSERT INTO organizations (
    slug, name, logo_object_key, status, access_starts_at, access_ends_at, settings
) VALUES (
    $1, $2, sqlc.narg(logo_object_key), COALESCE(sqlc.narg(status), 'active'),
    COALESCE(sqlc.narg(access_starts_at), NOW()), sqlc.narg(access_ends_at),
    COALESCE(sqlc.narg(settings), '{}'::jsonb)
)
RETURNING *;

-- name: GetOrganizationByUUID :one
SELECT * FROM organizations
WHERE uuid = $1 AND deleted_at IS NULL;

-- name: GetOrganizationBySlug :one
SELECT * FROM organizations
WHERE slug = $1 AND deleted_at IS NULL;

-- name: GetOrganizationByID :one
SELECT * FROM organizations
WHERE id = $1 AND deleted_at IS NULL;

-- name: SlugExists :one
SELECT EXISTS(
    SELECT 1 FROM organizations WHERE slug = $1 AND deleted_at IS NULL
)::boolean AS exists;

-- name: ListOrganizationsFiltered :many
SELECT *
FROM organizations
WHERE deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (
    sqlc.narg(q)::text IS NULL
    OR name ILIKE '%' || sqlc.narg(q) || '%'
    OR slug ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'slug' AND NOT sqlc.arg(sort_desc)::boolean THEN slug END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'slug' AND sqlc.arg(sort_desc)::boolean THEN slug END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND NOT sqlc.arg(sort_desc)::boolean THEN status END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND sqlc.arg(sort_desc)::boolean THEN status END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN updated_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN updated_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN created_at END DESC,
  created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountOrganizations :one
SELECT COUNT(*)::bigint
FROM organizations
WHERE deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (
    sqlc.narg(q)::text IS NULL
    OR name ILIKE '%' || sqlc.narg(q) || '%'
    OR slug ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: UpdateOrganization :one
UPDATE organizations
SET name = COALESCE(sqlc.narg(name), name),
    slug = COALESCE(sqlc.narg(slug), slug),
    logo_object_key = COALESCE(sqlc.narg(logo_object_key), logo_object_key),
    settings = COALESCE(sqlc.narg(settings), settings)
WHERE uuid = sqlc.arg(uuid) AND deleted_at IS NULL
RETURNING *;

-- name: SetOrganizationStatus :one
UPDATE organizations
SET status = sqlc.arg(status)
WHERE uuid = sqlc.arg(uuid) AND deleted_at IS NULL
RETURNING *;

-- name: SetOrganizationAccessWindow :one
UPDATE organizations
SET access_starts_at = COALESCE(sqlc.narg(access_starts_at), access_starts_at),
    access_ends_at = sqlc.narg(access_ends_at)
WHERE uuid = sqlc.arg(uuid) AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteOrganization :one
UPDATE organizations
SET deleted_at = NOW()
WHERE uuid = $1 AND deleted_at IS NULL
RETURNING *;

-- name: LockOrganizationRow :one
SELECT * FROM organizations
WHERE id = $1 AND deleted_at IS NULL
FOR UPDATE;

-- name: GetTenantRoleByKey :one
SELECT * FROM roles
WHERE slug = sqlc.arg(role_key)
  AND scope = 'tenant'
  AND (organization_id IS NULL OR organization_id = sqlc.arg(organization_id))
ORDER BY organization_id NULLS LAST
LIMIT 1;

-- name: ListTenantRolePermissionSlugsByRoleID :many
SELECT p.slug
FROM permissions p
INNER JOIN role_permissions rp ON rp.permission_id = p.id
INNER JOIN roles r ON r.id = rp.role_id
WHERE rp.role_id = $1
  AND r.scope = 'tenant'
ORDER BY p.slug;

-- name: AddOrganizationMember :one
INSERT INTO organization_members (organization_id, user_id, role_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetOrganizationMember :one
SELECT om.*, r.slug AS role_key, r.name AS role_name
FROM organization_members om
INNER JOIN roles r ON r.id = om.role_id AND r.scope = 'tenant'
WHERE om.organization_id = $1 AND om.user_id = $2;

-- name: GetOrganizationMemberByUserUUID :one
SELECT om.id, om.organization_id, om.user_id, om.role_id, om.created_at, om.updated_at,
       r.slug AS role_key, r.name AS role_name,
       u.uuid AS user_uuid, u.email, u.name, u.surname, u.status
FROM organization_members om
INNER JOIN roles r ON r.id = om.role_id AND r.scope = 'tenant'
INNER JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL
WHERE om.organization_id = $1 AND u.uuid = $2;

-- name: ListOrganizationMembersByOrg :many
SELECT om.id, om.organization_id, om.user_id, om.role_id, om.created_at, om.updated_at,
       r.slug AS role_key, r.name AS role_name,
       u.uuid AS user_uuid, u.email, u.name, u.surname, u.status
FROM organization_members om
INNER JOIN roles r ON r.id = om.role_id AND r.scope = 'tenant'
INNER JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL
WHERE om.organization_id = sqlc.arg(organization_id)
  AND (
    sqlc.narg(q)::text IS NULL
    OR u.email ILIKE '%' || sqlc.narg(q) || '%'
    OR u.name ILIKE '%' || sqlc.narg(q) || '%'
    OR u.surname ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'email' AND NOT sqlc.arg(sort_desc)::boolean THEN u.email END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'email' AND sqlc.arg(sort_desc)::boolean THEN u.email END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN u.name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN u.name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'surname' AND NOT sqlc.arg(sort_desc)::boolean THEN u.surname END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'surname' AND sqlc.arg(sort_desc)::boolean THEN u.surname END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'role' AND NOT sqlc.arg(sort_desc)::boolean THEN r.slug END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'role' AND sqlc.arg(sort_desc)::boolean THEN r.slug END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN om.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN om.created_at END DESC,
  om.created_at ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountOrganizationMembersByOrg :one
SELECT COUNT(*)::bigint
FROM organization_members om
INNER JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL
WHERE om.organization_id = sqlc.arg(organization_id)
  AND (
    sqlc.narg(q)::text IS NULL
    OR u.email ILIKE '%' || sqlc.narg(q) || '%'
    OR u.name ILIKE '%' || sqlc.narg(q) || '%'
    OR u.surname ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListOrganizationMembershipsByUser :many
SELECT om.id, om.organization_id, om.user_id, om.role_id, om.created_at, om.updated_at,
       r.slug AS role_key, r.name AS role_name,
       o.uuid AS organization_uuid, o.slug AS organization_slug, o.name AS organization_name,
       o.logo_object_key, o.status AS organization_status, o.access_ends_at
FROM organization_members om
INNER JOIN roles r ON r.id = om.role_id AND r.scope = 'tenant'
INNER JOIN organizations o ON o.id = om.organization_id AND o.deleted_at IS NULL
WHERE om.user_id = $1
ORDER BY o.name ASC;

-- name: UpdateOrganizationMemberRole :one
UPDATE organization_members
SET role_id = $3
WHERE organization_id = $1 AND user_id = $2
RETURNING *;

-- name: DeleteOrganizationMember :execrows
DELETE FROM organization_members
WHERE organization_id = $1 AND user_id = $2;

-- name: CountOrganizationOwners :one
SELECT COUNT(*)::bigint
FROM organization_members om
INNER JOIN roles r ON r.id = om.role_id AND r.scope = 'tenant'
INNER JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL
WHERE om.organization_id = $1 AND r.slug = 'owner';

-- name: ResolveOrganizationScopeByUUID :one
SELECT o.id, o.uuid, o.slug, o.name, o.status, o.access_starts_at, o.access_ends_at,
       om.role_id, r.slug AS role_key
FROM organizations o
INNER JOIN organization_members om ON om.organization_id = o.id AND om.user_id = sqlc.arg(user_id)
INNER JOIN roles r ON r.id = om.role_id AND r.scope = 'tenant'
WHERE o.uuid = sqlc.arg(uuid) AND o.deleted_at IS NULL;

-- name: ResolveOrganizationScopeBySlug :one
SELECT o.id, o.uuid, o.slug, o.name, o.status, o.access_starts_at, o.access_ends_at,
       om.role_id, r.slug AS role_key
FROM organizations o
INNER JOIN organization_members om ON om.organization_id = o.id AND om.user_id = sqlc.arg(user_id)
INNER JOIN roles r ON r.id = om.role_id AND r.scope = 'tenant'
WHERE o.slug = sqlc.arg(slug) AND o.deleted_at IS NULL;

-- name: CreateOrganizationInvitation :one
INSERT INTO organization_invitations (
    organization_id, email, role_id, token_hash, expires_at, invited_by
) VALUES (
    $1, $2, $3, $4, $5, sqlc.narg(invited_by)
)
RETURNING *;

-- name: GetOrganizationInvitationByUUID :one
SELECT * FROM organization_invitations
WHERE uuid = $1;

-- name: ListOrganizationInvitations :many
SELECT oi.*, r.slug AS role_key, r.name AS role_name
FROM organization_invitations oi
INNER JOIN roles r ON r.id = oi.role_id AND r.scope = 'tenant'
WHERE oi.organization_id = $1
ORDER BY oi.created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);
