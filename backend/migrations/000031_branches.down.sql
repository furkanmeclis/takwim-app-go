-- Data-loss note: drops every branch and service point and the tenant.branches.* permissions.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE slug IN (
        'tenant.branches.read',
        'tenant.branches.write',
        'tenant.branches.export',
        'tenant.branches.import',
        'tenant.branches.bulk.activate',
        'tenant.branches.bulk.deactivate'
    )
);

DELETE FROM permissions
WHERE slug IN (
    'tenant.branches.read',
    'tenant.branches.write',
    'tenant.branches.export',
    'tenant.branches.import',
    'tenant.branches.bulk.activate',
    'tenant.branches.bulk.deactivate'
);

DROP TABLE IF EXISTS service_points;
DROP TABLE IF EXISTS branches;
