-- Tenant resource: resources (TEC-891). Every query filters by organization_id.

-- name: CreateResource :one
INSERT INTO resources (
    organization_id, branch_id, kind, name, zone, description, min_capacity, capacity,
    combinable, show_online, is_active, sort_order, working_hours
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(branch_id), sqlc.arg(kind), sqlc.arg(name), sqlc.arg(zone),
    sqlc.arg(description), sqlc.arg(min_capacity), sqlc.arg(capacity), sqlc.arg(combinable),
    sqlc.arg(show_online), sqlc.arg(is_active), sqlc.arg(sort_order), sqlc.narg(working_hours)
)
RETURNING *;

-- name: UpdateResource :one
UPDATE resources
SET branch_id = sqlc.arg(branch_id), kind = sqlc.arg(kind), name = sqlc.arg(name), zone = sqlc.arg(zone),
    description = sqlc.arg(description), min_capacity = sqlc.arg(min_capacity), capacity = sqlc.arg(capacity),
    combinable = sqlc.arg(combinable), show_online = sqlc.arg(show_online), is_active = sqlc.arg(is_active),
    sort_order = sqlc.arg(sort_order), working_hours = sqlc.narg(working_hours)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: SetResourceActive :exec
UPDATE resources SET is_active = sqlc.arg(is_active)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: SoftDeleteResource :execrows
UPDATE resources SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: GetResource :one
SELECT
    r.*,
    b.uuid AS branch_uuid, b.name AS branch_name, b.code AS branch_code
FROM resources r
JOIN branches b ON b.id = r.branch_id AND b.organization_id = r.organization_id AND b.deleted_at IS NULL
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.uuid = sqlc.arg(uuid)
  AND r.deleted_at IS NULL;

-- name: ListResources :many
SELECT
    r.*,
    b.uuid AS branch_uuid, b.name AS branch_name, b.code AS branch_code
