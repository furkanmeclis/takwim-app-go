-- TEC-984: engines of the session packages (export of the sold packages,
-- bulk activate / deactivate of the definitions).
INSERT INTO permissions (name, slug) VALUES
    ('Export sold session packages', 'tenant.session_packages.export'),
    ('Bulk activate session packages', 'tenant.session_packages.bulk.activate'),
    ('Bulk deactivate session packages', 'tenant.session_packages.bulk.deactivate')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug IN (
    'tenant.session_packages.export', 'tenant.session_packages.bulk.activate',
    'tenant.session_packages.bulk.deactivate'
)
WHERE r.slug IN ('owner', 'admin') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug = 'tenant.session_packages.export'
WHERE r.slug = 'sube_yoneticisi' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
