-- External work skeleton (TEC-880, docs/design/integration-jobs.md).
--
-- integration_accounts: every provider an organization connects (SMS,
-- WhatsApp, e-mail, payment). The organization's own credentials are sealed
-- with the field cipher under the "integration.credentials" purpose and bound
-- to the organization and the account uuid (associated data), so a
-- ciphertext copied to another row does not open. Only a masked hint is ever
-- returned. Kinds and providers are validated by the adapter registry.
--
-- integration_jobs: every outbound call and inbound provider notification
-- that must happen exactly once (idempotency key unique per provider), with
-- the runner's state machine:
--   pending             nothing sent yet, or the provider confirmed the
--                       previous submit was not done
--   processing          an inline flow holds the job (inbound webhooks)
--   submitted           the provider accepted the work; the final outcome
--                       comes from CheckStatus (a delivery report)
--   succeeded / failed  final (failed always means "the provider confirmed
--                       it was not done", never "unknown")
--   needs_verification  the outcome is unknown (timeout, 5xx after the
--                       request left); only CheckStatus may move it on
--   ignored             an inbound notification that needs no action
--   blocked / waiting_dependency
--                       held by the owning module (data missing / no working
--                       adapter); the runner never calls a provider for them
--   reported_manually / reconciled
--                       staff resolutions (final)
-- organization_id is nullable: platform-level jobs have no organization;
-- every query names the scope (mixed_scope_tables.txt).
--
-- integration_attempts: one row per provider call (submit / check_status)
-- with short request / response summaries: references and non-secret facts
-- only, never credentials, message bodies, OTP codes, full phone numbers or
-- card data.

CREATE TABLE integration_accounts (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    kind VARCHAR(24) NOT NULL CHECK (kind ~ '^[a-z][a-z0-9_]*$'),
    provider VARCHAR(24) NOT NULL CHECK (provider ~ '^[a-z][a-z0-9_]*$'),
    display_name VARCHAR(120) NOT NULL,
    mode VARCHAR(8) NOT NULL DEFAULT 'sandbox' CHECK (mode IN ('sandbox', 'live')),
    status VARCHAR(12) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    -- "v<N>:" + base64(nonce | AES-256-GCM); JSON object of the secrets.
    credentials_ciphertext TEXT NOT NULL,
    -- Masked values for display ({"api_key": "sand••••1234"}).
    credentials_hint JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- Non-secret provider settings.
    settings JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_checked_at TIMESTAMPTZ NULL,
    last_check_status VARCHAR(8) NOT NULL DEFAULT '' CHECK (last_check_status IN ('', 'ok', 'failed')),
    last_check_error TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_integration_accounts_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_integration_accounts_organization_id UNIQUE (organization_id, id)
);

-- One active account per organization and kind (work of a kind goes to it).
CREATE UNIQUE INDEX uq_integration_accounts_organization_active_kind
    ON integration_accounts (organization_id, kind) WHERE status = 'active';
CREATE INDEX idx_integration_accounts_organization_created
    ON integration_accounts (organization_id, created_at DESC);

CREATE TRIGGER trg_integration_accounts_updated_at
    BEFORE UPDATE ON integration_accounts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE integration_jobs (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    integration_account_id BIGINT NULL,
    direction VARCHAR(8) NOT NULL CHECK (direction IN ('inbound', 'outbound')),
    provider VARCHAR(24) NOT NULL,
    -- sms.send, payment.webhook, payment.refund …
    kind VARCHAR(48) NOT NULL,
    idempotency_key VARCHAR(200) NOT NULL,
    status VARCHAR(24) NOT NULL DEFAULT 'pending'
        CHECK (status IN (
            'pending', 'processing', 'submitted', 'succeeded', 'failed', 'ignored', 'needs_verification',
            'blocked', 'waiting_dependency', 'reported_manually', 'reconciled'
        )),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts INTEGER NOT NULL DEFAULT 5 CHECK (max_attempts >= 1),
    -- Deliveries of the same key (an inbound notification sent again).
    deliveries INTEGER NOT NULL DEFAULT 1 CHECK (deliveries >= 1),
    last_error TEXT NOT NULL DEFAULT '',
    -- References and non-secret facts only (never credentials or card data).
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    result JSONB NOT NULL DEFAULT '{}'::jsonb,
    ref_type VARCHAR(24) NOT NULL DEFAULT '',
    ref_uuid UUID NULL,
    -- critical = OTP / transactional (own queue), normal = everything else.
    priority VARCHAR(8) NOT NULL DEFAULT 'normal' CHECK (priority IN ('critical', 'normal')),
    next_attempt_at TIMESTAMPTZ NULL,
    -- The provider's reference for the submitted work; CheckStatus uses it.
    provider_ref VARCHAR(200) NOT NULL DEFAULT '',
    submitted_at TIMESTAMPTZ NULL,
    -- A worker's claim on the job (crash-safe: an expired claim is free).
    locked_until TIMESTAMPTZ NULL,
    completed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_integration_jobs_account
        FOREIGN KEY (organization_id, integration_account_id)
        REFERENCES integration_accounts (organization_id, id) ON DELETE SET NULL (integration_account_id),
    CONSTRAINT chk_integration_jobs_account_scope CHECK (integration_account_id IS NULL OR organization_id IS NOT NULL)
);

-- Exactly once per provider and key (a webhook delivered three times is
-- one job with deliveries = 3).
CREATE UNIQUE INDEX uq_integration_jobs_provider_key ON integration_jobs (provider, idempotency_key);
CREATE UNIQUE INDEX uq_integration_jobs_uuid ON integration_jobs (uuid);
CREATE INDEX idx_integration_jobs_organization_created ON integration_jobs (organization_id, created_at DESC);
CREATE INDEX idx_integration_jobs_organization_account
    ON integration_jobs (organization_id, integration_account_id, created_at DESC);
CREATE INDEX idx_integration_jobs_organization_status
    ON integration_jobs (organization_id, status, created_at DESC);
-- Due work for the sweep.
CREATE INDEX idx_integration_jobs_due
    ON integration_jobs (status, next_attempt_at)
    WHERE status IN ('pending', 'submitted', 'needs_verification');

CREATE TRIGGER trg_integration_jobs_updated_at
    BEFORE UPDATE ON integration_jobs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE integration_attempts (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    integration_job_id BIGINT NOT NULL REFERENCES integration_jobs (id) ON DELETE CASCADE,
    attempt_no INTEGER NOT NULL CHECK (attempt_no >= 1),
    operation VARCHAR(16) NOT NULL CHECK (operation IN ('submit', 'check_status')),
    -- The queue worker, the sweep, a staff retry, or an inline API call.
    trigger VARCHAR(12) NOT NULL DEFAULT 'worker' CHECK (trigger IN ('worker', 'sweep', 'retry', 'inline')),
    outcome VARCHAR(16) NOT NULL
        CHECK (outcome IN ('succeeded', 'accepted', 'pending', 'failed', 'not_found', 'uncertain', 'unsupported')),
    request_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    response_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    error TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ NOT NULL,
    duration_ms INTEGER NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_integration_attempts_window CHECK (finished_at >= started_at)
);

-- Global by design: attempts are addressed by uuid only inside a job the
-- caller already resolved in its organization.
CREATE UNIQUE INDEX uq_integration_attempts_uuid ON integration_attempts (uuid);
CREATE INDEX idx_integration_attempts_organization_job
    ON integration_attempts (organization_id, integration_job_id, id);

-- Permission: connect / test / disable provider accounts and see / retry
-- their jobs. Organization-wide credentials: owners and admins only.
INSERT INTO permissions (name, slug) VALUES
    ('Manage integrations', 'tenant.integrations.manage')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.integrations.manage'
WHERE r.slug IN ('owner', 'admin', 'merkez_yoneticisi') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
