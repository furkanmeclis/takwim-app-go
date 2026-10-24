-- TEC-917: staff share rules, the share of each sale line and share
-- payouts. Every query takes the organization id.

-- ------------------------------------------------------------------ lookups

-- name: GetShareSaleLine :one
-- The sale line a share is calculated from.
SELECT s.id, s.kind, s.item_type, s.branch_id, s.reservation_id, s.service_id, s.product_id, s.staff_id,
    s.quantity, s.net_amount, s.currency, s.sale_date, s.reverses_line_id, s.replaces_line_id
FROM sale_lines s
WHERE s.organization_id = sqlc.arg(organization_id) AND s.id = sqlc.arg(id);

-- name: IsShareStaffOwner :one
-- The staff member is the account of a business owner (their own services
-- stay in the business share).
SELECT EXISTS (
    SELECT 1
    FROM staff st
    JOIN organization_members om ON om.organization_id = st.organization_id AND om.user_id = st.user_id
    JOIN organization_member_roles omr ON omr.organization_id = om.organization_id AND omr.member_id = om.id
    JOIN roles r ON r.id = omr.role_id AND r.scope = 'tenant' AND r.organization_id IS NULL AND r.slug = 'owner'
    WHERE st.organization_id = sqlc.arg(organization_id) AND st.id = sqlc.arg(staff_id)
)::boolean AS is_owner;

-- name: ResolveShareRule :one
-- The rule of a staff member for an item on a day: a rule of the service
-- (or of the product, TEC-976) wins over the default of the item type;
-- among them the latest start on or before the day.
SELECT r.id, r.service_id, r.product_id, r.basis, r.rate, r.fixed_amount
FROM staff_share_rules r
WHERE r.organization_id = sqlc.arg(organization_id) AND r.staff_id = sqlc.arg(staff_id)
  AND r.item_type = sqlc.arg(item_type)
  AND (r.service_id IS NULL OR r.service_id = sqlc.narg(service_id)::bigint)
  AND (r.product_id IS NULL OR r.product_id = sqlc.narg(product_id)::bigint)
  AND r.valid_from <= sqlc.arg(on_date)::date
ORDER BY (r.service_id IS NOT NULL OR r.product_id IS NOT NULL) DESC, r.valid_from DESC, r.id DESC
LIMIT 1;

-- name: GetShareStaffByUUID :one
SELECT st.id, st.uuid, st.name FROM staff st
WHERE st.organization_id = sqlc.arg(organization_id) AND st.uuid = sqlc.arg(uuid) AND st.deleted_at IS NULL;

-- name: GetShareStaffByUser :one
SELECT st.id, st.uuid, st.name FROM staff st
WHERE st.organization_id = sqlc.arg(organization_id) AND st.user_id = sqlc.arg(user_id) AND st.deleted_at IS NULL;

-- name: GetShareServiceByUUID :one
SELECT sv.id, sv.uuid, sv.name FROM services sv
WHERE sv.organization_id = sqlc.arg(organization_id) AND sv.uuid = sqlc.arg(uuid) AND sv.deleted_at IS NULL;

-- name: GetShareProductByUUID :one
SELECT p.id, p.uuid, p.name FROM products p
WHERE p.organization_id = sqlc.arg(organization_id) AND p.uuid = sqlc.arg(uuid) AND p.deleted_at IS NULL;

-- name: LockShareStaff :one
-- Serializes the payouts of one staff member.
SELECT st.id FROM staff st
WHERE st.organization_id = sqlc.arg(organization_id) AND st.id = sqlc.arg(id)
FOR UPDATE;

-- -------------------------------------------------------------------- rules

