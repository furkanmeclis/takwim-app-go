-- Mobile push devices (Expo Push Service). Browser Web Push stays in
-- push_subscriptions. Tokens are user-level: notifications and preferences
-- target users, and the payload's data.url carries the deep link.
CREATE TABLE push_devices (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    user_id         BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token           VARCHAR(255) NOT NULL,
    platform        VARCHAR(16)  NOT NULL,
    locale          VARCHAR(8)   NOT NULL DEFAULT '',
    app_version     VARCHAR(32)  NOT NULL DEFAULT '',
    device_name     VARCHAR(120) NOT NULL DEFAULT '',
    last_seen_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    disabled_at     TIMESTAMPTZ  NULL,
    disabled_reason VARCHAR(64)  NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_push_devices_uuid UNIQUE (uuid),
    CONSTRAINT uq_push_devices_token UNIQUE (token),
    CONSTRAINT chk_push_devices_platform CHECK (platform IN ('ios', 'android'))
);

CREATE INDEX idx_push_devices_user_active ON push_devices (user_id) WHERE disabled_at IS NULL;

CREATE TRIGGER trg_push_devices_set_updated_at BEFORE UPDATE ON push_devices
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Expo push tickets awaiting their receipt (checked ~15 min after sending;
-- DeviceNotRegistered disables the device). Rows are deleted once processed.
CREATE TABLE push_tickets (
    id              BIGSERIAL   PRIMARY KEY,
    ticket_id       VARCHAR(64) NOT NULL,
    push_device_id  BIGINT      NOT NULL REFERENCES push_devices (id) ON DELETE CASCADE,
    notification_id BIGINT      NULL REFERENCES notifications (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_push_tickets_ticket_id UNIQUE (ticket_id)
);

CREATE INDEX idx_push_tickets_created_at ON push_tickets (created_at);
