"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Undo2 } from "lucide-react";
import { useMemo } from "react";

import { EntityRowActions } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { amountTone, useCashAmount } from "@/features/cash/lib/format";
import {
  PAYOUT_KIND_LABEL_KEYS,
  STAFF_PAYOUT_KINDS,
} from "@/features/staff-shares/constants";
import type { StaffSharePayout } from "@/features/staff-shares/types";
import { cn } from "@/lib/utils";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** A payout the panel can reverse: a payout that is not reversed yet. */
export function canReversePayout(payout: StaffSharePayout) {
  return payout.kind === "payout" && !payout.reversed_by_uuid;
}

/**
 * Columns of the payouts (`GET /v1/tenant/staff-shares/payouts`). Payouts
 * are never edited; the only row action is the reversal, which also
 * reverses the cash movement.
 */
export function useStaffSharePayoutColumns({
  showStaff,
  filters,
  onReverse,
}: {
  showStaff: boolean;
  filters: boolean;
  /** Left out on the member's own view (no row actions). */
  onReverse?: (payout: StaffSharePayout) => void;
}) {
  const { t, locale } = useLocale();
  const money = useCashAmount();

  return useMemo(() => {
    const columns = [
      createColumn<StaffSharePayout>({
        accessorKey: "payout_date",
        labelKey: "staff_shares.payouts.columns.payout_date",
        enableSorting: filters,
        gridSecondary: true,
        cell: ({ row }) => date(row.original.payout_date, undefined, locale),
      }),
      ...(showStaff
        ? [
            createColumn<StaffSharePayout>({
              id: "staff",
              accessorFn: (row) => row.staff.name,
              labelKey: "staff_shares.payouts.columns.staff",
              enableSorting: false,
              cell: ({ row }) => (
                <span className="font-medium">{row.original.staff.name}</span>
              ),
            }),
          ]
        : []),
      createColumn<StaffSharePayout>({
        accessorKey: "amount",
        labelKey: "staff_shares.payouts.columns.amount",
        enableSorting: filters,
        gridPrimary: true,
        cell: ({ row }) => {
          const signed =
            row.original.kind === "reversal"
              ? `-${row.original.amount}`
              : row.original.amount;
          return (
            <span
              className={cn("font-medium tabular-nums", amountTone(signed))}
            >
              {money(signed, row.original.currency)}
            </span>
          );
        },
      }),
      createColumn<StaffSharePayout>({
        id: "cash_account",
        accessorFn: (row) => row.cash_account.name,
        labelKey: "staff_shares.payouts.columns.cash_account",
        enableSorting: false,
        cell: ({ row }) => row.original.cash_account.name,
      }),
      createColumn<StaffSharePayout>({
        accessorKey: "kind",
        labelKey: "staff_shares.payouts.columns.kind",
        enableSorting: false,
        filterVariant: filters ? "faceted" : undefined,
        filterOptions: STAFF_PAYOUT_KINDS.map((value) => ({
          value,
          label: t(PAYOUT_KIND_LABEL_KEYS[value]),
        })),
        cell: ({ row }) => (
          <div className="flex flex-wrap items-center gap-1">
            <Badge
              variant={
                row.original.kind === "reversal" ? "warning" : "secondary"
              }
            >
              {t(PAYOUT_KIND_LABEL_KEYS[row.original.kind])}
            </Badge>
            {row.original.reversed_by_uuid ? (
              <Badge variant="warning">
                {t("staff_shares.payouts.reversed")}
              </Badge>
            ) : null}
          </div>
        ),
      }),
      createColumn<StaffSharePayout>({
        accessorKey: "note",
        labelKey: "staff_shares.payouts.columns.note",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="text-muted-foreground line-clamp-2 text-sm">
            {row.original.note || "—"}
          </span>
        ),
      }),
      createColumn<StaffSharePayout>({
        id: "branch",
        accessorFn: (row) => row.branch?.name ?? "",
        labelKey: "staff_shares.payouts.columns.branch",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) =>
          row.original.branch?.name ?? t("staff_shares.head_office"),
      }),
      createColumn<StaffSharePayout>({
        accessorKey: "created_at",
        labelKey: "staff_shares.payouts.columns.created_at",
        enableSorting: filters,
        defaultHidden: true,
        cell: ({ row }) => datetime(row.original.created_at, undefined, locale),
      }),
      createColumn<StaffSharePayout>({
        accessorKey: "uuid",
        labelKey: "staff_shares.payouts.columns.uuid",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <code className="text-muted-foreground text-xs">
            {row.original.uuid}
          </code>
        ),
      }),
      ...(onReverse
        ? [
            createColumn<StaffSharePayout>({
              id: "actions",
              labelKey: "staff_shares.columns.actions",
              enableSorting: false,
              enableHiding: false,
              enableResizing: false,
              cell: ({ row }) =>
                canReversePayout(row.original) ? (
                  <EntityRowActions
                    actions={[
                      {
                        id: "reverse",
                        label: t("staff_shares.actions.reverse_payout"),
                        icon: Undo2,
                        permission: permissions.tenant.earnings.pay,
                        variant: "destructive",
                        onSelect: () => onReverse(row.original),
                      },
                    ]}
                  />
                ) : null,
            }),
          ]
        : []),
    ];
    return columns as ColumnDef<StaffSharePayout, unknown>[];
  }, [filters, locale, money, onReverse, showStaff, t]);
}
