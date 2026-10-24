-- TEC-919: products of a reservation and their stock reservation. Every
-- query takes the organization id.

-- name: GetReservationProductPick :one
-- A product (and, when named, one of its variants) as it can be added to a
-- visit in a branch: catalog state, prices, the branch offer and stock.
SELECT
    p.id, p.uuid, p.name, p.sku, p.price, p.cost, p.is_active, p.show_online,
    (SELECT COUNT(*) FROM product_variants pv
      WHERE pv.organization_id = p.organization_id AND pv.product_id = p.id AND pv.deleted_at IS NULL)::bigint AS variants_count,
    v.id AS variant_id, v.uuid AS variant_uuid, v.name AS variant_name, v.sku AS variant_sku,
    v.price AS variant_price, v.cost AS variant_cost, v.is_active AS variant_active,
    pb.is_active AS offered, pb.price AS branch_price
FROM products p
LEFT JOIN product_variants v
    ON v.organization_id = p.organization_id AND v.product_id = p.id
   AND v.uuid = sqlc.narg(variant_uuid)::uuid AND v.deleted_at IS NULL
LEFT JOIN product_branches pb
    ON pb.organization_id = p.organization_id AND pb.product_id = p.id AND pb.branch_id = sqlc.arg(branch_id)
WHERE p.organization_id = sqlc.arg(organization_id) AND p.uuid = sqlc.arg(product_uuid) AND p.deleted_at IS NULL;

-- name: ListReservationProductOptions :many
-- Stock items sold in a branch with their available units (on hand -
-- reserved): active products offered there, one row per active variant
-- (or the product itself without variants). online_only keeps the
-- products shown on the customer screens; in_stock_only drops items with
-- nothing available.
SELECT
    p.uuid AS product_uuid, p.name AS product_name, p.sku AS product_sku, p.image_uuid,
    v.uuid AS variant_uuid, v.name AS variant_name, v.sku AS variant_sku,
    COALESCE(v.price, pb.price, p.price)::numeric(14, 2) AS price,
    GREATEST(COALESCE(sl.on_hand, 0) - COALESCE(sl.reserved, 0), 0)::bigint AS available
FROM products p
JOIN product_branches pb
    ON pb.organization_id = p.organization_id AND pb.product_id = p.id
   AND pb.branch_id = sqlc.arg(branch_id) AND pb.is_active
LEFT JOIN product_variants v
    ON v.organization_id = p.organization_id AND v.product_id = p.id AND v.deleted_at IS NULL AND v.is_active
LEFT JOIN stock_levels sl
    ON sl.organization_id = p.organization_id AND sl.branch_id = pb.branch_id AND sl.product_id = p.id
   AND sl.variant_id IS NOT DISTINCT FROM v.id
WHERE p.organization_id = sqlc.arg(organization_id) AND p.deleted_at IS NULL AND p.is_active
  AND (NOT sqlc.arg(online_only)::boolean OR p.show_online)
  -- A product with variants is sold as one of them.
  AND (v.id IS NOT NULL OR NOT EXISTS (
        SELECT 1 FROM product_variants pv
         WHERE pv.organization_id = p.organization_id AND pv.product_id = p.id AND pv.deleted_at IS NULL))
  AND (NOT sqlc.arg(in_stock_only)::boolean OR COALESCE(sl.on_hand, 0) - COALESCE(sl.reserved, 0) > 0)
  AND (sqlc.narg(q)::text IS NULL
       OR p.name ILIKE '%' || sqlc.narg(q)::text || '%' OR p.sku ILIKE '%' || sqlc.narg(q)::text || '%'
       OR p.barcode = sqlc.narg(q)::text
       OR v.name ILIKE '%' || sqlc.narg(q)::text || '%' OR v.sku ILIKE '%' || sqlc.narg(q)::text || '%'
       OR v.barcode = sqlc.narg(q)::text)
ORDER BY p.name, p.id, v.sort_order, v.name
LIMIT sqlc.arg(limit_count);

