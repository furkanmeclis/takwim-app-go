-- TEC-888: branch profile for the customer screens. A branch has a URL slug
-- (unique in the business), a description, a customer-screen visibility flag
-- and a photo gallery (the first photo is the cover). branches.timezone
-- (migration 000037) stays NULL when the branch follows the business zone.
ALTER TABLE branches
    ADD COLUMN slug VARCHAR(64),
    ADD COLUMN description TEXT NOT NULL DEFAULT '',
    ADD COLUMN show_online BOOLEAN NOT NULL DEFAULT TRUE;

-- Backfill: the slug comes from the code; a collision gets the id appended.
WITH base AS (
    SELECT id, organization_id,
           COALESCE(NULLIF(trim(BOTH '-' FROM regexp_replace(lower(code), '[^a-z0-9]+', '-', 'g')), ''), 'sube') AS s
    FROM branches
),
ranked AS (
    SELECT id, s, ROW_NUMBER() OVER (PARTITION BY organization_id, s ORDER BY id) AS n FROM base
)
UPDATE branches b
SET slug = CASE WHEN r.n = 1 THEN r.s ELSE r.s || '-' || b.id::text END
FROM ranked r
WHERE r.id = b.id;

ALTER TABLE branches
    ALTER COLUMN slug SET NOT NULL,
    ADD CONSTRAINT branches_slug_check CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND length(slug) <= 64),
    ADD CONSTRAINT branches_description_check CHECK (length(description) <= 2000),
    ADD CONSTRAINT branches_timezone_check CHECK (timezone IS NULL OR length(timezone) BETWEEN 1 AND 64);

-- Inserts without a slug (seeds, older paths) get one from the code; the API
-- always sends its own (from the name, see the branches usecase).
CREATE FUNCTION branches_default_slug() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    base TEXT;
BEGIN
    IF NEW.slug IS NULL OR NEW.slug = '' THEN
        base := COALESCE(NULLIF(trim(BOTH '-' FROM regexp_replace(lower(NEW.code), '[^a-z0-9]+', '-', 'g')), ''), 'sube');
        NEW.slug := base;
        IF EXISTS (
            SELECT 1 FROM branches
            WHERE organization_id = NEW.organization_id AND slug = base AND deleted_at IS NULL
        ) THEN
            NEW.slug := base || '-' || substr(md5(random()::text), 1, 6);
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_branches_default_slug
    BEFORE INSERT ON branches
    FOR EACH ROW EXECUTE FUNCTION branches_default_slug();

CREATE UNIQUE INDEX uq_branches_organization_slug
    ON branches (organization_id, slug)
    WHERE deleted_at IS NULL;

-- Branch photos: bytes in object storage, metadata here. Served through the
-- API's public media route; the browser never builds object URLs.
CREATE TABLE branch_images (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL,
    branch_id BIGINT NOT NULL,
    object_key TEXT NOT NULL,
    filename VARCHAR(255) NOT NULL DEFAULT '',
    content_type VARCHAR(64) NOT NULL CHECK (content_type IN ('image/jpeg', 'image/png', 'image/webp')),
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_branch_images_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_branch_images_branch
        FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_branch_images_organization_branch
    ON branch_images (organization_id, branch_id, sort_order);