-- name: InsertShareRule :one
INSERT INTO staff_share_rules (
    organization_id, staff_id, item_type, service_id, product_id, basis, rate, fixed_amount, valid_from, note,
    created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(staff_id), sqlc.arg(item_type), sqlc.narg(service_id),
    sqlc.narg(product_id), sqlc.arg(basis),
    sqlc.narg(rate), sqlc.narg(fixed_amount), sqlc.arg(valid_from), sqlc.arg(note), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: UpdateShareRule :execrows
UPDATE staff_share_rules
SET basis = sqlc.arg(basis), rate = sqlc.narg(rate), fixed_amount = sqlc.narg(fixed_amount),
    valid_from = sqlc.arg(valid_from), note = sqlc.arg(note)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: DeleteShareRule :execrows
DELETE FROM staff_share_rules
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: GetShareRule :one
SELECT r.id, r.uuid, r.staff_id, r.item_type, r.basis, r.rate, r.fixed_amount, r.valid_from, r.note,
    r.created_at, r.updated_at,
    st.uuid AS staff_uuid, st.name AS staff_name,
    sv.uuid AS service_uuid, sv.name AS service_name,
    pr.uuid AS product_uuid, pr.name AS product_name,
    (SELECT COUNT(*) FROM staff_share_lines l
     WHERE l.organization_id = r.organization_id AND l.rule_id = r.id)::bigint AS share_count
FROM staff_share_rules r
JOIN staff st ON st.organization_id = r.organization_id AND st.id = r.staff_id
LEFT JOIN services sv ON sv.organization_id = r.organization_id AND sv.id = r.service_id
LEFT JOIN products pr ON pr.organization_id = r.organization_id AND pr.id = r.product_id
WHERE r.organization_id = sqlc.arg(organization_id) AND r.uuid = sqlc.arg(uuid);

-- name: ListShareRules :many
SELECT r.id, r.uuid, r.staff_id, r.item_type, r.basis, r.rate, r.fixed_amount, r.valid_from, r.note,
    r.created_at, r.updated_at,
    st.uuid AS staff_uuid, st.name AS staff_name,
    sv.uuid AS service_uuid, sv.name AS service_name,
    pr.uuid AS product_uuid, pr.name AS product_name,
    (SELECT COUNT(*) FROM staff_share_lines l
     WHERE l.organization_id = r.organization_id AND l.rule_id = r.id)::bigint AS share_count
FROM staff_share_rules r
JOIN staff st ON st.organization_id = r.organization_id AND st.id = r.staff_id
LEFT JOIN services sv ON sv.organization_id = r.organization_id AND sv.id = r.service_id
LEFT JOIN products pr ON pr.organization_id = r.organization_id AND pr.id = r.product_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(staff_ids)::bigint[] IS NULL OR r.staff_id = ANY(sqlc.narg(staff_ids)::bigint[]))
  AND (sqlc.narg(item_types)::text[] IS NULL OR r.item_type = ANY(sqlc.narg(item_types)::text[]))
ORDER BY st.name, r.item_type DESC, (r.service_id IS NOT NULL OR r.product_id IS NOT NULL),
    COALESCE(sv.name, pr.name) NULLS FIRST, r.valid_from DESC, r.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountShareRules :one
SELECT COUNT(*)::bigint FROM staff_share_rules r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(staff_ids)::bigint[] IS NULL OR r.staff_id = ANY(sqlc.narg(staff_ids)::bigint[]))
  AND (sqlc.narg(item_types)::text[] IS NULL OR r.item_type = ANY(sqlc.narg(item_types)::text[]));

-- ------------------------------------------------------------- share lines

-- name: GetShareLineBySaleLine :one
SELECT l.id, l.uuid, l.staff_id, l.kind, l.item_type, l.rule_id, l.rule_scope, l.basis, l.rate, l.fixed_amount,
    l.base_amount, l.share_amount
FROM staff_share_lines l
WHERE l.organization_id = sqlc.arg(organization_id) AND l.sale_line_id = sqlc.arg(sale_line_id);

-- name: InsertShareLine :one
INSERT INTO staff_share_lines (
    organization_id, branch_id, staff_id, sale_line_id, reservation_id, kind, reverses_share_id, item_type,
    rule_id, rule_scope, basis, rate, fixed_amount, base_amount, share_amount, currency, share_date
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(branch_id), sqlc.arg(staff_id), sqlc.arg(sale_line_id),
    sqlc.narg(reservation_id), sqlc.arg(kind), sqlc.narg(reverses_share_id), sqlc.arg(item_type),
    sqlc.narg(rule_id), sqlc.arg(rule_scope), sqlc.arg(basis), sqlc.narg(rate), sqlc.narg(fixed_amount),
    sqlc.arg(base_amount), sqlc.arg(share_amount), sqlc.arg(currency), sqlc.arg(share_date)
)
RETURNING id, uuid;

-- name: ListShareLines :many
-- A share is payable once its visit is paid in full (collections minus
-- refunds cover the posted sales); a counter sale is payable at once.
SELECT
    l.id, l.uuid, l.kind, l.item_type, l.rule_scope, l.basis, l.rate, l.fixed_amount, l.base_amount,
    l.share_amount, l.currency, l.share_date, l.created_at,
    b.uuid AS branch_uuid, b.name AS branch_name,
    st.uuid AS staff_uuid, st.name AS staff_name,
    s.uuid AS sale_line_uuid, s.description, s.quantity,
    r.uuid AS reservation_uuid, r.number AS reservation_number,
    c.uuid AS customer_uuid, c.first_name AS customer_first_name, c.last_name AS customer_last_name,
    rl.uuid AS rule_uuid, rv.uuid AS reverses_uuid, rb.uuid AS reversed_by_uuid,
    (pay.paid >= pay.charged)::boolean AS collected
