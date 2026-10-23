-- Integration accounts and jobs (TEC-880). Accounts are organization-owned
-- (every query filters by organization_id); jobs are mixed scope
-- (organization_id NULL = platform job) and every query names the scope.

-- name: InsertIntegrationAccount :one
INSERT INTO integration_accounts (
    uuid, organization_id, kind, provider, display_name, mode, status, credentials_ciphertext, credentials_hint,
    settings, created_by, updated_by
) VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.arg(kind), sqlc.arg(provider), sqlc.arg(display_name),
    sqlc.arg(mode), 'active', sqlc.arg(credentials_ciphertext), sqlc.arg(credentials_hint), sqlc.arg(settings),
    sqlc.narg(created_by), sqlc.narg(created_by)
)
RETURNING id;

-- name: GetIntegrationAccount :one
SELECT a.id, a.uuid, a.organization_id, a.kind, a.provider, a.display_name, a.mode, a.status,
    a.credentials_ciphertext, a.credentials_hint, a.settings, a.last_checked_at, a.last_check_status,
    a.last_check_error, a.created_at, a.updated_at,
    cu.uuid AS created_by_uuid, cu.name AS created_by_name, cu.surname AS created_by_surname, cu.email AS created_by_email
FROM integration_accounts a
LEFT JOIN users cu ON cu.id = a.created_by
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.uuid = sqlc.arg(uuid);

-- name: GetIntegrationAccountByID :one
SELECT a.id, a.uuid, a.organization_id, a.kind, a.provider, a.display_name, a.mode, a.status,
    a.credentials_ciphertext, a.credentials_hint, a.settings
FROM integration_accounts a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.id = sqlc.arg(id);

-- name: GetActiveIntegrationAccount :one
-- The organization's active account of a kind (payments go to it).
SELECT a.id, a.uuid, a.organization_id, a.kind, a.provider, a.display_name, a.mode, a.status,
    a.credentials_ciphertext, a.credentials_hint, a.settings
FROM integration_accounts a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.kind = sqlc.arg(kind)
  AND a.status = 'active';

-- name: ListIntegrationAccounts :many
SELECT a.uuid, a.kind, a.provider, a.display_name, a.mode, a.status, a.credentials_hint, a.last_checked_at,
    a.last_check_status, a.last_check_error, a.created_at, a.updated_at,
    COUNT(*) OVER ()::bigint AS total
FROM integration_accounts a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(statuses)::text[] IS NULL OR a.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR a.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(providers)::text[] IS NULL OR a.provider = ANY(sqlc.narg(providers)::text[]))
  AND (sqlc.narg(q)::text IS NULL OR a.display_name ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'display_name' AND NOT sqlc.arg(sort_desc)::boolean THEN a.display_name END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'display_name' AND sqlc.arg(sort_desc)::boolean THEN a.display_name END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN a.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN a.created_at END DESC,
    a.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: UpdateIntegrationAccount :execrows
UPDATE integration_accounts
SET display_name = sqlc.arg(display_name),
    mode = sqlc.arg(mode),
    credentials_ciphertext = sqlc.arg(credentials_ciphertext),
    credentials_hint = sqlc.arg(credentials_hint),
    settings = sqlc.arg(settings),
    updated_by = sqlc.narg(updated_by)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: SetIntegrationAccountStatus :execrows
UPDATE integration_accounts
SET status = sqlc.arg(status), updated_by = sqlc.narg(updated_by)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: SetIntegrationAccountCheck :exec
UPDATE integration_accounts
SET last_checked_at = sqlc.arg(checked_at), last_check_status = sqlc.arg(check_status),
    last_check_error = sqlc.arg(check_error)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: UpsertIntegrationJob :one
-- Records a job once per provider and idempotency key; a repeated key
-- counts the delivery and returns the existing row (inserted = false).
INSERT INTO integration_jobs (
    organization_id, integration_account_id, direction, provider, kind, idempotency_key, payload, ref_type, ref_uuid
) VALUES (
    sqlc.narg(organization_id), sqlc.narg(integration_account_id), sqlc.arg(direction), sqlc.arg(provider),
    sqlc.arg(kind), sqlc.arg(idempotency_key), sqlc.arg(payload), sqlc.arg(ref_type), sqlc.narg(ref_uuid)
)
ON CONFLICT (provider, idempotency_key) DO UPDATE
SET deliveries = integration_jobs.deliveries + 1
RETURNING id, uuid, organization_id, status, attempts, deliveries, (xmax = 0)::boolean AS inserted;

-- name: ClaimIntegrationJob :execrows
-- Moves a pending / failed job to processing (one worker at a time).
UPDATE integration_jobs
SET status = 'processing', attempts = attempts + 1
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id)
  AND status IN ('pending', 'failed');

