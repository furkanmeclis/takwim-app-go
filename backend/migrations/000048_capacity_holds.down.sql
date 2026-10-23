-- Data-loss note: drops every hold and booking commitment with its staff and
-- resource allocations. btree_gist stays (an extension other schemas may use).
DROP TABLE IF EXISTS capacity_allocations;
DROP TABLE IF EXISTS capacity_holds;
