-- Tenant resource: branches and service points.
-- Every query filters by organization_id (see docs/design/tenant-core.md §8).

-- name: CreateBranch :one
INSERT INTO branches (
    organization_id, code, name, address, city, district, latitude, longitude,
    phone, email, working_hours, holiday_exceptions, prep_minutes, is_active,
    slug, description, timezone, show_online
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(code), sqlc.arg(name), sqlc.arg(address),
    sqlc.arg(city), sqlc.arg(district), sqlc.narg(latitude), sqlc.narg(longitude),
    sqlc.arg(phone), sqlc.arg(email), sqlc.arg(working_hours), sqlc.arg(holiday_exceptions),
    sqlc.arg(prep_minutes), sqlc.arg(is_active),
    sqlc.arg(slug), sqlc.arg(description), sqlc.narg(timezone), sqlc.arg(show_online)
)
RETURNING *;

-- name: GetBranchByUUID :one
SELECT * FROM branches
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: GetBranchByCode :one
SELECT * FROM branches
WHERE organization_id = sqlc.arg(organization_id)
  AND code = sqlc.arg(code)
  AND deleted_at IS NULL;

-- name: BranchCodeTaken :one
SELECT EXISTS (
    SELECT 1 FROM branches
    WHERE organization_id = sqlc.arg(organization_id)
      AND code = sqlc.arg(code)
      AND deleted_at IS NULL
      AND (sqlc.narg(exclude_uuid)::uuid IS NULL OR uuid <> sqlc.narg(exclude_uuid))
)::boolean;

-- name: BranchSlugTaken :one
SELECT EXISTS (
    SELECT 1 FROM branches
    WHERE organization_id = sqlc.arg(organization_id)
      AND slug = sqlc.arg(slug)
      AND deleted_at IS NULL
      AND (sqlc.narg(exclude_uuid)::uuid IS NULL OR uuid <> sqlc.narg(exclude_uuid))
)::boolean;

-- name: ListBranches :many
SELECT
    b.*,
    (
        SELECT COUNT(*) FROM service_points sp
        WHERE sp.organization_id = b.organization_id AND sp.branch_id = b.id
    )::bigint AS service_points_count
FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.deleted_at IS NULL
  -- Branch scope of the caller: NULL = every branch.
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR b.id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(is_active)::boolean IS NULL OR b.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(cities)::text[] IS NULL OR b.city = ANY(sqlc.narg(cities)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR b.code ILIKE '%' || sqlc.narg(q) || '%'
    OR b.name ILIKE '%' || sqlc.narg(q) || '%'
    OR b.slug ILIKE '%' || sqlc.narg(q) || '%'
    OR b.city ILIKE '%' || sqlc.narg(q) || '%'
    OR b.district ILIKE '%' || sqlc.narg(q) || '%'
    OR b.phone ILIKE '%' || sqlc.narg(q) || '%'
    OR b.email ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'code' AND NOT sqlc.arg(sort_desc)::boolean THEN b.code END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'code' AND sqlc.arg(sort_desc)::boolean THEN b.code END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN b.name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN b.name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'city' AND NOT sqlc.arg(sort_desc)::boolean THEN b.city END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'city' AND sqlc.arg(sort_desc)::boolean THEN b.city END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'prep_minutes' AND NOT sqlc.arg(sort_desc)::boolean THEN b.prep_minutes END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'prep_minutes' AND sqlc.arg(sort_desc)::boolean THEN b.prep_minutes END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'is_active' AND NOT sqlc.arg(sort_desc)::boolean THEN b.is_active END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'is_active' AND sqlc.arg(sort_desc)::boolean THEN b.is_active END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN b.updated_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN b.updated_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN b.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN b.created_at END DESC,
  b.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountBranches :one
SELECT COUNT(*)::bigint
FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.deleted_at IS NULL
  -- Branch scope of the caller: NULL = every branch.
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR b.id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(is_active)::boolean IS NULL OR b.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(cities)::text[] IS NULL OR b.city = ANY(sqlc.narg(cities)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR b.code ILIKE '%' || sqlc.narg(q) || '%'
    OR b.name ILIKE '%' || sqlc.narg(q) || '%'
    OR b.slug ILIKE '%' || sqlc.narg(q) || '%'
    OR b.city ILIKE '%' || sqlc.narg(q) || '%'
    OR b.district ILIKE '%' || sqlc.narg(q) || '%'
    OR b.phone ILIKE '%' || sqlc.narg(q) || '%'
    OR b.email ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListBranchCityFacets :many
SELECT city, COUNT(*)::bigint AS count
FROM branches
WHERE organization_id = sqlc.arg(organization_id)
  AND deleted_at IS NULL
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND city <> ''
GROUP BY city
ORDER BY city ASC;

-- name: ListBranchUUIDs :many
SELECT uuid
FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.deleted_at IS NULL
  -- Branch scope of the caller: NULL = every branch.
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR b.id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(is_active)::boolean IS NULL OR b.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(cities)::text[] IS NULL OR b.city = ANY(sqlc.narg(cities)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR b.code ILIKE '%' || sqlc.narg(q) || '%'
    OR b.name ILIKE '%' || sqlc.narg(q) || '%'
    OR b.slug ILIKE '%' || sqlc.narg(q) || '%'
    OR b.city ILIKE '%' || sqlc.narg(q) || '%'
    OR b.district ILIKE '%' || sqlc.narg(q) || '%'
    OR b.phone ILIKE '%' || sqlc.narg(q) || '%'
    OR b.email ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY b.id ASC
LIMIT sqlc.arg(limit_count);

-- name: ListBranchesForExport :many
SELECT *
FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.deleted_at IS NULL
  -- Branch scope of the caller: NULL = every branch.
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR b.id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(is_active)::boolean IS NULL OR b.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(cities)::text[] IS NULL OR b.city = ANY(sqlc.narg(cities)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR b.code ILIKE '%' || sqlc.narg(q) || '%'
    OR b.name ILIKE '%' || sqlc.narg(q) || '%'
    OR b.slug ILIKE '%' || sqlc.narg(q) || '%'
    OR b.city ILIKE '%' || sqlc.narg(q) || '%'
    OR b.district ILIKE '%' || sqlc.narg(q) || '%'
    OR b.phone ILIKE '%' || sqlc.narg(q) || '%'
    OR b.email ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY b.code ASC, b.id ASC;

-- name: UpdateBranch :one
UPDATE branches
SET code = sqlc.arg(code),
    name = sqlc.arg(name),
    address = sqlc.arg(address),
    city = sqlc.arg(city),
    district = sqlc.arg(district),
    latitude = sqlc.narg(latitude),
    longitude = sqlc.narg(longitude),
    phone = sqlc.arg(phone),
    email = sqlc.arg(email),
    working_hours = sqlc.arg(working_hours),
    holiday_exceptions = sqlc.arg(holiday_exceptions),
    prep_minutes = sqlc.arg(prep_minutes),
    is_active = sqlc.arg(is_active),
    slug = sqlc.arg(slug),
    description = sqlc.arg(description),
    timezone = sqlc.narg(timezone),
    show_online = sqlc.arg(show_online)
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL
RETURNING *;

-- name: SetBranchActive :one
UPDATE branches
SET is_active = sqlc.arg(is_active)
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteBranch :execrows
UPDATE branches
SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND deleted_at IS NULL;

-- name: GetBranchSearchDocument :one
SELECT b.id, b.uuid, b.code, b.name, b.city, b.district, b.is_active, b.organization_id, o.slug AS organization_slug
FROM branches b
JOIN organizations o ON o.id = b.organization_id
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.uuid = sqlc.arg(uuid)
  AND b.deleted_at IS NULL;

-- name: ListBranchSearchDocuments :many
SELECT b.id, b.uuid, b.code, b.name, b.city, b.district, b.is_active, b.organization_id, o.slug AS organization_slug
FROM branches b
JOIN organizations o ON o.id = b.organization_id
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.deleted_at IS NULL
ORDER BY b.id ASC;

-- name: ListServicePointsByBranch :many
SELECT * FROM service_points
WHERE organization_id = sqlc.arg(organization_id)
  AND branch_id = sqlc.arg(branch_id)
ORDER BY created_at ASC, id ASC;

-- name: CreateServicePoint :one
INSERT INTO service_points (
    organization_id, branch_id, type, name, description, extra_fee,
    working_hours, online_selectable, is_active
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(branch_id), sqlc.arg(type), sqlc.arg(name),
    sqlc.arg(description), sqlc.arg(extra_fee), sqlc.narg(working_hours),
    sqlc.arg(online_selectable), sqlc.arg(is_active)
)
RETURNING *;

-- name: GetServicePoint :one
SELECT * FROM service_points
WHERE organization_id = sqlc.arg(organization_id)
  AND branch_id = sqlc.arg(branch_id)
  AND uuid = sqlc.arg(uuid);

-- name: UpdateServicePoint :one
UPDATE service_points
SET type = sqlc.arg(type),
    name = sqlc.arg(name),
    description = sqlc.arg(description),
    extra_fee = sqlc.arg(extra_fee),
    working_hours = sqlc.narg(working_hours),
    online_selectable = sqlc.arg(online_selectable),
    is_active = sqlc.arg(is_active)
WHERE organization_id = sqlc.arg(organization_id)
  AND branch_id = sqlc.arg(branch_id)
  AND uuid = sqlc.arg(uuid)
RETURNING *;

-- name: DeleteServicePoint :execrows
DELETE FROM service_points
WHERE organization_id = sqlc.arg(organization_id)
  AND branch_id = sqlc.arg(branch_id)
  AND uuid = sqlc.arg(uuid);

-- name: ListBranchImages :many
SELECT * FROM branch_images
WHERE organization_id = sqlc.arg(organization_id)
  AND branch_id = ANY(sqlc.arg(branch_ids)::bigint[])
ORDER BY branch_id, sort_order ASC, id ASC;

-- name: CountBranchImages :one
SELECT COUNT(*)::bigint FROM branch_images
WHERE organization_id = sqlc.arg(organization_id)
  AND branch_id = sqlc.arg(branch_id);

-- name: CreateBranchImage :one
INSERT INTO branch_images (
    uuid, organization_id, branch_id, object_key, filename, content_type, size_bytes, sort_order
) VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.arg(branch_id), sqlc.arg(object_key),
    sqlc.arg(filename), sqlc.arg(content_type), sqlc.arg(size_bytes),
    COALESCE((
        SELECT MAX(sort_order) + 1 FROM branch_images
        WHERE organization_id = sqlc.arg(organization_id) AND branch_id = sqlc.arg(branch_id)
    ), 0)
)
RETURNING *;

-- name: GetBranchImage :one
SELECT * FROM branch_images
WHERE organization_id = sqlc.arg(organization_id)
  AND branch_id = sqlc.arg(branch_id)
  AND uuid = sqlc.arg(uuid);

-- name: DeleteBranchImage :execrows
DELETE FROM branch_images
WHERE organization_id = sqlc.arg(organization_id)
  AND branch_id = sqlc.arg(branch_id)
  AND uuid = sqlc.arg(uuid);

-- name: SetBranchImageSort :execrows
UPDATE branch_images
SET sort_order = sqlc.arg(sort_order)
WHERE organization_id = sqlc.arg(organization_id)
  AND branch_id = sqlc.arg(branch_id)
  AND uuid = sqlc.arg(uuid);

-- name: TouchBranch :exec
UPDATE branches
SET updated_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: GetPublicBranchImage :one
-- Public media route: the image of a live branch of an active organization.
SELECT i.object_key, i.content_type, i.size_bytes
FROM branch_images i
JOIN branches b ON b.organization_id = i.organization_id AND b.id = i.branch_id
JOIN organizations o ON o.id = i.organization_id
WHERE o.uuid = sqlc.arg(organization_uuid)
  AND i.organization_id = o.id
  AND i.uuid = sqlc.arg(uuid)
  AND b.deleted_at IS NULL
  AND o.deleted_at IS NULL;
