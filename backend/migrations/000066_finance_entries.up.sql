-- TEC-915: income and expense records outside sales (requirements §10,
-- docs/design/finance-entries.md).
--
-- finance_categories: the income and expense categories of a business.
-- Every organization starts with the defaults (code set, is_system), seeded
-- here for the existing ones and by a trigger for new ones; a business
-- renames, archives or adds its own. A category is never removed once an
-- entry uses it.
--
-- finance_entries: money in (income) or out (expense) that is not a sale:
-- rent, bills, supplies, other income. Each entry posts its cash movement
-- (kind income / expense) on the paying account in the same transaction,
-- so the cash book and the records cannot drift apart. Sales revenue stays
-- in sale_lines and staff payouts in staff_share_payouts: neither is
-- entered here (A17, A20).
--
-- Append-only like the cash book: a wrong entry is reversed (entry_type
-- reversal, the exact negative, dated the day it is posted, with the
-- reversal of its cash movement), never edited or deleted. A correction is
-- a reversal plus a replacement entry (replaces_entry_id). Only the
-- classification (category, description, related staff member) and the
-- attached document may change afterwards; the activity log keeps the
-- before / after values.
--
-- The document (receipt, invoice) lives in private storage under
-- organizations/{org}/finance/{entry}/ and is streamed by the API. Removing
-- or replacing it only clears the row's reference: the object stays, so a
-- replacement entry can share it and nothing disappears without a trace.

CREATE TABLE finance_categories (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    kind VARCHAR(8) NOT NULL CHECK (kind IN ('income', 'expense')),
    -- Seeded defaults carry a stable code; business categories have none.
    code VARCHAR(32) NULL,
    name VARCHAR(120) NOT NULL,
    is_system BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(12) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_finance_categories_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_finance_categories_organization_id UNIQUE (organization_id, id),
    CONSTRAINT ck_finance_categories_system CHECK (is_system = (code IS NOT NULL))
);

CREATE UNIQUE INDEX uq_finance_categories_name ON finance_categories (organization_id, kind, lower(name));
CREATE UNIQUE INDEX uq_finance_categories_code ON finance_categories (organization_id, code) WHERE code IS NOT NULL;
CREATE INDEX idx_finance_categories_organization_kind ON finance_categories (organization_id, kind, sort_order);

CREATE TRIGGER trg_finance_categories_updated_at
    BEFORE UPDATE ON finance_categories
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- The default categories of one organization (idempotent).
CREATE FUNCTION finance_seed_default_categories(org BIGINT) RETURNS void AS $$
BEGIN
    INSERT INTO finance_categories (organization_id, kind, code, name, is_system, sort_order)
    SELECT org, d.kind, d.code, d.name, TRUE, d.sort_order
    FROM (VALUES
        ('expense', 'rent', 'Kira', 10),
        ('expense', 'utilities', 'Faturalar (elektrik, su, doğalgaz, internet)', 20),
        ('expense', 'supplies', 'Malzeme ve sarf', 30),
        ('expense', 'product_purchase', 'Ürün alımı', 40),
        ('expense', 'maintenance', 'Bakım ve onarım', 50),
        ('expense', 'marketing', 'Reklam ve pazarlama', 60),
        ('expense', 'staff_costs', 'Personel giderleri (pay ödemeleri hariç)', 70),
        ('expense', 'taxes', 'Vergi ve harçlar', 80),
        ('expense', 'bank_fees', 'Banka ve komisyon giderleri', 90),
        ('expense', 'other_expense', 'Diğer giderler', 100),
        ('income', 'rental_income', 'Kira geliri', 10),
        ('income', 'other_income', 'Diğer gelirler', 100)
    ) AS d (kind, code, name, sort_order)
    ON CONFLICT DO NOTHING;
END;
$$ LANGUAGE plpgsql;

SELECT finance_seed_default_categories(o.id) FROM organizations o;

-- New organizations get the defaults in their create transaction, whatever
-- path creates them.
CREATE FUNCTION finance_categories_seed_organization() RETURNS trigger AS $$
BEGIN
    PERFORM finance_seed_default_categories(NEW.id);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_organizations_finance_categories
    AFTER INSERT ON organizations
    FOR EACH ROW EXECUTE FUNCTION finance_categories_seed_organization();

