"use client";

import { useEffect, useState, type ReactNode } from "react";
import {
  Clock,
  Copy,
  Cpu,
  Database,
  Gauge,
  HardDrive,
  Layers,
  Radio,
  RefreshCw,
  Search,
  Zap,
  type LucideIcon,
} from "lucide-react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { permissions } from "@/config/permissions";
import { SettingsForbidden } from "@/features/settings/components/settings-forbidden";
import { SettingsPageHeader } from "@/features/settings/components/settings-page-header";
import {
  useSystemInfo,
  useSystemPing,
} from "@/features/settings/hooks/use-system-info";
import type {
  ServiceHealth,
  ServiceHealthStatus,
  SystemStatus,
} from "@/features/settings/types";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

type Translate = (key: string) => string;

function formatBytes(bytes: number, decimals = 1): string {
  if (!bytes) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(
    sizes.length - 1,
    Math.floor(Math.log(bytes) / Math.log(k)),
  );
  return `${parseFloat((bytes / k ** i).toFixed(decimals))} ${sizes[i]}`;
}

function formatUptime(seconds: number): string {
  if (!seconds) return "0s";
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = Math.floor(seconds % 60);
  const parts: string[] = [];
  if (days > 0) parts.push(`${days}d`);
  if (hours > 0) parts.push(`${hours}h`);
  if (minutes > 0) parts.push(`${minutes}m`);
  if (secs > 0 || parts.length === 0) parts.push(`${secs}s`);
  return parts.join(" ");
}

function formatLatency(health: ServiceHealth): string {
  return health.latency_ms > 0 ? `${health.latency_ms} ms` : "—";
}

function overallTone(status: SystemStatus) {
  if (status === "healthy") {
    return {
      badge:
        "border-emerald-500/40 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400",
      dot: "bg-emerald-500 animate-pulse",
      labelKey: "settings.system.status_operational",
    };
  }
  if (status === "degraded") {
    return {
      badge:
        "border-amber-500/40 bg-amber-500/10 text-amber-600 dark:text-amber-400",
      dot: "bg-amber-500",
      labelKey: "settings.system.status_degraded",
    };
  }
  return {
    badge: "border-rose-500/40 bg-rose-500/10 text-rose-600 dark:text-rose-400",
    dot: "bg-rose-500",
    labelKey: "settings.system.status_down",
  };
}

function ServiceStatusBadge({
  status,
  t,
}: {
  status: ServiceHealthStatus;
  t: Translate;
}) {
  switch (status) {
    case "healthy":
      return (
        <Badge variant="success" className="font-medium">
          <span className="mr-1.5 size-1.5 rounded-full bg-emerald-500" />
          {t("settings.system.status_healthy")}
        </Badge>
      );
    case "degraded":
      return (
        <Badge variant="warning" className="font-medium">
          <span className="mr-1.5 size-1.5 rounded-full bg-amber-500" />
          {t("settings.system.status_degraded")}
        </Badge>
      );
    case "down":
      return (
        <Badge variant="danger" className="font-medium">
          <span className="bg-destructive mr-1.5 size-1.5 animate-pulse rounded-full" />
          {t("settings.system.status_down")}
        </Badge>
      );
    default:
      return (
        <Badge
          variant="secondary"
          className="text-muted-foreground font-medium"
        >
          {t("settings.system.status_disabled")}
        </Badge>
      );
  }
}

function Row({
  label,
  children,
  last = false,
}: {
  label: ReactNode;
  children: ReactNode;
  last?: boolean;
}) {
  return (
    <div
      className={cn(
        "flex items-center justify-between gap-3 py-1.5",
        !last && "border-border/40 border-b",
      )}
    >
      <span className="text-muted-foreground">{label}</span>
      <span className="text-right">{children}</span>
    </div>
  );
}

