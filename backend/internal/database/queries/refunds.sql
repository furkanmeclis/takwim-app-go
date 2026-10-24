-- Refunds and the payment follow-up list (TEC-913, docs/design/payments.md).

-- name: InsertRefund :one
INSERT INTO refunds (
    organization_id, branch_id, reservation_id, collection_entry_id, payment_id, integration_account_id, channel,
    provider, cash_account_id, method, amount, currency, reason, idempotency_key, requested_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(branch_id), sqlc.arg(reservation_id), sqlc.arg(collection_entry_id),
    sqlc.narg(payment_id), sqlc.narg(integration_account_id), sqlc.arg(channel), sqlc.arg(provider),
    sqlc.arg(cash_account_id), sqlc.arg(method), sqlc.arg(amount), sqlc.arg(currency), sqlc.arg(reason),
    sqlc.arg(idempotency_key), sqlc.narg(requested_by)
)
RETURNING *;

-- name: GetRefundByIdempotencyKey :one
SELECT f.*, r.uuid AS reservation_uuid FROM refunds f
JOIN reservations r ON r.organization_id = f.organization_id AND r.id = f.reservation_id
WHERE f.organization_id = sqlc.arg(organization_id) AND f.idempotency_key = sqlc.arg(idempotency_key);

-- name: GetRefundByUUID :one
SELECT f.*, r.uuid AS reservation_uuid FROM refunds f
JOIN reservations r ON r.organization_id = f.organization_id AND r.id = f.reservation_id
WHERE f.organization_id = sqlc.arg(organization_id) AND f.uuid = sqlc.arg(uuid);

-- name: LockRefund :one
SELECT * FROM refunds
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
FOR UPDATE;

-- name: SetRefundProviderAnswer :execrows
-- The provider's answer to a refund that is still requested. A failed
-- answer ends the refund; succeeded / uncertain wait for the posting / a
-- person.
UPDATE refunds
SET provider_status = sqlc.arg(provider_status)::text, provider_refund_id = sqlc.arg(provider_refund_id),
    failure_reason = sqlc.arg(failure_reason), provider_answered_at = NOW(),
    status = CASE WHEN sqlc.arg(provider_status)::text = 'failed' THEN 'failed' ELSE status END
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'requested';

-- name: MarkRefundPosted :execrows
-- The refund ledger entry is written: the refund is done.
UPDATE refunds
SET status = 'succeeded', ledger_entry_id = sqlc.arg(ledger_entry_id), reflected_at = NOW(), failure_reason = ''
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'requested';

-- name: ResolveRefundFailed :execrows
-- A person settled an unknown result: the provider did not refund.
UPDATE refunds
SET status = 'failed', provider_status = 'failed', failure_reason = sqlc.arg(failure_reason),
    resolved_by = sqlc.narg(resolved_by), resolution_note = sqlc.arg(resolution_note),
    provider_answered_at = COALESCE(provider_answered_at, NOW())
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'requested';

-- name: SetRefundResolver :exec
UPDATE refunds
SET resolved_by = sqlc.narg(resolved_by), resolution_note = sqlc.arg(resolution_note), provider_status = 'succeeded'
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: ListReservationRefunds :many
SELECT f.uuid, f.channel, f.provider, f.method, f.amount, f.currency, f.status, f.provider_status,
    f.provider_refund_id, f.failure_reason, f.reason, f.provider_answered_at, f.reflected_at, f.resolution_note,
    f.created_at, c.uuid AS collection_uuid, a.uuid AS cash_account_uuid, a.name AS cash_account_name,
    le.uuid AS ledger_entry_uuid, u.uuid AS requested_by_uuid, u.name AS requested_by_name,
    u.surname AS requested_by_surname
FROM refunds f
JOIN ledger_entries c ON c.organization_id = f.organization_id AND c.id = f.collection_entry_id
JOIN cash_accounts a ON a.organization_id = f.organization_id AND a.id = f.cash_account_id
LEFT JOIN ledger_entries le ON le.organization_id = f.organization_id AND le.id = f.ledger_entry_id
LEFT JOIN users u ON u.id = f.requested_by
WHERE f.organization_id = sqlc.arg(organization_id) AND f.reservation_id = sqlc.arg(reservation_id)
ORDER BY f.created_at DESC, f.id DESC
LIMIT 100;

