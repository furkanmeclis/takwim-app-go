-- Bank statements / provider settlement reports and their reconciliation
-- lines (TEC-965). Every query filters by organization_id; the branch scope
-- is applied through the statement's branch_id like the cash tables.

-- name: InsertCashStatement :one
INSERT INTO cash_statements (
    organization_id, cash_account_id, branch_id, source, name, period_from, period_to, closing_balance, note, created_by
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(cash_account_id), sqlc.narg(branch_id), sqlc.arg(source), sqlc.arg(name),
    sqlc.narg(period_from), sqlc.narg(period_to), sqlc.narg(closing_balance), sqlc.arg(note), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: UpdateCashStatement :execrows
UPDATE cash_statements
SET name = sqlc.arg(name), period_from = sqlc.narg(period_from), period_to = sqlc.narg(period_to),
    closing_balance = sqlc.narg(closing_balance), note = sqlc.arg(note)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: DeleteCashStatement :execrows
DELETE FROM cash_statements
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: GetCashStatement :one
SELECT
    s.id, s.uuid, s.cash_account_id, s.branch_id, s.source, s.name, s.period_from, s.period_to, s.closing_balance,
    s.note, s.status, s.created_at, s.updated_at,
    a.uuid AS account_uuid, a.name AS account_name, a.kind AS account_kind, a.currency AS account_currency,
    a.opening_balance AS account_opening_balance,
    b.uuid AS branch_uuid, b.name AS branch_name,
    cl.uuid AS closing_uuid,
    u.uuid AS creator_uuid, u.name AS creator_name, u.surname AS creator_surname,
    (SELECT COUNT(*) FROM cash_statement_lines l WHERE l.organization_id = s.organization_id AND l.statement_id = s.id)::bigint AS line_count,
    (SELECT COUNT(*) FROM cash_statement_lines l WHERE l.organization_id = s.organization_id AND l.statement_id = s.id
        AND l.status IN ('pending', 'difference'))::bigint AS open_count
FROM cash_statements s
JOIN cash_accounts a ON a.organization_id = s.organization_id AND a.id = s.cash_account_id
LEFT JOIN branches b ON b.organization_id = s.organization_id AND b.id = s.branch_id
LEFT JOIN cash_closings cl ON cl.organization_id = s.organization_id AND cl.id = s.closing_id
LEFT JOIN users u ON u.id = s.created_by
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.uuid = sqlc.arg(uuid);

-- name: ListCashStatements :many
SELECT
    s.id, s.uuid, s.cash_account_id, s.branch_id, s.source, s.name, s.period_from, s.period_to, s.closing_balance,
    s.note, s.status, s.created_at, s.updated_at,
    a.uuid AS account_uuid, a.name AS account_name, a.kind AS account_kind, a.currency AS account_currency,
    a.opening_balance AS account_opening_balance,
    b.uuid AS branch_uuid, b.name AS branch_name,
    cl.uuid AS closing_uuid,
    u.uuid AS creator_uuid, u.name AS creator_name, u.surname AS creator_surname,
    (SELECT COUNT(*) FROM cash_statement_lines l WHERE l.organization_id = s.organization_id AND l.statement_id = s.id)::bigint AS line_count,
    (SELECT COUNT(*) FROM cash_statement_lines l WHERE l.organization_id = s.organization_id AND l.statement_id = s.id
        AND l.status IN ('pending', 'difference'))::bigint AS open_count
FROM cash_statements s
JOIN cash_accounts a ON a.organization_id = s.organization_id AND a.id = s.cash_account_id
LEFT JOIN branches b ON b.organization_id = s.organization_id AND b.id = s.branch_id
LEFT JOIN cash_closings cl ON cl.organization_id = s.organization_id AND cl.id = s.closing_id
LEFT JOIN users u ON u.id = s.created_by
WHERE s.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR s.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(account_ids)::bigint[] IS NULL OR s.cash_account_id = ANY(sqlc.narg(account_ids)::bigint[]))
  AND (sqlc.narg(sources)::text[] IS NULL OR s.source = ANY(sqlc.narg(sources)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR s.status = ANY(sqlc.narg(statuses)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR s.name ILIKE '%' || sqlc.narg(q) || '%'
    OR a.name ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN s.name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN s.name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'period_to' AND NOT sqlc.arg(sort_desc)::boolean THEN s.period_to END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'period_to' AND sqlc.arg(sort_desc)::boolean THEN s.period_to END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN s.created_at END ASC,
  s.created_at DESC, s.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountCashStatements :one
SELECT COUNT(*)::bigint
FROM cash_statements s
JOIN cash_accounts a ON a.organization_id = s.organization_id AND a.id = s.cash_account_id
WHERE s.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR s.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(account_ids)::bigint[] IS NULL OR s.cash_account_id = ANY(sqlc.narg(account_ids)::bigint[]))
  AND (sqlc.narg(sources)::text[] IS NULL OR s.source = ANY(sqlc.narg(sources)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR s.status = ANY(sqlc.narg(statuses)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR s.name ILIKE '%' || sqlc.narg(q) || '%'
    OR a.name ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: CashStatementSummary :one
-- Line counts and sums per reconciliation status of one statement.
SELECT
    COUNT(*) FILTER (WHERE l.status = 'pending')::bigint AS pending_count,
    COUNT(*) FILTER (WHERE l.status = 'matched')::bigint AS matched_count,
    COUNT(*) FILTER (WHERE l.status = 'difference')::bigint AS difference_count,
    COUNT(*) FILTER (WHERE l.status = 'posted')::bigint AS posted_count,
    COUNT(*) FILTER (WHERE l.status = 'ignored')::bigint AS ignored_count,
    COALESCE(SUM(CASE WHEN l.direction = 'in' THEN l.amount END), 0)::numeric AS total_in,
    COALESCE(SUM(CASE WHEN l.direction = 'out' THEN l.amount END), 0)::numeric AS total_out,
    COALESCE(SUM(l.fee), 0)::numeric AS total_fee,
    COALESCE(SUM(CASE WHEN l.status = 'pending' THEN CASE WHEN l.direction = 'in' THEN l.amount ELSE -l.amount END END), 0)::numeric AS pending_net,
    COALESCE(SUM(CASE WHEN l.status = 'difference' THEN l.difference_amount END), 0)::numeric AS difference_total,
    MIN(l.line_date)::date AS first_date,
    MAX(l.line_date)::date AS last_date
FROM cash_statement_lines l
WHERE l.organization_id = sqlc.arg(organization_id)
  AND l.statement_id = sqlc.arg(statement_id);

-- name: CashAccountBalanceAt :one
-- Book balance of an account at the end of a day (opening + movements
-- dated on or before it).
SELECT (a.opening_balance + COALESCE((
    SELECT SUM(CASE WHEN m.direction = 'in' THEN m.amount ELSE -m.amount END)
    FROM cash_movements m
    WHERE m.organization_id = a.organization_id AND m.cash_account_id = a.id AND m.movement_date <= sqlc.arg(day)::date
), 0))::numeric AS balance
FROM cash_accounts a
WHERE a.organization_id = sqlc.arg(organization_id) AND a.id = sqlc.arg(id);

-- name: SetCashStatementClosing :exec
-- Closes the open statements of an account whose lines are all resolved
-- and dated on or before the closed day.
UPDATE cash_statements s
SET status = 'closed', closing_id = sqlc.arg(closing_id)
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.cash_account_id = sqlc.arg(cash_account_id)
  AND s.status = 'open'
  AND NOT EXISTS (
    SELECT 1 FROM cash_statement_lines l
    WHERE l.organization_id = s.organization_id AND l.statement_id = s.id
      AND (l.status IN ('pending', 'difference') OR l.line_date > sqlc.arg(day)::date)
  );

-- name: ReopenCashStatementsOfClosing :exec
UPDATE cash_statements
SET status = 'open', closing_id = NULL
WHERE organization_id = sqlc.arg(organization_id) AND closing_id = sqlc.arg(closing_id);

-- name: CountUnreconciledCashStatementLines :one
-- Pending / difference lines of an account dated on or before a day.
SELECT COUNT(*)::bigint
FROM cash_statement_lines l
WHERE l.organization_id = sqlc.arg(organization_id)
  AND l.cash_account_id = sqlc.arg(cash_account_id)
  AND l.status IN ('pending', 'difference')
  AND l.line_date <= sqlc.arg(day)::date;

-- --------------------------------------------------------------- lines

-- name: InsertCashStatementLine :one
INSERT INTO cash_statement_lines (
    organization_id, statement_id, cash_account_id, line_no, line_date, direction, amount, fee, line_type,
    description, reference, balance_after
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(statement_id), sqlc.arg(cash_account_id),
    (SELECT COALESCE(MAX(l.line_no), 0) + 1 FROM cash_statement_lines l
      WHERE l.organization_id = sqlc.arg(organization_id) AND l.statement_id = sqlc.arg(statement_id)),
    sqlc.arg(line_date), sqlc.arg(direction), sqlc.arg(amount), sqlc.arg(fee), sqlc.arg(line_type),
    sqlc.arg(description), sqlc.arg(reference), sqlc.narg(balance_after)
)
RETURNING id, uuid;

-- name: DeleteCashStatementLine :execrows
DELETE FROM cash_statement_lines
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
  AND status IN ('pending', 'ignored');

-- name: RefreshCashStatementPeriod :exec
-- Widens the statement period to cover its lines.
UPDATE cash_statements s
SET period_from = LEAST(s.period_from, l.first_date), period_to = GREATEST(s.period_to, l.last_date)
FROM (
    SELECT MIN(line_date)::date AS first_date, MAX(line_date)::date AS last_date
    FROM cash_statement_lines
    WHERE organization_id = sqlc.arg(organization_id) AND statement_id = sqlc.arg(statement_id)
) l
WHERE s.organization_id = sqlc.arg(organization_id) AND s.id = sqlc.arg(statement_id) AND l.first_date IS NOT NULL;

-- name: GetCashStatementLine :one
SELECT
    l.id, l.uuid, l.statement_id, l.cash_account_id, l.line_no, l.line_date, l.direction, l.amount, l.fee, l.line_type,
    l.description, l.reference, l.balance_after, l.status, l.match_method, l.matched_movement_id, l.difference_amount,
    l.posted_movement_id, l.fee_movement_id, l.post_count, l.note, l.resolved_at, l.created_at, l.updated_at,
    s.uuid AS statement_uuid, s.status AS statement_status, s.branch_id,
    a.uuid AS account_uuid, a.name AS account_name, a.kind AS account_kind, a.currency AS account_currency,
    mm.uuid AS matched_uuid, mm.kind AS matched_kind, mm.movement_date AS matched_date, mm.amount AS matched_amount,
    mm.description AS matched_description,
    pm.uuid AS posted_uuid, pm.kind AS posted_kind, pm.transfer_group AS posted_transfer_group,
    fm.uuid AS fee_uuid,
    ru.uuid AS resolver_uuid, ru.name AS resolver_name, ru.surname AS resolver_surname
FROM cash_statement_lines l
JOIN cash_statements s ON s.organization_id = l.organization_id AND s.id = l.statement_id
JOIN cash_accounts a ON a.organization_id = l.organization_id AND a.id = l.cash_account_id
LEFT JOIN cash_movements mm ON mm.organization_id = l.organization_id AND mm.id = l.matched_movement_id
LEFT JOIN cash_movements pm ON pm.organization_id = l.organization_id AND pm.id = l.posted_movement_id
LEFT JOIN cash_movements fm ON fm.organization_id = l.organization_id AND fm.id = l.fee_movement_id
LEFT JOIN users ru ON ru.id = l.resolved_by
WHERE l.organization_id = sqlc.arg(organization_id)
  AND l.uuid = sqlc.arg(uuid);

-- name: ListCashStatementLines :many
SELECT
    l.id, l.uuid, l.statement_id, l.cash_account_id, l.line_no, l.line_date, l.direction, l.amount, l.fee, l.line_type,
    l.description, l.reference, l.balance_after, l.status, l.match_method, l.matched_movement_id, l.difference_amount,
    l.posted_movement_id, l.fee_movement_id, l.post_count, l.note, l.resolved_at, l.created_at, l.updated_at,
    s.uuid AS statement_uuid, s.status AS statement_status, s.branch_id,
    a.uuid AS account_uuid, a.name AS account_name, a.kind AS account_kind, a.currency AS account_currency,
    mm.uuid AS matched_uuid, mm.kind AS matched_kind, mm.movement_date AS matched_date, mm.amount AS matched_amount,
    mm.description AS matched_description,
    pm.uuid AS posted_uuid, pm.kind AS posted_kind, pm.transfer_group AS posted_transfer_group,
    fm.uuid AS fee_uuid,
    ru.uuid AS resolver_uuid, ru.name AS resolver_name, ru.surname AS resolver_surname
FROM cash_statement_lines l
JOIN cash_statements s ON s.organization_id = l.organization_id AND s.id = l.statement_id
JOIN cash_accounts a ON a.organization_id = l.organization_id AND a.id = l.cash_account_id
LEFT JOIN cash_movements mm ON mm.organization_id = l.organization_id AND mm.id = l.matched_movement_id
LEFT JOIN cash_movements pm ON pm.organization_id = l.organization_id AND pm.id = l.posted_movement_id
LEFT JOIN cash_movements fm ON fm.organization_id = l.organization_id AND fm.id = l.fee_movement_id
LEFT JOIN users ru ON ru.id = l.resolved_by
WHERE l.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR s.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(statement_ids)::bigint[] IS NULL OR l.statement_id = ANY(sqlc.narg(statement_ids)::bigint[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR l.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(line_types)::text[] IS NULL OR l.line_type = ANY(sqlc.narg(line_types)::text[]))
  AND (sqlc.narg(direction)::text IS NULL OR l.direction = sqlc.narg(direction)::text)
  AND (
    sqlc.narg(q)::text IS NULL
    OR l.description ILIKE '%' || sqlc.narg(q) || '%'
    OR l.reference ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'line_date' AND NOT sqlc.arg(sort_desc)::boolean THEN l.line_date END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'line_date' AND sqlc.arg(sort_desc)::boolean THEN l.line_date END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND NOT sqlc.arg(sort_desc)::boolean THEN l.amount END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND sqlc.arg(sort_desc)::boolean THEN l.amount END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND NOT sqlc.arg(sort_desc)::boolean THEN l.status END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND sqlc.arg(sort_desc)::boolean THEN l.status END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'line_no' AND sqlc.arg(sort_desc)::boolean THEN l.line_no END DESC,
  l.line_no ASC, l.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountCashStatementLines :one
SELECT COUNT(*)::bigint
FROM cash_statement_lines l
JOIN cash_statements s ON s.organization_id = l.organization_id AND s.id = l.statement_id
WHERE l.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR s.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(statement_ids)::bigint[] IS NULL OR l.statement_id = ANY(sqlc.narg(statement_ids)::bigint[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR l.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(line_types)::text[] IS NULL OR l.line_type = ANY(sqlc.narg(line_types)::text[]))
  AND (sqlc.narg(direction)::text IS NULL OR l.direction = sqlc.narg(direction)::text)
  AND (
    sqlc.narg(q)::text IS NULL
    OR l.description ILIKE '%' || sqlc.narg(q) || '%'
    OR l.reference ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListCashStatementLineIDs :many
-- Pending lines of a statement (auto-match pass), in line order.
SELECT l.id, l.uuid
FROM cash_statement_lines l
WHERE l.organization_id = sqlc.arg(organization_id)
  AND l.statement_id = sqlc.arg(statement_id)
  AND l.status = 'pending'
ORDER BY l.line_no ASC;

-- name: LockCashStatementLine :one
SELECT l.id, l.uuid, l.statement_id, l.cash_account_id, l.line_date, l.direction, l.amount, l.fee, l.line_type,
    l.description, l.reference, l.status, l.matched_movement_id, l.posted_movement_id, l.fee_movement_id, l.post_count
FROM cash_statement_lines l
WHERE l.organization_id = sqlc.arg(organization_id) AND l.id = sqlc.arg(id)
FOR UPDATE;

-- name: ListCashMatchCandidates :many
-- Movements of an account a statement line may be matched with: same
-- direction, dated inside the window, not reversed, not a reversal, not
-- matched by another line. Ordered by amount closeness, then date closeness.
SELECT
    m.id, m.uuid, m.kind, m.direction, m.amount, m.currency, m.movement_date, m.description, m.source_type,
    ca.name AS counter_account_name,
    ABS(m.amount - sqlc.arg(amount)::numeric)::numeric AS amount_delta,
    ABS(m.movement_date - sqlc.arg(line_date)::date)::int AS day_delta
FROM cash_movements m
LEFT JOIN cash_accounts ca ON ca.organization_id = m.organization_id AND ca.id = m.counter_account_id
WHERE m.organization_id = sqlc.arg(organization_id)
  AND m.cash_account_id = sqlc.arg(cash_account_id)
  AND m.direction = sqlc.arg(direction)
  AND m.kind <> 'reversal'
  AND m.movement_date BETWEEN sqlc.arg(date_from)::date AND sqlc.arg(date_to)::date
  AND (NOT sqlc.arg(exact_amount)::boolean OR m.amount = sqlc.arg(amount)::numeric)
  AND (
    sqlc.narg(q)::text IS NULL
    OR m.description ILIKE '%' || sqlc.narg(q) || '%'
  )
  AND NOT EXISTS (
    SELECT 1 FROM cash_movements rb
    WHERE rb.organization_id = m.organization_id AND rb.reverses_movement_id = m.id
  )
  AND NOT EXISTS (
    SELECT 1 FROM cash_statement_lines x
    WHERE x.organization_id = m.organization_id AND x.matched_movement_id = m.id
  )
  AND NOT EXISTS (
    SELECT 1 FROM cash_statement_lines x
    WHERE x.organization_id = m.organization_id AND (x.posted_movement_id = m.id OR x.fee_movement_id = m.id)
  )
ORDER BY amount_delta ASC, day_delta ASC, m.movement_date DESC, m.id DESC
LIMIT sqlc.arg(limit_count);

-- name: SetCashStatementLineMatched :execrows
UPDATE cash_statement_lines
SET status = sqlc.arg(status), match_method = sqlc.arg(match_method), matched_movement_id = sqlc.arg(matched_movement_id),
    difference_amount = sqlc.arg(difference_amount), fee_movement_id = sqlc.narg(fee_movement_id),
    post_count = sqlc.arg(post_count), note = sqlc.arg(note), resolved_by = sqlc.narg(resolved_by), resolved_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: SetCashStatementLinePosted :execrows
UPDATE cash_statement_lines
SET status = 'posted', match_method = '', matched_movement_id = NULL, difference_amount = 0,
    posted_movement_id = sqlc.arg(posted_movement_id), fee_movement_id = sqlc.narg(fee_movement_id),
    post_count = sqlc.arg(post_count), note = sqlc.arg(note), resolved_by = sqlc.narg(resolved_by), resolved_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: SetCashStatementLineIgnored :execrows
UPDATE cash_statement_lines
SET status = 'ignored', match_method = '', matched_movement_id = NULL, difference_amount = 0,
    note = sqlc.arg(note), resolved_by = sqlc.narg(resolved_by), resolved_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: ResetCashStatementLine :execrows
UPDATE cash_statement_lines
SET status = 'pending', match_method = '', matched_movement_id = NULL, difference_amount = 0,
    posted_movement_id = NULL, fee_movement_id = NULL, note = sqlc.arg(note), resolved_by = NULL, resolved_at = NULL
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: CountCashStatementPostings :one
-- Lines of a statement that wrote movements (a statement with postings is
-- kept: the movements stay in the book).
SELECT COUNT(*)::bigint
FROM cash_statement_lines l
WHERE l.organization_id = sqlc.arg(organization_id)
  AND l.statement_id = sqlc.arg(statement_id)
  AND (l.posted_movement_id IS NOT NULL OR l.fee_movement_id IS NOT NULL OR l.post_count > 0);

-- name: CashStatementLineStatusFacets :many
SELECT l.status AS value, COUNT(*)::bigint AS count
FROM cash_statement_lines l
WHERE l.organization_id = sqlc.arg(organization_id)
  AND l.statement_id = sqlc.arg(statement_id)
GROUP BY l.status
ORDER BY l.status ASC;
