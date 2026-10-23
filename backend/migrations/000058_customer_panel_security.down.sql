DROP TABLE IF EXISTS customer_auth_events;

ALTER TABLE customer_sessions DROP COLUMN IF EXISTS method;

DROP INDEX IF EXISTS idx_customer_otp_challenges_ip;
DELETE FROM customer_otp_challenges WHERE purpose <> 'panel_login';
ALTER TABLE customer_otp_challenges DROP CONSTRAINT customer_otp_challenges_purpose_check;
ALTER TABLE customer_otp_challenges ADD CONSTRAINT customer_otp_challenges_purpose_check
    CHECK (purpose IN ('panel_login'));
ALTER TABLE customer_otp_challenges DROP COLUMN IF EXISTS identity_id;

UPDATE customer_identity_links SET source = 'panel_login' WHERE source IN ('panel_link', 'phone_change');
ALTER TABLE customer_identity_links DROP CONSTRAINT customer_identity_links_source_check;
ALTER TABLE customer_identity_links ADD CONSTRAINT customer_identity_links_source_check
    CHECK (source IN ('otp', 'panel_login'));
ALTER TABLE customer_identity_links
    DROP COLUMN IF EXISTS visible_until,
    DROP COLUMN IF EXISTS visible_from;

DROP TABLE IF EXISTS customer_phone_releases;
DROP TABLE IF EXISTS customer_visit_links;
DROP TABLE IF EXISTS customer_access_links;
