"use client";

import { CircleAlert, CircleCheck } from "lucide-react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { FIELD_LABEL_KEYS } from "@/features/tenant-settings/constants";
import { useLocale } from "@/providers/locale-provider";

/**
 * Online sales readiness of the saved version: the reservation settings the
 * API requires before online sales can be published.
 */
export function SettingsReadiness({
  ready,
  missing,
  onFix,
}: {
  ready: boolean;
  missing: string[];
  onFix: () => void;
}) {
  const { t } = useLocale();
  if (ready) {
    return (
      <Alert>
        <CircleCheck className="text-emerald-600 dark:text-emerald-400" />
        <AlertTitle>{t("tenant_settings.readiness.ready_title")}</AlertTitle>
        <AlertDescription>
          {t("tenant_settings.readiness.ready_description")}
        </AlertDescription>
      </Alert>
    );
  }
  return (
    <Alert>
      <CircleAlert className="text-amber-600 dark:text-amber-400" />
      <AlertTitle>{t("tenant_settings.readiness.not_ready_title")}</AlertTitle>
      <AlertDescription>
        <p>{t("tenant_settings.readiness.not_ready_description")}</p>
        <ul className="mt-1 list-disc pl-5">
          {missing.map((path) => (
            <li key={path}>
              {FIELD_LABEL_KEYS[path] ? t(FIELD_LABEL_KEYS[path]) : path}
            </li>
          ))}
        </ul>
        <Button
          type="button"
          variant="link"
          size="sm"
          className="h-auto px-0"
          onClick={onFix}
        >
          {t("tenant_settings.readiness.fix")}
        </Button>
      </AlertDescription>
    </Alert>
  );
}
