-- Tenant resource: products and stock (TEC-918). Every query filters by
-- organization_id. The catalog (products, variants) is business-level;
-- product_branches holds the branch offers, stock_levels the running
-- balance per branch and stock item, stock_movements the append-only ledger.
-- stock_branch_ids is the caller's branch scope (nil = every branch): the
-- stock totals of a row only count those branches.

-- name: CreateProduct :one
INSERT INTO products (
    organization_id, name, description, sku, barcode, unit, price, cost, low_stock_threshold, show_online, is_active
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(name), sqlc.arg(description), sqlc.arg(sku), sqlc.arg(barcode), sqlc.arg(unit),
    sqlc.arg(price), sqlc.arg(cost), sqlc.arg(low_stock_threshold), sqlc.arg(show_online), sqlc.arg(is_active)
)
RETURNING *;

-- name: UpdateProduct :one
UPDATE products
SET name = sqlc.arg(name), description = sqlc.arg(description), sku = sqlc.arg(sku), barcode = sqlc.arg(barcode),
    unit = sqlc.arg(unit), price = sqlc.arg(price), cost = sqlc.arg(cost),
    low_stock_threshold = sqlc.arg(low_stock_threshold), show_online = sqlc.arg(show_online),
    is_active = sqlc.arg(is_active)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: SetProductActive :exec
UPDATE products SET is_active = sqlc.arg(is_active)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: SetProductImage :one
-- Returns the previous key so the caller can remove the old object.
WITH prev AS (
    SELECT p.id AS prev_id, p.image_key AS prev_key
    FROM products p
    WHERE p.organization_id = sqlc.arg(organization_id) AND p.id = sqlc.arg(id) AND p.deleted_at IS NULL
    FOR UPDATE
)
UPDATE products s
SET image_uuid = sqlc.narg(image_uuid), image_key = sqlc.narg(image_key),
    image_content_type = sqlc.narg(image_content_type), image_size_bytes = sqlc.narg(image_size_bytes)
FROM prev
WHERE s.organization_id = sqlc.arg(organization_id) AND s.id = prev.prev_id
RETURNING prev.prev_key AS previous_key;

-- name: GetProductPublicImage :one
SELECT p.image_key, p.image_content_type, p.image_size_bytes
FROM products p
JOIN organizations o ON o.id = p.organization_id
WHERE p.organization_id = o.id
  AND o.uuid = sqlc.arg(organization_uuid)
  AND p.image_uuid = sqlc.arg(image_uuid)
  AND p.deleted_at IS NULL;

-- name: SoftDeleteProduct :execrows
UPDATE products SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: GetProduct :one
SELECT
    p.*,
    (SELECT COUNT(*) FROM product_variants v
     WHERE v.organization_id = p.organization_id AND v.product_id = p.id AND v.deleted_at IS NULL)::bigint AS variants_count,
    st.on_hand::bigint AS stock_on_hand, st.reserved::bigint AS stock_reserved
FROM products p
CROSS JOIN LATERAL (
    SELECT COALESCE(SUM(sl.on_hand), 0) AS on_hand, COALESCE(SUM(sl.reserved), 0) AS reserved
    FROM stock_levels sl
    WHERE sl.organization_id = p.organization_id AND sl.product_id = p.id
      AND (sqlc.narg(stock_branch_ids)::bigint[] IS NULL OR sl.branch_id = ANY(sqlc.narg(stock_branch_ids)::bigint[]))
) st
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.uuid = sqlc.arg(uuid)
  AND p.deleted_at IS NULL;

-- name: GetProductBySKU :one
SELECT * FROM products
WHERE organization_id = sqlc.arg(organization_id) AND lower(sku) = lower(sqlc.arg(sku)::text) AND deleted_at IS NULL;

