-- Tenant resource: services (TEC-894). Every query filters by organization_id.
-- The catalog is business-level; service_branches holds the branch offers
-- and service_staff the staff who may perform a service.

-- name: CreateServiceCategory :one
INSERT INTO service_categories (organization_id, name, description, sort_order)
VALUES (sqlc.arg(organization_id), sqlc.arg(name), sqlc.arg(description), sqlc.arg(sort_order))
RETURNING *;

-- name: UpdateServiceCategory :one
UPDATE service_categories
SET name = sqlc.arg(name), description = sqlc.arg(description), sort_order = sqlc.arg(sort_order)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteServiceCategory :execrows
UPDATE service_categories SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: UncategorizeServices :exec
UPDATE services SET category_id = NULL
WHERE organization_id = sqlc.arg(organization_id) AND category_id = sqlc.arg(category_id);

-- name: GetServiceCategory :one
SELECT * FROM service_categories
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid) AND deleted_at IS NULL;

-- name: GetServiceCategoryByName :one
SELECT * FROM service_categories
WHERE organization_id = sqlc.arg(organization_id) AND lower(name) = lower(sqlc.arg(name)::text) AND deleted_at IS NULL;

-- name: ListServiceCategories :many
SELECT
    c.*,
    (SELECT COUNT(*) FROM services s
     WHERE s.organization_id = c.organization_id AND s.category_id = c.id AND s.deleted_at IS NULL)::bigint AS services_count
FROM service_categories c
WHERE c.organization_id = sqlc.arg(organization_id) AND c.deleted_at IS NULL
ORDER BY c.sort_order ASC, c.name ASC, c.id ASC;

-- name: CreateService :one
INSERT INTO services (
    organization_id, category_id, name, description, duration_minutes, buffer_before_minutes,
    buffer_after_minutes, price, payment_mode, staff_mode, resource_kinds, show_online, is_active, sort_order
)
VALUES (
    sqlc.arg(organization_id), sqlc.narg(category_id), sqlc.arg(name), sqlc.arg(description),
    sqlc.arg(duration_minutes), sqlc.arg(buffer_before_minutes), sqlc.arg(buffer_after_minutes), sqlc.arg(price),
    sqlc.arg(payment_mode), sqlc.arg(staff_mode), sqlc.arg(resource_kinds), sqlc.arg(show_online),
    sqlc.arg(is_active), sqlc.arg(sort_order)
)
RETURNING *;

-- name: UpdateService :one
UPDATE services
SET category_id = sqlc.narg(category_id), name = sqlc.arg(name), description = sqlc.arg(description),
    duration_minutes = sqlc.arg(duration_minutes), buffer_before_minutes = sqlc.arg(buffer_before_minutes),
    buffer_after_minutes = sqlc.arg(buffer_after_minutes), price = sqlc.arg(price),
    payment_mode = sqlc.arg(payment_mode), staff_mode = sqlc.arg(staff_mode),
    resource_kinds = sqlc.arg(resource_kinds), show_online = sqlc.arg(show_online),
    is_active = sqlc.arg(is_active), sort_order = sqlc.arg(sort_order)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: SetServiceActive :exec
UPDATE services SET is_active = sqlc.arg(is_active)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: SetServiceImage :one
-- Returns the previous key so the caller can remove the old object.
WITH prev AS (
    SELECT p.id AS prev_id, p.image_key AS prev_key
    FROM services p
    WHERE p.organization_id = sqlc.arg(organization_id) AND p.id = sqlc.arg(id) AND p.deleted_at IS NULL
    FOR UPDATE
)
UPDATE services s
SET image_uuid = sqlc.narg(image_uuid), image_key = sqlc.narg(image_key),
    image_content_type = sqlc.narg(image_content_type), image_size_bytes = sqlc.narg(image_size_bytes)
FROM prev
WHERE s.organization_id = sqlc.arg(organization_id) AND s.id = prev.prev_id
RETURNING prev.prev_key AS previous_key;

-- name: GetServicePublicImage :one
SELECT s.image_key, s.image_content_type, s.image_size_bytes
FROM services s
JOIN organizations o ON o.id = s.organization_id
WHERE s.organization_id = o.id
  AND o.uuid = sqlc.arg(organization_uuid)
  AND s.image_uuid = sqlc.arg(image_uuid)
  AND s.deleted_at IS NULL;

