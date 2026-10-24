-- The confirmation list of the panel and its follow-up alerts (TEC-924).
-- Every query filters by organization_id; joins by alias.organization_id
-- too.
--
-- state is what the list shows: the task status, with change_requested (an
-- open task the customer asked to move) and an expired task split by its
-- outcome (cancelled, flagged, unreachable). send_failed: a request of the
-- current round failed on every channel (or no channel could take it).
-- attention: the visit is still ahead and a person has to act (a change
-- request, no answer, the customer could not be reached, a failed request).
-- The list and the counts repeat the same tasks CTE.

-- name: ListConfirmationTasks :many
WITH tasks AS (
    SELECT c.uuid AS task_uuid, c.status AS task_status, c.channels, c.on_no_answer, c.attempt_times, c.attempts_sent,
        c.next_attempt_at, c.deadline_at, c.answer, c.answer_channel, c.answered_at, c.change_requested_at, c.change_note,
        c.outcome, c.last_error, c.planned_at, c.updated_at AS task_updated_at,
        r.uuid, r.number, r.status, r.approval_status, r.confirmation_status, r.version, r.starts_at, r.ends_at,
        b.uuid AS branch_uuid, b.name AS branch_name,
        cu.uuid AS customer_uuid, cu.first_name AS customer_first_name, cu.last_name AS customer_last_name,
        cu.phone_e164 AS customer_phone,
        COALESCE((SELECT string_agg(l.service_name, ', ' ORDER BY l.line_index) FROM reservation_lines l
                  WHERE l.organization_id = r.organization_id AND l.reservation_id = r.id), '')::TEXT AS services,
        COALESCE((SELECT string_agg(DISTINCT l.staff_name, ', ') FROM reservation_lines l
                  WHERE l.organization_id = r.organization_id AND l.reservation_id = r.id AND l.staff_name IS NOT NULL), '')::TEXT AS staff_names,
        COALESCE(s.failed_sends, 0)::BIGINT AS failed_sends,
        COALESCE(s.last_send_status, '')::TEXT AS last_send_status,
        COALESCE(s.last_send_error, '')::TEXT AS last_send_error,
        s.last_send_at::TIMESTAMPTZ AS last_send_at,
        (CASE
            WHEN c.status IN ('scheduled', 'pending') AND c.change_requested_at IS NOT NULL THEN 'change_requested'
            WHEN c.status = 'expired' AND c.outcome IS NOT NULL THEN c.outcome
            ELSE c.status
        END)::TEXT AS state
    FROM reservation_confirmations c
    JOIN reservations r ON r.organization_id = c.organization_id AND r.id = c.reservation_id
    JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
    JOIN customers cu ON cu.organization_id = r.organization_id AND cu.id = r.customer_id
    LEFT JOIN LATERAL (
        SELECT COUNT(*) FILTER (WHERE d.status IN ('failed', 'skipped')) AS failed_sends,
            (array_agg(d.status ORDER BY d.scheduled_at DESC, d.id DESC))[1] AS last_send_status,
            (array_agg(d.last_error ORDER BY d.scheduled_at DESC, d.id DESC))[1] AS last_send_error,
            MAX(d.updated_at) AS last_send_at
        FROM customer_notification_dispatches d
        WHERE d.organization_id = c.organization_id AND d.reservation_id = c.reservation_id
          AND d.event_code = sqlc.arg(event_code) AND d.scheduled_at >= c.planned_at
    ) s ON TRUE
    WHERE c.organization_id = sqlc.arg(organization_id)
      AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
      AND (sqlc.narg(branches)::uuid[] IS NULL OR b.uuid = ANY(sqlc.narg(branches)::uuid[]))
      AND (sqlc.narg(starts_from)::timestamptz IS NULL OR r.starts_at >= sqlc.narg(starts_from))
      AND (sqlc.narg(starts_to)::timestamptz IS NULL OR r.starts_at < sqlc.narg(starts_to))
      AND (
        sqlc.narg(q)::text IS NULL
        OR r.number::text = sqlc.narg(q)
        OR (cu.first_name || ' ' || cu.last_name) ILIKE '%' || sqlc.narg(q) || '%'
        OR (sqlc.narg(q_digits)::text IS NOT NULL AND cu.phone_e164 LIKE '%' || sqlc.narg(q_digits) || '%')
      )
), marked AS (
    SELECT tasks.*,
        (tasks.task_status IN ('scheduled', 'pending') AND tasks.failed_sends > 0)::BOOLEAN AS send_failed,
        (tasks.status = 'active' AND tasks.starts_at > sqlc.arg(now_at)::timestamptz AND (
            tasks.state IN ('change_requested', 'flagged', 'unreachable')
            OR (tasks.task_status IN ('scheduled', 'pending') AND tasks.failed_sends > 0)
        ))::BOOLEAN AS attention
    FROM tasks
)
SELECT * FROM marked m
WHERE ((sqlc.narg(states)::text[] IS NULL AND m.state <> 'dropped') OR m.state = ANY(sqlc.narg(states)::text[]))
  AND (sqlc.narg(attention)::boolean IS NULL OR m.attention = sqlc.narg(attention))
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'starts_at' AND NOT sqlc.arg(sort_desc)::boolean THEN m.starts_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'starts_at' AND sqlc.arg(sort_desc)::boolean THEN m.starts_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'deadline_at' AND NOT sqlc.arg(sort_desc)::boolean THEN m.deadline_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'deadline_at' AND sqlc.arg(sort_desc)::boolean THEN m.deadline_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'number' AND NOT sqlc.arg(sort_desc)::boolean THEN m.number END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'number' AND sqlc.arg(sort_desc)::boolean THEN m.number END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'customer' AND NOT sqlc.arg(sort_desc)::boolean THEN lower(m.customer_first_name || ' ' || m.customer_last_name) END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'customer' AND sqlc.arg(sort_desc)::boolean THEN lower(m.customer_first_name || ' ' || m.customer_last_name) END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN m.task_updated_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN m.task_updated_at END DESC,
  m.starts_at ASC, m.number ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountConfirmationTaskStates :many