-- name: StockCodeTaken :one
-- SKUs are unique across products and variants of the organization.
SELECT (
    EXISTS (
        SELECT 1 FROM products p
        WHERE p.organization_id = sqlc.arg(organization_id) AND lower(p.sku) = lower(sqlc.arg(sku)::text)
          AND p.deleted_at IS NULL
          AND (sqlc.narg(exclude_product_id)::bigint IS NULL OR p.id <> sqlc.narg(exclude_product_id)::bigint)
    )
    OR EXISTS (
        SELECT 1 FROM product_variants v
        WHERE v.organization_id = sqlc.arg(organization_id) AND lower(v.sku) = lower(sqlc.arg(sku)::text)
          AND v.deleted_at IS NULL
          AND (sqlc.narg(exclude_variant_id)::bigint IS NULL OR v.id <> sqlc.narg(exclude_variant_id)::bigint)
    )
)::boolean;

-- name: LookupStockCode :many
-- Barcode / SKU entry: the products and variants whose code or barcode is
-- exactly the given text (case-insensitive SKU).
SELECT p.uuid AS product_uuid, NULL::uuid AS variant_uuid
FROM products p
WHERE p.organization_id = sqlc.arg(organization_id) AND p.deleted_at IS NULL
  AND (lower(p.sku) = lower(sqlc.arg(code)::text) OR (p.barcode <> '' AND p.barcode = sqlc.arg(code)::text))
UNION ALL
SELECT p.uuid AS product_uuid, v.uuid AS variant_uuid
FROM product_variants v
JOIN products p ON p.id = v.product_id AND p.organization_id = v.organization_id AND p.deleted_at IS NULL
WHERE v.organization_id = sqlc.arg(organization_id) AND v.deleted_at IS NULL
  AND (lower(v.sku) = lower(sqlc.arg(code)::text) OR (v.barcode <> '' AND v.barcode = sqlc.arg(code)::text))
LIMIT 10;

-- name: ListProducts :many
SELECT
    p.*,
    (SELECT COUNT(*) FROM product_variants v
     WHERE v.organization_id = p.organization_id AND v.product_id = p.id AND v.deleted_at IS NULL)::bigint AS variants_count,
    st.on_hand::bigint AS stock_on_hand, st.reserved::bigint AS stock_reserved
