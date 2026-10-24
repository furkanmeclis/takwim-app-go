-- TEC-916: sale lines and the customer ledger (requirements §10, §11;
-- docs/design/sales-ledger.md). Revenue (ciro) and collection (tahsilat)
-- are kept apart:
--
-- sale_lines: one row per service delivered (a reservation line of a
-- completed visit) or product handed over (a stock sale). It keeps the
-- quantity, unit price, discount and net amount as they were, the staff
-- member, the branch and the business day. Revenue of a day = the net
-- amounts of the rows dated that day. Append-only: a wrong line is never
-- edited or deleted; it is reversed (kind reversal, the exact negative of
-- one sale line, at most once) and, for a correction, a replacement sale
-- line follows with the same accrual key (replaces_line_id). A key holds at
-- most one live (unreversed) sale line, so one service or product is never
-- revenue twice.
--
-- ledger_entries: what the customer owes and paid. A sale line posts a
-- debit (kind sale), money taken posts a credit (kind collection, with its
-- cash movement), money paid back a debit (kind refund). Money taken
-- before the service (online prepayment) is a collection on that day; the
-- service becomes revenue on the day it is delivered: the two never add up
-- to double income. Balance = debits - credits. Append-only as well: a
-- wrong entry gets a reversal (opposite direction, same amount, at most
-- once). Later modules (payments TEC-912, refunds TEC-913, staff shares
-- TEC-917) post through the sales service with an idempotency key.

CREATE TABLE sale_lines (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    branch_id BIGINT NOT NULL,
    -- NULL for a counter sale without a customer.
    customer_id BIGINT NULL,
    -- sale: a service delivered / product handed over (or its replacement
    -- after a correction). reversal: the exact negative of one sale line.
    kind VARCHAR(10) NOT NULL CHECK (kind IN ('sale', 'reversal')),
    item_type VARCHAR(10) NOT NULL CHECK (item_type IN ('service', 'product')),
    -- reservation (reservation_id + line_index) or stock_movement
    -- (source_uuid = the stock movement).
    source_type VARCHAR(20) NOT NULL CHECK (source_type IN ('reservation', 'stock_movement')),
    source_uuid UUID NULL,
    reservation_id BIGINT NULL,
    line_index INT NULL,
    -- Catalog references (NULL once the record is removed; the snapshot
    -- below stays).
    service_id BIGINT NULL,
    product_id BIGINT NULL,
    variant_id BIGINT NULL,
    staff_id BIGINT NULL,
    description VARCHAR(240) NOT NULL,
    quantity INT NOT NULL CHECK (quantity > 0),
    unit_price NUMERIC(14, 2) NOT NULL CHECK (unit_price >= 0),
    -- Signed: positive on a sale, the negated values on a reversal.
    gross_amount NUMERIC(14, 2) NOT NULL,
    discount_amount NUMERIC(14, 2) NOT NULL,
    net_amount NUMERIC(14, 2) NOT NULL,
    currency CHAR(3) NOT NULL,
    -- Business day the line counts as revenue.
    sale_date DATE NOT NULL,
    -- What was sold: reservation:<id>:line:<index>, stock_movement:<uuid>.
    accrual_key VARCHAR(128) NOT NULL,
    accrual_generation INT NOT NULL DEFAULT 0,
    reverses_line_id BIGINT NULL,
    replaces_line_id BIGINT NULL,
    reason TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_sale_lines_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_sale_lines_organization_id UNIQUE (organization_id, id),
    CONSTRAINT ck_sale_lines_net CHECK (net_amount = gross_amount - discount_amount),
    CONSTRAINT ck_sale_lines_sign CHECK (
        (kind = 'sale' AND gross_amount = unit_price * quantity AND discount_amount >= 0
            AND discount_amount <= gross_amount)
        OR (kind = 'reversal' AND gross_amount <= 0 AND discount_amount <= 0 AND net_amount <= 0)
    ),
    CONSTRAINT ck_sale_lines_reversal CHECK ((kind = 'reversal') = (reverses_line_id IS NOT NULL)),
    CONSTRAINT ck_sale_lines_replaces CHECK (kind = 'sale' OR replaces_line_id IS NULL),
    CONSTRAINT ck_sale_lines_source CHECK (
        (source_type = 'reservation' AND reservation_id IS NOT NULL AND line_index IS NOT NULL)
        OR (source_type = 'stock_movement' AND source_uuid IS NOT NULL)
    ),
    CONSTRAINT fk_sale_lines_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id),
    CONSTRAINT fk_sale_lines_customer FOREIGN KEY (organization_id, customer_id)
        REFERENCES customers (organization_id, id),
    CONSTRAINT fk_sale_lines_reservation FOREIGN KEY (organization_id, reservation_id)
        REFERENCES reservations (organization_id, id),
    CONSTRAINT fk_sale_lines_service FOREIGN KEY (organization_id, service_id)
        REFERENCES services (organization_id, id) ON DELETE SET NULL (service_id),
    CONSTRAINT fk_sale_lines_product FOREIGN KEY (organization_id, product_id)
        REFERENCES products (organization_id, id) ON DELETE SET NULL (product_id),
    CONSTRAINT fk_sale_lines_variant FOREIGN KEY (organization_id, variant_id)
        REFERENCES product_variants (organization_id, id) ON DELETE SET NULL (variant_id),
    CONSTRAINT fk_sale_lines_staff FOREIGN KEY (organization_id, staff_id)
        REFERENCES staff (organization_id, id) ON DELETE SET NULL (staff_id),
    CONSTRAINT fk_sale_lines_reverses FOREIGN KEY (organization_id, reverses_line_id)
        REFERENCES sale_lines (organization_id, id),
    CONSTRAINT fk_sale_lines_replaces FOREIGN KEY (organization_id, replaces_line_id)
        REFERENCES sale_lines (organization_id, id)
);

