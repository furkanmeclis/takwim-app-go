-- Data-loss note: drops every refund request. The refund ledger entries and
-- cash movements they posted stay (they are the money); return movements
-- stay in the stock ledger.
DROP INDEX IF EXISTS uq_stock_movements_sale_return;
DROP TABLE IF EXISTS refunds;
