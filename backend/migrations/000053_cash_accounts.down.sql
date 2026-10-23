-- Data-loss note: drops every cash / bank account, movement and daily
-- closing, and the tenant.cash.* permissions.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE slug IN (
        'tenant.cash.read',
        'tenant.cash.write',
        'tenant.cash.manage',
        'tenant.cash.close',
        'tenant.cash.reopen',
        'tenant.cash.export',
        'tenant.cash.bulk.archive',
        'tenant.cash.bulk.activate'
    )
);

DELETE FROM permissions
WHERE slug IN (
    'tenant.cash.read',
    'tenant.cash.write',
    'tenant.cash.manage',
    'tenant.cash.close',
    'tenant.cash.reopen',
    'tenant.cash.export',
    'tenant.cash.bulk.archive',
    'tenant.cash.bulk.activate'
);

DROP TABLE IF EXISTS cash_movements;
DROP FUNCTION IF EXISTS cash_movements_insert_rules();
DROP FUNCTION IF EXISTS cash_movements_append_only();
DROP TABLE IF EXISTS cash_closings;
DROP TABLE IF EXISTS cash_accounts;
