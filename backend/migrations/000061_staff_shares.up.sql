-- TEC-917: staff shares (requirements §11, docs/design/staff-shares.md).
--
-- staff_share_rules: the agreed rate of a staff member. One default rule
-- per item type (service, product: the product sales commission is kept
-- apart from the service rate), optionally a rule for one service that
-- overrides the service default. A rule is a percentage of the line's net
-- amount (after discount) or a fixed amount per unit, and applies from
-- valid_from on: a new rate is a new rule with a later valid_from, so
-- older lines keep the rate of their day.
--
-- staff_share_lines: the share of one sale line (TEC-916), written in the
-- transaction that writes the sale line. It keeps the rule, rate, base and
-- amount as they were (a later rate change never rewrites it). A reversed
-- sale line gets the exact negative share (kind reversal), a corrected one
-- the reversal plus the share of its replacement. Costs and expenses are
-- never deducted from the base. Append-only.
--
-- staff_share_payouts: money paid to a staff member against the shares. A
-- payout is a staff_payment movement of the cash book (it lowers the
-- account balance and what is owed to the person; it is not an expense,
-- so it never counts as a second cost). A wrong payout is reversed with
-- its cash movement. Append-only.

CREATE TABLE staff_share_rules (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    staff_id BIGINT NOT NULL,
    item_type VARCHAR(10) NOT NULL CHECK (item_type IN ('service', 'product')),
    -- NULL = the default of the item type; set = override for one service.
    service_id BIGINT NULL,
    basis VARCHAR(8) NOT NULL CHECK (basis IN ('percent', 'fixed')),
    -- percent: 0–100 with two decimals; fixed: amount per unit sold.
    rate NUMERIC(5, 2) NULL CHECK (rate >= 0 AND rate <= 100),
    fixed_amount NUMERIC(14, 2) NULL CHECK (fixed_amount >= 0),
    valid_from DATE NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_staff_share_rules_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_staff_share_rules_organization_id UNIQUE (organization_id, id),
    CONSTRAINT ck_staff_share_rules_basis CHECK (
        (basis = 'percent' AND rate IS NOT NULL AND fixed_amount IS NULL)
        OR (basis = 'fixed' AND fixed_amount IS NOT NULL AND rate IS NULL)
    ),
    CONSTRAINT ck_staff_share_rules_service CHECK (service_id IS NULL OR item_type = 'service'),
    CONSTRAINT fk_staff_share_rules_staff FOREIGN KEY (organization_id, staff_id)
        REFERENCES staff (organization_id, id),
    CONSTRAINT fk_staff_share_rules_service FOREIGN KEY (organization_id, service_id)
        REFERENCES services (organization_id, id) ON DELETE CASCADE
);

-- One rule per staff member, item type, service and start day.
CREATE UNIQUE INDEX uq_staff_share_rules_slot
    ON staff_share_rules (organization_id, staff_id, item_type, COALESCE(service_id, 0), valid_from);

CREATE INDEX idx_staff_share_rules_staff ON staff_share_rules (organization_id, staff_id, valid_from DESC);

