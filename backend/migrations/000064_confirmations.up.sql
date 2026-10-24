-- TEC-921: customer confirmation tasks of the Konfirmasyon add-on
-- (requirements §9, K-007, A11, A12).
--
--   * confirmation_settings: when and how a business asks its customers to
--     confirm a booking. One business-wide row (branch_id NULL) and
--     optional per-branch rows; no row means the starting template (first
--     request 24 hours, follow-up 3 hours, last answer 2 hours before).
--   * reservation_confirmations: the confirmation task of one reservation:
--     the attempt times planned from the settings, how far it got, the
--     customer's answer and the outcome. It follows the reservation's
--     version: every change of the reservation updates it in the same
--     transaction, and a task acts only on the version it tracks, so an old
--     task never touches a moved, cancelled or finished visit. The messages
--     themselves are customer notification dispatches (outbox, TEC-911).

CREATE TABLE confirmation_settings (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- NULL: the business-wide setting; a branch row overrides it.
    branch_id BIGINT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    -- Ordered channels: the first is the primary one, the others are tried
    -- when it fails (fallback).
    channels TEXT[] NOT NULL,
    fallback BOOLEAN NOT NULL DEFAULT TRUE,
    -- Minutes before the visit start.
    first_request_minutes INT NOT NULL CHECK (first_request_minutes BETWEEN 30 AND 10080),
    follow_up_minutes INT NOT NULL DEFAULT 0 CHECK (follow_up_minutes BETWEEN 0 AND 10080),
    -- Requests per reservation, the first one included (repeat limit).
    max_attempts INT NOT NULL DEFAULT 1 CHECK (max_attempts BETWEEN 1 AND 5),
    deadline_minutes INT NOT NULL CHECK (deadline_minutes BETWEEN 0 AND 10080),
    -- Local hours (minutes of the day) in which requests and calls may go
    -- out; NULL = any time.
    window_start_minute INT NULL CHECK (window_start_minute BETWEEN 0 AND 1439),
    window_end_minute INT NULL CHECK (window_end_minute BETWEEN 1 AND 1440),
    -- No answer by the deadline: cancel the booking (K-007) or flag it.
    on_no_answer VARCHAR(16) NOT NULL DEFAULT 'cancel' CHECK (on_no_answer IN ('cancel', 'flag')),
    updated_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_confirmation_settings UNIQUE NULLS NOT DISTINCT (organization_id, branch_id),
    CONSTRAINT fk_confirmation_settings_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT ck_confirmation_settings_order CHECK (first_request_minutes > deadline_minutes),
    CONSTRAINT ck_confirmation_settings_window CHECK (
        (window_start_minute IS NULL) = (window_end_minute IS NULL)
        AND (window_start_minute IS NULL OR window_end_minute > window_start_minute)
    )
);

CREATE TRIGGER trg_confirmation_settings_updated_at
    BEFORE UPDATE ON confirmation_settings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE reservation_confirmations (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    reservation_id BIGINT NOT NULL,
    -- Raised each time the task is planned again (the visit moved); the
    -- messages of an earlier round are dropped at send time.
    round INT NOT NULL DEFAULT 1,
    -- The reservation version and start the task acts on.
    reservation_version INT NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    status VARCHAR(16) NOT NULL
        CHECK (status IN ('scheduled', 'pending', 'confirmed', 'declined', 'expired', 'dropped')),
    -- Snapshot of the settings when planned.
    channels TEXT[] NOT NULL,
    fallback BOOLEAN NOT NULL DEFAULT TRUE,
    on_no_answer VARCHAR(16) NOT NULL CHECK (on_no_answer IN ('cancel', 'flag')),
    attempt_times TIMESTAMPTZ[] NOT NULL DEFAULT '{}',
    attempts_sent INT NOT NULL DEFAULT 0 CHECK (attempts_sent >= 0),
    next_attempt_at TIMESTAMPTZ NULL,
    deadline_at TIMESTAMPTZ NOT NULL,
    answer VARCHAR(16) NULL CHECK (answer IN ('confirm', 'cancel', 'change')),
    answer_channel VARCHAR(16) NULL,
    answered_at TIMESTAMPTZ NULL,
    -- The customer asked for another time: the booking stays as it is, no
    -- more requests, no automatic cancel; the business handles it.
    change_requested_at TIMESTAMPTZ NULL,
    change_note TEXT NOT NULL DEFAULT '',
    -- How an expired task ended: cancelled (K-007), flagged (the business
    -- decides) or unreachable (no request could be sent; flagged).
    outcome VARCHAR(16) NULL CHECK (outcome IN ('cancelled', 'flagged', 'unreachable')),
    last_error TEXT NOT NULL DEFAULT '',
    planned_at TIMESTAMPTZ NOT NULL,
    closed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_reservation_confirmations_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_reservation_confirmations_reservation UNIQUE (organization_id, reservation_id),
    CONSTRAINT fk_reservation_confirmations_reservation FOREIGN KEY (organization_id, reservation_id)
        REFERENCES reservations (organization_id, id) ON DELETE CASCADE
);

-- The worker's due scans.
CREATE INDEX idx_reservation_confirmations_attempt
    ON reservation_confirmations (next_attempt_at)
    WHERE status IN ('scheduled', 'pending') AND next_attempt_at IS NOT NULL;
CREATE INDEX idx_reservation_confirmations_deadline
    ON reservation_confirmations (deadline_at)
    WHERE status IN ('scheduled', 'pending') AND change_requested_at IS NULL;
CREATE INDEX idx_reservation_confirmations_status
    ON reservation_confirmations (organization_id, status);

CREATE TRIGGER trg_reservation_confirmations_updated_at
    BEFORE UPDATE ON reservation_confirmations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
