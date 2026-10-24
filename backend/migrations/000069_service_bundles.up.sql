-- TEC-895: combined service bundles (birleşik hizmet paketi, requirements
-- §13). A bundle offers several catalog services as one priced offer
-- ("İlkbahar paketi"): its contents in the order the customer is served,
-- the branches that offer it and one bundle price. The total duration and
-- the staff / resource needs follow from the contents (every service keeps
-- its own duration, staff mode and resource kinds). A bundle is a one-visit
-- offer; prepaid multi-session packages are a separate concept.
--
-- A booking of a bundle keeps every service as its own reservation line;
-- the bundle discount is spread over the lines in proportion to their list
-- prices (the rounding remainder goes to the last line), so staff earnings
-- are computed per line (A16).

CREATE TABLE service_bundles (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name VARCHAR(160) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    -- Bundle price in the organization currency.
    price NUMERIC(14, 2) NOT NULL CHECK (price >= 0),
    show_online BOOLEAN NOT NULL DEFAULT TRUE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_service_bundles_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_service_bundles_organization_id UNIQUE (organization_id, id)
);

CREATE UNIQUE INDEX uq_service_bundles_name
    ON service_bundles (organization_id, lower(name))
    WHERE deleted_at IS NULL;

CREATE INDEX idx_service_bundles_organization_sort
    ON service_bundles (organization_id, sort_order, name)
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_service_bundles_updated_at
    BEFORE UPDATE ON service_bundles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Contents: one row per service, position = the order of service. The same
-- service may appear twice (two sessions in one visit).
CREATE TABLE service_bundle_items (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    bundle_id BIGINT NOT NULL,
    service_id BIGINT NOT NULL,
    position INT NOT NULL CHECK (position >= 0),
    CONSTRAINT uq_service_bundle_items UNIQUE (organization_id, bundle_id, position),
    CONSTRAINT fk_service_bundle_items_bundle FOREIGN KEY (organization_id, bundle_id)
        REFERENCES service_bundles (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_service_bundle_items_service FOREIGN KEY (organization_id, service_id)
        REFERENCES services (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_service_bundle_items_service ON service_bundle_items (organization_id, service_id);

-- Branches that offer the bundle.
CREATE TABLE service_bundle_branches (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    bundle_id BIGINT NOT NULL,
    branch_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_service_bundle_branches UNIQUE (organization_id, bundle_id, branch_id),
    CONSTRAINT fk_service_bundle_branches_bundle FOREIGN KEY (organization_id, bundle_id)
        REFERENCES service_bundles (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_service_bundle_branches_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_service_bundle_branches_branch ON service_bundle_branches (organization_id, branch_id);

-- A line booked as part of a bundle keeps the bundle (and its name) and the
-- list price it had before the bundle discount; unit_price is the line's
-- share of the bundle price.
ALTER TABLE reservation_lines
    ADD COLUMN bundle_id BIGINT NULL,
    ADD COLUMN bundle_name VARCHAR(160) NULL,
    ADD COLUMN list_price NUMERIC(14, 2) NULL CHECK (list_price IS NULL OR list_price >= 0),
    ADD CONSTRAINT fk_reservation_lines_bundle FOREIGN KEY (organization_id, bundle_id)
        REFERENCES service_bundles (organization_id, id) ON DELETE SET NULL (bundle_id);

CREATE INDEX idx_reservation_lines_bundle ON reservation_lines (organization_id, bundle_id)
    WHERE bundle_id IS NOT NULL;