CREATE TRIGGER trg_staff_share_rules_updated_at
    BEFORE UPDATE ON staff_share_rules
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE staff_share_lines (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    branch_id BIGINT NOT NULL,
    staff_id BIGINT NOT NULL,
    -- The sale line (a sale, or the reversal of one) this share belongs to.
    sale_line_id BIGINT NOT NULL,
    -- Copied from the sale line: the visit whose collection makes the share
    -- payable (NULL for a counter sale, payable at once).
    reservation_id BIGINT NULL,
    kind VARCHAR(10) NOT NULL CHECK (kind IN ('share', 'reversal')),
    reverses_share_id BIGINT NULL,
    item_type VARCHAR(10) NOT NULL CHECK (item_type IN ('service', 'product')),
    -- The rule as it was applied (rule_id becomes NULL if it is removed).
    rule_id BIGINT NULL,
    rule_scope VARCHAR(10) NOT NULL CHECK (rule_scope IN ('default', 'service')),
    basis VARCHAR(8) NOT NULL CHECK (basis IN ('percent', 'fixed')),
    rate NUMERIC(5, 2) NULL,
    fixed_amount NUMERIC(14, 2) NULL,
    -- Signed: positive on a share, the negated values on a reversal.
    base_amount NUMERIC(14, 2) NOT NULL,
    share_amount NUMERIC(14, 2) NOT NULL,
    currency CHAR(3) NOT NULL,
    -- The business day of the sale line.
    share_date DATE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_staff_share_lines_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_staff_share_lines_organization_id UNIQUE (organization_id, id),
    CONSTRAINT ck_staff_share_lines_sign CHECK (
        (kind = 'share' AND base_amount >= 0 AND share_amount >= 0 AND share_amount <= base_amount)
        OR (kind = 'reversal' AND base_amount <= 0 AND share_amount <= 0 AND share_amount >= base_amount)
    ),
    CONSTRAINT ck_staff_share_lines_reversal CHECK ((kind = 'reversal') = (reverses_share_id IS NOT NULL)),
    CONSTRAINT fk_staff_share_lines_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id),
    CONSTRAINT fk_staff_share_lines_staff FOREIGN KEY (organization_id, staff_id)
        REFERENCES staff (organization_id, id),
    CONSTRAINT fk_staff_share_lines_sale_line FOREIGN KEY (organization_id, sale_line_id)
        REFERENCES sale_lines (organization_id, id),
    CONSTRAINT fk_staff_share_lines_reservation FOREIGN KEY (organization_id, reservation_id)
        REFERENCES reservations (organization_id, id),
    CONSTRAINT fk_staff_share_lines_reverses FOREIGN KEY (organization_id, reverses_share_id)
        REFERENCES staff_share_lines (organization_id, id),
    CONSTRAINT fk_staff_share_lines_rule FOREIGN KEY (organization_id, rule_id)
        REFERENCES staff_share_rules (organization_id, id) ON DELETE SET NULL (rule_id)
);

-- One share per sale line, one reversal per share.
CREATE UNIQUE INDEX uq_staff_share_lines_sale_line ON staff_share_lines (organization_id, sale_line_id);
CREATE UNIQUE INDEX uq_staff_share_lines_reverses ON staff_share_lines (organization_id, reverses_share_id)
    WHERE reverses_share_id IS NOT NULL;
CREATE INDEX idx_staff_share_lines_staff_date ON staff_share_lines (organization_id, staff_id, share_date);
CREATE INDEX idx_staff_share_lines_branch_date ON staff_share_lines (organization_id, branch_id, share_date);
CREATE INDEX idx_staff_share_lines_reservation ON staff_share_lines (organization_id, reservation_id)
    WHERE reservation_id IS NOT NULL;

-- Append-only; only the rule reference may become NULL (rule removed) and
-- an organization deletion (cascade) removes rows.
CREATE FUNCTION staff_share_lines_append_only() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF pg_trigger_depth() > 1 THEN
            RETURN OLD;
        END IF;
    ELSIF NEW.rule_id IS NULL AND to_jsonb(NEW) - 'rule_id' = to_jsonb(OLD) - 'rule_id' THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'staff shares are append-only (post a reversal)'
        USING ERRCODE = 'check_violation', CONSTRAINT = 'staff_share_lines_append_only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_staff_share_lines_append_only
    BEFORE UPDATE OR DELETE ON staff_share_lines
    FOR EACH ROW EXECUTE FUNCTION staff_share_lines_append_only();

