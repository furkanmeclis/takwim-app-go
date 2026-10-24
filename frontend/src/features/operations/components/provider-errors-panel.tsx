"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, RotateCcw } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PermissionGuard } from "@/components/common/permission-guard";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityDrawer,
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { IntegrationJobDetailBody } from "@/features/integrations/components/integration-jobs";
import {
  INTEGRATION_JOB_KIND_LABEL_KEYS,
  INTEGRATION_JOB_STATUSES,
  INTEGRATION_JOB_STATUS_LABEL_KEYS,
  INTEGRATION_JOB_STATUS_TONES,
  INTEGRATION_PROVIDER_LABEL_KEYS,
  labelOf,
} from "@/features/integrations/constants";
import {
  usePlatformIntegrationJob,
  usePlatformIntegrationJobs,
  useRetryPlatformIntegrationJob,
} from "@/features/operations/hooks/use-operations";
import type { PlatformIntegrationJob } from "@/features/operations/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const DATE = "d MMM yyyy HH:mm:ss";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  return [];
}

function JobDrawer({
  jobUuid,
  onClose,
}: {
  jobUuid: string | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const query = usePlatformIntegrationJob(jobUuid);
  const retry = useRetryPlatformIntegrationJob();
  const job = query.data;
  return (
    <EntityDrawer
      open={Boolean(jobUuid)}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      size="xl"
      title={t("integrations.jobs.detail_title")}
      description={t("operations.integrations.detail_description")}
      footer={
        job?.retryable ? (
          <PermissionGuard permission={permissions.settings.write}>
            <Button
              type="button"
              disabled={retry.isPending}
              onClick={() => retry.mutate(job.uuid)}
            >
              <RotateCcw className="size-4" />
              {t("integrations.jobs.actions.retry")}
            </Button>
          </PermissionGuard>
        ) : null
      }
    >
      {query.isLoading ? (
        <Loading />
      ) : query.isError || !job ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("integrations.jobs.load_failed")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : (
        <IntegrationJobDetailBody
          job={job}
          attemptsPersistKey="platform-integration-attempts-v1"
        />
      )}
    </EntityDrawer>
  );
}

/**
 * Provider errors of every business (TEC-930): integration jobs that failed,
 * whose outcome is unknown or that are held. A row opens the job with its
 * provider calls; "Yeniden dene" asks the provider first.
 */
export function ProviderErrorsPanel() {
  const { t, locale } = useLocale();
  const [openJob, setOpenJob] = useState<string | null>(null);
  const listState = useServerListState({
    initialSort: "-updated_at",
    initialPageSize: 20,
  });
  const status = filterValues(
    listState.columnFilters.find((f) => f.id === "status")?.value,
  );
  const params = {
    limit: listState.params.limit,
    offset: listState.params.offset,
    sort: listState.params.sort,
    q: listState.params.q,
    status: status.length ? status.join(",") : undefined,
  };
  const query = usePlatformIntegrationJobs(params);
  const total = query.data?.total ?? 0;

  const columns = useMemo(
    () =>
      [
        createColumn<PlatformIntegrationJob>({
          id: "organization",
          labelKey: "operations.integrations.columns.organization",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">
              {row.original.organization?.name ??
                t("operations.integrations.platform_job")}
            </span>
          ),
        }),
        createColumn<PlatformIntegrationJob>({
          accessorKey: "kind",
          labelKey: "integrations.jobs.columns.kind",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="flex flex-col">
              <span>
                {labelOf(t, INTEGRATION_JOB_KIND_LABEL_KEYS, row.original.kind)}
              </span>
              <span className="text-muted-foreground text-xs">
                {labelOf(
                  t,
                  INTEGRATION_PROVIDER_LABEL_KEYS,
                  row.original.provider,
                )}
              </span>
            </div>
          ),
        }),
        createColumn<PlatformIntegrationJob>({
          accessorKey: "status",
          labelKey: "integrations.jobs.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: INTEGRATION_JOB_STATUSES.map((s) => ({
            value: s,
            label: t(INTEGRATION_JOB_STATUS_LABEL_KEYS[s]),
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(INTEGRATION_JOB_STATUS_LABEL_KEYS[row.original.status])}
              tone={INTEGRATION_JOB_STATUS_TONES[row.original.status]}
            />
          ),
        }),
        createColumn<PlatformIntegrationJob>({
          accessorKey: "attempts",
          labelKey: "integrations.jobs.columns.attempts",
          enableSorting: false,
          cell: ({ row }) =>
            `${row.original.attempts} / ${row.original.max_attempts}`,
        }),
        createColumn<PlatformIntegrationJob>({
          accessorKey: "last_error",
          labelKey: "integrations.jobs.columns.last_error",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) =>
            row.original.last_error ? (
              <span className="text-destructive line-clamp-2 text-xs break-words">
                {row.original.last_error}
              </span>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<PlatformIntegrationJob>({
          accessorKey: "updated_at",
          labelKey: "integrations.jobs.columns.updated_at",
          enableSorting: true,
          cell: ({ row }) => datetime(row.original.updated_at, DATE, locale),
        }),
        createColumn<PlatformIntegrationJob>({
          accessorKey: "created_at",
          labelKey: "integrations.jobs.columns.created_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => datetime(row.original.created_at, DATE, locale),
        }),
        createColumn<PlatformIntegrationJob>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "open",
                  label: t("integrations.jobs.actions.open"),
                  icon: Eye,
                  onSelect: () => setOpenJob(row.original.uuid),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<PlatformIntegrationJob, unknown>[],
    [locale, t],
  );

  return (
    <EntitySectionCard title={t("operations.integrations.title")}>
      <p className="text-muted-foreground mb-4 text-sm">
        {t("operations.integrations.hint")}
      </p>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("integrations.jobs.load_failed")}
        onRetry={() => void query.refetch()}
        emptyTitle={t("operations.integrations.empty_title")}
        emptyDescription={t("operations.integrations.empty_description")}
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={listState.tableState}
        onRowClick={(row) => setOpenJob(row.uuid)}
        features={{
          persistKey: "platform-operations-integration-jobs-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
      <JobDrawer jobUuid={openJob} onClose={() => setOpenJob(null)} />
    </EntitySectionCard>
  );
}
