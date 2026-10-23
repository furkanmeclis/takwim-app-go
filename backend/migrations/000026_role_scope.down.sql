-- Data-loss note: tenant roles (templates and org-specific) are deleted so
-- the global slug uniqueness can be restored.
DELETE FROM roles WHERE scope = 'tenant';

DROP INDEX IF EXISTS uq_roles_tenant_org_slug;
DROP INDEX IF EXISTS uq_roles_tenant_template_slug;
DROP INDEX IF EXISTS uq_roles_platform_slug;

ALTER TABLE roles
    DROP CONSTRAINT IF EXISTS chk_roles_scope_organization,
    DROP CONSTRAINT IF EXISTS chk_roles_scope,
    DROP COLUMN IF EXISTS organization_id,
    DROP COLUMN IF EXISTS scope,
    ADD CONSTRAINT uq_roles_slug UNIQUE (slug);