-- name: ListRefundableCollections :many
-- The live collections of a reservation with what was paid back from each
-- (refund entries not reversed) and what is being refunded (requested
-- refunds), and the online payment behind a collection.
SELECT e.id, e.uuid, e.amount, e.currency, e.method, e.entry_date, e.created_at,
    a.id AS cash_account_id, a.uuid AS cash_account_uuid, a.name AS cash_account_name, a.kind AS cash_account_kind,
    COALESCE((
        SELECT SUM(f.amount) FROM ledger_entries f
        WHERE f.organization_id = e.organization_id AND f.refunds_entry_id = e.id
          AND NOT EXISTS (SELECT 1 FROM ledger_entries x WHERE x.organization_id = f.organization_id AND x.reverses_entry_id = f.id)
    ), 0)::numeric AS refunded,
    COALESCE((
        SELECT SUM(rq.amount) FROM refunds rq
        WHERE rq.organization_id = e.organization_id AND rq.collection_entry_id = e.id AND rq.status = 'requested'
    ), 0)::numeric AS pending,
    p.id AS payment_id, p.provider AS payment_provider, p.integration_account_id AS payment_account_id,
    p.order_ref AS payment_order_ref, p.provider_payment_id AS payment_provider_ref
FROM ledger_entries e
JOIN cash_movements m ON m.organization_id = e.organization_id AND m.id = e.cash_movement_id
JOIN cash_accounts a ON a.organization_id = m.organization_id AND a.id = m.cash_account_id
LEFT JOIN payments p ON p.organization_id = e.organization_id AND p.ledger_entry_id = e.id AND p.status = 'succeeded'
WHERE e.organization_id = sqlc.arg(organization_id) AND e.reservation_id = sqlc.arg(reservation_id)
  AND e.kind = 'collection'
  AND NOT EXISTS (SELECT 1 FROM ledger_entries r WHERE r.organization_id = e.organization_id AND r.reverses_entry_id = e.id)
ORDER BY e.created_at, e.id;

-- name: GetRefundAccount :one
SELECT id, uuid, branch_id, kind, status, currency, name FROM cash_accounts
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: GetCashAccountUUIDByID :one
SELECT uuid FROM cash_accounts WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: ListRefundAccounts :many
-- Accounts a manual refund may leave: tills, bank and online accounts of
-- the branch or business-wide, in the currency.
SELECT a.uuid, a.name, a.kind, a.currency, a.branch_id FROM cash_accounts a
WHERE a.organization_id = sqlc.arg(organization_id) AND a.kind IN ('cash', 'bank', 'online') AND a.status = 'active'
  AND a.currency = sqlc.arg(currency) AND (a.branch_id = sqlc.arg(branch_id) OR a.branch_id IS NULL)
ORDER BY (a.branch_id IS NULL), a.kind, a.sort_order, a.name;

-- name: ListCustomerRefunds :many
-- What the customer panel shows of a visit's refunds.
SELECT f.uuid, f.amount, f.currency, f.status, f.provider_status, f.created_at, f.reflected_at
FROM refunds f
WHERE f.organization_id = sqlc.arg(organization_id) AND f.reservation_id = sqlc.arg(reservation_id)
ORDER BY f.created_at DESC, f.id DESC
LIMIT 20;

