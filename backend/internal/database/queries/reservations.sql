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
    customer_note, internal_note, created_by, party_size, zone
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(number), sqlc.arg(branch_id), sqlc.arg(customer_id), sqlc.narg(hold_id),
    sqlc.arg(status), sqlc.arg(approval_mode), sqlc.arg(approval_status), sqlc.arg(confirmation_status),
    sqlc.arg(payment_status), sqlc.arg(source), sqlc.arg(starts_at), sqlc.arg(ends_at), sqlc.arg(total_price),
    sqlc.arg(currency), sqlc.arg(custom_fields), sqlc.arg(customer_note), sqlc.arg(internal_note), sqlc.narg(created_by),
    sqlc.narg(party_size), sqlc.arg(zone)
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

-- name: ListReservations :many
-- The panel list (TEC-900). branch_ids = the caller's branch scope (NULL =
-- every branch); own_user_id / own_staff_id narrow to an own calendar
-- member (created by the member or served by its staff record).
SELECT r.*, b.uuid AS branch_uuid, b.name AS branch_name,
       c.uuid AS customer_uuid, c.first_name AS customer_first_name, c.last_name AS customer_last_name,
       c.phone_e164 AS customer_phone,
       COALESCE((SELECT string_agg(l.service_name, ', ' ORDER BY l.line_index) FROM reservation_lines l
                 WHERE l.organization_id = r.organization_id AND l.reservation_id = r.id), '')::TEXT AS services,
       COALESCE((SELECT string_agg(DISTINCT l.staff_name, ', ') FROM reservation_lines l
                 WHERE l.organization_id = r.organization_id AND l.reservation_id = r.id AND l.staff_name IS NOT NULL), '')::TEXT AS staff_names
FROM reservations r
JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
JOIN customers c ON c.organization_id = r.organization_id AND c.id = r.customer_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(own_user_id)::bigint IS NULL OR r.created_by = sqlc.narg(own_user_id) OR EXISTS (
        SELECT 1 FROM reservation_lines ol
        WHERE ol.organization_id = r.organization_id AND ol.reservation_id = r.id AND ol.staff_id = sqlc.narg(own_staff_id)))
  AND (sqlc.narg(branches)::uuid[] IS NULL OR b.uuid = ANY(sqlc.narg(branches)::uuid[]))
  AND (sqlc.narg(customer_uuid)::uuid IS NULL OR c.uuid = sqlc.narg(customer_uuid))
  AND (sqlc.narg(staff)::uuid[] IS NULL OR EXISTS (
        SELECT 1 FROM reservation_lines sl
        JOIN staff st ON st.organization_id = sl.organization_id AND st.id = sl.staff_id
        WHERE sl.organization_id = r.organization_id AND sl.reservation_id = r.id AND st.uuid = ANY(sqlc.narg(staff)::uuid[])))
  AND (sqlc.narg(statuses)::text[] IS NULL OR r.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(approval_statuses)::text[] IS NULL OR r.approval_status = ANY(sqlc.narg(approval_statuses)::text[]))
  AND (sqlc.narg(payment_statuses)::text[] IS NULL OR r.payment_status = ANY(sqlc.narg(payment_statuses)::text[]))
  AND (sqlc.narg(sources)::text[] IS NULL OR r.source = ANY(sqlc.narg(sources)::text[]))
  AND (sqlc.narg(starts_from)::timestamptz IS NULL OR r.starts_at >= sqlc.narg(starts_from))
  AND (sqlc.narg(starts_to)::timestamptz IS NULL OR r.starts_at < sqlc.narg(starts_to))
  AND (
    sqlc.narg(q)::text IS NULL
    OR r.number::text = sqlc.narg(q)
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
    OR (sqlc.narg(q_digits)::text IS NOT NULL AND c.phone_e164 LIKE '%' || sqlc.narg(q_digits) || '%')
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'starts_at' AND NOT sqlc.arg(sort_desc)::boolean THEN r.starts_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'starts_at' AND sqlc.arg(sort_desc)::boolean THEN r.starts_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'number' AND NOT sqlc.arg(sort_desc)::boolean THEN r.number END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'number' AND sqlc.arg(sort_desc)::boolean THEN r.number END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'customer' AND NOT sqlc.arg(sort_desc)::boolean THEN lower(c.first_name || ' ' || c.last_name) END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'customer' AND sqlc.arg(sort_desc)::boolean THEN lower(c.first_name || ' ' || c.last_name) END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND NOT sqlc.arg(sort_desc)::boolean THEN r.status END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND sqlc.arg(sort_desc)::boolean THEN r.status END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'total_price' AND NOT sqlc.arg(sort_desc)::boolean THEN r.total_price END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'total_price' AND sqlc.arg(sort_desc)::boolean THEN r.total_price END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN r.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN r.created_at END DESC,
  r.starts_at DESC, r.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountReservations :one
SELECT count(*)
FROM reservations r
JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
JOIN customers c ON c.organization_id = r.organization_id AND c.id = r.customer_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(own_user_id)::bigint IS NULL OR r.created_by = sqlc.narg(own_user_id) OR EXISTS (
        SELECT 1 FROM reservation_lines ol
        WHERE ol.organization_id = r.organization_id AND ol.reservation_id = r.id AND ol.staff_id = sqlc.narg(own_staff_id)))
  AND (sqlc.narg(branches)::uuid[] IS NULL OR b.uuid = ANY(sqlc.narg(branches)::uuid[]))
  AND (sqlc.narg(customer_uuid)::uuid IS NULL OR c.uuid = sqlc.narg(customer_uuid))
  AND (sqlc.narg(staff)::uuid[] IS NULL OR EXISTS (
        SELECT 1 FROM reservation_lines sl
        JOIN staff st ON st.organization_id = sl.organization_id AND st.id = sl.staff_id
        WHERE sl.organization_id = r.organization_id AND sl.reservation_id = r.id AND st.uuid = ANY(sqlc.narg(staff)::uuid[])))
  AND (sqlc.narg(statuses)::text[] IS NULL OR r.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(approval_statuses)::text[] IS NULL OR r.approval_status = ANY(sqlc.narg(approval_statuses)::text[]))
  AND (sqlc.narg(payment_statuses)::text[] IS NULL OR r.payment_status = ANY(sqlc.narg(payment_statuses)::text[]))
  AND (sqlc.narg(sources)::text[] IS NULL OR r.source = ANY(sqlc.narg(sources)::text[]))
  AND (sqlc.narg(starts_from)::timestamptz IS NULL OR r.starts_at >= sqlc.narg(starts_from))
  AND (sqlc.narg(starts_to)::timestamptz IS NULL OR r.starts_at < sqlc.narg(starts_to))
  AND (
    sqlc.narg(q)::text IS NULL
    OR r.number::text = sqlc.narg(q)
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
    OR (sqlc.narg(q_digits)::text IS NOT NULL AND c.phone_e164 LIKE '%' || sqlc.narg(q_digits) || '%')
  );

