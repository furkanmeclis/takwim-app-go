-- TEC-916: sale lines (revenue) and the customer ledger (what was charged,
-- collected and refunded). Every query takes the organization id.

-- ------------------------------------------------------------------ lookups

-- name: GetSalesClock :one
-- Time zone (branch override, else the business) and currency of a branch.
SELECT COALESCE(NULLIF(b.timezone, ''), o.timezone)::text AS timezone, o.currency::text AS currency
FROM organizations o
JOIN branches b ON b.organization_id = o.id AND b.id = sqlc.arg(branch_id)
WHERE o.id = sqlc.arg(organization_id) AND b.organization_id = sqlc.arg(organization_id);

-- name: GetSalesReservation :one
SELECT r.id, r.uuid, r.number, r.branch_id, r.customer_id, r.status, r.total_price, r.currency, r.starts_at, r.ends_at
FROM reservations r
WHERE r.organization_id = sqlc.arg(organization_id) AND r.id = sqlc.arg(id);

-- name: GetSalesReservationByUUID :one
SELECT r.id, r.uuid, r.number, r.branch_id, r.customer_id, r.status, r.total_price, r.currency, r.starts_at, r.ends_at
FROM reservations r
WHERE r.organization_id = sqlc.arg(organization_id) AND r.uuid = sqlc.arg(uuid);

-- name: ListSalesReservationLines :many
SELECT rl.line_index, rl.service_id, rl.service_name, rl.staff_id, rl.unit_price
FROM reservation_lines rl
WHERE rl.organization_id = sqlc.arg(organization_id) AND rl.reservation_id = sqlc.arg(reservation_id)
ORDER BY rl.line_index;

-- name: GetSalesCustomerByUUID :one
SELECT c.id, c.uuid FROM customers c
WHERE c.organization_id = sqlc.arg(organization_id) AND c.uuid = sqlc.arg(uuid) AND c.deleted_at IS NULL;

-- name: ListSalesBranchesByUUID :many
SELECT b.id, b.uuid FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id) AND b.uuid = ANY(sqlc.arg(uuids)::uuid[]);

-- name: GetSalesCashMovement :one
SELECT m.id, m.uuid, m.kind, m.amount, m.currency FROM cash_movements m
WHERE m.organization_id = sqlc.arg(organization_id) AND m.uuid = sqlc.arg(uuid);

-- name: GetSalesCashMovementByID :one
SELECT m.id, m.uuid, m.kind, m.amount, m.currency FROM cash_movements m
WHERE m.organization_id = sqlc.arg(organization_id) AND m.id = sqlc.arg(id);

-- --------------------------------------------------------------- sale lines

