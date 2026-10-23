-- Shared customer panel (TEC-909). customer_otp_challenges and
-- customer_sessions are platform-level. Every read of business data starts
-- from the identity's verified links (customer_identity_links), never from
-- a phone typed by the caller, and joins each tenant table on the link's
-- organization_id.

-- name: CreateCustomerOTPChallenge :one
INSERT INTO customer_otp_challenges (uuid, phone_e164, code_hash, sent, max_attempts, expires_at, ip, created_at)
VALUES (
    sqlc.arg(uuid), sqlc.arg(phone_e164), sqlc.arg(code_hash), sqlc.arg(sent),
    sqlc.arg(max_attempts), sqlc.arg(expires_at), sqlc.arg(ip), sqlc.arg(created_at)
)
RETURNING *;

-- name: GetLatestCustomerOTPChallenge :one
SELECT * FROM customer_otp_challenges
WHERE phone_e164 = sqlc.arg(phone_e164)
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: CountCustomerOTPChallengesSince :one
SELECT COUNT(*)::BIGINT FROM customer_otp_challenges
WHERE phone_e164 = sqlc.arg(phone_e164) AND created_at >= sqlc.arg(since);

-- name: CloseOpenCustomerOTPChallenges :exec
-- A new code replaces the open ones: only the latest code works.
UPDATE customer_otp_challenges SET consumed_at = sqlc.arg(now)
WHERE phone_e164 = sqlc.arg(phone_e164) AND consumed_at IS NULL;

-- name: GetOpenCustomerOTPChallenge :one
-- The open code of a phone. Attempts are counted and the code consumed
-- with single atomic updates, so no lock is held.
SELECT * FROM customer_otp_challenges
WHERE phone_e164 = sqlc.arg(phone_e164)
  AND consumed_at IS NULL
  AND expires_at > sqlc.arg(now)
  AND attempts < max_attempts
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: CountCustomerOTPAttempt :one
UPDATE customer_otp_challenges SET attempts = attempts + 1
WHERE id = sqlc.arg(id)
RETURNING attempts, max_attempts;

-- name: ConsumeCustomerOTPChallenge :execrows
UPDATE customer_otp_challenges SET consumed_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND consumed_at IS NULL;

-- name: DeleteStaleCustomerOTPChallenges :execrows
DELETE FROM customer_otp_challenges WHERE created_at < sqlc.arg(before);

-- name: CustomerPhoneKnown :one
-- Whether a live business keeps a live customer record with the phone, or
-- the phone was verified before. Codes go only to such phones (no SMS to
-- arbitrary numbers); the answer to the caller is the same either way.
SELECT (
    EXISTS (
        SELECT 1 FROM customers c
        JOIN organizations o ON o.id = c.organization_id AND o.deleted_at IS NULL
        WHERE c.phone_e164 = sqlc.arg(phone_e164) AND c.deleted_at IS NULL
    )
    OR EXISTS (SELECT 1 FROM customer_identities i WHERE i.phone_e164 = sqlc.arg(phone_e164))
)::BOOLEAN;

-- name: ListUnlinkedCustomersByPhone :many
-- Live records with the proven phone that are not linked yet, in every live
-- business. The caller links each one with customers.LinkVerified.
SELECT c.organization_id, c.uuid
FROM customers c
JOIN organizations o ON o.id = c.organization_id AND o.deleted_at IS NULL
LEFT JOIN customer_identity_links l ON l.organization_id = c.organization_id AND l.customer_id = c.id
WHERE c.phone_e164 = sqlc.arg(phone_e164) AND c.deleted_at IS NULL AND l.id IS NULL
ORDER BY c.organization_id, c.id;

-- name: CreateCustomerSession :one
INSERT INTO customer_sessions (identity_id, phone_e164, verified_at, ip, user_agent, last_seen_at, idle_expires_at, expires_at)
VALUES (
    sqlc.arg(identity_id), sqlc.arg(phone_e164), sqlc.arg(verified_at), sqlc.arg(ip), sqlc.arg(user_agent),
    sqlc.arg(verified_at), sqlc.arg(idle_expires_at), sqlc.arg(expires_at)
)
RETURNING *;

-- name: GetCustomerSession :one
SELECT s.*, i.uuid AS identity_uuid, i.phone_e164 AS identity_phone
FROM customer_sessions s
JOIN customer_identities i ON i.id = s.identity_id
WHERE s.uuid = sqlc.arg(uuid);

-- name: TouchCustomerSession :exec
UPDATE customer_sessions SET last_seen_at = sqlc.arg(now), idle_expires_at = sqlc.arg(idle_expires_at)
WHERE id = sqlc.arg(id) AND revoked_at IS NULL;

-- name: RevokeCustomerSession :execrows
UPDATE customer_sessions SET revoked_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND revoked_at IS NULL;

-- name: RevokeCustomerSessionsOfIdentity :execrows
UPDATE customer_sessions SET revoked_at = sqlc.arg(now)
WHERE identity_id = sqlc.arg(identity_id) AND revoked_at IS NULL;

-- name: CountActiveCustomerSessions :one
SELECT COUNT(*)::BIGINT FROM customer_sessions
WHERE identity_id = sqlc.arg(identity_id) AND revoked_at IS NULL
  AND expires_at > sqlc.arg(now) AND idle_expires_at > sqlc.arg(now);

-- name: ListCustomerPanelBusinesses :many
-- The businesses that linked the identity, with the person's name as each
-- business keeps it.
SELECT o.uuid, o.slug, o.name, c.first_name, c.last_name, l.verified_at,
    (SELECT COUNT(*) FROM reservations r
     WHERE r.organization_id = l.organization_id AND r.customer_id = l.customer_id)::BIGINT AS reservation_count