-- name: SoftDeleteService :execrows
UPDATE services SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: GetService :one
SELECT
    s.*,
    c.uuid AS category_uuid, c.name AS category_name
FROM services s
LEFT JOIN service_categories c ON c.id = s.category_id AND c.organization_id = s.organization_id AND c.deleted_at IS NULL
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.uuid = sqlc.arg(uuid)
  AND s.deleted_at IS NULL;

-- name: GetServiceByName :one
SELECT * FROM services
WHERE organization_id = sqlc.arg(organization_id) AND lower(name) = lower(sqlc.arg(name)::text) AND deleted_at IS NULL;

-- name: ServiceNameTaken :one
SELECT EXISTS (
    SELECT 1 FROM services
    WHERE organization_id = sqlc.arg(organization_id) AND lower(name) = lower(sqlc.arg(name)::text)
      AND deleted_at IS NULL
      AND (sqlc.narg(exclude_id)::bigint IS NULL OR id <> sqlc.narg(exclude_id)::bigint)
)::boolean;

-- name: ListServices :many
SELECT
    s.*,
    c.uuid AS category_uuid, c.name AS category_name
FROM services s
LEFT JOIN service_categories c ON c.id = s.category_id AND c.organization_id = s.organization_id AND c.deleted_at IS NULL
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.deleted_at IS NULL
  -- Faceted filters.
  AND (
    (sqlc.narg(category_ids)::bigint[] IS NULL AND sqlc.narg(uncategorized)::boolean IS NULL)
    OR s.category_id = ANY(sqlc.narg(category_ids)::bigint[])
    OR (sqlc.narg(uncategorized)::boolean IS TRUE AND s.category_id IS NULL)
  )
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR EXISTS (
        SELECT 1 FROM service_branches sb
        WHERE sb.organization_id = s.organization_id AND sb.service_id = s.id
          AND sb.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])))
  AND (sqlc.narg(payment_modes)::text[] IS NULL OR s.payment_mode = ANY(sqlc.narg(payment_modes)::text[]))
  AND (sqlc.narg(is_active)::boolean IS NULL OR s.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(show_online)::boolean IS NULL OR s.show_online = sqlc.narg(show_online))
  AND (
    sqlc.narg(q)::text IS NULL
    OR s.name ILIKE '%' || sqlc.narg(q) || '%'
    OR s.description ILIKE '%' || sqlc.narg(q) || '%'
    OR c.name ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN s.name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN s.name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'duration' AND NOT sqlc.arg(sort_desc)::boolean THEN s.duration_minutes END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'duration' AND sqlc.arg(sort_desc)::boolean THEN s.duration_minutes END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'price' AND NOT sqlc.arg(sort_desc)::boolean THEN s.price END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'price' AND sqlc.arg(sort_desc)::boolean THEN s.price END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'sort_order' AND NOT sqlc.arg(sort_desc)::boolean THEN s.sort_order END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'sort_order' AND sqlc.arg(sort_desc)::boolean THEN s.sort_order END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'is_active' AND NOT sqlc.arg(sort_desc)::boolean THEN s.is_active END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'is_active' AND sqlc.arg(sort_desc)::boolean THEN s.is_active END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN s.updated_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN s.updated_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN s.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN s.created_at END DESC,
  c.sort_order ASC NULLS LAST, c.name ASC NULLS LAST, s.sort_order ASC, s.name ASC, s.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountServices :one
SELECT COUNT(*)::bigint
FROM services s
LEFT JOIN service_categories c ON c.id = s.category_id AND c.organization_id = s.organization_id AND c.deleted_at IS NULL
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.deleted_at IS NULL
  -- Faceted filters.
  AND (
    (sqlc.narg(category_ids)::bigint[] IS NULL AND sqlc.narg(uncategorized)::boolean IS NULL)
    OR s.category_id = ANY(sqlc.narg(category_ids)::bigint[])
    OR (sqlc.narg(uncategorized)::boolean IS TRUE AND s.category_id IS NULL)
  )
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR EXISTS (
        SELECT 1 FROM service_branches sb
        WHERE sb.organization_id = s.organization_id AND sb.service_id = s.id
          AND sb.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])))
  AND (sqlc.narg(payment_modes)::text[] IS NULL OR s.payment_mode = ANY(sqlc.narg(payment_modes)::text[]))
  AND (sqlc.narg(is_active)::boolean IS NULL OR s.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(show_online)::boolean IS NULL OR s.show_online = sqlc.narg(show_online))
  AND (
    sqlc.narg(q)::text IS NULL
    OR s.name ILIKE '%' || sqlc.narg(q) || '%'
    OR s.description ILIKE '%' || sqlc.narg(q) || '%'
    OR c.name ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListServiceUUIDs :many
SELECT s.uuid
FROM services s
LEFT JOIN service_categories c ON c.id = s.category_id AND c.organization_id = s.organization_id AND c.deleted_at IS NULL
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.deleted_at IS NULL
  -- Faceted filters.
  AND (
    (sqlc.narg(category_ids)::bigint[] IS NULL AND sqlc.narg(uncategorized)::boolean IS NULL)
    OR s.category_id = ANY(sqlc.narg(category_ids)::bigint[])
    OR (sqlc.narg(uncategorized)::boolean IS TRUE AND s.category_id IS NULL)
  )
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR EXISTS (
        SELECT 1 FROM service_branches sb
        WHERE sb.organization_id = s.organization_id AND sb.service_id = s.id
          AND sb.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])))
  AND (sqlc.narg(payment_modes)::text[] IS NULL OR s.payment_mode = ANY(sqlc.narg(payment_modes)::text[]))
  AND (sqlc.narg(is_active)::boolean IS NULL OR s.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(show_online)::boolean IS NULL OR s.show_online = sqlc.narg(show_online))
  AND (
    sqlc.narg(q)::text IS NULL
    OR s.name ILIKE '%' || sqlc.narg(q) || '%'
    OR s.description ILIKE '%' || sqlc.narg(q) || '%'
    OR c.name ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY s.id ASC
LIMIT sqlc.arg(limit_count);

-- name: ListServicesForExport :many
SELECT
    s.*,
    c.uuid AS category_uuid, c.name AS category_name
FROM services s
LEFT JOIN service_categories c ON c.id = s.category_id AND c.organization_id = s.organization_id AND c.deleted_at IS NULL
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.deleted_at IS NULL
  -- Faceted filters.
  AND (
    (sqlc.narg(category_ids)::bigint[] IS NULL AND sqlc.narg(uncategorized)::boolean IS NULL)
    OR s.category_id = ANY(sqlc.narg(category_ids)::bigint[])
    OR (sqlc.narg(uncategorized)::boolean IS TRUE AND s.category_id IS NULL)
  )
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR EXISTS (
        SELECT 1 FROM service_branches sb
        WHERE sb.organization_id = s.organization_id AND sb.service_id = s.id
          AND sb.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])))
  AND (sqlc.narg(payment_modes)::text[] IS NULL OR s.payment_mode = ANY(sqlc.narg(payment_modes)::text[]))
  AND (sqlc.narg(is_active)::boolean IS NULL OR s.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(show_online)::boolean IS NULL OR s.show_online = sqlc.narg(show_online))
  AND (
    sqlc.narg(q)::text IS NULL
    OR s.name ILIKE '%' || sqlc.narg(q) || '%'
    OR s.description ILIKE '%' || sqlc.narg(q) || '%'
    OR c.name ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY c.sort_order ASC NULLS LAST, c.name ASC NULLS LAST, s.sort_order ASC, s.name ASC, s.id ASC
LIMIT sqlc.arg(limit_count);

-- name: ListServiceCategoryFacets :many
SELECT c.uuid, c.name, COUNT(s.id)::bigint AS count
FROM service_categories c
JOIN services s ON s.category_id = c.id AND s.organization_id = c.organization_id AND s.deleted_at IS NULL
WHERE c.organization_id = sqlc.arg(organization_id) AND c.deleted_at IS NULL
GROUP BY c.id, c.uuid, c.name, c.sort_order
ORDER BY c.sort_order ASC, c.name ASC;

-- name: CountUncategorizedServices :one
SELECT COUNT(*)::bigint FROM services
WHERE organization_id = sqlc.arg(organization_id) AND category_id IS NULL AND deleted_at IS NULL;

-- name: ListServiceBranchFacets :many
SELECT b.uuid, b.name, COUNT(DISTINCT sb.service_id)::bigint AS count
FROM branches b
JOIN service_branches sb ON sb.branch_id = b.id AND sb.organization_id = b.organization_id
JOIN services s ON s.id = sb.service_id AND s.organization_id = sb.organization_id AND s.deleted_at IS NULL
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.deleted_at IS NULL
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR b.id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY b.id, b.uuid, b.name
ORDER BY b.name ASC;

-- name: ListServicePaymentModeFacets :many
SELECT payment_mode AS value, COUNT(*)::bigint AS count
FROM services
WHERE organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL
GROUP BY payment_mode
ORDER BY payment_mode ASC;

-- name: ListServiceBranches :many
-- Offers of the given services (list rows and the detail).
SELECT
    sb.service_id, sb.branch_id, sb.price, sb.duration_minutes, sb.is_active,
    b.uuid AS branch_uuid, b.name AS branch_name, b.code AS branch_code
FROM service_branches sb
JOIN branches b ON b.id = sb.branch_id AND b.organization_id = sb.organization_id AND b.deleted_at IS NULL
WHERE sb.organization_id = sqlc.arg(organization_id)
  AND sb.service_id = ANY(sqlc.arg(service_ids)::bigint[])
ORDER BY b.name ASC, b.id ASC;

-- name: DeleteServiceBranch :exec
DELETE FROM service_branches
WHERE organization_id = sqlc.arg(organization_id) AND service_id = sqlc.arg(service_id) AND branch_id = sqlc.arg(branch_id);

-- name: UpsertServiceBranch :exec
INSERT INTO service_branches (organization_id, service_id, branch_id, price, duration_minutes, is_active)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(service_id), sqlc.arg(branch_id), sqlc.narg(price),
    sqlc.narg(duration_minutes), sqlc.arg(is_active)
)
ON CONFLICT (organization_id, service_id, branch_id) DO UPDATE
SET price = EXCLUDED.price, duration_minutes = EXCLUDED.duration_minutes, is_active = EXCLUDED.is_active;

