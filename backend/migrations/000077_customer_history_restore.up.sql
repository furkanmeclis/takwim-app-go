-- TEC-991: reopening a business history closed by a number release
-- (docs/design/customer-panel.md, "Reopening closed history").
--
-- A dormant check or a reassignment notice (TEC-972) hides the earlier
-- visits of every business link with visible_from. When the number stayed
-- with the same person, the business that knows them reopens its own
-- history after checking who they are at the reception; no other business
-- is touched and a name match is never evidence.
--
--   * customer_identity_links.history_requested_at: the person asked this
--     business from the panel to reopen its history (cleared on restore or
--     on the next closing).
--   * history_restored_at / history_restored_by / history_restored_from:
--     who reopened it, when, and the visible_from it reverted (the trail
--     next to the business's activity entry customers.history_restored).
--   * customer_auth_events: 'history_requested', 'history_restored'.

ALTER TABLE customer_identity_links
    ADD COLUMN history_requested_at TIMESTAMPTZ NULL,
    ADD COLUMN history_restored_at TIMESTAMPTZ NULL,
    ADD COLUMN history_restored_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN history_restored_from TIMESTAMPTZ NULL;

ALTER TABLE customer_auth_events DROP CONSTRAINT customer_auth_events_kind_check;
ALTER TABLE customer_auth_events ADD CONSTRAINT customer_auth_events_kind_check CHECK (kind IN (
    'signed_in', 'code_failed', 'link_failed', 'visit_link_failed', 'ip_throttled',
    'phone_changed', 'phone_change_refused', 'reassigned', 'session_revoked',
    'platform_reassigned', 'dormant_closed', 'history_requested', 'history_restored'
));
