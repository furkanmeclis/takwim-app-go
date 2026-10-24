"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { EntitySectionCard, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { useCashAmount } from "@/features/cash/lib/format";
import {
  useCanReadSales,
  useCustomerBalance,
} from "@/features/sales/hooks/use-sales";
import type { LedgerTotals } from "@/features/sales/types";
import { cn } from "@/lib/utils";
import { toMinor } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function balanceTone(value: string) {
  const minor = toMinor(value) ?? 0;
  if (minor > 0) return "text-destructive";
  if (minor < 0) return "text-emerald-700 dark:text-emerald-300";
  return "text-muted-foreground";
}

function useTotalsColumns() {
  const money = useCashAmount();
  return useMemo(() => {
    const amountColumn = (
      key: "charged" | "collected" | "refunded" | "paid",
      labelKey: string,
      hidden = false,
    ) =>
      createColumn<LedgerTotals>({
        accessorKey: key,
        labelKey,
        enableSorting: false,
        defaultHidden: hidden,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {money(row.original[key], row.original.currency)}
          </span>
        ),
      });
    return [
      createColumn<LedgerTotals>({
        accessorKey: "currency",
        labelKey: "sales.balance.columns.currency",
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-medium">{row.original.currency}</span>
        ),
      }),
      amountColumn("charged", "sales.balance.columns.charged"),
      amountColumn("collected", "sales.balance.columns.collected", true),
      amountColumn("refunded", "sales.balance.columns.refunded", true),
      amountColumn("paid", "sales.balance.columns.paid"),
      createColumn<LedgerTotals>({
        accessorKey: "balance",
        labelKey: "sales.balance.columns.balance",
        enableSorting: false,
        gridSecondary: true,
        cell: ({ row }) => (
          <span
            className={cn(
              "font-semibold tabular-nums",
              balanceTone(row.original.balance),
            )}
          >
            {money(row.original.balance, row.original.currency)}
          </span>
        ),
      }),
    ] as ColumnDef<LedgerTotals, unknown>[];
  }, [money]);
}

/**
 * Ledger balance of a customer per currency in the member's branch scope:
 * charged (sales and packages), paid (collected − refunded) and the
 * balance (positive = owed, negative = held for the customer). Shown with
 * the finance module and `tenant.finance.read`.
 */
export function CustomerBalanceCard({
  customerUuid,
}: {
  customerUuid: string;
}) {
  const canRead = useCanReadSales();
  if (!canRead) return null;
  return <CustomerBalanceContent customerUuid={customerUuid} />;
}

function CustomerBalanceContent({ customerUuid }: { customerUuid: string }) {
  const { t } = useLocale();
  const query = useCustomerBalance(customerUuid);
  const columns = useTotalsColumns();

  return (
    <EntitySectionCard title={t("sales.balance.customer_title")}>
      <p className="text-muted-foreground mb-3 text-sm">
        {t("sales.balance.customer_description")}
      </p>
      <EntityTable
        columns={columns}
        data={query.data?.totals ?? []}
        getRowId={(row) => row.currency}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("sales.balance.load_failed")}
        onRetry={() => void query.refetch()}
        emptyTitle={t("sales.balance.customer_empty_title")}
        emptyDescription={t("sales.balance.customer_empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          globalFilter: false,
          sorting: false,
          pagination: false,
          viewMode: true,
          persistKey: "tenant-customer-balance-v1",
        }}
      />
    </EntitySectionCard>
  );
}
