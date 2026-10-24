-- TEC-974: stuck customer notification attempts.
--
-- An attempt is inserted as 'sending' when the engine claims a channel and
-- gets its result when the send returns. A process that stops in between
-- leaves it 'sending' for good (each channel is tried at most once per
-- dispatch, so it is never sent again). The worker's minute sweep closes
-- such attempts after a while as failed with an unknown outcome; this index
-- keeps that scan small.
CREATE INDEX idx_customer_notification_deliveries_in_flight
    ON customer_notification_deliveries (created_at)
    WHERE status = 'sending';
