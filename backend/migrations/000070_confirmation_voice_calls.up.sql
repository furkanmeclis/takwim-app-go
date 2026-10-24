-- TEC-923: confirmation by an automatic voice call (1 = Geleceğim,
-- 2 = cancel, 3 = change request).
--
--   * confirmation_settings gets the call rules: the local hours calls may
--     ring (NULL = the request window) and how many extra calls a task may
--     make when a call is not answered or brings no valid key, and how long
--     to wait before calling again.
--   * reservation_confirmations keeps a copy of those rules from planning,
--     the calls made again and when the next one is due.
--
-- The call itself is a customer notification delivery on the voice_call
-- channel; the key the customer pressed is a customer_notification_responses
-- row like a WhatsApp button. A picked up call is never a confirmation.

ALTER TABLE confirmation_settings
    ADD COLUMN call_window_start_minute INT NULL CHECK (call_window_start_minute BETWEEN 0 AND 1439),
    ADD COLUMN call_window_end_minute INT NULL CHECK (call_window_end_minute BETWEEN 1 AND 1440),
    ADD COLUMN call_retries INT NOT NULL DEFAULT 1 CHECK (call_retries BETWEEN 0 AND 3),
    ADD COLUMN call_retry_minutes INT NOT NULL DEFAULT 30 CHECK (call_retry_minutes BETWEEN 15 AND 240),
    ADD CONSTRAINT ck_confirmation_settings_call_window CHECK (
        (call_window_start_minute IS NULL) = (call_window_end_minute IS NULL)
        AND (call_window_start_minute IS NULL OR call_window_end_minute > call_window_start_minute)
    );

ALTER TABLE reservation_confirmations
    ADD COLUMN call_window_start_minute INT NULL,
    ADD COLUMN call_window_end_minute INT NULL,
    ADD COLUMN call_retries INT NOT NULL DEFAULT 0 CHECK (call_retries >= 0),
    ADD COLUMN call_retry_minutes INT NOT NULL DEFAULT 30,
    -- Extra calls made in this round (the requests' own calls not counted).
    ADD COLUMN calls_made INT NOT NULL DEFAULT 0 CHECK (calls_made >= 0),
    ADD COLUMN next_call_at TIMESTAMPTZ NULL;

-- The worker's due scan of the calls made again.
CREATE INDEX idx_reservation_confirmations_call
    ON reservation_confirmations (next_call_at)
    WHERE status IN ('scheduled', 'pending') AND next_call_at IS NOT NULL;
