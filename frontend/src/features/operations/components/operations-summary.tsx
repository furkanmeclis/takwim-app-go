"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatsCard } from "@/components/common/stats-card";
import { StatusChip } from "@/components/common/status-chip";
import { EntitySectionCard, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { useOperationsSummary } from "@/features/operations/hooks/use-operations";
import type {
  OperationsQueueStats,
  OperationsSummary as Summary,
} from "@/features/operations/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** Browser-side error tracking: the DSN the root layout loaded at runtime. */
function frontendTrackingEnabled() {
  if (typeof window === "undefined") return false;
  const env = (window as { __APP_ENV__?: { sentryDsn?: string } }).__APP_ENV__;
  return Boolean(env?.sentryDsn);
}

function QueueTable({ queues }: { queues: OperationsQueueStats[] }) {
  const { t } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<OperationsQueueStats>({
          accessorKey: "name",
          labelKey: "operations.summary.queue_columns.name",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="flex items-center gap-2">
              <span className="font-mono text-xs">{row.original.name}</span>
              {row.original.paused ? (
                <StatusChip
                  label={t("operations.summary.paused")}
                  tone="warning"
                />
              ) : null}
            </div>
          ),
        }),
        createColumn<OperationsQueueStats>({
          accessorKey: "pending",
          labelKey: "operations.summary.queue_columns.pending",
          enableSorting: false,
          gridSecondary: true,
        }),
        createColumn<OperationsQueueStats>({
          accessorKey: "active",
          labelKey: "operations.summary.queue_columns.active",
          enableSorting: false,
        }),
        createColumn<OperationsQueueStats>({
          accessorKey: "scheduled",
          labelKey: "operations.summary.queue_columns.scheduled",
          enableSorting: false,
          defaultHidden: true,
        }),
        createColumn<OperationsQueueStats>({
          accessorKey: "retry",
          labelKey: "operations.summary.queue_columns.retry",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.retry > 0 ? (
              <StatusChip label={String(row.original.retry)} tone="warning" />
            ) : (
              "0"
            ),
        }),
        createColumn<OperationsQueueStats>({
          accessorKey: "archived",
          labelKey: "operations.summary.queue_columns.archived",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.archived > 0 ? (
              <StatusChip label={String(row.original.archived)} tone="danger" />
            ) : (
              "0"
            ),
        }),
        createColumn<OperationsQueueStats>({
          accessorKey: "processed_today",
          labelKey: "operations.summary.queue_columns.processed_today",
          enableSorting: false,
          cell: ({ row }) =>
            `${row.original.processed_today} / ${row.original.failed_today}`,
        }),
        createColumn<OperationsQueueStats>({
          accessorKey: "latency_ms",
          labelKey: "operations.summary.queue_columns.latency",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            t("operations.summary.seconds", {
              value: Math.round(row.original.latency_ms / 100) / 10,
            }),
        }),
      ] as ColumnDef<OperationsQueueStats, unknown>[],
    [t],
  );
  return (
    <EntityTable
      columns={columns}
      data={queues}
      getRowId={(row) => row.name}
      emptyTitle={t("operations.summary.no_queues")}
      manual={{ filtering: false, sorting: false, pagination: false }}
      features={{
        persistKey: "platform-operations-queues-v1",
        rowSelection: false,
        columnFilters: false,
        facetedFilters: false,
        sorting: false,
        pagination: false,
      }}
    />
  );
}

function SummaryBody({ summary }: { summary: Summary }) {
  const { t, locale } = useLocale();
  const providerErrors =
    summary.integrations.failed +
    summary.integrations.needs_verification +
    summary.integrations.blocked;
  const engineFailures =
    summary.engine.exports + summary.engine.imports + summary.engine.bulk;
  const browserTracking = frontendTrackingEnabled();

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <StatusChip
          label={t(
            summary.status === "healthy"
              ? "operations.summary.status_healthy"
              : "operations.summary.status_attention",
          )}
          tone={summary.status === "healthy" ? "success" : "warning"}
        />
        <span className="text-muted-foreground text-xs">
          {t("operations.summary.generated_at", {
            time: datetime(summary.generated_at, "d MMM yyyy HH:mm:ss", locale),
          })}
        </span>
      </div>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <StatsCard
          title={t("operations.summary.workers")}
          value={summary.queue.available ? summary.queue.workers : "—"}
          hint={
            summary.queue.available
              ? summary.queue.workers === 0
                ? t("operations.summary.workers_none")
                : t("operations.summary.workers_hint")
              : t("operations.summary.queue_unavailable")
          }
        />
        <StatsCard
          title={t("operations.summary.failed_tasks")}
          value={summary.queue.archived}
          hint={t("operations.summary.failed_tasks_hint", {
            retry: summary.queue.retry,
          })}
        />
        <StatsCard
          title={t("operations.summary.provider_errors")}
          value={providerErrors}
          hint={t("operations.summary.provider_errors_hint", {
            failed: summary.integrations.failed,
            verify: summary.integrations.needs_verification,
            blocked: summary.integrations.blocked,
          })}
        />
        <StatsCard
          title={t("operations.summary.notice_failures")}
          value={summary.notices.failed_24h}
          hint={t("operations.summary.notice_failures_hint", {
            week: summary.notices.failed_7d,
          })}
        />
        <StatsCard
          title={t("operations.summary.outbox")}
          value={summary.outbox.failed}
          hint={t("operations.summary.outbox_hint", {
            pending: summary.outbox.pending,
          })}
        />
        <StatsCard
          title={t("operations.summary.engine_failures")}
          value={engineFailures}
          hint={t("operations.summary.engine_failures_hint", {
            exports: summary.engine.exports,
            imports: summary.engine.imports,
            bulk: summary.engine.bulk,
          })}
        />
        <StatsCard
          title={t("operations.summary.error_tracking")}
          value={
            <div className="flex flex-wrap gap-2 text-sm font-normal">
              <StatusChip
                label={t(
                  summary.error_tracking.enabled
                    ? "operations.summary.tracking_api_on"
                    : "operations.summary.tracking_api_off",
                )}
                tone={summary.error_tracking.enabled ? "success" : "warning"}
              />
              <StatusChip
                label={t(
                  browserTracking
                    ? "operations.summary.tracking_web_on"
                    : "operations.summary.tracking_web_off",
                )}
                tone={browserTracking ? "success" : "warning"}
              />
            </div>
          }
          hint={summary.error_tracking.environment || undefined}
        />
      </div>
      <EntitySectionCard title={t("operations.summary.queues_title")}>
        {summary.queue.available ? (
          <QueueTable queues={summary.queue.queues} />
        ) : (
          <p className="text-muted-foreground text-sm">
            {t("operations.summary.queue_unavailable")}
          </p>
        )}
      </EntitySectionCard>
    </div>
  );
}

/** Health of the background work (TEC-930): queues, workers, failures. */
export function OperationsSummaryPanel() {
  const { t } = useLocale();
  const query = useOperationsSummary();
  if (query.isLoading) return <Loading />;
  if (query.isError || !query.data) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        description={t("operations.summary.load_failed")}
        onRetry={() => void query.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }
  return <SummaryBody summary={query.data} />;
}
