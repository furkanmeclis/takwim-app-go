"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatsCard } from "@/components/common/stats-card";
import { EntitySectionCard, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { useCashAmount } from "@/features/cash/lib/format";
import { useSalesSummary } from "@/features/sales/hooks/use-sales";
import type { SalesFigures, SalesPeriod } from "@/features/sales/types";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type DayRow = SalesFigures & { date: string };

function useDayColumns() {
  const { locale } = useLocale();
  const money = useCashAmount();
  return useMemo(
    () =>
      [
        createColumn<DayRow>({
          accessorKey: "date",
          labelKey: "sales.summary.columns.date",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">
              {date(row.original.date, undefined, locale)}
            </span>
          ),
        }),
        createColumn<DayRow>({
          accessorKey: "currency",
          labelKey: "sales.summary.columns.currency",
          enableSorting: false,
          defaultHidden: true,
        }),
        createColumn<DayRow>({
          id: "revenue",
          accessorFn: (row) => row.revenue.net,
          labelKey: "sales.summary.columns.revenue",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {money(row.original.revenue.net, row.original.currency)}
            </span>
          ),
        }),
        createColumn<DayRow>({
          id: "collections",
          accessorFn: (row) => row.collections.net,
          labelKey: "sales.summary.columns.collections",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {money(row.original.collections.net, row.original.currency)}
            </span>
          ),
        }),
        createColumn<DayRow>({
          accessorKey: "transfers",
          labelKey: "sales.summary.columns.transfers",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {money(row.original.transfers, row.original.currency)}
            </span>
          ),
        }),
        createColumn<DayRow>({
          accessorKey: "staff_payments",
          labelKey: "sales.summary.columns.staff_payments",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {money(row.original.staff_payments, row.original.currency)}
            </span>
          ),
        }),
      ] as ColumnDef<DayRow, unknown>[],
    [locale, money],
  );
}

/**
 * Figures of the period per currency, each from its own source (A20):
 * revenue from the sale lines, collections from the ledger, transfers
 * between accounts and staff payouts from the cash book. They are never
 * added together. Below, the same figures per day.
 */
export function SalesSummary({ period }: { period: SalesPeriod }) {
  const { t } = useLocale();
  const money = useCashAmount();
  const query = useSalesSummary(period);
  const columns = useDayColumns();
  const data = query.data;
  const days = useMemo<DayRow[]>(
    () =>
      [...(data?.days ?? [])]
        .reverse()
        .flatMap((day) =>
          day.figures.map((figures) => ({ ...figures, date: day.date })),
        ),
    [data],
  );
  const multiCurrency = (data?.totals.length ?? 0) > 1;

  return (
    <div className="space-y-6">
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("sales.summary.load_failed")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {data && data.totals.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("sales.summary.empty")}
        </p>
      ) : null}

      {(data?.totals ?? []).map((totals) => (
        <div key={totals.currency} className="space-y-2">
          {multiCurrency ? (
            <h3 className="text-muted-foreground text-sm font-medium">
              {totals.currency}
            </h3>
          ) : null}
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <StatsCard
              title={t("sales.summary.revenue")}
              value={money(totals.revenue.net, totals.currency)}
              hint={t("sales.summary.revenue_hint", {
                gross: money(totals.revenue.gross, totals.currency),
                discount: money(totals.revenue.discount, totals.currency),
                reversed: money(totals.revenue.reversed, totals.currency),
              })}
            />
            <StatsCard
              title={t("sales.summary.collections")}
              value={money(totals.collections.net, totals.currency)}
              hint={t("sales.summary.collections_hint", {
                collected: money(totals.collections.collected, totals.currency),
                refunded: money(totals.collections.refunded, totals.currency),
              })}
            />
            <StatsCard
              title={t("sales.summary.transfers")}
              value={money(totals.transfers, totals.currency)}
              hint={t("sales.summary.transfers_hint")}
            />
            <StatsCard
              title={t("sales.summary.staff_payments")}
              value={money(totals.staff_payments, totals.currency)}
              hint={t("sales.summary.staff_payments_hint")}
            />
          </div>
        </div>
      ))}

      <EntitySectionCard
        title={t("sales.summary.days_title")}
        badge={data ? data.days.length : undefined}
        collapsible
        defaultOpen={false}
      >
        <EntityTable
          columns={columns}
          data={days}
          getRowId={(row) => `${row.date}-${row.currency}`}
          isLoading={query.isLoading}
          isError={query.isError}
          onRetry={() => void query.refetch()}
          emptyTitle={t("sales.summary.days_empty_title")}
          emptyDescription={t("sales.summary.days_empty_description")}
          manual={{ filtering: false, sorting: false, pagination: false }}
          features={{
            persistKey: "tenant-sales-summary-days-v1",
            rowSelection: false,
            columnFilters: false,
            facetedFilters: false,
            globalFilter: false,
            sorting: false,
            pagination: false,
            viewMode: true,
          }}
        />
      </EntitySectionCard>
    </div>
  );
}