FROM staff_share_lines l
JOIN branches b ON b.organization_id = l.organization_id AND b.id = l.branch_id
JOIN staff st ON st.organization_id = l.organization_id AND st.id = l.staff_id
JOIN sale_lines s ON s.organization_id = l.organization_id AND s.id = l.sale_line_id
LEFT JOIN reservations r ON r.organization_id = l.organization_id AND r.id = l.reservation_id
LEFT JOIN customers c ON c.organization_id = s.organization_id AND c.id = s.customer_id
LEFT JOIN staff_share_rules rl ON rl.organization_id = l.organization_id AND rl.id = l.rule_id
LEFT JOIN staff_share_lines rv ON rv.organization_id = l.organization_id AND rv.id = l.reverses_share_id
LEFT JOIN staff_share_lines rb ON rb.organization_id = l.organization_id AND rb.reverses_share_id = l.id
CROSS JOIN LATERAL (
    SELECT
        COALESCE(SUM(CASE WHEN COALESCE(o.kind, e.kind) = 'sale'
            THEN CASE WHEN e.direction = 'debit' THEN e.amount ELSE -e.amount END END), 0)::numeric AS charged,
        COALESCE(SUM(CASE WHEN COALESCE(o.kind, e.kind) IN ('collection', 'refund')
            THEN CASE WHEN e.direction = 'credit' THEN e.amount ELSE -e.amount END END), 0)::numeric AS paid
    FROM ledger_entries e
    LEFT JOIN ledger_entries o ON o.organization_id = e.organization_id AND o.id = e.reverses_entry_id
    WHERE e.organization_id = l.organization_id AND e.reservation_id = l.reservation_id
) pay
WHERE l.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR l.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR l.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(staff_ids)::bigint[] IS NULL OR l.staff_id = ANY(sqlc.narg(staff_ids)::bigint[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR l.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(item_types)::text[] IS NULL OR l.item_type = ANY(sqlc.narg(item_types)::text[]))
  AND (sqlc.narg(collected)::boolean IS NULL OR (pay.paid >= pay.charged) = sqlc.narg(collected)::boolean)
  AND (sqlc.narg(date_from)::date IS NULL OR l.share_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR l.share_date <= sqlc.narg(date_to)::date)
  AND (
    sqlc.narg(q)::text IS NULL
    OR s.description ILIKE '%' || sqlc.narg(q) || '%'
    OR st.name ILIKE '%' || sqlc.narg(q) || '%'
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'share_amount' AND NOT sqlc.arg(sort_desc)::boolean THEN l.share_amount END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'share_amount' AND sqlc.arg(sort_desc)::boolean THEN l.share_amount END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'base_amount' AND NOT sqlc.arg(sort_desc)::boolean THEN l.base_amount END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'base_amount' AND sqlc.arg(sort_desc)::boolean THEN l.base_amount END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'share_date' AND NOT sqlc.arg(sort_desc)::boolean THEN l.share_date END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'share_date' AND NOT sqlc.arg(sort_desc)::boolean THEN l.id END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN l.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN l.created_at END DESC,
  l.share_date DESC, l.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountShareLines :one
SELECT COUNT(*)::bigint
FROM staff_share_lines l
JOIN staff st ON st.organization_id = l.organization_id AND st.id = l.staff_id
JOIN sale_lines s ON s.organization_id = l.organization_id AND s.id = l.sale_line_id
LEFT JOIN customers c ON c.organization_id = s.organization_id AND c.id = s.customer_id
CROSS JOIN LATERAL (
    SELECT
        COALESCE(SUM(CASE WHEN COALESCE(o.kind, e.kind) = 'sale'
            THEN CASE WHEN e.direction = 'debit' THEN e.amount ELSE -e.amount END END), 0)::numeric AS charged,
        COALESCE(SUM(CASE WHEN COALESCE(o.kind, e.kind) IN ('collection', 'refund')
            THEN CASE WHEN e.direction = 'credit' THEN e.amount ELSE -e.amount END END), 0)::numeric AS paid
    FROM ledger_entries e
    LEFT JOIN ledger_entries o ON o.organization_id = e.organization_id AND o.id = e.reverses_entry_id
    WHERE e.organization_id = l.organization_id AND e.reservation_id = l.reservation_id
) pay
WHERE l.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR l.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR l.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(staff_ids)::bigint[] IS NULL OR l.staff_id = ANY(sqlc.narg(staff_ids)::bigint[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR l.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(item_types)::text[] IS NULL OR l.item_type = ANY(sqlc.narg(item_types)::text[]))
  AND (sqlc.narg(collected)::boolean IS NULL OR (pay.paid >= pay.charged) = sqlc.narg(collected)::boolean)
  AND (sqlc.narg(date_from)::date IS NULL OR l.share_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR l.share_date <= sqlc.narg(date_to)::date)
  AND (
    sqlc.narg(q)::text IS NULL
    OR s.description ILIKE '%' || sqlc.narg(q) || '%'
    OR st.name ILIKE '%' || sqlc.narg(q) || '%'
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
  );

-- ------------------------------------------------------------------ totals

-- name: StaffSharePeriodTotals :many
-- Per staff member and currency inside the caller's branch scope: the
-- staff member's sales (net), the shares and the part still waiting for
-- the collection, in [date_from, date_to].
SELECT s.staff_id, s.currency::text AS currency,
    COALESCE(SUM(s.net_amount), 0)::numeric AS sales,
    COALESCE(SUM(l.share_amount), 0)::numeric AS share,
    COALESCE(SUM(l.share_amount) FILTER (WHERE NOT (pay.paid >= pay.charged)), 0)::numeric AS pending,
    COUNT(*) FILTER (WHERE s.kind = 'sale')::bigint AS sale_count
FROM sale_lines s
LEFT JOIN staff_share_lines l ON l.organization_id = s.organization_id AND l.sale_line_id = s.id
CROSS JOIN LATERAL (
    SELECT
        COALESCE(SUM(CASE WHEN COALESCE(o.kind, e.kind) = 'sale'
            THEN CASE WHEN e.direction = 'debit' THEN e.amount ELSE -e.amount END END), 0)::numeric AS charged,
        COALESCE(SUM(CASE WHEN COALESCE(o.kind, e.kind) IN ('collection', 'refund')
            THEN CASE WHEN e.direction = 'credit' THEN e.amount ELSE -e.amount END END), 0)::numeric AS paid
    FROM ledger_entries e
    LEFT JOIN ledger_entries o ON o.organization_id = e.organization_id AND o.id = e.reverses_entry_id
    WHERE e.organization_id = s.organization_id AND e.reservation_id = s.reservation_id
) pay
WHERE s.organization_id = sqlc.arg(organization_id) AND s.staff_id IS NOT NULL
  AND s.sale_date BETWEEN sqlc.arg(date_from)::date AND sqlc.arg(date_to)::date
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR s.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR s.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(staff_ids)::bigint[] IS NULL OR s.staff_id = ANY(sqlc.narg(staff_ids)::bigint[]))
GROUP BY s.staff_id, s.currency;

-- name: StaffShareBalances :many
-- What each staff member earned and what of it is payable, over every
-- branch and day (a person's balance is one figure).
SELECT l.staff_id, l.currency::text AS currency,
    COALESCE(SUM(l.share_amount), 0)::numeric AS earned,
    COALESCE(SUM(l.share_amount) FILTER (WHERE pay.paid >= pay.charged), 0)::numeric AS payable
FROM staff_share_lines l
CROSS JOIN LATERAL (
    SELECT
        COALESCE(SUM(CASE WHEN COALESCE(o.kind, e.kind) = 'sale'
            THEN CASE WHEN e.direction = 'debit' THEN e.amount ELSE -e.amount END END), 0)::numeric AS charged,
        COALESCE(SUM(CASE WHEN COALESCE(o.kind, e.kind) IN ('collection', 'refund')
            THEN CASE WHEN e.direction = 'credit' THEN e.amount ELSE -e.amount END END), 0)::numeric AS paid
    FROM ledger_entries e
    LEFT JOIN ledger_entries o ON o.organization_id = e.organization_id AND o.id = e.reverses_entry_id
    WHERE e.organization_id = l.organization_id AND e.reservation_id = l.reservation_id
) pay
WHERE l.organization_id = sqlc.arg(organization_id)
  AND l.staff_id = ANY(sqlc.arg(staff_ids)::bigint[])
GROUP BY l.staff_id, l.currency;

-- name: StaffPayoutTotals :many
-- Paid to each staff member (payouts minus reversals): over every account
-- and day (paid), and in [date_from, date_to] from the accounts inside the
-- caller's branch scope (period_paid).
SELECT p.staff_id, p.currency::text AS currency,
    COALESCE(SUM(CASE WHEN p.kind = 'payout' THEN p.amount ELSE -p.amount END), 0)::numeric AS paid,
    COALESCE(SUM(CASE WHEN p.kind = 'payout' THEN p.amount ELSE -p.amount END) FILTER (
        WHERE p.payout_date BETWEEN sqlc.arg(date_from)::date AND sqlc.arg(date_to)::date
          AND (NOT sqlc.arg(limited)::boolean OR p.branch_id = ANY(sqlc.arg(branch_ids)::bigint[]))
          AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR p.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
    ), 0)::numeric AS period_paid
FROM staff_share_payouts p
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.staff_id = ANY(sqlc.arg(staff_ids)::bigint[])
GROUP BY p.staff_id, p.currency;

-- name: ListShareSummaryStaff :many
-- The staff members a summary shows: those with sales or shares in the
-- caller's branch scope, or with payouts from the caller's accounts.
SELECT st.id, st.uuid, st.name FROM staff st
WHERE st.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(staff_ids)::bigint[] IS NULL OR st.id = ANY(sqlc.narg(staff_ids)::bigint[]))
  AND (
    EXISTS (SELECT 1 FROM sale_lines s
            WHERE s.organization_id = st.organization_id AND s.staff_id = st.id
              AND (NOT sqlc.arg(limited)::boolean OR s.branch_id = ANY(sqlc.arg(branch_ids)::bigint[]))
              AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR s.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])))
    OR EXISTS (SELECT 1 FROM staff_share_payouts p
            WHERE p.organization_id = st.organization_id AND p.staff_id = st.id
              AND (NOT sqlc.arg(limited)::boolean OR p.branch_id = ANY(sqlc.arg(branch_ids)::bigint[]))
              AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR p.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])))
  )