-- Insert rules (backstop of the use case): a share follows its sale line
-- (same branch, staff, currency, day; a share needs a sale, a reversal the
-- reversal of the original's sale line) and a reversal is the exact
-- negative of one share.
CREATE FUNCTION staff_share_lines_insert_rules() RETURNS trigger AS $$
DECLARE
    line sale_lines%ROWTYPE;
    orig staff_share_lines%ROWTYPE;
BEGIN
    SELECT * INTO line FROM sale_lines WHERE organization_id = NEW.organization_id AND id = NEW.sale_line_id;
    IF NOT FOUND OR line.branch_id <> NEW.branch_id OR line.currency <> NEW.currency
       OR line.reservation_id IS DISTINCT FROM NEW.reservation_id OR line.item_type <> NEW.item_type
       OR line.sale_date <> NEW.share_date OR line.net_amount <> NEW.base_amount
       OR (NEW.kind = 'share') <> (line.kind = 'sale') THEN
        RAISE EXCEPTION 'a share follows its sale line'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'fk_staff_share_lines_sale_line';
    END IF;
    IF NEW.kind = 'reversal' THEN
        SELECT * INTO orig FROM staff_share_lines
         WHERE organization_id = NEW.organization_id AND id = NEW.reverses_share_id;
        IF NOT FOUND OR orig.kind <> 'share' OR orig.staff_id <> NEW.staff_id
           OR orig.sale_line_id <> line.reverses_line_id OR orig.share_amount <> -NEW.share_amount
           OR orig.base_amount <> -NEW.base_amount THEN
            RAISE EXCEPTION 'a share reversal is the exact negative of one share'
                USING ERRCODE = 'check_violation', CONSTRAINT = 'ck_staff_share_lines_reversal';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_staff_share_lines_insert_rules
    BEFORE INSERT ON staff_share_lines
    FOR EACH ROW EXECUTE FUNCTION staff_share_lines_insert_rules();

CREATE TABLE staff_share_payouts (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    staff_id BIGINT NOT NULL,
    -- Copied from the cash account (NULL = business-wide account).
    branch_id BIGINT NULL,
    cash_account_id BIGINT NOT NULL,
    cash_movement_id BIGINT NOT NULL,
    -- payout: money paid to the person. reversal: undoes one payout.
    kind VARCHAR(10) NOT NULL CHECK (kind IN ('payout', 'reversal')),
    reverses_payout_id BIGINT NULL,
    amount NUMERIC(14, 2) NOT NULL CHECK (amount > 0),
    currency CHAR(3) NOT NULL,
    payout_date DATE NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    idempotency_key VARCHAR(128) NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_staff_share_payouts_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_staff_share_payouts_organization_id UNIQUE (organization_id, id),
    CONSTRAINT ck_staff_share_payouts_reversal CHECK ((kind = 'reversal') = (reverses_payout_id IS NOT NULL)),
    CONSTRAINT fk_staff_share_payouts_staff FOREIGN KEY (organization_id, staff_id)
        REFERENCES staff (organization_id, id),
    CONSTRAINT fk_staff_share_payouts_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id),
    CONSTRAINT fk_staff_share_payouts_account FOREIGN KEY (organization_id, cash_account_id)
        REFERENCES cash_accounts (organization_id, id),
    CONSTRAINT fk_staff_share_payouts_movement FOREIGN KEY (organization_id, cash_movement_id)
        REFERENCES cash_movements (organization_id, id),
    CONSTRAINT fk_staff_share_payouts_reverses FOREIGN KEY (organization_id, reverses_payout_id)
        REFERENCES staff_share_payouts (organization_id, id)
);

CREATE UNIQUE INDEX uq_staff_share_payouts_reverses ON staff_share_payouts (organization_id, reverses_payout_id)
    WHERE reverses_payout_id IS NOT NULL;
CREATE UNIQUE INDEX uq_staff_share_payouts_idempotency ON staff_share_payouts (organization_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
CREATE UNIQUE INDEX uq_staff_share_payouts_movement ON staff_share_payouts (organization_id, cash_movement_id);
CREATE INDEX idx_staff_share_payouts_staff_date ON staff_share_payouts (organization_id, staff_id, payout_date);
CREATE INDEX idx_staff_share_payouts_date ON staff_share_payouts (organization_id, payout_date, id);

-- Append-only; the creator may become NULL (user removed), an
-- organization deletion (cascade) removes rows.
CREATE FUNCTION staff_share_payouts_append_only() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF pg_trigger_depth() > 1 THEN
            RETURN OLD;
        END IF;
    ELSIF NEW.created_by IS NULL AND to_jsonb(NEW) - 'created_by' = to_jsonb(OLD) - 'created_by' THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'staff payouts are append-only (post a reversal)'
        USING ERRCODE = 'check_violation', CONSTRAINT = 'staff_share_payouts_append_only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_staff_share_payouts_append_only
    BEFORE UPDATE OR DELETE ON staff_share_payouts
    FOR EACH ROW EXECUTE FUNCTION staff_share_payouts_append_only();

-- tenant.earnings.read / .own exist since 000042 (every share, own shares).
-- New: managing the rates and paying shares out.
INSERT INTO permissions (name, slug) VALUES
    ('Manage staff share rates', 'tenant.earnings.manage'),
    ('Pay staff shares', 'tenant.earnings.pay')
ON CONFLICT (slug) DO NOTHING;

-- Owner and admin: both. Branch manager: pays the staff of its branches
-- from their accounts; the rates are the owner's agreement.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug IN ('tenant.earnings.manage', 'tenant.earnings.pay')
WHERE r.slug IN ('owner', 'admin') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug = 'tenant.earnings.pay'
WHERE r.slug = 'sube_yoneticisi' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
