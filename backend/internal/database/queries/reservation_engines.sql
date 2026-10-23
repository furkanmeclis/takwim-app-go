-- Reservation engines (TEC-963): Cmd+K documents. Every query filters by
-- organization_id; joins by alias.organization_id too.

-- name: ListReservationSearchDocuments :many
-- owner_user_ids: the member who booked it and the accounts of the staff
-- members serving it (the own calendar reach of reservations).
SELECT r.uuid, r.organization_id, r.number, r.branch_id, r.starts_at,
    c.first_name AS customer_first_name, c.last_name AS customer_last_name, c.phone_e164 AS customer_phone,
    b.name AS branch_name, COALESCE(b.timezone, o.timezone)::TEXT AS timezone, o.slug AS organization_slug,
    ARRAY(
        SELECT DISTINCT u.uid FROM (
            SELECT r.created_by AS uid
            UNION
            SELECT st.user_id FROM reservation_lines l
            JOIN staff st ON st.organization_id = l.organization_id AND st.id = l.staff_id
            WHERE l.organization_id = r.organization_id AND l.reservation_id = r.id
        ) u WHERE u.uid IS NOT NULL
    )::BIGINT[] AS owner_user_ids
FROM reservations r
JOIN organizations o ON o.id = r.organization_id
JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
JOIN customers c ON c.organization_id = r.organization_id AND c.id = r.customer_id
WHERE r.organization_id = sqlc.arg(organization_id)
ORDER BY r.id;

-- name: GetReservationSearchDocument :one
SELECT r.uuid, r.organization_id, r.number, r.branch_id, r.starts_at,
    c.first_name AS customer_first_name, c.last_name AS customer_last_name, c.phone_e164 AS customer_phone,
    b.name AS branch_name, COALESCE(b.timezone, o.timezone)::TEXT AS timezone, o.slug AS organization_slug,
    ARRAY(
        SELECT DISTINCT u.uid FROM (
            SELECT r.created_by AS uid
            UNION
            SELECT st.user_id FROM reservation_lines l
            JOIN staff st ON st.organization_id = l.organization_id AND st.id = l.staff_id
            WHERE l.organization_id = r.organization_id AND l.reservation_id = r.id
        ) u WHERE u.uid IS NOT NULL
    )::BIGINT[] AS owner_user_ids
FROM reservations r
JOIN organizations o ON o.id = r.organization_id
JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
JOIN customers c ON c.organization_id = r.organization_id AND c.id = r.customer_id
WHERE r.organization_id = sqlc.arg(organization_id) AND r.uuid = sqlc.arg(uuid);

