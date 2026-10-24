"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { Badge } from "@/components/ui/badge";
import { createColumn } from "@/components/tables";
import { amountTone, useCashAmount } from "@/features/cash/lib/format";
import {
  mainBalance,
  mainMoney,
  percent,
} from "@/features/reports/lib/figures";
import type {
  ReportBranch,
  ReportDay,
  ReportMoney,
  ReportPackages,
} from "@/features/reports/types";
import { cn } from "@/lib/utils";
import { date as formatDate } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type MoneyKey =
  | "revenue_net"
  | "collections_net"
  | "transfers"
  | "expenses"
  | "staff_share"
  | "staff_payments"
  | "business_share"
  | "product_cost"
  | "operational_result";

function pick(m: ReportMoney | undefined, key: MoneyKey): string | undefined {
  if (!m) return undefined;
  switch (key) {
    case "revenue_net":
      return m.revenue.net;
    case "collections_net":
      return m.collections.net;
    default:
      return m[key];
  }
}

function useMoneyColumn<T extends ReportDay | ReportBranch>(
  currency: string,
  omitted: string[],
) {
  const money = useCashAmount();
  return useMemo(
    () =>
      (key: MoneyKey, options: { tone?: boolean; secondary?: boolean } = {}) =>
        createColumn<T>({
          id: key,
          accessorFn: (row) => pick(mainMoney(row, currency), key) ?? "",
          labelKey: `reports.columns.${key}`,
          enableSorting: false,
          gridSecondary: options.secondary,
          cell: ({ row }) => {
            const omittedKey = key === "collections_net" ? "collections" : key;
            if (omitted.includes(omittedKey)) {
              return <span className="text-muted-foreground">—</span>;
            }
            const value = pick(mainMoney(row.original, currency), key);
            return (
              <span
                className={cn(
                  "tabular-nums",
                  options.tone ? amountTone(value) : undefined,
                )}
              >
                {money(value, currency)}
              </span>
            );
          },
        }),
    [currency, money, omitted],
  );
}

/** One row per day: visits, confirmations and the money of the day. */
export function useReportDayColumns(currency: string, omitted: string[]) {
  const { locale } = useLocale();
  const moneyColumn = useMoneyColumn<ReportDay>(currency, omitted);
  return useMemo(
    () =>
      [
        createColumn<ReportDay>({
          accessorKey: "date",
          labelKey: "reports.columns.date",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">
              {formatDate(row.original.date, undefined, locale)}
            </span>
          ),
        }),
        createColumn<ReportDay>({
          id: "reservations",
          accessorFn: (row) => row.reservations.total,
          labelKey: "reports.columns.reservations",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {row.original.reservations.total}
            </span>
          ),
        }),
        createColumn<ReportDay>({
          id: "completed",
          accessorFn: (row) => row.reservations.completed,
          labelKey: "reports.columns.completed",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {row.original.reservations.completed}
            </span>
          ),
        }),
        moneyColumn("revenue_net", { secondary: true }),
        moneyColumn("collections_net"),
        moneyColumn("transfers"),
        moneyColumn("expenses"),
        moneyColumn("staff_share"),
        moneyColumn("staff_payments"),
        moneyColumn("operational_result", { tone: true }),
      ] as ColumnDef<ReportDay, unknown>[],
    [locale, moneyColumn],
  );
}

