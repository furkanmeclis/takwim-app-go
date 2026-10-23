-- Drops the uploaded Apple signing key (data loss: re-upload the .p8 or use AUTH_APPLE_* env).
ALTER TABLE oauth_provider_settings
    DROP COLUMN IF EXISTS apple_private_key_enc,
    DROP COLUMN IF EXISTS apple_key_id,
    DROP COLUMN IF EXISTS apple_team_id;
