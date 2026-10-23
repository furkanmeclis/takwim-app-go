-- password_set distinguishes a user-chosen password from the random hash that
-- OAuth / native sign-up stores (password_hash stays NOT NULL). Drives
-- has_password on GET /v1/auth/identities and the LAST_SIGN_IN_METHOD guard.
ALTER TABLE users ADD COLUMN password_set BOOLEAN NOT NULL DEFAULT TRUE;

-- Backfill: accounts created by an OAuth sign-up get their provider identity
-- linked right after the user row. A later password reset cannot be told
-- apart here; such users set a password again (any password update flips the
-- flag back to TRUE).
UPDATE users u
SET password_set = FALSE
WHERE EXISTS (
    SELECT 1 FROM oauth_accounts oa
    WHERE oa.user_id = u.id
      AND oa.created_at <= u.created_at + INTERVAL '1 minute'
);
