-- Versioned organization settings. Rows are immutable; the current settings
-- are the highest version of the organization.

-- name: GetCurrentOrganizationSettings :one
SELECT s.id, s.organization_id, s.version, s.data, s.created_at,
       u.uuid AS created_by_uuid, u.name AS created_by_name,
       u.surname AS created_by_surname, u.email AS created_by_email
FROM organization_settings_versions s
LEFT JOIN users u ON u.id = s.created_by
WHERE s.organization_id = sqlc.arg(organization_id)
ORDER BY s.version DESC
LIMIT 1;

-- name: GetOrganizationSettingsVersion :one
SELECT s.id, s.organization_id, s.version, s.data, s.created_at,
       u.uuid AS created_by_uuid, u.name AS created_by_name,
       u.surname AS created_by_surname, u.email AS created_by_email
FROM organization_settings_versions s
LEFT JOIN users u ON u.id = s.created_by
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.version = sqlc.arg(version);

-- name: ListOrganizationSettingsVersions :many
SELECT s.id, s.organization_id, s.version, s.data, s.created_at,
       u.uuid AS created_by_uuid, u.name AS created_by_name,
       u.surname AS created_by_surname, u.email AS created_by_email
FROM organization_settings_versions s
LEFT JOIN users u ON u.id = s.created_by
WHERE s.organization_id = sqlc.arg(organization_id)
ORDER BY
  CASE WHEN sqlc.arg(sort_desc)::boolean THEN s.version END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::boolean THEN s.version END ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountOrganizationSettingsVersions :one
SELECT COUNT(*)::bigint
FROM organization_settings_versions
WHERE organization_id = sqlc.arg(organization_id);

-- name: LatestOrganizationSettingsVersion :one
SELECT COALESCE(MAX(version), 0)::integer
FROM organization_settings_versions
WHERE organization_id = sqlc.arg(organization_id);

-- name: InsertOrganizationSettingsVersion :one
INSERT INTO organization_settings_versions (organization_id, version, data, created_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(version), sqlc.arg(data), sqlc.narg(created_by))
RETURNING id, organization_id, version, data, created_by, created_at;
