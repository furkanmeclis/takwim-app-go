-- TEC-906: checkouts of the public booking API. A checkout is the timed
-- capacity hold a customer takes on the booking page, the widget or the
-- mobile app while filling in the contact form, with the lines it was
-- planned for (service and the staff member the slot offered). Booking it
-- needs only the checkout token and the contact details; the same checkout
-- books once (reservation_id), so a retried booking answers the same
-- reservation.

CREATE TABLE public_checkouts (
    id BIGSERIAL PRIMARY KEY,
    -- The checkout token handed to the client (random, never sequential).
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    branch_id BIGINT NOT NULL,
    hold_id BIGINT NOT NULL,
    -- Sales channel of the caller: the takwim.link page, the widget or the
    -- API (mobile app, partners).
    channel VARCHAR(16) NOT NULL CHECK (channel IN ('web', 'widget', 'api')),
    -- [{"service_uuid": "...", "staff_uuid": "..." | null}] in serving order.
    lines JSONB NOT NULL CHECK (jsonb_typeof(lines) = 'array'),
    starts_at TIMESTAMPTZ NOT NULL,
    -- The reservation the checkout became.
    reservation_id BIGINT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_public_checkouts_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_public_checkouts_organization_hold UNIQUE (organization_id, hold_id),
    CONSTRAINT fk_public_checkouts_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_public_checkouts_hold FOREIGN KEY (organization_id, hold_id)
        REFERENCES capacity_holds (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_public_checkouts_reservation FOREIGN KEY (organization_id, reservation_id)
        REFERENCES reservations (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_public_checkouts_reservation
    ON public_checkouts (organization_id, reservation_id)
    WHERE reservation_id IS NOT NULL;

CREATE TRIGGER trg_public_checkouts_updated_at
    BEFORE UPDATE ON public_checkouts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
