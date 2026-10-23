-- TEC-889: Takwim tenant roles (requirements §4).
--
-- Template roles: owner (İşletme sahibi), admin (Yönetici, every branch, no
-- role management), sube_yoneticisi (Şube yöneticisi), resepsiyon
-- (Resepsiyon) and member (Çalışan). The reference app's rental roles go:
-- rezervasyon_satis is renamed to resepsiyon in place; merkez_yoneticisi
-- folds into admin, muhasebe into sube_yoneticisi and teslim_iade into
-- member (memberships and open invitations move with them).
--
-- Permission slugs for the coming modules are seeded now so roles are
-- designed up front. An ".own" slug limits the action to the member's own
-- records (own reservations, own schedule, own earnings); the full slug
-- covers every record inside the member's branch scope.

INSERT INTO permissions (name, slug) VALUES
    ('Read reservations', 'tenant.reservations.read'),
    ('Manage reservations', 'tenant.reservations.write'),
    ('Manage own reservations', 'tenant.reservations.own'),
    ('Read schedules and resources', 'tenant.schedules.read'),
    ('Manage schedules and resources', 'tenant.schedules.write'),
    ('Read own schedule', 'tenant.schedules.own'),
    ('Read customers', 'tenant.customers.read'),
    ('Manage customers', 'tenant.customers.write'),
    ('Read revenue, cash and expenses', 'tenant.finance.read'),
    ('Manage cash and expenses', 'tenant.finance.write'),
    ('Read staff earnings', 'tenant.earnings.read'),
    ('Read own earnings', 'tenant.earnings.own'),
    ('Read integrations', 'tenant.integrations.read')
ON CONFLICT (slug) DO NOTHING;

-- Rental-only actions of the reference app.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN ('tenant.damage.approve', 'tenant.signature.waive')
);
DELETE FROM permissions WHERE slug IN ('tenant.damage.approve', 'tenant.signature.waive');

-- Move memberships and invitations from one template role to another, then
-- drop the old template.
CREATE FUNCTION pg_temp.fold_template_role(old_slug TEXT, new_slug TEXT) RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
    old_id BIGINT;
    new_id BIGINT;
BEGIN
    SELECT id INTO old_id FROM roles WHERE slug = old_slug AND scope = 'tenant' AND organization_id IS NULL;
    SELECT id INTO new_id FROM roles WHERE slug = new_slug AND scope = 'tenant' AND organization_id IS NULL;
    IF old_id IS NULL OR new_id IS NULL THEN
        RETURN;
    END IF;
    INSERT INTO organization_member_roles (organization_id, member_id, role_id)
    SELECT organization_id, member_id, new_id FROM organization_member_roles WHERE role_id = old_id
    ON CONFLICT DO NOTHING;
    DELETE FROM organization_member_roles WHERE role_id = old_id;
    UPDATE organization_members SET role_id = new_id WHERE role_id = old_id;
    INSERT INTO organization_invitation_roles (organization_id, invitation_id, role_id)
    SELECT organization_id, invitation_id, new_id FROM organization_invitation_roles WHERE role_id = old_id
    ON CONFLICT DO NOTHING;
    DELETE FROM organization_invitation_roles WHERE role_id = old_id;
    UPDATE organization_invitations SET role_id = new_id WHERE role_id = old_id;
    DELETE FROM role_permissions WHERE role_id = old_id;
    DELETE FROM roles WHERE id = old_id;
END;
$$;

UPDATE roles
SET slug = 'resepsiyon', name = 'Resepsiyon', description = 'Reception: bookings, customers and payments of the assigned branches'
WHERE slug = 'rezervasyon_satis' AND scope = 'tenant' AND organization_id IS NULL;

INSERT INTO roles (name, slug, description, is_system, scope, organization_id) VALUES
    ('Resepsiyon', 'resepsiyon', 'Reception: bookings, customers and payments of the assigned branches', true, 'tenant', NULL),
    ('Şube Yöneticisi', 'sube_yoneticisi', 'Branch manager: everything inside the assigned branches', true, 'tenant', NULL)