ORDER BY st.name, st.id;

-- name: BranchShareTotals :many
-- Per branch and currency in [date_from, date_to]: revenue (net sales),
-- the staff shares of it and what stays with the business (before costs
-- and expenses).
SELECT b.uuid AS branch_uuid, b.name AS branch_name, s.currency::text AS currency,
    COALESCE(SUM(s.net_amount), 0)::numeric AS revenue,
    COALESCE(SUM(l.share_amount), 0)::numeric AS share
FROM sale_lines s
JOIN branches b ON b.organization_id = s.organization_id AND b.id = s.branch_id
LEFT JOIN staff_share_lines l ON l.organization_id = s.organization_id AND l.sale_line_id = s.id
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.sale_date BETWEEN sqlc.arg(date_from)::date AND sqlc.arg(date_to)::date
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR s.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR s.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
GROUP BY b.uuid, b.name, s.currency
ORDER BY b.name, s.currency;

-- ------------------------------------------------------------------ payouts

-- name: InsertSharePayout :one
INSERT INTO staff_share_payouts (
    uuid, organization_id, staff_id, branch_id, cash_account_id, cash_movement_id, kind, reverses_payout_id, amount,
    currency, payout_date, note, idempotency_key, created_by
) VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.arg(staff_id), sqlc.narg(branch_id), sqlc.arg(cash_account_id),
    sqlc.arg(cash_movement_id), sqlc.arg(kind), sqlc.narg(reverses_payout_id), sqlc.arg(amount), sqlc.arg(currency),
    sqlc.arg(payout_date), sqlc.arg(note), sqlc.narg(idempotency_key), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: GetSharePayoutByIdempotencyKey :one