-- name: FinishIntegrationJob :exec
UPDATE integration_jobs
SET status = sqlc.arg(status)::text, last_error = sqlc.arg(last_error), result = sqlc.arg(result),
    completed_at = CASE WHEN sqlc.arg(status)::text IN ('succeeded', 'ignored') THEN NOW() ELSE completed_at END
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: ListIntegrationJobs :many
SELECT j.uuid, j.direction, j.provider, j.kind, j.status, j.attempts, j.max_attempts, j.deliveries, j.last_error,
    j.ref_type, j.ref_uuid, j.priority, j.next_attempt_at, j.submitted_at, j.completed_at, j.created_at, j.updated_at,
    COUNT(*) OVER ()::bigint AS total
FROM integration_jobs j
WHERE j.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(account_id)::bigint IS NULL OR j.integration_account_id = sqlc.narg(account_id)::bigint)
  AND (sqlc.narg(statuses)::text[] IS NULL OR j.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR j.kind = ANY(sqlc.narg(kinds)::text[]))
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN j.updated_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN j.updated_at END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN j.created_at END ASC,
    j.created_at DESC, j.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- Generic runner (TEC-880). Every query names the organization scope; the
-- worker carries the job's organization in the task payload.

-- name: EnqueueIntegrationJob :one
-- Creates a runner job once per provider and idempotency key; a repeated
-- key returns the existing row (inserted = false) and sends nothing again.
INSERT INTO integration_jobs (
    organization_id, integration_account_id, direction, provider, kind, idempotency_key, payload, ref_type, ref_uuid,
    priority, max_attempts, status, next_attempt_at
) VALUES (
    sqlc.narg(organization_id), sqlc.narg(integration_account_id), 'outbound', sqlc.arg(provider),
    sqlc.arg(kind), sqlc.arg(idempotency_key), sqlc.arg(payload), sqlc.arg(ref_type), sqlc.narg(ref_uuid),
    sqlc.arg(priority), sqlc.arg(max_attempts), 'pending', NOW()
)
ON CONFLICT (provider, idempotency_key) DO UPDATE
SET deliveries = integration_jobs.deliveries + 1
RETURNING id, uuid, organization_id, status, (xmax = 0)::boolean AS inserted;

-- name: GetRunnerJob :one
SELECT j.id, j.uuid, j.organization_id, j.integration_account_id, j.direction, j.provider, j.kind, j.idempotency_key,
    j.status, j.attempts, j.max_attempts, j.deliveries, j.last_error, j.payload, j.result, j.ref_type, j.ref_uuid,
    j.priority, j.provider_ref, j.next_attempt_at, j.submitted_at, j.locked_until, j.completed_at, j.created_at,
    j.updated_at, a.uuid AS account_uuid
FROM integration_jobs j
LEFT JOIN integration_accounts a ON a.organization_id = j.organization_id AND a.id = j.integration_account_id
WHERE j.organization_id IS NOT DISTINCT FROM sqlc.narg(organization_id)
  AND j.id = sqlc.arg(id);

