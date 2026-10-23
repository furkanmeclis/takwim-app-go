-- TEC-909: the shared customer panel (takwim.link/panel).
--
-- Both tables are platform-level, like customer_identities: a person signs
-- in once and sees the records every business linked to the identity
-- (customer_identity_links). No business reads them.
--
--   * customer_otp_challenges: one-time sign-in codes sent to a phone. Only
--     a keyed hash of the code is stored; attempts are counted per code.
--   * customer_sessions: a panel session, opened after a code was verified.
--     The customer token (JWT kind "customer") names the session; logout
--     and expiry are decided here on every request.

CREATE TABLE customer_otp_challenges (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    phone_e164 VARCHAR(16) NOT NULL CHECK (phone_e164 ~ '^\+[1-9][0-9]{6,14}$'),
    purpose VARCHAR(16) NOT NULL DEFAULT 'panel_login' CHECK (purpose IN ('panel_login')),
    -- Keyed hash (HMAC) of the code; the code itself is never stored.
    code_hash VARCHAR(128) NOT NULL,
    -- Whether a message went out (a phone no business knows gets none).
    sent BOOLEAN NOT NULL DEFAULT FALSE,
    attempts INT NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts INT NOT NULL DEFAULT 5 CHECK (max_attempts > 0),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ NULL,
    ip VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_customer_otp_challenges_phone ON customer_otp_challenges (phone_e164, created_at DESC);

CREATE TABLE customer_sessions (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    identity_id BIGINT NOT NULL REFERENCES customer_identities (id) ON DELETE CASCADE,
    -- The phone proven at sign-in and when.
    phone_e164 VARCHAR(16) NOT NULL,
    verified_at TIMESTAMPTZ NOT NULL,
    ip VARCHAR(64) NOT NULL DEFAULT '',
    user_agent VARCHAR(512) NOT NULL DEFAULT '',
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Sliding idle expiry, inside the hard cap expires_at.
    idle_expires_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_customer_sessions_identity ON customer_sessions (identity_id) WHERE revoked_at IS NULL;
