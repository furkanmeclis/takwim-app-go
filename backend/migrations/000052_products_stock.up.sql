-- TEC-918: products and stock (ürün kartı, varyant, şube stoğu, hareketler).
--
-- The product catalog lives at the business level (like services): name,
-- SKU, barcode, sale price, cost, cover image, variants with their own
-- stock codes. Each branch decides whether it offers a product and may
-- override its price (product_branches).
--
-- Stock is held per branch and stock item. A stock item is the product
-- itself (variant_id NULL) when it has no variants, otherwise one of its
-- variants. stock_movements is an append-only ledger (receipt, sale,
-- return, count adjustment, transfer legs); stock_levels keeps the running
-- balance and is written in the same transaction as each movement.
-- reserved is the seam of appointment reservations (TEC-919):
-- available = on_hand - reserved.

CREATE TABLE products (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name VARCHAR(160) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    sku VARCHAR(64) NOT NULL,
    barcode VARCHAR(64) NOT NULL DEFAULT '',
    unit VARCHAR(16) NOT NULL DEFAULT 'piece'
        CHECK (unit IN ('piece', 'bottle', 'box', 'pack', 'tube', 'set')),
    -- Sale price and unit cost in the organization currency.
    price NUMERIC(14, 2) NOT NULL DEFAULT 0,
    cost NUMERIC(14, 2) NOT NULL DEFAULT 0,
    -- Branch stock under this level shows as "low".
    low_stock_threshold INT NOT NULL DEFAULT 0,
    show_online BOOLEAN NOT NULL DEFAULT TRUE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    -- Cover image (API-owned media; the key never leaves the API).
    image_uuid UUID NULL,
    image_key TEXT NULL,
    image_content_type VARCHAR(64) NULL,
    image_size_bytes BIGINT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT ck_products_price CHECK (price >= 0 AND cost >= 0),
    CONSTRAINT ck_products_low_stock CHECK (low_stock_threshold >= 0),
    CONSTRAINT uq_products_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_products_organization_id UNIQUE (organization_id, id)
);

CREATE UNIQUE INDEX uq_products_sku
    ON products (organization_id, lower(sku))
    WHERE deleted_at IS NULL;

CREATE INDEX idx_products_organization_barcode
    ON products (organization_id, barcode)
    WHERE deleted_at IS NULL AND barcode <> '';

CREATE UNIQUE INDEX uq_products_image ON products (organization_id, image_uuid) WHERE image_uuid IS NOT NULL;

