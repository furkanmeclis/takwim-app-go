-- TEC-925: reports. Read-only aggregates over the records of the other
-- modules, grouped by business day, branch and currency so one query feeds
-- the daily rows, the branch comparison and the totals. Every query takes
-- the organization id; branch_ids is the caller's branch scope (NULL =
-- every branch and the business-wide records), filter_branch_ids the
-- branches picked in the report (NULL = no filter; head_office adds the
-- business-wide records to a filter) and staff_ids the staff filter (NULL
-- = everyone).

-- name: ListReportBranches :many
-- The branches of the organization (names for the comparison rows).
SELECT b.id, b.uuid, b.code, b.name, b.is_active
FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id)
ORDER BY b.name, b.id;

-- name: ReportRevenue :many
-- Revenue per day, branch and currency from the sale lines (reversals
-- netted on the day they were posted) and the cost of the products sold:
-- the unit cost of the stock movement (a product handed over with a visit
-- keeps it), else the current cost of the variant / product.
SELECT s.sale_date, s.branch_id, s.currency::text AS currency,
    COALESCE(SUM(s.gross_amount) FILTER (WHERE s.kind = 'sale'), 0)::numeric AS gross,
    COALESCE(SUM(s.discount_amount) FILTER (WHERE s.kind = 'sale'), 0)::numeric AS discount,
    COALESCE(SUM(s.net_amount) FILTER (WHERE s.kind = 'reversal'), 0)::numeric AS reversed,
    COALESCE(SUM(s.net_amount), 0)::numeric AS net,
    COALESCE(SUM(s.net_amount) FILTER (WHERE s.item_type = 'service'), 0)::numeric AS service_net,
    COALESCE(SUM(s.net_amount) FILTER (WHERE s.item_type = 'product'), 0)::numeric AS product_net,
    COUNT(*) FILTER (WHERE s.kind = 'sale')::bigint AS sale_count,
    COALESCE(SUM(
        CASE WHEN s.item_type = 'product' THEN
            (CASE WHEN s.kind = 'reversal' THEN -1 ELSE 1 END) * s.quantity
            * COALESCE(sm.unit_cost, v.cost, p.cost, 0)
        END
    ), 0)::numeric AS product_cost
FROM sale_lines s
LEFT JOIN sale_lines o ON o.organization_id = s.organization_id AND o.id = s.reverses_line_id
LEFT JOIN stock_movements sm ON sm.organization_id = s.organization_id
    AND s.item_type = 'product' AND sm.uuid = COALESCE(s.source_uuid, o.source_uuid)
LEFT JOIN product_variants v ON v.organization_id = s.organization_id AND v.id = s.variant_id
LEFT JOIN products p ON p.organization_id = s.organization_id AND p.id = s.product_id
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.sale_date BETWEEN sqlc.arg(date_from)::date AND sqlc.arg(date_to)::date
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR s.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR s.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(staff_ids)::bigint[] IS NULL OR s.staff_id = ANY(sqlc.narg(staff_ids)::bigint[]))
GROUP BY s.sale_date, s.branch_id, s.currency
ORDER BY s.sale_date, s.branch_id, s.currency;

-- name: ReportCollections :many
-- Money taken and paid back per day, branch and currency (ledger);
-- reversals of either are netted on the day they were posted.
SELECT e.entry_date, e.branch_id, e.currency::text AS currency,
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
GROUP BY e.entry_date, e.branch_id, e.currency
ORDER BY e.entry_date, e.branch_id, e.currency;

-- name: ReportCashBook :many
-- Transfers between accounts, other income and expenses per day, branch of
-- the account (NULL = business-wide) and currency from the cash book;
-- reversals netted. Staff payouts come from the payouts (staff filter).
SELECT m.movement_date, m.branch_id, m.currency::text AS currency,
    COALESCE(SUM(CASE WHEN m.kind = 'transfer_out' THEN m.amount
                      WHEN m.kind = 'reversal' AND o.kind = 'transfer_out' THEN -m.amount END), 0)::numeric AS transfers,
    COALESCE(SUM(CASE WHEN m.kind = 'income' THEN m.amount
                      WHEN m.kind = 'reversal' AND o.kind = 'income' THEN -m.amount END), 0)::numeric AS income,
    COALESCE(SUM(CASE WHEN m.kind = 'expense' THEN m.amount
                      WHEN m.kind = 'reversal' AND o.kind = 'expense' THEN -m.amount END), 0)::numeric AS expenses
FROM cash_movements m
LEFT JOIN cash_movements o ON o.organization_id = m.organization_id AND o.id = m.reverses_movement_id
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.movement_date BETWEEN sqlc.arg(date_from)::date AND sqlc.arg(date_to)::date
  AND (m.kind IN ('transfer_out', 'income', 'expense') OR o.kind IN ('transfer_out', 'income', 'expense'))
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR m.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR m.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])
       OR (m.branch_id IS NULL AND sqlc.arg(head_office)::boolean))
