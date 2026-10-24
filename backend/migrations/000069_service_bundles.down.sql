-- Data-loss note: drops the service bundles and the bundle snapshot of
-- booked lines (their unit prices, the bundle shares, stay as they are).
DROP INDEX IF EXISTS idx_reservation_lines_bundle;
ALTER TABLE reservation_lines
    DROP CONSTRAINT IF EXISTS fk_reservation_lines_bundle,
    DROP COLUMN IF EXISTS list_price,
    DROP COLUMN IF EXISTS bundle_name,
    DROP COLUMN IF EXISTS bundle_id;

DROP TABLE IF EXISTS service_bundle_branches;
DROP TABLE IF EXISTS service_bundle_items;
DROP TABLE IF EXISTS service_bundles;
