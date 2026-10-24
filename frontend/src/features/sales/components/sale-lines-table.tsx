"use client";

import { useMemo } from "react";

import {
  EntityTable,
  EntityToolbar,
  useServerListState,
  useUrlListDefaults,
} from "@/components/entity";
import { useSaleLineColumns } from "@/features/sales/components/sale-line-columns";
import { useSaleLineDialogs } from "@/features/sales/components/sale-line-dialogs";
import { SALE_LINES_RESOURCE } from "@/features/sales/constants";
import { useSaleLines } from "@/features/sales/hooks/use-sales";
import { useSalesFilterOptions } from "@/features/sales/hooks/use-sales-filter-options";
import type { ListSaleLinesParams, SalesPeriod } from "@/features/sales/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";

const FILTER_IDS = ["kind", "item_type", "branch", "staff"] as const;
const SORT_FIELDS = ["sale_date", "net_amount", "kind", "created_at"] as const;

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

function joined(values: string[]) {
  return values.length ? values.join(",") : undefined;
}

/**
 * Sale lines of the period (services delivered, products handed over and
 * their reversals) with server-side filters and live updates. A row opens
 * its drawer; a live line is reversed or corrected from the row or there.
 */
export function SaleLinesTable({ period }: { period: SalesPeriod }) {
  const { t } = useLocale();
  const { organization } = useTenant();
  const urlDefaults = useUrlListDefaults(FILTER_IDS, SORT_FIELDS);
  const listState = useServerListState({
    initialSort: urlDefaults.initialSort ?? "-sale_date",
    initialColumnFilters: urlDefaults.initialColumnFilters,
    initialPageSize: 50,
    realtime: tenantLiveList(organization, SALE_LINES_RESOURCE),
  });

  const params = useMemo<ListSaleLinesParams>(() => {
    const column = (id: string) =>
      joined(
        filterValues(
          listState.columnFilters.find((filter) => filter.id === id)?.value,
        ),
      );
    return {
      ...listState.params,
      kind: column("kind"),
      item_type: column("item_type"),
      branch: column("branch"),
      staff: column("staff"),
      date_from: period.date_from,
      date_to: period.date_to,
    };
  }, [listState.columnFilters, listState.params, period]);

  const query = useSaleLines(params);
  const { branchOptions, staffOptions } = useSalesFilterOptions();
  const dialogs = useSaleLineDialogs();
  const columns = useSaleLineColumns({
    onOpen: dialogs.open,
    onReverse: dialogs.reverse,
    onCorrect: dialogs.correct,
    branchOptions,
    staffOptions,
  });

  const total = query.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 50)),
  );
  const hasFilters = Boolean(
    params.q ||
    params.kind ||
    params.item_type ||
    params.branch ||
    params.staff,
  );

  return (
    <>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("sales.lines.load_failed")}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() => query.refetch())}
        onRowClick={dialogs.open}
        emptyTitle={
          hasFilters
            ? t("sales.empty_filtered_title")
            : t("sales.lines.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("sales.empty_filtered_description")
            : t("sales.lines.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-sale-lines-v1",
          rowSelection: false,
          viewMode: true,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
      {dialogs.element}
    </>
  );
}
