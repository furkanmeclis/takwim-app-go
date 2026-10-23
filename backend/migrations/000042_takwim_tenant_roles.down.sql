-- Restores the reference app's permission slugs and template roles with
-- empty memberships; folded memberships stay on the roles they moved to.
INSERT INTO permissions (name, slug) VALUES
    ('Approve damage records', 'tenant.damage.approve'),
    ('Waive signatures', 'tenant.signature.waive')
ON CONFLICT (slug) DO NOTHING;

UPDATE roles SET slug = 'rezervasyon_satis', name = 'Rezervasyon / Satış', description = 'Reservations and sales desk'
WHERE slug = 'resepsiyon' AND scope = 'tenant' AND organization_id IS NULL;

INSERT INTO roles (name, slug, description, is_system, scope, organization_id) VALUES
    ('Merkez Yöneticisi', 'merkez_yoneticisi', 'Head office manager: every tenant permission', true, 'tenant', NULL),
    ('Muhasebe', 'muhasebe', 'Accounting: payments, refunds and reports', true, 'tenant', NULL),
    ('Teslim / İade', 'teslim_iade', 'Vehicle hand-over and return staff', true, 'tenant', NULL)
ON CONFLICT (slug) WHERE scope = 'tenant' AND organization_id IS NULL DO NOTHING;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'tenant.reservations.read', 'tenant.reservations.write', 'tenant.reservations.own',
        'tenant.schedules.read', 'tenant.schedules.write', 'tenant.schedules.own',
        'tenant.customers.read', 'tenant.customers.write',
        'tenant.finance.read', 'tenant.finance.write',
        'tenant.earnings.read', 'tenant.earnings.own', 'tenant.integrations.read'
    )
);
DELETE FROM permissions WHERE slug IN (
    'tenant.reservations.read', 'tenant.reservations.write', 'tenant.reservations.own',
    'tenant.schedules.read', 'tenant.schedules.write', 'tenant.schedules.own',
    'tenant.customers.read', 'tenant.customers.write',
    'tenant.finance.read', 'tenant.finance.write',
    'tenant.earnings.read', 'tenant.earnings.own', 'tenant.integrations.read'
);