FROM products p
CROSS JOIN LATERAL (
    SELECT COALESCE(SUM(sl.on_hand), 0) AS on_hand, COALESCE(SUM(sl.reserved), 0) AS reserved
    FROM stock_levels sl
    WHERE sl.organization_id = p.organization_id AND sl.product_id = p.id
      AND (sqlc.narg(stock_branch_ids)::bigint[] IS NULL OR sl.branch_id = ANY(sqlc.narg(stock_branch_ids)::bigint[]))
) st
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.deleted_at IS NULL
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR EXISTS (
        SELECT 1 FROM product_branches pb
        WHERE pb.organization_id = p.organization_id AND pb.product_id = p.id
          AND pb.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])))
  AND (sqlc.narg(is_active)::boolean IS NULL OR p.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(show_online)::boolean IS NULL OR p.show_online = sqlc.narg(show_online))
  AND (sqlc.narg(stock_status)::text[] IS NULL OR (CASE
        WHEN st.on_hand - st.reserved <= 0 THEN 'out' WHEN st.on_hand - st.reserved <= p.low_stock_threshold THEN 'low' ELSE 'in'
      END) = ANY(sqlc.narg(stock_status)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.name ILIKE '%' || sqlc.narg(q) || '%'
    OR p.sku ILIKE '%' || sqlc.narg(q) || '%'
    OR p.barcode = sqlc.narg(q)
    OR EXISTS (
        SELECT 1 FROM product_variants v
        WHERE v.organization_id = p.organization_id AND v.product_id = p.id AND v.deleted_at IS NULL
          AND (v.name ILIKE '%' || sqlc.narg(q) || '%' OR v.sku ILIKE '%' || sqlc.narg(q) || '%' OR v.barcode = sqlc.narg(q)))
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN p.name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN p.name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'sku' AND NOT sqlc.arg(sort_desc)::boolean THEN p.sku END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'sku' AND sqlc.arg(sort_desc)::boolean THEN p.sku END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'price' AND NOT sqlc.arg(sort_desc)::boolean THEN p.price END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'price' AND sqlc.arg(sort_desc)::boolean THEN p.price END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'cost' AND NOT sqlc.arg(sort_desc)::boolean THEN p.cost END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'cost' AND sqlc.arg(sort_desc)::boolean THEN p.cost END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'stock' AND NOT sqlc.arg(sort_desc)::boolean THEN st.on_hand END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'stock' AND sqlc.arg(sort_desc)::boolean THEN st.on_hand END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'is_active' AND NOT sqlc.arg(sort_desc)::boolean THEN p.is_active END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'is_active' AND sqlc.arg(sort_desc)::boolean THEN p.is_active END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN p.updated_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN p.updated_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN p.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN p.created_at END DESC,
  p.name ASC, p.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountProducts :one
SELECT COUNT(*)::bigint
FROM products p
CROSS JOIN LATERAL (
    SELECT COALESCE(SUM(sl.on_hand), 0) AS on_hand, COALESCE(SUM(sl.reserved), 0) AS reserved
    FROM stock_levels sl
    WHERE sl.organization_id = p.organization_id AND sl.product_id = p.id
      AND (sqlc.narg(stock_branch_ids)::bigint[] IS NULL OR sl.branch_id = ANY(sqlc.narg(stock_branch_ids)::bigint[]))
) st
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.deleted_at IS NULL
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR EXISTS (
        SELECT 1 FROM product_branches pb
        WHERE pb.organization_id = p.organization_id AND pb.product_id = p.id
          AND pb.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])))
  AND (sqlc.narg(is_active)::boolean IS NULL OR p.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(show_online)::boolean IS NULL OR p.show_online = sqlc.narg(show_online))
  AND (sqlc.narg(stock_status)::text[] IS NULL OR (CASE
        WHEN st.on_hand - st.reserved <= 0 THEN 'out' WHEN st.on_hand - st.reserved <= p.low_stock_threshold THEN 'low' ELSE 'in'
      END) = ANY(sqlc.narg(stock_status)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.name ILIKE '%' || sqlc.narg(q) || '%'
    OR p.sku ILIKE '%' || sqlc.narg(q) || '%'
    OR p.barcode = sqlc.narg(q)
    OR EXISTS (
        SELECT 1 FROM product_variants v
        WHERE v.organization_id = p.organization_id AND v.product_id = p.id AND v.deleted_at IS NULL
          AND (v.name ILIKE '%' || sqlc.narg(q) || '%' OR v.sku ILIKE '%' || sqlc.narg(q) || '%' OR v.barcode = sqlc.narg(q)))
  );

-- name: ListProductUUIDs :many
SELECT p.uuid
FROM products p
CROSS JOIN LATERAL (
    SELECT COALESCE(SUM(sl.on_hand), 0) AS on_hand, COALESCE(SUM(sl.reserved), 0) AS reserved
    FROM stock_levels sl
    WHERE sl.organization_id = p.organization_id AND sl.product_id = p.id
      AND (sqlc.narg(stock_branch_ids)::bigint[] IS NULL OR sl.branch_id = ANY(sqlc.narg(stock_branch_ids)::bigint[]))
) st
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.deleted_at IS NULL
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR EXISTS (
        SELECT 1 FROM product_branches pb
        WHERE pb.organization_id = p.organization_id AND pb.product_id = p.id
          AND pb.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])))
  AND (sqlc.narg(is_active)::boolean IS NULL OR p.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(show_online)::boolean IS NULL OR p.show_online = sqlc.narg(show_online))
  AND (sqlc.narg(stock_status)::text[] IS NULL OR (CASE
        WHEN st.on_hand - st.reserved <= 0 THEN 'out' WHEN st.on_hand - st.reserved <= p.low_stock_threshold THEN 'low' ELSE 'in'
      END) = ANY(sqlc.narg(stock_status)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.name ILIKE '%' || sqlc.narg(q) || '%'
    OR p.sku ILIKE '%' || sqlc.narg(q) || '%'
    OR p.barcode = sqlc.narg(q)
    OR EXISTS (
        SELECT 1 FROM product_variants v
        WHERE v.organization_id = p.organization_id AND v.product_id = p.id AND v.deleted_at IS NULL
          AND (v.name ILIKE '%' || sqlc.narg(q) || '%' OR v.sku ILIKE '%' || sqlc.narg(q) || '%' OR v.barcode = sqlc.narg(q)))
  )
ORDER BY p.id ASC
LIMIT sqlc.arg(limit_count);

-- name: ListProductsForExport :many
SELECT
    p.*,
    (SELECT COUNT(*) FROM product_variants v
     WHERE v.organization_id = p.organization_id AND v.product_id = p.id AND v.deleted_at IS NULL)::bigint AS variants_count,
    st.on_hand::bigint AS stock_on_hand, st.reserved::bigint AS stock_reserved
FROM products p
CROSS JOIN LATERAL (
    SELECT COALESCE(SUM(sl.on_hand), 0) AS on_hand, COALESCE(SUM(sl.reserved), 0) AS reserved
    FROM stock_levels sl
    WHERE sl.organization_id = p.organization_id AND sl.product_id = p.id
      AND (sqlc.narg(stock_branch_ids)::bigint[] IS NULL OR sl.branch_id = ANY(sqlc.narg(stock_branch_ids)::bigint[]))
) st
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.deleted_at IS NULL
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR EXISTS (
        SELECT 1 FROM product_branches pb
        WHERE pb.organization_id = p.organization_id AND pb.product_id = p.id
          AND pb.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])))
  AND (sqlc.narg(is_active)::boolean IS NULL OR p.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(show_online)::boolean IS NULL OR p.show_online = sqlc.narg(show_online))
  AND (sqlc.narg(stock_status)::text[] IS NULL OR (CASE
        WHEN st.on_hand - st.reserved <= 0 THEN 'out' WHEN st.on_hand - st.reserved <= p.low_stock_threshold THEN 'low' ELSE 'in'
      END) = ANY(sqlc.narg(stock_status)::text[]))
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.name ILIKE '%' || sqlc.narg(q) || '%'
    OR p.sku ILIKE '%' || sqlc.narg(q) || '%'
    OR p.barcode = sqlc.narg(q)
    OR EXISTS (
        SELECT 1 FROM product_variants v
        WHERE v.organization_id = p.organization_id AND v.product_id = p.id AND v.deleted_at IS NULL
          AND (v.name ILIKE '%' || sqlc.narg(q) || '%' OR v.sku ILIKE '%' || sqlc.narg(q) || '%' OR v.barcode = sqlc.narg(q)))
  )
