-- TEC-914: cash and bank accounts, movements, transfers and the daily
-- closing of a till (requirements §10, docs/design/cash-accounts.md).
--
-- cash_accounts: a till (kind cash), a bank account (bank) or an online
-- payment provider's receivable (online). It belongs to a branch, or to
-- the whole business when branch_id is NULL (only members with access to
-- every branch see those). Balance = opening_balance + money in - money out.
--
-- cash_movements: the cash book. Append-only: a wrong movement is reversed
-- (kind reversal, at most once per movement), never edited or deleted. A
-- transfer is two rows (transfer_out on the source, transfer_in on the
-- target) sharing transfer_group, written in one transaction; it is not
-- revenue. Later modules (payments, sales, income / expense, staff
-- payouts) post into the same book with their own kind, a source
-- reference and an idempotency key, so a retried posting is written once.
--
-- cash_closings: the daily closing of a till: the expected balance at the
-- end of the day, the counted amount and the difference (posted as a
-- closing_difference movement so the book follows the count). A closed day
-- locks the account for movements dated on or before it; a permitted user
-- reopens the latest closing (the difference movement is reversed).

CREATE TABLE cash_accounts (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- NULL = business-wide (merkez) account.
    branch_id BIGINT NULL,
    kind VARCHAR(8) NOT NULL CHECK (kind IN ('cash', 'bank', 'online')),
    name VARCHAR(120) NOT NULL,
    currency CHAR(3) NOT NULL DEFAULT 'TRY',
    opening_balance NUMERIC(14, 2) NOT NULL DEFAULT 0,
    opening_date DATE NOT NULL,
    bank_name VARCHAR(120) NOT NULL DEFAULT '',
    iban VARCHAR(34) NOT NULL DEFAULT '',
    -- Online payment provider code (iyzico, paytr ...) of an online account.
    provider VARCHAR(32) NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    status VARCHAR(12) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    sort_order INT NOT NULL DEFAULT 0,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_cash_accounts_cash_opening CHECK (kind <> 'cash' OR opening_balance >= 0),
    CONSTRAINT uq_cash_accounts_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_cash_accounts_organization_id UNIQUE (organization_id, id),
    CONSTRAINT fk_cash_accounts_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id)
);

-- A name is unique per branch (business-wide accounts count as one branch).
CREATE UNIQUE INDEX uq_cash_accounts_branch_name
    ON cash_accounts (organization_id, COALESCE(branch_id, 0), lower(name));

CREATE INDEX idx_cash_accounts_organization_branch ON cash_accounts (organization_id, branch_id, sort_order);

CREATE TRIGGER trg_cash_accounts_updated_at
    BEFORE UPDATE ON cash_accounts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE cash_closings (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    cash_account_id BIGINT NOT NULL,
    -- Copied from the account (branch scope without a join).
    branch_id BIGINT NULL,
    business_date DATE NOT NULL,
    currency CHAR(3) NOT NULL,
    -- Balance at the start of the day and the day's money in / out (before
    -- the difference movement).
    opening_amount NUMERIC(14, 2) NOT NULL,
    total_in NUMERIC(14, 2) NOT NULL,
    total_out NUMERIC(14, 2) NOT NULL,
    expected_amount NUMERIC(14, 2) NOT NULL,
    counted_amount NUMERIC(14, 2) NOT NULL CHECK (counted_amount >= 0),
    difference NUMERIC(14, 2) NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    status VARCHAR(12) NOT NULL DEFAULT 'closed' CHECK (status IN ('closed', 'reopened')),
    closed_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    closed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reopened_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    reopened_at TIMESTAMPTZ NULL,
    reopen_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_cash_closings_difference CHECK (difference = counted_amount - expected_amount),
    CONSTRAINT ck_cash_closings_reopen CHECK ((status = 'reopened') = (reopened_at IS NOT NULL)),
    CONSTRAINT uq_cash_closings_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_cash_closings_organization_id UNIQUE (organization_id, id),
    CONSTRAINT fk_cash_closings_account FOREIGN KEY (organization_id, cash_account_id)
        REFERENCES cash_accounts (organization_id, id),
    CONSTRAINT fk_cash_closings_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id)
);

