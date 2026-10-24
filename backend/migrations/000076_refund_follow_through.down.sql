-- Data-loss note: visits closed without a refund (payment_status 'kept')
-- go back to refund_pending, so they return to the payment follow-up
-- list; the refund check counters are dropped.

DROP INDEX IF EXISTS idx_refunds_uncertain_due;

ALTER TABLE refunds
    DROP COLUMN IF EXISTS next_check_at,
    DROP COLUMN IF EXISTS provider_checked_at,
    DROP COLUMN IF EXISTS provider_checks;

UPDATE reservations SET payment_status = 'refund_pending' WHERE payment_status = 'kept';

ALTER TABLE reservations DROP CONSTRAINT reservations_payment_status_check;
ALTER TABLE reservations ADD CONSTRAINT reservations_payment_status_check
    CHECK (payment_status IN (
        'on_site', 'online_pending', 'paid', 'refund_pending', 'partially_refunded', 'refunded', 'failed'
    ));
