-- Money of walk-in product sales (TEC-992, docs/design/payments.md
-- "Product sales"). The collections taken for a stock sale and the refunds
-- paid back from them carry the sale movement as their ledger source
-- (source_type 'stock_movement', source_uuid = the movement), like the
-- sale's own revenue debit.

-- name: StockSaleLedgerTotals :one
-- Charged (live sale debits), collected and refunded of one stock sale,
-- reversals netted into the kind they undo.
SELECT
    COALESCE(SUM(CASE WHEN COALESCE(o.kind, e.kind) = 'sale'
        THEN CASE WHEN e.direction = 'debit' THEN e.amount ELSE -e.amount END END), 0)::numeric AS charged,
    COALESCE(SUM(CASE WHEN COALESCE(o.kind, e.kind) = 'collection'
        THEN CASE WHEN e.direction = 'credit' THEN e.amount ELSE -e.amount END END), 0)::numeric AS collected,
    COALESCE(SUM(CASE WHEN COALESCE(o.kind, e.kind) = 'refund'
        THEN CASE WHEN e.direction = 'debit' THEN e.amount ELSE -e.amount END END), 0)::numeric AS refunded,
    COALESCE(MAX(e.currency), '')::text AS currency
FROM ledger_entries e
LEFT JOIN ledger_entries o ON o.organization_id = e.organization_id AND o.id = e.reverses_entry_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.source_type = 'stock_movement' AND e.source_uuid = sqlc.arg(movement_uuid)::uuid;

-- name: LockStockSaleDebits :many
-- Serializes the collections of one stock sale (its sale debits; a
-- corrected sale has a reversed one and a live one).
SELECT e.id FROM ledger_entries e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.source_type = 'stock_movement' AND e.source_uuid = sqlc.arg(movement_uuid)::uuid AND e.kind = 'sale'
ORDER BY e.id
FOR UPDATE;

-- name: ListStockSaleEntries :many
-- The ledger entries of one stock sale, oldest first (same columns as
-- GetLedgerEntry).
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
  AND e.source_type = 'stock_movement' AND e.source_uuid = sqlc.arg(movement_uuid)::uuid
ORDER BY e.entry_date, e.id;

-- name: ListStockSaleCollections :many
-- The live collections of one stock sale with what was paid back from each
-- (refund entries not reversed), oldest first.
SELECT e.id, e.uuid, e.amount, e.currency, e.method, e.entry_date,
    a.uuid AS cash_account_uuid, a.name AS cash_account_name,
    COALESCE((
        SELECT SUM(f.amount) FROM ledger_entries f
        WHERE f.organization_id = e.organization_id AND f.refunds_entry_id = e.id AND f.kind = 'refund'
          AND NOT EXISTS (SELECT 1 FROM ledger_entries x WHERE x.organization_id = f.organization_id AND x.reverses_entry_id = f.id)
    ), 0)::numeric AS refunded
FROM ledger_entries e
JOIN cash_movements m ON m.organization_id = e.organization_id AND m.id = e.cash_movement_id
JOIN cash_accounts a ON a.organization_id = m.organization_id AND a.id = m.cash_account_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.source_type = 'stock_movement' AND e.source_uuid = sqlc.arg(movement_uuid)::uuid
  AND e.kind = 'collection'
  AND NOT EXISTS (SELECT 1 FROM ledger_entries r WHERE r.organization_id = e.organization_id AND r.reverses_entry_id = e.id)
ORDER BY e.created_at, e.id;

-- name: ListLedgerEntriesByIdempotencyPrefix :many
-- The entries one request wrote under a key (a refund split over several
-- collections writes key, key:2, key:3, ...).
SELECT e.uuid, e.kind, e.amount FROM ledger_entries e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND (e.idempotency_key = sqlc.arg(key)::text OR e.idempotency_key LIKE sqlc.arg(key)::text || ':%')
ORDER BY e.id;