-- One live closing per account and day.
CREATE UNIQUE INDEX uq_cash_closings_day
    ON cash_closings (organization_id, cash_account_id, business_date)
    WHERE status = 'closed';

CREATE INDEX idx_cash_closings_account ON cash_closings (organization_id, cash_account_id, business_date DESC);

CREATE TRIGGER trg_cash_closings_updated_at
    BEFORE UPDATE ON cash_closings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE cash_movements (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    cash_account_id BIGINT NOT NULL,
    -- deposit / withdrawal: manual money in / out from the panel.
    -- transfer_in / transfer_out: the two legs of a transfer.
    -- closing_difference: over / short of a daily closing.
    -- reversal: the exact opposite of one earlier movement.
    -- collection, refund, income, expense, staff_payment: posted by the
    -- payment, sales, income / expense and staff payout modules.
    kind VARCHAR(20) NOT NULL CHECK (kind IN (
        'deposit', 'withdrawal', 'transfer_in', 'transfer_out', 'closing_difference', 'reversal',
        'collection', 'refund', 'income', 'expense', 'staff_payment'
    )),
    direction VARCHAR(4) NOT NULL CHECK (direction IN ('in', 'out')),
    amount NUMERIC(14, 2) NOT NULL CHECK (amount > 0),
    currency CHAR(3) NOT NULL,
    movement_date DATE NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    -- Copied from the account (branch scope without a join).
    branch_id BIGINT NULL,
    transfer_group UUID NULL,
    counter_account_id BIGINT NULL,
    reverses_movement_id BIGINT NULL,
    closing_id BIGINT NULL,
    -- The record of another module this movement belongs to
    -- (source_type 'payment' + its uuid ...).
    source_type VARCHAR(32) NOT NULL DEFAULT '',
    source_uuid UUID NULL,
    -- Set by modules that post automatically; a retried posting with the
    -- same key returns the first movement.
    idempotency_key VARCHAR(128) NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_cash_movements_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_cash_movements_organization_id UNIQUE (organization_id, id),
    CONSTRAINT ck_cash_movements_direction CHECK (
        kind IN ('closing_difference', 'reversal')
        OR (kind IN ('deposit', 'transfer_in', 'collection', 'income') AND direction = 'in')
        OR (kind IN ('withdrawal', 'transfer_out', 'refund', 'expense', 'staff_payment') AND direction = 'out')
    ),
    CONSTRAINT ck_cash_movements_transfer CHECK (
        (kind IN ('transfer_in', 'transfer_out')) = (transfer_group IS NOT NULL AND counter_account_id IS NOT NULL)
    ),
    CONSTRAINT ck_cash_movements_reversal CHECK ((kind = 'reversal') = (reverses_movement_id IS NOT NULL)),
    CONSTRAINT ck_cash_movements_closing CHECK ((kind = 'closing_difference') = (closing_id IS NOT NULL)),
    CONSTRAINT fk_cash_movements_account FOREIGN KEY (organization_id, cash_account_id)
        REFERENCES cash_accounts (organization_id, id),
    CONSTRAINT fk_cash_movements_counter FOREIGN KEY (organization_id, counter_account_id)
        REFERENCES cash_accounts (organization_id, id),
    CONSTRAINT fk_cash_movements_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id),
    CONSTRAINT fk_cash_movements_reverses FOREIGN KEY (organization_id, reverses_movement_id)
        REFERENCES cash_movements (organization_id, id),
    CONSTRAINT fk_cash_movements_closing FOREIGN KEY (organization_id, closing_id)
        REFERENCES cash_closings (organization_id, id)
);

-- A movement is reversed at most once.
CREATE UNIQUE INDEX uq_cash_movements_reverses
    ON cash_movements (organization_id, reverses_movement_id)
    WHERE reverses_movement_id IS NOT NULL;
