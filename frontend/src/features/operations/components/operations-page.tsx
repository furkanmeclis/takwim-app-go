"use client";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { NoticeFailuresPanel } from "@/features/operations/components/notice-failures-panel";
import { OperationsSummaryPanel } from "@/features/operations/components/operations-summary";
import { ProviderErrorsPanel } from "@/features/operations/components/provider-errors-panel";
import { QueueTasksPanel } from "@/features/operations/components/queue-tasks-panel";
import { useLocale } from "@/providers/locale-provider";

/**
 * Background work and failures of the platform (TEC-930): queue health,
 * failed tasks, provider errors and failed customer notice sends of every
 * business. Backups and restore drills: docs/OPERATIONS.md.
 */
export function OperationsPage() {
  const { t } = useLocale();
  return (
    <EntityPage
      title={t("operations.title")}
      description={t("operations.description")}
      permission={permissions.settings.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("operations.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        {
          label: t("settings.subnav.system_overview"),
          href: routes.platform.settings.root,
        },
        { label: t("operations.title") },
      ]}
    >
      <Tabs defaultValue="summary" className="gap-6">
        <TabsList className="w-full justify-start overflow-x-auto sm:w-auto">
          <TabsTrigger value="summary">
            {t("operations.tabs.summary")}
          </TabsTrigger>
          <TabsTrigger value="queue">{t("operations.tabs.queue")}</TabsTrigger>
          <TabsTrigger value="integrations">
            {t("operations.tabs.integrations")}
          </TabsTrigger>
          <TabsTrigger value="notices">
            {t("operations.tabs.notices")}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="summary" className="mt-2">
          <OperationsSummaryPanel />
        </TabsContent>
        <TabsContent value="queue" className="mt-2">
          <QueueTasksPanel />
        </TabsContent>
        <TabsContent value="integrations" className="mt-2">
          <ProviderErrorsPanel />
        </TabsContent>
        <TabsContent value="notices" className="mt-2">
          <NoticeFailuresPanel />
        </TabsContent>
      </Tabs>
    </EntityPage>
  );
}
