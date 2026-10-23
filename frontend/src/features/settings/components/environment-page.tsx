"use client";

import type { ReactNode } from "react";
import { Cpu, Server, type LucideIcon } from "lucide-react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { permissions } from "@/config/permissions";
import { SettingsForbidden } from "@/features/settings/components/settings-forbidden";
import { SettingsPageHeader } from "@/features/settings/components/settings-page-header";
import { useSystemInfo } from "@/features/settings/hooks/use-system-info";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function InfoCard({
  title,
  description,
  icon: Icon,
  iconClass,
  rows,
}: {
  title: string;
  description: string;
  icon: LucideIcon;
  iconClass: string;
  rows: { label: string; value: ReactNode }[];
}) {
  return (
    <Card className="border-border/60 shadow-xs">
      <CardHeader>
        <div className="flex items-center gap-2">
          <div className={cn("rounded-lg p-2", iconClass)}>
            <Icon className="size-4" />
          </div>
          <div>
            <CardTitle className="text-base font-semibold">{title}</CardTitle>
            <CardDescription className="text-xs">{description}</CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="text-xs">
        {rows.map((row, index) => (
          <div
            key={row.label}
            className={cn(
              "flex items-center justify-between gap-3 py-1.5",
              index < rows.length - 1 && "border-border/40 border-b",
            )}
          >
            <span className="text-muted-foreground">{row.label}</span>
            <span className="text-right">{row.value}</span>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

/** Read-only, non-secret runtime/environment summary reported by the API. */
export function EnvironmentPage() {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const { data: info, isLoading, isError, refetch } = useSystemInfo();

  if (!can(permissions.settings.read)) return <SettingsForbidden />;

  if (isLoading) return <Loading label={t("common.loading")} />;

  if (isError || !info) {
    return (
      <ErrorState
        title={t("settings.system.error_title")}
        description={t("settings.system.error_description")}
        onRetry={() => void refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }

  const dash = "—";

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title={t("settings.subnav.system_environment")}
        description={t("settings.system.env_description")}
      />

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <InfoCard
          title={t("settings.system.env_app_config")}
          description={t("settings.system.env_app_config_sub")}
          icon={Server}
          iconClass="bg-primary/10 text-primary"
          rows={[
            {
              label: t("settings.system.env_app_name"),
              value: (
                <span className="text-foreground font-mono font-medium">
                  {info.environment.app_name || dash}
                </span>
              ),
            },
            {
              label: t("settings.system.env_stage"),
              value: (
                <Badge
                  variant="outline"
                  className="font-mono text-[10px] uppercase"
                >
                  {info.environment.env || dash}
                </Badge>
              ),
            },
            {
              label: t("settings.system.log_level"),
              value: (
                <Badge
                  variant="secondary"
                  className="font-mono text-[10px] uppercase"
                >
                  {info.environment.log_level || dash}
                </Badge>
              ),
            },
            {
              label: t("settings.system.env_server_time"),
              value: (
                <span className="text-muted-foreground font-mono">
                  {new Date(info.runtime.server_time).toLocaleString(locale)}
                </span>
              ),
            },
          ]}
        />

        <InfoCard
          title={t("settings.system.stat_runtime")}
          description={t("settings.system.env_runtime_sub")}
          icon={Cpu}
          iconClass="bg-blue-500/10 text-blue-600 dark:text-blue-400"
          rows={[
            {
              label: t("settings.system.env_go_version"),
              value: (
                <span className="text-foreground font-mono font-semibold">
                  {info.runtime.go_version}
                </span>
              ),
            },
            {
              label: t("settings.system.env_os_arch"),
              value: (
                <span className="font-mono font-medium">
                  {info.runtime.os} / {info.runtime.arch}
                </span>
              ),
            },
            {
              label: t("settings.system.env_compiler"),
              value: <span className="font-mono">{info.runtime.compiler}</span>,
            },
            {
              label: t("settings.system.env_cpu"),
              value: (
                <span className="font-mono font-semibold">
                  {info.runtime.num_cpu} {t("settings.system.cores")}
                </span>
              ),
            },
          ]}
        />
      </div>
    </div>
  );
}