-- A line is reversed at most once and replaced at most once.
CREATE UNIQUE INDEX uq_sale_lines_reverses ON sale_lines (organization_id, reverses_line_id)
    WHERE reverses_line_id IS NOT NULL;
CREATE UNIQUE INDEX uq_sale_lines_replaces ON sale_lines (organization_id, replaces_line_id)
    WHERE replaces_line_id IS NOT NULL;
-- Two concurrent postings of the same key cannot both win.
CREATE UNIQUE INDEX uq_sale_lines_accrual ON sale_lines (organization_id, accrual_key, accrual_generation)
    WHERE kind = 'sale';
CREATE INDEX idx_sale_lines_branch_date ON sale_lines (organization_id, branch_id, sale_date);
CREATE INDEX idx_sale_lines_date ON sale_lines (organization_id, sale_date, id);
CREATE INDEX idx_sale_lines_reservation ON sale_lines (organization_id, reservation_id)
    WHERE reservation_id IS NOT NULL;
CREATE INDEX idx_sale_lines_customer ON sale_lines (organization_id, customer_id)
    WHERE customer_id IS NOT NULL;
CREATE INDEX idx_sale_lines_staff ON sale_lines (organization_id, staff_id, sale_date)
    WHERE staff_id IS NOT NULL;

-- Append-only. The only updates are catalog references and the creator
-- becoming NULL (the record was removed); only an organization deletion
-- (cascade) removes rows.
CREATE FUNCTION sale_lines_append_only() RETURNS trigger AS $$
DECLARE
    refs TEXT[] := ARRAY['service_id', 'product_id', 'variant_id', 'staff_id', 'created_by'];
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF pg_trigger_depth() > 1 THEN
            RETURN OLD;
        END IF;
    ELSIF to_jsonb(NEW) - refs = to_jsonb(OLD) - refs
        AND (NEW.service_id IS NULL OR NEW.service_id IS NOT DISTINCT FROM OLD.service_id)
        AND (NEW.product_id IS NULL OR NEW.product_id IS NOT DISTINCT FROM OLD.product_id)
        AND (NEW.variant_id IS NULL OR NEW.variant_id IS NOT DISTINCT FROM OLD.variant_id)
        AND (NEW.staff_id IS NULL OR NEW.staff_id IS NOT DISTINCT FROM OLD.staff_id)
        AND (NEW.created_by IS NULL OR NEW.created_by IS NOT DISTINCT FROM OLD.created_by) THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'sale lines are append-only (post a reversal)'
        USING ERRCODE = 'check_violation', CONSTRAINT = 'sale_lines_append_only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_sale_lines_append_only
    BEFORE UPDATE OR DELETE ON sale_lines
    FOR EACH ROW EXECUTE FUNCTION sale_lines_append_only();

