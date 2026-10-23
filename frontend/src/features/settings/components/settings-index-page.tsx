"use client";

import { useEffect, useMemo } from "react";
import { useRouter } from "next/navigation";

import { Loading } from "@/components/common/loading";
import { permissions } from "@/config/permissions";
import { SettingsForbidden } from "@/features/settings/components/settings-forbidden";
import { SystemOverviewPage } from "@/features/settings/components/system-overview-page";
import { visibleSettingsGroups } from "@/features/settings/lib/settings-nav";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/**
 * `/platform/settings` landing. Users who can read system settings see the
 * overview; everyone else is sent to the first settings page they can open.
 */
export function SettingsIndexPage() {
  const { t } = useLocale();
  const router = useRouter();
  const { can, canAny } = usePermission();
  const canReadSystem = can(permissions.settings.read);

  const fallbackHref = useMemo(() => {
    if (canReadSystem) return null;
    return visibleSettingsGroups({ can, canAny }, t)[0]?.items[0]?.href ?? null;
  }, [canReadSystem, can, canAny, t]);

  useEffect(() => {
    if (fallbackHref) router.replace(fallbackHref);
  }, [fallbackHref, router]);

  if (canReadSystem) return <SystemOverviewPage />;
  if (fallbackHref) return <Loading label={t("common.loading")} />;
  return <SettingsForbidden />;
}