GROUP BY m.movement_date, m.branch_id, m.currency
ORDER BY m.movement_date, m.branch_id, m.currency;

-- name: ReportStaffShares :many
-- Calculated staff shares per day, branch and currency (reversals are
-- negative rows of the day they were posted).
SELECT l.share_date, l.branch_id, l.currency::text AS currency,
    COALESCE(SUM(l.share_amount), 0)::numeric AS share
FROM staff_share_lines l
WHERE l.organization_id = sqlc.arg(organization_id)
  AND l.share_date BETWEEN sqlc.arg(date_from)::date AND sqlc.arg(date_to)::date
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR l.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR l.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(staff_ids)::bigint[] IS NULL OR l.staff_id = ANY(sqlc.narg(staff_ids)::bigint[]))
GROUP BY l.share_date, l.branch_id, l.currency
ORDER BY l.share_date, l.branch_id, l.currency;

-- name: ReportStaffPayouts :many
-- Money paid to staff per day, branch of the paying account (NULL =
-- business-wide) and currency; reversed payouts netted.
SELECT p.payout_date, p.branch_id, p.currency::text AS currency,
    COALESCE(SUM(CASE WHEN p.kind = 'payout' THEN p.amount ELSE -p.amount END), 0)::numeric AS paid
FROM staff_share_payouts p
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.payout_date BETWEEN sqlc.arg(date_from)::date AND sqlc.arg(date_to)::date
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR p.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR p.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])
       OR (p.branch_id IS NULL AND sqlc.arg(head_office)::boolean))
  AND (sqlc.narg(staff_ids)::bigint[] IS NULL OR p.staff_id = ANY(sqlc.narg(staff_ids)::bigint[]))
GROUP BY p.payout_date, p.branch_id, p.currency
ORDER BY p.payout_date, p.branch_id, p.currency;

