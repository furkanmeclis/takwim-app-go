-- Customer confirmation (TEC-921). Every query filters by organization_id;
-- joins by alias.organization_id too.

-- name: GetConfirmationOrganization :one
-- What decides whether the add-on is on: the sector, plan and add-ons.
SELECT id, sector, plan, addons, branch_limit, staff_limit, status, (deleted_at IS NOT NULL)::BOOLEAN AS deleted
FROM organizations WHERE id = sqlc.arg(organization_id);

-- name: ListConfirmationSettings :many
SELECT s.*, b.uuid AS branch_uuid
FROM confirmation_settings s
LEFT JOIN branches b ON b.organization_id = s.organization_id AND b.id = s.branch_id
WHERE s.organization_id = sqlc.arg(organization_id)
ORDER BY s.branch_id NULLS FIRST;

-- name: GetEffectiveConfirmationSettings :one
-- The branch's own row, else the business-wide row.
SELECT * FROM confirmation_settings
WHERE organization_id = sqlc.arg(organization_id) AND (branch_id = sqlc.arg(branch_id) OR branch_id IS NULL)
ORDER BY branch_id NULLS LAST
LIMIT 1;

-- name: UpsertConfirmationSettings :one
INSERT INTO confirmation_settings (
    organization_id, branch_id, enabled, channels, fallback, first_request_minutes, follow_up_minutes,
    max_attempts, deadline_minutes, window_start_minute, window_end_minute, on_no_answer, updated_by
)
VALUES (
    sqlc.arg(organization_id), sqlc.narg(branch_id), sqlc.arg(enabled), sqlc.arg(channels)::text[], sqlc.arg(fallback),
    sqlc.arg(first_request_minutes), sqlc.arg(follow_up_minutes), sqlc.arg(max_attempts), sqlc.arg(deadline_minutes),
    sqlc.narg(window_start_minute), sqlc.narg(window_end_minute), sqlc.arg(on_no_answer), sqlc.narg(updated_by)
)
ON CONFLICT ON CONSTRAINT uq_confirmation_settings DO UPDATE
SET enabled = EXCLUDED.enabled, channels = EXCLUDED.channels, fallback = EXCLUDED.fallback,
    first_request_minutes = EXCLUDED.first_request_minutes, follow_up_minutes = EXCLUDED.follow_up_minutes,
    max_attempts = EXCLUDED.max_attempts, deadline_minutes = EXCLUDED.deadline_minutes,
    window_start_minute = EXCLUDED.window_start_minute, window_end_minute = EXCLUDED.window_end_minute,
    on_no_answer = EXCLUDED.on_no_answer, updated_by = EXCLUDED.updated_by
RETURNING *;

-- name: DeleteConfirmationSettings :execrows
DELETE FROM confirmation_settings
WHERE organization_id = sqlc.arg(organization_id) AND branch_id IS NOT DISTINCT FROM sqlc.narg(branch_id);

-- name: ListConfirmationBranches :many
-- The branches the settings page lists (inside the caller's scope).
SELECT id, uuid, name, COALESCE(timezone, '')::TEXT AS timezone
FROM branches
WHERE organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR id = ANY(sqlc.narg(branch_ids)::bigint[]))
ORDER BY name, id;

-- name: GetConfirmationBranch :one
SELECT id, uuid, name FROM branches
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid) AND deleted_at IS NULL;

-- name: GetConfirmationReservation :one
-- The reservation a task acts on, locked (lock order: reservation, then
-- its task), with the time zone of its branch.
SELECT r.id, r.uuid, r.branch_id, r.customer_id, r.status, r.approval_status, r.confirmation_status, r.version,
       r.starts_at, r.cancel_reason
FROM reservations r
WHERE r.organization_id = sqlc.arg(organization_id) AND r.id = sqlc.arg(id)
FOR UPDATE;

-- name: GetConfirmationReservationByUUID :one
SELECT r.id, r.uuid, r.branch_id, r.customer_id, r.status, r.approval_status, r.confirmation_status, r.version,
       r.starts_at, r.cancel_reason
FROM reservations r
WHERE r.organization_id = sqlc.arg(organization_id) AND r.uuid = sqlc.arg(uuid)
FOR UPDATE;

-- name: GetConfirmationTimezone :one
SELECT COALESCE(NULLIF(b.timezone, ''), o.timezone)::TEXT AS timezone
FROM branches b
JOIN organizations o ON o.id = b.organization_id
WHERE b.organization_id = sqlc.arg(organization_id) AND b.id = sqlc.arg(branch_id);

