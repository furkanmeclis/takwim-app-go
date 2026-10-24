-- Customer notifications (TEC-911). Every query filters by organization_id;
-- joins by alias.organization_id too.

-- name: GetCustomerNotificationRule :one
SELECT * FROM customer_notification_rules
WHERE organization_id = sqlc.arg(organization_id) AND event_code = sqlc.arg(event_code);

-- name: ListCustomerNotificationRules :many
SELECT * FROM customer_notification_rules
WHERE organization_id = sqlc.arg(organization_id)
ORDER BY event_code;

-- name: UpsertCustomerNotificationRule :one
INSERT INTO customer_notification_rules (organization_id, event_code, enabled, channels, fallback, reminder_offsets, updated_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(event_code), sqlc.arg(enabled), sqlc.arg(channels)::text[], sqlc.arg(fallback),
        sqlc.arg(reminder_offsets)::int[], sqlc.narg(updated_by))
ON CONFLICT (organization_id, event_code) DO UPDATE
SET enabled = EXCLUDED.enabled, channels = EXCLUDED.channels, fallback = EXCLUDED.fallback,
    reminder_offsets = EXCLUDED.reminder_offsets, updated_by = EXCLUDED.updated_by
RETURNING *;

-- name: DeleteCustomerNotificationRule :exec
DELETE FROM customer_notification_rules
WHERE organization_id = sqlc.arg(organization_id) AND event_code = sqlc.arg(event_code);

-- name: GetCustomerNotificationTemplate :one
SELECT * FROM customer_notification_templates
WHERE organization_id = sqlc.arg(organization_id) AND event_code = sqlc.arg(event_code) AND channel = sqlc.arg(channel);

-- name: ListCustomerNotificationTemplates :many
SELECT * FROM customer_notification_templates
WHERE organization_id = sqlc.arg(organization_id)
ORDER BY event_code, channel;

-- name: UpsertCustomerNotificationTemplate :exec
INSERT INTO customer_notification_templates (organization_id, event_code, channel, subject, body, updated_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(event_code), sqlc.arg(channel), sqlc.arg(subject), sqlc.arg(body), sqlc.narg(updated_by))
ON CONFLICT (organization_id, event_code, channel) DO UPDATE
SET subject = EXCLUDED.subject, body = EXCLUDED.body, updated_by = EXCLUDED.updated_by;

-- name: DeleteCustomerNotificationTemplate :exec
DELETE FROM customer_notification_templates
WHERE organization_id = sqlc.arg(organization_id) AND event_code = sqlc.arg(event_code) AND channel = sqlc.arg(channel);

-- name: DeleteCustomerNotificationTemplates :exec
DELETE FROM customer_notification_templates
WHERE organization_id = sqlc.arg(organization_id) AND event_code = sqlc.arg(event_code);

-- name: GetNotifyOrganization :one
SELECT id, uuid, slug, name, status, timezone::TEXT AS timezone, (deleted_at IS NOT NULL)::BOOLEAN AS deleted
FROM organizations WHERE id = sqlc.arg(organization_id);

-- name: GetNotifyReservation :one
-- What a reservation notice says: the visit, its branch (and the time zone
-- its times are shown in) and its services.
SELECT r.id, r.uuid, r.number, r.status, r.version, r.customer_id, r.source, r.starts_at, r.ends_at, r.created_at,
       b.name AS branch_name, COALESCE(NULLIF(b.timezone, ''), o.timezone)::TEXT AS timezone,
       COALESCE((SELECT string_agg(l.service_name, ', ' ORDER BY l.line_index) FROM reservation_lines l
                 WHERE l.organization_id = r.organization_id AND l.reservation_id = r.id), '')::TEXT AS services
FROM reservations r
JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
JOIN organizations o ON o.id = r.organization_id
WHERE r.organization_id = sqlc.arg(organization_id) AND r.id = sqlc.arg(id);

