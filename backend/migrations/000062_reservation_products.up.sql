-- TEC-919: products added to a reservation and their stock reservation
-- (requirements §12, A18; docs/design/reservation-products.md).
--
-- A reservation product is one product (or variant) the customer takes
-- with the visit: quantity, the unit price and cost as they were when it
-- was added (snapshots) and the staff member credited with the sale.
--
-- Stock: while a line is held its units count in stock_levels.reserved of
-- the visit's branch (available = on_hand - reserved), written in the same
-- transaction as the line after locking the level row, so the last unit is
-- never reserved twice. The line ends in one of two ways:
--   delivered: the visit was completed; the reserved units left the stock
--              as one sale movement (stock_movement_uuid) and became one
--              product sale line of the visit (TEC-916), once;
--   released:  the line was removed, or the visit was cancelled, rejected,
--              expired or marked as a no-show; the units are free again.
-- An online payment never delivers a product.

CREATE TABLE reservation_products (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    reservation_id BIGINT NOT NULL,
    -- The stock is held in the visit's branch.
    branch_id BIGINT NOT NULL,
    product_id BIGINT NOT NULL,
    variant_id BIGINT NULL,
    -- Snapshots: what the customer was offered.
    product_name VARCHAR(160) NOT NULL,
    variant_name VARCHAR(120) NULL,
    sku VARCHAR(64) NOT NULL,
    quantity INT NOT NULL CHECK (quantity > 0 AND quantity <= 1000),
    unit_price NUMERIC(14, 2) NOT NULL CHECK (unit_price >= 0),
    unit_cost NUMERIC(14, 2) NOT NULL DEFAULT 0 CHECK (unit_cost >= 0),
    -- Staff member credited with the sale (ürün primi); NULL = none.
    staff_id BIGINT NULL,
    status VARCHAR(12) NOT NULL DEFAULT 'held' CHECK (status IN ('held', 'delivered', 'released')),
    -- removed, cancelled, rejected, expired, no_show.
    release_reason VARCHAR(32) NULL,
    stock_movement_uuid UUID NULL,
    -- Who added it: panel staff or the customer on the booking page.
    source VARCHAR(16) NOT NULL CHECK (source IN ('panel', 'online', 'embed')),
    delivered_at TIMESTAMPTZ NULL,
    released_at TIMESTAMPTZ NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_reservation_products_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT ck_reservation_products_delivered CHECK (
        (status = 'delivered') = (stock_movement_uuid IS NOT NULL AND delivered_at IS NOT NULL)
    ),
    CONSTRAINT ck_reservation_products_released CHECK (
        (status = 'released') = (released_at IS NOT NULL AND release_reason IS NOT NULL)
    ),
    CONSTRAINT fk_reservation_products_reservation FOREIGN KEY (organization_id, reservation_id)
        REFERENCES reservations (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_reservation_products_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id),
    CONSTRAINT fk_reservation_products_product FOREIGN KEY (organization_id, product_id)
        REFERENCES products (organization_id, id),
    CONSTRAINT fk_reservation_products_variant FOREIGN KEY (organization_id, product_id, variant_id)
        REFERENCES product_variants (organization_id, product_id, id),
    CONSTRAINT fk_reservation_products_staff FOREIGN KEY (organization_id, staff_id)
        REFERENCES staff (organization_id, id) ON DELETE SET NULL (staff_id),
    CONSTRAINT fk_reservation_products_movement FOREIGN KEY (organization_id, stock_movement_uuid)
        REFERENCES stock_movements (organization_id, uuid)
);

CREATE INDEX idx_reservation_products_reservation ON reservation_products (organization_id, reservation_id, id);
CREATE INDEX idx_reservation_products_held ON reservation_products (organization_id, branch_id, product_id, variant_id)
    WHERE status = 'held';

CREATE TRIGGER trg_reservation_products_updated_at
    BEFORE UPDATE ON reservation_products
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- A finished line stays as it is (history of the stock and the sale).
CREATE FUNCTION reservation_products_final() RETURNS trigger AS $$
BEGIN
    IF OLD.status <> 'held' AND (NEW.status <> OLD.status OR NEW.quantity <> OLD.quantity
        OR NEW.unit_price <> OLD.unit_price OR NEW.stock_movement_uuid IS DISTINCT FROM OLD.stock_movement_uuid) THEN
        RAISE EXCEPTION 'a delivered or released reservation product is final'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'reservation_products_final';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_reservation_products_final
    BEFORE UPDATE ON reservation_products
    FOR EACH ROW EXECUTE FUNCTION reservation_products_final();
