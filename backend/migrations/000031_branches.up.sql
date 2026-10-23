-- Branches and service points: the first organization-scoped list resource.
-- Every index starts with organization_id (tenant isolation guard rail).

CREATE TABLE branches (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    code VARCHAR(32) NOT NULL,
    name VARCHAR(255) NOT NULL,
    address TEXT NOT NULL DEFAULT '',
    city VARCHAR(128) NOT NULL DEFAULT '',
    district VARCHAR(128) NOT NULL DEFAULT '',
    latitude NUMERIC(9, 6) NULL CHECK (latitude BETWEEN -90 AND 90),
    longitude NUMERIC(9, 6) NULL CHECK (longitude BETWEEN -180 AND 180),
    phone VARCHAR(32) NOT NULL DEFAULT '',
    email VARCHAR(255) NOT NULL DEFAULT '',
    -- Weekly schedule: {"mon": {"closed": false, "open": "09:00", "close": "18:00"}, ..., "sun": {...}}
    working_hours JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- Dated overrides: [{"date": "2026-01-01", "closed": true, "open": "", "close": "", "note": ""}]
    holiday_exceptions JSONB NOT NULL DEFAULT '[]'::jsonb,
    prep_minutes INTEGER NOT NULL DEFAULT 60 CHECK (prep_minutes >= 0 AND prep_minutes <= 10080),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_branches_organization_uuid UNIQUE (organization_id, uuid),
    -- Target of the composite foreign key from service_points.
    CONSTRAINT uq_branches_organization_id UNIQUE (organization_id, id)
);

CREATE UNIQUE INDEX uq_branches_organization_code
    ON branches (organization_id, code)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_branches_organization_created
    ON branches (organization_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_branches_organization_city
    ON branches (organization_id, city)
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_branches_updated_at
    BEFORE UPDATE ON branches
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE service_points (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    branch_id BIGINT NOT NULL,
    type VARCHAR(16) NOT NULL CHECK (type IN ('office', 'airport', 'address_zone', 'hotel')),
    name VARCHAR(255) NOT NULL,
    -- Street address, or the zone description for address_zone points.
    description TEXT NOT NULL DEFAULT '',
    extra_fee NUMERIC(14, 2) NOT NULL DEFAULT 0 CHECK (extra_fee >= 0),
    -- NULL = use the branch working hours.
    working_hours JSONB NULL,
    online_selectable BOOLEAN NOT NULL DEFAULT TRUE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_service_points_organization_uuid UNIQUE (organization_id, uuid),
    -- A service point always belongs to a branch of the same organization.
    CONSTRAINT fk_service_points_branch
        FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_service_points_organization_branch
    ON service_points (organization_id, branch_id, created_at);

CREATE TRIGGER trg_service_points_updated_at
    BEFORE UPDATE ON service_points
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO permissions (name, slug) VALUES
    ('Read branches', 'tenant.branches.read'),
    ('Write branches', 'tenant.branches.write'),
    ('Export branches', 'tenant.branches.export'),
    ('Import branches', 'tenant.branches.import'),
    ('Bulk activate branches', 'tenant.branches.bulk.activate'),
    ('Bulk deactivate branches', 'tenant.branches.bulk.deactivate')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'tenant.branches.read',
    'tenant.branches.write',
    'tenant.branches.export',
    'tenant.branches.import',
    'tenant.branches.bulk.activate',
    'tenant.branches.bulk.deactivate'
)
WHERE r.slug IN ('owner', 'admin') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.branches.read'
WHERE r.slug = 'member' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
