-- TEC-920: multi-session packages (definitions, packages sold to
-- customers, their sessions on reservations). Every query takes the
-- organization id.

-- ------------------------------------------------------------- definitions

-- name: InsertSessionPackage :one
INSERT INTO session_packages (
    organization_id, name, description, service_id, total_sessions, price, validity_days, all_branches, is_active, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(name), sqlc.arg(description), sqlc.arg(service_id), sqlc.arg(total_sessions),
    sqlc.arg(price), sqlc.narg(validity_days), sqlc.arg(all_branches), sqlc.arg(is_active), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: UpdateSessionPackage :execrows
UPDATE session_packages SET
    name = sqlc.arg(name), description = sqlc.arg(description), service_id = sqlc.arg(service_id),
    total_sessions = sqlc.arg(total_sessions), price = sqlc.arg(price), validity_days = sqlc.narg(validity_days),
    all_branches = sqlc.arg(all_branches), is_active = sqlc.arg(is_active)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: SoftDeleteSessionPackage :execrows
UPDATE session_packages SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: DeleteSessionPackageBranches :exec
DELETE FROM session_package_branches
WHERE organization_id = sqlc.arg(organization_id) AND package_id = sqlc.arg(package_id);

-- name: InsertSessionPackageBranch :exec
INSERT INTO session_package_branches (organization_id, package_id, branch_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(package_id), sqlc.arg(branch_id))
ON CONFLICT DO NOTHING;

-- name: GetSessionPackage :one
SELECT p.id, p.uuid, p.name, p.description, p.service_id, p.total_sessions, p.price, p.validity_days,
    p.all_branches, p.is_active, p.created_at, p.updated_at,
    s.uuid AS service_uuid, s.name AS service_name,
    (SELECT COUNT(*) FROM customer_packages c
      WHERE c.organization_id = p.organization_id AND c.package_id = p.id)::bigint AS sold_count
FROM session_packages p
JOIN services s ON s.organization_id = p.organization_id AND s.id = p.service_id
WHERE p.organization_id = sqlc.arg(organization_id) AND p.uuid = sqlc.arg(uuid) AND p.deleted_at IS NULL;

-- name: ListSessionPackageBranches :many
SELECT pb.package_id, b.id, b.uuid, b.name
FROM session_package_branches pb
JOIN branches b ON b.organization_id = pb.organization_id AND b.id = pb.branch_id
WHERE pb.organization_id = sqlc.arg(organization_id) AND pb.package_id = ANY(sqlc.arg(package_ids)::bigint[])
ORDER BY b.name;

-- name: ListSessionPackages :many
SELECT p.id, p.uuid, p.name, p.description, p.service_id, p.total_sessions, p.price, p.validity_days,
    p.all_branches, p.is_active, p.created_at, p.updated_at,
    s.uuid AS service_uuid, s.name AS service_name,
    (SELECT COUNT(*) FROM customer_packages c
      WHERE c.organization_id = p.organization_id AND c.package_id = p.id)::bigint AS sold_count
FROM session_packages p
JOIN services s ON s.organization_id = p.organization_id AND s.id = p.service_id
WHERE p.organization_id = sqlc.arg(organization_id) AND p.deleted_at IS NULL
  AND (sqlc.narg(is_active)::boolean IS NULL OR p.is_active = sqlc.narg(is_active)::boolean)
  AND (sqlc.narg(service_ids)::bigint[] IS NULL OR p.service_id = ANY(sqlc.narg(service_ids)::bigint[]))
  AND (sqlc.narg(branch_id)::bigint IS NULL OR p.all_branches OR EXISTS (
        SELECT 1 FROM session_package_branches pb
         WHERE pb.organization_id = p.organization_id AND pb.package_id = p.id AND pb.branch_id = sqlc.narg(branch_id)::bigint))
  AND (sqlc.narg(q)::text IS NULL OR p.name ILIKE '%' || sqlc.narg(q) || '%' OR s.name ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN lower(p.name) END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN lower(p.name) END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'price' AND NOT sqlc.arg(sort_desc)::boolean THEN p.price END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'price' AND sqlc.arg(sort_desc)::boolean THEN p.price END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'total_sessions' AND NOT sqlc.arg(sort_desc)::boolean THEN p.total_sessions END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'total_sessions' AND sqlc.arg(sort_desc)::boolean THEN p.total_sessions END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN p.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN p.created_at END DESC,
  lower(p.name) ASC, p.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountSessionPackages :one
SELECT COUNT(*)::bigint
FROM session_packages p
JOIN services s ON s.organization_id = p.organization_id AND s.id = p.service_id
WHERE p.organization_id = sqlc.arg(organization_id) AND p.deleted_at IS NULL
  AND (sqlc.narg(is_active)::boolean IS NULL OR p.is_active = sqlc.narg(is_active)::boolean)
  AND (sqlc.narg(service_ids)::bigint[] IS NULL OR p.service_id = ANY(sqlc.narg(service_ids)::bigint[]))
  AND (sqlc.narg(branch_id)::bigint IS NULL OR p.all_branches OR EXISTS (
        SELECT 1 FROM session_package_branches pb
         WHERE pb.organization_id = p.organization_id AND pb.package_id = p.id AND pb.branch_id = sqlc.narg(branch_id)::bigint))
  AND (sqlc.narg(q)::text IS NULL OR p.name ILIKE '%' || sqlc.narg(q) || '%' OR s.name ILIKE '%' || sqlc.narg(q) || '%');

-- name: GetPackageServiceByUUID :one
SELECT s.id, s.uuid, s.name, s.is_active FROM services s
WHERE s.organization_id = sqlc.arg(organization_id) AND s.uuid = sqlc.arg(uuid) AND s.deleted_at IS NULL;

-- name: ListPackageServicesByUUID :many
SELECT s.id, s.uuid FROM services s
WHERE s.organization_id = sqlc.arg(organization_id) AND s.uuid = ANY(sqlc.arg(uuids)::uuid[]);

-- name: ListPackageBranchesByUUID :many
SELECT b.id, b.uuid, b.name FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id) AND b.uuid = ANY(sqlc.arg(uuids)::uuid[]) AND b.deleted_at IS NULL;

-- name: GetPackageCustomerByUUID :one
SELECT c.id, c.uuid FROM customers c
WHERE c.organization_id = sqlc.arg(organization_id) AND c.uuid = sqlc.arg(uuid) AND c.deleted_at IS NULL;

-- name: GetPackageStaffByUUID :one
SELECT st.id, st.uuid, st.name FROM staff st
WHERE st.organization_id = sqlc.arg(organization_id) AND st.uuid = sqlc.arg(uuid) AND st.deleted_at IS NULL;

-- name: ListBranchRefsByIDs :many
SELECT b.id, b.uuid, b.name FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id) AND b.id = ANY(sqlc.arg(ids)::bigint[])
ORDER BY b.name;

