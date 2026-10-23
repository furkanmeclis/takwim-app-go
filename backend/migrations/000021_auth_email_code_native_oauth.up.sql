-- Email one-time-code login, native (mobile) OAuth link flow and self-service
-- account deactivation. Deactivation never deletes rows: the user is marked
-- disabled with deactivated_at so every login path can report it distinctly.

ALTER TABLE users ADD COLUMN deactivated_at TIMESTAMPTZ NULL;

ALTER TABLE otp_codes DROP CONSTRAINT chk_otp_codes_type;
ALTER TABLE otp_codes ADD CONSTRAINT chk_otp_codes_type CHECK (
    type IN ('password_reset', 'email_verification', 'login_code', 'oauth_link', 'account_deactivation')
);

-- OAuth client that issued the stored tokens (NULL = the web client from
-- oauth_provider_settings). Needed to revoke Apple refresh tokens.
ALTER TABLE oauth_accounts ADD COLUMN client_id VARCHAR(255) NULL;

INSERT INTO notification_templates (code, channel, language, subject, body, active) VALUES
    ('auth.login_code', 'email', 'tr', 'Giriş kodunuz: {{code}}',
     E'Merhaba {{name}},\n\nGiriş kodunuz: {{code}}\nKod {{expires_minutes}} dakika geçerlidir ve yalnızca bir kez kullanılabilir.\n\nBu isteği siz yapmadıysanız bu e-postayı yok sayabilirsiniz.', TRUE),
    ('auth.login_code', 'email', 'en', 'Your sign-in code: {{code}}',
     E'Hello {{name}},\n\nYour sign-in code is: {{code}}\nThe code is valid for {{expires_minutes}} minutes and can be used once.\n\nIf you did not request this, you can ignore this email.', TRUE),
    ('auth.oauth_link_code', 'email', 'tr', 'Hesap bağlama kodu: {{code}}',
     E'Merhaba {{name}},\n\n{{provider}} hesabınızı mevcut hesabınıza bağlamak için kodunuz: {{code}}\nKod {{expires_minutes}} dakika geçerlidir.\n\nBu isteği siz yapmadıysanız bu e-postayı yok sayın; hesabınıza bir şey bağlanmaz.', TRUE),
    ('auth.oauth_link_code', 'email', 'en', 'Account linking code: {{code}}',
     E'Hello {{name}},\n\nYour code to link your {{provider}} sign-in to your existing account is: {{code}}\nThe code is valid for {{expires_minutes}} minutes.\n\nIf you did not request this, ignore this email; nothing will be linked.', TRUE),
    ('auth.account_deactivation_code', 'email', 'tr', 'Hesap silme onay kodu: {{code}}',
     E'Merhaba {{name}},\n\nHesabınızı silme (pasifleştirme) isteğinizi onaylamak için kodunuz: {{code}}\nKod {{expires_minutes}} dakika geçerlidir.\n\nBu isteği siz yapmadıysanız şifrenizi değiştirmenizi öneririz.', TRUE),
    ('auth.account_deactivation_code', 'email', 'en', 'Account deletion confirmation code: {{code}}',
     E'Hello {{name}},\n\nYour code to confirm deleting (deactivating) your account is: {{code}}\nThe code is valid for {{expires_minutes}} minutes.\n\nIf you did not request this, we recommend changing your password.', TRUE),
    ('auth.account_deactivated', 'email', 'tr', 'Hesabınız silindi',
     E'Merhaba {{name}},\n\nHesabınız isteğiniz üzerine pasifleştirildi ve tüm oturumlarınız kapatıldı. Artık giriş yapamazsınız.\n\nBu işlemi siz yapmadıysanız lütfen destek ekibiyle iletişime geçin.', TRUE),
    ('auth.account_deactivated', 'email', 'en', 'Your account was deleted',
     E'Hello {{name}},\n\nYour account was deactivated at your request and all sessions were signed out. You can no longer sign in.\n\nIf you did not do this, please contact support.', TRUE)
ON CONFLICT (code, channel, language) DO NOTHING;