ORDER BY p.name ASC, p.id ASC
LIMIT sqlc.arg(limit_count);

-- name: ListProductBranchFacets :many
SELECT b.uuid, b.name, COUNT(DISTINCT pb.product_id)::bigint AS count
FROM branches b
JOIN product_branches pb ON pb.branch_id = b.id AND pb.organization_id = b.organization_id
JOIN products p ON p.id = pb.product_id AND p.organization_id = pb.organization_id AND p.deleted_at IS NULL
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.deleted_at IS NULL
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR b.id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY b.id, b.uuid, b.name
ORDER BY b.name ASC;

-- name: ListProductBranches :many
-- Offers of the given products (list rows and the detail).
SELECT
    pb.product_id, pb.branch_id, pb.price, pb.is_active,
    b.uuid AS branch_uuid, b.name AS branch_name, b.code AS branch_code
FROM product_branches pb
JOIN branches b ON b.id = pb.branch_id AND b.organization_id = pb.organization_id AND b.deleted_at IS NULL
WHERE pb.organization_id = sqlc.arg(organization_id)
  AND pb.product_id = ANY(sqlc.arg(product_ids)::bigint[])
ORDER BY b.name ASC, b.id ASC;

-- name: DeleteProductBranch :exec
DELETE FROM product_branches
WHERE organization_id = sqlc.arg(organization_id) AND product_id = sqlc.arg(product_id) AND branch_id = sqlc.arg(branch_id);

-- name: UpsertProductBranch :exec
INSERT INTO product_branches (organization_id, product_id, branch_id, price, is_active)
VALUES (sqlc.arg(organization_id), sqlc.arg(product_id), sqlc.arg(branch_id), sqlc.narg(price), sqlc.arg(is_active))
ON CONFLICT (organization_id, product_id, branch_id) DO UPDATE
SET price = EXCLUDED.price, is_active = EXCLUDED.is_active;

