-- TEC-891: resources (kaynaklar). A resource is bookable capacity of one
-- branch besides staff time: a chair, a table, an area, a room, a device or
-- a wash bay (the sector decides which kinds the panel offers). Resources
-- follow the branch's opening hours unless they have their own, and can be
-- closed for periods (maintenance, private event).

CREATE TABLE resources (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    branch_id BIGINT NOT NULL,
    kind VARCHAR(16) NOT NULL CHECK (kind IN ('chair', 'table', 'area', 'room', 'device', 'bay')),
    name VARCHAR(120) NOT NULL,
    -- Area / zone inside the branch (restaurant: iç salon, bahçe, teras).
    zone VARCHAR(64) NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    -- People the resource serves at once (table: party size range).
    min_capacity INT NOT NULL DEFAULT 1,
    capacity INT NOT NULL DEFAULT 1,
    -- Tables that can be joined with a neighbouring table for larger parties.
    combinable BOOLEAN NOT NULL DEFAULT FALSE,
    -- Customers may pick it on the booking screens.
    show_online BOOLEAN NOT NULL DEFAULT TRUE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INT NOT NULL DEFAULT 0,
    -- Own weekly hours ({"mon": {"closed": false, "open": "09:00", "close": "18:00"}, ...});
    -- NULL = the branch's hours.
    working_hours JSONB NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT ck_resources_capacity CHECK (min_capacity >= 1 AND capacity >= min_capacity AND capacity <= 1000),
    CONSTRAINT uq_resources_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_resources_organization_id UNIQUE (organization_id, id),
    CONSTRAINT fk_resources_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id) ON DELETE CASCADE
);

-- A name is unique among the live resources of a branch.
CREATE UNIQUE INDEX uq_resources_branch_name
    ON resources (organization_id, branch_id, lower(name))
    WHERE deleted_at IS NULL;

CREATE INDEX idx_resources_organization_branch
    ON resources (organization_id, branch_id, sort_order)
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_resources_updated_at
    BEFORE UPDATE ON resources
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Closed periods of a resource (half-open [starts_at, ends_at)).
CREATE TABLE resource_closures (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    resource_id BIGINT NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_resource_closures_range CHECK (ends_at > starts_at),
    CONSTRAINT uq_resource_closures_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_resource_closures_resource FOREIGN KEY (organization_id, resource_id)
        REFERENCES resources (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_resource_closures_resource ON resource_closures (organization_id, resource_id, starts_at);

CREATE TRIGGER trg_resource_closures_updated_at
    BEFORE UPDATE ON resource_closures
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO permissions (name, slug) VALUES
    ('Read resources', 'tenant.resources.read'),
    ('Manage resources', 'tenant.resources.write'),
    ('Export resources', 'tenant.resources.export'),
    ('Import resources', 'tenant.resources.import'),
    ('Bulk activate resources', 'tenant.resources.bulk.activate'),
    ('Bulk deactivate resources', 'tenant.resources.bulk.deactivate')
ON CONFLICT (slug) DO NOTHING;

-- Owner and admin: everything. Branch manager: the resources of the
-- assigned branches (read, write, export). Reception: read (seating,
-- booking).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug IN (
    'tenant.resources.read', 'tenant.resources.write', 'tenant.resources.export', 'tenant.resources.import',
    'tenant.resources.bulk.activate', 'tenant.resources.bulk.deactivate')
WHERE r.slug IN ('owner', 'admin') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug IN (
    'tenant.resources.read', 'tenant.resources.write', 'tenant.resources.export')
WHERE r.slug = 'sube_yoneticisi' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug = 'tenant.resources.read'
WHERE r.slug = 'resepsiyon' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
