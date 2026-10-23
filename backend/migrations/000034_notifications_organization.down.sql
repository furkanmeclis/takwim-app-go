DROP INDEX IF EXISTS notifications_user_unscoped_created_idx;
DROP INDEX IF EXISTS notifications_org_user_created_idx;

ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_organization_id_fkey;
ALTER TABLE notifications
    ADD CONSTRAINT notifications_organization_id_fkey
    FOREIGN KEY (organization_id) REFERENCES organizations (id) ON DELETE SET NULL;
