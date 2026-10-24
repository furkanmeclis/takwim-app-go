-- TEC-920: multi-session packages (requirements §13 "Çok seanslı paket",
-- A19; docs/design/session-packages.md).
--
-- session_packages: what the business sells: N sessions of one service for
-- a price, usable in every branch or in the listed ones
-- (session_package_branches), valid for a number of days from the sale.
--
-- customer_packages: one package sold to a customer, with a snapshot of
-- the definition (name, service, sessions, price, branches, validity), the
-- day it was sold and its ledger entries:
--   - the collection (money taken: a cash movement, kasa girişi);
--   - a "package" debit of the price, so the customer's ledger balance
--     stays right (the package is charged, it is not revenue).
-- The revenue (ciro) of a package comes from its sessions only: a
-- completed session becomes one sale line of the session's value, so the
-- sale itself and its use are never counted twice. A refund closes the
-- package: the paid back amount leaves the cash book, the package debit
-- is reversed and the kept part charged again.
--
-- package_sessions: one session of a package given to a reservation line.
-- held (booked, the right is reserved) → consumed (the visit was
-- completed) or released (cancelled, rejected, expired, no-show, removed,
-- rescheduled to another service). The value of session k of n is the
-- price split evenly with the remainder on the last session (5 × 2.500,00
-- = 500,00 each); the share base of the staff member is this value.
-- A session number is given at most once among the live (held or
-- consumed) sessions of a package, so the last session can never be
-- given to two reservations, and a reservation line uses at most one.

CREATE TABLE session_packages (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name VARCHAR(160) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    service_id BIGINT NOT NULL,
    total_sessions INT NOT NULL CHECK (total_sessions BETWEEN 1 AND 1000),
    price NUMERIC(14, 2) NOT NULL CHECK (price >= 0),
    -- Days a sold package can be used, counted from the sale day; NULL =
    -- no end.
    validity_days INT NULL CHECK (validity_days BETWEEN 1 AND 3650),
    -- TRUE: every branch of the business; FALSE: session_package_branches.
    all_branches BOOLEAN NOT NULL DEFAULT TRUE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_session_packages_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_session_packages_organization_id UNIQUE (organization_id, id),
    CONSTRAINT fk_session_packages_service FOREIGN KEY (organization_id, service_id)
        REFERENCES services (organization_id, id)
);

CREATE UNIQUE INDEX uq_session_packages_name ON session_packages (organization_id, lower(name))
    WHERE deleted_at IS NULL;
CREATE INDEX idx_session_packages_service ON session_packages (organization_id, service_id)
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_session_packages_updated_at
    BEFORE UPDATE ON session_packages
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE session_package_branches (
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    package_id BIGINT NOT NULL,
    branch_id BIGINT NOT NULL,
    PRIMARY KEY (organization_id, package_id, branch_id),
    CONSTRAINT fk_session_package_branches_package FOREIGN KEY (organization_id, package_id)
        REFERENCES session_packages (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_session_package_branches_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_session_package_branches_branch ON session_package_branches (organization_id, branch_id);

CREATE TABLE customer_packages (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    package_id BIGINT NULL,
    customer_id BIGINT NOT NULL,
    -- The branch that sold it (its cash book took the money).
    branch_id BIGINT NOT NULL,
    service_id BIGINT NULL,
    -- Snapshots of the definition at the sale.
    name VARCHAR(160) NOT NULL,
    service_name VARCHAR(160) NOT NULL,
    total_sessions INT NOT NULL CHECK (total_sessions BETWEEN 1 AND 1000),
    price NUMERIC(14, 2) NOT NULL CHECK (price >= 0),
    currency CHAR(3) NOT NULL,
    -- Branches the sessions can be used in; NULL = every branch.
    branch_ids BIGINT[] NULL,
    sold_on DATE NOT NULL,
    -- Last day a session can be booked for; NULL = no end.
    expires_on DATE NULL,
    status VARCHAR(10) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'refunded')),
    -- Who sold it (staff record), optional.
    staff_id BIGINT NULL,
    note TEXT NOT NULL DEFAULT '',
    collection_entry_id BIGINT NULL,
    charge_entry_id BIGINT NULL,
    idempotency_key VARCHAR(128) NULL,
    refund_amount NUMERIC(14, 2) NULL CHECK (refund_amount >= 0),
    refund_entry_id BIGINT NULL,
    refund_reason TEXT NULL,
    refunded_on DATE NULL,
    refunded_at TIMESTAMPTZ NULL,
    refunded_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_customer_packages_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_customer_packages_organization_id UNIQUE (organization_id, id),
    CONSTRAINT ck_customer_packages_expiry CHECK (expires_on IS NULL OR expires_on >= sold_on),
    CONSTRAINT ck_customer_packages_refund CHECK (
        (status = 'refunded') = (refunded_at IS NOT NULL AND refunded_on IS NOT NULL AND refund_amount IS NOT NULL)
    ),
    CONSTRAINT ck_customer_packages_refund_amount CHECK (refund_amount IS NULL OR refund_amount <= price),
    CONSTRAINT fk_customer_packages_package FOREIGN KEY (organization_id, package_id)
        REFERENCES session_packages (organization_id, id) ON DELETE SET NULL (package_id),
    CONSTRAINT fk_customer_packages_customer FOREIGN KEY (organization_id, customer_id)
        REFERENCES customers (organization_id, id),
    CONSTRAINT fk_customer_packages_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id),
    CONSTRAINT fk_customer_packages_service FOREIGN KEY (organization_id, service_id)
        REFERENCES services (organization_id, id) ON DELETE SET NULL (service_id),
    CONSTRAINT fk_customer_packages_staff FOREIGN KEY (organization_id, staff_id)
        REFERENCES staff (organization_id, id) ON DELETE SET NULL (staff_id),
    CONSTRAINT fk_customer_packages_collection FOREIGN KEY (organization_id, collection_entry_id)
        REFERENCES ledger_entries (organization_id, id),
    CONSTRAINT fk_customer_packages_charge FOREIGN KEY (organization_id, charge_entry_id)
        REFERENCES ledger_entries (organization_id, id),
    CONSTRAINT fk_customer_packages_refund FOREIGN KEY (organization_id, refund_entry_id)
        REFERENCES ledger_entries (organization_id, id)
);

