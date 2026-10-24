-- Customer panel "Paketlerim" and bookings with a package (TEC-984). Like
-- customer_panel.sql, every read starts from the identity's verified links
-- (customer_identity_links) and joins each tenant table on the link's
-- organization_id.

-- name: ListCustomerPanelPackages :many
-- The person's session packages in every business, under the same
-- visibility window as the visits (on the sale time). state is the state
-- on the business day in the business's time zone.
SELECT cp.organization_id, cp.id, cp.uuid, cp.name, cp.service_name, cp.total_sessions, cp.price, cp.currency,
    cp.sold_on, cp.expires_on, cp.status, cp.refunded_on, cp.created_at,
    (cp.branch_ids IS NULL)::BOOLEAN AS all_branches,
    ARRAY(
        SELECT ab.uuid FROM branches ab
        WHERE ab.organization_id = cp.organization_id AND ab.id = ANY(cp.branch_ids) AND ab.deleted_at IS NULL
        ORDER BY ab.name
    )::UUID[] AS branch_uuids,
    ARRAY(
        SELECT ab.name FROM branches ab
        WHERE ab.organization_id = cp.organization_id AND ab.id = ANY(cp.branch_ids) AND ab.deleted_at IS NULL
        ORDER BY ab.name
    )::TEXT[] AS branch_names,
    o.uuid AS business_uuid, o.slug AS business_slug, o.name AS business_name,
    b.uuid AS branch_uuid, b.name AS branch_name, s.uuid AS service_uuid,
    COALESCE(u.consumed, 0)::INT AS consumed_count, COALESCE(u.held, 0)::INT AS held_count,
    (CASE
        WHEN cp.status = 'refunded' THEN 'refunded'
        WHEN COALESCE(u.consumed, 0) >= cp.total_sessions THEN 'used_up'
        WHEN cp.expires_on IS NOT NULL AND cp.expires_on < (sqlc.arg(now)::TIMESTAMPTZ AT TIME ZONE o.timezone)::DATE THEN 'expired'
        ELSE 'active' END)::TEXT AS state
FROM customer_identity_links l
JOIN customer_packages cp ON cp.organization_id = l.organization_id AND cp.customer_id = l.customer_id
JOIN organizations o ON o.id = cp.organization_id AND o.deleted_at IS NULL
JOIN branches b ON b.organization_id = cp.organization_id AND b.id = cp.branch_id
LEFT JOIN services s ON s.organization_id = cp.organization_id AND s.id = cp.service_id
LEFT JOIN LATERAL (
    SELECT COUNT(*) FILTER (WHERE ps.status = 'consumed') AS consumed,
           COUNT(*) FILTER (WHERE ps.status = 'held') AS held
    FROM package_sessions ps
    WHERE ps.organization_id = cp.organization_id AND ps.customer_package_id = cp.id
) u ON TRUE
WHERE l.identity_id = sqlc.arg(identity_id)
  AND (l.visible_from IS NULL OR cp.created_at >= l.visible_from)
  AND (l.visible_until IS NULL OR cp.created_at < l.visible_until)
  AND (sqlc.narg(business_uuid)::UUID IS NULL OR o.uuid = sqlc.narg(business_uuid)::UUID)
  AND (sqlc.narg(package_uuid)::UUID IS NULL OR cp.uuid = sqlc.narg(package_uuid)::UUID)
  AND (sqlc.narg(states)::TEXT[] IS NULL OR (CASE
        WHEN cp.status = 'refunded' THEN 'refunded'
        WHEN COALESCE(u.consumed, 0) >= cp.total_sessions THEN 'used_up'
        WHEN cp.expires_on IS NOT NULL AND cp.expires_on < (sqlc.arg(now)::TIMESTAMPTZ AT TIME ZONE o.timezone)::DATE THEN 'expired'
        ELSE 'active' END) = ANY(sqlc.narg(states)::TEXT[]))
ORDER BY
    (CASE
        WHEN cp.status = 'refunded' THEN 3
        WHEN COALESCE(u.consumed, 0) >= cp.total_sessions THEN 2
        WHEN cp.expires_on IS NOT NULL AND cp.expires_on < (sqlc.arg(now)::TIMESTAMPTZ AT TIME ZONE o.timezone)::DATE THEN 2
        ELSE 1 END),
    cp.expires_on ASC NULLS LAST, cp.sold_on DESC, cp.organization_id, cp.id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountCustomerPanelPackages :one
SELECT COUNT(*)::BIGINT
FROM customer_identity_links l
JOIN customer_packages cp ON cp.organization_id = l.organization_id AND cp.customer_id = l.customer_id
JOIN organizations o ON o.id = cp.organization_id AND o.deleted_at IS NULL
LEFT JOIN LATERAL (
    SELECT COUNT(*) FILTER (WHERE ps.status = 'consumed') AS consumed
    FROM package_sessions ps
    WHERE ps.organization_id = cp.organization_id AND ps.customer_package_id = cp.id
) u ON TRUE
WHERE l.identity_id = sqlc.arg(identity_id)
  AND (l.visible_from IS NULL OR cp.created_at >= l.visible_from)
  AND (l.visible_until IS NULL OR cp.created_at < l.visible_until)
  AND (sqlc.narg(business_uuid)::UUID IS NULL OR o.uuid = sqlc.narg(business_uuid)::UUID)
  AND (sqlc.narg(states)::TEXT[] IS NULL OR (CASE
        WHEN cp.status = 'refunded' THEN 'refunded'
        WHEN COALESCE(u.consumed, 0) >= cp.total_sessions THEN 'used_up'
        WHEN cp.expires_on IS NOT NULL AND cp.expires_on < (sqlc.arg(now)::TIMESTAMPTZ AT TIME ZONE o.timezone)::DATE THEN 'expired'
        ELSE 'active' END) = ANY(sqlc.narg(states)::TEXT[]));

-- name: ListCustomerPanelPackageSessions :many
-- The sessions of one package that were booked or used (released ones are
-- left out), latest visit first.
SELECT ps.uuid, ps.session_no, ps.status, ps.consumed_on, ps.created_at,
    r.uuid AS reservation_uuid, r.number AS reservation_number, r.starts_at AS reservation_starts_at,
    r.status AS reservation_status, rl.service_name, rl.staff_name, b.name AS branch_name,
    COALESCE(NULLIF(b.timezone, ''), o.timezone)::TEXT AS timezone
FROM package_sessions ps
JOIN reservations r ON r.organization_id = ps.organization_id AND r.id = ps.reservation_id
JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
JOIN organizations o ON o.id = r.organization_id
LEFT JOIN reservation_lines rl ON rl.organization_id = ps.organization_id AND rl.reservation_id = ps.reservation_id
    AND rl.line_index = ps.line_index
WHERE ps.organization_id = sqlc.arg(organization_id) AND ps.customer_package_id = sqlc.arg(customer_package_id)
  AND ps.status <> 'released'
ORDER BY r.starts_at DESC, ps.id DESC;

-- name: GetCustomerPanelLinkedCustomer :one
-- The business's customer record a signed-in identity books as: its
-- current link (a link closed by a number change is not used).
SELECT c.id, c.uuid
FROM customer_identity_links l
JOIN customers c ON c.organization_id = l.organization_id AND c.id = l.customer_id AND c.deleted_at IS NULL
WHERE l.organization_id = sqlc.arg(organization_id) AND l.identity_id = sqlc.arg(identity_id)
  AND l.visible_until IS NULL;
