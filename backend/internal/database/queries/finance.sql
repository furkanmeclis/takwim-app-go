-- Income and expense records (TEC-915). Every query takes the organization
-- id and filters by it; branch-bound reads take the caller's branch scope
-- (NULL = every branch; business-wide rows need every branch).

-- -------------------------------------------------------------- categories

-- name: ListFinanceCategories :many
SELECT c.id, c.uuid, c.kind, c.code, c.name, c.is_system, c.status, c.sort_order, c.created_at, c.updated_at,
    (SELECT COUNT(*) FROM finance_entries e
      WHERE e.organization_id = c.organization_id AND e.category_id = c.id AND e.entry_type = 'entry')::bigint AS entry_count
FROM finance_categories c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(kinds)::text[] IS NULL OR c.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR c.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(q)::text IS NULL OR c.name ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN lower(c.name) END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN lower(c.name) END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'kind' AND NOT sqlc.arg(sort_desc)::boolean THEN c.kind END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'kind' AND sqlc.arg(sort_desc)::boolean THEN c.kind END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND NOT sqlc.arg(sort_desc)::boolean THEN c.status END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'status' AND sqlc.arg(sort_desc)::boolean THEN c.status END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'sort_order' AND NOT sqlc.arg(sort_desc)::boolean THEN c.sort_order END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'sort_order' AND sqlc.arg(sort_desc)::boolean THEN c.sort_order END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN c.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN c.created_at END DESC,
  c.kind DESC, c.sort_order, lower(c.name), c.id
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountFinanceCategories :one
SELECT COUNT(*)::bigint FROM finance_categories c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(kinds)::text[] IS NULL OR c.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(statuses)::text[] IS NULL OR c.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(q)::text IS NULL OR c.name ILIKE '%' || sqlc.narg(q) || '%');

-- name: GetFinanceCategory :one
SELECT c.id, c.uuid, c.kind, c.code, c.name, c.is_system, c.status, c.sort_order, c.created_at, c.updated_at,
    (SELECT COUNT(*) FROM finance_entries e
      WHERE e.organization_id = c.organization_id AND e.category_id = c.id AND e.entry_type = 'entry')::bigint AS entry_count
FROM finance_categories c
WHERE c.organization_id = sqlc.arg(organization_id) AND c.uuid = sqlc.arg(uuid);

-- name: ListFinanceCategoryIDsByUUIDs :many
SELECT c.id, c.uuid FROM finance_categories c
WHERE c.organization_id = sqlc.arg(organization_id) AND c.uuid = ANY(sqlc.arg(uuids)::uuid[]);

-- name: InsertFinanceCategory :one
INSERT INTO finance_categories (organization_id, kind, name, status, sort_order)
VALUES (sqlc.arg(organization_id), sqlc.arg(kind), sqlc.arg(name), sqlc.arg(status), sqlc.arg(sort_order))
RETURNING id, uuid;

-- name: UpdateFinanceCategory :execrows
UPDATE finance_categories
SET name = sqlc.arg(name), status = sqlc.arg(status), sort_order = sqlc.arg(sort_order)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: DeleteFinanceCategory :execrows
-- Removes a business category no entry uses (seeded ones are archived).
DELETE FROM finance_categories c
WHERE c.organization_id = sqlc.arg(organization_id) AND c.id = sqlc.arg(id) AND NOT c.is_system
  AND NOT EXISTS (SELECT 1 FROM finance_entries e WHERE e.organization_id = c.organization_id AND e.category_id = c.id);

-- ----------------------------------------------------------------- entries

