"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { NotificationEventsTab } from "@/features/customer-notifications/components/notification-events-tab";
import { NotificationHistoryTab } from "@/features/customer-notifications/components/notification-history-tab";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

type NotificationsTab = "events" | "history";

/**
 * Müşteri bildirimleri (TEC-911): which reservation events notify the
 * customer, on which channels and with which text, plus the read-only
 * history of what was sent. The active tab is mirrored to `?tab=`.
 */
export function CustomerNotificationsPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const tab: NotificationsTab =
    searchParams.get("tab") === "history" ? "history" : "events";
  const setTab = (next: string) => {
    router.replace(next === "history" ? `${pathname}?tab=history` : pathname, {
      scroll: false,
    });
  };

  return (
    <EntityPage
      title={t("customer_notifications.title")}
      description={t("customer_notifications.description")}
      permission={permissions.tenant.settings.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("customer_notifications.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("layout.section_tenant_settings"),
          href: routes.tenant.settings.root(slug),
        },
        { label: t("customer_notifications.title") },
      ]}
    >
      <Tabs value={tab} onValueChange={setTab} className="space-y-4">
        <TabsList>
          <TabsTrigger value="events">
            {t("customer_notifications.tabs.events")}
          </TabsTrigger>
          <TabsTrigger value="history">
            {t("customer_notifications.tabs.history")}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="events">
          <NotificationEventsTab />
        </TabsContent>
        <TabsContent value="history">
          <NotificationHistoryTab />
        </TabsContent>
      </Tabs>
    </EntityPage>
  );
}
