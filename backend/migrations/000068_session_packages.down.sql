-- Data-loss note: drops every session package, sold customer package and
-- package session, the tenant.session_packages.* permissions, and the
-- "package" ledger debits (with their reversals). The collections and
-- refunds of package sales stay in the ledger and the cash book as plain
-- customer collections and refunds; sale lines of consumed sessions stay
-- as revenue of their visits.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'tenant.session_packages.read', 'tenant.session_packages.write', 'tenant.session_packages.sell'
    )
);

DELETE FROM permissions WHERE slug IN (
    'tenant.session_packages.read', 'tenant.session_packages.write', 'tenant.session_packages.sell'
);

DROP TABLE IF EXISTS package_sessions;
DROP FUNCTION IF EXISTS package_sessions_rules();
DROP TABLE IF EXISTS customer_packages;
DROP FUNCTION IF EXISTS customer_packages_sold();
DROP TABLE IF EXISTS session_package_branches;
DROP TABLE IF EXISTS session_packages;

ALTER TABLE ledger_entries DISABLE TRIGGER trg_ledger_entries_append_only;
DELETE FROM ledger_entries e
USING ledger_entries o
WHERE e.organization_id = o.organization_id AND e.reverses_entry_id = o.id AND o.kind = 'package';
DELETE FROM ledger_entries WHERE kind = 'package';
ALTER TABLE ledger_entries ENABLE TRIGGER trg_ledger_entries_append_only;

ALTER TABLE ledger_entries DROP CONSTRAINT IF EXISTS ck_ledger_entries_package;
ALTER TABLE ledger_entries DROP CONSTRAINT ck_ledger_entries_direction;
ALTER TABLE ledger_entries ADD CONSTRAINT ck_ledger_entries_direction CHECK (
    kind = 'reversal'
    OR (kind IN ('sale', 'refund') AND direction = 'debit')
    OR (kind = 'collection' AND direction = 'credit')
);
ALTER TABLE ledger_entries DROP CONSTRAINT ledger_entries_kind_check;
ALTER TABLE ledger_entries ADD CONSTRAINT ledger_entries_kind_check
    CHECK (kind IN ('sale', 'collection', 'refund', 'reversal'));