-- name: GetPackageClock :one
-- Time zone (branch override, else the business) and currency of a branch.
SELECT COALESCE(NULLIF(b.timezone, ''), o.timezone)::text AS timezone, o.currency::text AS currency
FROM organizations o
JOIN branches b ON b.organization_id = o.id AND b.id = sqlc.arg(branch_id)
WHERE o.id = sqlc.arg(organization_id) AND b.organization_id = sqlc.arg(organization_id);

-- ------------------------------------------------------- customer packages

-- name: InsertCustomerPackage :one
INSERT INTO customer_packages (
    uuid, organization_id, package_id, customer_id, branch_id, service_id, name, service_name, total_sessions, price,
    currency, branch_ids, sold_on, expires_on, staff_id, note, idempotency_key, created_by
) VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.arg(package_id), sqlc.arg(customer_id), sqlc.arg(branch_id),
    sqlc.arg(service_id), sqlc.arg(name), sqlc.arg(service_name), sqlc.arg(total_sessions), sqlc.arg(price),
    sqlc.arg(currency), sqlc.narg(branch_ids)::bigint[], sqlc.arg(sold_on), sqlc.narg(expires_on), sqlc.narg(staff_id),
    sqlc.arg(note), sqlc.narg(idempotency_key), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: SetCustomerPackageEntries :exec
UPDATE customer_packages SET collection_entry_id = sqlc.narg(collection_entry_id), charge_entry_id = sqlc.narg(charge_entry_id)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: RefundCustomerPackage :exec
UPDATE customer_packages SET
    status = 'refunded', refund_amount = sqlc.arg(refund_amount), refund_entry_id = sqlc.narg(refund_entry_id),
    charge_entry_id = sqlc.narg(charge_entry_id), refund_reason = sqlc.arg(refund_reason),
    refunded_on = sqlc.arg(refunded_on), refunded_at = NOW(), refunded_by = sqlc.narg(refunded_by)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'active';

