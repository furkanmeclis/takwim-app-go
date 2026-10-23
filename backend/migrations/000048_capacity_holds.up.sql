-- TEC-898: capacity commitments and timed holds (requirements A03).
--
-- capacity_holds is one booking unit (all lines of a visit): a timed hold
-- (waiting for the business's approval, for online payment, or for the
-- customer to finish the booking form) or a confirmed booking. Each line
-- holds a staff member and / or resources for its held time (buffers
-- included) in capacity_allocations. The exclusion constraints make a
-- double booking impossible whatever the application does: two allocations
-- of one person or one resource never overlap. Only active commitments have
-- allocation rows; releasing or expiring a hold deletes them in the same
-- transaction that changes the status.
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE capacity_holds (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    branch_id BIGINT NOT NULL,
    -- hold = timed (expires_at set); booking = confirmed, no expiry.
    kind VARCHAR(16) NOT NULL CHECK (kind IN ('hold', 'booking')),
    -- Why a hold waits: the business's approval, online payment, or the
    -- customer still filling in the booking form.
    reason VARCHAR(16) NULL CHECK (reason IN ('approval', 'payment', 'checkout')),
    status VARCHAR(16) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'released', 'expired')),
    channel VARCHAR(16) NOT NULL DEFAULT 'panel' CHECK (channel IN ('panel', 'online')),
    -- Customer time of the whole visit (first line start .. last line end).
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NULL,
    -- The record the commitment belongs to (reservation, TEC-899).
    ref_type VARCHAR(32) NULL,
    ref_uuid UUID NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    released_at TIMESTAMPTZ NULL,
    release_reason VARCHAR(32) NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_capacity_holds_range CHECK (ends_at > starts_at),
    CONSTRAINT ck_capacity_holds_kind CHECK (
        (kind = 'hold' AND reason IS NOT NULL AND expires_at IS NOT NULL)
        OR (kind = 'booking' AND expires_at IS NULL)
    ),
    CONSTRAINT uq_capacity_holds_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_capacity_holds_organization_id UNIQUE (organization_id, id),
    CONSTRAINT fk_capacity_holds_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id) ON DELETE CASCADE
);

-- The expiry sweep and the lazy expiry of writers.
CREATE INDEX idx_capacity_holds_due
    ON capacity_holds (organization_id, expires_at)
    WHERE status = 'active' AND kind = 'hold';

CREATE INDEX idx_capacity_holds_ref
    ON capacity_holds (organization_id, ref_type, ref_uuid)
    WHERE ref_uuid IS NOT NULL;

CREATE TRIGGER trg_capacity_holds_updated_at
    BEFORE UPDATE ON capacity_holds
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE capacity_allocations (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    hold_id BIGINT NOT NULL,
    -- Position of the line in the visit (0 = first service).
    line_index INT NOT NULL CHECK (line_index >= 0),
    staff_id BIGINT NULL,
    resource_id BIGINT NULL,
    -- Held time of the line: buffers before / after included.
    period TSTZRANGE NOT NULL,
    CONSTRAINT ck_capacity_allocations_holder CHECK ((staff_id IS NULL) <> (resource_id IS NULL)),
    CONSTRAINT ck_capacity_allocations_period CHECK (NOT isempty(period) AND lower_inc(period) AND NOT upper_inc(period)),
    CONSTRAINT fk_capacity_allocations_hold FOREIGN KEY (organization_id, hold_id)
        REFERENCES capacity_holds (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_capacity_allocations_staff FOREIGN KEY (organization_id, staff_id)
        REFERENCES staff (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_capacity_allocations_resource FOREIGN KEY (organization_id, resource_id)
        REFERENCES resources (organization_id, id) ON DELETE CASCADE,
    -- One person, one calendar: never two commitments at once, in any branch
    -- (A04). The lines of one visit may share a holder: its held times are
    -- merged where they touch, so a visit never conflicts with itself.
    CONSTRAINT ex_capacity_allocations_staff EXCLUDE USING gist (
        organization_id WITH =, staff_id WITH =, period WITH &&
    ) WHERE (staff_id IS NOT NULL),
    CONSTRAINT ex_capacity_allocations_resource EXCLUDE USING gist (
        organization_id WITH =, resource_id WITH =, period WITH &&
    ) WHERE (resource_id IS NOT NULL)
);

CREATE INDEX idx_capacity_allocations_hold ON capacity_allocations (organization_id, hold_id);
