-- Data-loss note: removes the invitation e-mail templates (and any edits
-- made to them) and the organization export / bulk permissions with their
-- role grants.
DELETE FROM notification_templates WHERE code = 'organizations.invitation';

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'platform.organizations.export',
        'platform.organizations.bulk.activate',
        'platform.organizations.bulk.suspend'
    )
);

DELETE FROM permissions WHERE slug IN (
    'platform.organizations.export',
    'platform.organizations.bulk.activate',
    'platform.organizations.bulk.suspend'
);