-- name: ListPaymentFollowUps :many
-- Visits whose money needs a person: a refund waiting (cancelled after it
-- was paid, a late online payment), a refund whose provider result is
-- unknown, a last refund that failed while money is still held, and
-- finished visits not paid in full.
WITH money AS (
    SELECT e.reservation_id,
        COALESCE(SUM(CASE WHEN COALESCE(o.kind, e.kind) = 'sale'
            THEN CASE WHEN e.direction = 'debit' THEN e.amount ELSE -e.amount END END), 0)::numeric AS charged,
        COALESCE(SUM(CASE WHEN COALESCE(o.kind, e.kind) = 'collection'
            THEN CASE WHEN e.direction = 'credit' THEN e.amount ELSE -e.amount END END), 0)::numeric AS collected,
        COALESCE(SUM(CASE WHEN COALESCE(o.kind, e.kind) = 'refund'
            THEN CASE WHEN e.direction = 'debit' THEN e.amount ELSE -e.amount END END), 0)::numeric AS refunded
    FROM ledger_entries e
    LEFT JOIN ledger_entries o ON o.organization_id = e.organization_id AND o.id = e.reverses_entry_id
    WHERE e.organization_id = sqlc.arg(organization_id) AND e.reservation_id IS NOT NULL
    GROUP BY e.reservation_id
), refund_state AS (
    SELECT f.reservation_id,
        COUNT(*) FILTER (WHERE f.status = 'requested')::bigint AS open_count,
        (array_agg(f.status ORDER BY f.created_at DESC, f.id DESC))[1]::text AS last_status,
        (array_agg(f.failure_reason ORDER BY f.created_at DESC, f.id DESC))[1]::text AS last_failure,
        MAX(f.created_at)::timestamptz AS last_at
    FROM refunds f
    WHERE f.organization_id = sqlc.arg(organization_id)
    GROUP BY f.reservation_id
), base AS (
    SELECT r.id, r.uuid, r.number, r.status, r.payment_status, r.starts_at, r.currency, r.cancel_reason, r.updated_at,
        b.uuid AS branch_uuid, b.name AS branch_name,
        c.uuid AS customer_uuid, c.first_name AS customer_first_name, c.last_name AS customer_last_name,
        COALESCE(m.charged, 0)::numeric AS charged, COALESCE(m.collected, 0)::numeric AS collected,
        COALESCE(m.refunded, 0)::numeric AS refunded,
        COALESCE(rs.open_count, 0)::bigint AS open_refunds, COALESCE(rs.last_status, '')::text AS last_refund_status,
        COALESCE(rs.last_failure, '')::text AS last_refund_failure, rs.last_at AS last_refund_at,
        EXISTS (
            SELECT 1 FROM payments p
            WHERE p.organization_id = r.organization_id AND p.reservation_id = r.id AND p.outcome = 'late'
        )::boolean AS late,
        (CASE
            WHEN COALESCE(rs.open_count, 0) > 0 THEN 'refund_unverified'
            WHEN rs.last_status = 'failed' AND COALESCE(m.collected, 0) - COALESCE(m.refunded, 0) > 0 THEN 'refund_failed'
            WHEN r.payment_status = 'refund_pending' THEN 'refund_pending'
            WHEN r.status = 'completed'
                AND COALESCE(m.charged, 0) - COALESCE(m.collected, 0) + COALESCE(m.refunded, 0) > 0 THEN 'unpaid'
            ELSE ''
        END)::text AS kind
    FROM reservations r
    JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
    LEFT JOIN customers c ON c.organization_id = r.organization_id AND c.id = r.customer_id
    LEFT JOIN money m ON m.reservation_id = r.id
    LEFT JOIN refund_state rs ON rs.reservation_id = r.id
    WHERE r.organization_id = sqlc.arg(organization_id)
      AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
      AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
      AND (
        r.payment_status = 'refund_pending' OR rs.reservation_id IS NOT NULL
        OR (r.status = 'completed' AND COALESCE(m.charged, 0) - COALESCE(m.collected, 0) + COALESCE(m.refunded, 0) > 0)
      )
      AND (
        sqlc.narg(q)::text IS NULL
        OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
        OR r.number::text = sqlc.narg(q)::text
      )
)
SELECT id, uuid, number, status, payment_status, starts_at, currency, cancel_reason, updated_at, branch_uuid, branch_name,
    customer_uuid, customer_first_name, customer_last_name, charged, collected, refunded, open_refunds,
    last_refund_status, last_refund_failure, last_refund_at, late, kind,
    (CASE WHEN kind = 'unpaid' THEN charged - collected + refunded ELSE collected - refunded END)::numeric AS amount
