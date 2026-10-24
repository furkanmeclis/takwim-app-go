"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { HandCoins } from "lucide-react";
import { useMemo } from "react";

import { EntityRowActions } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { permissions } from "@/config/permissions";
import { amountTone, useCashAmount } from "@/features/cash/lib/format";
import type {
  StaffShareBranchRow,
  StaffShareSummaryRow,
} from "@/features/staff-shares/types";
import { cn } from "@/lib/utils";
import { toMinor } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type MoneyKey =
  "sales" | "share" | "business_share" | "pending" | "payable" | "period_paid";

/**
 * Per-staff summary of a period: sales, share and the business share, the
 * pending / payable split, and the running balance over every day.
 */
export function useStaffShareSummaryColumns({
  onPay,
  showBalance = true,
}: {
  /** Left out on the member's own view. */
  onPay?: (row: StaffShareSummaryRow) => void;
  showBalance?: boolean;
}) {
  const { t } = useLocale();
  const money = useCashAmount();

  return useMemo(() => {
    const amountColumn = (
      key: MoneyKey,
      labelKey: string,
      extra: { gridPrimary?: boolean; defaultHidden?: boolean } = {},
    ) =>
      createColumn<StaffShareSummaryRow>({
        id: key,
        accessorFn: (row) => row[key],
        labelKey,
        enableSorting: false,
        ...extra,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {money(row.original[key], row.original.currency)}
          </span>
        ),
      });

    const columns = [
      createColumn<StaffShareSummaryRow>({
        id: "staff",
        accessorFn: (row) => row.staff.name,
        labelKey: "staff_shares.summary.columns.staff",
        enableSorting: false,
        gridSecondary: true,
        cell: ({ row }) => (
          <div className="space-y-0.5">
            <span className="font-medium">{row.original.staff.name}</span>
            <span className="text-muted-foreground block text-xs">
              {t("staff_shares.summary.sale_count", {
                count: row.original.sale_count,
              })}
            </span>
          </div>
        ),
      }),
      amountColumn("sales", "staff_shares.summary.columns.sales"),
      amountColumn("share", "staff_shares.summary.columns.share", {
        gridPrimary: true,
      }),
      amountColumn(
        "business_share",
        "staff_shares.summary.columns.business_share",
      ),
      amountColumn("pending", "staff_shares.summary.columns.pending", {
        defaultHidden: true,
      }),
      amountColumn("payable", "staff_shares.summary.columns.payable", {
        defaultHidden: true,
      }),
      amountColumn("period_paid", "staff_shares.summary.columns.period_paid"),
      ...(showBalance
        ? [
            createColumn<StaffShareSummaryRow>({
              id: "remaining",
              accessorFn: (row) => row.balance.remaining,
              labelKey: "staff_shares.summary.columns.remaining",
              enableSorting: false,
              cell: ({ row }) => (
                <span
                  className={cn(
                    "tabular-nums",
                    amountTone(row.original.balance.remaining),
                  )}
                >
                  {money(row.original.balance.remaining, row.original.currency)}
                </span>
              ),
            }),
            createColumn<StaffShareSummaryRow>({
              id: "payable_now",
              accessorFn: (row) => row.balance.payable_now,
              labelKey: "staff_shares.summary.columns.payable_now",
              enableSorting: false,
              cell: ({ row }) => (
                <span className="font-medium tabular-nums">
                  {money(
                    row.original.balance.payable_now,
                    row.original.currency,
                  )}
                </span>
              ),
            }),
          ]
        : []),
      ...(onPay
        ? [
            createColumn<StaffShareSummaryRow>({
              id: "actions",
              labelKey: "staff_shares.columns.actions",
              enableSorting: false,
              enableHiding: false,
              enableResizing: false,
              cell: ({ row }) =>
                (toMinor(row.original.balance.payable_now) ?? 0) > 0 ? (
                  <EntityRowActions
                    actions={[
                      {
                        id: "pay",
                        label: t("staff_shares.actions.pay"),
                        icon: HandCoins,
                        permission: permissions.tenant.earnings.pay,
                        onSelect: () => onPay(row.original),
                      },
                    ]}
                  />
                ) : null,
            }),
          ]
        : []),
    ];
    return columns as ColumnDef<StaffShareSummaryRow, unknown>[];
  }, [money, onPay, showBalance, t]);
}

/** Revenue, staff share and business share per branch. */
export function useStaffShareBranchColumns() {
  const money = useCashAmount();
  return useMemo(() => {
    const amountColumn = (
      key: "revenue" | "share" | "business_share",
      labelKey: string,
      gridPrimary = false,
    ) =>
      createColumn<StaffShareBranchRow>({
        id: key,
        accessorFn: (row) => row[key],
        labelKey,
        enableSorting: false,
        gridPrimary,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {money(row.original[key], row.original.currency)}
          </span>
        ),
      });
    return [
      createColumn<StaffShareBranchRow>({
        id: "branch",
        accessorFn: (row) => row.branch.name,
        labelKey: "staff_shares.summary.columns.branch",
        enableSorting: false,
        gridSecondary: true,
        cell: ({ row }) => (
          <span className="font-medium">{row.original.branch.name}</span>
        ),
      }),
      amountColumn("revenue", "staff_shares.summary.columns.revenue"),
      amountColumn("share", "staff_shares.summary.columns.share"),
      amountColumn(
        "business_share",
        "staff_shares.summary.columns.business_share",
        true,
      ),
    ] as ColumnDef<StaffShareBranchRow, unknown>[];
  }, [money]);
}
