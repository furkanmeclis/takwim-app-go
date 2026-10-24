"use client";

import { useRouter } from "next/navigation";
import { useCallback, useMemo, useRef, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityCreateButton,
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
  useUrlListDefaults,
} from "@/components/entity";
import { createSelectColumnDef } from "@/components/tables";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { routes } from "@/config/routes";
import {
  BulkActionMenu,
  SelectionBanner,
  resolveBulkActionsWithIcons,
  useBulkSelection,
} from "@/features/bulk-engine";
import type { BulkActionDef, BulkSummary } from "@/features/bulk-engine/types";
import { ResourceIOToolbar } from "@/features/io";
import {
  ReservationActionDialog,
  type ActionTarget,
} from "@/features/reservations/components/reservation-action-dialog";
import { ReservationBulkCancelDialog } from "@/features/reservations/components/reservation-bulk-cancel-dialog";
import {
  ReservationBulkReportDialog,
  type BulkReport,
} from "@/features/reservations/components/reservation-bulk-report-dialog";
import { useReservationColumns } from "@/features/reservations/components/reservation-columns";
import type { ReservationRowActionHandlers } from "@/features/reservations/components/reservation-row-actions";
import { RescheduleDialog } from "@/features/reservations/components/reschedule-dialog";
import { RESERVATIONS_RESOURCE } from "@/features/reservations/constants";
import {
  useReservationsFacets,
  useReservationsList,
  useReservationsMeta,
} from "@/features/reservations/hooks/use-reservations";
import {
  READ_ANY,
  useCanWriteReservations,
} from "@/features/reservations/lib/permissions";
import type {
  ListReservationsParams,
  ReservationListItem,
} from "@/features/reservations/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

function joined(values: string[]) {
  return values.length ? values.join(",") : undefined;
}

type Period = "all" | "upcoming" | "past";

/** Column filters another page may set through the URL. */
const URL_FILTER_IDS = [
  "status",
  "approval_status",
  "confirmation_status",
  "payment_status",
] as const;

type CancelPrompt = {
  resolve: (params: Record<string, string> | null) => void;
};

/**
 * Tenant list standard: server-side pagination / sort / search (number,
 * customer, phone), faceted filters, column visibility, row actions, row
 * click → detail, live refresh, export (same filters) and bulk accept /
 * reject / cancel. Members with only their own calendar see their own
 * visits (the API scopes the rows, the export and the bulk targets).
 *
 * Bulk runs send the versions of the selected rows; a booking changed
 * meanwhile, or one the action does not apply to, is skipped and listed in
 * the result report. Bulk changes cannot be rolled back.
 */
