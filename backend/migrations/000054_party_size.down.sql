ALTER TABLE public_checkouts
    DROP COLUMN IF EXISTS zone,
    DROP COLUMN IF EXISTS party_size;

ALTER TABLE reservations
    DROP COLUMN IF EXISTS zone,
    DROP COLUMN IF EXISTS party_size;
