-- Organization stamp on notifications (TEC-748). The nullable column exists
-- since 000029 but nothing wrote or read it. Tenant engine jobs (export,
-- import, bulk) now stamp the job's organization; platform and personal
-- notifications keep NULL. The inbox of a tenant panel shows the user's rows
-- of that organization plus the un-stamped ones; the platform shell shows only
-- un-stamped rows.

-- ON DELETE SET NULL would turn an organization's notifications into
-- platform (un-stamped) rows when the organization is purged and leak them
-- into every shell. Delete them with the organization instead, like its jobs.
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_organization_id_fkey;
ALTER TABLE notifications
    ADD CONSTRAINT notifications_organization_id_fkey
    FOREIGN KEY (organization_id) REFERENCES organizations (id) ON DELETE CASCADE;

-- Tenant inbox and unread count: organization first, then the recipient.
CREATE INDEX notifications_org_user_created_idx
    ON notifications (organization_id, user_id, created_at DESC)
    WHERE organization_id IS NOT NULL;

-- Platform inbox and unread count: un-stamped rows of one recipient.
CREATE INDEX notifications_user_unscoped_created_idx
    ON notifications (user_id, created_at DESC)
    WHERE organization_id IS NULL AND user_id IS NOT NULL;
