DELETE FROM notification_templates
WHERE code IN ('auth.login_code', 'auth.oauth_link_code', 'auth.account_deactivation_code', 'auth.account_deactivated');

ALTER TABLE oauth_accounts DROP COLUMN IF EXISTS client_id;

-- Drops pending one-time codes of the new types (short-lived, safe to lose).
DELETE FROM otp_codes WHERE type IN ('login_code', 'oauth_link', 'account_deactivation');
ALTER TABLE otp_codes DROP CONSTRAINT chk_otp_codes_type;
ALTER TABLE otp_codes ADD CONSTRAINT chk_otp_codes_type CHECK (
    type IN ('password_reset', 'email_verification')
);

-- Data-loss note: deactivated users stay status='disabled' but lose the marker.
ALTER TABLE users DROP COLUMN IF EXISTS deactivated_at;
