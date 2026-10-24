-- TEC-910: access security of the shared customer panel.
--
--   * customer_access_links: single-use panel links sent by SMS after a
--     booking (takwim.link/panel/l#token). Only a keyed hash is stored.
--   * customer_visit_links: links that open one reservation (view, cancel;
--     confirmation later). A separate token type: it never opens the panel
--     or another visit. Organization-owned.
--   * customer_phone_releases: a phone the person moved away from (number
--     change) or reported as reassigned to them. Records of the phone made
--     before the release never open for the next holder.
--   * customer_identity_links.visible_from / visible_until: the window of
--     reservations (by creation time) the identity sees through the link.
--   * customer_auth_events: sign-in failures and security changes, for
--     abuse limits and monitoring. Platform-level, never shown to a business.

CREATE TABLE customer_access_links (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    -- Keyed hash (HMAC) of the token; the token itself is never stored.
    token_hash VARCHAR(128) NOT NULL UNIQUE,
    phone_e164 VARCHAR(16) NOT NULL CHECK (phone_e164 ~ '^\+[1-9][0-9]{6,14}$'),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_customer_access_links_phone ON customer_access_links (phone_e164, created_at DESC);

CREATE TABLE customer_visit_links (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    reservation_id BIGINT NOT NULL,
    token_hash VARCHAR(128) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ NULL,
    last_used_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_customer_visit_links_reservation FOREIGN KEY (organization_id, reservation_id)
        REFERENCES reservations (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_customer_visit_links_reservation ON customer_visit_links (organization_id, reservation_id);

CREATE TABLE customer_phone_releases (
    id BIGSERIAL PRIMARY KEY,
    phone_e164 VARCHAR(16) NOT NULL CHECK (phone_e164 ~ '^\+[1-9][0-9]{6,14}$'),
    identity_id BIGINT NULL REFERENCES customer_identities (id) ON DELETE SET NULL,
    reason VARCHAR(16) NOT NULL CHECK (reason IN ('number_changed', 'reassigned')),
    released_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_customer_phone_releases_phone ON customer_phone_releases (phone_e164, released_at DESC);

ALTER TABLE customer_identity_links
    ADD COLUMN visible_from TIMESTAMPTZ NULL,
    ADD COLUMN visible_until TIMESTAMPTZ NULL;

ALTER TABLE customer_identity_links DROP CONSTRAINT customer_identity_links_source_check;
ALTER TABLE customer_identity_links ADD CONSTRAINT customer_identity_links_source_check
    CHECK (source IN ('otp', 'panel_login', 'panel_link', 'phone_change'));

-- A phone change code belongs to the signed-in identity that asked for it.
ALTER TABLE customer_otp_challenges
    ADD COLUMN identity_id BIGINT NULL REFERENCES customer_identities (id) ON DELETE CASCADE;
ALTER TABLE customer_otp_challenges DROP CONSTRAINT customer_otp_challenges_purpose_check;
ALTER TABLE customer_otp_challenges ADD CONSTRAINT customer_otp_challenges_purpose_check
    CHECK (purpose IN ('panel_login', 'phone_change'));
CREATE INDEX idx_customer_otp_challenges_ip ON customer_otp_challenges (ip, created_at DESC);

-- How the session was opened: a code or a panel link.
ALTER TABLE customer_sessions
    ADD COLUMN method VARCHAR(16) NOT NULL DEFAULT 'code' CHECK (method IN ('code', 'link'));

CREATE TABLE customer_auth_events (
    id BIGSERIAL PRIMARY KEY,
    kind VARCHAR(32) NOT NULL CHECK (kind IN (
        'signed_in', 'code_failed', 'link_failed', 'visit_link_failed', 'ip_throttled',
        'phone_changed', 'phone_change_refused', 'reassigned', 'session_revoked'
    )),
    phone_e164 VARCHAR(16) NULL,
    identity_id BIGINT NULL REFERENCES customer_identities (id) ON DELETE SET NULL,
    ip VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_customer_auth_events_ip ON customer_auth_events (ip, kind, created_at DESC);
CREATE INDEX idx_customer_auth_events_identity ON customer_auth_events (identity_id, created_at DESC);
CREATE INDEX idx_customer_auth_events_created ON customer_auth_events (created_at);