-- name: ListReservationBranchFacets :many
-- Branches with reservations (in the caller's scope) for the list filter.
SELECT b.uuid AS value, b.name AS label, count(*) AS count
FROM reservations r
JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY b.uuid, b.name
ORDER BY b.name;

-- name: ListReservationStaffFacets :many
SELECT st.uuid AS value, st.name AS label, count(DISTINCT r.id) AS count
FROM reservations r
JOIN reservation_lines l ON l.organization_id = r.organization_id AND l.reservation_id = r.id
JOIN staff st ON st.organization_id = l.organization_id AND st.id = l.staff_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY st.uuid, st.name
ORDER BY st.name;

-- name: ReplaceReservationSchedule :one
-- A change of time / services / staff: the new commitment and times.
UPDATE reservations
SET hold_id = sqlc.arg(hold_id), starts_at = sqlc.arg(starts_at), ends_at = sqlc.arg(ends_at),
    total_price = sqlc.arg(total_price), party_size = sqlc.narg(party_size), zone = sqlc.arg(zone),
    version = version + 1
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: DeleteReservationLines :exec
DELETE FROM reservation_lines WHERE organization_id = sqlc.arg(organization_id) AND reservation_id = sqlc.arg(reservation_id);

-- name: ListReservationLinePrices :many
SELECT line_index, service_id, staff_id, unit_price FROM reservation_lines
WHERE organization_id = sqlc.arg(organization_id) AND reservation_id = sqlc.arg(reservation_id)
ORDER BY line_index;

-- name: UpdateReservationNotes :one
UPDATE reservations
SET customer_note = sqlc.arg(customer_note), internal_note = sqlc.arg(internal_note),
    custom_fields = sqlc.arg(custom_fields), version = version + 1
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: ListCalendarBranches :many
-- Branches of the calendar (TEC-901): one branch, or every branch of the
-- caller's scope (branch_ids NULL = all).
SELECT b.id, b.uuid, b.name, b.working_hours, b.holiday_exceptions,
       COALESCE(NULLIF(b.timezone, ''), o.timezone)::TEXT AS timezone
FROM branches b
JOIN organizations o ON o.id = b.organization_id
WHERE b.organization_id = sqlc.arg(organization_id) AND b.deleted_at IS NULL AND b.is_active
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR b.id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(branch_uuid)::uuid IS NULL OR b.uuid = sqlc.narg(branch_uuid))
ORDER BY b.name, b.id;