-- name: InsertFinanceEntry :one
INSERT INTO finance_entries (
    uuid, organization_id, entry_type, kind, category_id, branch_id, staff_id, cash_account_id, cash_movement_id,
    amount, currency, entry_date, description, reverses_entry_id, replaces_entry_id, reason,
    document_key, document_name, document_content_type, document_size_bytes, idempotency_key, created_by
) VALUES (
    sqlc.arg(uuid), sqlc.arg(organization_id), sqlc.arg(entry_type), sqlc.arg(kind), sqlc.arg(category_id),
    sqlc.narg(branch_id), sqlc.narg(staff_id), sqlc.arg(cash_account_id), sqlc.arg(cash_movement_id),
    sqlc.arg(amount), sqlc.arg(currency), sqlc.arg(entry_date), sqlc.arg(description),
    sqlc.narg(reverses_entry_id), sqlc.narg(replaces_entry_id), sqlc.arg(reason),
    sqlc.narg(document_key), sqlc.arg(document_name), sqlc.arg(document_content_type), sqlc.arg(document_size_bytes),
    sqlc.narg(idempotency_key), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: GetFinanceEntry :one
SELECT e.id, e.uuid, e.entry_type, e.kind, e.category_id, e.branch_id, e.staff_id, e.cash_account_id,
    e.cash_movement_id, e.amount, e.currency, e.entry_date, e.description, e.reason, e.document_key,
    e.document_name, e.document_content_type, e.document_size_bytes, e.idempotency_key, e.created_at, e.updated_at,
    c.uuid AS category_uuid, c.name AS category_name, c.code AS category_code,
    b.uuid AS branch_uuid, b.name AS branch_name,
    st.uuid AS staff_uuid, st.name AS staff_name,
    a.uuid AS account_uuid, a.name AS account_name, a.kind AS account_kind,
    m.uuid AS cash_movement_uuid,
    rv.uuid AS reverses_uuid, rb.uuid AS reversed_by_uuid,
    rp.uuid AS replaces_uuid, rpb.uuid AS replaced_by_uuid,
    u.uuid AS creator_uuid, u.name AS creator_name, u.surname AS creator_surname
FROM finance_entries e
JOIN finance_categories c ON c.organization_id = e.organization_id AND c.id = e.category_id
JOIN cash_accounts a ON a.organization_id = e.organization_id AND a.id = e.cash_account_id
JOIN cash_movements m ON m.organization_id = e.organization_id AND m.id = e.cash_movement_id
LEFT JOIN branches b ON b.organization_id = e.organization_id AND b.id = e.branch_id
LEFT JOIN staff st ON st.organization_id = e.organization_id AND st.id = e.staff_id
LEFT JOIN finance_entries rv ON rv.organization_id = e.organization_id AND rv.id = e.reverses_entry_id
LEFT JOIN finance_entries rb ON rb.organization_id = e.organization_id AND rb.reverses_entry_id = e.id
LEFT JOIN finance_entries rp ON rp.organization_id = e.organization_id AND rp.id = e.replaces_entry_id
LEFT JOIN finance_entries rpb ON rpb.organization_id = e.organization_id AND rpb.replaces_entry_id = e.id
LEFT JOIN users u ON u.id = e.created_by
WHERE e.organization_id = sqlc.arg(organization_id) AND e.uuid = sqlc.arg(uuid);

-- name: GetFinanceEntryByIdempotencyKey :one
SELECT e.id, e.uuid, e.kind, e.amount, e.category_id, e.cash_account_id FROM finance_entries e
WHERE e.organization_id = sqlc.arg(organization_id) AND e.idempotency_key = sqlc.arg(idempotency_key);

-- name: LockFinanceEntry :one
SELECT e.id FROM finance_entries e
WHERE e.organization_id = sqlc.arg(organization_id) AND e.id = sqlc.arg(id)
FOR UPDATE;

-- name: FinanceEntryReversed :one
SELECT EXISTS (
    SELECT 1 FROM finance_entries r
    WHERE r.organization_id = sqlc.arg(organization_id) AND r.reverses_entry_id = sqlc.arg(id)::bigint
)::boolean;

-- name: UpdateFinanceEntryClassification :execrows
UPDATE finance_entries
SET category_id = sqlc.arg(category_id), description = sqlc.arg(description), staff_id = sqlc.narg(staff_id)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: SetFinanceEntryDocument :execrows
UPDATE finance_entries
SET document_key = sqlc.narg(document_key), document_name = sqlc.arg(document_name),
    document_content_type = sqlc.arg(document_content_type), document_size_bytes = sqlc.arg(document_size_bytes)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND entry_type = 'entry';

-- name: GetFinanceCashMovement :one
SELECT m.id, m.uuid FROM cash_movements m
WHERE m.organization_id = sqlc.arg(organization_id) AND m.uuid = sqlc.arg(uuid);

-- name: GetFinanceStaffByUUID :one
SELECT st.id, st.uuid, st.name FROM staff st
WHERE st.organization_id = sqlc.arg(organization_id) AND st.uuid = sqlc.arg(uuid) AND st.deleted_at IS NULL;

-- name: ListFinanceStaffIDsByUUIDs :many
SELECT st.id, st.uuid FROM staff st
WHERE st.organization_id = sqlc.arg(organization_id) AND st.uuid = ANY(sqlc.arg(uuids)::uuid[]);

-- name: ListFinanceAccountIDsByUUIDs :many
SELECT a.id, a.uuid, a.branch_id FROM cash_accounts a
WHERE a.organization_id = sqlc.arg(organization_id) AND a.uuid = ANY(sqlc.arg(uuids)::uuid[]);

-- ------------------------------------------------------------------ ledger

-- name: ListFinanceLedger :many
-- The income / expense list: the business's own records (source manual)
-- and the sale lines as automatic income (source sale), reversals as
-- negative rows. Filters apply to both sides; a filter that only manual
-- records have (account, category uuid) leaves the sale rows out.
SELECT x.source, x.id, x.uuid, x.entry_type, x.kind, x.entry_date, x.amount, x.currency,
    x.branch_uuid, x.branch_name, x.category_uuid, x.category_name, x.item_type,
    x.staff_uuid, x.staff_name, x.account_uuid, x.account_name, x.description, x.has_document,
    x.reversed, x.reservation_uuid, x.created_at
FROM (
    SELECT 'manual'::text AS source, e.id, e.uuid, e.entry_type::text AS entry_type, e.kind::text AS kind,
        e.entry_date, e.amount, e.currency::text AS currency, e.branch_id,
        b.uuid AS branch_uuid, b.name AS branch_name, e.category_id, c.uuid AS category_uuid, c.name AS category_name,
        ''::text AS item_type, e.staff_id, st.uuid AS staff_uuid, st.name AS staff_name,
        e.cash_account_id, a.uuid AS account_uuid, a.name AS account_name, e.description::text AS description,
        (e.document_key IS NOT NULL)::boolean AS has_document,
        EXISTS (SELECT 1 FROM finance_entries r WHERE r.organization_id = e.organization_id AND r.reverses_entry_id = e.id)::boolean AS reversed,
        NULL::uuid AS reservation_uuid, e.created_at
    FROM finance_entries e
    LEFT JOIN finance_categories c ON c.organization_id = e.organization_id AND c.id = e.category_id
    LEFT JOIN cash_accounts a ON a.organization_id = e.organization_id AND a.id = e.cash_account_id
    LEFT JOIN branches b ON b.organization_id = e.organization_id AND b.id = e.branch_id
    LEFT JOIN staff st ON st.organization_id = e.organization_id AND st.id = e.staff_id
    WHERE e.organization_id = sqlc.arg(organization_id)
    UNION ALL
    SELECT 'sale'::text, s.id, s.uuid, CASE WHEN s.kind = 'sale' THEN 'entry' ELSE 'reversal' END::text, 'income'::text,
        s.sale_date, s.net_amount, s.currency::text, s.branch_id,
        sb.uuid, sb.name, NULL::bigint, NULL::uuid, NULL::text,
        s.item_type::text, s.staff_id, ss.uuid, ss.name,
        NULL::bigint, NULL::uuid, NULL::text, s.description::text,
        FALSE,
        EXISTS (SELECT 1 FROM sale_lines r WHERE r.organization_id = s.organization_id AND r.reverses_line_id = s.id),
        res.uuid, s.created_at
    FROM sale_lines s
    LEFT JOIN branches sb ON sb.organization_id = s.organization_id AND sb.id = s.branch_id
    LEFT JOIN staff ss ON ss.organization_id = s.organization_id AND ss.id = s.staff_id
    LEFT JOIN reservations res ON res.organization_id = s.organization_id AND res.id = s.reservation_id
    WHERE s.organization_id = sqlc.arg(organization_id)
) x
WHERE (sqlc.narg(branch_ids)::bigint[] IS NULL OR x.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (
    (sqlc.narg(filter_branch_ids)::bigint[] IS NULL AND NOT sqlc.arg(filter_head_office)::boolean)
    OR x.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])
    OR (sqlc.arg(filter_head_office)::boolean AND x.branch_id IS NULL)
  )
  AND (sqlc.narg(sources)::text[] IS NULL OR x.source = ANY(sqlc.narg(sources)::text[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR x.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (
    (sqlc.narg(category_ids)::bigint[] IS NULL AND sqlc.narg(item_types)::text[] IS NULL)
    OR x.category_id = ANY(sqlc.narg(category_ids)::bigint[])
    OR (x.source = 'sale' AND x.item_type = ANY(sqlc.narg(item_types)::text[]))
  )
  AND (sqlc.narg(staff_ids)::bigint[] IS NULL OR x.staff_id = ANY(sqlc.narg(staff_ids)::bigint[]))
  AND (sqlc.narg(account_ids)::bigint[] IS NULL OR x.cash_account_id = ANY(sqlc.narg(account_ids)::bigint[]))
  AND (sqlc.narg(date_from)::date IS NULL OR x.entry_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR x.entry_date <= sqlc.narg(date_to)::date)
  AND (
    sqlc.narg(q)::text IS NULL
    OR x.description ILIKE '%' || sqlc.narg(q) || '%'
    OR x.category_name ILIKE '%' || sqlc.narg(q) || '%'
    OR x.staff_name ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND NOT sqlc.arg(sort_desc)::boolean THEN x.amount END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND sqlc.arg(sort_desc)::boolean THEN x.amount END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'entry_date' AND NOT sqlc.arg(sort_desc)::boolean THEN x.entry_date END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'entry_date' AND NOT sqlc.arg(sort_desc)::boolean THEN x.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN x.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN x.created_at END DESC,
  x.entry_date DESC, x.created_at DESC, x.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountFinanceLedger :one
SELECT COUNT(*)::bigint
FROM (
    SELECT 'manual'::text AS source, e.kind::text AS kind, e.entry_date, e.branch_id, e.category_id,
        ''::text AS item_type, e.staff_id, e.cash_account_id, e.description::text AS description,
        c.name AS category_name, st.name AS staff_name
    FROM finance_entries e
    LEFT JOIN finance_categories c ON c.organization_id = e.organization_id AND c.id = e.category_id
    LEFT JOIN staff st ON st.organization_id = e.organization_id AND st.id = e.staff_id
    WHERE e.organization_id = sqlc.arg(organization_id)
    UNION ALL
    SELECT 'sale'::text, 'income'::text, s.sale_date, s.branch_id, NULL::bigint,
        s.item_type::text, s.staff_id, NULL::bigint, s.description::text,
        NULL::text, ss.name
    FROM sale_lines s
    LEFT JOIN staff ss ON ss.organization_id = s.organization_id AND ss.id = s.staff_id
    WHERE s.organization_id = sqlc.arg(organization_id)
) x
WHERE (sqlc.narg(branch_ids)::bigint[] IS NULL OR x.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (
    (sqlc.narg(filter_branch_ids)::bigint[] IS NULL AND NOT sqlc.arg(filter_head_office)::boolean)
    OR x.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])
    OR (sqlc.arg(filter_head_office)::boolean AND x.branch_id IS NULL)
  )
  AND (sqlc.narg(sources)::text[] IS NULL OR x.source = ANY(sqlc.narg(sources)::text[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR x.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (
    (sqlc.narg(category_ids)::bigint[] IS NULL AND sqlc.narg(item_types)::text[] IS NULL)
    OR x.category_id = ANY(sqlc.narg(category_ids)::bigint[])
    OR (x.source = 'sale' AND x.item_type = ANY(sqlc.narg(item_types)::text[]))
  )
  AND (sqlc.narg(staff_ids)::bigint[] IS NULL OR x.staff_id = ANY(sqlc.narg(staff_ids)::bigint[]))
  AND (sqlc.narg(account_ids)::bigint[] IS NULL OR x.cash_account_id = ANY(sqlc.narg(account_ids)::bigint[]))
  AND (sqlc.narg(date_from)::date IS NULL OR x.entry_date >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR x.entry_date <= sqlc.narg(date_to)::date)
  AND (
    sqlc.narg(q)::text IS NULL
    OR x.description ILIKE '%' || sqlc.narg(q) || '%'
    OR x.category_name ILIKE '%' || sqlc.narg(q) || '%'
    OR x.staff_name ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListFinanceCategoryFacets :many
-- Categories with the number of records using them (inside the scope).
SELECT c.uuid, c.kind, c.name, c.status,
    COUNT(e.id) FILTER (WHERE e.entry_type = 'entry')::bigint AS count
FROM finance_categories c
LEFT JOIN finance_entries e ON e.organization_id = c.organization_id AND e.category_id = c.id
    AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
WHERE c.organization_id = sqlc.arg(organization_id)
GROUP BY c.id, c.uuid, c.kind, c.name, c.status, c.sort_order
ORDER BY c.kind DESC, c.sort_order, lower(c.name);

-- name: ListFinanceBranchFacets :many
-- Branches with records (manual and sales); NULL = business-wide records.
SELECT b.uuid AS branch_uuid, b.name AS branch_name, COUNT(*)::bigint AS count
FROM (
    SELECT e.branch_id FROM finance_entries e
    WHERE e.organization_id = sqlc.arg(organization_id) AND e.entry_type = 'entry'
    UNION ALL
    SELECT s.branch_id FROM sale_lines s
    WHERE s.organization_id = sqlc.arg(organization_id) AND s.kind = 'sale'
) x
LEFT JOIN branches b ON b.organization_id = sqlc.arg(organization_id) AND b.id = x.branch_id
WHERE (sqlc.narg(branch_ids)::bigint[] IS NULL OR x.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY b.uuid, b.name
ORDER BY b.name NULLS FIRST;

-- name: ListFinanceStaffFacets :many
SELECT st.uuid AS staff_uuid, st.name AS staff_name, COUNT(*)::bigint AS count
FROM (
    SELECT e.staff_id, e.branch_id FROM finance_entries e
    WHERE e.organization_id = sqlc.arg(organization_id) AND e.entry_type = 'entry' AND e.staff_id IS NOT NULL
    UNION ALL
    SELECT s.staff_id, s.branch_id FROM sale_lines s
    WHERE s.organization_id = sqlc.arg(organization_id) AND s.kind = 'sale' AND s.staff_id IS NOT NULL
) x
JOIN staff st ON st.organization_id = sqlc.arg(organization_id) AND st.id = x.staff_id
WHERE (sqlc.narg(branch_ids)::bigint[] IS NULL OR x.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY st.uuid, st.name
ORDER BY st.name;

-- ----------------------------------------------------------------- summary

-- name: FinanceTotalsByCategory :many
-- The records of a period per kind, category and currency (reversals
-- netted on the day they were posted).
SELECT e.kind::text AS kind, c.uuid AS category_uuid, c.name AS category_name, e.currency::text AS currency,
    COALESCE(SUM(e.amount), 0)::numeric AS total,
    COUNT(*) FILTER (WHERE e.entry_type = 'entry')::bigint AS entry_count
FROM finance_entries e
JOIN finance_categories c ON c.organization_id = e.organization_id AND c.id = e.category_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.entry_date BETWEEN sqlc.arg(date_from)::date AND sqlc.arg(date_to)::date
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR e.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
GROUP BY e.kind, c.uuid, c.name, c.sort_order, e.currency
ORDER BY e.kind DESC, e.currency, c.sort_order, c.name;
