-- Online payments (TEC-912, docs/design/payments.md).
--
-- payments: one online full-payment attempt of a reservation on the hosted
-- page of the business's own payment provider (integration_accounts, kind
-- payment). The money is recorded only when the provider's signed server
-- notification arrives: then the attempt is succeeded, a ledger
-- collection (and its cash movement into the provider's receivable cash
-- account, kind online) is written and the reservation is told, all in one
-- transaction. A browser return changes nothing.
--
-- order_ref is our reference at the provider (the uuid without dashes:
-- PayTR takes letters and digits only). It is unique across businesses on
-- purpose: an incoming notification names only it, and the business (and
-- its secret to verify the notification) is found from this row.
--
-- Money taken at the business (cash, card, transfer) needs no row here: it
-- is a ledger collection straight away.

CREATE TABLE payments (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    branch_id BIGINT NOT NULL,
    reservation_id BIGINT NOT NULL,
    -- The provider account the page was opened with (its secret verifies
    -- the notification even after the business switched accounts).
    integration_account_id BIGINT NULL,
    -- The provider's receivable account the collection goes into.
    cash_account_id BIGINT NOT NULL,
    provider VARCHAR(24) NOT NULL,
    order_ref VARCHAR(64) NOT NULL CHECK (order_ref ~ '^[A-Za-z0-9]+$'),
    amount NUMERIC(14, 2) NOT NULL CHECK (amount > 0),
    currency CHAR(3) NOT NULL,
    -- pending: the page is open; succeeded / failed: the verified result.
    status VARCHAR(12) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'succeeded', 'failed')),
    page_url TEXT NOT NULL DEFAULT '',
    provider_token VARCHAR(200) NOT NULL DEFAULT '',
    provider_payment_id VARCHAR(200) NOT NULL DEFAULT '',
    -- What the customer paid (installment interest included).
    paid_amount NUMERIC(14, 2) NULL,
    failure_reason VARCHAR(500) NOT NULL DEFAULT '',
    -- What the reservation did with a succeeded payment: confirmed (the
    -- request became a booking), recorded, late (the visit was cancelled
    -- before: refund pending, the visit is not reopened).
    outcome VARCHAR(12) NOT NULL DEFAULT '' CHECK (outcome IN ('', 'confirmed', 'recorded', 'late')),
    ledger_entry_id BIGINT NULL,
    -- The page is not offered after this instant (the payment window).
    expires_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_payments_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_payments_organization_id UNIQUE (organization_id, id),
    CONSTRAINT ck_payments_succeeded CHECK (status <> 'succeeded' OR (ledger_entry_id IS NOT NULL AND outcome <> '')),
    CONSTRAINT fk_payments_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id),
    CONSTRAINT fk_payments_reservation FOREIGN KEY (organization_id, reservation_id)
        REFERENCES reservations (organization_id, id),
    CONSTRAINT fk_payments_integration_account FOREIGN KEY (organization_id, integration_account_id)
        REFERENCES integration_accounts (organization_id, id) ON DELETE SET NULL (integration_account_id),
    CONSTRAINT fk_payments_cash_account FOREIGN KEY (organization_id, cash_account_id)
        REFERENCES cash_accounts (organization_id, id),
    CONSTRAINT fk_payments_ledger_entry FOREIGN KEY (organization_id, ledger_entry_id)
        REFERENCES ledger_entries (organization_id, id)
);

-- Global by design (see above): a notification resolves its business here.
CREATE UNIQUE INDEX uq_payments_order_ref ON payments (order_ref);
CREATE INDEX idx_payments_organization_reservation ON payments (organization_id, reservation_id, created_at DESC);
CREATE INDEX idx_payments_organization_status ON payments (organization_id, status, created_at DESC);

CREATE TRIGGER trg_payments_updated_at
    BEFORE UPDATE ON payments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