export function ReservationsPage() {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const canWrite = useCanWriteReservations();

  // The home page links here with filters (`?approval_status=pending`).
  const urlDefaults = useUrlListDefaults(URL_FILTER_IDS);
  const listState = useServerListState({
    initialSort: "",
    initialColumnFilters: urlDefaults.initialColumnFilters,
    initialPageSize: 20,
    realtime: tenantLiveList(organization, RESERVATIONS_RESOURCE),
  });

  const [period, setPeriod] = useState<{ value: Period; at: string }>({
    value: "all",
    at: "",
  });
  const [actionTarget, setActionTarget] = useState<ActionTarget | null>(null);
  const [rescheduleUuid, setRescheduleUuid] = useState<string | null>(null);
  const [cancelPrompt, setCancelPrompt] = useState<CancelPrompt | null>(null);
  const cancelPromptRef = useRef<CancelPrompt | null>(null);
  const [report, setReport] = useState<BulkReport | null>(null);

  const listParams = useMemo<ListReservationsParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    return {
      ...listState.params,
      branch: joined(column("branch")),
      staff: joined(column("staff")),
      status: joined(column("status")),
      approval_status: joined(column("approval_status")),
      confirmation_status: joined(column("confirmation_status")),
      payment_status: joined(column("payment_status")),
      source: joined(column("source")),
      starts_from: period.value === "upcoming" ? period.at : undefined,
      starts_to: period.value === "past" ? period.at : undefined,
    };
  }, [listState.columnFilters, listState.params, period]);

  const listQuery = useReservationsList(listParams);
  const metaQuery = useReservationsMeta();
  const facetsQuery = useReservationsFacets();

  const openDetail = useCallback(
    (row: ReservationListItem) =>
      router.push(routes.tenant.reservations.detail(slug, row.uuid)),
    [router, slug],
  );

  const handlers = useMemo<ReservationRowActionHandlers>(
    () => ({
      onView: openDetail,
      onAction: (row, action) =>
        setActionTarget({
          uuid: row.uuid,
          number: row.number,
          version: row.version,
          action,
        }),
      onReschedule: (row) => setRescheduleUuid(row.uuid),
    }),
    [openDetail],
  );

  const baseColumns = useReservationColumns({
    handlers,
    facets: facetsQuery.data,
  });
  const columns = useMemo(
    () => [createSelectColumnDef<ReservationListItem>(), ...baseColumns],
    [baseColumns],
  );

  // Same filters for the export and the "all matching" bulk scope.
  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      sort: listParams.sort,
      branch: listParams.branch,
      staff: listParams.staff,
      status: listParams.status,
      approval_status: listParams.approval_status,
      confirmation_status: listParams.confirmation_status,
      payment_status: listParams.payment_status,
      source: listParams.source,
      starts_from: listParams.starts_from,
      starts_to: listParams.starts_to,
    }),
    [
      listParams.approval_status,
      listParams.branch,
      listParams.confirmation_status,
      listParams.payment_status,
      listParams.q,
      listParams.sort,
      listParams.source,
      listParams.staff,
      listParams.starts_from,
      listParams.starts_to,
      listParams.status,
    ],
  );

  const total = listQuery.data?.total ?? 0;
  const bulkSelection = useBulkSelection({
    listQueryKey: listParams,
    bulkQuery: engineQuery,
    total,
  });

  const meta = metaQuery.data;
  const bulkActions = useMemo(
    () =>
      resolveBulkActionsWithIcons(
        RESERVATIONS_RESOURCE,
        meta?.bulk_actions ?? [],
      ),
    [meta?.bulk_actions],
  );

  // The versions of the selected rows go along: a booking changed since
  // it was listed is skipped instead of overwritten.
  const versionsParam = useCallback(() => {
    if (bulkSelection.scope.mode !== "page") return undefined;
    const selected = new Set(bulkSelection.scope.ids);
    const pairs = (listQuery.data?.items ?? [])
      .filter((row) => selected.has(row.uuid))
      .map((row) => `${row.uuid}:${row.version}`);
    return pairs.length ? pairs.join(",") : undefined;
  }, [bulkSelection.scope, listQuery.data?.items]);

  const resolveParams = useCallback(
    (action: BulkActionDef) => {
      const versions = versionsParam();
      const withVersions = (params: Record<string, string>) =>
        versions ? { ...params, versions } : params;
      if (action.id !== "cancel") {
        return Promise.resolve(versions ? withVersions({}) : undefined);
      }
      return new Promise<Record<string, string> | null>((resolve) => {
        const prompt: CancelPrompt = {
          resolve: (params) => resolve(params ? withVersions(params) : null),
        };
        cancelPromptRef.current = prompt;
        setCancelPrompt(prompt);
      });
    },
    [versionsParam],
  );

  const closeCancelPrompt = (params: Record<string, string> | null) => {
    cancelPromptRef.current?.resolve(params);
    cancelPromptRef.current = null;
    setCancelPrompt(null);
  };

  const showReport = useCallback(
    (action: BulkActionDef, summary: BulkSummary) => {
      if ((summary.skipped ?? 0) + summary.failed > 0) {
        setReport({ title: t(action.label_key), summary });
      }
    },
    [t],
  );
  const pageCount = useMemo(() => {
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listState.pagination.pageSize, total]);

  const hasFilters =
    Boolean(listParams.q) ||
    Boolean(listParams.branch) ||
    Boolean(listParams.staff) ||
    Boolean(listParams.status) ||
    Boolean(listParams.approval_status) ||
    Boolean(listParams.confirmation_status) ||
    Boolean(listParams.payment_status) ||
    Boolean(listParams.source) ||
    period.value !== "all";

  const refresh = () => {
    void listQuery.refetch();
    void facetsQuery.refetch();
  };

  return (
    <EntityPage
      title={t("reservations.title")}
      description={t("reservations.description")}
      permission={READ_ANY}
      permissionMode="any"
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("reservations.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("reservations.title") },
      ]}
      actions={
        canWrite ? (
          <EntityCreateButton
            onClick={() => router.push(routes.tenant.reservations.create(slug))}
            label={t("reservations.actions.create")}
          />
        ) : null
      }
    >
      <SelectionBanner
        selectedCount={bulkSelection.selectedCount}
        total={total}
        showSelectAll={bulkSelection.showSelectAllBanner}
        allMatchingSelected={bulkSelection.scope.mode === "all"}
        onSelectAllMatching={bulkSelection.selectAllMatching}
        onClearSelection={bulkSelection.clearSelection}
      />
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={openDetail}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("reservations.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("reservations.empty_filtered_title")
            : t("reservations.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("reservations.empty_filtered_description")
            : t("reservations.empty_description")
        }
        pageCount={pageCount}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: "tenant-reservations-v1",
          rowSelection: true,
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <Select
              value={period.value}
              onValueChange={(value) => {
                setPeriod({
                  value: value as Period,
                  // Fixed at the choice, so the query key stays stable.
                  at: new Date().toISOString(),
                });
                listState.tableState.onPaginationChange?.((prev) => ({
                  ...prev,
                  pageIndex: 0,
                }));
              }}
            >
              <SelectTrigger
                size="sm"
                className="w-36"
                aria-label={t("reservations.period.label")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">
                  {t("reservations.period.all")}
                </SelectItem>
                <SelectItem value="upcoming">
                  {t("reservations.period.upcoming")}
                </SelectItem>
                <SelectItem value="past">
                  {t("reservations.period.past")}
                </SelectItem>
              </SelectContent>
            </Select>
            <BulkActionMenu
              resource={RESERVATIONS_RESOURCE}
              actions={bulkActions}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              resolveParams={resolveParams}
              onResult={showReport}
              onComplete={() => {
                bulkSelection.clearSelection();
                refresh();
              }}
            />
            <ResourceIOToolbar
              resource={RESERVATIONS_RESOURCE}
              query={engineQuery}
              capabilities={meta?.capabilities}
            />
            <EntityToolbar
              onRefresh={refresh}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />

      <ReservationActionDialog
        target={actionTarget}
        onOpenChange={(open) => {
          if (!open) setActionTarget(null);
        }}
      />
      <RescheduleDialog
        uuid={rescheduleUuid}
        onOpenChange={(open) => {
          if (!open) setRescheduleUuid(null);
        }}
      />
      <ReservationBulkCancelDialog
        open={Boolean(cancelPrompt)}
        count={bulkSelection.selectedCount}
        onSubmit={(reason) => closeCancelPrompt({ reason })}
        onCancel={() => closeCancelPrompt(null)}
      />
      <ReservationBulkReportDialog
        report={report}
        onOpenChange={(open) => {
          if (!open) setReport(null);
        }}
      />
    </EntityPage>
  );
}
