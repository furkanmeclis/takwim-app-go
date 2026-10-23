-- Mobile push devices (Expo) and their pending push tickets.

-- name: UpsertPushDevice :one
-- A token moves to the calling user when the device switches accounts and is
-- re-enabled on every registration.
INSERT INTO push_devices (user_id, token, platform, locale, app_version, device_name, last_seen_at)
VALUES (sqlc.arg(user_id), sqlc.arg(token), sqlc.arg(platform), sqlc.arg(locale), sqlc.arg(app_version), sqlc.arg(device_name), NOW())
ON CONFLICT (token) DO UPDATE SET
    user_id = EXCLUDED.user_id,
    platform = EXCLUDED.platform,
    locale = EXCLUDED.locale,
    app_version = EXCLUDED.app_version,
    device_name = EXCLUDED.device_name,
    last_seen_at = NOW(),
    disabled_at = NULL,
    disabled_reason = ''
RETURNING *;

-- name: DeletePushDeviceForUser :execrows
DELETE FROM push_devices
WHERE user_id = sqlc.arg(user_id) AND token = sqlc.arg(token);

-- name: ListActivePushDevicesByUser :many
SELECT * FROM push_devices
WHERE user_id = $1 AND disabled_at IS NULL
ORDER BY last_seen_at DESC, id DESC;

-- name: HasActivePushDevices :one
SELECT EXISTS (
    SELECT 1 FROM push_devices WHERE user_id = $1 AND disabled_at IS NULL
)::bool AS has_devices;

-- name: DisablePushDevice :exec
UPDATE push_devices
SET disabled_at = COALESCE(disabled_at, NOW()),
    disabled_reason = sqlc.arg(reason)
WHERE id = sqlc.arg(id);

-- name: InsertPushTicket :exec
INSERT INTO push_tickets (ticket_id, push_device_id, notification_id)
VALUES (sqlc.arg(ticket_id), sqlc.arg(push_device_id), sqlc.narg(notification_id))
ON CONFLICT (ticket_id) DO NOTHING;

-- name: ListDuePushTickets :many
SELECT id, ticket_id, push_device_id, created_at FROM push_tickets
WHERE created_at <= sqlc.arg(before_at)
ORDER BY created_at ASC, id ASC
LIMIT sqlc.arg(limit_count);

-- name: DeletePushTickets :exec
DELETE FROM push_tickets WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: EnsureNotificationPreferencesPushDefault :exec
-- First device registration opts the user into push (the OS permission
-- prompt is the consent). An existing preference row is left untouched.
INSERT INTO notification_preferences (user_id, push_enabled)
VALUES ($1, TRUE)
ON CONFLICT (user_id) DO NOTHING;
