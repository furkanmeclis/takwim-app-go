-- Reservation record (TEC-899). Every query filters by organization_id;
-- joins by alias.organization_id too.

-- name: NextReservationNumber :one
-- Runs under the capacity lock of the organization (uq number guards).
SELECT (COALESCE(MAX(number), 0) + 1)::BIGINT FROM reservations WHERE organization_id = sqlc.arg(organization_id);

-- name: GetReservationCustomer :one
SELECT id, uuid, first_name, last_name FROM customers
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid) AND deleted_at IS NULL;

-- name: GetReservationLineService :one
-- The service of a planned line with its prices: personal (service_staff) ›
-- branch offer (service_branches) › list price.
SELECT s.id, s.name, s.payment_mode, s.price AS service_price, sb.price AS branch_price, ss.price AS staff_price
FROM services s
LEFT JOIN service_branches sb
    ON sb.organization_id = s.organization_id AND sb.service_id = s.id AND sb.branch_id = sqlc.arg(branch_id)
LEFT JOIN service_staff ss
    ON ss.organization_id = s.organization_id AND ss.service_id = s.id AND ss.staff_id = sqlc.arg(staff_id)
WHERE s.organization_id = sqlc.arg(organization_id) AND s.uuid = sqlc.arg(uuid);

-- name: GetReservationCurrency :one
SELECT currency::TEXT FROM organizations WHERE id = sqlc.arg(organization_id);

-- name: CreateReservation :one
INSERT INTO reservations (
    organization_id, number, branch_id, customer_id, hold_id, status, approval_mode, approval_status,
    confirmation_status, payment_status, source, starts_at, ends_at, total_price, currency, custom_fields,
    customer_note, internal_note, created_by
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(number), sqlc.arg(branch_id), sqlc.arg(customer_id), sqlc.narg(hold_id),
    sqlc.arg(status), sqlc.arg(approval_mode), sqlc.arg(approval_status), sqlc.arg(confirmation_status),
    sqlc.arg(payment_status), sqlc.arg(source), sqlc.arg(starts_at), sqlc.arg(ends_at), sqlc.arg(total_price),
    sqlc.arg(currency), sqlc.arg(custom_fields), sqlc.arg(customer_note), sqlc.arg(internal_note), sqlc.narg(created_by)
)
RETURNING *;

-- name: CreateReservationLine :exec
INSERT INTO reservation_lines (
    organization_id, reservation_id, line_index, service_id, service_name, staff_id, staff_name, resources,
    starts_at, ends_at, duration_minutes, buffer_before_minutes, buffer_after_minutes, unit_price
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(reservation_id), sqlc.arg(line_index), sqlc.narg(service_id),
    sqlc.arg(service_name), sqlc.narg(staff_id), sqlc.narg(staff_name), sqlc.arg(resources), sqlc.arg(starts_at),
    sqlc.arg(ends_at), sqlc.arg(duration_minutes), sqlc.arg(buffer_before_minutes), sqlc.arg(buffer_after_minutes),
    sqlc.arg(unit_price)
);

-- name: CreateReservationEvent :exec
INSERT INTO reservation_events (organization_id, reservation_id, version, type, from_status, to_status, actor_id, note)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(reservation_id), sqlc.arg(version), sqlc.arg(type), sqlc.narg(from_status),
    sqlc.narg(to_status), sqlc.narg(actor_id), sqlc.arg(note)
);

-- name: GetReservation :one
SELECT r.*, b.uuid AS branch_uuid, b.name AS branch_name,
       c.uuid AS customer_uuid, c.first_name AS customer_first_name, c.last_name AS customer_last_name,
       h.expires_at AS hold_expires_at
FROM reservations r
JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
JOIN customers c ON c.organization_id = r.organization_id AND c.id = r.customer_id
LEFT JOIN capacity_holds h ON h.organization_id = r.organization_id AND h.id = r.hold_id AND h.status = 'active'
WHERE r.organization_id = sqlc.arg(organization_id) AND r.uuid = sqlc.arg(uuid);

-- name: GetReservationRecord :one
SELECT * FROM reservations
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: GetReservationForUpdate :one
SELECT * FROM reservations
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid)
FOR UPDATE;

-- name: ListReservationStaffIDs :many
SELECT DISTINCT staff_id::BIGINT FROM reservation_lines
WHERE organization_id = sqlc.arg(organization_id) AND reservation_id = sqlc.arg(reservation_id) AND staff_id IS NOT NULL;

-- name: ListReservationLines :many
SELECT l.*, s.uuid AS service_uuid, st.uuid AS staff_uuid
FROM reservation_lines l
LEFT JOIN services s ON s.organization_id = l.organization_id AND s.id = l.service_id
LEFT JOIN staff st ON st.organization_id = l.organization_id AND st.id = l.staff_id
WHERE l.organization_id = sqlc.arg(organization_id) AND l.reservation_id = sqlc.arg(reservation_id)
ORDER BY l.line_index;

-- name: ListReservationEvents :many
SELECT e.*, COALESCE(TRIM(u.name || ' ' || u.surname), '')::TEXT AS actor_name
FROM reservation_events e
LEFT JOIN users u ON u.id = e.actor_id
WHERE e.organization_id = sqlc.arg(organization_id) AND e.reservation_id = sqlc.arg(reservation_id)
ORDER BY e.id;

-- name: UpdateReservationState :one
-- One transition: the new fields and version + 1.
UPDATE reservations
SET status = sqlc.arg(status), approval_status = sqlc.arg(approval_status),
    payment_status = sqlc.arg(payment_status), cancel_reason = sqlc.narg(cancel_reason),
    version = version + 1
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: ListReservationsByHolds :many
-- Records waiting on holds that just lapsed (the expiry hook).
SELECT * FROM reservations
WHERE organization_id = sqlc.arg(organization_id) AND hold_id = ANY(sqlc.arg(hold_ids)::BIGINT[])
FOR UPDATE;