CREATE TRIGGER trg_products_updated_at
    BEFORE UPDATE ON products
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Variants (renk, boy, hacim …) with their own stock code; NULL price /
-- cost use the product values.
CREATE TABLE product_variants (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL,
    name VARCHAR(120) NOT NULL,
    sku VARCHAR(64) NOT NULL,
    barcode VARCHAR(64) NOT NULL DEFAULT '',
    price NUMERIC(14, 2) NULL CHECK (price IS NULL OR price >= 0),
    cost NUMERIC(14, 2) NULL CHECK (cost IS NULL OR cost >= 0),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_product_variants_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_product_variants_organization_id UNIQUE (organization_id, id),
    -- (product, id) pairs let stock rows reference a variant of their product.
    CONSTRAINT uq_product_variants_product_id UNIQUE (organization_id, product_id, id),
    CONSTRAINT fk_product_variants_product FOREIGN KEY (organization_id, product_id)
        REFERENCES products (organization_id, id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX uq_product_variants_sku
    ON product_variants (organization_id, lower(sku))
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX uq_product_variants_name
    ON product_variants (organization_id, product_id, lower(name))
    WHERE deleted_at IS NULL;

CREATE INDEX idx_product_variants_organization_barcode
    ON product_variants (organization_id, barcode)
    WHERE deleted_at IS NULL AND barcode <> '';

CREATE TRIGGER trg_product_variants_updated_at
    BEFORE UPDATE ON product_variants
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Branch offer: the product is sold in the branch; NULL price uses the
-- catalog price.
CREATE TABLE product_branches (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL,
    branch_id BIGINT NOT NULL,
    price NUMERIC(14, 2) NULL CHECK (price IS NULL OR price >= 0),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_product_branches UNIQUE (organization_id, product_id, branch_id),
    CONSTRAINT fk_product_branches_product FOREIGN KEY (organization_id, product_id)
        REFERENCES products (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_product_branches_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_product_branches_branch ON product_branches (organization_id, branch_id);

CREATE TRIGGER trg_product_branches_updated_at
    BEFORE UPDATE ON product_branches
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Running balance of one stock item in one branch. Written only together
-- with a stock_movements row (same transaction, row locked).
CREATE TABLE stock_levels (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    branch_id BIGINT NOT NULL,
    product_id BIGINT NOT NULL,
    variant_id BIGINT NULL,
    on_hand INT NOT NULL DEFAULT 0,
    -- Units held for appointments (TEC-919); never more than on_hand when
    -- they are placed.
    reserved INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Stock never goes negative (decision of TEC-918).
    CONSTRAINT ck_stock_levels_on_hand CHECK (on_hand >= 0),
    CONSTRAINT ck_stock_levels_reserved CHECK (reserved >= 0),
    CONSTRAINT uq_stock_levels UNIQUE NULLS NOT DISTINCT (organization_id, branch_id, product_id, variant_id),
    CONSTRAINT fk_stock_levels_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_stock_levels_product FOREIGN KEY (organization_id, product_id)
        REFERENCES products (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_stock_levels_variant FOREIGN KEY (organization_id, product_id, variant_id)
        REFERENCES product_variants (organization_id, product_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_stock_levels_product ON stock_levels (organization_id, product_id);

CREATE TRIGGER trg_stock_levels_updated_at
    BEFORE UPDATE ON stock_levels
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Append-only stock ledger. quantity is the signed change of on_hand;
-- balance_after the on_hand right after it. A transfer writes two rows
-- (transfer_out in the source, transfer_in in the target) with the same
-- transfer_uuid. source / source_uuid name what produced the row
-- ('manual' panel entry, 'import'; reservations later).
CREATE TABLE stock_movements (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    branch_id BIGINT NOT NULL,
    product_id BIGINT NOT NULL,
    variant_id BIGINT NULL,
    kind VARCHAR(16) NOT NULL
        CHECK (kind IN ('receipt', 'sale', 'return', 'adjustment', 'transfer_out', 'transfer_in')),
    quantity INT NOT NULL,
    balance_after INT NOT NULL CHECK (balance_after >= 0),
    -- Sale / return price and receipt cost per unit (snapshot).
    unit_price NUMERIC(14, 2) NULL CHECK (unit_price IS NULL OR unit_price >= 0),
    unit_cost NUMERIC(14, 2) NULL CHECK (unit_cost IS NULL OR unit_cost >= 0),
    -- Staff credited with a sale (ürün primi).
    staff_id BIGINT NULL,
    -- The other branch of a transfer.
    counterpart_branch_id BIGINT NULL,
    transfer_uuid UUID NULL,
    reason VARCHAR(200) NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    source VARCHAR(32) NOT NULL DEFAULT 'manual',
    source_uuid UUID NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_stock_movements_sign CHECK (
        (kind IN ('receipt', 'return', 'transfer_in') AND quantity > 0)
        OR (kind IN ('sale', 'transfer_out') AND quantity < 0)
        OR kind = 'adjustment'
    ),
    CONSTRAINT ck_stock_movements_transfer CHECK (
        (kind IN ('transfer_out', 'transfer_in')) = (transfer_uuid IS NOT NULL AND counterpart_branch_id IS NOT NULL)
    ),
    CONSTRAINT uq_stock_movements_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_stock_movements_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_stock_movements_counterpart FOREIGN KEY (organization_id, counterpart_branch_id)
        REFERENCES branches (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_stock_movements_product FOREIGN KEY (organization_id, product_id)
        REFERENCES products (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_stock_movements_variant FOREIGN KEY (organization_id, product_id, variant_id)
        REFERENCES product_variants (organization_id, product_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_stock_movements_staff FOREIGN KEY (organization_id, staff_id)
        REFERENCES staff (organization_id, id)
);

CREATE INDEX idx_stock_movements_branch ON stock_movements (organization_id, branch_id, occurred_at DESC);
CREATE INDEX idx_stock_movements_product ON stock_movements (organization_id, product_id, occurred_at DESC);
CREATE INDEX idx_stock_movements_staff ON stock_movements (organization_id, staff_id) WHERE staff_id IS NOT NULL;

-- The ledger is append-only: a correction is a new movement. The one
-- allowed update is created_by becoming NULL (the user was deleted).
CREATE FUNCTION stock_movements_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.created_by IS NULL AND to_jsonb(NEW) - 'created_by' = to_jsonb(OLD) - 'created_by' THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'stock_movements is append-only';
END;
$$;

CREATE TRIGGER trg_stock_movements_append_only
    BEFORE UPDATE ON stock_movements
    FOR EACH ROW EXECUTE FUNCTION stock_movements_append_only();

INSERT INTO permissions (name, slug) VALUES
    ('Read products and stock', 'tenant.products.read'),
    ('Manage the product catalog', 'tenant.products.write'),
    ('Manage branch offers of products', 'tenant.products.offer'),
    ('Record stock receipts, returns, counts and transfers', 'tenant.products.stock'),
    ('Record product sales', 'tenant.products.sell'),
    ('Export products', 'tenant.products.export'),
    ('Import products', 'tenant.products.import'),
    ('Bulk activate products', 'tenant.products.bulk.activate'),
    ('Bulk deactivate products', 'tenant.products.bulk.deactivate')
ON CONFLICT (slug) DO NOTHING;

-- Owner and admin: everything. Branch manager: reads the catalog, prices
-- and stocks its branches, sells, exports. Reception: reads and sells.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug IN (
    'tenant.products.read', 'tenant.products.write', 'tenant.products.offer', 'tenant.products.stock',
    'tenant.products.sell', 'tenant.products.export', 'tenant.products.import',
    'tenant.products.bulk.activate', 'tenant.products.bulk.deactivate')
WHERE r.slug IN ('owner', 'admin') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug IN (
    'tenant.products.read', 'tenant.products.offer', 'tenant.products.stock', 'tenant.products.sell',
    'tenant.products.export')
WHERE r.slug = 'sube_yoneticisi' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug IN ('tenant.products.read', 'tenant.products.sell')
WHERE r.slug = 'resepsiyon' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
