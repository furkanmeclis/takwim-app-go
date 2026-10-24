DROP TABLE IF EXISTS customer_notification_responses;
DROP INDEX IF EXISTS idx_customer_notification_deliveries_unreported;
DROP INDEX IF EXISTS idx_customer_notification_deliveries_provider_ref;
ALTER TABLE customer_notification_deliveries
    DROP COLUMN IF EXISTS read_at,
    DROP COLUMN IF EXISTS delivered_at;
