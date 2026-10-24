-- TEC-965: bank statement / payment provider settlement import and
-- reconciliation of a bank or online account (requirements §10).
--
-- cash_statements: one imported document (a bank statement CSV / XLSX or a
-- provider settlement report) of one bank or online account. It carries the
-- period the lines cover and, when the document states it, the balance at
-- the end of the period. A statement is closed by the period closing of
-- its account (a cash_closings row) and reopened with it.
--
-- cash_statement_lines: the rows of the document. Each line is reconciled
-- against the cash book of the account:
--   pending    not decided yet (auto-match found no single candidate)
--   matched    linked to one existing movement of the same amount
--   difference linked to a movement of another amount (gross vs net …)
--   posted     written into the book as a movement (or a transfer for a
--              provider payout) through the cash posting path, with an
--              idempotency key per attempt
--   ignored    left out on purpose (duplicate, noise) with a note
-- Movements stay append-only: undoing a posting writes reversals and the
-- line goes back to pending.

CREATE TABLE cash_statements (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    cash_account_id BIGINT NOT NULL,
    -- Copied from the account (branch scope without a join).
    branch_id BIGINT NULL,
    -- bank: a bank statement; provider: an online payment provider's
    -- settlement (hakediş) report.
    source VARCHAR(8) NOT NULL CHECK (source IN ('bank', 'provider')),
    name VARCHAR(160) NOT NULL,
    period_from DATE NULL,
    period_to DATE NULL,
    -- Balance the document states at the end of the period (bank: the
    -- account balance; provider: the receivable). NULL = not stated.
    closing_balance NUMERIC(14, 2) NULL,
    note TEXT NOT NULL DEFAULT '',
    status VARCHAR(12) NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed')),
    -- The period closing of the account that closed this statement.
    closing_id BIGINT NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_cash_statements_period CHECK (period_from IS NULL OR period_to IS NULL OR period_from <= period_to),
    CONSTRAINT ck_cash_statements_closed CHECK ((status = 'closed') = (closing_id IS NOT NULL)),
    CONSTRAINT uq_cash_statements_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_cash_statements_organization_id UNIQUE (organization_id, id),
    CONSTRAINT fk_cash_statements_account FOREIGN KEY (organization_id, cash_account_id)
        REFERENCES cash_accounts (organization_id, id),
    CONSTRAINT fk_cash_statements_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id),
    CONSTRAINT fk_cash_statements_closing FOREIGN KEY (organization_id, closing_id)
        REFERENCES cash_closings (organization_id, id)
);

CREATE INDEX idx_cash_statements_account ON cash_statements (organization_id, cash_account_id, created_at DESC);
CREATE INDEX idx_cash_statements_branch ON cash_statements (organization_id, branch_id);

CREATE TRIGGER trg_cash_statements_updated_at
    BEFORE UPDATE ON cash_statements
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE cash_statement_lines (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    statement_id BIGINT NOT NULL,
    cash_account_id BIGINT NOT NULL,
    line_no INT NOT NULL,
    line_date DATE NOT NULL,
    direction VARCHAR(4) NOT NULL CHECK (direction IN ('in', 'out')),
    amount NUMERIC(14, 2) NOT NULL CHECK (amount > 0),
    -- Provider commission on the line (posted as an expense when the line
    -- is matched or posted).
    fee NUMERIC(14, 2) NOT NULL DEFAULT 0 CHECK (fee >= 0),
    -- payment / refund / fee / payout (provider -> bank transfer) / other.
    line_type VARCHAR(12) NOT NULL DEFAULT 'other'
        CHECK (line_type IN ('payment', 'refund', 'fee', 'payout', 'other')),
    description TEXT NOT NULL DEFAULT '',
    -- Bank reference / provider transaction id; a repeated reference of the
    -- same direction on the account is refused on import.
    reference VARCHAR(120) NOT NULL DEFAULT '',
    balance_after NUMERIC(14, 2) NULL,
    status VARCHAR(12) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'matched', 'difference', 'posted', 'ignored')),
    match_method VARCHAR(8) NOT NULL DEFAULT '' CHECK (match_method IN ('', 'auto', 'manual')),
    matched_movement_id BIGINT NULL,
    -- Line amount minus movement amount while status = difference.
    difference_amount NUMERIC(14, 2) NOT NULL DEFAULT 0,
    -- The movement written from this line (the out leg of a payout transfer).
    posted_movement_id BIGINT NULL,
    -- The expense movement of the fee, while the line is matched / posted.
    fee_movement_id BIGINT NULL,
    -- Number of posting attempts: the idempotency key of each attempt is
    -- cash_statement_line:{uuid}:{post_count}.
    post_count INT NOT NULL DEFAULT 0,
    note TEXT NOT NULL DEFAULT '',
    resolved_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    resolved_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_cash_statement_lines_matched CHECK ((status IN ('matched', 'difference')) = (matched_movement_id IS NOT NULL)),
    CONSTRAINT ck_cash_statement_lines_posted CHECK ((status = 'posted') = (posted_movement_id IS NOT NULL)),
    CONSTRAINT ck_cash_statement_lines_difference CHECK (status = 'difference' OR difference_amount = 0),
    CONSTRAINT uq_cash_statement_lines_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_cash_statement_lines_organization_id UNIQUE (organization_id, id),
    CONSTRAINT fk_cash_statement_lines_statement FOREIGN KEY (organization_id, statement_id)
        REFERENCES cash_statements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_cash_statement_lines_account FOREIGN KEY (organization_id, cash_account_id)
        REFERENCES cash_accounts (organization_id, id),
    CONSTRAINT fk_cash_statement_lines_matched FOREIGN KEY (organization_id, matched_movement_id)
        REFERENCES cash_movements (organization_id, id),
    CONSTRAINT fk_cash_statement_lines_posted FOREIGN KEY (organization_id, posted_movement_id)
        REFERENCES cash_movements (organization_id, id),
    CONSTRAINT fk_cash_statement_lines_fee FOREIGN KEY (organization_id, fee_movement_id)
        REFERENCES cash_movements (organization_id, id)
);

-- A movement is matched by one line at most.
CREATE UNIQUE INDEX uq_cash_statement_lines_movement
    ON cash_statement_lines (organization_id, matched_movement_id)
    WHERE matched_movement_id IS NOT NULL;
-- A bank reference / provider transaction id is imported once per
-- direction on an account (the same file imported twice adds nothing).
CREATE UNIQUE INDEX uq_cash_statement_lines_reference
    ON cash_statement_lines (organization_id, cash_account_id, direction, reference)
    WHERE reference <> '';
CREATE INDEX idx_cash_statement_lines_statement ON cash_statement_lines (organization_id, statement_id, status, line_no);
CREATE INDEX idx_cash_statement_lines_account_date ON cash_statement_lines (organization_id, cash_account_id, line_date);

CREATE TRIGGER trg_cash_statement_lines_updated_at
    BEFORE UPDATE ON cash_statement_lines
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Bank and online accounts are closed by period against the statement
-- balance, which can be negative (overdraft, chargebacks); the till rule
-- (counted cash >= 0) moves to the use case.
ALTER TABLE cash_closings DROP CONSTRAINT IF EXISTS cash_closings_counted_amount_check;

INSERT INTO permissions (name, slug) VALUES
    ('Import bank statements and settlement reports', 'tenant.cash.import')
ON CONFLICT (slug) DO NOTHING;

-- Owner, admin and branch manager import statements of their accounts;
-- matching and posting lines is tenant.cash.write.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug = 'tenant.cash.import'
WHERE r.slug IN ('owner', 'admin', 'sube_yoneticisi') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
