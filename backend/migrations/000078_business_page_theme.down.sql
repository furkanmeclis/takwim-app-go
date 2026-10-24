-- Data-loss note: forgets the cover image keys (the objects stay in storage).
ALTER TABLE organizations
    DROP COLUMN cover_object_key;
