-- Platform organization management (TEC-670) and staff invitation e-mails
-- (TEC-671).

-- Export and bulk status changes of the platform organizations list.
INSERT INTO permissions (name, slug) VALUES
    ('Export platform organizations', 'platform.organizations.export'),
    ('Bulk activate platform organizations', 'platform.organizations.bulk.activate'),
    ('Bulk suspend platform organizations', 'platform.organizations.bulk.suspend')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'platform.organizations.export',
    'platform.organizations.bulk.activate',
    'platform.organizations.bulk.suspend'
)
WHERE r.slug = 'super_admin' AND r.scope = 'platform'
ON CONFLICT DO NOTHING;

-- Invitation e-mail. {{inviter}} is the inviting person (tenant member or
-- platform operator; the organization name when unknown), {{link}} the
-- ${PUBLIC_FRONTEND_URL}/invite/{token} page.
INSERT INTO notification_templates (code, channel, language, subject, body, active) VALUES
    ('organizations.invitation', 'email', 'tr', '{{organization}} sizi ekibine davet ediyor',
     'Merhaba,

{{inviter}} sizi {{organization}} ekibine {{roles}} rolüyle davet etti.

Daveti kabul etmek için bağlantıyı açın:
{{link}}

Bu davet {{expires_at}} tarihine kadar geçerlidir ve yalnızca {{email}} adresiyle kabul edilebilir. Bu daveti beklemiyorsanız e-postayı yok sayabilirsiniz.', TRUE),
    ('organizations.invitation', 'email', 'en', '{{organization}} invited you to join their team',
     'Hello,

{{inviter}} invited you to join {{organization}} as {{roles}}.

Open the link to accept the invitation:
{{link}}

The invitation is valid until {{expires_at}} and can only be accepted with {{email}}. If you were not expecting it, you can ignore this e-mail.', TRUE)
ON CONFLICT (code, channel, language) DO NOTHING;