CREATE UNIQUE INDEX uq_customer_packages_idempotency ON customer_packages (organization_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
CREATE INDEX idx_customer_packages_customer ON customer_packages (organization_id, customer_id, id);
CREATE INDEX idx_customer_packages_branch ON customer_packages (organization_id, branch_id, sold_on);
CREATE INDEX idx_customer_packages_package ON customer_packages (organization_id, package_id)
    WHERE package_id IS NOT NULL;

CREATE TRIGGER trg_customer_packages_updated_at
    BEFORE UPDATE ON customer_packages
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- A sold package keeps what was sold: only the refund (once), the note,
-- the seller and references cleared by a deletion change.
CREATE FUNCTION customer_packages_sold() RETURNS trigger AS $$
BEGIN
    IF NEW.customer_id <> OLD.customer_id OR NEW.branch_id <> OLD.branch_id
        OR NEW.total_sessions <> OLD.total_sessions OR NEW.price <> OLD.price OR NEW.currency <> OLD.currency
        OR NEW.sold_on <> OLD.sold_on OR NEW.branch_ids IS DISTINCT FROM OLD.branch_ids
        OR (OLD.collection_entry_id IS NOT NULL AND NEW.collection_entry_id IS DISTINCT FROM OLD.collection_entry_id)
        OR (OLD.status = 'refunded' AND (NEW.status <> OLD.status OR NEW.refund_amount IS DISTINCT FROM OLD.refund_amount
            OR NEW.refund_entry_id IS DISTINCT FROM OLD.refund_entry_id OR NEW.expires_on IS DISTINCT FROM OLD.expires_on)) THEN
        RAISE EXCEPTION 'a sold package keeps its terms (refund it instead)'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'customer_packages_sold';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_customer_packages_sold
    BEFORE UPDATE ON customer_packages
    FOR EACH ROW EXECUTE FUNCTION customer_packages_sold();

CREATE TABLE package_sessions (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    customer_package_id BIGINT NOT NULL,
    reservation_id BIGINT NOT NULL,
    line_index INT NOT NULL,
    session_no INT NOT NULL CHECK (session_no >= 1),
    -- The session's share of the package price (revenue and share base).
    amount NUMERIC(14, 2) NOT NULL CHECK (amount >= 0),
    status VARCHAR(10) NOT NULL DEFAULT 'held' CHECK (status IN ('held', 'consumed', 'released')),
    -- removed, cancelled, rejected, expired, no_show, rescheduled.
    release_reason VARCHAR(32) NULL,
    consumed_at TIMESTAMPTZ NULL,
    consumed_on DATE NULL,
    released_at TIMESTAMPTZ NULL,
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_package_sessions_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT ck_package_sessions_consumed CHECK (
        (status = 'consumed') = (consumed_at IS NOT NULL AND consumed_on IS NOT NULL)
    ),
    CONSTRAINT ck_package_sessions_released CHECK (
        (status = 'released') = (released_at IS NOT NULL AND release_reason IS NOT NULL)
    ),
    CONSTRAINT fk_package_sessions_package FOREIGN KEY (organization_id, customer_package_id)
        REFERENCES customer_packages (organization_id, id),
    CONSTRAINT fk_package_sessions_reservation FOREIGN KEY (organization_id, reservation_id)
        REFERENCES reservations (organization_id, id) ON DELETE CASCADE
);

-- The same session of a package is never live twice (the last session
-- cannot be given to two reservations); a line uses one session.
CREATE UNIQUE INDEX uq_package_sessions_slot ON package_sessions (organization_id, customer_package_id, session_no)
    WHERE status <> 'released';
CREATE UNIQUE INDEX uq_package_sessions_line ON package_sessions (organization_id, reservation_id, line_index)
    WHERE status <> 'released';
CREATE INDEX idx_package_sessions_package ON package_sessions (organization_id, customer_package_id, id);
CREATE INDEX idx_package_sessions_reservation ON package_sessions (organization_id, reservation_id, id);
CREATE INDEX idx_package_sessions_consumed ON package_sessions (organization_id, consumed_on)
    WHERE status = 'consumed';

CREATE TRIGGER trg_package_sessions_updated_at
    BEFORE UPDATE ON package_sessions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Backstop of the use case: a session belongs to an active package within
-- its session count; a consumed or released session is final.
CREATE FUNCTION package_sessions_rules() RETURNS trigger AS $$
DECLARE
    pkg customer_packages%ROWTYPE;
BEGIN
    IF TG_OP = 'INSERT' THEN
        SELECT * INTO pkg FROM customer_packages
         WHERE organization_id = NEW.organization_id AND id = NEW.customer_package_id;
        IF NOT FOUND OR pkg.status <> 'active' OR NEW.session_no > pkg.total_sessions OR NEW.status <> 'held' THEN
            RAISE EXCEPTION 'a session is held on an active package within its session count'
                USING ERRCODE = 'check_violation', CONSTRAINT = 'package_sessions_rules';
        END IF;
    ELSIF OLD.status <> 'held' AND (NEW.status <> OLD.status OR NEW.amount <> OLD.amount
        OR NEW.session_no <> OLD.session_no OR NEW.customer_package_id <> OLD.customer_package_id
        OR NEW.reservation_id <> OLD.reservation_id OR NEW.line_index <> OLD.line_index) THEN
        RAISE EXCEPTION 'a consumed or released package session is final'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'package_sessions_rules';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_package_sessions_rules
    BEFORE INSERT OR UPDATE ON package_sessions
    FOR EACH ROW EXECUTE FUNCTION package_sessions_rules();

-- Ledger: a "package" debit charges the customer for a sold package. It
-- has no sale line (a package sale is not revenue) and never belongs to a
-- reservation.
ALTER TABLE ledger_entries DROP CONSTRAINT ledger_entries_kind_check;
ALTER TABLE ledger_entries ADD CONSTRAINT ledger_entries_kind_check
    CHECK (kind IN ('sale', 'collection', 'refund', 'reversal', 'package'));
ALTER TABLE ledger_entries DROP CONSTRAINT ck_ledger_entries_direction;
ALTER TABLE ledger_entries ADD CONSTRAINT ck_ledger_entries_direction CHECK (
    kind = 'reversal'
    OR (kind IN ('sale', 'refund', 'package') AND direction = 'debit')
    OR (kind = 'collection' AND direction = 'credit')
);
ALTER TABLE ledger_entries ADD CONSTRAINT ck_ledger_entries_package CHECK (
    kind <> 'package' OR (reservation_id IS NULL AND customer_id IS NOT NULL AND sale_line_id IS NULL)
);

-- Permissions: read the packages and the customers' packages, manage the
-- definitions, sell a package to a customer. A refund also needs
-- tenant.payment.refund; using a session on a visit needs reservation
-- write access only.
INSERT INTO permissions (name, slug) VALUES
    ('Read session packages', 'tenant.session_packages.read'),
    ('Manage session packages', 'tenant.session_packages.write'),
    ('Sell session packages', 'tenant.session_packages.sell')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug IN (
    'tenant.session_packages.read', 'tenant.session_packages.write', 'tenant.session_packages.sell'
)
WHERE r.slug IN ('owner', 'admin') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug IN (
    'tenant.session_packages.read', 'tenant.session_packages.sell'
)
WHERE r.slug IN ('sube_yoneticisi', 'resepsiyon') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
