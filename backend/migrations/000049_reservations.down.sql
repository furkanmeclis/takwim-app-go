-- Data-loss note: drops every reservation with its lines and history.
-- Capacity commitments (capacity_holds) stay; their ref_uuid then names a
-- record that no longer exists.
DROP TABLE IF EXISTS reservation_events;
DROP TABLE IF EXISTS reservation_lines;
DROP TABLE IF EXISTS reservations;
