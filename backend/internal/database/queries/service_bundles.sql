-- Tenant resource: combined service bundles (TEC-895). Every query filters by
-- organization_id. A bundle is business-level catalog data; its branches are
-- the branches that offer it.

-- name: CreateServiceBundle :one
INSERT INTO service_bundles (organization_id, name, description, price, show_online, is_active, sort_order)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(name), sqlc.arg(description), sqlc.arg(price), sqlc.arg(show_online),
    sqlc.arg(is_active), sqlc.arg(sort_order)
)
RETURNING *;

-- name: UpdateServiceBundle :one
UPDATE service_bundles
SET name = sqlc.arg(name), description = sqlc.arg(description), price = sqlc.arg(price),
    show_online = sqlc.arg(show_online), is_active = sqlc.arg(is_active), sort_order = sqlc.arg(sort_order)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: TouchServiceBundle :exec
-- Raises updated_at after a change of the contents or branches only.
UPDATE service_bundles SET updated_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: SoftDeleteServiceBundle :execrows
UPDATE service_bundles SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: GetServiceBundle :one
SELECT * FROM service_bundles
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid) AND deleted_at IS NULL;

-- name: GetServiceBundleByName :one
SELECT * FROM service_bundles
WHERE organization_id = sqlc.arg(organization_id) AND lower(name) = lower(sqlc.arg(name)::text) AND deleted_at IS NULL;

-- name: ListServiceBundles :many
SELECT b.*
FROM service_bundles b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.deleted_at IS NULL
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR EXISTS (
        SELECT 1 FROM service_bundle_branches bb
        WHERE bb.organization_id = b.organization_id AND bb.bundle_id = b.id
          AND bb.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])))
  AND (sqlc.narg(is_active)::boolean IS NULL OR b.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(show_online)::boolean IS NULL OR b.show_online = sqlc.narg(show_online))
  AND (sqlc.narg(q)::text IS NULL OR b.name ILIKE '%' || sqlc.narg(q) || '%' OR b.description ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN lower(b.name) END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN lower(b.name) END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'price' AND NOT sqlc.arg(sort_desc)::boolean THEN b.price END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'price' AND sqlc.arg(sort_desc)::boolean THEN b.price END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'sort_order' AND NOT sqlc.arg(sort_desc)::boolean THEN b.sort_order END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'sort_order' AND sqlc.arg(sort_desc)::boolean THEN b.sort_order END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'is_active' AND NOT sqlc.arg(sort_desc)::boolean THEN b.is_active END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'is_active' AND sqlc.arg(sort_desc)::boolean THEN b.is_active END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN b.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN b.created_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN b.updated_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN b.updated_at END DESC,
  b.sort_order ASC, lower(b.name) ASC, b.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountServiceBundles :one
SELECT COUNT(*)::bigint
FROM service_bundles b
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.deleted_at IS NULL
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR EXISTS (
        SELECT 1 FROM service_bundle_branches bb
        WHERE bb.organization_id = b.organization_id AND bb.bundle_id = b.id
          AND bb.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])))
  AND (sqlc.narg(is_active)::boolean IS NULL OR b.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(show_online)::boolean IS NULL OR b.show_online = sqlc.narg(show_online))
  AND (sqlc.narg(q)::text IS NULL OR b.name ILIKE '%' || sqlc.narg(q) || '%' OR b.description ILIKE '%' || sqlc.narg(q) || '%');

-- name: ListServiceBundleBranchFacets :many
-- Branches offering live bundles, limited to the caller's branch scope.
SELECT br.uuid, br.name, COUNT(*)::bigint AS count
FROM service_bundle_branches bb
JOIN service_bundles b ON b.organization_id = bb.organization_id AND b.id = bb.bundle_id AND b.deleted_at IS NULL
JOIN branches br ON br.organization_id = bb.organization_id AND br.id = bb.branch_id AND br.deleted_at IS NULL
WHERE bb.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR bb.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY br.uuid, br.name
ORDER BY br.name;

-- name: ListServiceBundleItems :many
-- The contents of the bundles with the catalog values of their services.
SELECT i.bundle_id, i.position, s.id AS service_id, s.uuid AS service_uuid, s.name AS service_name,
       s.duration_minutes, s.buffer_before_minutes, s.buffer_after_minutes, s.price AS service_price,
       s.staff_mode, s.resource_kinds, s.payment_mode, s.is_active AS service_is_active,
       (s.deleted_at IS NOT NULL)::boolean AS service_deleted
FROM service_bundle_items i
JOIN services s ON s.organization_id = i.organization_id AND s.id = i.service_id
WHERE i.organization_id = sqlc.arg(organization_id) AND i.bundle_id = ANY(sqlc.arg(bundle_ids)::bigint[])
ORDER BY i.bundle_id, i.position;

-- name: DeleteServiceBundleItems :exec
DELETE FROM service_bundle_items
WHERE organization_id = sqlc.arg(organization_id) AND bundle_id = sqlc.arg(bundle_id);

-- name: InsertServiceBundleItem :exec
INSERT INTO service_bundle_items (organization_id, bundle_id, service_id, position)
VALUES (sqlc.arg(organization_id), sqlc.arg(bundle_id), sqlc.arg(service_id), sqlc.arg(position));

-- name: ListServiceBundleBranches :many
SELECT bb.bundle_id, br.id AS branch_id, br.uuid AS branch_uuid, br.name AS branch_name, br.code AS branch_code
FROM service_bundle_branches bb
JOIN branches br ON br.organization_id = bb.organization_id AND br.id = bb.branch_id AND br.deleted_at IS NULL
WHERE bb.organization_id = sqlc.arg(organization_id) AND bb.bundle_id = ANY(sqlc.arg(bundle_ids)::bigint[])
ORDER BY bb.bundle_id, br.name;

-- name: InsertServiceBundleBranch :exec
INSERT INTO service_bundle_branches (organization_id, bundle_id, branch_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(bundle_id), sqlc.arg(branch_id))
ON CONFLICT (organization_id, bundle_id, branch_id) DO NOTHING;

-- name: DeleteServiceBundleBranch :exec
DELETE FROM service_bundle_branches
WHERE organization_id = sqlc.arg(organization_id) AND bundle_id = sqlc.arg(bundle_id) AND branch_id = sqlc.arg(branch_id);

-- name: ListBundleServicesByUUIDs :many
-- Services named as bundle contents (live ones of the organization).
SELECT id, uuid, name, group_capacity
FROM services
WHERE organization_id = sqlc.arg(organization_id) AND uuid = ANY(sqlc.arg(uuids)::uuid[]) AND deleted_at IS NULL;

-- name: ListServiceBranchOffersFor :many
-- Active offers of the given services in the given branches.
SELECT sb.service_id, sb.branch_id
FROM service_branches sb
WHERE sb.organization_id = sqlc.arg(organization_id)
  AND sb.service_id = ANY(sqlc.arg(service_ids)::bigint[])
  AND sb.branch_id = ANY(sqlc.arg(branch_ids)::bigint[])
  AND sb.is_active;

-- name: GetReservationBundle :one
-- The bundle a booking names, with whether the branch offers it.
SELECT b.id, b.uuid, b.name, b.price, b.is_active, b.show_online,
       EXISTS (SELECT 1 FROM service_bundle_branches bb
               WHERE bb.organization_id = b.organization_id AND bb.bundle_id = b.id
                 AND bb.branch_id = sqlc.arg(branch_id))::boolean AS offered
FROM service_bundles b
WHERE b.organization_id = sqlc.arg(organization_id) AND b.uuid = sqlc.arg(uuid) AND b.deleted_at IS NULL;