-- Insert rules (backstop of the use case): a sale line needs a key without
-- a live sale line and gets the next generation of it; a replacement names
-- a reversed line of the same key; a reversal is the exact negative of one
-- sale line.
CREATE FUNCTION sale_lines_insert_rules() RETURNS trigger AS $$
DECLARE
    orig sale_lines%ROWTYPE;
BEGIN
    IF NEW.kind = 'sale' THEN
        PERFORM pg_advisory_xact_lock(hashtextextended('sale_lines:' || NEW.organization_id || ':' || NEW.accrual_key, 0));
        IF EXISTS (
            SELECT 1 FROM sale_lines s
             WHERE s.organization_id = NEW.organization_id AND s.accrual_key = NEW.accrual_key AND s.kind = 'sale'
               AND NOT EXISTS (SELECT 1 FROM sale_lines r
                                WHERE r.organization_id = s.organization_id AND r.reverses_line_id = s.id)
        ) THEN
            RAISE EXCEPTION 'the item has a live sale line already'
                USING ERRCODE = 'unique_violation', CONSTRAINT = 'uq_sale_lines_accrual';
        END IF;
        IF NEW.replaces_line_id IS NOT NULL AND NOT EXISTS (
            SELECT 1 FROM sale_lines s
             WHERE s.organization_id = NEW.organization_id AND s.id = NEW.replaces_line_id
               AND s.kind = 'sale' AND s.accrual_key = NEW.accrual_key
        ) THEN
            RAISE EXCEPTION 'a replacement names a sale line of the same item'
                USING ERRCODE = 'check_violation', CONSTRAINT = 'ck_sale_lines_replaces';
        END IF;
        SELECT COUNT(*) INTO NEW.accrual_generation FROM sale_lines s
         WHERE s.organization_id = NEW.organization_id AND s.accrual_key = NEW.accrual_key AND s.kind = 'sale';
    ELSE
        SELECT * INTO orig FROM sale_lines
         WHERE organization_id = NEW.organization_id AND id = NEW.reverses_line_id;
        IF NOT FOUND OR orig.kind <> 'sale' OR orig.branch_id <> NEW.branch_id
           OR orig.customer_id IS DISTINCT FROM NEW.customer_id OR orig.currency <> NEW.currency
           OR orig.accrual_key <> NEW.accrual_key OR orig.item_type <> NEW.item_type
           OR orig.quantity <> NEW.quantity OR orig.unit_price <> NEW.unit_price
           OR orig.gross_amount <> -NEW.gross_amount OR orig.discount_amount <> -NEW.discount_amount
           OR orig.net_amount <> -NEW.net_amount THEN
            RAISE EXCEPTION 'a reversal is the exact negative of one sale line'
                USING ERRCODE = 'check_violation', CONSTRAINT = 'ck_sale_lines_reversal';
        END IF;
        NEW.accrual_generation := orig.accrual_generation;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_sale_lines_insert_rules
    BEFORE INSERT ON sale_lines
    FOR EACH ROW EXECUTE FUNCTION sale_lines_insert_rules();

