-- Data-loss note: drops every staff record, branch assignment, shift and
-- time off, and the tenant.staff.* permissions.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE slug IN (
        'tenant.staff.read',
        'tenant.staff.write',
        'tenant.staff.export',
        'tenant.staff.import',
        'tenant.staff.bulk.activate',
        'tenant.staff.bulk.deactivate'
    )
);

DELETE FROM permissions
WHERE slug IN (
    'tenant.staff.read',
    'tenant.staff.write',
    'tenant.staff.export',
    'tenant.staff.import',
    'tenant.staff.bulk.activate',
    'tenant.staff.bulk.deactivate'
);

DROP TABLE IF EXISTS staff_time_off;
DROP TABLE IF EXISTS staff_branches;
DROP TABLE IF EXISTS staff;
