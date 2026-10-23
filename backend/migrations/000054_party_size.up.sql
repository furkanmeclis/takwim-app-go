-- TEC-903: restaurant bookings carry the number of people and the area the
-- customer chose. The slot engine seats the party at a table whose range
-- holds it (or joined combinable tables, panel only); a move plans the
-- same party again.

ALTER TABLE reservations
    ADD COLUMN party_size INT NULL CHECK (party_size BETWEEN 1 AND 1000),
    ADD COLUMN zone VARCHAR(64) NOT NULL DEFAULT '';

ALTER TABLE public_checkouts
    ADD COLUMN party_size INT NULL CHECK (party_size BETWEEN 1 AND 1000),
    ADD COLUMN zone VARCHAR(64) NOT NULL DEFAULT '';
