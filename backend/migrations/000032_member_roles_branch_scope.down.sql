-- Data-loss note: drops extra member roles (only the primary role_id is kept),
-- branch selections (every member gets all branches back), the Rentopia
-- template roles and the sensitive-action permissions. Members whose primary
-- role is one of the removed template roles fall back to 'member'.

DROP TABLE IF EXISTS organization_member_branches;

DROP TRIGGER IF EXISTS trg_organization_members_primary_role_sync ON organization_members;
DROP FUNCTION IF EXISTS organization_members_primary_role_sync();
DROP TRIGGER IF EXISTS trg_organization_member_roles_scope_check ON organization_member_roles;
DROP FUNCTION IF EXISTS organization_member_roles_scope_check();
DROP TABLE IF EXISTS organization_member_roles;

ALTER TABLE organization_members
    DROP CONSTRAINT IF EXISTS uq_organization_members_organization_id,
    DROP CONSTRAINT IF EXISTS chk_organization_members_branch_scope,
    DROP COLUMN IF EXISTS branch_scope;

UPDATE organization_members om
SET role_id = (
    SELECT id FROM roles
    WHERE slug = 'member' AND scope = 'tenant' AND organization_id IS NULL
)
WHERE om.role_id IN (
    SELECT id FROM roles
    WHERE slug IN ('merkez_yoneticisi', 'sube_yoneticisi', 'rezervasyon_satis', 'muhasebe', 'teslim_iade')
      AND scope = 'tenant' AND organization_id IS NULL
);

UPDATE organization_invitations oi
SET role_id = (
    SELECT id FROM roles
    WHERE slug = 'member' AND scope = 'tenant' AND organization_id IS NULL
)
WHERE oi.role_id IN (
    SELECT id FROM roles
    WHERE slug IN ('merkez_yoneticisi', 'sube_yoneticisi', 'rezervasyon_satis', 'muhasebe', 'teslim_iade')
      AND scope = 'tenant' AND organization_id IS NULL
);

DELETE FROM roles
WHERE slug IN ('merkez_yoneticisi', 'sube_yoneticisi', 'rezervasyon_satis', 'muhasebe', 'teslim_iade')
  AND scope = 'tenant' AND organization_id IS NULL;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE slug IN (
        'tenant.price.override', 'tenant.discount.apply', 'tenant.payment.collect',
        'tenant.payment.refund', 'tenant.damage.approve', 'tenant.signature.waive',
        'tenant.document.download', 'tenant.report.export'
    )
);

DELETE FROM permissions
WHERE slug IN (
    'tenant.price.override', 'tenant.discount.apply', 'tenant.payment.collect',
    'tenant.payment.refund', 'tenant.damage.approve', 'tenant.signature.waive',
    'tenant.document.download', 'tenant.report.export'
);