-- name: GetNotifyCustomer :one
SELECT id, uuid, first_name, last_name, phone_e164, email, (deleted_at IS NOT NULL)::BOOLEAN AS deleted
FROM customers
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: InsertCustomerNotificationDispatch :one
-- No row when the idempotency key was already used (the notice exists).
INSERT INTO customer_notification_dispatches (
    organization_id, event_code, customer_id, reservation_id, reservation_version, idempotency_key, channels,
    fallback, vars, status, last_error, due_at, scheduled_at, completed_at
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(event_code), sqlc.arg(customer_id), sqlc.narg(reservation_id),
    sqlc.arg(reservation_version), sqlc.arg(idempotency_key), sqlc.arg(channels)::text[], sqlc.arg(fallback),
    sqlc.arg(vars), sqlc.arg(status), sqlc.arg(last_error), sqlc.narg(due_at), sqlc.arg(scheduled_at), sqlc.narg(completed_at)
)
ON CONFLICT (organization_id, idempotency_key) DO NOTHING
RETURNING *;

-- name: GetCustomerNotificationDispatch :one
SELECT * FROM customer_notification_dispatches
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: GetCustomerNotificationDispatchByKey :one
SELECT * FROM customer_notification_dispatches
WHERE organization_id = sqlc.arg(organization_id) AND idempotency_key = sqlc.arg(idempotency_key);

-- name: LockCustomerNotificationDispatch :one
-- Another worker holding the row: no row (it sends it).
SELECT * FROM customer_notification_dispatches
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
FOR UPDATE SKIP LOCKED;

-- name: LockCustomerNotificationDispatchWait :one
SELECT * FROM customer_notification_dispatches
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
FOR UPDATE;

-- name: UpdateCustomerNotificationDispatchState :exec
UPDATE customer_notification_dispatches
SET status = sqlc.arg(status), next_channel_index = sqlc.arg(next_channel_index), last_error = sqlc.arg(last_error),
    completed_at = sqlc.narg(completed_at)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: ListDueCustomerNotificationOrganizations :many
