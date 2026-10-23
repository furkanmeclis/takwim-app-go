-- Multi-role memberships and branch-scoped access (TEC-672 / TEC-689).
--
-- organization_member_roles is the source of tenant permissions: a member's
-- permissions are the union over all of its roles. organization_members.role_id
-- is kept as the member's primary (display) role for compatibility: it is
-- always one of the member's roles and the usecase keeps it in sync.
--
-- organization_members.branch_scope = 'all' grants every branch of the
-- organization; 'selected' grants only the branches in
-- organization_member_branches.

ALTER TABLE organization_members
    ADD COLUMN branch_scope VARCHAR(16) NOT NULL DEFAULT 'all',
    ADD CONSTRAINT chk_organization_members_branch_scope CHECK (branch_scope IN ('all', 'selected')),
    -- Target of the composite foreign keys below.
    ADD CONSTRAINT uq_organization_members_organization_id UNIQUE (organization_id, id);

CREATE TABLE organization_member_roles (
    organization_id BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    member_id       BIGINT      NOT NULL,
    role_id         BIGINT      NOT NULL REFERENCES roles (id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT pk_organization_member_roles PRIMARY KEY (organization_id, member_id, role_id),
    CONSTRAINT fk_organization_member_roles_member
        FOREIGN KEY (organization_id, member_id)
        REFERENCES organization_members (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_organization_member_roles_organization_role
    ON organization_member_roles (organization_id, role_id);

CREATE OR REPLACE FUNCTION organization_member_roles_scope_check()
RETURNS trigger AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM roles r
        WHERE r.id = NEW.role_id
          AND r.scope = 'tenant'
          AND (r.organization_id IS NULL OR r.organization_id = NEW.organization_id)
    ) THEN
        RAISE EXCEPTION 'organization member role must be tenant-scoped for this organization'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_organization_member_roles_scope_check
    BEFORE INSERT OR UPDATE ON organization_member_roles
    FOR EACH ROW
    EXECUTE FUNCTION organization_member_roles_scope_check();

-- Every new membership starts with its primary role, whoever inserts it
-- (organization create, platform add member, invitation accept).
CREATE OR REPLACE FUNCTION organization_members_primary_role_sync()
RETURNS trigger AS $$
BEGIN
    INSERT INTO organization_member_roles (organization_id, member_id, role_id)
    VALUES (NEW.organization_id, NEW.id, NEW.role_id)
    ON CONFLICT DO NOTHING;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_organization_members_primary_role_sync
    AFTER INSERT ON organization_members
    FOR EACH ROW
    EXECUTE FUNCTION organization_members_primary_role_sync();

INSERT INTO organization_member_roles (organization_id, member_id, role_id)
SELECT organization_id, id, role_id
FROM organization_members
ON CONFLICT DO NOTHING;

CREATE TABLE organization_member_branches (
    organization_id BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    member_id       BIGINT      NOT NULL,
    branch_id       BIGINT      NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT pk_organization_member_branches PRIMARY KEY (organization_id, member_id, branch_id),
    CONSTRAINT fk_organization_member_branches_member
        FOREIGN KEY (organization_id, member_id)
        REFERENCES organization_members (organization_id, id) ON DELETE CASCADE,
    -- A member can only be granted branches of its own organization.
    CONSTRAINT fk_organization_member_branches_branch
        FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_organization_member_branches_organization_branch
    ON organization_member_branches (organization_id, branch_id);

-- Permissions for sensitive actions of later modules (pricing, payments,
-- damage, contracts, reports). Seeded now so roles can be designed up front.
INSERT INTO permissions (name, slug) VALUES
    ('Override prices', 'tenant.price.override'),
    ('Apply discounts', 'tenant.discount.apply'),
    ('Collect payments', 'tenant.payment.collect'),
    ('Refund payments', 'tenant.payment.refund'),
    ('Approve damage records', 'tenant.damage.approve'),
    ('Waive signatures', 'tenant.signature.waive'),
    ('Download documents', 'tenant.document.download'),
    ('Export reports', 'tenant.report.export')
ON CONFLICT (slug) DO NOTHING;

-- owner: every tenant permission; admin: every tenant permission except
-- role management (same rule as 000030).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.price.override', 'tenant.discount.apply', 'tenant.payment.collect',
    'tenant.payment.refund', 'tenant.damage.approve', 'tenant.signature.waive',
    'tenant.document.download', 'tenant.report.export'
)
WHERE r.slug IN ('owner', 'admin') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Rentopia template roles.
INSERT INTO roles (name, slug, description, is_system, scope, organization_id) VALUES
    ('Merkez Yöneticisi', 'merkez_yoneticisi', 'Head office manager: every tenant permission', true, 'tenant', NULL),
    ('Şube Yöneticisi', 'sube_yoneticisi', 'Branch manager: read and write on branch-scoped resources', true, 'tenant', NULL),
    ('Rezervasyon / Satış', 'rezervasyon_satis', 'Reservations and sales desk', true, 'tenant', NULL),
    ('Muhasebe', 'muhasebe', 'Accounting: payments, refunds and reports', true, 'tenant', NULL),
    ('Teslim / İade', 'teslim_iade', 'Vehicle hand-over and return staff', true, 'tenant', NULL)
ON CONFLICT (slug) WHERE scope = 'tenant' AND organization_id IS NULL DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug LIKE 'tenant.%'
WHERE r.slug = 'merkez_yoneticisi' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.settings.read', 'tenant.members.read', 'tenant.roles.read',
    'tenant.branches.read', 'tenant.branches.write', 'tenant.branches.export',
    'tenant.branches.bulk.activate', 'tenant.branches.bulk.deactivate',
    'tenant.price.override', 'tenant.discount.apply', 'tenant.payment.collect',
    'tenant.payment.refund', 'tenant.damage.approve', 'tenant.signature.waive',
    'tenant.document.download', 'tenant.report.export'
)
WHERE r.slug = 'sube_yoneticisi' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.settings.read', 'tenant.branches.read',
    'tenant.discount.apply', 'tenant.payment.collect', 'tenant.document.download'
)
WHERE r.slug = 'rezervasyon_satis' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.settings.read', 'tenant.branches.read', 'tenant.branches.export',
    'tenant.payment.collect', 'tenant.payment.refund',
    'tenant.document.download', 'tenant.report.export'
)
WHERE r.slug = 'muhasebe' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.settings.read', 'tenant.branches.read',
    'tenant.payment.collect', 'tenant.document.download'
)
WHERE r.slug = 'teslim_iade' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
