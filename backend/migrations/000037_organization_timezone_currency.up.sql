-- TEC-878: every business has a time zone and a currency. Instants stay in
-- UTC (timestamptz); calendar decisions use the zone. A branch without its
-- own zone (NULL) inherits the business zone.
ALTER TABLE organizations
    ADD COLUMN timezone VARCHAR(64) NOT NULL DEFAULT 'Europe/Istanbul',
    ADD COLUMN currency CHAR(3) NOT NULL DEFAULT 'TRY',
    ADD CONSTRAINT organizations_currency_check CHECK (currency ~ '^[A-Z]{3}$');

ALTER TABLE branches
    ADD COLUMN timezone VARCHAR(64);