CREATE TABLE ledger_entries (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    branch_id BIGINT NOT NULL,
    customer_id BIGINT NULL,
    reservation_id BIGINT NULL,
    -- sale: debit of a sale line. collection: money taken (credit).
    -- refund: money paid back (debit). reversal: the opposite of one entry.
    kind VARCHAR(12) NOT NULL CHECK (kind IN ('sale', 'collection', 'refund', 'reversal')),
    direction VARCHAR(6) NOT NULL CHECK (direction IN ('debit', 'credit')),
    amount NUMERIC(14, 2) NOT NULL CHECK (amount > 0),
    currency CHAR(3) NOT NULL,
    entry_date DATE NOT NULL,
    -- The sale line a sale entry (or the reversal of one) posts.
    sale_line_id BIGINT NULL,
    -- How the money moved (collections and refunds).
    method VARCHAR(12) NOT NULL DEFAULT ''
        CHECK (method IN ('', 'cash', 'card', 'transfer', 'online', 'other')),
    cash_movement_id BIGINT NULL,
    -- The collection a refund pays back.
    refunds_entry_id BIGINT NULL,
    reverses_entry_id BIGINT NULL,
    -- The record of another module the entry belongs to (a payment ...).
    source_type VARCHAR(32) NOT NULL DEFAULT '',
    source_uuid UUID NULL,
    idempotency_key VARCHAR(128) NULL,
    description TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_ledger_entries_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_ledger_entries_organization_id UNIQUE (organization_id, id),
    CONSTRAINT ck_ledger_entries_direction CHECK (
        kind = 'reversal'
        OR (kind IN ('sale', 'refund') AND direction = 'debit')
        OR (kind = 'collection' AND direction = 'credit')
    ),
    CONSTRAINT ck_ledger_entries_sale CHECK (kind <> 'sale' OR sale_line_id IS NOT NULL),
    CONSTRAINT ck_ledger_entries_reversal CHECK ((kind = 'reversal') = (reverses_entry_id IS NOT NULL)),
    CONSTRAINT ck_ledger_entries_refunds CHECK (kind = 'refund' OR refunds_entry_id IS NULL),
    CONSTRAINT fk_ledger_entries_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id),
    CONSTRAINT fk_ledger_entries_customer FOREIGN KEY (organization_id, customer_id)
        REFERENCES customers (organization_id, id),
    CONSTRAINT fk_ledger_entries_reservation FOREIGN KEY (organization_id, reservation_id)
        REFERENCES reservations (organization_id, id),
    CONSTRAINT fk_ledger_entries_sale_line FOREIGN KEY (organization_id, sale_line_id)
        REFERENCES sale_lines (organization_id, id),
    CONSTRAINT fk_ledger_entries_cash_movement FOREIGN KEY (organization_id, cash_movement_id)
        REFERENCES cash_movements (organization_id, id),
    CONSTRAINT fk_ledger_entries_refunds FOREIGN KEY (organization_id, refunds_entry_id)
        REFERENCES ledger_entries (organization_id, id),
    CONSTRAINT fk_ledger_entries_reverses FOREIGN KEY (organization_id, reverses_entry_id)
        REFERENCES ledger_entries (organization_id, id)
);

-- One entry per sale line, one reversal per entry, one entry per key.
CREATE UNIQUE INDEX uq_ledger_entries_sale_line ON ledger_entries (organization_id, sale_line_id)
    WHERE sale_line_id IS NOT NULL;
CREATE UNIQUE INDEX uq_ledger_entries_reverses ON ledger_entries (organization_id, reverses_entry_id)
    WHERE reverses_entry_id IS NOT NULL;
