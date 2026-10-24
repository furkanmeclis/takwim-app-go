-- Confirmation channels (TEC-922): delivery reports, receipt times and
-- channel replies of the customer notification deliveries.

-- name: GetWebhookIntegrationAccount :one
-- The account a provider webhook URL names (/v1/public/whatsapp/meta/{uuid}).
-- The only unscoped account lookup: the uuid gives the organization, and the
-- caller verifies the request with that account's own secret before acting.
SELECT a.id, a.uuid, a.organization_id, a.kind, a.provider, a.status
FROM integration_accounts a
WHERE a.uuid = sqlc.arg(uuid);

-- name: GetCustomerNotificationDeliveryByProviderRef :one
-- The attempt a delivery report or a reply is about (the provider's id of
-- the message we sent).
SELECT * FROM customer_notification_deliveries
WHERE organization_id = sqlc.arg(organization_id) AND channel = sqlc.arg(channel)
  AND provider_ref = sqlc.arg(provider_ref) AND provider_ref <> ''
ORDER BY id DESC
LIMIT 1;

-- name: MarkCustomerNotificationDeliveryReceipt :exec
-- Receipt times from a delivery report: the first report of each kind
-- wins. A late report of an attempt that already ended still records when
-- the message arrived; it never changes the attempt's status.
UPDATE customer_notification_deliveries
SET delivered_at = COALESCE(delivered_at, sqlc.arg(at)::timestamptz),
    read_at = CASE WHEN sqlc.arg(is_read)::boolean THEN COALESCE(read_at, sqlc.arg(at)::timestamptz) ELSE read_at END
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: ListUnreportedCustomerNotificationDeliveries :many
-- Accepted attempts whose delivery report never came (every business):
-- the receipt timeout fails them so the next channel runs.
SELECT organization_id, uuid, channel
FROM customer_notification_deliveries
WHERE status = 'sent' AND finished_at IS NULL AND sent_at < sqlc.arg(before)
  AND channel = ANY(sqlc.arg(channels)::text[])
ORDER BY sent_at
LIMIT sqlc.arg(limit_count);

-- name: InsertCustomerNotificationResponse :one
-- One reply on a channel; a repeated webhook of the same inbound message
-- writes nothing.
INSERT INTO customer_notification_responses (
    organization_id, delivery_id, channel, provider, provider_ref, answer, result, reason, responded_at
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(delivery_id), sqlc.arg(channel), sqlc.arg(provider), sqlc.arg(provider_ref),
    sqlc.arg(answer), sqlc.arg(result), sqlc.arg(reason), sqlc.arg(responded_at)
)
ON CONFLICT (organization_id, channel, provider_ref) DO NOTHING
RETURNING *;

-- name: ListConfirmationResponses :many
-- The replies to one booking's confirmation requests, oldest first.
SELECT dl.uuid AS delivery_uuid, cr.uuid, cr.channel, cr.answer, cr.result, cr.reason, cr.responded_at
FROM customer_notification_responses cr
JOIN customer_notification_deliveries dl ON dl.organization_id = cr.organization_id AND dl.id = cr.delivery_id
JOIN customer_notification_dispatches d ON d.organization_id = dl.organization_id AND d.id = dl.dispatch_id
WHERE cr.organization_id = sqlc.arg(organization_id) AND d.reservation_id = sqlc.arg(reservation_id)
  AND d.event_code = sqlc.arg(event_code)
ORDER BY cr.responded_at, cr.id
LIMIT 200;

-- name: ListOpenCustomerNotificationDeliveriesByProvider :many
-- Accepted attempts of one channel and provider still waiting for their
-- delivery report (every business), oldest first: a provider without a
-- webhook (Netgsm SMS, TEC-982) is asked for them.
SELECT id, organization_id, uuid, provider_ref, sent_at
FROM customer_notification_deliveries
WHERE status = 'sent' AND finished_at IS NULL AND channel = sqlc.arg(channel) AND provider = sqlc.arg(provider)
  AND provider_ref <> ''
ORDER BY organization_id, sent_at
LIMIT sqlc.arg(limit_count);
