"use client";

import Link from "next/link";
import type { ReactNode } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { EntityDrawer, EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import {
  STATUS_TONES,
  channelLabel,
  eventLabel,
  statusLabel,
} from "@/features/customer-notifications/constants";
import type { CustomerNotificationDelivery } from "@/features/customer-notifications/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function Row({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-4 py-1.5 text-sm">
      <span className="text-muted-foreground shrink-0">{label}</span>
      <span className="min-w-0 text-right font-medium break-words">
        {value}
      </span>
    </div>
  );
}

/** One channel attempt: where it went, how it ended and the text sent. */
export function DeliveryDrawer({
  delivery,
  onOpenChange,
}: {
  delivery: CustomerNotificationDelivery | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const when = (value?: string) =>
    value ? datetime(value, undefined, locale) : "—";

  return (
    <EntityDrawer
      open={Boolean(delivery)}
      onOpenChange={onOpenChange}
      title={delivery ? eventLabel(t, delivery.event) : ""}
      description={delivery ? when(delivery.created_at) : undefined}
      size="lg"
      footer={
        delivery ? (
          <div className="flex flex-wrap gap-2">
            {delivery.reservation_uuid ? (
              <Button asChild variant="outline" size="sm">
                <Link
                  href={routes.tenant.reservations.detail(
                    slug,
                    delivery.reservation_uuid,
                  )}
                >
                  {t("customer_notifications.history.open_reservation")}
                </Link>
              </Button>
            ) : null}
            <Button asChild variant="outline" size="sm">
              <Link
                href={routes.tenant.customers.detail(
                  slug,
                  delivery.customer_uuid,
                )}
              >
                {t("customer_notifications.history.open_customer")}
              </Link>
            </Button>
          </div>
        ) : undefined
      }
    >
      {delivery ? (
        <div className="space-y-4">
          <div className="divide-y">
            <Row
              label={t("customer_notifications.columns.status")}
              value={
                <StatusChip
                  label={statusLabel(t, delivery.status)}
                  tone={STATUS_TONES[delivery.status]}
                />
              }
            />
            <Row
              label={t("customer_notifications.columns.channel")}
              value={channelLabel(t, delivery.channel)}
            />
            <Row
              label={t("customer_notifications.columns.customer")}
              value={delivery.customer_name}
            />
            {delivery.reservation_number ? (
              <Row
                label={t("customer_notifications.columns.reservation")}
                value={`#${delivery.reservation_number}`}
              />
            ) : null}
            <Row
              label={t("customer_notifications.columns.recipient")}
              value={delivery.recipient_masked || "—"}
            />
            <Row
              label={t("customer_notifications.columns.provider")}
              value={delivery.provider || "—"}
            />
            {delivery.provider_ref ? (
              <Row
                label={t("customer_notifications.history.provider_ref")}
                value={delivery.provider_ref}
              />
            ) : null}
            <Row
              label={t("customer_notifications.history.attempt")}
              value={delivery.attempt_no}
            />
            <Row
              label={t("customer_notifications.history.sent_at")}
              value={when(delivery.sent_at)}
            />
            <Row
              label={t("customer_notifications.history.finished_at")}
              value={when(delivery.finished_at)}
            />
          </div>
          {delivery.error ? (
            <EntitySectionCard
              title={t("customer_notifications.columns.error")}
            >
              <p className="text-sm break-words whitespace-pre-wrap">
                {delivery.error}
              </p>
            </EntitySectionCard>
          ) : null}
          <EntitySectionCard title={t("customer_notifications.history.text")}>
            <div className="space-y-2 text-sm">
              {delivery.subject ? (
                <p className="font-medium break-words">{delivery.subject}</p>
              ) : null}
              <p className="break-words whitespace-pre-wrap">
                {delivery.body || "—"}
              </p>
            </div>
          </EntitySectionCard>
        </div>
      ) : null}
    </EntityDrawer>
  );
}