function StatCard({
  title,
  icon: Icon,
  iconClass,
  children,
}: {
  title: string;
  icon: LucideIcon;
  iconClass: string;
  children: ReactNode;
}) {
  return (
    <Card className="border-border/60 gap-2 py-4 shadow-xs">
      <CardHeader className="flex flex-row items-center justify-between space-y-0 px-4">
        <CardTitle className="text-muted-foreground text-xs font-semibold tracking-wider uppercase">
          {title}
        </CardTitle>
        <div className={cn("rounded-lg p-2", iconClass)}>
          <Icon className="size-4" />
        </div>
      </CardHeader>
      <CardContent className="px-4">{children}</CardContent>
    </Card>
  );
}

function ServiceCard({
  title,
  description,
  icon: Icon,
  iconClass,
  status,
  t,
  children,
}: {
  title: string;
  description: string;
  icon: LucideIcon;
  iconClass: string;
  status: ServiceHealthStatus;
  t: Translate;
  children: ReactNode;
}) {
  return (
    <Card className="border-border/60 hover:border-border gap-3 shadow-xs transition-colors">
      <CardHeader>
        <div className="flex items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-2.5">
            <div className={cn("rounded-lg p-2", iconClass)}>
              <Icon className="size-5" />
            </div>
            <div className="min-w-0">
              <CardTitle className="text-base font-semibold">{title}</CardTitle>
              <CardDescription className="text-xs">
                {description}
              </CardDescription>
            </div>
          </div>
          <ServiceStatusBadge status={status} t={t} />
        </div>
      </CardHeader>
      <CardContent className="space-y-1 text-xs">{children}</CardContent>
    </Card>
  );
}

