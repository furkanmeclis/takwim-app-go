"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatsCard } from "@/components/common/stats-card";
import { EntitySectionCard, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { amountTone, useCashAmount } from "@/features/cash/lib/format";
import { useFinanceSummary } from "@/features/finance/hooks/use-finance";
import { kindLabel, signedAmount } from "@/features/finance/lib/format";
import type {
  FinanceCategoryTotal,
  FinancePeriod,
} from "@/features/finance/types";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

function useCategoryTotalColumns() {
  const { t } = useLocale();
  const money = useCashAmount();
  return useMemo(
    () =>
      [
        createColumn<FinanceCategoryTotal>({
          id: "category",
          accessorFn: (row) => row.category.name,
          labelKey: "finance.summary.columns.category",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.category.name}</span>
          ),
        }),
        createColumn<FinanceCategoryTotal>({
          accessorKey: "kind",
          labelKey: "finance.summary.columns.kind",
          enableSorting: false,
          cell: ({ row }) => (
            <Badge
              variant={row.original.kind === "income" ? "success" : "danger"}
            >
              {kindLabel(t, row.original.kind)}
            </Badge>
          ),
        }),
        createColumn<FinanceCategoryTotal>({
          accessorKey: "entry_count",
          labelKey: "finance.summary.columns.entry_count",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.entry_count}</span>
          ),
        }),
        createColumn<FinanceCategoryTotal>({
          accessorKey: "total",
          labelKey: "finance.summary.columns.total",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => {
            const signed = signedAmount(row.original.kind, row.original.total);
            return (
              <span
                className={cn("font-medium tabular-nums", amountTone(signed))}
              >
                {money(signed, row.original.currency)}
              </span>
            );
          },
        }),
      ] as ColumnDef<FinanceCategoryTotal, unknown>[],
    [money, t],
  );
}

/**
 * Figures of the period per currency, kept apart: sale revenue, other
 * income, expenses and staff payouts (not an expense); net = revenue +
 * income − expenses. Below, the total of each category.
 */
export function FinanceSummary({ period }: { period: FinancePeriod }) {
  const { t } = useLocale();
  const money = useCashAmount();
  const query = useFinanceSummary(period);
  const columns = useCategoryTotalColumns();
  const data = query.data;

  return (
    <div className="space-y-6">
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("finance.summary.load_failed")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {data && data.totals.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("finance.summary.empty")}
        </p>
      ) : null}

      {(data?.totals ?? []).map((totals) => (
        <div key={totals.currency} className="space-y-2">
          {(data?.totals.length ?? 0) > 1 ? (
            <h3 className="text-muted-foreground text-sm font-medium">
              {totals.currency}
            </h3>
          ) : null}
          <div className="grid grid-cols-2 gap-4 lg:grid-cols-5">
            <StatsCard
              title={t("finance.summary.revenue")}
              value={money(totals.revenue, totals.currency)}
              hint={t("finance.summary.revenue_hint")}
            />
            <StatsCard
              title={t("finance.summary.income")}
              value={money(totals.income, totals.currency)}
              hint={t("finance.summary.income_hint")}
            />
            <StatsCard
              title={t("finance.summary.expenses")}
              value={money(totals.expenses, totals.currency)}
              hint={t("finance.summary.expenses_hint")}
            />
            <StatsCard
              title={t("finance.summary.staff_payments")}
              value={money(totals.staff_payments, totals.currency)}
              hint={t("finance.summary.staff_payments_hint")}
            />
            <StatsCard
              title={t("finance.summary.net")}
              value={
                <span className={cn("tabular-nums", amountTone(totals.net))}>
                  {money(totals.net, totals.currency)}
                </span>
              }
              hint={t("finance.summary.net_hint")}
            />
          </div>
        </div>
      ))}

      <EntitySectionCard
        title={t("finance.summary.categories_title")}
        badge={data ? data.categories.length : undefined}
      >
        <EntityTable
          columns={columns}
          data={data?.categories ?? []}
          getRowId={(row) => `${row.kind}-${row.category.uuid}-${row.currency}`}
          isLoading={query.isLoading}
          isError={query.isError}
          onRetry={() => void query.refetch()}
          emptyTitle={t("finance.summary.categories_empty_title")}
          emptyDescription={t("finance.summary.categories_empty_description")}
          manual={{ filtering: false, sorting: false, pagination: false }}
          features={{
            persistKey: "tenant-finance-category-totals-v1",
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
