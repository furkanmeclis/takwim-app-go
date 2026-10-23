DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug = 'tenant.activity.read');
DELETE FROM permissions WHERE slug = 'tenant.activity.read';

DROP INDEX IF EXISTS idx_activity_events_organization_branch_created;
ALTER TABLE activity_events DROP COLUMN IF EXISTS branch_id;