-- name: GetRunnerJobByUUID :one
SELECT j.id, j.uuid, j.organization_id, j.integration_account_id, j.direction, j.provider, j.kind, j.idempotency_key,
    j.status, j.attempts, j.max_attempts, j.deliveries, j.last_error, j.payload, j.result, j.ref_type, j.ref_uuid,
    j.priority, j.provider_ref, j.next_attempt_at, j.submitted_at, j.locked_until, j.completed_at, j.created_at,
    j.updated_at, a.uuid AS account_uuid
FROM integration_jobs j
LEFT JOIN integration_accounts a ON a.organization_id = j.organization_id AND a.id = j.integration_account_id
WHERE j.organization_id = sqlc.arg(organization_id)
  AND j.uuid = sqlc.arg(uuid);

-- name: LockRunnerJob :execrows
-- Claims a job for one provider call (one worker at a time; an expired
-- claim of a crashed worker is free again).
UPDATE integration_jobs
SET locked_until = NOW() + make_interval(secs => sqlc.arg(lease_seconds)::int)
WHERE organization_id IS NOT DISTINCT FROM sqlc.narg(organization_id)
  AND id = sqlc.arg(id)
  AND status = ANY(sqlc.arg(statuses)::text[])
  AND (locked_until IS NULL OR locked_until < NOW());

-- name: SaveRunnerJob :exec
-- Stores the outcome of a provider call and releases the claim.
UPDATE integration_jobs
SET status = sqlc.arg(status)::text,
    attempts = sqlc.arg(attempts),
    max_attempts = sqlc.arg(max_attempts),
    provider_ref = sqlc.arg(provider_ref),
    last_error = sqlc.arg(last_error),
    result = sqlc.arg(result),
    next_attempt_at = sqlc.narg(next_attempt_at),
    submitted_at = COALESCE(submitted_at, sqlc.narg(submitted_at)),
    completed_at = CASE WHEN sqlc.arg(status)::text IN ('succeeded', 'failed') THEN NOW() ELSE NULL END,
    locked_until = NULL
WHERE organization_id IS NOT DISTINCT FROM sqlc.narg(organization_id)
  AND id = sqlc.arg(id);

-- name: UnlockRunnerJob :exec
UPDATE integration_jobs
SET locked_until = NULL
WHERE organization_id IS NOT DISTINCT FROM sqlc.narg(organization_id)
  AND id = sqlc.arg(id);

-- name: ListDueRunnerJobs :many
-- The sweep: runner jobs whose next call is due, across organizations
-- (each row carries its organization_id; the runner re-scopes from it).
SELECT j.id, j.organization_id, j.priority
FROM integration_jobs j
WHERE j.status IN ('pending', 'submitted', 'needs_verification')
  AND j.next_attempt_at IS NOT NULL
  AND j.next_attempt_at <= NOW()
  AND (j.locked_until IS NULL OR j.locked_until < NOW())
  AND j.attempts < j.max_attempts
ORDER BY j.priority = 'critical' DESC, j.next_attempt_at
LIMIT sqlc.arg(limit_count);

-- name: InsertIntegrationAttempt :one
INSERT INTO integration_attempts (
    organization_id, integration_job_id, attempt_no, operation, trigger, outcome, request_summary, response_summary,
    error, started_at, finished_at, duration_ms
) VALUES (
    sqlc.narg(organization_id), sqlc.arg(integration_job_id), sqlc.arg(attempt_no), sqlc.arg(operation),
    sqlc.arg(trigger), sqlc.arg(outcome), sqlc.arg(request_summary), sqlc.arg(response_summary), sqlc.arg(error),
    sqlc.arg(started_at), sqlc.arg(finished_at), sqlc.arg(duration_ms)
)
RETURNING id, uuid;

-- name: ListIntegrationAttempts :many
SELECT t.uuid, t.attempt_no, t.operation, t.trigger, t.outcome, t.request_summary, t.response_summary, t.error,
    t.started_at, t.finished_at, t.duration_ms
FROM integration_attempts t
WHERE t.organization_id IS NOT DISTINCT FROM sqlc.narg(organization_id)
  AND t.integration_job_id = sqlc.arg(integration_job_id)
ORDER BY t.id;
