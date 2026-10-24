-- Refund follow-through (TEC-978, docs/design/payments.md "Refunds").
--
-- reservations.payment_status 'kept': a person closed a refund to make (a
-- paid visit cancelled, a late online payment, a refund that failed)
-- without paying the money back, with a reason in the visit's history
-- (event payment_kept): the customer agreed to leave it, or it counts for
-- the next visit. The money stays on the customer's ledger as a credit; a
-- later refund still pays it back.
--
-- refunds.provider_checks / next_check_at: a provider refund whose result
-- is unknown (provider_status uncertain) is asked about again by the
-- worker through the provider's refund status query, never sent again.
-- next_check_at is when the next query is due (NULL: at once); every
-- query counts in provider_checks and pushes the next one further.

ALTER TABLE reservations DROP CONSTRAINT reservations_payment_status_check;
ALTER TABLE reservations ADD CONSTRAINT reservations_payment_status_check
    CHECK (payment_status IN (
        'on_site', 'online_pending', 'paid', 'refund_pending', 'partially_refunded', 'refunded', 'failed', 'kept'
    ));

ALTER TABLE refunds
    ADD COLUMN provider_checks INT NOT NULL DEFAULT 0 CHECK (provider_checks >= 0),
    ADD COLUMN provider_checked_at TIMESTAMPTZ NULL,
    ADD COLUMN next_check_at TIMESTAMPTZ NULL;

CREATE INDEX idx_refunds_uncertain_due ON refunds (next_check_at NULLS FIRST, id)
    WHERE status = 'requested' AND provider_status = 'uncertain';
