-- name: InsertActivityEvent :one
-- branch_uuid ties the event to a branch of the same organization (NULL or
-- an unknown uuid = an event of the whole business).
INSERT INTO activity_events (actor_user_id, action, resource, resource_uuid, payload, ip_address, user_agent, organization_id, branch_id)
VALUES (
    $1, $2, $3, $4, $5, $6, $7, sqlc.narg(organization_id),
    (SELECT b.id FROM branches b
     WHERE b.organization_id = sqlc.narg(organization_id) AND b.uuid = sqlc.narg(branch_uuid)::uuid)
)
RETURNING *;

-- name: ListActivityEvents :many
-- Platform audit feed: platform events only. Organization events are read
-- per resource through ListOrganizationResourceActivity.
SELECT * FROM activity_events
WHERE organization_id IS NULL
  AND (sqlc.narg(actor_user_id)::bigint IS NULL OR actor_user_id = sqlc.narg(actor_user_id))
  AND (sqlc.narg(resource)::text IS NULL OR resource = sqlc.narg(resource))
  AND (sqlc.narg(action)::text IS NULL OR action = sqlc.narg(action))
  AND (
    sqlc.narg(q)::text IS NULL
    OR action ILIKE '%' || sqlc.narg(q) || '%'
    OR resource ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountActivityEvents :one
SELECT COUNT(*)::bigint FROM activity_events
WHERE organization_id IS NULL
  AND (sqlc.narg(actor_user_id)::bigint IS NULL OR actor_user_id = sqlc.narg(actor_user_id))
  AND (sqlc.narg(resource)::text IS NULL OR resource = sqlc.narg(resource))
  AND (sqlc.narg(action)::text IS NULL OR action = sqlc.narg(action))
  AND (
    sqlc.narg(q)::text IS NULL
    OR action ILIKE '%' || sqlc.narg(q) || '%'
    OR resource ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListOrganizationResourceActivity :many
SELECT
    e.uuid, e.action, e.resource, e.resource_uuid, e.payload, e.created_at,
    u.uuid AS actor_uuid, u.name AS actor_name, u.surname AS actor_surname, u.email AS actor_email
FROM activity_events e
LEFT JOIN users u ON u.id = e.actor_user_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.resource = sqlc.arg(resource)
  AND e.resource_uuid = sqlc.arg(resource_uuid)
ORDER BY e.created_at DESC, e.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountOrganizationResourceActivity :one
SELECT COUNT(*)::bigint
FROM activity_events
WHERE organization_id = sqlc.arg(organization_id)
  AND resource = sqlc.arg(resource)
  AND resource_uuid = sqlc.arg(resource_uuid);

-- name: ListTenantActivity :many
-- Activity history of one organization. branch_ids is the caller's branch
-- scope (narrowed by X-Branch): NULL = every event, otherwise only events of
-- those branches (events of the whole business are hidden).
SELECT
    e.uuid, e.action, e.resource, e.resource_uuid, e.payload, e.created_at,
    u.uuid AS actor_uuid, u.name AS actor_name, u.surname AS actor_surname, u.email AS actor_email,
    b.uuid AS branch_uuid, b.name AS branch_name
FROM activity_events e
LEFT JOIN users u ON u.id = e.actor_user_id
LEFT JOIN branches b ON b.id = e.branch_id AND b.organization_id = e.organization_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(resources)::text[] IS NULL OR e.resource = ANY(sqlc.narg(resources)::text[]))
  AND (sqlc.narg(actions)::text[] IS NULL OR e.action = ANY(sqlc.narg(actions)::text[]))
  AND (sqlc.narg(resource_uuid)::uuid IS NULL OR e.resource_uuid = sqlc.narg(resource_uuid))
  AND (sqlc.narg(actor_uuid)::uuid IS NULL OR u.uuid = sqlc.narg(actor_uuid))
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR e.created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR e.created_at < sqlc.narg(created_to))
  AND (
    sqlc.narg(q)::text IS NULL
    OR e.action ILIKE '%' || sqlc.narg(q) || '%'
    OR u.email ILIKE '%' || sqlc.narg(q) || '%'
    OR (u.name || ' ' || u.surname) ILIKE '%' || sqlc.narg(q) || '%'
    OR b.name ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_desc)::boolean THEN e.created_at END DESC,
  CASE WHEN NOT sqlc.arg(sort_desc)::boolean THEN e.created_at END ASC,
  e.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountTenantActivity :one
SELECT COUNT(*)::bigint
FROM activity_events e
LEFT JOIN users u ON u.id = e.actor_user_id
LEFT JOIN branches b ON b.id = e.branch_id AND b.organization_id = e.organization_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(resources)::text[] IS NULL OR e.resource = ANY(sqlc.narg(resources)::text[]))
  AND (sqlc.narg(actions)::text[] IS NULL OR e.action = ANY(sqlc.narg(actions)::text[]))
  AND (sqlc.narg(resource_uuid)::uuid IS NULL OR e.resource_uuid = sqlc.narg(resource_uuid))
  AND (sqlc.narg(actor_uuid)::uuid IS NULL OR u.uuid = sqlc.narg(actor_uuid))
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR e.created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR e.created_at < sqlc.narg(created_to))
  AND (
    sqlc.narg(q)::text IS NULL
    OR e.action ILIKE '%' || sqlc.narg(q) || '%'
    OR u.email ILIKE '%' || sqlc.narg(q) || '%'
    OR (u.name || ' ' || u.surname) ILIKE '%' || sqlc.narg(q) || '%'
    OR b.name ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: TenantActivityFacets :many
-- Distinct resources and actions (with counts) in the caller's scope, for
-- the faceted filters.
SELECT e.resource, e.action, COUNT(*)::bigint AS count
FROM activity_events e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY e.resource, e.action
ORDER BY e.resource, e.action;