-- Businesses with notices to send (the worker's scan).
SELECT DISTINCT d.organization_id
FROM customer_notification_dispatches d
WHERE d.status = 'pending' AND d.scheduled_at <= sqlc.arg(now_at)
ORDER BY d.organization_id;

-- name: ListDueCustomerNotificationDispatches :many
SELECT id FROM customer_notification_dispatches
WHERE organization_id = sqlc.arg(organization_id) AND status = 'pending' AND scheduled_at <= sqlc.arg(now_at)
ORDER BY scheduled_at, id
LIMIT sqlc.arg(limit_count);

-- name: InsertCustomerNotificationDelivery :one
-- No row when this channel was already tried for the dispatch.
INSERT INTO customer_notification_deliveries (organization_id, dispatch_id, channel, attempt_no, status, subject, body)
VALUES (sqlc.arg(organization_id), sqlc.arg(dispatch_id), sqlc.arg(channel), sqlc.arg(attempt_no), 'sending',
        sqlc.arg(subject), sqlc.arg(body))
ON CONFLICT (organization_id, dispatch_id, channel) DO NOTHING
RETURNING *;

-- name: UpdateCustomerNotificationDelivery :execrows
-- The send result of an attempt still in flight.
UPDATE customer_notification_deliveries
SET status = sqlc.arg(status), provider = sqlc.arg(provider), provider_ref = sqlc.arg(provider_ref),
    external_uuid = sqlc.narg(external_uuid), recipient_masked = sqlc.arg(recipient_masked), error = sqlc.arg(error),
    sent_at = sqlc.narg(sent_at), finished_at = sqlc.narg(finished_at)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'sending' AND finished_at IS NULL;

-- name: FinishCustomerNotificationDelivery :execrows
-- A delivery report settles an accepted attempt (once).
UPDATE customer_notification_deliveries
SET status = sqlc.arg(status), error = sqlc.arg(error),
    provider_ref = CASE WHEN sqlc.arg(provider_ref)::text <> '' THEN sqlc.arg(provider_ref)::text ELSE provider_ref END,
    finished_at = sqlc.arg(finished_at)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND finished_at IS NULL;

-- name: GetCustomerNotificationDeliveryByUUID :one
SELECT * FROM customer_notification_deliveries
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: GetCustomerNotificationDeliveryByExternal :one
SELECT * FROM customer_notification_deliveries
WHERE organization_id = sqlc.arg(organization_id) AND channel = sqlc.arg(channel) AND external_uuid = sqlc.arg(external_uuid);

-- name: ListCustomerNotificationDeliveriesForDispatch :many
SELECT * FROM customer_notification_deliveries
WHERE organization_id = sqlc.arg(organization_id) AND dispatch_id = sqlc.arg(dispatch_id)
ORDER BY attempt_no;

-- name: ListNotifyReminderOrganizations :many
-- Businesses with active visits starting inside the reminder window.
SELECT DISTINCT r.organization_id
FROM reservations r
JOIN organizations o ON o.id = r.organization_id AND o.status = 'active' AND o.deleted_at IS NULL
WHERE r.status = 'active' AND r.starts_at > sqlc.arg(now_at) AND r.starts_at <= sqlc.arg(until_at)
ORDER BY r.organization_id;

-- name: ListNotifyReminderCandidates :many
SELECT r.id, r.uuid, r.customer_id, r.version, r.starts_at, r.created_at
FROM reservations r
WHERE r.organization_id = sqlc.arg(organization_id) AND r.status = 'active'
  AND r.starts_at > sqlc.arg(now_at) AND r.starts_at <= sqlc.arg(until_at)
ORDER BY r.starts_at, r.id;

-- name: ListCustomerNotificationDeliveries :many
-- The send history: one row per channel attempt. branch_ids = the caller's
-- branch scope (NULL = every branch).
SELECT dl.uuid, dl.channel, dl.attempt_no, dl.status, dl.provider, dl.provider_ref, dl.recipient_masked,
       dl.subject, dl.body, dl.error, dl.sent_at, dl.finished_at, dl.created_at,
       d.uuid AS dispatch_uuid, d.event_code, d.status AS dispatch_status,
       c.uuid AS customer_uuid, c.first_name AS customer_first_name, c.last_name AS customer_last_name,
       r.uuid AS reservation_uuid, r.number AS reservation_number
FROM customer_notification_deliveries dl
JOIN customer_notification_dispatches d ON d.organization_id = dl.organization_id AND d.id = dl.dispatch_id
JOIN customers c ON c.organization_id = d.organization_id AND c.id = d.customer_id
LEFT JOIN reservations r ON r.organization_id = d.organization_id AND r.id = d.reservation_id
WHERE dl.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(channels)::text[] IS NULL OR dl.channel = ANY(sqlc.narg(channels)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR dl.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(events)::text[] IS NULL OR d.event_code = ANY(sqlc.narg(events)::text[]))
  AND (sqlc.narg(customer_uuid)::uuid IS NULL OR c.uuid = sqlc.narg(customer_uuid))
  AND (sqlc.narg(reservation_uuid)::uuid IS NULL OR r.uuid = sqlc.narg(reservation_uuid))
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR dl.created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR dl.created_at < sqlc.narg(created_to))
  AND (
    sqlc.narg(q)::text IS NULL
    OR r.number::text = sqlc.narg(q)
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN dl.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN dl.created_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'channel' AND NOT sqlc.arg(sort_desc)::boolean THEN dl.channel END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'channel' AND sqlc.arg(sort_desc)::boolean THEN dl.channel END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND NOT sqlc.arg(sort_desc)::boolean THEN dl.status END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND sqlc.arg(sort_desc)::boolean THEN dl.status END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'event' AND NOT sqlc.arg(sort_desc)::boolean THEN d.event_code END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'event' AND sqlc.arg(sort_desc)::boolean THEN d.event_code END DESC,
  dl.created_at DESC, dl.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountCustomerNotificationDeliveries :one
SELECT count(*)
FROM customer_notification_deliveries dl
JOIN customer_notification_dispatches d ON d.organization_id = dl.organization_id AND d.id = dl.dispatch_id
JOIN customers c ON c.organization_id = d.organization_id AND c.id = d.customer_id
LEFT JOIN reservations r ON r.organization_id = d.organization_id AND r.id = d.reservation_id
WHERE dl.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(channels)::text[] IS NULL OR dl.channel = ANY(sqlc.narg(channels)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR dl.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(events)::text[] IS NULL OR d.event_code = ANY(sqlc.narg(events)::text[]))
  AND (sqlc.narg(customer_uuid)::uuid IS NULL OR c.uuid = sqlc.narg(customer_uuid))
  AND (sqlc.narg(reservation_uuid)::uuid IS NULL OR r.uuid = sqlc.narg(reservation_uuid))
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR dl.created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR dl.created_at < sqlc.narg(created_to))
  AND (
    sqlc.narg(q)::text IS NULL
    OR r.number::text = sqlc.narg(q)
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
  );