-- name: CreateProductVariant :one
INSERT INTO product_variants (organization_id, product_id, name, sku, barcode, price, cost, is_active, sort_order)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(product_id), sqlc.arg(name), sqlc.arg(sku), sqlc.arg(barcode),
    sqlc.narg(price), sqlc.narg(cost), sqlc.arg(is_active), sqlc.arg(sort_order)
)
RETURNING *;

-- name: UpdateProductVariant :one
UPDATE product_variants
SET name = sqlc.arg(name), sku = sqlc.arg(sku), barcode = sqlc.arg(barcode), price = sqlc.narg(price),
    cost = sqlc.narg(cost), is_active = sqlc.arg(is_active), sort_order = sqlc.arg(sort_order)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteProductVariant :execrows
UPDATE product_variants SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: GetProductVariant :one
SELECT * FROM product_variants
WHERE organization_id = sqlc.arg(organization_id) AND product_id = sqlc.arg(product_id)
  AND uuid = sqlc.arg(uuid) AND deleted_at IS NULL;

-- name: ListProductVariants :many
SELECT * FROM product_variants
WHERE organization_id = sqlc.arg(organization_id)
  AND product_id = ANY(sqlc.arg(product_ids)::bigint[])
  AND deleted_at IS NULL
ORDER BY sort_order ASC, name ASC, id ASC;

-- name: ProductVariantNameTaken :one
SELECT EXISTS (
    SELECT 1 FROM product_variants
    WHERE organization_id = sqlc.arg(organization_id) AND product_id = sqlc.arg(product_id)
      AND lower(name) = lower(sqlc.arg(name)::text) AND deleted_at IS NULL
      AND (sqlc.narg(exclude_id)::bigint IS NULL OR id <> sqlc.narg(exclude_id)::bigint)
)::boolean;

-- name: SumStockItem :one
-- Units on hand and reserved of one stock item over every branch (a
-- product row with variant NULL, or one variant).
SELECT COALESCE(SUM(on_hand), 0)::bigint AS on_hand, COALESCE(SUM(reserved), 0)::bigint AS reserved
FROM stock_levels
WHERE organization_id = sqlc.arg(organization_id) AND product_id = sqlc.arg(product_id)
  AND variant_id IS NOT DISTINCT FROM sqlc.narg(variant_id)::bigint;

-- name: EnsureStockLevel :exec
INSERT INTO stock_levels (organization_id, branch_id, product_id, variant_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(branch_id), sqlc.arg(product_id), sqlc.narg(variant_id))
ON CONFLICT ON CONSTRAINT uq_stock_levels DO NOTHING;

-- name: LockStockLevel :one
SELECT * FROM stock_levels
WHERE organization_id = sqlc.arg(organization_id) AND branch_id = sqlc.arg(branch_id)
  AND product_id = sqlc.arg(product_id) AND variant_id IS NOT DISTINCT FROM sqlc.narg(variant_id)::bigint
FOR UPDATE;

-- name: SetStockLevelOnHand :exec
UPDATE stock_levels SET on_hand = sqlc.arg(on_hand)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: ListStockLevels :many
-- Stock of the given products in the branches of the scope.
SELECT
    sl.product_id, sl.variant_id, sl.branch_id, sl.on_hand, sl.reserved, sl.updated_at,
    b.uuid AS branch_uuid, b.name AS branch_name, b.code AS branch_code,
    v.uuid AS variant_uuid, v.name AS variant_name, v.sku AS variant_sku
FROM stock_levels sl
JOIN branches b ON b.id = sl.branch_id AND b.organization_id = sl.organization_id AND b.deleted_at IS NULL
LEFT JOIN product_variants v ON v.id = sl.variant_id AND v.organization_id = sl.organization_id
WHERE sl.organization_id = sqlc.arg(organization_id)
  AND sl.product_id = ANY(sqlc.arg(product_ids)::bigint[])
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR sl.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (v.id IS NULL OR v.deleted_at IS NULL)
ORDER BY b.name ASC, b.id ASC, v.sort_order ASC NULLS FIRST, v.name ASC NULLS FIRST;

