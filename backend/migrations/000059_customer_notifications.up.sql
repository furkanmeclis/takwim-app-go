-- TEC-911: basic customer notifications (panel link, request received,
-- accepted, rejected, rescheduled, cancelled, reminder before the visit).
-- They work in every business, without the confirmation add-on (A28).
--
--   * customer_notification_rules: a business's override of an event's
--     rule (on / off, ordered channels, fallback, reminder offsets). No row
--     means the catalog default.
--   * customer_notification_templates: a business's text for one event and
--     channel. No row means the catalog default.
--   * customer_notification_dispatches: the outbox. One event for one
--     customer, written in the transaction of the reservation change, so a
--     committed change always has its notice and a rolled back one never
--     does. idempotency_key is unique per business: a replayed change or a
--     second reminder sweep never creates a second notice.
--   * customer_notification_deliveries: one attempt per dispatch and
--     channel (unique): a retry never sends the same notice twice on one
--     channel. A channel without a ready provider is logged as skipped,
--     never as sent.

CREATE TABLE customer_notification_rules (
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_code VARCHAR(48) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    channels TEXT[] NOT NULL DEFAULT '{}',
    fallback BOOLEAN NOT NULL DEFAULT TRUE,
    -- Minutes before the visit (reminder events only).
    reminder_offsets INT[] NOT NULL DEFAULT '{}',
    updated_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, event_code)
);

CREATE TRIGGER trg_customer_notification_rules_updated_at
    BEFORE UPDATE ON customer_notification_rules
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE customer_notification_templates (
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_code VARCHAR(48) NOT NULL,
    channel VARCHAR(16) NOT NULL CHECK (channel IN ('sms', 'whatsapp', 'email')),
    -- E-mail subject; empty for text channels.
    subject VARCHAR(200) NOT NULL DEFAULT '',
    body TEXT NOT NULL CHECK (length(body) BETWEEN 1 AND 1000),
    updated_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, event_code, channel)
);

CREATE TRIGGER trg_customer_notification_templates_updated_at
    BEFORE UPDATE ON customer_notification_templates
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE customer_notification_dispatches (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_code VARCHAR(48) NOT NULL,
    customer_id BIGINT NOT NULL,
    reservation_id BIGINT NULL,
    -- Version of the reservation the notice was written for.
    reservation_version INT NOT NULL DEFAULT 0,
    idempotency_key VARCHAR(200) NOT NULL,
    -- Ordered channel snapshot of the rule; next_channel_index is the one
    -- to try next.
    channels TEXT[] NOT NULL DEFAULT '{}',
    fallback BOOLEAN NOT NULL DEFAULT TRUE,
    next_channel_index INT NOT NULL DEFAULT 0 CHECK (next_channel_index >= 0),
    -- Message variables (names, reference, times, the public panel
    -- address). Never a sign-in token.
    vars JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(16) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'sending', 'delivered', 'failed', 'skipped', 'cancelled')),
    last_error TEXT NOT NULL DEFAULT '',
    -- The visit start a reminder refers to: a moved or cancelled visit
    -- drops its reminder at send time.
    due_at TIMESTAMPTZ NULL,
    scheduled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_customer_notification_dispatches_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_customer_notification_dispatches_id UNIQUE (organization_id, id),
    CONSTRAINT uq_customer_notification_dispatches_key UNIQUE (organization_id, idempotency_key),
    CONSTRAINT fk_customer_notification_dispatches_customer
        FOREIGN KEY (organization_id, customer_id) REFERENCES customers (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_customer_notification_dispatches_reservation
        FOREIGN KEY (organization_id, reservation_id) REFERENCES reservations (organization_id, id) ON DELETE CASCADE
);

-- The worker's due scan.
CREATE INDEX idx_customer_notification_dispatches_due
    ON customer_notification_dispatches (organization_id, scheduled_at)
    WHERE status = 'pending';
CREATE INDEX idx_customer_notification_dispatches_reservation
    ON customer_notification_dispatches (organization_id, reservation_id);
CREATE INDEX idx_customer_notification_dispatches_customer
    ON customer_notification_dispatches (organization_id, customer_id);

CREATE TRIGGER trg_customer_notification_dispatches_updated_at
    BEFORE UPDATE ON customer_notification_dispatches
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE customer_notification_deliveries (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    dispatch_id BIGINT NOT NULL,
    channel VARCHAR(16) NOT NULL,
    attempt_no INT NOT NULL CHECK (attempt_no >= 1),
    status VARCHAR(16) NOT NULL DEFAULT 'sending'
        CHECK (status IN ('sending', 'sent', 'delivered', 'failed', 'skipped')),
    provider VARCHAR(32) NOT NULL DEFAULT '',
    provider_ref VARCHAR(128) NOT NULL DEFAULT '',
    -- Set by channels that confirm later (delivery reports).
    external_uuid UUID NULL,
    recipient_masked VARCHAR(255) NOT NULL DEFAULT '',
    subject VARCHAR(200) NOT NULL DEFAULT '',
    -- The rendered text as stored: a personal sign-in link is never kept.
    body TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    sent_at TIMESTAMPTZ NULL,
    finished_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_customer_notification_deliveries_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_customer_notification_deliveries_channel UNIQUE (organization_id, dispatch_id, channel),
    CONSTRAINT fk_customer_notification_deliveries_dispatch
        FOREIGN KEY (organization_id, dispatch_id) REFERENCES customer_notification_dispatches (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_customer_notification_deliveries_created
    ON customer_notification_deliveries (organization_id, created_at DESC);
CREATE INDEX idx_customer_notification_deliveries_external
    ON customer_notification_deliveries (organization_id, channel, external_uuid)
    WHERE external_uuid IS NOT NULL;

CREATE TRIGGER trg_customer_notification_deliveries_updated_at
    BEFORE UPDATE ON customer_notification_deliveries
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
