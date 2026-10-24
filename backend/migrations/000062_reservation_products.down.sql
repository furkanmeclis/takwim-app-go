-- Data-loss note: drops the products added to reservations. Units still
-- held go back to the available stock (stock_levels.reserved is reset to
-- what no reservation holds, i.e. zero).
UPDATE stock_levels SET reserved = 0 WHERE reserved <> 0;

DROP TABLE IF EXISTS reservation_products;
DROP FUNCTION IF EXISTS reservation_products_final();