-- name: GetCustomerPackageByIdempotencyKey :one
SELECT cp.id, cp.uuid, cp.package_id, cp.customer_id, cp.branch_id
FROM customer_packages cp
WHERE cp.organization_id = sqlc.arg(organization_id) AND cp.idempotency_key = sqlc.arg(idempotency_key);

-- name: LockCustomerPackage :one
SELECT cp.* FROM customer_packages cp
WHERE cp.organization_id = sqlc.arg(organization_id) AND cp.id = sqlc.arg(id)
FOR UPDATE;

-- name: GetCustomerPackageRow :one
SELECT cp.* FROM customer_packages cp
WHERE cp.organization_id = sqlc.arg(organization_id) AND cp.uuid = sqlc.arg(uuid);

-- name: GetCustomerPackage :one
SELECT cp.id, cp.uuid, cp.name, cp.service_name, cp.total_sessions, cp.price, cp.currency, cp.branch_ids, cp.sold_on,
    cp.expires_on, cp.status, cp.note, cp.refund_amount, cp.refund_reason, cp.refunded_on, cp.refunded_at,
    cp.created_at, cp.updated_at, cp.branch_id, cp.customer_id,
    c.uuid AS customer_uuid, c.first_name AS customer_first_name, c.last_name AS customer_last_name,
    c.phone_e164 AS customer_phone,
    b.uuid AS branch_uuid, b.name AS branch_name,
    p.uuid AS package_uuid, s.uuid AS service_uuid, st.uuid AS staff_uuid, st.name AS staff_name,
    ce.uuid AS collection_uuid, re.uuid AS refund_entry_uuid,
    COALESCE(u.consumed, 0)::int AS consumed_count, COALESCE(u.held, 0)::int AS held_count,
    COALESCE(u.consumed_amount, 0)::numeric(14, 2) AS consumed_amount,
    COALESCE(u.held_amount, 0)::numeric(14, 2) AS held_amount
FROM customer_packages cp
JOIN customers c ON c.organization_id = cp.organization_id AND c.id = cp.customer_id
JOIN branches b ON b.organization_id = cp.organization_id AND b.id = cp.branch_id
LEFT JOIN session_packages p ON p.organization_id = cp.organization_id AND p.id = cp.package_id
LEFT JOIN services s ON s.organization_id = cp.organization_id AND s.id = cp.service_id
LEFT JOIN staff st ON st.organization_id = cp.organization_id AND st.id = cp.staff_id
LEFT JOIN ledger_entries ce ON ce.organization_id = cp.organization_id AND ce.id = cp.collection_entry_id
LEFT JOIN ledger_entries re ON re.organization_id = cp.organization_id AND re.id = cp.refund_entry_id
LEFT JOIN LATERAL (
    SELECT COUNT(*) FILTER (WHERE ps.status = 'consumed') AS consumed,
           COUNT(*) FILTER (WHERE ps.status = 'held') AS held,
           SUM(ps.amount) FILTER (WHERE ps.status = 'consumed') AS consumed_amount,
           SUM(ps.amount) FILTER (WHERE ps.status = 'held') AS held_amount
    FROM package_sessions ps
    WHERE ps.organization_id = cp.organization_id AND ps.customer_package_id = cp.id
) u ON TRUE
WHERE cp.organization_id = sqlc.arg(organization_id) AND cp.uuid = sqlc.arg(uuid);

