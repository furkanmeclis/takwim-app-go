-- Sign in with Apple signing key (.p8) uploaded from the platform admin panel.
-- Used to build ES256 client secrets for the web (NextAuth) client and for
-- native code exchange / token revocation. Env AUTH_APPLE_* stays a fallback.
-- apple_private_key_enc is encrypted with the app SecretBox (ENCRYPTION_KEY),
-- like client_secret_enc. Only meaningful on the 'apple' row.
ALTER TABLE oauth_provider_settings
    ADD COLUMN apple_team_id         TEXT NOT NULL DEFAULT '',
    ADD COLUMN apple_key_id          TEXT NOT NULL DEFAULT '',
    ADD COLUMN apple_private_key_enc TEXT NULL;
