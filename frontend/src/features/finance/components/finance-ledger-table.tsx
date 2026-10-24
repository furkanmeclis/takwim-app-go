"use client";

import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

import {
  EntityTable,
  EntityToolbar,
  useServerListState,
  useUrlListDefaults,
} from "@/components/entity";
import { routes } from "@/config/routes";
import { ReasonDialog } from "@/features/cash/components/reason-dialog";
import { useFinanceLedgerColumns } from "@/features/finance/components/finance-ledger-columns";
import { FINANCE_ENTRIES_RESOURCE } from "@/features/finance/constants";
import {
  useFinanceEntries,
  useFinanceEntriesMeta,
  useFinanceFacets,
  useReverseFinanceEntry,
} from "@/features/finance/hooks/use-finance";
import type {
  FinanceLedgerRow,
  FinancePeriod,
  ListFinanceEntriesParams,
} from "@/features/finance/types";
import { ResourceIOToolbar } from "@/features/io";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";

const FILTER_IDS = ["source", "kind", "category", "branch", "staff"] as const;
const SORT_FIELDS = ["entry_date", "amount", "created_at"] as const;

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

function joined(values: string[]) {
  return values.length ? values.join(",") : undefined;
}

/**
 * Income and expenses of the period: the business's records and the sale
 * lines (automatic income), with faceted filters, CSV / XLSX / PDF export
 * and live updates. A record opens its detail page; a wrong one is
 * reversed from the row.
 */
export function FinanceLedgerTable({ period }: { period: FinancePeriod }) {
  const { t } = useLocale();
  const router = useRouter();
  const { slug, organization } = useTenant();
  const urlDefaults = useUrlListDefaults(FILTER_IDS, SORT_FIELDS);

  const listState = useServerListState({
    initialSort: urlDefaults.initialSort ?? "-entry_date",
    initialColumnFilters: urlDefaults.initialColumnFilters,
    initialPageSize: 50,
    realtime: tenantLiveList(organization, FINANCE_ENTRIES_RESOURCE),
  });

  const listParams = useMemo<ListFinanceEntriesParams>(() => {
    const column = (id: string) =>
      joined(
        filterValues(
          listState.columnFilters.find((filter) => filter.id === id)?.value,
        ),
      );
    return {
      ...listState.params,
      source: column("source"),
      kind: column("kind"),
      category: column("category"),
      branch: column("branch"),
      staff: column("staff"),
      date_from: period.date_from,
      date_to: period.date_to,
    };
  }, [listState.columnFilters, listState.params, period]);

  const listQuery = useFinanceEntries(listParams);
  const metaQuery = useFinanceEntriesMeta();
  const facetsQuery = useFinanceFacets();
  const reverse = useReverseFinanceEntry();
  const [target, setTarget] = useState<FinanceLedgerRow | null>(null);

  const onOpen = useCallback(
    (row: FinanceLedgerRow) => {
      if (row.source === "manual") {
        router.push(routes.tenant.finance.detail(slug, row.uuid));
      } else if (row.reservation_uuid) {
        router.push(
          routes.tenant.reservations.detail(slug, row.reservation_uuid),
        );
      }
    },
    [router, slug],
  );
  const onReverse = useCallback((row: FinanceLedgerRow) => setTarget(row), []);
  const columns = useFinanceLedgerColumns({
    facets: facetsQuery.data,
    onOpen,
    onReverse,
  });

  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      source: listParams.source,
      kind: listParams.kind,
      category: listParams.category,
      branch: listParams.branch,
      staff: listParams.staff,
      date_from: listParams.date_from,
      date_to: listParams.date_to,
      sort: listParams.sort,
    }),
    [listParams],
  );

  const total = listQuery.data?.total ?? 0;
  const pageCount = useMemo(() => {
    const size = listState.pagination.pageSize || 50;
    return Math.max(1, Math.ceil(total / size));
  }, [listState.pagination.pageSize, total]);

  const hasFilters =
    Boolean(listParams.q) ||
    listState.columnFilters.some(
      (filter) => filterValues(filter.value).length > 0,
    );

  const refresh = () => {
    void listQuery.refetch();
    void facetsQuery.refetch();
  };

  return (
    <>
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => `${row.source}-${row.uuid}`}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("finance.entries.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        onRowClick={onOpen}
        emptyTitle={
          hasFilters
            ? t("finance.empty_filtered_title")
            : t("finance.entries.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("finance.empty_filtered_description")
            : t("finance.entries.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-finance-entries-v1",
          rowSelection: false,
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <ResourceIOToolbar
              resource={FINANCE_ENTRIES_RESOURCE}
              query={engineQuery}
              capabilities={metaQuery.data?.capabilities}
            />
            <EntityToolbar
              onRefresh={refresh}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />
      <ReasonDialog
        open={Boolean(target)}
        title={t("finance.reverse_dialog.title")}
        description={t("finance.reverse_dialog.description")}
        confirmLabel={t("finance.actions.reverse")}
        isPending={reverse.isPending}
        onOpenChange={(open) => {
          if (!open) setTarget(null);
        }}
        onConfirm={(reason) =>
          reverse.mutateAsync({ uuid: target!.uuid, reason })
        }
      />
    </>
  );
}
