-- Data-loss note: drops the whole service catalog (categories, services,
-- branch offers, staff competences) and the tenant.services.* permissions.
-- Image objects stay in storage.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE slug IN (
        'tenant.services.read',
        'tenant.services.write',
        'tenant.services.offer',
        'tenant.services.export',
        'tenant.services.import',
        'tenant.services.bulk.activate',
        'tenant.services.bulk.deactivate'
    )
);

DELETE FROM permissions
WHERE slug IN (
    'tenant.services.read',
    'tenant.services.write',
    'tenant.services.offer',
    'tenant.services.export',
    'tenant.services.import',
    'tenant.services.bulk.activate',
    'tenant.services.bulk.deactivate'
);

DROP TABLE IF EXISTS service_staff;
DROP TABLE IF EXISTS service_branches;
DROP TABLE IF EXISTS services;
DROP TABLE IF EXISTS service_categories;
