-- Data-loss note: drops every resource and closed period, and the
-- tenant.resources.* permissions.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE slug IN (
        'tenant.resources.read',
        'tenant.resources.write',
        'tenant.resources.export',
        'tenant.resources.import',
        'tenant.resources.bulk.activate',
        'tenant.resources.bulk.deactivate'
    )
);

DELETE FROM permissions
WHERE slug IN (
    'tenant.resources.read',
    'tenant.resources.write',
    'tenant.resources.export',
    'tenant.resources.import',
    'tenant.resources.bulk.activate',
    'tenant.resources.bulk.deactivate'
);

DROP TABLE IF EXISTS resource_closures;
DROP TABLE IF EXISTS resources;
