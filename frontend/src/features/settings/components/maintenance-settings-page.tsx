"use client";

import { permissions } from "@/config/permissions";
import { PurgeRulesPanel } from "@/features/logs/components/purge-rules-panel";
import { SettingsForbidden } from "@/features/settings/components/settings-forbidden";
import { SettingsPageHeader } from "@/features/settings/components/settings-page-header";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export function MaintenanceSettingsPage() {
  const { t } = useLocale();
  const { can } = usePermission();

  if (!can(permissions.logs.read)) return <SettingsForbidden />;

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title={t("settings.subnav.purge_rules")}
        description={t("settings.system.maintenance_description")}
      />
      <PurgeRulesPanel />
    </div>
  );
}