ON CONFLICT (slug) WHERE scope = 'tenant' AND organization_id IS NULL DO NOTHING;

SELECT pg_temp.fold_template_role('merkez_yoneticisi', 'admin');
SELECT pg_temp.fold_template_role('muhasebe', 'sube_yoneticisi');
SELECT pg_temp.fold_template_role('teslim_iade', 'member');

UPDATE roles SET name = 'İşletme Sahibi', description = 'Business owner: every permission'
WHERE slug = 'owner' AND scope = 'tenant' AND organization_id IS NULL;
UPDATE roles SET name = 'Yönetici', description = 'Manager of every branch; no role management'
WHERE slug = 'admin' AND scope = 'tenant' AND organization_id IS NULL;
UPDATE roles SET name = 'Şube Yöneticisi', description = 'Branch manager: everything inside the assigned branches'
WHERE slug = 'sube_yoneticisi' AND scope = 'tenant' AND organization_id IS NULL;
UPDATE roles SET name = 'Çalışan', description = 'Staff: own reservations, own schedule and own earnings'
WHERE slug = 'member' AND scope = 'tenant' AND organization_id IS NULL;

-- Template permission sets are rebuilt from scratch.
DELETE FROM role_permissions
WHERE role_id IN (
    SELECT id FROM roles
    WHERE scope = 'tenant' AND organization_id IS NULL
      AND slug IN ('owner', 'admin', 'sube_yoneticisi', 'resepsiyon', 'member')
);

-- owner: every tenant permission.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug LIKE 'tenant.%'
WHERE r.slug = 'owner' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- admin: every tenant permission except role management.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug LIKE 'tenant.%' AND p.slug <> 'tenant.roles.write'
WHERE r.slug = 'admin' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Şube yöneticisi: everything of the assigned branches; module settings,
-- integrations and roles read-only; price override and refunds only when
-- the owner grants them through a custom role.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug IN (
    'tenant.settings.read', 'tenant.members.read', 'tenant.roles.read',
    'tenant.branches.read', 'tenant.branches.write', 'tenant.branches.export',
    'tenant.branches.bulk.activate', 'tenant.branches.bulk.deactivate',
    'tenant.integrations.read',
    'tenant.reservations.read', 'tenant.reservations.write',
    'tenant.schedules.read', 'tenant.schedules.write',
    'tenant.customers.read', 'tenant.customers.write',
    'tenant.finance.read', 'tenant.finance.write',
    'tenant.earnings.read',
    'tenant.discount.apply', 'tenant.payment.collect',
    'tenant.document.download', 'tenant.report.export'
)
WHERE r.slug = 'sube_yoneticisi' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Resepsiyon: bookings and customers of the assigned branches, schedules
-- read-only, payments it is allowed to collect; no earnings, no finance.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug IN (
    'tenant.settings.read', 'tenant.branches.read',
    'tenant.reservations.read', 'tenant.reservations.write',
    'tenant.schedules.read',
    'tenant.customers.read', 'tenant.customers.write',
    'tenant.payment.collect', 'tenant.document.download'
)
WHERE r.slug = 'resepsiyon' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Çalışan: own reservations (managed directly, no approval flow), own
-- schedule, own earnings and payments on its own reservations (K-002,
-- K-003). Never other staff's earnings or the business totals.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug IN (
    'tenant.settings.read', 'tenant.branches.read',
    'tenant.reservations.own', 'tenant.schedules.own', 'tenant.earnings.own',
    'tenant.payment.collect'
)
WHERE r.slug = 'member' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Integration lists are read with tenant.integrations.read from now on:
-- every role that could manage them (custom roles included) keeps seeing them.
INSERT INTO role_permissions (role_id, permission_id)
SELECT rp.role_id, rd.id
FROM role_permissions rp
JOIN permissions mg ON mg.id = rp.permission_id AND mg.slug = 'tenant.integrations.manage'
CROSS JOIN permissions rd
WHERE rd.slug = 'tenant.integrations.read'
ON CONFLICT DO NOTHING;
