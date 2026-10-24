-- Platform operations (TEC-930): the failed work of every organization for
-- the platform operator (platform.settings.read). Lists carry the
-- organization's name, never customer data (no recipients, no bodies).

-- name: ListPlatformIntegrationJobs :many
SELECT j.uuid, j.direction, j.provider, j.kind, j.status, j.attempts, j.max_attempts, j.last_error,
    j.ref_type, j.priority, j.next_attempt_at, j.completed_at, j.created_at, j.updated_at,
    o.uuid AS organization_uuid, COALESCE(o.name, '')::text AS organization_name,
    COUNT(*) OVER ()::bigint AS total
FROM integration_jobs j
LEFT JOIN organizations o ON o.id = j.organization_id
WHERE (sqlc.narg(statuses)::text[] IS NULL OR j.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(providers)::text[] IS NULL OR j.provider = ANY(sqlc.narg(providers)::text[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR j.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.arg(q)::text = ''
       OR o.name ILIKE '%' || sqlc.arg(q)::text || '%'
       OR j.last_error ILIKE '%' || sqlc.arg(q)::text || '%'
       OR j.kind ILIKE '%' || sqlc.arg(q)::text || '%')
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN j.updated_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN j.updated_at END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN j.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN j.created_at END DESC,
    j.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: GetPlatformIntegrationJobScope :one
-- The organization of a job (0 = platform job), for the runner calls that
-- name their scope.
SELECT COALESCE(j.organization_id, 0)::bigint AS organization_id
FROM integration_jobs j
WHERE j.uuid = sqlc.arg(uuid);

-- name: CountPlatformIntegrationJobsByStatus :many
SELECT j.status, COUNT(*)::bigint AS total
FROM integration_jobs j
WHERE j.status = ANY(sqlc.arg(statuses)::text[])
GROUP BY j.status;

-- name: ListPlatformNoticeFailures :many
SELECT d.uuid, d.channel, d.provider, d.attempt_no, d.error, d.created_at, d.finished_at,
    p.event_code, o.uuid AS organization_uuid, o.name AS organization_name,
    COUNT(*) OVER ()::bigint AS total
FROM customer_notification_deliveries d
JOIN customer_notification_dispatches p ON p.organization_id = d.organization_id AND p.id = d.dispatch_id
JOIN organizations o ON o.id = d.organization_id
WHERE d.status = 'failed'
  AND (sqlc.narg(channels)::text[] IS NULL OR d.channel = ANY(sqlc.narg(channels)::text[]))
  AND (sqlc.arg(q)::text = ''
       OR o.name ILIKE '%' || sqlc.arg(q)::text || '%'
       OR d.error ILIKE '%' || sqlc.arg(q)::text || '%')
ORDER BY
    CASE WHEN NOT sqlc.arg(sort_desc)::boolean THEN d.created_at END ASC,
    d.created_at DESC, d.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountPlatformNoticeFailuresSince :one
SELECT COUNT(*)::bigint
FROM customer_notification_deliveries d
JOIN organizations o ON o.id = d.organization_id
WHERE d.status = 'failed' AND d.created_at >= sqlc.arg(since);

-- name: CountPlatformEngineFailuresSince :one
-- Failed export, import and bulk jobs of the platform and every
-- organization.
SELECT
    (SELECT COUNT(*) FROM export_jobs e WHERE e.status = 'failed' AND e.created_at >= sqlc.arg(since))::bigint AS exports,
    (SELECT COUNT(*) FROM import_jobs i WHERE i.status = 'failed' AND i.created_at >= sqlc.arg(since))::bigint AS imports,
    (SELECT COUNT(*) FROM bulk_jobs b WHERE b.status = 'failed' AND b.created_at >= sqlc.arg(since))::bigint AS bulk;
