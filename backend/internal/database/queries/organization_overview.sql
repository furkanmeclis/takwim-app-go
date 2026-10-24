-- Platform business overview (TEC-927): the 360 page of a business in
-- Takwim yönetimi. Every query names the organization; the caller resolves
-- it from the platform route's uuid.

-- name: CountOrganizationOverview :one
-- Catalogue sizes and the reservation volume of one business. Volume counts
-- bookings by the time they were taken (created_at); upcoming counts open
-- visits that start after now.
SELECT
    (SELECT COUNT(*) FROM customers c
     WHERE c.organization_id = sqlc.arg(organization_id) AND c.deleted_at IS NULL)::bigint AS customers,
    (SELECT COUNT(*) FROM services s
     WHERE s.organization_id = sqlc.arg(organization_id) AND s.deleted_at IS NULL)::bigint AS services,
    (SELECT COUNT(*) FROM reservations r
     WHERE r.organization_id = sqlc.arg(organization_id))::bigint AS reservations_total,
    (SELECT COUNT(*) FROM reservations r
     WHERE r.organization_id = sqlc.arg(organization_id)
       AND r.created_at >= sqlc.arg(month_start)::timestamptz
       AND r.created_at < sqlc.arg(month_end)::timestamptz)::bigint AS reservations_this_month,
    (SELECT COUNT(*) FROM reservations r
     WHERE r.organization_id = sqlc.arg(organization_id)
       AND r.created_at >= sqlc.arg(previous_month_start)::timestamptz
       AND r.created_at < sqlc.arg(month_start)::timestamptz)::bigint AS reservations_previous_month,
    (SELECT COUNT(*) FROM reservations r
     WHERE r.organization_id = sqlc.arg(organization_id)
       AND r.created_at >= sqlc.arg(month_start)::timestamptz
       AND r.created_at < sqlc.arg(month_end)::timestamptz
       AND r.status IN ('cancelled', 'rejected', 'no_show'))::bigint AS reservations_cancelled_this_month,
    (SELECT COUNT(*) FROM reservations r
     WHERE r.organization_id = sqlc.arg(organization_id)
       AND r.starts_at >= sqlc.arg(now)::timestamptz
       AND r.status IN ('requested', 'active'))::bigint AS reservations_upcoming;

-- name: ListOrganizationReservationVolume :many
-- Bookings taken per month (business time zone) since from_time.
SELECT to_char(date_trunc('month', r.created_at AT TIME ZONE sqlc.arg(timezone)::text), 'YYYY-MM')::text AS period,
    COUNT(*)::bigint AS reservations
FROM reservations r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.created_at >= sqlc.arg(from_time)::timestamptz
GROUP BY 1
ORDER BY 1;

-- name: CountOrganizationDeliveries :many
-- Customer notification attempts per channel and outcome in a period (a
-- channel that is not ready shows up as skipped, never as sent).
SELECT d.channel, d.status, COUNT(*)::bigint AS deliveries
FROM customer_notification_deliveries d
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.created_at >= sqlc.arg(from_time)::timestamptz
  AND d.created_at < sqlc.arg(to_time)::timestamptz
GROUP BY d.channel, d.status
ORDER BY d.channel, d.status;

-- name: CountOrganizationConfirmations :one
-- Confirmation tasks planned in a period, by outcome.
SELECT
    COUNT(*)::bigint AS total,
    COUNT(*) FILTER (WHERE rc.status = 'confirmed')::bigint AS confirmed,
    COUNT(*) FILTER (WHERE rc.status = 'declined')::bigint AS declined,
    COUNT(*) FILTER (WHERE rc.status = 'expired')::bigint AS expired,
    COUNT(*) FILTER (WHERE rc.status IN ('scheduled', 'pending'))::bigint AS open
FROM reservation_confirmations rc
WHERE rc.organization_id = sqlc.arg(organization_id)
  AND rc.created_at >= sqlc.arg(from_time)::timestamptz
  AND rc.created_at < sqlc.arg(to_time)::timestamptz;

-- name: ListOrganizationOverviewAccounts :many
-- Every connected provider account of a business (no secrets).
SELECT a.uuid, a.kind, a.provider, a.display_name, a.mode, a.status, a.last_checked_at, a.last_check_status
FROM integration_accounts a
WHERE a.organization_id = sqlc.arg(organization_id)
ORDER BY a.kind, (a.status = 'active') DESC, a.updated_at DESC, a.id DESC;