/** Visits and confirmation results per day. */
export function useReportVisitColumns() {
  const { locale } = useLocale();
  return useMemo(() => {
    const count = (
      id: string,
      labelKey: string,
      value: (row: ReportDay) => number,
    ) =>
      createColumn<ReportDay>({
        id,
        accessorFn: value,
        labelKey,
        enableSorting: false,
        cell: ({ row }) => (
          <span className="tabular-nums">{value(row.original)}</span>
        ),
      });
    return [
      createColumn<ReportDay>({
        accessorKey: "date",
        labelKey: "reports.columns.date",
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-medium">
            {formatDate(row.original.date, undefined, locale)}
          </span>
        ),
      }),
      createColumn<ReportDay>({
        id: "reservations",
        accessorFn: (row) => row.reservations.total,
        labelKey: "reports.columns.reservations",
        enableSorting: false,
        gridSecondary: true,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {row.original.reservations.total}
          </span>
        ),
      }),
      count(
        "completed",
        "reports.columns.completed",
        (r) => r.reservations.completed,
      ),
      count(
        "cancelled",
        "reports.columns.cancelled",
        (r) => r.reservations.cancelled,
      ),
      count(
        "no_show",
        "reports.columns.no_show",
        (r) => r.reservations.no_show,
      ),
      count(
        "online",
        "reports.columns.online",
        (r) => r.reservations.from_online + r.reservations.from_embed,
      ),
      count(
        "confirmed",
        "reports.columns.confirmed",
        (r) => r.confirmations.confirmed,
      ),
      count(
        "declined",
        "reports.columns.declined",
        (r) => r.confirmations.declined,
      ),
      count(
        "change_requested",
        "reports.columns.change_requested",
        (r) => r.confirmations.change_requested,
      ),
      count(
        "no_answer",
        "reports.columns.no_answer",
        (r) =>
          r.confirmations.cancelled +
          r.confirmations.flagged +
          r.confirmations.unreachable,
      ),
      count(
        "open",
        "reports.columns.open",
        (r) => r.confirmations.scheduled + r.confirmations.pending,
      ),
    ] as ColumnDef<ReportDay, unknown>[];
  }, [locale]);
}

function BranchName({ row }: { row: ReportBranch }) {
  const { t } = useLocale();
  if (!row.branch) {
    return (
      <div className="space-y-0.5">
        <span className="font-medium">{t("reports.head_office")}</span>
        <span className="text-muted-foreground block text-xs">
          {t("reports.head_office_hint")}
        </span>
      </div>
    );
  }
  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="font-medium">{row.branch.name}</span>
      <span className="text-muted-foreground text-xs">{row.branch.code}</span>
      {!row.branch.is_active ? (
        <Badge variant="secondary">{t("reports.inactive_branch")}</Badge>
      ) : null}
    </div>
  );
}

function branchId(row: ReportBranch) {
  return row.branch?.uuid ?? "head_office";
}

/** Branch comparison: visits, completion rate and the money of each branch. */
export function useReportBranchColumns(currency: string, omitted: string[]) {
  const moneyColumn = useMoneyColumn<ReportBranch>(currency, omitted);
  return useMemo(
    () =>
      [
        createColumn<ReportBranch>({
          id: "branch",
          accessorFn: branchId,
          labelKey: "reports.columns.branch",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => <BranchName row={row.original} />,
        }),
        createColumn<ReportBranch>({
          id: "reservations",
          accessorFn: (row) => row.reservations.total,
          labelKey: "reports.columns.reservations",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {row.original.reservations.total}
            </span>
          ),
        }),
        createColumn<ReportBranch>({
          id: "completion",
          accessorFn: (row) => row.reservations.completed,
          labelKey: "reports.columns.completion",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {percent(
                row.original.reservations.completed,
                row.original.reservations.total,
              )}
            </span>
          ),
        }),
        moneyColumn("revenue_net", { secondary: true }),
        moneyColumn("collections_net"),
        moneyColumn("transfers"),
        moneyColumn("expenses"),
        moneyColumn("product_cost"),
        moneyColumn("staff_share"),
        moneyColumn("business_share"),
        moneyColumn("operational_result", { tone: true }),
      ] as ColumnDef<ReportBranch, unknown>[],
    [moneyColumn],
  );
}

/** Stock per branch: units moved in the period and the stock now. */
export function useReportStockColumns() {
  const money = useCashAmount();
  return useMemo(() => {
    const units = (
      id: string,
      value: (row: ReportBranch) => number,
      secondary = false,
    ) =>
      createColumn<ReportBranch>({
        id,
        accessorFn: value,
        labelKey: `reports.stock.${id}`,
        enableSorting: false,
        gridSecondary: secondary,
        cell: ({ row }) => (
          <span className="tabular-nums">{value(row.original)}</span>
        ),
      });
    return [
      createColumn<ReportBranch>({
        id: "branch",
        accessorFn: branchId,
        labelKey: "reports.columns.branch",
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) => <BranchName row={row.original} />,
      }),
      units("sold", (r) => r.stock.sold, true),
      units("received", (r) => r.stock.received),
      units("returned", (r) => r.stock.returned),
      units("adjusted", (r) => r.stock.adjusted),
      units(
        "transferred",
        (r) => r.stock.transferred_in - r.stock.transferred_out,
      ),
      units("on_hand", (r) => r.stock.on_hand),
      units("reserved", (r) => r.stock.reserved),
      units("low_items", (r) => r.stock.low_items),
      units("out_items", (r) => r.stock.out_items),
      createColumn<ReportBranch>({
        id: "value",
        accessorFn: (row) => row.stock.value,
        labelKey: "reports.stock.value",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {money(row.original.stock.value, row.original.stock.currency)}
          </span>
        ),
      }),
    ] as ColumnDef<ReportBranch, unknown>[];
  }, [money]);
}

