-- TEC-899: the reservation record (requirements §8 and §15). The lifecycle,
-- the business's approval, the customer's confirmation and the payment are
-- separate fields: a request the business accepted but the customer has not
-- confirmed shows both, and a payment alone never confirms. Each line keeps
-- the service, staff member, resources, times, duration and price as they
-- were when the booking was made (snapshots), so later catalog changes do
-- not rewrite history. The capacity of the visit is the capacity_holds row
-- named by hold_id (a timed hold while the record waits for approval or
-- payment, a booking once it is active).

CREATE TABLE reservations (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- Per business, shown to people ("#1024").
    number BIGINT NOT NULL,
    branch_id BIGINT NOT NULL,
    customer_id BIGINT NOT NULL,
    hold_id BIGINT NULL,
    status VARCHAR(16) NOT NULL
        CHECK (status IN ('requested', 'active', 'in_service', 'completed', 'cancelled', 'rejected', 'no_show')),
    -- How the business decided: auto (accepted on creation), staff (a
    -- person accepts or rejects) or panel (staff booked it themselves).
    approval_mode VARCHAR(16) NOT NULL CHECK (approval_mode IN ('auto', 'staff', 'panel')),
    approval_status VARCHAR(16) NOT NULL CHECK (approval_status IN ('pending', 'accepted', 'rejected')),
    confirmation_status VARCHAR(16) NOT NULL DEFAULT 'not_required'
        CHECK (confirmation_status IN ('not_required', 'scheduled', 'pending', 'confirmed', 'declined', 'expired')),
    payment_status VARCHAR(24) NOT NULL
        CHECK (payment_status IN ('on_site', 'online_pending', 'paid', 'refund_pending', 'partially_refunded', 'refunded', 'failed')),
    source VARCHAR(16) NOT NULL CHECK (source IN ('panel', 'online', 'embed')),
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    total_price NUMERIC(14, 2) NOT NULL DEFAULT 0 CHECK (total_price >= 0),
    currency CHAR(3) NOT NULL,
    -- Answers to the business's booking form questions.
    custom_fields JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(custom_fields) = 'object'),
    customer_note TEXT NOT NULL DEFAULT '',
    internal_note TEXT NOT NULL DEFAULT '',
    -- Raised by every change; jobs and clients act only on the version they
    -- read (a stale confirmation reminder never cancels a moved booking).
    version INT NOT NULL DEFAULT 1,
    cancel_reason VARCHAR(32) NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_reservations_range CHECK (ends_at > starts_at),
    CONSTRAINT uq_reservations_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_reservations_organization_id UNIQUE (organization_id, id),
    CONSTRAINT uq_reservations_organization_number UNIQUE (organization_id, number),
    CONSTRAINT fk_reservations_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id),
    CONSTRAINT fk_reservations_customer FOREIGN KEY (organization_id, customer_id)
        REFERENCES customers (organization_id, id),
    CONSTRAINT fk_reservations_hold FOREIGN KEY (organization_id, hold_id)
        REFERENCES capacity_holds (organization_id, id) ON DELETE SET NULL (hold_id)
);

CREATE INDEX idx_reservations_branch_start ON reservations (organization_id, branch_id, starts_at);
CREATE INDEX idx_reservations_customer ON reservations (organization_id, customer_id, starts_at DESC);
CREATE INDEX idx_reservations_hold ON reservations (organization_id, hold_id) WHERE hold_id IS NOT NULL;

CREATE TRIGGER trg_reservations_updated_at
    BEFORE UPDATE ON reservations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- One service of the visit, in the order the customer is served.
CREATE TABLE reservation_lines (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    reservation_id BIGINT NOT NULL,
    line_index INT NOT NULL,
    service_id BIGINT NULL,
    service_name VARCHAR(160) NOT NULL,
    staff_id BIGINT NULL,
    staff_name VARCHAR(160) NULL,
    -- [{uuid, name, kind}] of the resources the line held.
    resources JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(resources) = 'array'),
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    duration_minutes INT NOT NULL,
    buffer_before_minutes INT NOT NULL DEFAULT 0,
    buffer_after_minutes INT NOT NULL DEFAULT 0,
    unit_price NUMERIC(14, 2) NOT NULL CHECK (unit_price >= 0),
    -- Filled by the finance modules (VAT, staff earnings); NULL until then.
    vat_rate NUMERIC(5, 2) NULL,
    staff_share_rate NUMERIC(5, 2) NULL,
    CONSTRAINT ck_reservation_lines_range CHECK (ends_at > starts_at),
    CONSTRAINT uq_reservation_lines UNIQUE (organization_id, reservation_id, line_index),
    CONSTRAINT fk_reservation_lines_reservation FOREIGN KEY (organization_id, reservation_id)
        REFERENCES reservations (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_reservation_lines_service FOREIGN KEY (organization_id, service_id)
        REFERENCES services (organization_id, id) ON DELETE SET NULL (service_id),
    CONSTRAINT fk_reservation_lines_staff FOREIGN KEY (organization_id, staff_id)
        REFERENCES staff (organization_id, id) ON DELETE SET NULL (staff_id)
);

CREATE INDEX idx_reservation_lines_staff ON reservation_lines (organization_id, staff_id, starts_at)
    WHERE staff_id IS NOT NULL;

-- History: every change with the version it produced.
CREATE TABLE reservation_events (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    reservation_id BIGINT NOT NULL,
    version INT NOT NULL,
    type VARCHAR(32) NOT NULL,
    from_status VARCHAR(16) NULL,
    to_status VARCHAR(16) NULL,
    actor_id BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_reservation_events_reservation FOREIGN KEY (organization_id, reservation_id)
        REFERENCES reservations (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_reservation_events_reservation ON reservation_events (organization_id, reservation_id, id);