-- name: SetReservationConfirmationStatus :one
-- The customer confirmation field. bump raises the version (a change of
-- its own: request sent, answer, deadline); a status set as part of
-- another change keeps that change's version.
UPDATE reservations
SET confirmation_status = sqlc.arg(confirmation_status),
    version = version + CASE WHEN sqlc.arg(bump)::BOOLEAN THEN 1 ELSE 0 END
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING version;

-- name: GetReservationConfirmation :one
SELECT * FROM reservation_confirmations
WHERE organization_id = sqlc.arg(organization_id) AND reservation_id = sqlc.arg(reservation_id);

-- name: LockReservationConfirmation :one
SELECT * FROM reservation_confirmations
WHERE organization_id = sqlc.arg(organization_id) AND reservation_id = sqlc.arg(reservation_id)
FOR UPDATE;

-- name: PlanReservationConfirmation :one
-- A new task, or the task planned again for a moved visit (next round,
-- answers cleared).
INSERT INTO reservation_confirmations (
    organization_id, reservation_id, reservation_version, starts_at, status, channels, fallback, on_no_answer,
    attempt_times, attempts_sent, next_attempt_at, deadline_at, planned_at
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(reservation_id), sqlc.arg(reservation_version), sqlc.arg(starts_at), 'scheduled',
    sqlc.arg(channels)::text[], sqlc.arg(fallback), sqlc.arg(on_no_answer), sqlc.arg(attempt_times)::timestamptz[], 0,
    sqlc.narg(next_attempt_at), sqlc.arg(deadline_at), sqlc.arg(planned_at)
)
ON CONFLICT (organization_id, reservation_id) DO UPDATE
SET round = reservation_confirmations.round + 1, reservation_version = EXCLUDED.reservation_version,
    starts_at = EXCLUDED.starts_at, status = 'scheduled', channels = EXCLUDED.channels, fallback = EXCLUDED.fallback,
    on_no_answer = EXCLUDED.on_no_answer, attempt_times = EXCLUDED.attempt_times, attempts_sent = 0,
    next_attempt_at = EXCLUDED.next_attempt_at, deadline_at = EXCLUDED.deadline_at, answer = NULL,
    answer_channel = NULL, answered_at = NULL, change_requested_at = NULL, change_note = '', outcome = NULL,
    last_error = '', planned_at = EXCLUDED.planned_at, closed_at = NULL
RETURNING *;

-- name: SaveReservationConfirmation :exec
-- Writes the mutable state of a task.
UPDATE reservation_confirmations
SET reservation_version = sqlc.arg(reservation_version), status = sqlc.arg(status),
    attempts_sent = sqlc.arg(attempts_sent), next_attempt_at = sqlc.narg(next_attempt_at),
    answer = sqlc.narg(answer), answer_channel = sqlc.narg(answer_channel), answered_at = sqlc.narg(answered_at),
    change_requested_at = sqlc.narg(change_requested_at), change_note = sqlc.arg(change_note),
    outcome = sqlc.narg(outcome), last_error = sqlc.arg(last_error), closed_at = sqlc.narg(closed_at)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: ListDueReservationConfirmations :many
-- Tasks with an attempt or a deadline due (the worker's scan; every
-- business, or one when organization_id is given).
SELECT c.organization_id, c.reservation_id
FROM reservation_confirmations c
WHERE c.status IN ('scheduled', 'pending')
  AND (sqlc.narg(organization_id)::bigint IS NULL OR c.organization_id = sqlc.narg(organization_id))
  AND ((c.next_attempt_at IS NOT NULL AND c.next_attempt_at <= sqlc.arg(now_at))
       OR (c.change_requested_at IS NULL AND c.deadline_at <= sqlc.arg(now_at)))
ORDER BY LEAST(COALESCE(c.next_attempt_at, c.deadline_at), c.deadline_at), c.id
LIMIT sqlc.arg(limit_count);

-- name: CountConfirmationSends :one
-- Requests of the current round a channel really sent (accepted by a
-- provider): an expired task without one is unreachable, never cancelled.
SELECT count(*)
FROM customer_notification_deliveries dl
JOIN customer_notification_dispatches d ON d.organization_id = dl.organization_id AND d.id = dl.dispatch_id
WHERE dl.organization_id = sqlc.arg(organization_id) AND d.reservation_id = sqlc.arg(reservation_id)
  AND d.event_code = sqlc.arg(event_code) AND d.scheduled_at >= sqlc.arg(since)
  AND dl.status IN ('sent', 'delivered');

-- name: CreateConfirmationEvent :exec
-- A confirmation row of the reservation history (from / to are
-- confirmation statuses).
INSERT INTO reservation_events (organization_id, reservation_id, version, type, from_status, to_status, actor_id, note)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(reservation_id), sqlc.arg(version), sqlc.arg(type), sqlc.narg(from_status),
    sqlc.narg(to_status), sqlc.narg(actor_id), sqlc.arg(note)
);
