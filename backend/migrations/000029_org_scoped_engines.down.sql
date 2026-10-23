ALTER TABLE notifications DROP COLUMN IF EXISTS organization_id;
ALTER TABLE activity_events DROP COLUMN IF EXISTS organization_id;
ALTER TABLE bulk_jobs DROP COLUMN IF EXISTS organization_id;
ALTER TABLE import_jobs DROP COLUMN IF EXISTS organization_id;
ALTER TABLE export_jobs DROP COLUMN IF EXISTS organization_id;