-- name: InsertSaleLine :one
INSERT INTO sale_lines (
    organization_id, branch_id, customer_id, kind, item_type, source_type, source_uuid, reservation_id, line_index,
    service_id, product_id, variant_id, staff_id, description, quantity, unit_price, gross_amount, discount_amount,
    net_amount, currency, sale_date, accrual_key, reverses_line_id, replaces_line_id, reason, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(branch_id), sqlc.narg(customer_id), sqlc.arg(kind), sqlc.arg(item_type),
    sqlc.arg(source_type), sqlc.narg(source_uuid), sqlc.narg(reservation_id), sqlc.narg(line_index),
    sqlc.narg(service_id), sqlc.narg(product_id), sqlc.narg(variant_id), sqlc.narg(staff_id), sqlc.arg(description),
    sqlc.arg(quantity), sqlc.arg(unit_price), sqlc.arg(gross_amount), sqlc.arg(discount_amount), sqlc.arg(net_amount),
    sqlc.arg(currency), sqlc.arg(sale_date), sqlc.arg(accrual_key), sqlc.narg(reverses_line_id),
    sqlc.narg(replaces_line_id), sqlc.arg(reason), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: GetSaleLineRow :one
SELECT s.*,
    EXISTS (SELECT 1 FROM sale_lines r WHERE r.organization_id = s.organization_id AND r.reverses_line_id = s.id)::boolean AS reversed
FROM sale_lines s
WHERE s.organization_id = sqlc.arg(organization_id) AND s.uuid = sqlc.arg(uuid);

-- name: GetSaleLineRowByID :one
SELECT s.*,
    EXISTS (SELECT 1 FROM sale_lines r WHERE r.organization_id = s.organization_id AND r.reverses_line_id = s.id)::boolean AS reversed
FROM sale_lines s
WHERE s.organization_id = sqlc.arg(organization_id) AND s.id = sqlc.arg(id);

-- name: GetLiveSaleLineByKey :one
-- The unreversed sale line of an item, if any.
SELECT s.id, s.uuid FROM sale_lines s
WHERE s.organization_id = sqlc.arg(organization_id) AND s.accrual_key = sqlc.arg(accrual_key) AND s.kind = 'sale'
  AND NOT EXISTS (SELECT 1 FROM sale_lines r WHERE r.organization_id = s.organization_id AND r.reverses_line_id = s.id)
ORDER BY s.id DESC
LIMIT 1;

-- name: GetSaleLine :one
SELECT
    s.id, s.uuid, s.kind, s.item_type, s.source_type, s.source_uuid, s.line_index, s.description, s.quantity,
    s.unit_price, s.gross_amount, s.discount_amount, s.net_amount, s.currency, s.sale_date, s.accrual_key,
    s.accrual_generation, s.reason, s.created_at, s.branch_id,
    b.uuid AS branch_uuid, b.name AS branch_name,
    c.uuid AS customer_uuid, c.first_name AS customer_first_name, c.last_name AS customer_last_name,
    r.uuid AS reservation_uuid, r.number AS reservation_number,
    sv.uuid AS service_uuid, p.uuid AS product_uuid, v.uuid AS variant_uuid,
    st.uuid AS staff_uuid, st.name AS staff_name,
    rv.uuid AS reverses_uuid, rb.uuid AS reversed_by_uuid, rp.uuid AS replaces_uuid, rpb.uuid AS replaced_by_uuid,
    u.uuid AS creator_uuid, u.name AS creator_name, u.surname AS creator_surname
FROM sale_lines s
JOIN branches b ON b.organization_id = s.organization_id AND b.id = s.branch_id
LEFT JOIN customers c ON c.organization_id = s.organization_id AND c.id = s.customer_id
LEFT JOIN reservations r ON r.organization_id = s.organization_id AND r.id = s.reservation_id
LEFT JOIN services sv ON sv.organization_id = s.organization_id AND sv.id = s.service_id
LEFT JOIN products p ON p.organization_id = s.organization_id AND p.id = s.product_id
LEFT JOIN product_variants v ON v.organization_id = s.organization_id AND v.id = s.variant_id
LEFT JOIN staff st ON st.organization_id = s.organization_id AND st.id = s.staff_id
LEFT JOIN sale_lines rv ON rv.organization_id = s.organization_id AND rv.id = s.reverses_line_id
LEFT JOIN sale_lines rb ON rb.organization_id = s.organization_id AND rb.reverses_line_id = s.id
LEFT JOIN sale_lines rp ON rp.organization_id = s.organization_id AND rp.id = s.replaces_line_id
LEFT JOIN sale_lines rpb ON rpb.organization_id = s.organization_id AND rpb.replaces_line_id = s.id
LEFT JOIN users u ON u.id = s.created_by
WHERE s.organization_id = sqlc.arg(organization_id) AND s.uuid = sqlc.arg(uuid);

-- name: ListSaleLines :many
SELECT
    s.id, s.uuid, s.kind, s.item_type, s.source_type, s.source_uuid, s.line_index, s.description, s.quantity,
    s.unit_price, s.gross_amount, s.discount_amount, s.net_amount, s.currency, s.sale_date, s.accrual_key,
    s.accrual_generation, s.reason, s.created_at, s.branch_id,
    b.uuid AS branch_uuid, b.name AS branch_name,
    c.uuid AS customer_uuid, c.first_name AS customer_first_name, c.last_name AS customer_last_name,
    r.uuid AS reservation_uuid, r.number AS reservation_number,
    sv.uuid AS service_uuid, p.uuid AS product_uuid, v.uuid AS variant_uuid,
    st.uuid AS staff_uuid, st.name AS staff_name,
    rv.uuid AS reverses_uuid, rb.uuid AS reversed_by_uuid, rp.uuid AS replaces_uuid, rpb.uuid AS replaced_by_uuid,
    u.uuid AS creator_uuid, u.name AS creator_name, u.surname AS creator_surname
FROM sale_lines s
JOIN branches b ON b.organization_id = s.organization_id AND b.id = s.branch_id
LEFT JOIN customers c ON c.organization_id = s.organization_id AND c.id = s.customer_id
LEFT JOIN reservations r ON r.organization_id = s.organization_id AND r.id = s.reservation_id
LEFT JOIN services sv ON sv.organization_id = s.organization_id AND sv.id = s.service_id
LEFT JOIN products p ON p.organization_id = s.organization_id AND p.id = s.product_id
LEFT JOIN product_variants v ON v.organization_id = s.organization_id AND v.id = s.variant_id
LEFT JOIN staff st ON st.organization_id = s.organization_id AND st.id = s.staff_id
LEFT JOIN sale_lines rv ON rv.organization_id = s.organization_id AND rv.id = s.reverses_line_id
LEFT JOIN sale_lines rb ON rb.organization_id = s.organization_id AND rb.reverses_line_id = s.id
LEFT JOIN sale_lines rp ON rp.organization_id = s.organization_id AND rp.id = s.replaces_line_id
LEFT JOIN sale_lines rpb ON rpb.organization_id = s.organization_id AND rpb.replaces_line_id = s.id
LEFT JOIN users u ON u.id = s.created_by
WHERE s.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR s.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR s.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR s.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(item_types)::text[] IS NULL OR s.item_type = ANY(sqlc.narg(item_types)::text[]))
  AND (sqlc.narg(reservation_id)::bigint IS NULL OR s.reservation_id = sqlc.narg(reservation_id)::bigint)
  AND (sqlc.narg(customer_id)::bigint IS NULL OR s.customer_id = sqlc.narg(customer_id)::bigint)
  AND (sqlc.narg(staff_ids)::bigint[] IS NULL OR s.staff_id = ANY(sqlc.narg(staff_ids)::bigint[]))
  AND (sqlc.narg(date_from)::date IS NULL OR s.sale_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR s.sale_date <= sqlc.narg(date_to)::date)
  AND (
    sqlc.narg(q)::text IS NULL
    OR s.description ILIKE '%' || sqlc.narg(q) || '%'
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
    OR st.name ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'net_amount' AND NOT sqlc.arg(sort_desc)::boolean THEN s.net_amount END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'net_amount' AND sqlc.arg(sort_desc)::boolean THEN s.net_amount END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'kind' AND NOT sqlc.arg(sort_desc)::boolean THEN s.kind END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'kind' AND sqlc.arg(sort_desc)::boolean THEN s.kind END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'sale_date' AND NOT sqlc.arg(sort_desc)::boolean THEN s.sale_date END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'sale_date' AND NOT sqlc.arg(sort_desc)::boolean THEN s.id END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN s.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN s.created_at END DESC,
  s.sale_date DESC, s.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountSaleLines :one
SELECT COUNT(*)::bigint
FROM sale_lines s
LEFT JOIN customers c ON c.organization_id = s.organization_id AND c.id = s.customer_id
LEFT JOIN staff st ON st.organization_id = s.organization_id AND st.id = s.staff_id
WHERE s.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR s.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR s.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR s.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(item_types)::text[] IS NULL OR s.item_type = ANY(sqlc.narg(item_types)::text[]))
  AND (sqlc.narg(reservation_id)::bigint IS NULL OR s.reservation_id = sqlc.narg(reservation_id)::bigint)
  AND (sqlc.narg(customer_id)::bigint IS NULL OR s.customer_id = sqlc.narg(customer_id)::bigint)
  AND (sqlc.narg(staff_ids)::bigint[] IS NULL OR s.staff_id = ANY(sqlc.narg(staff_ids)::bigint[]))
  AND (sqlc.narg(date_from)::date IS NULL OR s.sale_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR s.sale_date <= sqlc.narg(date_to)::date)
  AND (
    sqlc.narg(q)::text IS NULL
    OR s.description ILIKE '%' || sqlc.narg(q) || '%'
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
    OR st.name ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: CountReservationSaleLines :one
SELECT COUNT(*)::bigint FROM sale_lines s
WHERE s.organization_id = sqlc.arg(organization_id) AND s.reservation_id = sqlc.arg(reservation_id);

-- ----------------------------------------------------------- ledger entries

-- name: InsertLedgerEntry :one
INSERT INTO ledger_entries (
    uuid, organization_id, branch_id, customer_id, reservation_id, kind, direction, amount, currency, entry_date,
    sale_line_id, method, cash_movement_id, refunds_entry_id, reverses_entry_id, source_type, source_uuid,
    idempotency_key, description, created_by
) VALUES (
    COALESCE(sqlc.narg(uuid)::uuid, gen_random_uuid()), sqlc.arg(organization_id), sqlc.arg(branch_id),
    sqlc.narg(customer_id), sqlc.narg(reservation_id), sqlc.arg(kind),
    sqlc.arg(direction), sqlc.arg(amount), sqlc.arg(currency), sqlc.arg(entry_date), sqlc.narg(sale_line_id),
    sqlc.arg(method), sqlc.narg(cash_movement_id), sqlc.narg(refunds_entry_id), sqlc.narg(reverses_entry_id),
    sqlc.arg(source_type), sqlc.narg(source_uuid), sqlc.narg(idempotency_key), sqlc.arg(description),
    sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: GetLedgerEntryRow :one
SELECT e.*,
    EXISTS (SELECT 1 FROM ledger_entries r WHERE r.organization_id = e.organization_id AND r.reverses_entry_id = e.id)::boolean AS reversed
FROM ledger_entries e
WHERE e.organization_id = sqlc.arg(organization_id) AND e.uuid = sqlc.arg(uuid);

-- name: GetLedgerEntryRowByID :one
SELECT e.*,
    EXISTS (SELECT 1 FROM ledger_entries r WHERE r.organization_id = e.organization_id AND r.reverses_entry_id = e.id)::boolean AS reversed
FROM ledger_entries e
WHERE e.organization_id = sqlc.arg(organization_id) AND e.id = sqlc.arg(id);

-- name: GetLedgerEntryByIdempotencyKey :one
SELECT e.id, e.uuid, e.kind, e.amount, e.currency FROM ledger_entries e
WHERE e.organization_id = sqlc.arg(organization_id) AND e.idempotency_key = sqlc.arg(idempotency_key);

-- name: GetLedgerEntryBySaleLine :one
SELECT e.id, e.uuid FROM ledger_entries e
WHERE e.organization_id = sqlc.arg(organization_id) AND e.sale_line_id = sqlc.arg(sale_line_id);

-- name: LockLedgerEntry :one
-- Serializes the refunds and the reversal of one collection.
SELECT e.id FROM ledger_entries e
WHERE e.organization_id = sqlc.arg(organization_id) AND e.id = sqlc.arg(id)
FOR UPDATE;

-- name: RefundedOfCollection :one
-- Amount paid back from one collection (refunds minus reversed refunds).
SELECT COALESCE(SUM(f.amount) FILTER (
    WHERE NOT EXISTS (SELECT 1 FROM ledger_entries r WHERE r.organization_id = f.organization_id AND r.reverses_entry_id = f.id)
), 0)::numeric AS refunded
FROM ledger_entries f
WHERE f.organization_id = sqlc.arg(organization_id) AND f.refunds_entry_id = sqlc.arg(collection_id) AND f.kind = 'refund';

-- name: GetLedgerEntry :one
SELECT
    e.id, e.uuid, e.kind, e.direction, e.amount, e.currency, e.entry_date, e.method, e.source_type, e.source_uuid,
    e.description, e.created_at, e.branch_id,
    COALESCE(o.kind, e.kind)::text AS effective_kind,
    b.uuid AS branch_uuid, b.name AS branch_name,
    c.uuid AS customer_uuid, c.first_name AS customer_first_name, c.last_name AS customer_last_name,
    r.uuid AS reservation_uuid, r.number AS reservation_number,
    sl.uuid AS sale_line_uuid,
    m.uuid AS cash_movement_uuid, a.uuid AS cash_account_uuid, a.name AS cash_account_name,
    rf.uuid AS refunds_uuid, o.uuid AS reverses_uuid, rb.uuid AS reversed_by_uuid,
    u.uuid AS creator_uuid, u.name AS creator_name, u.surname AS creator_surname
FROM ledger_entries e
JOIN branches b ON b.organization_id = e.organization_id AND b.id = e.branch_id
LEFT JOIN customers c ON c.organization_id = e.organization_id AND c.id = e.customer_id
LEFT JOIN reservations r ON r.organization_id = e.organization_id AND r.id = e.reservation_id
LEFT JOIN sale_lines sl ON sl.organization_id = e.organization_id AND sl.id = e.sale_line_id
LEFT JOIN cash_movements m ON m.organization_id = e.organization_id AND m.id = e.cash_movement_id
LEFT JOIN cash_accounts a ON a.organization_id = m.organization_id AND a.id = m.cash_account_id
LEFT JOIN ledger_entries rf ON rf.organization_id = e.organization_id AND rf.id = e.refunds_entry_id
LEFT JOIN ledger_entries o ON o.organization_id = e.organization_id AND o.id = e.reverses_entry_id
LEFT JOIN ledger_entries rb ON rb.organization_id = e.organization_id AND rb.reverses_entry_id = e.id
LEFT JOIN users u ON u.id = e.created_by
WHERE e.organization_id = sqlc.arg(organization_id) AND e.uuid = sqlc.arg(uuid);

-- name: ListLedgerEntries :many
SELECT
    e.id, e.uuid, e.kind, e.direction, e.amount, e.currency, e.entry_date, e.method, e.source_type, e.source_uuid,
    e.description, e.created_at, e.branch_id,
    COALESCE(o.kind, e.kind)::text AS effective_kind,
    b.uuid AS branch_uuid, b.name AS branch_name,
    c.uuid AS customer_uuid, c.first_name AS customer_first_name, c.last_name AS customer_last_name,
    r.uuid AS reservation_uuid, r.number AS reservation_number,
    sl.uuid AS sale_line_uuid,
    m.uuid AS cash_movement_uuid, a.uuid AS cash_account_uuid, a.name AS cash_account_name,
    rf.uuid AS refunds_uuid, o.uuid AS reverses_uuid, rb.uuid AS reversed_by_uuid,
    u.uuid AS creator_uuid, u.name AS creator_name, u.surname AS creator_surname
FROM ledger_entries e
JOIN branches b ON b.organization_id = e.organization_id AND b.id = e.branch_id
LEFT JOIN customers c ON c.organization_id = e.organization_id AND c.id = e.customer_id
LEFT JOIN reservations r ON r.organization_id = e.organization_id AND r.id = e.reservation_id
LEFT JOIN sale_lines sl ON sl.organization_id = e.organization_id AND sl.id = e.sale_line_id
LEFT JOIN cash_movements m ON m.organization_id = e.organization_id AND m.id = e.cash_movement_id
LEFT JOIN cash_accounts a ON a.organization_id = m.organization_id AND a.id = m.cash_account_id
LEFT JOIN ledger_entries rf ON rf.organization_id = e.organization_id AND rf.id = e.refunds_entry_id
LEFT JOIN ledger_entries o ON o.organization_id = e.organization_id AND o.id = e.reverses_entry_id
LEFT JOIN ledger_entries rb ON rb.organization_id = e.organization_id AND rb.reverses_entry_id = e.id
LEFT JOIN users u ON u.id = e.created_by
WHERE e.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR e.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(reservation_id)::bigint IS NULL OR e.reservation_id = sqlc.narg(reservation_id)::bigint)
  AND (sqlc.narg(customer_id)::bigint IS NULL OR e.customer_id = sqlc.narg(customer_id)::bigint)
  AND (sqlc.narg(date_from)::date IS NULL OR e.entry_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR e.entry_date <= sqlc.narg(date_to)::date)
  AND (
    sqlc.narg(q)::text IS NULL
    OR e.description ILIKE '%' || sqlc.narg(q) || '%'
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND NOT sqlc.arg(sort_desc)::boolean THEN e.amount END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND sqlc.arg(sort_desc)::boolean THEN e.amount END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'kind' AND NOT sqlc.arg(sort_desc)::boolean THEN e.kind END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'kind' AND sqlc.arg(sort_desc)::boolean THEN e.kind END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'entry_date' AND NOT sqlc.arg(sort_desc)::boolean THEN e.entry_date END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'entry_date' AND NOT sqlc.arg(sort_desc)::boolean THEN e.id END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN e.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN e.created_at END DESC,
  e.entry_date DESC, e.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountLedgerEntries :one
SELECT COUNT(*)::bigint
FROM ledger_entries e
LEFT JOIN customers c ON c.organization_id = e.organization_id AND c.id = e.customer_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR e.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(reservation_id)::bigint IS NULL OR e.reservation_id = sqlc.narg(reservation_id)::bigint)
  AND (sqlc.narg(customer_id)::bigint IS NULL OR e.customer_id = sqlc.narg(customer_id)::bigint)
  AND (sqlc.narg(date_from)::date IS NULL OR e.entry_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR e.entry_date <= sqlc.narg(date_to)::date)
  AND (
    sqlc.narg(q)::text IS NULL
    OR e.description ILIKE '%' || sqlc.narg(q) || '%'
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: LedgerTotals :many
-- Charged (sale entries), collected and refunded per currency of one
-- reservation or one customer, reversals netted into the kind they undo.
SELECT e.currency::text AS currency,
    COALESCE(SUM(CASE WHEN COALESCE(o.kind, e.kind) = 'sale'
        THEN CASE WHEN e.direction = 'debit' THEN e.amount ELSE -e.amount END END), 0)::numeric AS charged,
    COALESCE(SUM(CASE WHEN COALESCE(o.kind, e.kind) = 'collection'
        THEN CASE WHEN e.direction = 'credit' THEN e.amount ELSE -e.amount END END), 0)::numeric AS collected,
    COALESCE(SUM(CASE WHEN COALESCE(o.kind, e.kind) = 'refund'
        THEN CASE WHEN e.direction = 'debit' THEN e.amount ELSE -e.amount END END), 0)::numeric AS refunded
FROM ledger_entries e
LEFT JOIN ledger_entries o ON o.organization_id = e.organization_id AND o.id = e.reverses_entry_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(reservation_id)::bigint IS NULL OR e.reservation_id = sqlc.narg(reservation_id)::bigint)
  AND (sqlc.narg(customer_id)::bigint IS NULL OR e.customer_id = sqlc.narg(customer_id)::bigint)
GROUP BY e.currency
ORDER BY e.currency;

-- ------------------------------------------------------------------ summary

-- name: SalesRevenueByDay :many
-- Revenue per business day and currency: sales at their gross price, the
-- discounts, the reversals (negative) and the net.
SELECT s.sale_date, s.currency::text AS currency,
    COALESCE(SUM(s.gross_amount) FILTER (WHERE s.kind = 'sale'), 0)::numeric AS gross,
    COALESCE(SUM(s.discount_amount) FILTER (WHERE s.kind = 'sale'), 0)::numeric AS discount,
    COALESCE(SUM(s.net_amount) FILTER (WHERE s.kind = 'reversal'), 0)::numeric AS reversed,
    COALESCE(SUM(s.net_amount), 0)::numeric AS net,
    COALESCE(SUM(s.net_amount) FILTER (WHERE s.item_type = 'service'), 0)::numeric AS service_net,
    COALESCE(SUM(s.net_amount) FILTER (WHERE s.item_type = 'product'), 0)::numeric AS product_net,
    COUNT(*) FILTER (WHERE s.kind = 'sale')::bigint AS sale_count
FROM sale_lines s
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.sale_date BETWEEN sqlc.arg(date_from)::date AND sqlc.arg(date_to)::date
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR s.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR s.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
GROUP BY s.sale_date, s.currency
ORDER BY s.sale_date, s.currency;

-- name: SalesCollectionsByDay :many
-- Money taken and paid back per day and currency; reversals of either are
-- netted on the day they were posted.
SELECT e.entry_date, e.currency::text AS currency,
    COALESCE(SUM(e.amount) FILTER (WHERE e.kind = 'collection'), 0)::numeric AS collected,
    COALESCE(SUM(e.amount) FILTER (WHERE e.kind = 'refund'), 0)::numeric AS refunded,
    COALESCE(SUM(CASE WHEN e.kind = 'reversal' AND o.kind = 'collection' THEN -e.amount
                      WHEN e.kind = 'reversal' AND o.kind = 'refund' THEN e.amount END), 0)::numeric AS reversed,
    COUNT(*) FILTER (WHERE e.kind = 'collection')::bigint AS collection_count
FROM ledger_entries e
LEFT JOIN ledger_entries o ON o.organization_id = e.organization_id AND o.id = e.reverses_entry_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.entry_date BETWEEN sqlc.arg(date_from)::date AND sqlc.arg(date_to)::date
  AND (e.kind IN ('collection', 'refund') OR o.kind IN ('collection', 'refund'))
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
GROUP BY e.entry_date, e.currency
ORDER BY e.entry_date, e.currency;

-- name: SalesCashByDay :many
-- Transfers between accounts and staff payouts per day and currency from
-- the cash book (reversals netted); neither is revenue.
SELECT m.movement_date, m.currency::text AS currency,
    COALESCE(SUM(CASE WHEN m.kind = 'transfer_out' THEN m.amount
                      WHEN m.kind = 'reversal' AND o.kind = 'transfer_out' THEN -m.amount END), 0)::numeric AS transfers,
    COALESCE(SUM(CASE WHEN m.kind = 'staff_payment' THEN m.amount
                      WHEN m.kind = 'reversal' AND o.kind = 'staff_payment' THEN -m.amount END), 0)::numeric AS staff_payments
FROM cash_movements m
LEFT JOIN cash_movements o ON o.organization_id = m.organization_id AND o.id = m.reverses_movement_id
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.movement_date BETWEEN sqlc.arg(date_from)::date AND sqlc.arg(date_to)::date
  AND (m.kind IN ('transfer_out', 'staff_payment') OR o.kind IN ('transfer_out', 'staff_payment'))
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR m.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR m.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
GROUP BY m.movement_date, m.currency
ORDER BY m.movement_date, m.currency;
