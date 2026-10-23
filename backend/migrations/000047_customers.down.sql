-- Data-loss note: drops every business customer record, the shared customer
-- identities and their verified links, and the customer engine permissions
-- (tenant.customers.read / write stay; they come from 000042).
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE slug IN (
        'tenant.customers.export',
        'tenant.customers.import',
        'tenant.customers.bulk.tag',
        'tenant.customers.bulk.delete'
    )
);

DELETE FROM permissions
WHERE slug IN (
    'tenant.customers.export',
    'tenant.customers.import',
    'tenant.customers.bulk.tag',
    'tenant.customers.bulk.delete'
);

DROP TABLE IF EXISTS customer_identity_links;
DROP TABLE IF EXISTS customers;
DROP TABLE IF EXISTS customer_identities;
