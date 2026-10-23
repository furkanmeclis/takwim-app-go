-- Data-loss note: drops the product catalog, variants, branch offers,
-- stock levels, the stock ledger and the tenant.products.* permissions.
-- Image objects stay in storage.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE slug IN (
        'tenant.products.read',
        'tenant.products.write',
        'tenant.products.offer',
        'tenant.products.stock',
        'tenant.products.sell',
        'tenant.products.export',
        'tenant.products.import',
        'tenant.products.bulk.activate',
        'tenant.products.bulk.deactivate'
    )
);

DELETE FROM permissions
WHERE slug IN (
    'tenant.products.read',
    'tenant.products.write',
    'tenant.products.offer',
    'tenant.products.stock',
    'tenant.products.sell',
    'tenant.products.export',
    'tenant.products.import',
    'tenant.products.bulk.activate',
    'tenant.products.bulk.deactivate'
);

DROP TABLE IF EXISTS stock_movements;
DROP FUNCTION IF EXISTS stock_movements_append_only();
DROP TABLE IF EXISTS stock_levels;
DROP TABLE IF EXISTS product_branches;
DROP TABLE IF EXISTS product_variants;
DROP TABLE IF EXISTS products;
