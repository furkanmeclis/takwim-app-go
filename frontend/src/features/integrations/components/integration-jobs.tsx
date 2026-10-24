"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, RotateCcw } from "lucide-react";
import { useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
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
import { PermissionGuard } from "@/components/common/permission-guard";
import {
  labelOf,
  ATTEMPT_OPERATION_LABEL_KEYS,
  ATTEMPT_OUTCOME_LABEL_KEYS,
  ATTEMPT_OUTCOME_TONES,
  ATTEMPT_TRIGGER_LABEL_KEYS,
  INTEGRATION_JOBS_RESOURCE,
  INTEGRATION_JOB_KIND_LABEL_KEYS,
  INTEGRATION_JOB_STATUSES,
  INTEGRATION_JOB_STATUS_LABEL_KEYS,
  INTEGRATION_JOB_STATUS_TONES,
  INTEGRATION_PROVIDER_LABEL_KEYS,
} from "@/features/integrations/constants";
import {
  useIntegrationJob,
  useIntegrationJobs,
  useRetryIntegrationJob,
} from "@/features/integrations/hooks/use-integrations";
import type {
  IntegrationAccount,
  IntegrationAttempt,
  IntegrationJob,
  IntegrationJobDetail,
} from "@/features/integrations/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const DATE = "d MMM yyyy HH:mm:ss";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  return [];
}

/**
 * Integration jobs (TEC-880): every outbound call and inbound notification
 * with its state, of one account or (account null) of the whole
 * organization, the work list of failed and unverified jobs. Live over the
 * `tenant.integration_jobs` channel. A row opens the job drawer.
 */