type PackageAmountKey = "sold" | "used" | "refunded" | "expired";

function usePackageColumns<T extends ReportDay | ReportBranch>(
  currency: string,
) {
  const money = useCashAmount();
  return useMemo(() => {
    const packagesOf = (row: T): ReportPackages | undefined =>
      mainMoney(row, currency)?.packages;
    const amount = (id: string, key: PackageAmountKey, secondary = false) =>
      createColumn<T>({
        id,
        accessorFn: (row) => packagesOf(row)?.[key].amount ?? "",
        labelKey: `reports.columns.${id}`,
        enableSorting: false,
        gridSecondary: secondary,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {money(packagesOf(row.original)?.[key].amount, currency)}
          </span>
        ),
      });
    const count = (id: string, value: (p: ReportPackages) => number) =>
      createColumn<T>({
        id,
        accessorFn: (row) => {
          const p = packagesOf(row);
          return p ? value(p) : 0;
        },
        labelKey: `reports.columns.${id}`,
        enableSorting: false,
        cell: ({ row }) => {
          const p = packagesOf(row.original);
          return <span className="tabular-nums">{p ? value(p) : 0}</span>;
        },
      });
    return [
      count("package_sold_count", (p) => p.sold.count),
      amount("package_sold_amount", "sold", true),
      count("package_used_sessions", (p) => p.used.sessions),
      amount("package_used_amount", "used"),
      amount("package_refunded_amount", "refunded"),
      count("package_expired_sessions", (p) => p.expired.sessions),
      amount("package_expired_amount", "expired"),
    ];
  }, [currency, money]);
}

/** Package activity per day: sales, sessions used, refunds, expiries. */
export function useReportPackageDayColumns(currency: string) {
  const { locale } = useLocale();
  const packageColumns = usePackageColumns<ReportDay>(currency);
  return useMemo(
    () =>
      [
        createColumn<ReportDay>({
          accessorKey: "date",
          labelKey: "reports.columns.date",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">
              {formatDate(row.original.date, undefined, locale)}
            </span>
          ),
        }),
        ...packageColumns,
      ] as ColumnDef<ReportDay, unknown>[],
    [locale, packageColumns],
  );
}

/** Packages per branch: the period's activity and the rights left today. */
export function useReportPackageBranchColumns(currency: string) {
  const money = useCashAmount();
  const packageColumns = usePackageColumns<ReportBranch>(currency);
  return useMemo(
    () =>
      [
        createColumn<ReportBranch>({
          id: "branch",
          accessorFn: branchId,
          labelKey: "reports.columns.branch",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => <BranchName row={row.original} />,
        }),
        ...packageColumns,
        createColumn<ReportBranch>({
          id: "package_open_sessions",
          accessorFn: (row) =>
            mainBalance(row.package_balance, currency)?.open_sessions ?? 0,
          labelKey: "reports.columns.package_open_sessions",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {mainBalance(row.original.package_balance, currency)
                ?.open_sessions ?? 0}
            </span>
          ),
        }),
        createColumn<ReportBranch>({
          id: "package_open_amount",
          accessorFn: (row) =>
            mainBalance(row.package_balance, currency)?.open_amount ?? "",
          labelKey: "reports.columns.package_open_amount",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {money(
                mainBalance(row.original.package_balance, currency)
                  ?.open_amount,
                currency,
              )}
            </span>
          ),
        }),
      ] as ColumnDef<ReportBranch, unknown>[],
    [currency, money, packageColumns],
  );
}

export { branchId };