CREATE TABLE finance_entries (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- entry: the record itself (or the replacement of a corrected one).
    -- reversal: the exact negative of one entry.
    entry_type VARCHAR(10) NOT NULL CHECK (entry_type IN ('entry', 'reversal')),
    kind VARCHAR(8) NOT NULL CHECK (kind IN ('income', 'expense')),
    category_id BIGINT NOT NULL,
    -- NULL = business-wide (merkez); only members with every branch see it.
    branch_id BIGINT NULL,
    -- Optional staff member the money belongs to (training, uniform ...).
    staff_id BIGINT NULL,
    cash_account_id BIGINT NOT NULL,
    cash_movement_id BIGINT NOT NULL,
    -- Signed: positive on an entry, the negated amount on a reversal.
    amount NUMERIC(14, 2) NOT NULL,
    currency CHAR(3) NOT NULL,
    entry_date DATE NOT NULL,
    description VARCHAR(500) NOT NULL DEFAULT '',
    reverses_entry_id BIGINT NULL,
    replaces_entry_id BIGINT NULL,
    reason TEXT NOT NULL DEFAULT '',
    document_key TEXT NULL,
    document_name VARCHAR(255) NOT NULL DEFAULT '',
    document_content_type VARCHAR(128) NOT NULL DEFAULT '',
    document_size_bytes BIGINT NOT NULL DEFAULT 0,
    idempotency_key VARCHAR(128) NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_finance_entries_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_finance_entries_organization_id UNIQUE (organization_id, id),
    CONSTRAINT ck_finance_entries_sign CHECK (
        (entry_type = 'entry' AND amount > 0) OR (entry_type = 'reversal' AND amount < 0)
    ),
    CONSTRAINT ck_finance_entries_reversal CHECK ((entry_type = 'reversal') = (reverses_entry_id IS NOT NULL)),
    CONSTRAINT ck_finance_entries_replaces CHECK (entry_type = 'entry' OR replaces_entry_id IS NULL),
    CONSTRAINT ck_finance_entries_document CHECK (entry_type = 'entry' OR document_key IS NULL),
    CONSTRAINT fk_finance_entries_category FOREIGN KEY (organization_id, category_id)
        REFERENCES finance_categories (organization_id, id),
    CONSTRAINT fk_finance_entries_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id),
    CONSTRAINT fk_finance_entries_staff FOREIGN KEY (organization_id, staff_id)
        REFERENCES staff (organization_id, id),
    CONSTRAINT fk_finance_entries_account FOREIGN KEY (organization_id, cash_account_id)
        REFERENCES cash_accounts (organization_id, id),
    CONSTRAINT fk_finance_entries_movement FOREIGN KEY (organization_id, cash_movement_id)
        REFERENCES cash_movements (organization_id, id),
    CONSTRAINT fk_finance_entries_reverses FOREIGN KEY (organization_id, reverses_entry_id)
        REFERENCES finance_entries (organization_id, id),
    CONSTRAINT fk_finance_entries_replaces FOREIGN KEY (organization_id, replaces_entry_id)
        REFERENCES finance_entries (organization_id, id)
);

-- An entry is reversed at most once and replaced at most once; a cash
-- movement belongs to one entry.
CREATE UNIQUE INDEX uq_finance_entries_reverses ON finance_entries (organization_id, reverses_entry_id)
    WHERE reverses_entry_id IS NOT NULL;
CREATE UNIQUE INDEX uq_finance_entries_replaces ON finance_entries (organization_id, replaces_entry_id)
    WHERE replaces_entry_id IS NOT NULL;
