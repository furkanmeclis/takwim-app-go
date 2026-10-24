-- Refunds and the return of products handed over (TEC-913,
-- docs/design/payments.md "Refunds").
--
-- refunds: one refund of (part of) a ledger collection of a reservation.
-- The three steps are kept apart, as the requirements ask:
--   1. the request (the row: amount, channel, who asked and why);
--   2. the provider's answer (provider_status: succeeded / failed /
--      uncertain when the call's result is unknown; never sent again then);
--   3. the posting to the books (ledger_entry_id + reflected_at: the refund
--      ledger entry and its cash movement out of the account).
-- status sums them up: requested until the money is both returned and
-- posted (succeeded), or failed when the refund did not happen. A manual
-- refund (cash, card or transfer handed back at the business, or a refund
-- made in the provider's own panel) is posted at once; a provider refund
-- (the provider's refund call) when the provider confirms it.
--
-- idempotency_key is the client's key of one refund request: the same key
-- answers the same refund and sends nothing again.

CREATE TABLE refunds (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    branch_id BIGINT NOT NULL,
    reservation_id BIGINT NOT NULL,
    -- The collection paid back (ledger_entries kind collection).
    collection_entry_id BIGINT NOT NULL,
    -- The online payment refunded through its provider.
    payment_id BIGINT NULL,
    integration_account_id BIGINT NULL,
    channel VARCHAR(12) NOT NULL CHECK (channel IN ('provider', 'manual')),
    provider VARCHAR(24) NOT NULL DEFAULT '',
    -- The account the money leaves.
    cash_account_id BIGINT NOT NULL,
    method VARCHAR(16) NOT NULL CHECK (method IN ('cash', 'card', 'transfer', 'online', 'other')),
    amount NUMERIC(14, 2) NOT NULL CHECK (amount > 0),
    currency CHAR(3) NOT NULL,
    status VARCHAR(12) NOT NULL DEFAULT 'requested' CHECK (status IN ('requested', 'succeeded', 'failed')),
    provider_status VARCHAR(12) NOT NULL DEFAULT ''
        CHECK (provider_status IN ('', 'succeeded', 'failed', 'uncertain')),
    provider_refund_id VARCHAR(200) NOT NULL DEFAULT '',
    failure_reason VARCHAR(500) NOT NULL DEFAULT '',
    provider_answered_at TIMESTAMPTZ NULL,
    ledger_entry_id BIGINT NULL,
    reflected_at TIMESTAMPTZ NULL,
    reason VARCHAR(240) NOT NULL DEFAULT '',
    idempotency_key VARCHAR(100) NOT NULL,
    requested_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    -- A person who settled an unknown provider result (after checking the
    -- provider's panel).
    resolved_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    resolution_note VARCHAR(240) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_refunds_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_refunds_organization_id UNIQUE (organization_id, id),
    CONSTRAINT uq_refunds_organization_idempotency UNIQUE (organization_id, idempotency_key),
    CONSTRAINT ck_refunds_posted CHECK (
        (status = 'succeeded') = (ledger_entry_id IS NOT NULL AND reflected_at IS NOT NULL)
    ),
    CONSTRAINT ck_refunds_provider CHECK (channel = 'manual' OR (payment_id IS NOT NULL AND provider <> '')),
    CONSTRAINT fk_refunds_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id),
    CONSTRAINT fk_refunds_reservation FOREIGN KEY (organization_id, reservation_id)
        REFERENCES reservations (organization_id, id),
    CONSTRAINT fk_refunds_collection FOREIGN KEY (organization_id, collection_entry_id)
        REFERENCES ledger_entries (organization_id, id),
    CONSTRAINT fk_refunds_ledger_entry FOREIGN KEY (organization_id, ledger_entry_id)
        REFERENCES ledger_entries (organization_id, id),
    CONSTRAINT fk_refunds_payment FOREIGN KEY (organization_id, payment_id)
        REFERENCES payments (organization_id, id),
    CONSTRAINT fk_refunds_integration_account FOREIGN KEY (organization_id, integration_account_id)
        REFERENCES integration_accounts (organization_id, id) ON DELETE SET NULL (integration_account_id),
    CONSTRAINT fk_refunds_cash_account FOREIGN KEY (organization_id, cash_account_id)
        REFERENCES cash_accounts (organization_id, id)
);

CREATE INDEX idx_refunds_organization_reservation ON refunds (organization_id, reservation_id, created_at DESC);
CREATE INDEX idx_refunds_organization_collection ON refunds (organization_id, collection_entry_id);
CREATE INDEX idx_refunds_organization_open ON refunds (organization_id, status) WHERE status <> 'succeeded';

CREATE TRIGGER trg_refunds_updated_at
    BEFORE UPDATE ON refunds
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- A product handed over is taken back at most once: the return movement
-- names the sale movement it reverses (source sale_return).
CREATE UNIQUE INDEX uq_stock_movements_sale_return ON stock_movements (organization_id, source_uuid)
    WHERE source = 'sale_return';