FROM customer_identity_links l
JOIN organizations o ON o.id = l.organization_id AND o.deleted_at IS NULL
JOIN customers c ON c.organization_id = l.organization_id AND c.id = l.customer_id
WHERE l.identity_id = sqlc.arg(identity_id)
ORDER BY o.name, o.id;

-- name: ListCustomerPanelReservations :many
SELECT r.uuid, r.number, r.status, r.approval_status, r.confirmation_status, r.payment_status,
    r.starts_at, r.ends_at, r.total_price, r.currency, r.cancel_reason, r.created_at,
    o.uuid AS business_uuid, o.slug AS business_slug, o.name AS business_name,
    b.uuid AS branch_uuid, b.name AS branch_name,
    COALESCE(NULLIF(b.timezone, ''), o.timezone)::TEXT AS timezone,
    ARRAY(
        SELECT rl.service_name FROM reservation_lines rl
        WHERE rl.organization_id = r.organization_id AND rl.reservation_id = r.id
        ORDER BY rl.line_index
    )::TEXT[] AS service_names,
    ARRAY(
        SELECT DISTINCT rs.staff_name FROM reservation_lines rs
        WHERE rs.organization_id = r.organization_id AND rs.reservation_id = r.id AND rs.staff_name IS NOT NULL
        ORDER BY rs.staff_name
    )::TEXT[] AS staff_names
FROM customer_identity_links l
JOIN reservations r ON r.organization_id = l.organization_id AND r.customer_id = l.customer_id
JOIN organizations o ON o.id = r.organization_id AND o.deleted_at IS NULL
JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
WHERE l.identity_id = sqlc.arg(identity_id)
  AND (sqlc.narg(upcoming)::BOOLEAN IS NULL
       OR sqlc.narg(upcoming)::BOOLEAN = (r.status IN ('requested', 'active', 'in_service') AND r.ends_at > sqlc.arg(now)))
  AND (sqlc.narg(business_uuid)::UUID IS NULL OR o.uuid = sqlc.narg(business_uuid)::UUID)
  AND (sqlc.narg(statuses)::TEXT[] IS NULL OR r.status = ANY(sqlc.narg(statuses)::TEXT[]))
  -- Payments view: leave out visits that ended without money moving.
  AND (NOT sqlc.arg(with_money)::BOOLEAN
       OR NOT (r.status IN ('cancelled', 'rejected') AND r.payment_status IN ('on_site', 'online_pending', 'failed')))
ORDER BY
    CASE WHEN sqlc.arg(sort_desc)::BOOLEAN THEN r.starts_at END DESC,
    CASE WHEN NOT sqlc.arg(sort_desc)::BOOLEAN THEN r.starts_at END ASC,
    r.organization_id, r.id
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountCustomerPanelReservations :one
SELECT COUNT(*)::BIGINT
FROM customer_identity_links l
JOIN reservations r ON r.organization_id = l.organization_id AND r.customer_id = l.customer_id
JOIN organizations o ON o.id = r.organization_id AND o.deleted_at IS NULL
WHERE l.identity_id = sqlc.arg(identity_id)
  AND (sqlc.narg(upcoming)::BOOLEAN IS NULL
       OR sqlc.narg(upcoming)::BOOLEAN = (r.status IN ('requested', 'active', 'in_service') AND r.ends_at > sqlc.arg(now)))
  AND (sqlc.narg(business_uuid)::UUID IS NULL OR o.uuid = sqlc.narg(business_uuid)::UUID)
  AND (sqlc.narg(statuses)::TEXT[] IS NULL OR r.status = ANY(sqlc.narg(statuses)::TEXT[]))
  AND (NOT sqlc.arg(with_money)::BOOLEAN
       OR NOT (r.status IN ('cancelled', 'rejected') AND r.payment_status IN ('on_site', 'online_pending', 'failed')));

-- name: CustomerPanelPaymentTotals :many
-- Amounts per currency, payment state and whether the visit still stands
-- (cancelled / rejected visits count only for refunds). Currencies are
-- never added together.
SELECT r.currency, r.payment_status, (r.status NOT IN ('cancelled', 'rejected'))::BOOLEAN AS standing,
    COUNT(*)::BIGINT AS reservation_count, COALESCE(SUM(r.total_price), 0)::NUMERIC(14, 2) AS amount
FROM customer_identity_links l
JOIN reservations r ON r.organization_id = l.organization_id AND r.customer_id = l.customer_id
JOIN organizations o ON o.id = r.organization_id AND o.deleted_at IS NULL
WHERE l.identity_id = sqlc.arg(identity_id)
GROUP BY r.currency, r.payment_status, standing
ORDER BY r.currency, r.payment_status, standing;

-- name: GetCustomerPanelReservation :one
-- One reservation of the identity: its business and branch context. Any
-- uuid outside the identity's links answers no rows.
SELECT r.organization_id, r.id, r.uuid, r.customer_id,
    o.uuid AS business_uuid, o.slug AS business_slug, o.name AS business_name, o.status AS business_status,
    o.access_starts_at, o.access_ends_at, o.sector, o.plan, o.addons, o.branch_limit, o.staff_limit,
    b.address AS branch_address, b.city AS branch_city, b.district AS branch_district, b.phone AS branch_phone,
    COALESCE(NULLIF(b.timezone, ''), o.timezone)::TEXT AS timezone
FROM customer_identity_links l
JOIN reservations r ON r.organization_id = l.organization_id AND r.customer_id = l.customer_id
JOIN organizations o ON o.id = r.organization_id AND o.deleted_at IS NULL
JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
WHERE l.identity_id = sqlc.arg(identity_id) AND r.uuid = sqlc.arg(uuid)
ORDER BY r.organization_id
LIMIT 1;
