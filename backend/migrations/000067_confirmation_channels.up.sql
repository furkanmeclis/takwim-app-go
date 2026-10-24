-- TEC-922: confirmation channels (WhatsApp, SMS, e-mail).
--
--   * customer_notification_deliveries gets the provider's receipt times:
--     sent_at stays the moment the provider accepted the message,
--     delivered_at and read_at come from delivery reports (WhatsApp
--     statuses). Neither of them is ever an answer.
--   * customer_notification_responses: the customer's replies on a channel
--     (a WhatsApp button), one row per inbound message, with what the
--     reply asked for and what was done with it. A reply that could not be
--     applied is kept as rejected or ignored, never as a confirmation.

ALTER TABLE customer_notification_deliveries
    ADD COLUMN delivered_at TIMESTAMPTZ NULL,
    ADD COLUMN read_at TIMESTAMPTZ NULL;

-- Delivery reports and replies name the provider's message id.
CREATE INDEX idx_customer_notification_deliveries_provider_ref
    ON customer_notification_deliveries (organization_id, channel, provider_ref)
    WHERE provider_ref <> '';

-- The receipt timeout scans accepted attempts still waiting for a report.
CREATE INDEX idx_customer_notification_deliveries_unreported
    ON customer_notification_deliveries (sent_at)
    WHERE status = 'sent' AND finished_at IS NULL;

CREATE TABLE customer_notification_responses (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    delivery_id BIGINT NOT NULL REFERENCES customer_notification_deliveries (id) ON DELETE CASCADE,
    channel VARCHAR(16) NOT NULL,
    provider VARCHAR(32) NOT NULL DEFAULT '',
    -- The provider's id of the inbound message (a repeated webhook is the
    -- same reply).
    provider_ref VARCHAR(128) NOT NULL,
    answer VARCHAR(16) NOT NULL CHECK (answer IN ('confirm', 'cancel', 'change', 'unknown')),
    result VARCHAR(16) NOT NULL CHECK (result IN ('applied', 'rejected', 'ignored')),
    -- Why a reply was rejected or ignored (answered already, the visit
    -- moved, another number).
    reason VARCHAR(255) NOT NULL DEFAULT '',
    -- When the customer replied (the provider's time).
    responded_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_customer_notification_responses_ref UNIQUE (organization_id, channel, provider_ref)
);

CREATE INDEX idx_customer_notification_responses_delivery
    ON customer_notification_responses (organization_id, delivery_id, responded_at);
