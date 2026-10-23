DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug = 'tenant.integrations.manage');
DELETE FROM permissions WHERE slug = 'tenant.integrations.manage';

DROP TABLE IF EXISTS integration_attempts;
DROP TABLE IF EXISTS integration_jobs;
DROP TABLE IF EXISTS integration_accounts;