-- name: InsertStockMovement :one
INSERT INTO stock_movements (
    organization_id, branch_id, product_id, variant_id, kind, quantity, balance_after, unit_price, unit_cost,
    staff_id, counterpart_branch_id, transfer_uuid, reason, note, source, source_uuid, occurred_at, created_by
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(branch_id), sqlc.arg(product_id), sqlc.narg(variant_id), sqlc.arg(kind),
    sqlc.arg(quantity), sqlc.arg(balance_after), sqlc.narg(unit_price), sqlc.narg(unit_cost), sqlc.narg(staff_id),
    sqlc.narg(counterpart_branch_id), sqlc.narg(transfer_uuid), sqlc.arg(reason), sqlc.arg(note), sqlc.arg(source),
    sqlc.narg(source_uuid), sqlc.arg(occurred_at), sqlc.narg(created_by)
)
RETURNING *;

-- name: GetStockMovement :one
SELECT
    m.*,
    p.uuid AS product_uuid, p.name AS product_name, p.sku AS product_sku,
    v.uuid AS variant_uuid, v.name AS variant_name, v.sku AS variant_sku,
    b.uuid AS branch_uuid, b.name AS branch_name,
    cb.uuid AS counterpart_branch_uuid, cb.name AS counterpart_branch_name,
    st.uuid AS staff_uuid, st.name AS staff_name,
    u.uuid AS created_by_uuid, u.name AS created_by_name, u.surname AS created_by_surname
FROM stock_movements m
JOIN products p ON p.id = m.product_id AND p.organization_id = m.organization_id
JOIN branches b ON b.id = m.branch_id AND b.organization_id = m.organization_id
LEFT JOIN product_variants v ON v.id = m.variant_id AND v.organization_id = m.organization_id
LEFT JOIN branches cb ON cb.id = m.counterpart_branch_id AND cb.organization_id = m.organization_id
LEFT JOIN staff st ON st.id = m.staff_id AND st.organization_id = m.organization_id
LEFT JOIN users u ON u.id = m.created_by
WHERE m.organization_id = sqlc.arg(organization_id) AND m.uuid = sqlc.arg(uuid);

-- name: ListStockMovements :many
SELECT
    m.*,
    p.uuid AS product_uuid, p.name AS product_name, p.sku AS product_sku,
    v.uuid AS variant_uuid, v.name AS variant_name, v.sku AS variant_sku,
    b.uuid AS branch_uuid, b.name AS branch_name,
    cb.uuid AS counterpart_branch_uuid, cb.name AS counterpart_branch_name,
    st.uuid AS staff_uuid, st.name AS staff_name,
    u.uuid AS created_by_uuid, u.name AS created_by_name, u.surname AS created_by_surname