FROM base
WHERE kind <> '' AND (sqlc.narg(kinds)::text[] IS NULL OR kind = ANY(sqlc.narg(kinds)::text[]))
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND NOT sqlc.arg(sort_desc)::boolean
    THEN (CASE WHEN kind = 'unpaid' THEN charged - collected + refunded ELSE collected - refunded END) END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND sqlc.arg(sort_desc)::boolean
    THEN (CASE WHEN kind = 'unpaid' THEN charged - collected + refunded ELSE collected - refunded END) END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'number' AND NOT sqlc.arg(sort_desc)::boolean THEN number END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'number' AND sqlc.arg(sort_desc)::boolean THEN number END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'starts_at' AND NOT sqlc.arg(sort_desc)::boolean THEN starts_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN updated_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN updated_at END DESC,
  starts_at DESC, id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountPaymentFollowUps :many
-- Matching visits per kind (the list total and the kind facet).
WITH money AS (
    SELECT e.reservation_id,
        COALESCE(SUM(CASE WHEN COALESCE(o.kind, e.kind) = 'sale'
            THEN CASE WHEN e.direction = 'debit' THEN e.amount ELSE -e.amount END END), 0)::numeric AS charged,
        COALESCE(SUM(CASE WHEN COALESCE(o.kind, e.kind) = 'collection'
            THEN CASE WHEN e.direction = 'credit' THEN e.amount ELSE -e.amount END END), 0)::numeric AS collected,
        COALESCE(SUM(CASE WHEN COALESCE(o.kind, e.kind) = 'refund'
            THEN CASE WHEN e.direction = 'debit' THEN e.amount ELSE -e.amount END END), 0)::numeric AS refunded
    FROM ledger_entries e
    LEFT JOIN ledger_entries o ON o.organization_id = e.organization_id AND o.id = e.reverses_entry_id
    WHERE e.organization_id = sqlc.arg(organization_id) AND e.reservation_id IS NOT NULL
    GROUP BY e.reservation_id
), refund_state AS (
    SELECT f.reservation_id,
        COUNT(*) FILTER (WHERE f.status = 'requested')::bigint AS open_count,
        (array_agg(f.status ORDER BY f.created_at DESC, f.id DESC))[1]::text AS last_status
    FROM refunds f
    WHERE f.organization_id = sqlc.arg(organization_id)
    GROUP BY f.reservation_id
), base AS (
    SELECT (CASE
            WHEN COALESCE(rs.open_count, 0) > 0 THEN 'refund_unverified'
            WHEN rs.last_status = 'failed' AND COALESCE(m.collected, 0) - COALESCE(m.refunded, 0) > 0 THEN 'refund_failed'
            WHEN r.payment_status = 'refund_pending' THEN 'refund_pending'
            WHEN r.status = 'completed'
                AND COALESCE(m.charged, 0) - COALESCE(m.collected, 0) + COALESCE(m.refunded, 0) > 0 THEN 'unpaid'
            ELSE ''
        END)::text AS kind
    FROM reservations r
    LEFT JOIN customers c ON c.organization_id = r.organization_id AND c.id = r.customer_id
    LEFT JOIN money m ON m.reservation_id = r.id
    LEFT JOIN refund_state rs ON rs.reservation_id = r.id
    WHERE r.organization_id = sqlc.arg(organization_id)
      AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
      AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
      AND (
        r.payment_status = 'refund_pending' OR rs.reservation_id IS NOT NULL
        OR (r.status = 'completed' AND COALESCE(m.charged, 0) - COALESCE(m.collected, 0) + COALESCE(m.refunded, 0) > 0)
      )
      AND (
        sqlc.narg(q)::text IS NULL
        OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
        OR r.number::text = sqlc.narg(q)::text
      )
)
SELECT kind, COUNT(*)::bigint AS count FROM base
WHERE kind <> ''
GROUP BY kind
ORDER BY kind;
