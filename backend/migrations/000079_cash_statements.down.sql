-- Data-loss note: imported statements and their reconciliation state are
-- dropped; the movements posted from them stay in the cash book.
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE slug = 'tenant.cash.import');
DELETE FROM permissions WHERE slug = 'tenant.cash.import';

-- Closings counted below zero (bank periods) would refuse the constraint:
-- clamp them first.
UPDATE cash_closings SET counted_amount = 0, difference = 0 - expected_amount WHERE counted_amount < 0;
ALTER TABLE cash_closings ADD CONSTRAINT cash_closings_counted_amount_check CHECK (counted_amount >= 0);

DROP TRIGGER IF EXISTS trg_cash_statement_lines_updated_at ON cash_statement_lines;
DROP TABLE IF EXISTS cash_statement_lines;
DROP TRIGGER IF EXISTS trg_cash_statements_updated_at ON cash_statements;
DROP TABLE IF EXISTS cash_statements;
