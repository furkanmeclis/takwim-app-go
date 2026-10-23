ALTER TABLE export_jobs
    ADD COLUMN organization_id BIGINT NULL REFERENCES organizations (id) ON DELETE CASCADE;
CREATE INDEX idx_export_jobs_organization_created
    ON export_jobs (organization_id, created_at DESC)
    WHERE organization_id IS NOT NULL;

ALTER TABLE import_jobs
    ADD COLUMN organization_id BIGINT NULL REFERENCES organizations (id) ON DELETE CASCADE;
CREATE INDEX idx_import_jobs_organization_created
    ON import_jobs (organization_id, created_at DESC)
    WHERE organization_id IS NOT NULL;

ALTER TABLE bulk_jobs
    ADD COLUMN organization_id BIGINT NULL REFERENCES organizations (id) ON DELETE CASCADE;
CREATE INDEX idx_bulk_jobs_organization_created
    ON bulk_jobs (organization_id, created_at DESC)
    WHERE organization_id IS NOT NULL;

ALTER TABLE activity_events
    ADD COLUMN organization_id BIGINT NULL REFERENCES organizations (id) ON DELETE SET NULL;
CREATE INDEX idx_activity_events_organization_created
    ON activity_events (organization_id, created_at DESC)
    WHERE organization_id IS NOT NULL;

ALTER TABLE notifications
    ADD COLUMN organization_id BIGINT NULL REFERENCES organizations (id) ON DELETE SET NULL;
CREATE INDEX idx_notifications_organization_created
    ON notifications (organization_id, created_at DESC)
    WHERE organization_id IS NOT NULL;
