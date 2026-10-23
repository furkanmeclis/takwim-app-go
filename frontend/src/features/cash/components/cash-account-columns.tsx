"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import {
  CashAccountRowActions,
  type CashAccountRowActionHandlers,
} from "@/features/cash/components/cash-account-row-actions";
import {
  CASH_ACCOUNT_KINDS,
  CASH_ACCOUNT_STATUSES,
  HEAD_OFFICE,
} from "@/features/cash/constants";
import {
  amountTone,
  kindLabel,
  useCashAmount,
} from "@/features/cash/lib/format";
import type { CashAccount, CashAccountFacets } from "@/features/cash/types";
import { cn } from "@/lib/utils";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * Columns follow `GET /v1/tenant/cash/accounts/meta`: sortable keys match
 * `sortable_fields`; `branch`, `kind` and `status` are server-side faceted
 * filters (`branch=head_office` = business-wide accounts).
 */
export function useCashAccountColumns({
  handlers,
  facets,
}: {
  handlers: CashAccountRowActionHandlers;
  facets?: CashAccountFacets;
}) {
  const { t, locale } = useLocale();
  const money = useCashAmount();

  return useMemo(
    () =>
      [
        createColumn<CashAccount>({
          accessorKey: "name",
          labelKey: "cash.columns.name",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.name}</span>
          ),
        }),
        createColumn<CashAccount>({
          accessorKey: "kind",
          labelKey: "cash.columns.kind",
          enableSorting: true,
          gridSecondary: true,
          filterVariant: "faceted",
          filterOptions: CASH_ACCOUNT_KINDS.map((kind) => ({
            value: kind,
            label: kindLabel(t, kind),
          })),
          cell: ({ row }) => (
            <Badge variant="secondary">{kindLabel(t, row.original.kind)}</Badge>
          ),
        }),
        createColumn<CashAccount>({
          id: "branch",
          accessorFn: (row) => row.branch?.name ?? "",
          labelKey: "cash.columns.branch",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: (facets?.branch ?? []).map((facet) => ({
            value: facet.value,
            label: `${
              facet.value === HEAD_OFFICE
                ? t("cash.head_office")
                : (facet.label ?? facet.value)
            } (${facet.count})`,
          })),
          cell: ({ row }) => row.original.branch?.name ?? t("cash.head_office"),
        }),
        createColumn<CashAccount>({
          accessorKey: "balance",
          labelKey: "cash.columns.balance",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <span
              className={cn(
                "font-medium tabular-nums",
                amountTone(row.original.balance),
              )}
            >
              {money(row.original.balance, row.original.currency)}
            </span>
          ),
        }),
        createColumn<CashAccount>({
          accessorKey: "last_closed_date",
          labelKey: "cash.columns.last_closed_date",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.last_closed_date
              ? date(row.original.last_closed_date, undefined, locale)
              : "—",
        }),
        createColumn<CashAccount>({
          accessorKey: "status",
          labelKey: "cash.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: CASH_ACCOUNT_STATUSES.map((status) => ({
            value: status,
            label: t(`cash.status.${status}`),
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(`cash.status.${row.original.status}`)}
              tone={row.original.status === "active" ? "success" : "default"}
            />
          ),
        }),
        createColumn<CashAccount>({
          accessorKey: "bank_name",
          labelKey: "cash.columns.bank_name",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.bank_name || row.original.provider || "—",
        }),
        createColumn<CashAccount>({
          accessorKey: "opening_balance",
          labelKey: "cash.columns.opening_balance",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            money(row.original.opening_balance, row.original.currency),
        }),
        createColumn<CashAccount>({
          accessorKey: "sort_order",
          labelKey: "cash.columns.sort_order",
          enableSorting: true,
          defaultHidden: true,
        }),
        createColumn<CashAccount>({
          accessorKey: "created_at",
          labelKey: "cash.columns.created_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, undefined, locale),
        }),
        createColumn<CashAccount>({
          accessorKey: "uuid",
          labelKey: "cash.columns.uuid",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <code className="text-muted-foreground text-xs">
              {row.original.uuid}
            </code>
          ),
        }),
        createColumn<CashAccount>({
          id: "actions",
          labelKey: "cash.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <CashAccountRowActions account={row.original} handlers={handlers} />
          ),
        }),
      ] as ColumnDef<CashAccount, unknown>[],
    [facets?.branch, handlers, locale, money, t],
  );
}