export function SystemOverviewPage() {
  const { t } = useLocale();
  const { can } = usePermission();
  const { data: info, isLoading, isError, refetch } = useSystemInfo();
  const pingMutation = useSystemPing();
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!copied) return;
    const timer = window.setTimeout(() => setCopied(false), 2000);
    return () => window.clearTimeout(timer);
  }, [copied]);

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

  const handleCopyDiagnostics = async () => {
    try {
      await navigator.clipboard.writeText(JSON.stringify(info, null, 2));
      setCopied(true);
      appToast.success(t("settings.system.toast_copied"));
    } catch {
      appToast.error(t("settings.system.toast_copy_failed"));
    }
  };

  const tone = overallTone(info.status);
  const { database, redis, storage, meilisearch } = info.services;
  const pool = database.pool;
  const poolUsagePercent = pool
    ? Math.min(
        100,
        Math.round((pool.acquired_conns / Math.max(1, pool.max_conns)) * 100),
      )
    : 0;
  const memory = info.runtime.memory;
  const memoryPercent =
    memory.heap_sys_bytes > 0
      ? Math.min(
          100,
          Math.round((memory.heap_alloc_bytes / memory.heap_sys_bytes) * 100),
        )
      : 0;

  const featureBadge = (enabled: boolean) => (
    <Badge variant={enabled ? "success" : "secondary"}>
      {enabled
        ? t("settings.system.feature_active")
        : t("settings.system.feature_disabled")}
    </Badge>
  );

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title={
          <span className="flex flex-wrap items-center gap-2">
            {t("settings.system.title")}
            <Badge
              variant="outline"
              className={cn(
                "text-[10px] font-semibold tracking-wider uppercase",
                tone.badge,
              )}
            >
              <span className={cn("mr-1.5 size-1.5 rounded-full", tone.dot)} />
              {t(tone.labelKey)}
            </Badge>
          </span>
        }
        description={t("settings.system.description")}
        actions={
          <>
            <Button
              variant="outline"
              size="sm"
              onClick={() => void handleCopyDiagnostics()}
              className="text-xs"
            >
              <Copy className="mr-1.5 size-3.5" />
              {copied
                ? t("settings.system.action_copied")
                : t("settings.system.action_copy_json")}
            </Button>
            <Button
              size="sm"
              onClick={() => pingMutation.mutate()}
              disabled={pingMutation.isPending}
              className="text-xs"
            >
              <RefreshCw
                className={cn(
                  "mr-1.5 size-3.5",
                  pingMutation.isPending && "animate-spin",
                )}
              />
              {t("settings.system.action_run_probe")}
            </Button>
          </>
        }
      />

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          title={t("settings.system.stat_uptime")}
          icon={Clock}
          iconClass="bg-primary/10 text-primary"
        >
          <div className="text-xl font-bold tracking-tight">
            {formatUptime(info.runtime.uptime_seconds)}
          </div>
          <p className="text-muted-foreground mt-1 text-xs">
            {t("settings.system.stat_uptime_sub")}
          </p>
        </StatCard>

        <StatCard
          title={t("settings.system.stat_runtime")}
          icon={Cpu}
          iconClass="bg-blue-500/10 text-blue-600 dark:text-blue-400"
        >
          <div className="font-mono text-xl font-bold tracking-tight">
            {info.runtime.go_version}
          </div>
          <p className="text-muted-foreground mt-1 text-xs">
            {info.runtime.num_cpu} {t("settings.system.cores")} ·{" "}
            {info.runtime.os}/{info.runtime.arch}
          </p>
        </StatCard>

        <StatCard
          title={t("settings.system.stat_goroutines")}
          icon={Layers}
          iconClass="bg-purple-500/10 text-purple-600 dark:text-purple-400"
        >
          <div className="font-mono text-xl font-bold tracking-tight">
            {info.runtime.num_goroutine.toLocaleString()}
          </div>
          <p className="text-muted-foreground mt-1 text-xs">
            {memory.num_gc.toLocaleString()} {t("settings.system.gc_cycles")}
          </p>
        </StatCard>

        <StatCard
          title={t("settings.system.stat_memory_heap")}
          icon={Gauge}
          iconClass="bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
        >
          <div className="font-mono text-xl font-bold tracking-tight">
            {formatBytes(memory.heap_alloc_bytes)}
          </div>
          <Progress value={memoryPercent} className="mt-2 h-1.5" />
          <p className="text-muted-foreground mt-1 flex justify-between text-[11px]">
            <span>{t("settings.system.stat_sys_total")}</span>
            <span className="font-mono">{formatBytes(memory.sys_bytes)}</span>
          </p>
        </StatCard>
      </div>

      <section className="space-y-3">
        <h2 className="text-muted-foreground/90 text-sm font-semibold tracking-wider uppercase">
          {t("settings.system.section_services")}
        </h2>

        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <ServiceCard
            title={t("settings.system.service_database")}
            description={t("settings.system.service_database_sub")}
            icon={Database}
            iconClass="bg-sky-500/10 text-sky-600 dark:text-sky-400"
            status={database.status}
            t={t}
          >
            <Row label={t("settings.system.latency")} last={!pool}>
              <span className="font-mono font-medium">
                {formatLatency(database)}
              </span>
            </Row>
            {pool ? (
              <div className="space-y-1.5 pt-2">
                <div className="flex items-center justify-between">
                  <span className="text-muted-foreground">
                    {t("settings.system.db_pool_utilization")}
                  </span>
                  <span className="font-mono font-semibold">
                    {pool.acquired_conns} / {pool.max_conns} ({poolUsagePercent}
                    %)
                  </span>
                </div>
                <Progress value={poolUsagePercent} className="h-2" />
                <div className="text-muted-foreground flex justify-between pt-0.5 text-[11px]">
                  <span>
                    {pool.idle_conns} {t("settings.system.idle")}
                  </span>
                  <span>
                    {pool.total_conns} {t("settings.system.total_open")}
                  </span>
                </div>
              </div>
            ) : null}
            {database.message ? (
              <p className="text-destructive pt-1 break-words">
                {database.message}
              </p>
            ) : null}
          </ServiceCard>

          <ServiceCard
            title={t("settings.system.service_redis")}
            description={t("settings.system.service_redis_sub")}
            icon={Zap}
            iconClass="bg-red-500/10 text-red-600 dark:text-red-400"
            status={redis.status}
            t={t}
          >
            <Row label={t("settings.system.latency")}>
              <span className="font-mono font-medium">
                {formatLatency(redis)}
              </span>
            </Row>
            <Row label={t("settings.system.role")} last>
              <span className="text-foreground font-medium">
                {t("settings.system.service_redis_role")}
              </span>
            </Row>
          </ServiceCard>

          <ServiceCard
            title={t("settings.system.service_storage")}
            description={t("settings.system.service_storage_sub")}
            icon={HardDrive}
            iconClass="bg-amber-500/10 text-amber-600 dark:text-amber-400"
            status={storage.status}
            t={t}
          >
            <Row label={t("settings.system.driver")}>
              <Badge
                variant="secondary"
                className="font-mono text-[11px] uppercase"
              >
                {storage.driver}
              </Badge>
            </Row>
            <Row label={t("settings.system.latency")} last>
              <span className="font-mono font-medium">
                {formatLatency(storage)}
              </span>
            </Row>
          </ServiceCard>

          <ServiceCard
            title={t("settings.system.service_search")}
            description={t("settings.system.service_search_sub")}
            icon={Search}
            iconClass="bg-pink-500/10 text-pink-600 dark:text-pink-400"
            status={meilisearch.status}
            t={t}
          >
            <Row label={t("settings.system.status")}>
              <span className="font-medium">
                {info.environment.search_enabled
                  ? t("settings.system.feature_enabled")
                  : t("settings.system.feature_disabled")}
              </span>
            </Row>
            <Row label={t("settings.system.latency")} last>
              <span className="font-mono font-medium">
                {formatLatency(meilisearch)}
              </span>
            </Row>
          </ServiceCard>
        </div>
      </section>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card className="border-border/60 shadow-xs">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-sm font-semibold">
              <Cpu className="text-primary size-4" />
              {t("settings.system.section_memory_details")}
            </CardTitle>
          </CardHeader>
          <CardContent className="text-xs">
            <Row label={t("settings.system.heap_in_use")}>
              <span className="font-mono font-semibold">
                {formatBytes(memory.heap_inuse_bytes)}
              </span>
            </Row>
            <Row label={t("settings.system.heap_idle")}>
              <span className="font-mono">
                {formatBytes(memory.heap_idle_bytes)}
              </span>
            </Row>
            <Row label={t("settings.system.total_alloc")}>
              <span className="font-mono">
                {formatBytes(memory.total_alloc_bytes)}
              </span>
            </Row>
            <Row label={t("settings.system.gc_runs")} last>
              <span className="font-mono font-semibold">
                {memory.num_gc.toLocaleString()}
              </span>
            </Row>
          </CardContent>
        </Card>

        <Card className="border-border/60 shadow-xs">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-sm font-semibold">
              <Radio className="text-primary size-4" />
              {t("settings.system.section_subsystems")}
            </CardTitle>
          </CardHeader>
          <CardContent className="text-xs">
            <Row label={t("settings.system.subsystem_realtime")}>
              {featureBadge(info.environment.centrifugo_enabled)}
            </Row>
            <Row label={t("settings.system.subsystem_web_push")}>
              {featureBadge(info.environment.vapid_enabled)}
            </Row>
            <Row label={t("settings.system.log_level")}>
              <Badge
                variant="secondary"
                className="font-mono text-[10px] uppercase"
              >
                {info.environment.log_level || "—"}
              </Badge>
            </Row>
            <Row label={t("settings.system.server_time_utc")} last>
              <span className="text-muted-foreground font-mono">
                {new Date(info.runtime.server_time).toISOString()}
              </span>
            </Row>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
