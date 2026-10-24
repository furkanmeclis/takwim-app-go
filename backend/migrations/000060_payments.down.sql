-- Data-loss note: drops every online payment attempt. The ledger
-- collections and cash movements they wrote stay (they are the money).
DROP TABLE IF EXISTS payments;
