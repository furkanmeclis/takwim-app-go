-- Data-loss note: removes the history requests and the restore trail of the
-- identity links (reopened histories stay open: visible_from is already
-- NULL) and the history auth events.
DELETE FROM customer_auth_events WHERE kind IN ('history_requested', 'history_restored');
ALTER TABLE customer_auth_events DROP CONSTRAINT customer_auth_events_kind_check;
ALTER TABLE customer_auth_events ADD CONSTRAINT customer_auth_events_kind_check CHECK (kind IN (
    'signed_in', 'code_failed', 'link_failed', 'visit_link_failed', 'ip_throttled',
    'phone_changed', 'phone_change_refused', 'reassigned', 'session_revoked',
    'platform_reassigned', 'dormant_closed'
));

ALTER TABLE customer_identity_links
    DROP COLUMN IF EXISTS history_restored_from,
    DROP COLUMN IF EXISTS history_restored_by,
    DROP COLUMN IF EXISTS history_restored_at,
    DROP COLUMN IF EXISTS history_requested_at;
