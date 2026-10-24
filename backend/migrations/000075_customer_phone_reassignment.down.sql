-- Data-loss note: removes the dormant releases (the history windows they
-- set on identity links stay), the new auth event kinds, the operator,
-- note and counts of the releases, and the customer phone permissions with
-- their role grants.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN ('platform.customer_phones.read', 'platform.customer_phones.write')
);
DELETE FROM permissions WHERE slug IN ('platform.customer_phones.read', 'platform.customer_phones.write');

DELETE FROM customer_auth_events WHERE kind IN ('platform_reassigned', 'dormant_closed');
ALTER TABLE customer_auth_events DROP CONSTRAINT customer_auth_events_kind_check;
ALTER TABLE customer_auth_events ADD CONSTRAINT customer_auth_events_kind_check CHECK (kind IN (
    'signed_in', 'code_failed', 'link_failed', 'visit_link_failed', 'ip_throttled',
    'phone_changed', 'phone_change_refused', 'reassigned', 'session_revoked'
));

DROP INDEX IF EXISTS idx_customer_phone_releases_identity;
DROP INDEX IF EXISTS idx_customer_phone_releases_released;

DELETE FROM customer_phone_releases WHERE reason = 'dormant';
ALTER TABLE customer_phone_releases DROP CONSTRAINT customer_phone_releases_reason_check;
ALTER TABLE customer_phone_releases ADD CONSTRAINT customer_phone_releases_reason_check
    CHECK (reason IN ('number_changed', 'reassigned'));

ALTER TABLE customer_phone_releases
    DROP COLUMN IF EXISTS sessions_revoked,
    DROP COLUMN IF EXISTS businesses_hidden,
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS note,
    DROP COLUMN IF EXISTS source,
    DROP COLUMN IF EXISTS uuid;
