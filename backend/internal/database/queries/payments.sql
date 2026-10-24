-- Online payments (TEC-912, docs/design/payments.md).

-- name: InsertPayment :one
INSERT INTO payments (
    organization_id, branch_id, reservation_id, integration_account_id, cash_account_id, provider, order_ref,
    amount, currency, expires_at
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(branch_id), sqlc.arg(reservation_id), sqlc.narg(integration_account_id),
    sqlc.arg(cash_account_id), sqlc.arg(provider), sqlc.arg(order_ref), sqlc.arg(amount), sqlc.arg(currency),
    sqlc.arg(expires_at)
)
RETURNING *;

-- name: SetPaymentCheckout :exec
UPDATE payments SET page_url = sqlc.arg(page_url), provider_token = sqlc.arg(provider_token)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: FindOpenPayment :one
-- The latest attempt of a reservation whose page is still open.
SELECT * FROM payments
WHERE organization_id = sqlc.arg(organization_id) AND reservation_id = sqlc.arg(reservation_id)
  AND status = 'pending' AND page_url <> '' AND expires_at > sqlc.arg(now)::TIMESTAMPTZ
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: GetPaymentByOrderRef :one
-- A notification names only our reference; the row gives its business
-- (organization_id) and the account whose secret verifies it.
SELECT p.organization_id, p.id, p.uuid, p.provider, p.integration_account_id, p.status, p.amount, p.currency,
    r.uuid AS reservation_uuid
FROM payments p
JOIN reservations r ON r.organization_id = p.organization_id AND r.id = p.reservation_id
WHERE p.order_ref = sqlc.arg(order_ref);

-- name: LockPayment :one
SELECT p.*, a.uuid AS cash_account_uuid
FROM payments p
JOIN cash_accounts a ON a.organization_id = p.organization_id AND a.id = p.cash_account_id
WHERE p.organization_id = sqlc.arg(organization_id) AND a.organization_id = sqlc.arg(organization_id)
  AND p.id = sqlc.arg(id)
FOR UPDATE OF p;

-- name: SucceedPayment :execrows
-- One winner: only an attempt not succeeded yet moves.
UPDATE payments
SET status = 'succeeded', paid_amount = sqlc.arg(paid_amount), provider_payment_id = sqlc.arg(provider_payment_id),
    outcome = sqlc.arg(outcome), ledger_entry_id = sqlc.arg(ledger_entry_id), failure_reason = '',
    completed_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status <> 'succeeded';

-- name: FailPayment :execrows
UPDATE payments
SET status = 'failed', failure_reason = sqlc.arg(failure_reason), completed_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'pending';

-- name: ListReservationPayments :many
SELECT p.uuid, p.provider, p.amount, p.paid_amount, p.currency, p.status, p.outcome, p.failure_reason,
    p.expires_at, p.completed_at, p.created_at, a.uuid AS cash_account_uuid, a.name AS cash_account_name
FROM payments p
JOIN cash_accounts a ON a.organization_id = p.organization_id AND a.id = p.cash_account_id
WHERE p.organization_id = sqlc.arg(organization_id) AND a.organization_id = sqlc.arg(organization_id)
  AND p.reservation_id = sqlc.arg(reservation_id)
ORDER BY p.created_at DESC, p.id DESC
LIMIT 50;

-- name: GetTestPaymentPage :one
-- What the development provider's test page shows.
SELECT p.organization_id, p.uuid, p.provider, p.integration_account_id, p.amount, p.currency, p.status, p.expires_at,
    o.slug AS organization_slug, o.name AS organization_name, r.uuid AS reservation_uuid, r.number AS reservation_number
FROM payments p
JOIN organizations o ON o.id = p.organization_id
JOIN reservations r ON r.organization_id = p.organization_id AND r.id = p.reservation_id
WHERE p.order_ref = sqlc.arg(order_ref) AND r.organization_id = p.organization_id;

-- name: FindOnlineCashAccount :one
-- The receivable account of the provider for a branch: the branch's own
-- online account before a business-wide one, an account naming the
-- provider before one naming none.
SELECT id, uuid, branch_id, currency FROM cash_accounts
WHERE organization_id = sqlc.arg(organization_id) AND kind = 'online' AND status = 'active'
  AND currency = sqlc.arg(currency)
  AND (branch_id = sqlc.arg(branch_id) OR branch_id IS NULL)
  AND (provider = sqlc.arg(provider) OR provider = '')
ORDER BY (branch_id IS NULL), (provider = ''), sort_order, id
LIMIT 1;

-- name: ListCollectAccounts :many
-- Tills and bank accounts a collection at the business may go into: the
-- branch's own and the business-wide ones of the currency.
SELECT a.uuid, a.name, a.kind, a.currency, a.branch_id FROM cash_accounts a
WHERE a.organization_id = sqlc.arg(organization_id) AND a.kind IN ('cash', 'bank') AND a.status = 'active'
  AND a.currency = sqlc.arg(currency) AND (a.branch_id = sqlc.arg(branch_id) OR a.branch_id IS NULL)
ORDER BY (a.branch_id IS NULL), a.kind, a.sort_order, a.name;

-- name: GetCollectAccount :one
SELECT id, uuid, branch_id, kind, status, currency FROM cash_accounts
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: CountOnlineCashAccounts :one
-- Online (provider receivable) accounts that can take the provider's money.
SELECT COUNT(*) FROM cash_accounts
WHERE organization_id = sqlc.arg(organization_id) AND kind = 'online' AND status = 'active'
  AND (provider = sqlc.arg(provider) OR provider = '');
