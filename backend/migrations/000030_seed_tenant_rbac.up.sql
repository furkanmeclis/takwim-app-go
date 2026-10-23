INSERT INTO permissions (name, slug) VALUES
    ('Read platform organizations', 'platform.organizations.read'),
    ('Write platform organizations', 'platform.organizations.write'),
    ('Read tenant settings', 'tenant.settings.read'),
    ('Write tenant settings', 'tenant.settings.write'),
    ('Read tenant members', 'tenant.members.read'),
    ('Write tenant members', 'tenant.members.write'),
    ('Read tenant roles', 'tenant.roles.read'),
    ('Write tenant roles', 'tenant.roles.write')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('platform.organizations.read', 'platform.organizations.write')
WHERE r.slug = 'super_admin' AND r.scope = 'platform'
ON CONFLICT DO NOTHING;

INSERT INTO roles (name, slug, description, is_system, scope, organization_id) VALUES
    ('Organization Owner', 'owner', 'Tenant owner template role', true, 'tenant', NULL),
    ('Organization Admin', 'admin', 'Tenant admin template role', true, 'tenant', NULL),
    ('Organization Member', 'member', 'Tenant member template role', true, 'tenant', NULL)
ON CONFLICT (slug) WHERE scope = 'tenant' AND organization_id IS NULL DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug LIKE 'tenant.%'
WHERE r.slug = 'owner' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug LIKE 'tenant.%' AND p.slug <> 'tenant.roles.write'
WHERE r.slug = 'admin' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.settings.read', 'tenant.members.read')
WHERE r.slug = 'member' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