-- Counts per state, attention, failed request and upcoming visit (every
-- filter but the state and attention ones): the list total, the facets
-- and the follow-up summary.
WITH tasks AS (
    SELECT c.status AS task_status, r.status, r.starts_at,
        COALESCE(s.failed_sends, 0)::BIGINT AS failed_sends,
        (CASE
            WHEN c.status IN ('scheduled', 'pending') AND c.change_requested_at IS NOT NULL THEN 'change_requested'
            WHEN c.status = 'expired' AND c.outcome IS NOT NULL THEN c.outcome
            ELSE c.status
        END)::TEXT AS state
    FROM reservation_confirmations c
    JOIN reservations r ON r.organization_id = c.organization_id AND r.id = c.reservation_id
    JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
    JOIN customers cu ON cu.organization_id = r.organization_id AND cu.id = r.customer_id
    LEFT JOIN LATERAL (
        SELECT COUNT(*) FILTER (WHERE d.status IN ('failed', 'skipped')) AS failed_sends
        FROM customer_notification_dispatches d
        WHERE d.organization_id = c.organization_id AND d.reservation_id = c.reservation_id
          AND d.event_code = sqlc.arg(event_code) AND d.scheduled_at >= c.planned_at
    ) s ON TRUE
    WHERE c.organization_id = sqlc.arg(organization_id)
      AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
      AND (sqlc.narg(branches)::uuid[] IS NULL OR b.uuid = ANY(sqlc.narg(branches)::uuid[]))
      AND (sqlc.narg(starts_from)::timestamptz IS NULL OR r.starts_at >= sqlc.narg(starts_from))
      AND (sqlc.narg(starts_to)::timestamptz IS NULL OR r.starts_at < sqlc.narg(starts_to))
      AND (
        sqlc.narg(q)::text IS NULL
        OR r.number::text = sqlc.narg(q)
        OR (cu.first_name || ' ' || cu.last_name) ILIKE '%' || sqlc.narg(q) || '%'
        OR (sqlc.narg(q_digits)::text IS NOT NULL AND cu.phone_e164 LIKE '%' || sqlc.narg(q_digits) || '%')
      )
)
SELECT tasks.state,
    (tasks.task_status IN ('scheduled', 'pending') AND tasks.failed_sends > 0)::BOOLEAN AS send_failed,
    (tasks.status = 'active' AND tasks.starts_at > sqlc.arg(now_at)::timestamptz)::BOOLEAN AS upcoming,
    count(*)::BIGINT AS count
FROM tasks
GROUP BY 1, 2, 3;

-- name: ListConfirmationAttempts :many
-- The requests of one booking's confirmation, newest first: one row per
-- channel tried (a request no channel took yet has no delivery).
SELECT d.uuid AS dispatch_uuid, d.idempotency_key, d.status AS dispatch_status, d.last_error AS dispatch_error,
       d.scheduled_at, d.completed_at,
       dl.uuid AS delivery_uuid, dl.channel, dl.attempt_no, dl.status AS delivery_status, dl.provider,
       dl.recipient_masked, dl.error AS delivery_error, dl.sent_at, dl.finished_at, dl.created_at AS delivery_created_at,
       dl.delivered_at, dl.read_at
FROM customer_notification_dispatches d
LEFT JOIN customer_notification_deliveries dl ON dl.organization_id = d.organization_id AND dl.dispatch_id = d.id
WHERE d.organization_id = sqlc.arg(organization_id) AND d.reservation_id = sqlc.arg(reservation_id)
  AND d.event_code = sqlc.arg(event_code)
ORDER BY d.scheduled_at DESC, d.id DESC, dl.attempt_no, dl.id
LIMIT 200;

-- name: GetConfirmationReservationRef :one
-- A booking of the panel by uuid (not locked): its id, branch and
-- version.
SELECT r.id, r.uuid, r.branch_id, r.number, r.version
FROM reservations r
WHERE r.organization_id = sqlc.arg(organization_id) AND r.uuid = sqlc.arg(uuid);