FROM resources r
JOIN branches b ON b.id = r.branch_id AND b.organization_id = r.organization_id AND b.deleted_at IS NULL
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.deleted_at IS NULL
  -- Branch scope of the caller (NULL = every branch).
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  -- Faceted filters.
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR r.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(zones)::text[] IS NULL OR r.zone = ANY(sqlc.narg(zones)::text[]))
  AND (sqlc.narg(is_active)::boolean IS NULL OR r.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(show_online)::boolean IS NULL OR r.show_online = sqlc.narg(show_online))
  AND (
    sqlc.narg(q)::text IS NULL
    OR r.name ILIKE '%' || sqlc.narg(q) || '%'
    OR r.zone ILIKE '%' || sqlc.narg(q) || '%'
    OR r.description ILIKE '%' || sqlc.narg(q) || '%'
    OR b.name ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN r.name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN r.name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'kind' AND NOT sqlc.arg(sort_desc)::boolean THEN r.kind END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'kind' AND sqlc.arg(sort_desc)::boolean THEN r.kind END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'zone' AND NOT sqlc.arg(sort_desc)::boolean THEN r.zone END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'zone' AND sqlc.arg(sort_desc)::boolean THEN r.zone END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'capacity' AND NOT sqlc.arg(sort_desc)::boolean THEN r.capacity END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'capacity' AND sqlc.arg(sort_desc)::boolean THEN r.capacity END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'sort_order' AND NOT sqlc.arg(sort_desc)::boolean THEN r.sort_order END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'sort_order' AND sqlc.arg(sort_desc)::boolean THEN r.sort_order END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'is_active' AND NOT sqlc.arg(sort_desc)::boolean THEN r.is_active END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'is_active' AND sqlc.arg(sort_desc)::boolean THEN r.is_active END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN r.updated_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN r.updated_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN r.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN r.created_at END DESC,
  b.name ASC, r.sort_order ASC, r.name ASC, r.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountResources :one
SELECT COUNT(*)::bigint
FROM resources r
JOIN branches b ON b.id = r.branch_id AND b.organization_id = r.organization_id AND b.deleted_at IS NULL
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.deleted_at IS NULL
  -- Branch scope of the caller (NULL = every branch).
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  -- Faceted filters.
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR r.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(zones)::text[] IS NULL OR r.zone = ANY(sqlc.narg(zones)::text[]))
  AND (sqlc.narg(is_active)::boolean IS NULL OR r.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(show_online)::boolean IS NULL OR r.show_online = sqlc.narg(show_online))
  AND (
    sqlc.narg(q)::text IS NULL
    OR r.name ILIKE '%' || sqlc.narg(q) || '%'
    OR r.zone ILIKE '%' || sqlc.narg(q) || '%'
    OR r.description ILIKE '%' || sqlc.narg(q) || '%'
    OR b.name ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListResourceUUIDs :many
SELECT r.uuid
FROM resources r
JOIN branches b ON b.id = r.branch_id AND b.organization_id = r.organization_id AND b.deleted_at IS NULL
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.deleted_at IS NULL
  -- Branch scope of the caller (NULL = every branch).
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  -- Faceted filters.
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR r.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(zones)::text[] IS NULL OR r.zone = ANY(sqlc.narg(zones)::text[]))
  AND (sqlc.narg(is_active)::boolean IS NULL OR r.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(show_online)::boolean IS NULL OR r.show_online = sqlc.narg(show_online))
  AND (
    sqlc.narg(q)::text IS NULL
    OR r.name ILIKE '%' || sqlc.narg(q) || '%'
    OR r.zone ILIKE '%' || sqlc.narg(q) || '%'
    OR r.description ILIKE '%' || sqlc.narg(q) || '%'
    OR b.name ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY r.id ASC
LIMIT sqlc.arg(limit_count);

-- name: ListResourcesForExport :many
SELECT
    r.*,
    b.uuid AS branch_uuid, b.name AS branch_name, b.code AS branch_code
FROM resources r
JOIN branches b ON b.id = r.branch_id AND b.organization_id = r.organization_id AND b.deleted_at IS NULL
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.deleted_at IS NULL
  -- Branch scope of the caller (NULL = every branch).
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  -- Faceted filters.
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR r.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(zones)::text[] IS NULL OR r.zone = ANY(sqlc.narg(zones)::text[]))
  AND (sqlc.narg(is_active)::boolean IS NULL OR r.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(show_online)::boolean IS NULL OR r.show_online = sqlc.narg(show_online))
  AND (
    sqlc.narg(q)::text IS NULL
    OR r.name ILIKE '%' || sqlc.narg(q) || '%'
    OR r.zone ILIKE '%' || sqlc.narg(q) || '%'
    OR r.description ILIKE '%' || sqlc.narg(q) || '%'
    OR b.name ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY b.name ASC, r.sort_order ASC, r.name ASC, r.id ASC;

-- name: ListResourceBranchFacets :many
-- Branches with their resource count, inside the caller's branch scope.
SELECT b.uuid, b.name, COUNT(*)::bigint AS count
FROM resources r
JOIN branches b ON b.id = r.branch_id AND b.organization_id = r.organization_id AND b.deleted_at IS NULL
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.deleted_at IS NULL
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY b.uuid, b.name
ORDER BY b.name ASC;

-- name: ListResourceKindFacets :many
SELECT r.kind AS value, COUNT(*)::bigint AS count
FROM resources r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.deleted_at IS NULL
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY r.kind
ORDER BY r.kind ASC;

-- name: ListResourceZoneFacets :many
SELECT r.zone AS value, COUNT(*)::bigint AS count
FROM resources r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.deleted_at IS NULL
  AND r.zone <> ''
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY r.zone
ORDER BY r.zone ASC;

-- name: ResourceNameTaken :one
SELECT EXISTS (
    SELECT 1 FROM resources
    WHERE organization_id = sqlc.arg(organization_id)
      AND branch_id = sqlc.arg(branch_id)
      AND lower(name) = lower(sqlc.arg(name))
      AND deleted_at IS NULL
      AND (sqlc.narg(exclude_id)::bigint IS NULL OR id <> sqlc.narg(exclude_id))
)::boolean;

-- name: GetResourceByBranchName :one
-- Import key: a resource of the branch with the same name.
SELECT id, uuid FROM resources
WHERE organization_id = sqlc.arg(organization_id)
  AND branch_id = sqlc.arg(branch_id)
  AND lower(name) = lower(sqlc.arg(name))
  AND deleted_at IS NULL;

-- name: ListResourceClosures :many
SELECT * FROM resource_closures
WHERE organization_id = sqlc.arg(organization_id)
  AND resource_id = sqlc.arg(resource_id)
  AND (sqlc.narg(range_from)::timestamptz IS NULL OR ends_at > sqlc.narg(range_from))
  AND (sqlc.narg(range_to)::timestamptz IS NULL OR starts_at < sqlc.narg(range_to))
ORDER BY starts_at DESC, id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountResourceClosures :one
SELECT COUNT(*)::bigint FROM resource_closures
WHERE organization_id = sqlc.arg(organization_id)
  AND resource_id = sqlc.arg(resource_id)
  AND (sqlc.narg(range_from)::timestamptz IS NULL OR ends_at > sqlc.narg(range_from))
  AND (sqlc.narg(range_to)::timestamptz IS NULL OR starts_at < sqlc.narg(range_to));

-- name: GetResourceClosure :one
SELECT * FROM resource_closures
WHERE organization_id = sqlc.arg(organization_id)
  AND resource_id = sqlc.arg(resource_id)
  AND uuid = sqlc.arg(uuid);

-- name: CreateResourceClosure :one
INSERT INTO resource_closures (organization_id, resource_id, starts_at, ends_at, reason, created_by)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(resource_id), sqlc.arg(starts_at), sqlc.arg(ends_at),
    sqlc.arg(reason), sqlc.narg(created_by)
)
RETURNING *;

-- name: UpdateResourceClosure :one
UPDATE resource_closures
SET starts_at = sqlc.arg(starts_at), ends_at = sqlc.arg(ends_at), reason = sqlc.arg(reason)
WHERE organization_id = sqlc.arg(organization_id)
  AND resource_id = sqlc.arg(resource_id)
  AND uuid = sqlc.arg(uuid)
RETURNING *;

-- name: DeleteResourceClosure :execrows
DELETE FROM resource_closures
WHERE organization_id = sqlc.arg(organization_id)
  AND resource_id = sqlc.arg(resource_id)
  AND uuid = sqlc.arg(uuid);

-- name: ListResourceSearchDocuments :many
SELECT r.id, r.uuid, r.organization_id, r.branch_id, r.kind, r.name, r.zone, b.name AS branch_name,
    o.slug AS organization_slug
FROM resources r
JOIN branches b ON b.id = r.branch_id AND b.organization_id = r.organization_id
JOIN organizations o ON o.id = r.organization_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.deleted_at IS NULL;

-- name: GetResourceSearchDocument :one
SELECT r.id, r.uuid, r.organization_id, r.branch_id, r.kind, r.name, r.zone, b.name AS branch_name,
    o.slug AS organization_slug
FROM resources r
JOIN branches b ON b.id = r.branch_id AND b.organization_id = r.organization_id
JOIN organizations o ON o.id = r.organization_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.uuid = sqlc.arg(uuid)
  AND r.deleted_at IS NULL;