SELECT p.id, p.uuid, p.staff_id, p.amount, p.kind FROM staff_share_payouts p
WHERE p.organization_id = sqlc.arg(organization_id) AND p.idempotency_key = sqlc.arg(idempotency_key);

-- name: GetSharePayoutRow :one
SELECT p.*,
    m.uuid AS cash_movement_uuid,
    EXISTS (SELECT 1 FROM staff_share_payouts r WHERE r.organization_id = p.organization_id AND r.reverses_payout_id = p.id)::boolean AS reversed
FROM staff_share_payouts p
JOIN cash_movements m ON m.organization_id = p.organization_id AND m.id = p.cash_movement_id
WHERE p.organization_id = sqlc.arg(organization_id) AND p.uuid = sqlc.arg(uuid);

-- name: GetSharePayout :one
SELECT p.id, p.uuid, p.kind, p.amount, p.currency, p.payout_date, p.note, p.created_at, p.branch_id, p.staff_id,
    st.uuid AS staff_uuid, st.name AS staff_name,
    a.uuid AS cash_account_uuid, a.name AS cash_account_name,
    m.uuid AS cash_movement_uuid,
    b.uuid AS branch_uuid, b.name AS branch_name,
    rv.uuid AS reverses_uuid, rb.uuid AS reversed_by_uuid,
    u.uuid AS creator_uuid, u.name AS creator_name, u.surname AS creator_surname
