-- TEC-894: service catalog (hizmet kataloğu). The catalog lives at the
-- business level (categories, services, who may perform them, which
-- resource kinds they need); each branch decides whether it offers a
-- service and may override its price and duration (şube teklifi).

CREATE TABLE service_categories (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name VARCHAR(120) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_service_categories_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_service_categories_organization_id UNIQUE (organization_id, id)
);

CREATE UNIQUE INDEX uq_service_categories_name
    ON service_categories (organization_id, lower(name))
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_service_categories_updated_at
    BEFORE UPDATE ON service_categories
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE services (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- Deleting a category (soft) moves its services to "uncategorized".
    category_id BIGINT NULL,
    name VARCHAR(160) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    -- Minutes the customer is served; buffers hold the staff / resource
    -- before (preparation) and after (cleaning) without being shown.
    duration_minutes INT NOT NULL,
    buffer_before_minutes INT NOT NULL DEFAULT 0,
    buffer_after_minutes INT NOT NULL DEFAULT 0,
    -- List price in the organization currency; branches may override it.
    price NUMERIC(14, 2) NOT NULL DEFAULT 0,
    -- How the customer pays: at the business, online, or either.
    payment_mode VARCHAR(16) NOT NULL DEFAULT 'on_site'
        CHECK (payment_mode IN ('on_site', 'online', 'both')),
    -- Whether a booking holds a staff member's time.
    staff_mode VARCHAR(16) NOT NULL DEFAULT 'required'
        CHECK (staff_mode IN ('required', 'optional', 'none')),
    -- Resource kinds a booking also holds, one of each (e.g. room + device).
    resource_kinds TEXT[] NOT NULL DEFAULT '{}'
        CHECK (resource_kinds <@ ARRAY['chair', 'table', 'area', 'room', 'device', 'bay']::TEXT[]),
    show_online BOOLEAN NOT NULL DEFAULT TRUE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INT NOT NULL DEFAULT 0,
    -- Cover image (API-owned media; the key never leaves the API).
    image_uuid UUID NULL,
    image_key TEXT NULL,
    image_content_type VARCHAR(64) NULL,
    image_size_bytes BIGINT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT ck_services_duration CHECK (duration_minutes BETWEEN 5 AND 1440),
    CONSTRAINT ck_services_buffers CHECK (buffer_before_minutes BETWEEN 0 AND 480 AND buffer_after_minutes BETWEEN 0 AND 480),
    CONSTRAINT ck_services_price CHECK (price >= 0),
    CONSTRAINT uq_services_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_services_organization_id UNIQUE (organization_id, id),
    CONSTRAINT fk_services_category FOREIGN KEY (organization_id, category_id)
        REFERENCES service_categories (organization_id, id)
);

CREATE UNIQUE INDEX uq_services_name
    ON services (organization_id, lower(name))
    WHERE deleted_at IS NULL;

CREATE INDEX idx_services_organization_category
    ON services (organization_id, category_id, sort_order)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX uq_services_image ON services (organization_id, image_uuid) WHERE image_uuid IS NOT NULL;

CREATE TRIGGER trg_services_updated_at
    BEFORE UPDATE ON services
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Branch offer: the service is bookable in the branch; NULL price /
-- duration use the catalog values.
CREATE TABLE service_branches (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    service_id BIGINT NOT NULL,
    branch_id BIGINT NOT NULL,
    price NUMERIC(14, 2) NULL CHECK (price IS NULL OR price >= 0),
    duration_minutes INT NULL CHECK (duration_minutes IS NULL OR duration_minutes BETWEEN 5 AND 1440),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_service_branches UNIQUE (organization_id, service_id, branch_id),
    CONSTRAINT fk_service_branches_service FOREIGN KEY (organization_id, service_id)
        REFERENCES services (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_service_branches_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_service_branches_branch ON service_branches (organization_id, branch_id);

CREATE TRIGGER trg_service_branches_updated_at
    BEFORE UPDATE ON service_branches
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Staff who may perform the service (yetkinlik), with optional personal
-- duration and price (a senior stylist takes longer or costs more).
CREATE TABLE service_staff (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    service_id BIGINT NOT NULL,
    staff_id BIGINT NOT NULL,
    price NUMERIC(14, 2) NULL CHECK (price IS NULL OR price >= 0),
    duration_minutes INT NULL CHECK (duration_minutes IS NULL OR duration_minutes BETWEEN 5 AND 1440),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_service_staff UNIQUE (organization_id, service_id, staff_id),
    CONSTRAINT fk_service_staff_service FOREIGN KEY (organization_id, service_id)
        REFERENCES services (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_service_staff_staff FOREIGN KEY (organization_id, staff_id)
        REFERENCES staff (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_service_staff_staff ON service_staff (organization_id, staff_id);

CREATE TRIGGER trg_service_staff_updated_at
    BEFORE UPDATE ON service_staff
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO permissions (name, slug) VALUES
    ('Read services', 'tenant.services.read'),
    ('Manage the service catalog', 'tenant.services.write'),
    ('Manage branch offers of services', 'tenant.services.offer'),
    ('Export services', 'tenant.services.export'),
    ('Import services', 'tenant.services.import'),
    ('Bulk activate services', 'tenant.services.bulk.activate'),
    ('Bulk deactivate services', 'tenant.services.bulk.deactivate')
ON CONFLICT (slug) DO NOTHING;

-- Owner and admin: everything. Branch manager: reads the catalog, exports
-- it and manages the offers (price, duration, on/off) of its branches.
-- Reception: read (booking).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug IN (
    'tenant.services.read', 'tenant.services.write', 'tenant.services.offer', 'tenant.services.export',
    'tenant.services.import', 'tenant.services.bulk.activate', 'tenant.services.bulk.deactivate')
WHERE r.slug IN ('owner', 'admin') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug IN (
    'tenant.services.read', 'tenant.services.offer', 'tenant.services.export')
WHERE r.slug = 'sube_yoneticisi' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug = 'tenant.services.read'
WHERE r.slug = 'resepsiyon' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