FROM stock_movements m
JOIN products p ON p.id = m.product_id AND p.organization_id = m.organization_id
JOIN branches b ON b.id = m.branch_id AND b.organization_id = m.organization_id
LEFT JOIN product_variants v ON v.id = m.variant_id AND v.organization_id = m.organization_id
LEFT JOIN branches cb ON cb.id = m.counterpart_branch_id AND cb.organization_id = m.organization_id
LEFT JOIN staff st ON st.id = m.staff_id AND st.organization_id = m.organization_id
LEFT JOIN users u ON u.id = m.created_by
WHERE m.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR m.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR m.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(product_ids)::bigint[] IS NULL OR m.product_id = ANY(sqlc.narg(product_ids)::bigint[]))
  AND (sqlc.narg(staff_ids)::bigint[] IS NULL OR m.staff_id = ANY(sqlc.narg(staff_ids)::bigint[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR m.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(occurred_from)::timestamptz IS NULL OR m.occurred_at >= sqlc.narg(occurred_from)::timestamptz)
  AND (sqlc.narg(occurred_to)::timestamptz IS NULL OR m.occurred_at < sqlc.narg(occurred_to)::timestamptz)
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.name ILIKE '%' || sqlc.narg(q) || '%'
    OR p.sku ILIKE '%' || sqlc.narg(q) || '%'
    OR v.sku ILIKE '%' || sqlc.narg(q) || '%'
    OR m.reason ILIKE '%' || sqlc.narg(q) || '%'
    OR m.note ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'occurred_at' AND NOT sqlc.arg(sort_desc)::boolean THEN m.occurred_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'quantity' AND NOT sqlc.arg(sort_desc)::boolean THEN m.quantity END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'quantity' AND sqlc.arg(sort_desc)::boolean THEN m.quantity END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'kind' AND NOT sqlc.arg(sort_desc)::boolean THEN m.kind END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'kind' AND sqlc.arg(sort_desc)::boolean THEN m.kind END DESC,
  m.occurred_at DESC, m.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountStockMovements :one
SELECT COUNT(*)::bigint
FROM stock_movements m
JOIN products p ON p.id = m.product_id AND p.organization_id = m.organization_id
LEFT JOIN product_variants v ON v.id = m.variant_id AND v.organization_id = m.organization_id
WHERE m.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR m.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR m.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[]))
  AND (sqlc.narg(product_ids)::bigint[] IS NULL OR m.product_id = ANY(sqlc.narg(product_ids)::bigint[]))
  AND (sqlc.narg(staff_ids)::bigint[] IS NULL OR m.staff_id = ANY(sqlc.narg(staff_ids)::bigint[]))
  AND (sqlc.narg(kinds)::text[] IS NULL OR m.kind = ANY(sqlc.narg(kinds)::text[]))
  AND (sqlc.narg(occurred_from)::timestamptz IS NULL OR m.occurred_at >= sqlc.narg(occurred_from)::timestamptz)
  AND (sqlc.narg(occurred_to)::timestamptz IS NULL OR m.occurred_at < sqlc.narg(occurred_to)::timestamptz)
  AND (
    sqlc.narg(q)::text IS NULL
    OR p.name ILIKE '%' || sqlc.narg(q) || '%'
    OR p.sku ILIKE '%' || sqlc.narg(q) || '%'
    OR v.sku ILIKE '%' || sqlc.narg(q) || '%'
    OR m.reason ILIKE '%' || sqlc.narg(q) || '%'
    OR m.note ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListStockMovementKindFacets :many
SELECT m.kind AS value, COUNT(*)::bigint AS count
FROM stock_movements m
WHERE m.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR m.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY m.kind
ORDER BY m.kind ASC;

-- name: ListStockMovementBranchFacets :many
SELECT b.uuid, b.name, COUNT(m.id)::bigint AS count
FROM stock_movements m
JOIN branches b ON b.id = m.branch_id AND b.organization_id = m.organization_id
WHERE m.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR m.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY b.id, b.uuid, b.name
ORDER BY b.name ASC;

-- name: ListStockMovementStaffFacets :many
SELECT st.uuid, st.name, COUNT(m.id)::bigint AS count
FROM stock_movements m
JOIN staff st ON st.id = m.staff_id AND st.organization_id = m.organization_id
WHERE m.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR m.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY st.id, st.uuid, st.name
ORDER BY st.name ASC;

-- name: ListProductIDsByUUIDs :many
SELECT id, uuid FROM products
WHERE organization_id = sqlc.arg(organization_id) AND uuid = ANY(sqlc.arg(uuids)::uuid[]) AND deleted_at IS NULL;

-- name: ListProductSearchDocuments :many
SELECT p.id, p.uuid, p.organization_id, p.name, p.sku, p.barcode, o.slug AS organization_slug
FROM products p
JOIN organizations o ON o.id = p.organization_id
WHERE p.organization_id = sqlc.arg(organization_id)
  AND p.deleted_at IS NULL;

-- name: GetProductSearchDocument :one
SELECT p.id, p.uuid, p.organization_id, p.name, p.sku, p.barcode, o.slug AS organization_slug
FROM products p
JOIN organizations o ON o.id = p.organization_id
WHERE p.organization_id = sqlc.arg(organization_id) AND p.uuid = sqlc.arg(uuid) AND p.deleted_at IS NULL;