export function IntegrationJobsSection({
  account,
  onOpenJob,
}: {
  account: IntegrationAccount | null;
  onOpenJob: (uuid: string) => void;
}) {
  const { t, locale } = useLocale();
  const { organization } = useTenant();
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 10,
    realtime: tenantLiveList(organization, INTEGRATION_JOBS_RESOURCE),
  });
  const status = filterValues(
    listState.columnFilters.find((f) => f.id === "status")?.value,
  );
  const params = {
    limit: listState.params.limit,
    offset: listState.params.offset,
    sort: listState.params.sort,
    status: status.length ? status.join(",") : undefined,
  };
  const query = useIntegrationJobs(account?.uuid ?? null, params);
  const total = query.data?.total ?? 0;

  const columns = useMemo(
    () =>
      [
        createColumn<IntegrationJob>({
          accessorKey: "kind",
          labelKey: "integrations.jobs.columns.kind",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="flex flex-col">
              <span className="font-medium">
                {labelOf(t, INTEGRATION_JOB_KIND_LABEL_KEYS, row.original.kind)}
              </span>
              <span className="text-muted-foreground text-xs">
                {labelOf(
                  t,
                  INTEGRATION_PROVIDER_LABEL_KEYS,
                  row.original.provider,
                )}{" "}
                ·{" "}
                <span className="font-mono">
                  {row.original.uuid.slice(0, 8)}
                </span>
              </span>
            </div>
          ),
        }),
        createColumn<IntegrationJob>({
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
        createColumn<IntegrationJob>({
          accessorKey: "attempts",
          labelKey: "integrations.jobs.columns.attempts",
          enableSorting: false,
          cell: ({ row }) =>
            `${row.original.attempts} / ${row.original.max_attempts}`,
        }),
        createColumn<IntegrationJob>({
          accessorKey: "last_error",
          labelKey: "integrations.jobs.columns.last_error",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.last_error ? (
              <span className="text-destructive line-clamp-2 text-xs">
                {row.original.last_error}
              </span>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<IntegrationJob>({
          accessorKey: "next_attempt_at",
          labelKey: "integrations.jobs.columns.next_attempt_at",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.next_attempt_at
              ? datetime(row.original.next_attempt_at, DATE, locale)
              : "—",
        }),
        createColumn<IntegrationJob>({
          accessorKey: "created_at",
          labelKey: "integrations.jobs.columns.created_at",
          enableSorting: true,
          cell: ({ row }) => datetime(row.original.created_at, DATE, locale),
        }),
        createColumn<IntegrationJob>({
          accessorKey: "updated_at",
          labelKey: "integrations.jobs.columns.updated_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => datetime(row.original.updated_at, DATE, locale),
        }),
        createColumn<IntegrationJob>({
          id: "actions",
          labelKey: "integrations.jobs.columns.actions",
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
                  onSelect: () => onOpenJob(row.original.uuid),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<IntegrationJob, unknown>[],
    [locale, onOpenJob, t],
  );

  return (
    <EntitySectionCard
      title={
        account
          ? t("integrations.jobs.title", { account: account.display_name })
          : t("integrations.jobs.all_title")
      }
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("integrations.jobs.load_failed")}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={t("integrations.jobs.empty_title")}
        emptyDescription={t("integrations.jobs.empty_description")}
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 10)),
        )}
        state={listState.tableState}
        onRowClick={(row) => onOpenJob(row.uuid)}
        features={{
          persistKey: account
            ? "tenant-integration-jobs-v1"
            : "tenant-integration-jobs-all-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
    </EntitySectionCard>
  );
}

export function SummaryList({ value }: { value: Record<string, unknown> }) {
  const entries = Object.entries(value);
  if (!entries.length) return <span className="text-muted-foreground">—</span>;
  return (
    <div className="flex flex-col gap-0.5 font-mono text-xs">
      {entries.map(([k, v]) => (
        <span key={k} className="break-all">
          <span className="text-muted-foreground">{k}:</span>{" "}
          {typeof v === "object" ? JSON.stringify(v) : String(v)}
        </span>
      ))}
    </div>
  );
}

function Fact({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-muted-foreground text-xs">{label}</span>
      <span className="text-sm">{value}</span>
    </div>
  );
}

/**
 * Job drawer: state, references and every provider call (attempts). "Yeniden
 * dene" asks the provider first and resends only work it confirms was not
 * done.
 */
export function IntegrationJobDrawer({
  jobUuid,
  onClose,
}: {
  jobUuid: string | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const query = useIntegrationJob(jobUuid);
  const retry = useRetryIntegrationJob();
  const job = query.data;
  return (
    <EntityDrawer
      open={Boolean(jobUuid)}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      size="xl"
      title={t("integrations.jobs.detail_title")}
      description={t("integrations.jobs.detail_description")}
      footer={
        job?.retryable ? (
          <PermissionGuard permission={permissions.tenant.integrations.manage}>
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
        <IntegrationJobDetailBody job={job} />
      )}
    </EntityDrawer>
  );
}

/**
 * Every provider call of a job (submit / status check, trigger, outcome,
 * short request / response summaries, time and duration). Used by the
 * integration job drawer.
 */
export function IntegrationAttemptsTable({
  attempts,
  persistKey,
}: {
  attempts: IntegrationAttempt[];
  persistKey: string;
}) {
  const { t, locale } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<IntegrationAttempt>({
          accessorKey: "attempt_no",
          labelKey: "integrations.jobs.attempts.columns.no",
          enableSorting: false,
          cell: ({ row }) => `#${row.original.attempt_no}`,
        }),
        createColumn<IntegrationAttempt>({
          accessorKey: "operation",
          labelKey: "integrations.jobs.attempts.columns.operation",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="flex flex-col">
              <span>
                {labelOf(
                  t,
                  ATTEMPT_OPERATION_LABEL_KEYS,
                  row.original.operation,
                )}
              </span>
              <span className="text-muted-foreground text-xs">
                {labelOf(t, ATTEMPT_TRIGGER_LABEL_KEYS, row.original.trigger)}
              </span>
            </div>
          ),
        }),
        createColumn<IntegrationAttempt>({
          accessorKey: "outcome",
          labelKey: "integrations.jobs.attempts.columns.outcome",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="flex flex-col gap-1">
              <StatusChip
                label={labelOf(
                  t,
                  ATTEMPT_OUTCOME_LABEL_KEYS,
                  row.original.outcome,
                )}
                tone={ATTEMPT_OUTCOME_TONES[row.original.outcome] ?? "default"}
              />
              {row.original.error ? (
                <span className="text-destructive text-xs break-words">
                  {row.original.error}
                </span>
              ) : null}
            </div>
          ),
        }),
        createColumn<IntegrationAttempt>({
          id: "summary",
          labelKey: "integrations.jobs.attempts.columns.summary",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="flex flex-col gap-1">
              <SummaryList value={row.original.request_summary} />
              <SummaryList value={row.original.response_summary} />
            </div>
          ),
        }),
        createColumn<IntegrationAttempt>({
          accessorKey: "started_at",
          labelKey: "integrations.jobs.attempts.columns.started_at",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="flex flex-col">
              <span>{datetime(row.original.started_at, DATE, locale)}</span>
              <span className="text-muted-foreground text-xs">
                {t("integrations.jobs.attempts.duration", {
                  ms: row.original.duration_ms,
                })}
              </span>
            </div>
          ),
        }),
      ] as ColumnDef<IntegrationAttempt, unknown>[],
    [locale, t],
  );
  return (
    <EntityTable
      columns={columns}
      data={attempts}
      getRowId={(row) => row.uuid}
      emptyTitle={t("integrations.jobs.attempts.empty")}
      manual={{ filtering: false, sorting: false, pagination: false }}
      features={{
        persistKey,
        rowSelection: false,
        columnFilters: false,
        facetedFilters: false,
        sorting: false,
        pagination: false,
      }}
    />
  );
}