-- name: ListCalendarStaff :many
-- Active people working in a branch, with their shifts there.
SELECT s.id, s.uuid, s.name, s.color, sb.shifts
FROM staff s
JOIN staff_branches sb ON sb.organization_id = s.organization_id AND sb.staff_id = s.id
WHERE s.organization_id = sqlc.arg(organization_id) AND sb.branch_id = sqlc.arg(branch_id)
  AND s.deleted_at IS NULL AND s.is_active
ORDER BY s.name, s.id;

-- name: ListCalendarResources :many
-- Active resources of a branch (own hours NULL = branch hours).
SELECT id, uuid, name, kind, working_hours
FROM resources
WHERE organization_id = sqlc.arg(organization_id) AND branch_id = sqlc.arg(branch_id)
  AND deleted_at IS NULL AND is_active
ORDER BY sort_order, name, id;

-- name: ListCalendarLines :many
-- Lines overlapping [since, until) of the calendar's branches, with their
-- visit. Cancelled and rejected visits only when asked; an own calendar
-- member sees what it created or serves.
SELECT l.line_index, l.service_name, l.staff_name, l.resources, l.starts_at, l.ends_at,
       l.buffer_before_minutes, l.buffer_after_minutes,
       st.uuid AS staff_uuid,
       r.uuid, r.number, r.status, r.approval_status, r.confirmation_status, r.payment_status,
       r.source, r.version, r.party_size,
       b.uuid AS branch_uuid,
       c.first_name AS customer_first_name, c.last_name AS customer_last_name
FROM reservation_lines l
JOIN reservations r ON r.organization_id = l.organization_id AND r.id = l.reservation_id
JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
JOIN customers c ON c.organization_id = r.organization_id AND c.id = r.customer_id
LEFT JOIN staff st ON st.organization_id = l.organization_id AND st.id = l.staff_id
WHERE l.organization_id = sqlc.arg(organization_id)
  AND r.branch_id = ANY(sqlc.arg(branch_ids)::bigint[])
  AND l.starts_at < sqlc.arg(until) AND l.ends_at > sqlc.arg(since)
  AND (sqlc.arg(include_cancelled)::boolean OR r.status NOT IN ('cancelled', 'rejected'))
  AND (sqlc.narg(own_user_id)::bigint IS NULL OR r.created_by = sqlc.narg(own_user_id) OR l.staff_id = sqlc.narg(own_staff_id))
ORDER BY l.starts_at, r.number, l.line_index;

-- name: GetAgendaStaff :one
-- The staff record of a member (TEC-902 agenda).
SELECT id, uuid, name, color FROM staff
WHERE organization_id = sqlc.arg(organization_id) AND user_id = sqlc.arg(user_id) AND deleted_at IS NULL;

-- name: ListAgendaShifts :many
-- The branches a person works in (inside the caller's branch scope), with
-- the person's shifts and the branch hours.
SELECT b.id, b.uuid, b.name, b.working_hours, b.holiday_exceptions,
       COALESCE(NULLIF(b.timezone, ''), o.timezone)::TEXT AS timezone, sb.shifts
FROM staff_branches sb
JOIN branches b ON b.organization_id = sb.organization_id AND b.id = sb.branch_id
JOIN organizations o ON o.id = b.organization_id
WHERE sb.organization_id = sqlc.arg(organization_id) AND sb.staff_id = sqlc.arg(staff_id)
  AND b.deleted_at IS NULL AND b.is_active
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR b.id = ANY(sqlc.narg(branch_ids)::bigint[]))
ORDER BY b.name, b.id;

-- name: ListAgendaLines :many
-- Lines a person serves overlapping [since, until), in the caller's branch
-- scope, with their visit.
SELECT l.line_index, l.service_name, l.staff_name, l.resources, l.starts_at, l.ends_at,
       l.buffer_before_minutes, l.buffer_after_minutes,
       r.uuid, r.number, r.status, r.approval_status, r.confirmation_status, r.payment_status,
       r.source, r.version, r.party_size, r.customer_note,
       b.uuid AS branch_uuid, b.name AS branch_name,
       c.first_name AS customer_first_name, c.last_name AS customer_last_name, c.phone_e164 AS customer_phone
FROM reservation_lines l
JOIN reservations r ON r.organization_id = l.organization_id AND r.id = l.reservation_id
JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
JOIN customers c ON c.organization_id = r.organization_id AND c.id = r.customer_id
WHERE l.organization_id = sqlc.arg(organization_id) AND l.staff_id = sqlc.arg(staff_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND l.starts_at < sqlc.arg(until) AND l.ends_at > sqlc.arg(since)
  AND (sqlc.arg(include_cancelled)::boolean OR r.status NOT IN ('cancelled', 'rejected'))
ORDER BY l.starts_at, r.number, l.line_index;
