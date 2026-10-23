-- TEC-890: staff (personel). A staff member is one person of the business
-- with one calendar: linked to a member account or without an account,
-- assigned to one or more branches with weekly shifts per branch, and with
-- time off that applies in every branch.

CREATE TABLE staff (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- The member account of the person; NULL = staff without an account.
    user_id BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    name VARCHAR(120) NOT NULL,
    title VARCHAR(120) NOT NULL DEFAULT '',
    bio TEXT NOT NULL DEFAULT '',
    phone VARCHAR(32) NOT NULL DEFAULT '',
    email VARCHAR(255) NOT NULL DEFAULT '',
    -- Calendar colour (#RRGGBB).
    color VARCHAR(7) NOT NULL DEFAULT '#2563EB' CHECK (color ~ '^#[0-9A-Fa-f]{6}$'),
    -- Listed on the customer booking screens.
    show_online BOOLEAN NOT NULL DEFAULT TRUE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_staff_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_staff_organization_id UNIQUE (organization_id, id)
);

-- One staff record per member account.
CREATE UNIQUE INDEX uq_staff_organization_user
    ON staff (organization_id, user_id)
    WHERE deleted_at IS NULL AND user_id IS NOT NULL;

CREATE INDEX idx_staff_organization_created
    ON staff (organization_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_staff_updated_at
    BEFORE UPDATE ON staff
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Branch assignments with the weekly shifts of the person in that branch:
-- {"mon": [{"start": "09:00", "end": "13:00", "breaks": [{"start": "11:00", "end": "11:15"}]}], ...}
-- The shifts of one person never overlap across branches (A04, checked by
-- the use case under a lock on the staff row).
CREATE TABLE staff_branches (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    staff_id BIGINT NOT NULL,
    branch_id BIGINT NOT NULL,
    shifts JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_staff_branches_staff FOREIGN KEY (organization_id, staff_id)
        REFERENCES staff (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_staff_branches_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT uq_staff_branches UNIQUE (organization_id, staff_id, branch_id)
);

CREATE INDEX idx_staff_branches_branch ON staff_branches (organization_id, branch_id);

CREATE TRIGGER trg_staff_branches_updated_at
    BEFORE UPDATE ON staff_branches
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Time off (leave, sick leave, training, other): the person is unavailable
-- in every branch between starts_at and ends_at (half-open).
CREATE TABLE staff_time_off (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    staff_id BIGINT NOT NULL,
    kind VARCHAR(16) NOT NULL DEFAULT 'leave' CHECK (kind IN ('leave', 'sick', 'training', 'other')),
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_staff_time_off_range CHECK (ends_at > starts_at),
    CONSTRAINT uq_staff_time_off_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_staff_time_off_staff FOREIGN KEY (organization_id, staff_id)
        REFERENCES staff (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_staff_time_off_staff ON staff_time_off (organization_id, staff_id, starts_at);

CREATE TRIGGER trg_staff_time_off_updated_at
    BEFORE UPDATE ON staff_time_off
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO permissions (name, slug) VALUES
    ('Read staff', 'tenant.staff.read'),
    ('Manage staff', 'tenant.staff.write'),
    ('Export staff', 'tenant.staff.export'),
    ('Import staff', 'tenant.staff.import'),
    ('Bulk activate staff', 'tenant.staff.bulk.activate'),
    ('Bulk deactivate staff', 'tenant.staff.bulk.deactivate')
ON CONFLICT (slug) DO NOTHING;

-- Owner and admin: everything. Branch manager: the staff of the assigned
-- branches (read, write, export). Reception: read (calendar, booking).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug IN (
    'tenant.staff.read', 'tenant.staff.write', 'tenant.staff.export', 'tenant.staff.import',
    'tenant.staff.bulk.activate', 'tenant.staff.bulk.deactivate')
WHERE r.slug IN ('owner', 'admin') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug IN (
    'tenant.staff.read', 'tenant.staff.write', 'tenant.staff.export')
WHERE r.slug = 'sube_yoneticisi' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug = 'tenant.staff.read'
WHERE r.slug = 'resepsiyon' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
