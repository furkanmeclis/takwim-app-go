-- Data-loss note: drops every sale line and ledger entry.
DROP TABLE IF EXISTS ledger_entries;
DROP FUNCTION IF EXISTS ledger_entries_insert_rules();
DROP FUNCTION IF EXISTS ledger_entries_append_only();
DROP TABLE IF EXISTS sale_lines;
DROP FUNCTION IF EXISTS sale_lines_insert_rules();
DROP FUNCTION IF EXISTS sale_lines_append_only();