FROM staff_share_payouts p
JOIN staff st ON st.organization_id = p.organization_id AND st.id = p.staff_id
JOIN cash_accounts a ON a.organization_id = p.organization_id AND a.id = p.cash_account_id
JOIN cash_movements m ON m.organization_id = p.organization_id AND m.id = p.cash_movement_id
LEFT JOIN branches b ON b.organization_id = p.organization_id AND b.id = p.branch_id
LEFT JOIN staff_share_payouts rv ON rv.organization_id = p.organization_id AND rv.id = p.reverses_payout_id
LEFT JOIN staff_share_payouts rb ON rb.organization_id = p.organization_id AND rb.reverses_payout_id = p.id
LEFT JOIN users u ON u.id = p.created_by
WHERE p.organization_id = sqlc.arg(organization_id) AND p.uuid = sqlc.arg(uuid);

-- name: ListSharePayouts :many
SELECT p.id, p.uuid, p.kind, p.amount, p.currency, p.payout_date, p.note, p.created_at, p.branch_id, p.staff_id,
    st.uuid AS staff_uuid, st.name AS staff_name,
    a.uuid AS cash_account_uuid, a.name AS cash_account_name,
    m.uuid AS cash_movement_uuid,
    b.uuid AS branch_uuid, b.name AS branch_name,
    rv.uuid AS reverses_uuid, rb.uuid AS reversed_by_uuid,
    u.uuid AS creator_uuid, u.name AS creator_name, u.surname AS creator_surname
FROM staff_share_payouts p
JOIN staff st ON st.organization_id = p.organization_id AND st.id = p.staff_id
JOIN cash_accounts a ON a.organization_id = p.organization_id AND a.id = p.cash_account_id
JOIN cash_movements m ON m.organization_id = p.organization_id AND m.id = p.cash_movement_id
LEFT JOIN branches b ON b.organization_id = p.organization_id AND b.id = p.branch_id
LEFT JOIN staff_share_payouts rv ON rv.organization_id = p.organization_id AND rv.id = p.reverses_payout_id
LEFT JOIN staff_share_payouts rb ON rb.organization_id = p.organization_id AND rb.reverses_payout_id = p.id
LEFT JOIN users u ON u.id = p.created_by
WHERE p.organization_id = sqlc.arg(organization_id)
  AND (NOT sqlc.arg(limited)::boolean OR p.branch_id = ANY(sqlc.arg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR p.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(staff_ids)::bigint[] IS NULL OR p.staff_id = ANY(sqlc.narg(staff_ids)::bigint[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR p.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(date_from)::date IS NULL OR p.payout_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR p.payout_date <= sqlc.narg(date_to)::date)
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.note ILIKE '%' || sqlc.narg(q) || '%'
    OR st.name ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND NOT sqlc.arg(sort_desc)::boolean THEN p.amount END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND sqlc.arg(sort_desc)::boolean THEN p.amount END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'payout_date' AND NOT sqlc.arg(sort_desc)::boolean THEN p.payout_date END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'payout_date' AND NOT sqlc.arg(sort_desc)::boolean THEN p.id END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN p.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN p.created_at END DESC,
  p.payout_date DESC, p.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountSharePayouts :one
SELECT COUNT(*)::bigint
FROM staff_share_payouts p
JOIN staff st ON st.organization_id = p.organization_id AND st.id = p.staff_id
WHERE p.organization_id = sqlc.arg(organization_id)
  AND (NOT sqlc.arg(limited)::boolean OR p.branch_id = ANY(sqlc.arg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR p.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(staff_ids)::bigint[] IS NULL OR p.staff_id = ANY(sqlc.narg(staff_ids)::bigint[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR p.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(date_from)::date IS NULL OR p.payout_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR p.payout_date <= sqlc.narg(date_to)::date)
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.note ILIKE '%' || sqlc.narg(q) || '%'
    OR st.name ILIKE '%' || sqlc.narg(q) || '%'
  );