CREATE UNIQUE INDEX uq_ledger_entries_idempotency ON ledger_entries (organization_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
CREATE INDEX idx_ledger_entries_branch_date ON ledger_entries (organization_id, branch_id, entry_date);
CREATE INDEX idx_ledger_entries_date ON ledger_entries (organization_id, entry_date, id);
CREATE INDEX idx_ledger_entries_customer ON ledger_entries (organization_id, customer_id)
    WHERE customer_id IS NOT NULL;
CREATE INDEX idx_ledger_entries_reservation ON ledger_entries (organization_id, reservation_id)
    WHERE reservation_id IS NOT NULL;
CREATE INDEX idx_ledger_entries_refunds ON ledger_entries (organization_id, refunds_entry_id)
    WHERE refunds_entry_id IS NOT NULL;
CREATE INDEX idx_ledger_entries_source ON ledger_entries (organization_id, source_type, source_uuid)
    WHERE source_uuid IS NOT NULL;

-- Append-only; the creator may become NULL (user removed), an
-- organization deletion (cascade) removes rows.
CREATE FUNCTION ledger_entries_append_only() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF pg_trigger_depth() > 1 THEN
            RETURN OLD;
        END IF;
    ELSIF NEW.created_by IS NULL AND to_jsonb(NEW) - 'created_by' = to_jsonb(OLD) - 'created_by' THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'ledger entries are append-only (post a reversal)'
        USING ERRCODE = 'check_violation', CONSTRAINT = 'ledger_entries_append_only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_ledger_entries_append_only
    BEFORE UPDATE OR DELETE ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION ledger_entries_append_only();

-- Insert rules: a sale entry mirrors its sale line (debit of the net
-- amount, same branch / customer / reservation / currency); a reversal is
-- the opposite of one entry of the same party (a sale entry is reversed
-- only together with its sale line); a refund pays back a collection of
-- the same party and currency.
CREATE FUNCTION ledger_entries_insert_rules() RETURNS trigger AS $$
DECLARE
    line sale_lines%ROWTYPE;
    orig ledger_entries%ROWTYPE;
BEGIN
    IF NEW.kind = 'sale' THEN
        SELECT * INTO line FROM sale_lines WHERE organization_id = NEW.organization_id AND id = NEW.sale_line_id;
        IF NOT FOUND OR line.kind <> 'sale' OR line.net_amount <> NEW.amount OR line.branch_id <> NEW.branch_id
           OR line.customer_id IS DISTINCT FROM NEW.customer_id
           OR line.reservation_id IS DISTINCT FROM NEW.reservation_id OR line.currency <> NEW.currency THEN
            RAISE EXCEPTION 'a sale entry mirrors one sale line'
                USING ERRCODE = 'check_violation', CONSTRAINT = 'ck_ledger_entries_sale';
        END IF;
    ELSIF NEW.kind = 'reversal' THEN
        SELECT * INTO orig FROM ledger_entries WHERE organization_id = NEW.organization_id AND id = NEW.reverses_entry_id;
        IF NOT FOUND OR orig.kind = 'reversal' OR orig.direction = NEW.direction OR orig.amount <> NEW.amount
           OR orig.currency <> NEW.currency OR orig.branch_id <> NEW.branch_id
           OR orig.customer_id IS DISTINCT FROM NEW.customer_id
           OR orig.reservation_id IS DISTINCT FROM NEW.reservation_id THEN
            RAISE EXCEPTION 'a reversal is the opposite of one entry'
                USING ERRCODE = 'check_violation', CONSTRAINT = 'ck_ledger_entries_reversal';
        END IF;
        IF orig.kind = 'sale' AND NOT EXISTS (
            SELECT 1 FROM sale_lines s
             WHERE s.organization_id = NEW.organization_id AND s.id = NEW.sale_line_id
               AND s.reverses_line_id = orig.sale_line_id
        ) THEN
            RAISE EXCEPTION 'a sale entry is reversed with its sale line'
                USING ERRCODE = 'check_violation', CONSTRAINT = 'ck_ledger_entries_reversal';
        END IF;
    ELSIF NEW.kind = 'refund' AND NEW.refunds_entry_id IS NOT NULL THEN
        SELECT * INTO orig FROM ledger_entries WHERE organization_id = NEW.organization_id AND id = NEW.refunds_entry_id;
        IF NOT FOUND OR orig.kind <> 'collection' OR orig.currency <> NEW.currency
           OR orig.customer_id IS DISTINCT FROM NEW.customer_id
           OR orig.reservation_id IS DISTINCT FROM NEW.reservation_id THEN
            RAISE EXCEPTION 'a refund pays back a collection of the same party'
                USING ERRCODE = 'check_violation', CONSTRAINT = 'ck_ledger_entries_refunds';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_ledger_entries_insert_rules
    BEFORE INSERT ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION ledger_entries_insert_rules();