-- name: ListCustomerPackages :many
-- state: active (sessions left, valid), used_up (every session consumed),
-- expired (sessions left after the last day), refunded.
SELECT cp.id, cp.uuid, cp.name, cp.service_name, cp.total_sessions, cp.price, cp.currency, cp.branch_ids, cp.sold_on,
    cp.expires_on, cp.status, cp.note, cp.refund_amount, cp.refund_reason, cp.refunded_on, cp.refunded_at,
    cp.created_at, cp.updated_at, cp.branch_id, cp.customer_id,
    c.uuid AS customer_uuid, c.first_name AS customer_first_name, c.last_name AS customer_last_name,
    c.phone_e164 AS customer_phone,
    b.uuid AS branch_uuid, b.name AS branch_name,
    p.uuid AS package_uuid, s.uuid AS service_uuid, st.uuid AS staff_uuid, st.name AS staff_name,
    ce.uuid AS collection_uuid, re.uuid AS refund_entry_uuid,
    COALESCE(u.consumed, 0)::int AS consumed_count, COALESCE(u.held, 0)::int AS held_count,
    COALESCE(u.consumed_amount, 0)::numeric(14, 2) AS consumed_amount,
    COALESCE(u.held_amount, 0)::numeric(14, 2) AS held_amount
FROM customer_packages cp
JOIN customers c ON c.organization_id = cp.organization_id AND c.id = cp.customer_id
JOIN branches b ON b.organization_id = cp.organization_id AND b.id = cp.branch_id
LEFT JOIN session_packages p ON p.organization_id = cp.organization_id AND p.id = cp.package_id
LEFT JOIN services s ON s.organization_id = cp.organization_id AND s.id = cp.service_id
LEFT JOIN staff st ON st.organization_id = cp.organization_id AND st.id = cp.staff_id
LEFT JOIN ledger_entries ce ON ce.organization_id = cp.organization_id AND ce.id = cp.collection_entry_id
LEFT JOIN ledger_entries re ON re.organization_id = cp.organization_id AND re.id = cp.refund_entry_id
LEFT JOIN LATERAL (
    SELECT COUNT(*) FILTER (WHERE ps.status = 'consumed') AS consumed,
           COUNT(*) FILTER (WHERE ps.status = 'held') AS held,
           SUM(ps.amount) FILTER (WHERE ps.status = 'consumed') AS consumed_amount,
           SUM(ps.amount) FILTER (WHERE ps.status = 'held') AS held_amount
    FROM package_sessions ps
    WHERE ps.organization_id = cp.organization_id AND ps.customer_package_id = cp.id
) u ON TRUE
WHERE cp.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR cp.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR cp.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(customer_id)::bigint IS NULL OR cp.customer_id = sqlc.narg(customer_id)::bigint)
  AND (sqlc.narg(package_ids)::bigint[] IS NULL OR cp.package_id = ANY(sqlc.narg(package_ids)::bigint[]))
  AND (sqlc.narg(date_from)::date IS NULL OR cp.sold_on >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR cp.sold_on <= sqlc.narg(date_to)::date)
  AND (sqlc.narg(states)::text[] IS NULL OR (CASE
        WHEN cp.status = 'refunded' THEN 'refunded'
        WHEN COALESCE(u.consumed, 0) >= cp.total_sessions THEN 'used_up'
        WHEN cp.expires_on IS NOT NULL AND cp.expires_on < sqlc.arg(today)::date THEN 'expired'
        ELSE 'active' END) = ANY(sqlc.narg(states)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR cp.name ILIKE '%' || sqlc.narg(q) || '%'
    OR cp.service_name ILIKE '%' || sqlc.narg(q) || '%'
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
    OR COALESCE(c.phone_e164, '') ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'sold_on' AND NOT sqlc.arg(sort_desc)::boolean THEN cp.sold_on END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'sold_on' AND sqlc.arg(sort_desc)::boolean THEN cp.sold_on END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'expires_on' AND NOT sqlc.arg(sort_desc)::boolean THEN cp.expires_on END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'expires_on' AND sqlc.arg(sort_desc)::boolean THEN cp.expires_on END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'price' AND NOT sqlc.arg(sort_desc)::boolean THEN cp.price END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'price' AND sqlc.arg(sort_desc)::boolean THEN cp.price END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN cp.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN cp.created_at END DESC,
  cp.sold_on DESC, cp.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountCustomerPackages :one
SELECT COUNT(*)::bigint
FROM customer_packages cp
JOIN customers c ON c.organization_id = cp.organization_id AND c.id = cp.customer_id
LEFT JOIN LATERAL (
    SELECT COUNT(*) FILTER (WHERE ps.status = 'consumed') AS consumed
    FROM package_sessions ps
    WHERE ps.organization_id = cp.organization_id AND ps.customer_package_id = cp.id
) u ON TRUE
WHERE cp.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR cp.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR cp.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(customer_id)::bigint IS NULL OR cp.customer_id = sqlc.narg(customer_id)::bigint)
  AND (sqlc.narg(package_ids)::bigint[] IS NULL OR cp.package_id = ANY(sqlc.narg(package_ids)::bigint[]))
  AND (sqlc.narg(date_from)::date IS NULL OR cp.sold_on >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR cp.sold_on <= sqlc.narg(date_to)::date)
  AND (sqlc.narg(states)::text[] IS NULL OR (CASE
        WHEN cp.status = 'refunded' THEN 'refunded'
        WHEN COALESCE(u.consumed, 0) >= cp.total_sessions THEN 'used_up'
        WHEN cp.expires_on IS NOT NULL AND cp.expires_on < sqlc.arg(today)::date THEN 'expired'
        ELSE 'active' END) = ANY(sqlc.narg(states)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR cp.name ILIKE '%' || sqlc.narg(q) || '%'
    OR cp.service_name ILIKE '%' || sqlc.narg(q) || '%'
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
    OR COALESCE(c.phone_e164, '') ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListPackageListFilterPackages :many
SELECT p.id, p.uuid FROM session_packages p
WHERE p.organization_id = sqlc.arg(organization_id) AND p.uuid = ANY(sqlc.arg(uuids)::uuid[]);

-- name: CustomerPackageFacetPackages :many
-- Package definitions sold in the caller's branches (list filter options).
SELECT p.uuid, p.name, COUNT(*)::bigint AS count
FROM customer_packages cp
JOIN session_packages p ON p.organization_id = cp.organization_id AND p.id = cp.package_id
WHERE cp.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR cp.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY p.uuid, p.name
ORDER BY p.name;

-- name: CustomerPackageFacetBranches :many
SELECT b.uuid, b.name, COUNT(*)::bigint AS count
FROM customer_packages cp
JOIN branches b ON b.organization_id = cp.organization_id AND b.id = cp.branch_id
WHERE cp.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR cp.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY b.uuid, b.name
ORDER BY b.name;

-- ---------------------------------------------------------------- sessions

-- name: ListCustomerPackageSessions :many
SELECT ps.id, ps.uuid, ps.session_no, ps.amount, ps.status, ps.release_reason, ps.consumed_at, ps.consumed_on,
    ps.released_at, ps.created_at, ps.line_index,
    r.uuid AS reservation_uuid, r.number AS reservation_number, r.starts_at AS reservation_starts_at,
    r.branch_id AS reservation_branch_id, r.status AS reservation_status,
    rl.service_name, rl.staff_name
FROM package_sessions ps
JOIN reservations r ON r.organization_id = ps.organization_id AND r.id = ps.reservation_id
LEFT JOIN reservation_lines rl ON rl.organization_id = ps.organization_id AND rl.reservation_id = ps.reservation_id
    AND rl.line_index = ps.line_index
WHERE ps.organization_id = sqlc.arg(organization_id) AND ps.customer_package_id = sqlc.arg(customer_package_id)
ORDER BY ps.id DESC;

-- name: ListReservationPackageSessions :many
SELECT ps.id, ps.uuid, ps.session_no, ps.amount, ps.status, ps.release_reason, ps.consumed_at, ps.released_at,
    ps.created_at, ps.line_index,
    cp.uuid AS customer_package_uuid, cp.name AS package_name, cp.total_sessions
FROM package_sessions ps
JOIN customer_packages cp ON cp.organization_id = ps.organization_id AND cp.id = ps.customer_package_id
WHERE ps.organization_id = sqlc.arg(organization_id) AND ps.reservation_id = sqlc.arg(reservation_id)
ORDER BY ps.id;

-- name: ListHeldReservationPackageSessions :many
SELECT ps.id, ps.uuid, ps.customer_package_id, ps.line_index, ps.session_no,
    cp.service_id AS package_service_id, cp.expires_on, cp.name AS package_name
FROM package_sessions ps
JOIN customer_packages cp ON cp.organization_id = ps.organization_id AND cp.id = ps.customer_package_id
WHERE ps.organization_id = sqlc.arg(organization_id) AND ps.reservation_id = sqlc.arg(reservation_id)
  AND ps.status = 'held'
ORDER BY ps.id
FOR UPDATE OF ps;

-- name: GetReservationPackageSessionForUpdate :one
SELECT ps.id, ps.uuid, ps.status, ps.line_index, ps.session_no, cp.name AS package_name
FROM package_sessions ps
JOIN customer_packages cp ON cp.organization_id = ps.organization_id AND cp.id = ps.customer_package_id
WHERE ps.organization_id = sqlc.arg(organization_id) AND ps.reservation_id = sqlc.arg(reservation_id)
  AND ps.uuid = sqlc.arg(uuid)
FOR UPDATE OF ps;

-- name: LivePackageSessionNumbers :many
-- Session numbers of a package that are held or consumed.
SELECT ps.session_no FROM package_sessions ps
WHERE ps.organization_id = sqlc.arg(organization_id) AND ps.customer_package_id = sqlc.arg(customer_package_id)
  AND ps.status <> 'released'
ORDER BY ps.session_no;

-- name: InsertPackageSession :one
INSERT INTO package_sessions (
    organization_id, customer_package_id, reservation_id, line_index, session_no, amount, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(customer_package_id), sqlc.arg(reservation_id), sqlc.arg(line_index),
    sqlc.arg(session_no), sqlc.arg(amount), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: ReleasePackageSession :exec
UPDATE package_sessions SET status = 'released', release_reason = sqlc.arg(release_reason), released_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'held';

-- name: ReleaseReservationPackageSessions :execrows
UPDATE package_sessions SET status = 'released', release_reason = sqlc.arg(release_reason), released_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND reservation_id = sqlc.arg(reservation_id) AND status = 'held';

-- name: ConsumeReservationPackageSessions :execrows
-- A completed visit uses its held sessions; the business day is the day
-- the visit ended (never later than now) in the branch's time zone.
UPDATE package_sessions ps SET status = 'consumed', consumed_at = sqlc.arg(now)::timestamptz,
    consumed_on = (LEAST(r.ends_at, sqlc.arg(now)::timestamptz) AT TIME ZONE COALESCE(NULLIF(b.timezone, ''), o.timezone))::date
FROM reservations r
JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
JOIN organizations o ON o.id = r.organization_id
WHERE ps.organization_id = sqlc.arg(organization_id) AND ps.reservation_id = sqlc.arg(reservation_id)
  AND ps.status = 'held'
  AND r.organization_id = ps.organization_id AND r.id = ps.reservation_id;

-- name: ReservationPackageOptions :many
-- Packages of a customer that can give a session to a visit in a branch:
-- active, not used up, valid on the visit day and usable in the branch.
SELECT cp.id, cp.uuid, cp.name, cp.service_id, cp.service_name, cp.total_sessions, cp.price, cp.expires_on,
    s.uuid AS service_uuid,
    (SELECT COUNT(*) FROM package_sessions ps
      WHERE ps.organization_id = cp.organization_id AND ps.customer_package_id = cp.id AND ps.status <> 'released')::int AS live_count
FROM customer_packages cp
LEFT JOIN services s ON s.organization_id = cp.organization_id AND s.id = cp.service_id
WHERE cp.organization_id = sqlc.arg(organization_id) AND cp.customer_id = sqlc.arg(customer_id)
  AND cp.status = 'active'
  AND (cp.branch_ids IS NULL OR sqlc.arg(branch_id)::bigint = ANY(cp.branch_ids))
  AND (cp.expires_on IS NULL OR cp.expires_on >= sqlc.arg(visit_day)::date)
ORDER BY cp.expires_on ASC NULLS LAST, cp.id;

-- name: GetPackageVisitDay :one
-- The local day of a visit's start in its branch's time zone.
SELECT (r.starts_at AT TIME ZONE COALESCE(NULLIF(b.timezone, ''), o.timezone))::date AS visit_day
FROM reservations r
JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
JOIN organizations o ON o.id = r.organization_id
WHERE r.organization_id = sqlc.arg(organization_id) AND r.id = sqlc.arg(reservation_id);

-- name: GetPackageLine :one
SELECT rl.line_index, rl.service_id, rl.service_name
FROM reservation_lines rl
WHERE rl.organization_id = sqlc.arg(organization_id) AND rl.reservation_id = sqlc.arg(reservation_id)
  AND rl.line_index = sqlc.arg(line_index);

-- ------------------------------------------------------------------ report

-- name: PackageSalesTotals :many
SELECT cp.currency::text AS currency, COUNT(*)::bigint AS sold_count,
    COALESCE(SUM(cp.price), 0)::numeric(14, 2) AS sold_amount,
    COALESCE(SUM(cp.total_sessions), 0)::bigint AS sold_sessions
FROM customer_packages cp
WHERE cp.organization_id = sqlc.arg(organization_id)
  AND cp.sold_on BETWEEN sqlc.arg(date_from)::date AND sqlc.arg(date_to)::date
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR cp.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR cp.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
GROUP BY cp.currency ORDER BY cp.currency;

-- name: PackageUsageTotals :many
-- Sessions used in the range (their value is service revenue of the
-- visits, through the sale lines).
SELECT cp.currency::text AS currency, COUNT(*)::bigint AS used_count,
    COALESCE(SUM(ps.amount), 0)::numeric(14, 2) AS used_amount
FROM package_sessions ps
JOIN customer_packages cp ON cp.organization_id = ps.organization_id AND cp.id = ps.customer_package_id
JOIN reservations r ON r.organization_id = ps.organization_id AND r.id = ps.reservation_id
WHERE ps.organization_id = sqlc.arg(organization_id) AND ps.status = 'consumed'
  AND ps.consumed_on BETWEEN sqlc.arg(date_from)::date AND sqlc.arg(date_to)::date
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
GROUP BY cp.currency ORDER BY cp.currency;

-- name: PackageRefundTotals :many
SELECT cp.currency::text AS currency, COUNT(*)::bigint AS refunded_count,
    COALESCE(SUM(cp.refund_amount), 0)::numeric(14, 2) AS refunded_amount,
    COALESCE(SUM(cp.total_sessions - COALESCE(u.consumed, 0)), 0)::bigint AS refunded_sessions
FROM customer_packages cp
LEFT JOIN LATERAL (
    SELECT COUNT(*) AS consumed FROM package_sessions ps
    WHERE ps.organization_id = cp.organization_id AND ps.customer_package_id = cp.id AND ps.status = 'consumed'
) u ON TRUE
WHERE cp.organization_id = sqlc.arg(organization_id) AND cp.status = 'refunded'
  AND cp.refunded_on BETWEEN sqlc.arg(date_from)::date AND sqlc.arg(date_to)::date
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR cp.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR cp.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
GROUP BY cp.currency ORDER BY cp.currency;

-- name: PackageOutstanding :many
-- Packages not refunded, today: the sessions and value still open on
-- valid packages, and what was left on expired ones (forfeited).
SELECT cp.currency::text AS currency,
    (CASE WHEN COALESCE(u.consumed, 0) >= cp.total_sessions THEN 'used_up'
          WHEN cp.expires_on IS NOT NULL AND cp.expires_on < sqlc.arg(today)::date THEN 'expired'
          ELSE 'active' END)::text AS state,
    COUNT(*)::bigint AS package_count,
    COALESCE(SUM(cp.total_sessions - COALESCE(u.consumed, 0) - COALESCE(u.held, 0)), 0)::bigint AS open_sessions,
    COALESCE(SUM(COALESCE(u.held, 0)), 0)::bigint AS held_sessions,
    COALESCE(SUM(cp.price - COALESCE(u.consumed_amount, 0) - COALESCE(u.held_amount, 0)), 0)::numeric(14, 2) AS open_amount,
    COALESCE(SUM(COALESCE(u.held_amount, 0)), 0)::numeric(14, 2) AS held_amount
FROM customer_packages cp
LEFT JOIN LATERAL (
    SELECT COUNT(*) FILTER (WHERE ps.status = 'consumed') AS consumed,
           COUNT(*) FILTER (WHERE ps.status = 'held') AS held,
           SUM(ps.amount) FILTER (WHERE ps.status = 'consumed') AS consumed_amount,
           SUM(ps.amount) FILTER (WHERE ps.status = 'held') AS held_amount
    FROM package_sessions ps
    WHERE ps.organization_id = cp.organization_id AND ps.customer_package_id = cp.id
) u ON TRUE
WHERE cp.organization_id = sqlc.arg(organization_id) AND cp.status = 'active'
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR cp.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR cp.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
GROUP BY 1, 2 ORDER BY 1, 2;