-- name: ListServiceStaff :many
SELECT
    ss.service_id, ss.staff_id, ss.price, ss.duration_minutes,
    st.uuid AS staff_uuid, st.name AS staff_name, st.title AS staff_title, st.color AS staff_color,
    st.is_active AS staff_is_active
FROM service_staff ss
JOIN staff st ON st.id = ss.staff_id AND st.organization_id = ss.organization_id AND st.deleted_at IS NULL
WHERE ss.organization_id = sqlc.arg(organization_id)
  AND ss.service_id = ANY(sqlc.arg(service_ids)::bigint[])
ORDER BY st.name ASC, st.id ASC;

-- name: DeleteServiceStaffExcept :exec
DELETE FROM service_staff
WHERE organization_id = sqlc.arg(organization_id) AND service_id = sqlc.arg(service_id)
  AND NOT (staff_id = ANY(sqlc.arg(keep_staff_ids)::bigint[]));

-- name: UpsertServiceStaff :exec
INSERT INTO service_staff (organization_id, service_id, staff_id, price, duration_minutes)
VALUES (sqlc.arg(organization_id), sqlc.arg(service_id), sqlc.arg(staff_id), sqlc.narg(price), sqlc.narg(duration_minutes))
ON CONFLICT (organization_id, service_id, staff_id) DO UPDATE
SET price = EXCLUDED.price, duration_minutes = EXCLUDED.duration_minutes;

-- name: GetStaffIDsByUUIDs :many
SELECT id, uuid FROM staff
WHERE organization_id = sqlc.arg(organization_id) AND uuid = ANY(sqlc.arg(uuids)::uuid[]) AND deleted_at IS NULL;

-- name: ListServiceSearchDocuments :many
SELECT s.id, s.uuid, s.organization_id, s.name, s.description, c.name AS category_name, o.slug AS organization_slug
FROM services s
JOIN organizations o ON o.id = s.organization_id
LEFT JOIN service_categories c ON c.id = s.category_id AND c.organization_id = s.organization_id AND c.deleted_at IS NULL
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.deleted_at IS NULL;

-- name: GetServiceSearchDocument :one
SELECT s.id, s.uuid, s.organization_id, s.name, s.description, c.name AS category_name, o.slug AS organization_slug
FROM services s
JOIN organizations o ON o.id = s.organization_id
LEFT JOIN service_categories c ON c.id = s.category_id AND c.organization_id = s.organization_id AND c.deleted_at IS NULL
WHERE s.organization_id = sqlc.arg(organization_id) AND s.uuid = sqlc.arg(uuid) AND s.deleted_at IS NULL;
