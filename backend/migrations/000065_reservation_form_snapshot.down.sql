-- Data-loss note: drops the settings version and the booking form question
-- snapshots of every reservation (the answers in custom_fields stay).
DROP INDEX IF EXISTS idx_reservations_customer_open;
ALTER TABLE reservations
    DROP COLUMN IF EXISTS custom_field_defs,
    DROP COLUMN IF EXISTS settings_version;
