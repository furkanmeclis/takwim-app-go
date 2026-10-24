-- TEC-972: number reassignments processed from Takwim management and the
-- ownership check of numbers not proven for a long time.
--
--   * customer_phone_releases gets an id for the platform list (uuid), who
--     made the release (source: the person in the panel, a platform operator,
--     or the system), the operator and their note, and what the release
--     closed (businesses whose earlier history was hidden, sessions ended).
--   * reason 'dormant': the system closed the earlier history of a number
--     nobody proved for a long time, at its next sign-in (the number may
--     have changed hands unreported).
--   * customer_auth_events: 'platform_reassigned' and 'dormant_closed'.
--   * permissions platform.customer_phones.read / .write (super admin).

ALTER TABLE customer_phone_releases
    ADD COLUMN uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    ADD COLUMN source VARCHAR(16) NOT NULL DEFAULT 'customer' CHECK (source IN ('customer', 'platform', 'system')),
    ADD COLUMN note VARCHAR(500) NOT NULL DEFAULT '',
    ADD COLUMN created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN businesses_hidden INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN sessions_revoked INTEGER NOT NULL DEFAULT 0;

ALTER TABLE customer_phone_releases DROP CONSTRAINT customer_phone_releases_reason_check;
ALTER TABLE customer_phone_releases ADD CONSTRAINT customer_phone_releases_reason_check
    CHECK (reason IN ('number_changed', 'reassigned', 'dormant'));

CREATE INDEX idx_customer_phone_releases_released ON customer_phone_releases (released_at DESC, id DESC);
CREATE INDEX idx_customer_phone_releases_identity ON customer_phone_releases (identity_id, released_at DESC);

ALTER TABLE customer_auth_events DROP CONSTRAINT customer_auth_events_kind_check;
ALTER TABLE customer_auth_events ADD CONSTRAINT customer_auth_events_kind_check CHECK (kind IN (
    'signed_in', 'code_failed', 'link_failed', 'visit_link_failed', 'ip_throttled',
    'phone_changed', 'phone_change_refused', 'reassigned', 'session_revoked',
    'platform_reassigned', 'dormant_closed'
));

INSERT INTO permissions (name, slug) VALUES
    ('Read customer phone releases', 'platform.customer_phones.read'),
    ('Process customer phone reassignments', 'platform.customer_phones.write')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('platform.customer_phones.read', 'platform.customer_phones.write')
WHERE r.slug = 'super_admin' AND r.scope = 'platform'
ON CONFLICT DO NOTHING;
