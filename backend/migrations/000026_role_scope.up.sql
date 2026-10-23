ALTER TABLE roles
    ADD COLUMN scope VARCHAR(16) NOT NULL DEFAULT 'platform',
    ADD COLUMN organization_id BIGINT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    ADD CONSTRAINT chk_roles_scope CHECK (scope IN ('platform', 'tenant')),
    ADD CONSTRAINT chk_roles_scope_organization CHECK (
        (scope = 'platform' AND organization_id IS NULL)
        OR scope = 'tenant'
    );

ALTER TABLE roles
    DROP CONSTRAINT uq_roles_slug;

CREATE INDEX idx_roles_scope ON roles (scope);
CREATE INDEX idx_roles_organization_id ON roles (organization_id)
    WHERE organization_id IS NOT NULL;

CREATE UNIQUE INDEX uq_roles_platform_slug
    ON roles (slug)
    WHERE scope = 'platform';

CREATE UNIQUE INDEX uq_roles_tenant_template_slug
    ON roles (slug)
    WHERE scope = 'tenant' AND organization_id IS NULL;

CREATE UNIQUE INDEX uq_roles_tenant_org_slug
    ON roles (organization_id, slug)
    WHERE scope = 'tenant' AND organization_id IS NOT NULL;