/**
 * The facts, payload and provider calls of one job: the business's job
 * drawer and the platform's provider error drawer (TEC-930).
 */
export function IntegrationJobDetailBody({
  job,
  attemptsPersistKey = "tenant-integration-attempts-v1",
}: {
  job: IntegrationJobDetail;
  attemptsPersistKey?: string;
}) {
  const { t, locale } = useLocale();
  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <Fact
          label={t("integrations.jobs.columns.kind")}
          value={labelOf(t, INTEGRATION_JOB_KIND_LABEL_KEYS, job.kind)}
        />
        <Fact
          label={t("integrations.jobs.columns.status")}
          value={
            <StatusChip
              label={t(INTEGRATION_JOB_STATUS_LABEL_KEYS[job.status])}
              tone={INTEGRATION_JOB_STATUS_TONES[job.status]}
            />
          }
        />
        <Fact
          label={t("integrations.jobs.columns.attempts")}
          value={`${job.attempts} / ${job.max_attempts}`}
        />
        <Fact
          label={t("integrations.jobs.fields.priority")}
          value={t(
            job.priority === "critical"
              ? "integrations.jobs.priorities.critical"
              : "integrations.jobs.priorities.normal",
          )}
        />
        <Fact
          label={t("integrations.jobs.fields.provider_ref")}
          value={
            <span className="font-mono text-xs break-all">
              {job.provider_ref || "—"}
            </span>
          }
        />
        <Fact
          label={t("integrations.jobs.columns.next_attempt_at")}
          value={
            job.next_attempt_at
              ? datetime(job.next_attempt_at, DATE, locale)
              : "—"
          }
        />
        <Fact
          label={t("integrations.jobs.columns.created_at")}
          value={datetime(job.created_at, DATE, locale)}
        />
        <Fact
          label={t("integrations.jobs.fields.completed_at")}
          value={
            job.completed_at ? datetime(job.completed_at, DATE, locale) : "—"
          }
        />
      </div>
      {job.status === "needs_verification" ? (
        <p className="bg-muted rounded-md p-3 text-sm">
          {t("integrations.jobs.needs_verification_note")}
        </p>
      ) : null}
      {job.last_error ? (
        <Fact
          label={t("integrations.jobs.columns.last_error")}
          value={
            <span className="text-destructive break-words">
              {job.last_error}
            </span>
          }
        />
      ) : null}
      <Fact
        label={t("integrations.jobs.fields.payload")}
        value={<SummaryList value={job.payload} />}
      />
      <IntegrationAttemptsTable
        attempts={job.attempt_log}
        persistKey={attemptsPersistKey}
      />
    </div>
  );
}
