"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { amountTone, useCashAmount } from "@/features/cash/lib/format";
import {
  ITEM_TYPE_LABEL_KEYS,
  LINE_KIND_LABEL_KEYS,
  STAFF_SHARE_ITEM_TYPES,
  STAFF_SHARE_LINE_KINDS,
  STAFF_SHARE_STATUSES,
  STATUS_LABEL_KEYS,
} from "@/features/staff-shares/constants";
import { useShareRateLabel } from "@/features/staff-shares/lib/format";
import type { StaffShareLine } from "@/features/staff-shares/types";
import { cn } from "@/lib/utils";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * Columns of the calculated shares (`GET /v1/tenant/staff-shares/lines`).
 * A share keeps the rate of its sale day; a reversed sale line shows a
 * negative `reversal` row instead of a changed share.
 */
export function useStaffShareLineColumns({
  showStaff,
  filters,
}: {
  showStaff: boolean;
  /** Server-side faceted filters (status, kind, item type). */
  filters: boolean;
}) {
  const { t, locale } = useLocale();
  const money = useCashAmount();
  const rateLabel = useShareRateLabel();

  return useMemo(() => {
    const columns = [
      createColumn<StaffShareLine>({
        accessorKey: "share_date",
        labelKey: "staff_shares.columns.share_date",
        enableSorting: filters,
        gridSecondary: true,
        cell: ({ row }) => date(row.original.share_date, undefined, locale),
      }),
      ...(showStaff
        ? [
            createColumn<StaffShareLine>({
              id: "staff",
              accessorFn: (row) => row.staff.name,
              labelKey: "staff_shares.columns.staff",
              enableSorting: false,
              cell: ({ row }) => (
                <span className="font-medium">{row.original.staff.name}</span>
              ),
            }),
          ]
        : []),
      createColumn<StaffShareLine>({
        accessorKey: "description",
        labelKey: "staff_shares.columns.description",
        enableSorting: false,
        cell: ({ row }) => (
          <div className="min-w-0 space-y-0.5">
            <span className="line-clamp-2 text-sm">
              {row.original.description || "—"}
              {row.original.quantity > 1 ? ` × ${row.original.quantity}` : ""}
            </span>
            {row.original.customer ? (
              <span className="text-muted-foreground block text-xs">
                {row.original.customer.name}
              </span>
            ) : null}
          </div>
        ),
      }),
      createColumn<StaffShareLine>({
        accessorKey: "item_type",
        labelKey: "staff_shares.columns.item_type",
        enableSorting: false,
        defaultHidden: true,
        filterVariant: filters ? "faceted" : undefined,
        filterOptions: STAFF_SHARE_ITEM_TYPES.map((value) => ({
          value,
          label: t(ITEM_TYPE_LABEL_KEYS[value]),
        })),
        cell: ({ row }) => t(ITEM_TYPE_LABEL_KEYS[row.original.item_type]),
      }),
      createColumn<StaffShareLine>({
        accessorKey: "base_amount",
        labelKey: "staff_shares.columns.base_amount",
        enableSorting: filters,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {money(row.original.base_amount, row.original.currency)}
          </span>
        ),
      }),
      createColumn<StaffShareLine>({
        id: "rate",
        accessorFn: (row) => row.rate ?? row.fixed_amount ?? "",
        labelKey: "staff_shares.columns.rate",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {rateLabel(row.original, row.original.currency)}
          </span>
        ),
      }),
      createColumn<StaffShareLine>({
        accessorKey: "share_amount",
        labelKey: "staff_shares.columns.share_amount",
        enableSorting: filters,
        gridPrimary: true,
        cell: ({ row }) => (
          <span
            className={cn(
              "font-medium tabular-nums",
              amountTone(row.original.share_amount),
            )}
          >
            {money(row.original.share_amount, row.original.currency)}
          </span>
        ),
      }),
      createColumn<StaffShareLine>({
        accessorKey: "status",
        labelKey: "staff_shares.columns.status",
        enableSorting: false,
        filterVariant: filters ? "faceted" : undefined,
        filterOptions: STAFF_SHARE_STATUSES.map((value) => ({
          value,
          label: t(STATUS_LABEL_KEYS[value]),
        })),
        cell: ({ row }) => (
          <Badge
            variant={
              row.original.status === "payable" ? "success" : "secondary"
            }
          >
            {t(STATUS_LABEL_KEYS[row.original.status])}
          </Badge>
        ),
      }),
      createColumn<StaffShareLine>({
        accessorKey: "kind",
        labelKey: "staff_shares.columns.kind",
        enableSorting: false,
        filterVariant: filters ? "faceted" : undefined,
        filterOptions: STAFF_SHARE_LINE_KINDS.map((value) => ({
          value,
          label: t(LINE_KIND_LABEL_KEYS[value]),
        })),
        cell: ({ row }) => (
          <div className="flex flex-wrap items-center gap-1">
            <Badge
              variant={
                row.original.kind === "reversal" ? "warning" : "secondary"
              }
            >
              {t(LINE_KIND_LABEL_KEYS[row.original.kind])}
            </Badge>
            {row.original.reversed_by_uuid ? (
              <Badge variant="warning">
                {t("staff_shares.lines.reversed")}
              </Badge>
            ) : null}
          </div>
        ),
      }),
      createColumn<StaffShareLine>({
        id: "branch",
        accessorFn: (row) => row.branch.name,
        labelKey: "staff_shares.columns.branch",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => row.original.branch.name,
      }),
      createColumn<StaffShareLine>({
        accessorKey: "created_at",
        labelKey: "staff_shares.columns.created_at",
        enableSorting: filters,
        defaultHidden: true,
        cell: ({ row }) => datetime(row.original.created_at, undefined, locale),
      }),
      createColumn<StaffShareLine>({
        accessorKey: "uuid",
        labelKey: "staff_shares.columns.uuid",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <code className="text-muted-foreground text-xs">
            {row.original.uuid}
          </code>
        ),
      }),
    ];
    return columns as ColumnDef<StaffShareLine, unknown>[];
  }, [filters, locale, money, rateLabel, showStaff, t]);
}