-- name: ReportReservations :many
-- Reservations per local start day (branch time zone, else the
-- business's), branch, status and channel. With a staff filter: the visits
-- with a line of one of the staff members.
WITH rv AS (
    SELECT r.branch_id, r.status, r.source,
        (r.starts_at AT TIME ZONE COALESCE(NULLIF(b.timezone, ''), org.timezone))::date AS day
    FROM reservations r
    JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
    JOIN organizations org ON org.id = r.organization_id
    WHERE r.organization_id = sqlc.arg(organization_id)
      AND r.starts_at >= sqlc.arg(date_from)::date - INTERVAL '1 day'
      AND r.starts_at < sqlc.arg(date_to)::date + INTERVAL '2 days'
      AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
      AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
      AND (sqlc.narg(staff_ids)::bigint[] IS NULL OR EXISTS (
          SELECT 1 FROM reservation_lines l
          WHERE l.organization_id = r.organization_id AND l.reservation_id = r.id
            AND l.staff_id = ANY(sqlc.narg(staff_ids)::bigint[])
      ))
)
SELECT rv.day, rv.branch_id, rv.status::text AS status, rv.source::text AS source, COUNT(*)::bigint AS count
FROM rv
WHERE rv.day BETWEEN sqlc.arg(date_from)::date AND sqlc.arg(date_to)::date
GROUP BY rv.day, rv.branch_id, rv.status, rv.source
ORDER BY rv.day, rv.branch_id, rv.status, rv.source;

-- name: ReportConfirmations :many
-- Confirmation tasks per local day of the visit they confirm, branch and
-- state (the state the confirmation list shows: change_requested and the
-- outcome of an expired task stand apart).
WITH ct AS (
    SELECT r.branch_id,
        (CASE
            WHEN c.status IN ('scheduled', 'pending') AND c.change_requested_at IS NOT NULL THEN 'change_requested'
            WHEN c.status = 'expired' AND c.outcome IS NOT NULL THEN c.outcome
            ELSE c.status
        END)::text AS state,
        (c.starts_at AT TIME ZONE COALESCE(NULLIF(b.timezone, ''), org.timezone))::date AS day
    FROM reservation_confirmations c
    JOIN reservations r ON r.organization_id = c.organization_id AND r.id = c.reservation_id
    JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
    JOIN organizations org ON org.id = c.organization_id
    WHERE c.organization_id = sqlc.arg(organization_id)
      AND c.starts_at >= sqlc.arg(date_from)::date - INTERVAL '1 day'
      AND c.starts_at < sqlc.arg(date_to)::date + INTERVAL '2 days'
      AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
      AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
      AND (sqlc.narg(staff_ids)::bigint[] IS NULL OR EXISTS (
          SELECT 1 FROM reservation_lines l
          WHERE l.organization_id = r.organization_id AND l.reservation_id = r.id
            AND l.staff_id = ANY(sqlc.narg(staff_ids)::bigint[])
      ))
)
SELECT ct.day, ct.branch_id, ct.state, COUNT(*)::bigint AS count
FROM ct
WHERE ct.day BETWEEN sqlc.arg(date_from)::date AND sqlc.arg(date_to)::date
GROUP BY ct.day, ct.branch_id, ct.state
ORDER BY ct.day, ct.branch_id, ct.state;

-- name: ReportStockMovements :many
-- Units moved per branch and kind in the period (local day of
-- occurred_at in the branch time zone). Adjustments are signed.
WITH mv AS (
    SELECT m.branch_id, m.kind, m.quantity,
        (m.occurred_at AT TIME ZONE COALESCE(NULLIF(b.timezone, ''), org.timezone))::date AS day
    FROM stock_movements m
    JOIN branches b ON b.organization_id = m.organization_id AND b.id = m.branch_id
    JOIN organizations org ON org.id = m.organization_id
    WHERE m.organization_id = sqlc.arg(organization_id)
      AND m.occurred_at >= sqlc.arg(date_from)::date - INTERVAL '1 day'
      AND m.occurred_at < sqlc.arg(date_to)::date + INTERVAL '2 days'
      AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR m.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
      AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR m.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
)
SELECT mv.branch_id, mv.kind::text AS kind, COALESCE(SUM(mv.quantity), 0)::bigint AS quantity, COUNT(*)::bigint AS movement_count
FROM mv
WHERE mv.day BETWEEN sqlc.arg(date_from)::date AND sqlc.arg(date_to)::date
GROUP BY mv.branch_id, mv.kind
ORDER BY mv.branch_id, mv.kind;

-- name: ReportStockLevels :many
-- Stock now per branch (not bound to the period): units on hand and held,
-- the value at the current unit cost, and the items that are low (at or
-- below the threshold) or out (nothing free). Removed products are left
-- out.
SELECT st.branch_id,
    COALESCE(SUM(st.on_hand), 0)::bigint AS on_hand,
    COALESCE(SUM(st.reserved), 0)::bigint AS reserved,
    COALESCE(SUM(st.on_hand * COALESCE(v.cost, p.cost)), 0)::numeric AS stock_value,
    COUNT(*) FILTER (WHERE st.on_hand - st.reserved > 0 AND st.on_hand - st.reserved <= p.low_stock_threshold)::bigint AS low_count,
    COUNT(*) FILTER (WHERE st.on_hand - st.reserved <= 0)::bigint AS out_count,
    COUNT(*)::bigint AS item_count
FROM stock_levels st
JOIN products p ON p.organization_id = st.organization_id AND p.id = st.product_id AND p.deleted_at IS NULL
LEFT JOIN product_variants v ON v.organization_id = st.organization_id AND v.id = st.variant_id
WHERE st.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR st.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR st.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
GROUP BY st.branch_id
ORDER BY st.branch_id;

-- name: ListDailyStaffLines :many
-- TEC-926 daily summary: the lines a staff member serves on one business
-- day (branch time zone, else the business's) in the caller's branch
-- scope, with their visit. Cancelled and rejected visits are left out.
SELECT l.line_index, l.service_name, l.starts_at, l.ends_at,
       r.uuid, r.number, r.status, r.approval_status, r.confirmation_status, r.payment_status,
       b.uuid AS branch_uuid, b.name AS branch_name,
       c.first_name AS customer_first_name, c.last_name AS customer_last_name
FROM reservation_lines l
JOIN reservations r ON r.organization_id = l.organization_id AND r.id = l.reservation_id
JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
JOIN organizations org ON org.id = r.organization_id
JOIN customers c ON c.organization_id = r.organization_id AND c.id = r.customer_id
WHERE l.organization_id = sqlc.arg(organization_id) AND l.staff_id = sqlc.arg(staff_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND l.starts_at >= sqlc.arg(day)::date - INTERVAL '1 day'
  AND l.starts_at < sqlc.arg(day)::date + INTERVAL '2 days'
  AND (l.starts_at AT TIME ZONE COALESCE(NULLIF(b.timezone, ''), org.timezone))::date = sqlc.arg(day)::date
  AND r.status NOT IN ('cancelled', 'rejected')
ORDER BY l.starts_at, r.number, l.line_index;

-- name: CountPendingApprovals :one
-- TEC-926 daily summary: visits still waiting for the business to accept
-- or reject them, in the caller's branch scope (any date: each one needs
-- a decision).
SELECT COUNT(*)::bigint
FROM reservations r
WHERE r.organization_id = sqlc.arg(organization_id)
  AND r.status = 'requested' AND r.approval_status = 'pending'
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]));
