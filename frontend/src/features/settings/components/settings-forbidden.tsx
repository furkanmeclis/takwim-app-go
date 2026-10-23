"use client";

import { ErrorState } from "@/components/common/error-state";
import { useLocale } from "@/providers/locale-provider";

export function SettingsForbidden() {
  const { t } = useLocale();
  return (
    <ErrorState
      title={t("common.error_forbidden")}
      description={t("settings.system.forbidden")}
    />
  );
}
