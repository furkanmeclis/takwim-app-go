"use client";

import { EntitySectionCard } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { NotificationHistoryTab } from "@/features/customer-notifications/components/notification-history-tab";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/**
 * "Gönderilen bildirimler" on the reservation and customer detail pages
 * (TEC-974): the send history filtered to one booking or one customer,
 * one row per channel attempt. The history is a business setting
 * (`tenant.settings.read`, as on Ayarlar → Müşteri bildirimleri), so the
 * card is left out without it.
 */
export function CustomerNotificationHistoryCard({
  reservationUuid,
  customerUuid,
}: {
  reservationUuid?: string;
  customerUuid?: string;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  if (!can(permissions.tenant.settings.read)) return null;
  return (
    <EntitySectionCard
      title={t("customer_notifications.history.section_title")}
    >
      <NotificationHistoryTab
        reservationUuid={reservationUuid}
        customerUuid={customerUuid}
      />
    </EntitySectionCard>
  );
}