CREATE UNIQUE INDEX uq_cash_movements_idempotency
    ON cash_movements (organization_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
CREATE INDEX idx_cash_movements_account_date ON cash_movements (organization_id, cash_account_id, movement_date, id);
CREATE INDEX idx_cash_movements_branch_date ON cash_movements (organization_id, branch_id, movement_date);
CREATE INDEX idx_cash_movements_transfer ON cash_movements (organization_id, transfer_group)
    WHERE transfer_group IS NOT NULL;
CREATE INDEX idx_cash_movements_source ON cash_movements (organization_id, source_type, source_uuid)
    WHERE source_uuid IS NOT NULL;

-- Append-only; only an organization deletion (cascade) removes rows.
CREATE FUNCTION cash_movements_append_only() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' AND pg_trigger_depth() > 1 THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION 'cash movements are append-only (post a reversal)'
        USING ERRCODE = 'check_violation', CONSTRAINT = 'cash_movements_append_only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_cash_movements_append_only
    BEFORE UPDATE OR DELETE ON cash_movements
    FOR EACH ROW EXECUTE FUNCTION cash_movements_append_only();

-- Insert rules (backstop of the use case checks): a reversal is the exact
-- opposite of one movement of the same account, and no movement is dated
-- on or before a closed day of its account.
CREATE FUNCTION cash_movements_insert_rules() RETURNS trigger AS $$
DECLARE
    orig cash_movements%ROWTYPE;
BEGIN
    IF NEW.kind = 'reversal' THEN
        SELECT * INTO orig FROM cash_movements
         WHERE organization_id = NEW.organization_id AND id = NEW.reverses_movement_id;
        IF NOT FOUND OR orig.kind = 'reversal' OR orig.cash_account_id <> NEW.cash_account_id
           OR orig.direction = NEW.direction OR orig.amount <> NEW.amount OR orig.currency <> NEW.currency THEN
            RAISE EXCEPTION 'a reversal is the exact opposite of one movement of the same account'
                USING ERRCODE = 'check_violation', CONSTRAINT = 'ck_cash_movements_reversal';
        END IF;
    END IF;
    -- The difference movement of a closing is written with the closing.
    IF NEW.kind <> 'closing_difference' AND EXISTS (
        SELECT 1 FROM cash_closings c
         WHERE c.organization_id = NEW.organization_id AND c.cash_account_id = NEW.cash_account_id
           AND c.status = 'closed' AND c.business_date >= NEW.movement_date
    ) THEN
        RAISE EXCEPTION 'the day is closed for this account'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'cash_movements_day_closed';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_cash_movements_insert_rules
    BEFORE INSERT ON cash_movements
    FOR EACH ROW EXECUTE FUNCTION cash_movements_insert_rules();

INSERT INTO permissions (name, slug) VALUES
    ('Read cash and bank accounts', 'tenant.cash.read'),
    ('Post cash movements and transfers', 'tenant.cash.write'),
    ('Manage cash and bank accounts', 'tenant.cash.manage'),
    ('Close the day of a till', 'tenant.cash.close'),
    ('Reopen a closed day', 'tenant.cash.reopen'),
    ('Export cash accounts and movements', 'tenant.cash.export'),
    ('Bulk archive cash accounts', 'tenant.cash.bulk.archive'),
    ('Bulk activate cash accounts', 'tenant.cash.bulk.activate')
ON CONFLICT (slug) DO NOTHING;

-- Owner and admin: everything. Branch manager: the accounts of the
-- assigned branches, including the daily closing, but no reopening.
-- Reception and staff get nothing by default (a custom role can grant
-- read / write / close).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug IN (
    'tenant.cash.read', 'tenant.cash.write', 'tenant.cash.manage', 'tenant.cash.close', 'tenant.cash.reopen',
    'tenant.cash.export', 'tenant.cash.bulk.archive', 'tenant.cash.bulk.activate')
WHERE r.slug IN ('owner', 'admin') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug IN (
    'tenant.cash.read', 'tenant.cash.write', 'tenant.cash.manage', 'tenant.cash.close', 'tenant.cash.export')
WHERE r.slug = 'sube_yoneticisi' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
