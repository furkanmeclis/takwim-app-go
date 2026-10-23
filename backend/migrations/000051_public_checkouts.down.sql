-- Data-loss note: drops the checkouts of the public booking API. Their
-- capacity holds and reservations stay; open checkout tokens stop working.
DROP TABLE IF EXISTS public_checkouts;