CREATE UNIQUE INDEX uq_finance_entries_movement ON finance_entries (organization_id, cash_movement_id);
CREATE UNIQUE INDEX uq_finance_entries_idempotency ON finance_entries (organization_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
CREATE INDEX idx_finance_entries_date ON finance_entries (organization_id, entry_date, id);
CREATE INDEX idx_finance_entries_branch_date ON finance_entries (organization_id, branch_id, entry_date);
CREATE INDEX idx_finance_entries_category ON finance_entries (organization_id, category_id);
CREATE INDEX idx_finance_entries_staff ON finance_entries (organization_id, staff_id) WHERE staff_id IS NOT NULL;

CREATE TRIGGER trg_finance_entries_updated_at
    BEFORE UPDATE ON finance_entries
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Append-only. The classification (category of the same kind, description,
-- staff member) and the document of an entry may change; the creator may
-- become NULL (user removed). Only an organization deletion (cascade)
-- removes rows.
CREATE FUNCTION finance_entries_append_only() RETURNS trigger AS $$
DECLARE
    mutable TEXT[] := ARRAY['category_id', 'description', 'staff_id', 'document_key', 'document_name',
        'document_content_type', 'document_size_bytes', 'created_by', 'updated_at'];
    new_kind TEXT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF pg_trigger_depth() > 1 THEN
            RETURN OLD;
        END IF;
    ELSIF to_jsonb(NEW) - mutable = to_jsonb(OLD) - mutable
        AND (NEW.created_by IS NULL OR NEW.created_by IS NOT DISTINCT FROM OLD.created_by) THEN
        IF NEW.category_id <> OLD.category_id THEN
            SELECT kind INTO new_kind FROM finance_categories
             WHERE organization_id = NEW.organization_id AND id = NEW.category_id;
            IF new_kind IS DISTINCT FROM NEW.kind THEN
                RAISE EXCEPTION 'the category belongs to the other kind'
                    USING ERRCODE = 'check_violation', CONSTRAINT = 'ck_finance_entries_category_kind';
            END IF;
        END IF;
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'finance entries are append-only (post a reversal)'
        USING ERRCODE = 'check_violation', CONSTRAINT = 'finance_entries_append_only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_finance_entries_append_only
    BEFORE UPDATE OR DELETE ON finance_entries
    FOR EACH ROW EXECUTE FUNCTION finance_entries_append_only();

-- Insert rules (backstop of the use case): the category has the entry's
-- kind; a reversal is the exact negative of one entry (same kind,
-- category, account, branch and currency); a replacement names an entry of
-- the same kind that is reversed.
CREATE FUNCTION finance_entries_insert_rules() RETURNS trigger AS $$
DECLARE
    orig finance_entries%ROWTYPE;
    cat_kind TEXT;
BEGIN
    SELECT kind INTO cat_kind FROM finance_categories
     WHERE organization_id = NEW.organization_id AND id = NEW.category_id;
    IF cat_kind IS DISTINCT FROM NEW.kind THEN
        RAISE EXCEPTION 'the category belongs to the other kind'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'ck_finance_entries_category_kind';
    END IF;
    IF NEW.entry_type = 'reversal' THEN
        SELECT * INTO orig FROM finance_entries
         WHERE organization_id = NEW.organization_id AND id = NEW.reverses_entry_id;
        IF NOT FOUND OR orig.entry_type <> 'entry' OR orig.kind <> NEW.kind
           OR orig.category_id <> NEW.category_id OR orig.cash_account_id <> NEW.cash_account_id
           OR orig.branch_id IS DISTINCT FROM NEW.branch_id OR orig.currency <> NEW.currency
           OR orig.amount <> -NEW.amount THEN
            RAISE EXCEPTION 'a reversal is the exact negative of one entry'
                USING ERRCODE = 'check_violation', CONSTRAINT = 'ck_finance_entries_reversal';
        END IF;
    ELSIF NEW.replaces_entry_id IS NOT NULL THEN
        SELECT * INTO orig FROM finance_entries
         WHERE organization_id = NEW.organization_id AND id = NEW.replaces_entry_id;
        IF NOT FOUND OR orig.entry_type <> 'entry' OR orig.kind <> NEW.kind OR NOT EXISTS (
            SELECT 1 FROM finance_entries r
             WHERE r.organization_id = orig.organization_id AND r.reverses_entry_id = orig.id
        ) THEN
            RAISE EXCEPTION 'a replacement names a reversed entry of the same kind'
                USING ERRCODE = 'check_violation', CONSTRAINT = 'ck_finance_entries_replaces';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_finance_entries_insert_rules
    BEFORE INSERT ON finance_entries
    FOR EACH ROW EXECUTE FUNCTION finance_entries_insert_rules();

-- tenant.finance.read / .write exist since 000042 ("revenue, cash and
-- expenses"): reading the records, entering, correcting and reversing
-- them. Exports use tenant.report.export; documents are streamed with
-- tenant.document.download. No new permission slug.