-- name: SetStockLevelReserved :exec
UPDATE stock_levels SET reserved = sqlc.arg(reserved)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: SetStockLevelStock :exec
UPDATE stock_levels SET on_hand = sqlc.arg(on_hand), reserved = sqlc.arg(reserved)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: InsertReservationProduct :one
INSERT INTO reservation_products (
    organization_id, reservation_id, branch_id, product_id, variant_id, product_name, variant_name, sku,
    quantity, unit_price, unit_cost, staff_id, source, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(reservation_id), sqlc.arg(branch_id), sqlc.arg(product_id),
    sqlc.narg(variant_id), sqlc.arg(product_name), sqlc.narg(variant_name), sqlc.arg(sku), sqlc.arg(quantity),
    sqlc.arg(unit_price), sqlc.arg(unit_cost), sqlc.narg(staff_id), sqlc.arg(source), sqlc.narg(created_by)
)
RETURNING *;

-- name: ListReservationProducts :many
SELECT rp.*, p.uuid AS product_uuid, v.uuid AS variant_uuid, st.uuid AS staff_uuid, st.name AS staff_name
FROM reservation_products rp
JOIN products p ON p.organization_id = rp.organization_id AND p.id = rp.product_id
LEFT JOIN product_variants v ON v.organization_id = rp.organization_id AND v.id = rp.variant_id
LEFT JOIN staff st ON st.organization_id = rp.organization_id AND st.id = rp.staff_id
WHERE rp.organization_id = sqlc.arg(organization_id) AND rp.reservation_id = sqlc.arg(reservation_id)
ORDER BY rp.id;

-- name: ListHeldReservationProducts :many
-- Held lines of a visit, locked, in stock-level lock order.
SELECT * FROM reservation_products
WHERE organization_id = sqlc.arg(organization_id) AND reservation_id = sqlc.arg(reservation_id) AND status = 'held'
ORDER BY branch_id, product_id, variant_id NULLS FIRST, id
FOR UPDATE;

-- name: GetReservationProductForUpdate :one
SELECT * FROM reservation_products
WHERE organization_id = sqlc.arg(organization_id) AND reservation_id = sqlc.arg(reservation_id)
  AND uuid = sqlc.arg(uuid)
FOR UPDATE;

-- name: ReleaseReservationProduct :exec
UPDATE reservation_products
SET status = 'released', release_reason = sqlc.arg(release_reason), released_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'held';

-- name: DeliverReservationProduct :exec
UPDATE reservation_products
SET status = 'delivered', stock_movement_uuid = sqlc.arg(stock_movement_uuid), delivered_at = sqlc.arg(delivered_at)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'held';

-- name: SumReservationProducts :one
-- What the visit's products add to its total (held and delivered lines).
SELECT COALESCE(SUM(unit_price * quantity), 0)::numeric(14, 2) AS total
FROM reservation_products
WHERE organization_id = sqlc.arg(organization_id) AND reservation_id = sqlc.arg(reservation_id)
  AND status IN ('held', 'delivered');

-- name: SumReservationLinePrices :one
-- Lines served on a package session (TEC-920) are paid by the package.
SELECT COALESCE(SUM(rl.unit_price), 0)::numeric(14, 2) AS total
FROM reservation_lines rl
WHERE rl.organization_id = sqlc.arg(organization_id) AND rl.reservation_id = sqlc.arg(reservation_id)
  AND NOT EXISTS (
    SELECT 1 FROM package_sessions ps
    WHERE ps.organization_id = rl.organization_id AND ps.reservation_id = rl.reservation_id
      AND ps.line_index = rl.line_index AND ps.status <> 'released'
  );

-- name: UpdateReservationProductsTotal :one
-- A product added or removed: the new total and payment state.
UPDATE reservations
SET total_price = sqlc.arg(total_price), payment_status = sqlc.arg(payment_status), version = version + 1
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: GetReservationFirstStaff :one
-- The person serving the first staffed line (credited with products by
-- default).
SELECT staff_id::bigint FROM reservation_lines
WHERE organization_id = sqlc.arg(organization_id) AND reservation_id = sqlc.arg(reservation_id) AND staff_id IS NOT NULL
ORDER BY line_index
LIMIT 1;

-- name: SetReservationTotal :exec
-- The total of a new booking once its products are reserved.
UPDATE reservations SET total_price = sqlc.arg(total_price)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);
