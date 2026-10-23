-- name: InsertActivityEvent :one
INSERT INTO activity_events (actor_user_id, action, resource, resource_uuid, payload, ip_address, user_agent, organization_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, sqlc.narg(organization_id))
RETURNING *;

-- name: ListActivityEvents :many
SELECT * FROM activity_events
WHERE (sqlc.narg(actor_user_id)::bigint IS NULL OR actor_user_id = sqlc.narg(actor_user_id))
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
WHERE (sqlc.narg(actor_user_id)::bigint IS NULL OR actor_user_id = sqlc.narg(actor_user_id))
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
