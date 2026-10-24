"use client";

import { useCallback, useMemo, useState } from "react";

import {
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { ReasonDialog } from "@/features/cash/components/reason-dialog";
import { ResourceIOToolbar } from "@/features/io";
import { useStaffShareLineColumns } from "@/features/staff-shares/components/staff-share-line-columns";
import { useStaffSharePayoutColumns } from "@/features/staff-shares/components/staff-share-payout-columns";
import {
  STAFF_SHARE_LINES_RESOURCE,
  STAFF_SHARE_PAYOUTS_RESOURCE,
} from "@/features/staff-shares/constants";
import {
  useReverseStaffSharePayout,
  useStaffShareLines,
  useStaffShareLinesMeta,
  useStaffSharePayouts,
  useStaffSharePayoutsMeta,
} from "@/features/staff-shares/hooks/use-staff-shares";
import type {
  ListStaffShareLinesParams,
  ListStaffSharePayoutsParams,
  StaffSharePayout,
  StaffSharePeriod,
  StaffShareStatus,
} from "@/features/staff-shares/types";
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

/** The list query as an export takes it (paging left out). */
function exportQuery(
  params: Record<string, string | number | undefined>,
): Record<string, string | undefined> {
  const out: Record<string, string | undefined> = {};
  for (const [key, value] of Object.entries(params)) {
    if (key === "limit" || key === "offset" || value === undefined) continue;
    out[key] = String(value);
  }
  return out;
}

type ListProps = {
  period: StaffSharePeriod;
  /** All staff (tenant page) or only the member's own records. */
  showStaff: boolean;
  /** Follow live list events (needs `tenant.earnings.read`). */
  live: boolean;
  persistKey: string;
};

/** Calculated shares of the period, newest first. */
export function StaffShareLinesTable({
  period,
  showStaff,
  live,
  persistKey,
}: ListProps) {
  const { t } = useLocale();
  const { organization } = useTenant();
  const listState = useServerListState({
    initialSort: "-share_date",
    initialPageSize: 50,
    realtime: live
      ? tenantLiveList(organization, STAFF_SHARE_LINES_RESOURCE)
      : undefined,
  });

  const params = useMemo<ListStaffShareLinesParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    const status = column("status");
    return {
      ...listState.params,
      ...period,
      kind: joined(column("kind")),
      item_type: joined(column("item_type")),
      status: status.length === 1 ? (status[0] as StaffShareStatus) : undefined,
    };
  }, [listState.columnFilters, listState.params, period]);

  const query = useStaffShareLines(params);
  // Exports are for viewers of every share (the business page).
  const meta = useStaffShareLinesMeta(live);
  const columns = useStaffShareLineColumns({ showStaff, filters: true });
  const total = query.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 50)),
  );
  const hasFilters = Boolean(
    params.q || params.kind || params.item_type || params.status,
  );

  return (
    <EntityTable
      columns={columns}
      data={query.data?.items ?? []}
      getRowId={(row) => row.uuid}
      isLoading={query.isLoading}
      isError={query.isError}
      errorDescription={t("staff_shares.lines.load_failed")}
      onRetry={() => void query.refetch()}
      realtime={listState.realtimeFor(() => query.refetch())}
      emptyTitle={
        hasFilters
          ? t("staff_shares.empty_filtered_title")
          : t("staff_shares.lines.empty_title")
      }
      emptyDescription={
        hasFilters
          ? t("staff_shares.empty_filtered_description")
          : t("staff_shares.lines.empty_description")
      }
      pageCount={pageCount}
      state={listState.tableState}
      features={{ persistKey, rowSelection: false, viewMode: true }}
      toolbarExtra={
        <>
          {live ? (
            <ResourceIOToolbar
              resource={STAFF_SHARE_LINES_RESOURCE}
              query={exportQuery(params)}
              capabilities={meta.data?.capabilities}
            />
          ) : null}
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        </>
      }
    />
  );
}

/** Payouts of the period with their reversals. */
export function StaffSharePayoutsTable({
  period,
  showStaff,
  live,
  persistKey,
  canReverse,
}: ListProps & { canReverse: boolean }) {
  const { t } = useLocale();
  const { organization } = useTenant();
  const listState = useServerListState({
    initialSort: "-payout_date",
    initialPageSize: 50,
    realtime: live
      ? tenantLiveList(organization, STAFF_SHARE_PAYOUTS_RESOURCE)
      : undefined,
  });

  const params = useMemo<ListStaffSharePayoutsParams>(() => {
    const kind = filterValues(
      listState.columnFilters.find((filter) => filter.id === "kind")?.value,
    );
    return { ...listState.params, ...period, kind: joined(kind) };
  }, [listState.columnFilters, listState.params, period]);

  const query = useStaffSharePayouts(params);
  const meta = useStaffSharePayoutsMeta(live);
  const reverse = useReverseStaffSharePayout();
  const [target, setTarget] = useState<StaffSharePayout | null>(null);
  const onReverse = useCallback((p: StaffSharePayout) => setTarget(p), []);
  const columns = useStaffSharePayoutColumns({
    showStaff,
    filters: true,
    onReverse: canReverse ? onReverse : undefined,
  });
  const total = query.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 50)),
  );
  const hasFilters = Boolean(params.q || params.kind);

  return (
    <>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("staff_shares.payouts.load_failed")}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={
          hasFilters
            ? t("staff_shares.empty_filtered_title")
            : t("staff_shares.payouts.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("staff_shares.empty_filtered_description")
            : t("staff_shares.payouts.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{ persistKey, rowSelection: false, viewMode: true }}
        toolbarExtra={
          <>
            {live ? (
              <ResourceIOToolbar
                resource={STAFF_SHARE_PAYOUTS_RESOURCE}
                query={exportQuery(params)}
                capabilities={meta.data?.capabilities}
              />
            ) : null}
            <EntityToolbar
              onRefresh={() => void query.refetch()}
              refreshDisabled={query.isFetching}
            />
          </>
        }
      />
      {canReverse ? (
        <ReasonDialog
          open={Boolean(target)}
          title={t("staff_shares.reverse_dialog.title")}
          description={t("staff_shares.reverse_dialog.description")}
          confirmLabel={t("staff_shares.actions.reverse_payout")}
          isPending={reverse.isPending}
          onOpenChange={(open) => {
            if (!open) setTarget(null);
          }}
          onConfirm={(reason) =>
            reverse.mutateAsync({ uuid: target!.uuid, reason })
          }
        />
      ) : null}
    </>
  );
}
