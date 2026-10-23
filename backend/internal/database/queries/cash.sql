-- Cash and bank accounts, movements and daily closings (TEC-914). Every
-- query filters by organization_id. Branch scope: accounts of a branch
-- outside the caller's scope, and business-wide accounts (branch_id NULL)
-- for a branch-limited member, are left out.

-- name: CreateCashAccount :one
INSERT INTO cash_accounts (
    organization_id, branch_id, kind, name, currency, opening_balance, opening_date,
    bank_name, iban, provider, note, sort_order, created_by
)
VALUES (
    sqlc.arg(organization_id), sqlc.narg(branch_id), sqlc.arg(kind), sqlc.arg(name), sqlc.arg(currency),
    sqlc.arg(opening_balance), sqlc.arg(opening_date), sqlc.arg(bank_name), sqlc.arg(iban),
    sqlc.arg(provider), sqlc.arg(note), sqlc.arg(sort_order), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: UpdateCashAccount :execrows
UPDATE cash_accounts
SET name = sqlc.arg(name), opening_balance = sqlc.arg(opening_balance), opening_date = sqlc.arg(opening_date),
    bank_name = sqlc.arg(bank_name), iban = sqlc.arg(iban), provider = sqlc.arg(provider),
    note = sqlc.arg(note), sort_order = sqlc.arg(sort_order)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: SetCashAccountStatus :exec
UPDATE cash_accounts SET status = sqlc.arg(status)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: GetCashAccount :one
SELECT
    a.id, a.uuid, a.organization_id, a.branch_id, a.kind, a.name, a.currency, a.opening_balance,
    a.opening_date, a.bank_name, a.iban, a.provider, a.note, a.status, a.sort_order, a.created_at, a.updated_at,
    b.uuid AS branch_uuid, b.name AS branch_name, b.code AS branch_code,
    (a.opening_balance + COALESCE((
        SELECT SUM(CASE WHEN m.direction = 'in' THEN m.amount ELSE -m.amount END)
        FROM cash_movements m
        WHERE m.organization_id = a.organization_id AND m.cash_account_id = a.id
    ), 0))::numeric AS balance,
    (SELECT MAX(c.business_date) FROM cash_closings c
     WHERE c.organization_id = a.organization_id AND c.cash_account_id = a.id AND c.status = 'closed')::date AS last_closed_date
FROM cash_accounts a
LEFT JOIN branches b ON b.organization_id = a.organization_id AND b.id = a.branch_id
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.uuid = sqlc.arg(uuid);

-- name: LockCashAccounts :many
-- Locks the accounts in id order (concurrent transfers in opposite
-- directions cannot deadlock).
SELECT a.id, a.uuid, a.branch_id, a.kind, a.name, a.currency, a.opening_balance, a.opening_date, a.status
FROM cash_accounts a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.id = ANY(sqlc.arg(ids)::bigint[])
ORDER BY a.id
FOR UPDATE;

-- name: ListCashAccounts :many
SELECT
    a.id, a.uuid, a.organization_id, a.branch_id, a.kind, a.name, a.currency, a.opening_balance,
    a.opening_date, a.bank_name, a.iban, a.provider, a.note, a.status, a.sort_order, a.created_at, a.updated_at,
    b.uuid AS branch_uuid, b.name AS branch_name, b.code AS branch_code,
    bal.balance::numeric AS balance,
    (SELECT MAX(c.business_date) FROM cash_closings c
     WHERE c.organization_id = a.organization_id AND c.cash_account_id = a.id AND c.status = 'closed')::date AS last_closed_date
FROM cash_accounts a
LEFT JOIN branches b ON b.organization_id = a.organization_id AND b.id = a.branch_id
CROSS JOIN LATERAL (
    SELECT a.opening_balance + COALESCE(SUM(CASE WHEN m.direction = 'in' THEN m.amount ELSE -m.amount END), 0) AS balance
    FROM cash_movements m
    WHERE m.organization_id = a.organization_id AND m.cash_account_id = a.id
) bal
WHERE a.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR a.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (
    (sqlc.narg(filter_branch_ids)::bigint[] IS NULL AND NOT sqlc.arg(filter_head_office)::boolean)
    OR a.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])
    OR (sqlc.arg(filter_head_office)::boolean AND a.branch_id IS NULL)
  )
  AND (sqlc.narg(kinds)::text[] IS NULL OR a.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR a.status = ANY(sqlc.narg(statuses)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR a.name ILIKE '%' || sqlc.narg(q) || '%'
    OR a.bank_name ILIKE '%' || sqlc.narg(q) || '%'
    OR a.iban ILIKE '%' || sqlc.narg(q) || '%'
    OR b.name ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN a.name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN a.name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'kind' AND NOT sqlc.arg(sort_desc)::boolean THEN a.kind END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'kind' AND sqlc.arg(sort_desc)::boolean THEN a.kind END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'balance' AND NOT sqlc.arg(sort_desc)::boolean THEN bal.balance END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'balance' AND sqlc.arg(sort_desc)::boolean THEN bal.balance END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND NOT sqlc.arg(sort_desc)::boolean THEN a.status END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND sqlc.arg(sort_desc)::boolean THEN a.status END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'sort_order' AND NOT sqlc.arg(sort_desc)::boolean THEN a.sort_order END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'sort_order' AND sqlc.arg(sort_desc)::boolean THEN a.sort_order END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN a.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN a.created_at END DESC,
  a.status ASC, b.name ASC NULLS FIRST, a.sort_order ASC, a.name ASC, a.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountCashAccounts :one
SELECT COUNT(*)::bigint
FROM cash_accounts a
LEFT JOIN branches b ON b.organization_id = a.organization_id AND b.id = a.branch_id
WHERE a.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR a.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (
    (sqlc.narg(filter_branch_ids)::bigint[] IS NULL AND NOT sqlc.arg(filter_head_office)::boolean)
    OR a.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])
    OR (sqlc.arg(filter_head_office)::boolean AND a.branch_id IS NULL)
  )
  AND (sqlc.narg(kinds)::text[] IS NULL OR a.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR a.status = ANY(sqlc.narg(statuses)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR a.name ILIKE '%' || sqlc.narg(q) || '%'
    OR a.bank_name ILIKE '%' || sqlc.narg(q) || '%'
    OR a.iban ILIKE '%' || sqlc.narg(q) || '%'
    OR b.name ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListCashAccountUUIDs :many
SELECT a.uuid
FROM cash_accounts a
LEFT JOIN branches b ON b.organization_id = a.organization_id AND b.id = a.branch_id
WHERE a.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR a.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (
    (sqlc.narg(filter_branch_ids)::bigint[] IS NULL AND NOT sqlc.arg(filter_head_office)::boolean)
    OR a.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])
    OR (sqlc.arg(filter_head_office)::boolean AND a.branch_id IS NULL)
  )
  AND (sqlc.narg(kinds)::text[] IS NULL OR a.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR a.status = ANY(sqlc.narg(statuses)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR a.name ILIKE '%' || sqlc.narg(q) || '%'
    OR a.bank_name ILIKE '%' || sqlc.narg(q) || '%'
    OR a.iban ILIKE '%' || sqlc.narg(q) || '%'
    OR b.name ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY a.id ASC
LIMIT sqlc.arg(limit_count);

-- name: ListCashAccountBranchFacets :many
-- Branches with their account count; business-wide accounts have a NULL uuid.
SELECT b.uuid AS branch_uuid, b.name AS branch_name, COUNT(*)::bigint AS count
FROM cash_accounts a
LEFT JOIN branches b ON b.organization_id = a.organization_id AND b.id = a.branch_id
WHERE a.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR a.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY b.uuid, b.name
ORDER BY b.name ASC NULLS FIRST;

-- name: ListCashAccountOptions :many
-- Every account of the scope (pickers: transfer target, movement filter).
SELECT a.uuid, a.name, a.kind, a.status, a.currency, b.name AS branch_name
FROM cash_accounts a
LEFT JOIN branches b ON b.organization_id = a.organization_id AND b.id = a.branch_id
WHERE a.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR a.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
ORDER BY a.status ASC, b.name ASC NULLS FIRST, a.sort_order ASC, a.name ASC;

-- name: CashAccountNameTaken :one
SELECT EXISTS (
    SELECT 1 FROM cash_accounts a
    WHERE a.organization_id = sqlc.arg(organization_id)
      AND COALESCE(a.branch_id, 0) = COALESCE(sqlc.narg(branch_id)::bigint, 0)
      AND lower(a.name) = lower(sqlc.arg(name))
      AND (sqlc.narg(exclude_id)::bigint IS NULL OR a.id <> sqlc.narg(exclude_id)::bigint)
);

-- name: CashAccountHasActivity :one
SELECT EXISTS (
    SELECT 1 FROM cash_movements m
    WHERE m.organization_id = sqlc.arg(organization_id) AND m.cash_account_id = sqlc.arg(id)
) OR EXISTS (
    SELECT 1 FROM cash_closings c
    WHERE c.organization_id = sqlc.arg(organization_id) AND c.cash_account_id = sqlc.arg(id)
);

-- name: ListCashAccountSearchDocuments :many
SELECT a.id, a.uuid, a.organization_id, a.branch_id, a.kind, a.name, a.bank_name, a.status,
    b.name AS branch_name, o.slug AS organization_slug
FROM cash_accounts a
LEFT JOIN branches b ON b.organization_id = a.organization_id AND b.id = a.branch_id
JOIN organizations o ON o.id = a.organization_id
WHERE a.organization_id = sqlc.arg(organization_id);

-- name: GetCashAccountSearchDocument :one
SELECT a.id, a.uuid, a.organization_id, a.branch_id, a.kind, a.name, a.bank_name, a.status,
    b.name AS branch_name, o.slug AS organization_slug
FROM cash_accounts a
LEFT JOIN branches b ON b.organization_id = a.organization_id AND b.id = a.branch_id
JOIN organizations o ON o.id = a.organization_id
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.uuid = sqlc.arg(uuid);

-- ------------------------------------------------------------ movements

-- name: InsertCashMovement :one
INSERT INTO cash_movements (
    organization_id, cash_account_id, kind, direction, amount, currency, movement_date, description,
    branch_id, transfer_group, counter_account_id, reverses_movement_id, closing_id,
    source_type, source_uuid, idempotency_key, created_by
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(cash_account_id), sqlc.arg(kind), sqlc.arg(direction), sqlc.arg(amount),
    sqlc.arg(currency), sqlc.arg(movement_date), sqlc.arg(description), sqlc.narg(branch_id),
    sqlc.narg(transfer_group), sqlc.narg(counter_account_id), sqlc.narg(reverses_movement_id), sqlc.narg(closing_id),
    sqlc.arg(source_type), sqlc.narg(source_uuid), sqlc.narg(idempotency_key), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: GetCashMovement :one
SELECT
    m.id, m.uuid, m.cash_account_id, m.kind, m.direction, m.amount, m.currency, m.movement_date, m.description,
    m.branch_id, m.transfer_group, m.reverses_movement_id, m.source_type, m.source_uuid, m.created_at,
    a.uuid AS account_uuid, a.name AS account_name, a.kind AS account_kind,
    ca.uuid AS counter_account_uuid, ca.name AS counter_account_name,
    rv.uuid AS reverses_uuid, rb.uuid AS reversed_by_uuid,
    cl.uuid AS closing_uuid,
    u.uuid AS creator_uuid, u.name AS creator_name, u.surname AS creator_surname
FROM cash_movements m
JOIN cash_accounts a ON a.organization_id = m.organization_id AND a.id = m.cash_account_id
LEFT JOIN cash_accounts ca ON ca.organization_id = m.organization_id AND ca.id = m.counter_account_id
LEFT JOIN cash_movements rv ON rv.organization_id = m.organization_id AND rv.id = m.reverses_movement_id
LEFT JOIN cash_movements rb ON rb.organization_id = m.organization_id AND rb.reverses_movement_id = m.id
LEFT JOIN cash_closings cl ON cl.organization_id = m.organization_id AND cl.id = m.closing_id
LEFT JOIN users u ON u.id = m.created_by
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.uuid = sqlc.arg(uuid);

-- name: GetCashMovementByIdempotencyKey :one
SELECT m.uuid, m.cash_account_id, m.kind, m.direction, m.amount
FROM cash_movements m
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.idempotency_key = sqlc.arg(idempotency_key);

-- name: ListCashMovements :many
SELECT
    m.id, m.uuid, m.cash_account_id, m.kind, m.direction, m.amount, m.currency, m.movement_date, m.description,
    m.branch_id, m.transfer_group, m.reverses_movement_id, m.source_type, m.source_uuid, m.created_at,
    a.uuid AS account_uuid, a.name AS account_name, a.kind AS account_kind,
    ca.uuid AS counter_account_uuid, ca.name AS counter_account_name,
    rv.uuid AS reverses_uuid, rb.uuid AS reversed_by_uuid,
    cl.uuid AS closing_uuid,
    u.uuid AS creator_uuid, u.name AS creator_name, u.surname AS creator_surname
FROM cash_movements m
JOIN cash_accounts a ON a.organization_id = m.organization_id AND a.id = m.cash_account_id
LEFT JOIN cash_accounts ca ON ca.organization_id = m.organization_id AND ca.id = m.counter_account_id
LEFT JOIN cash_movements rv ON rv.organization_id = m.organization_id AND rv.id = m.reverses_movement_id
LEFT JOIN cash_movements rb ON rb.organization_id = m.organization_id AND rb.reverses_movement_id = m.id
LEFT JOIN cash_closings cl ON cl.organization_id = m.organization_id AND cl.id = m.closing_id
LEFT JOIN users u ON u.id = m.created_by
WHERE m.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR m.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(account_ids)::bigint[] IS NULL OR m.cash_account_id = ANY(sqlc.narg(account_ids)::bigint[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR m.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(direction)::text IS NULL OR m.direction = sqlc.narg(direction)::text)
  AND (sqlc.narg(date_from)::date IS NULL OR m.movement_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR m.movement_date <= sqlc.narg(date_to)::date)
  AND (
    sqlc.narg(q)::text IS NULL
    OR m.description ILIKE '%' || sqlc.narg(q) || '%'
    OR a.name ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND NOT sqlc.arg(sort_desc)::boolean THEN m.amount END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND sqlc.arg(sort_desc)::boolean THEN m.amount END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'kind' AND NOT sqlc.arg(sort_desc)::boolean THEN m.kind END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'kind' AND sqlc.arg(sort_desc)::boolean THEN m.kind END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'movement_date' AND NOT sqlc.arg(sort_desc)::boolean THEN m.movement_date END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN m.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN m.created_at END DESC,
  m.movement_date DESC, m.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountCashMovements :one
SELECT COUNT(*)::bigint
FROM cash_movements m
JOIN cash_accounts a ON a.organization_id = m.organization_id AND a.id = m.cash_account_id
WHERE m.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR m.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(account_ids)::bigint[] IS NULL OR m.cash_account_id = ANY(sqlc.narg(account_ids)::bigint[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR m.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(direction)::text IS NULL OR m.direction = sqlc.narg(direction)::text)
  AND (sqlc.narg(date_from)::date IS NULL OR m.movement_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR m.movement_date <= sqlc.narg(date_to)::date)
  AND (
    sqlc.narg(q)::text IS NULL
    OR m.description ILIKE '%' || sqlc.narg(q) || '%'
    OR a.name ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListCashAccountMovements :many
-- The movements of one account with the balance after each one (book
-- order: date, then entry order), newest first.
SELECT * FROM (
    SELECT
        m.id, m.uuid, m.kind, m.direction, m.amount, m.currency, m.movement_date, m.description,
        m.transfer_group, m.source_type, m.source_uuid, m.created_at,
        ca.uuid AS counter_account_uuid, ca.name AS counter_account_name,
        rv.uuid AS reverses_uuid, rb.uuid AS reversed_by_uuid, cl.uuid AS closing_uuid,
        u.uuid AS creator_uuid, u.name AS creator_name, u.surname AS creator_surname,
        (SUM(CASE WHEN m.direction = 'in' THEN m.amount ELSE -m.amount END)
            OVER (ORDER BY m.movement_date, m.id))::numeric AS running_total
    FROM cash_movements m
    LEFT JOIN cash_accounts ca ON ca.organization_id = m.organization_id AND ca.id = m.counter_account_id
    LEFT JOIN cash_movements rv ON rv.organization_id = m.organization_id AND rv.id = m.reverses_movement_id
    LEFT JOIN cash_movements rb ON rb.organization_id = m.organization_id AND rb.reverses_movement_id = m.id
    LEFT JOIN cash_closings cl ON cl.organization_id = m.organization_id AND cl.id = m.closing_id
    LEFT JOIN users u ON u.id = m.created_by
    WHERE m.organization_id = sqlc.arg(organization_id)
      AND m.cash_account_id = sqlc.arg(cash_account_id)
) t
WHERE (sqlc.narg(date_from)::date IS NULL OR t.movement_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR t.movement_date <= sqlc.narg(date_to)::date)
  AND (sqlc.narg(kinds)::text[] IS NULL OR t.kind = ANY(sqlc.narg(kinds)::text[]))
ORDER BY t.movement_date DESC, t.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountCashAccountMovements :one
SELECT COUNT(*)::bigint
FROM cash_movements m
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.cash_account_id = sqlc.arg(cash_account_id)
  AND (sqlc.narg(date_from)::date IS NULL OR m.movement_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR m.movement_date <= sqlc.narg(date_to)::date)
  AND (sqlc.narg(kinds)::text[] IS NULL OR m.kind = ANY(sqlc.narg(kinds)::text[]));

-- name: ListCashMovementKindFacets :many
SELECT m.kind AS value, COUNT(*)::bigint AS count
FROM cash_movements m
WHERE m.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR m.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY m.kind
ORDER BY m.kind ASC;

-- name: CashAccountDayTotals :one
-- Balance before the day and the day's money in / out of one account.
-- Closing differences of the day (and their reversals after a reopen) are
-- left out of the day's totals: they follow a count, not a payment.
SELECT
    COALESCE(SUM(CASE WHEN m.movement_date < sqlc.arg(day)::date
        THEN CASE WHEN m.direction = 'in' THEN m.amount ELSE -m.amount END END), 0)::numeric AS before_day,
    COALESCE(SUM(CASE WHEN m.movement_date = sqlc.arg(day)::date AND m.direction = 'in'
        AND m.kind <> 'closing_difference' AND COALESCE(rv.kind, '') <> 'closing_difference'
        THEN m.amount END), 0)::numeric AS day_in,
    COALESCE(SUM(CASE WHEN m.movement_date = sqlc.arg(day)::date AND m.direction = 'out'
        AND m.kind <> 'closing_difference' AND COALESCE(rv.kind, '') <> 'closing_difference'
        THEN m.amount END), 0)::numeric AS day_out,
    COALESCE(SUM(CASE WHEN m.movement_date = sqlc.arg(day)::date
        AND (m.kind = 'closing_difference' OR COALESCE(rv.kind, '') = 'closing_difference')
        THEN CASE WHEN m.direction = 'in' THEN m.amount ELSE -m.amount END END), 0)::numeric AS day_adjustment,
    COUNT(*) FILTER (WHERE m.movement_date = sqlc.arg(day)::date
        AND m.kind <> 'closing_difference' AND COALESCE(rv.kind, '') <> 'closing_difference')::bigint AS day_count
FROM cash_movements m
LEFT JOIN cash_movements rv ON rv.organization_id = m.organization_id AND rv.id = m.reverses_movement_id
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.cash_account_id = sqlc.arg(cash_account_id)
  AND m.movement_date <= sqlc.arg(day)::date;

-- name: CashDailySummary :many
-- One row per account of the scope for a day: balance before the day,
-- money in / out split into transfers, closing differences and the rest,
-- and the closing of the day.
SELECT
    a.id, a.uuid, a.name, a.kind, a.currency, a.status, a.opening_balance, a.opening_date,
    b.uuid AS branch_uuid, b.name AS branch_name,
    COALESCE(SUM(CASE WHEN m.movement_date < sqlc.arg(day)::date
        THEN CASE WHEN m.direction = 'in' THEN m.amount ELSE -m.amount END END), 0)::numeric AS before_day,
    COALESCE(SUM(CASE WHEN m.movement_date = sqlc.arg(day)::date AND m.direction = 'in' AND NOT m.adj
        AND m.kind <> 'transfer_in' THEN m.amount END), 0)::numeric AS day_in,
    COALESCE(SUM(CASE WHEN m.movement_date = sqlc.arg(day)::date AND m.direction = 'out' AND NOT m.adj
        AND m.kind <> 'transfer_out' THEN m.amount END), 0)::numeric AS day_out,
    COALESCE(SUM(CASE WHEN m.movement_date = sqlc.arg(day)::date AND m.kind = 'transfer_in' THEN m.amount END), 0)::numeric AS transfer_in,
    COALESCE(SUM(CASE WHEN m.movement_date = sqlc.arg(day)::date AND m.kind = 'transfer_out' THEN m.amount END), 0)::numeric AS transfer_out,
    COALESCE(SUM(CASE WHEN m.movement_date = sqlc.arg(day)::date AND m.adj
        THEN CASE WHEN m.direction = 'in' THEN m.amount ELSE -m.amount END END), 0)::numeric AS difference,
    cl.uuid AS closing_uuid, cl.counted_amount AS closing_counted, cl.closed_at AS closing_closed_at
FROM cash_accounts a
LEFT JOIN branches b ON b.organization_id = a.organization_id AND b.id = a.branch_id
LEFT JOIN (
    SELECT mm.organization_id, mm.cash_account_id, mm.movement_date, mm.direction, mm.kind, mm.amount,
        (mm.kind = 'closing_difference' OR COALESCE(rv.kind, '') = 'closing_difference') AS adj
    FROM cash_movements mm
    LEFT JOIN cash_movements rv ON rv.organization_id = mm.organization_id AND rv.id = mm.reverses_movement_id
    WHERE mm.organization_id = sqlc.arg(organization_id)
      AND mm.movement_date <= sqlc.arg(day)::date
) m ON m.organization_id = a.organization_id AND m.cash_account_id = a.id
LEFT JOIN cash_closings cl ON cl.organization_id = a.organization_id AND cl.cash_account_id = a.id
    AND cl.business_date = sqlc.arg(day)::date AND cl.status = 'closed'
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.opening_date <= sqlc.arg(day)::date
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR a.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (
    (sqlc.narg(filter_branch_ids)::bigint[] IS NULL AND NOT sqlc.arg(filter_head_office)::boolean)
    OR a.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])
    OR (sqlc.arg(filter_head_office)::boolean AND a.branch_id IS NULL)
  )
GROUP BY a.id, b.uuid, b.name, cl.uuid, cl.counted_amount, cl.closed_at
ORDER BY b.name ASC NULLS FIRST, a.sort_order ASC, a.name ASC;

-- ------------------------------------------------------------- closings

-- name: InsertCashClosing :one
INSERT INTO cash_closings (
    organization_id, cash_account_id, branch_id, business_date, currency, opening_amount, total_in, total_out,
    expected_amount, counted_amount, difference, note, closed_by
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(cash_account_id), sqlc.narg(branch_id), sqlc.arg(business_date),
    sqlc.arg(currency), sqlc.arg(opening_amount), sqlc.arg(total_in), sqlc.arg(total_out),
    sqlc.arg(expected_amount), sqlc.arg(counted_amount), sqlc.arg(difference), sqlc.arg(note), sqlc.narg(closed_by)
)
RETURNING id, uuid;

-- name: GetCashClosing :one
SELECT
    c.id, c.uuid, c.cash_account_id, c.business_date, c.currency, c.opening_amount, c.total_in, c.total_out,
    c.expected_amount, c.counted_amount, c.difference, c.note, c.status, c.closed_at, c.reopened_at,
    c.reopen_reason,
    cu.uuid AS closed_by_uuid, cu.name AS closed_by_name, cu.surname AS closed_by_surname,
    ru.uuid AS reopened_by_uuid, ru.name AS reopened_by_name, ru.surname AS reopened_by_surname,
    dm.id AS difference_movement_id
FROM cash_closings c
LEFT JOIN users cu ON cu.id = c.closed_by
LEFT JOIN users ru ON ru.id = c.reopened_by
LEFT JOIN cash_movements dm ON dm.organization_id = c.organization_id AND dm.closing_id = c.id
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.cash_account_id = sqlc.arg(cash_account_id)
  AND c.uuid = sqlc.arg(uuid);

-- name: LatestCashClosingDate :one
SELECT MAX(c.business_date)::date
FROM cash_closings c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.cash_account_id = sqlc.arg(cash_account_id)
  AND c.status = 'closed';

-- name: ListCashClosings :many
SELECT
    c.id, c.uuid, c.cash_account_id, c.business_date, c.currency, c.opening_amount, c.total_in, c.total_out,
    c.expected_amount, c.counted_amount, c.difference, c.note, c.status, c.closed_at, c.reopened_at,
    c.reopen_reason,
    cu.uuid AS closed_by_uuid, cu.name AS closed_by_name, cu.surname AS closed_by_surname,
    ru.uuid AS reopened_by_uuid, ru.name AS reopened_by_name, ru.surname AS reopened_by_surname,
    dm.id AS difference_movement_id
FROM cash_closings c
LEFT JOIN users cu ON cu.id = c.closed_by
LEFT JOIN users ru ON ru.id = c.reopened_by
LEFT JOIN cash_movements dm ON dm.organization_id = c.organization_id AND dm.closing_id = c.id
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.cash_account_id = sqlc.arg(cash_account_id)
ORDER BY c.business_date DESC, c.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountCashClosings :one
SELECT COUNT(*)::bigint
FROM cash_closings c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.cash_account_id = sqlc.arg(cash_account_id);

-- name: ReopenCashClosing :execrows
UPDATE cash_closings
SET status = 'reopened', reopened_at = NOW(), reopened_by = sqlc.narg(reopened_by), reopen_reason = sqlc.arg(reopen_reason)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'closed';

-- name: ListCashTransferLegs :many
SELECT m.id, m.uuid, m.cash_account_id, m.kind, m.direction, m.amount, m.currency
FROM cash_movements m
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.transfer_group = sqlc.arg(transfer_group)
ORDER BY m.id;

-- name: GetCashAccountByID :one
SELECT a.id, a.uuid, a.branch_id, a.name
FROM cash_accounts a
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.id = sqlc.arg(id);
