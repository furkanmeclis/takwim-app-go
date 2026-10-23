DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE slug IN (
        'platform.organizations.read',
        'platform.organizations.write',
        'tenant.settings.read',
        'tenant.settings.write',
        'tenant.members.read',
        'tenant.members.write',
        'tenant.roles.read',
        'tenant.roles.write'
    )
)
OR role_id IN (
    SELECT id FROM roles WHERE slug IN ('owner', 'admin', 'member') AND scope = 'tenant'
);

-- Data-loss note: removes seeded tenant template roles and seeded tenant/platform organization permissions.
DELETE FROM roles
WHERE slug IN ('owner', 'admin', 'member') AND scope = 'tenant';

DELETE FROM permissions
WHERE slug IN (
    'platform.organizations.read',
    'platform.organizations.write',
    'tenant.settings.read',
    'tenant.settings.write',
    'tenant.members.read',
    'tenant.members.write',
    'tenant.roles.read',
    'tenant.roles.write'
);
