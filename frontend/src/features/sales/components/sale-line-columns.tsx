"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, PencilLine, Undo2 } from "lucide-react";
import { useMemo } from "react";

import { EntityRowActions } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { amountTone, useCashAmount } from "@/features/cash/lib/format";
import {
  ITEM_TYPE_LABEL_KEYS,
  LINE_KIND_LABEL_KEYS,
  SALE_ITEM_TYPES,
  SALE_LINE_KINDS,
} from "@/features/sales/constants";
import {
  LINE_STATE_LABEL_KEYS,
  isLiveSale,
  lineState,
} from "@/features/sales/lib/format";
import type { SaleLine } from "@/features/sales/types";
import { cn } from "@/lib/utils";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type Option = { value: string; label: string };

/** Kind of a line plus where it stands in its correction chain. */
export function SaleLineKindBadges({ line }: { line: SaleLine }) {
  const { t } = useLocale();
  const state = lineState(line);
  return (
    <div className="flex flex-wrap items-center gap-1">
      <Badge variant={line.kind === "sale" ? "success" : "warning"}>
        {t(LINE_KIND_LABEL_KEYS[line.kind])}
      </Badge>
      {state ? (
        <Badge variant={state === "replacement" ? "outline" : "warning"}>
          {t(LINE_STATE_LABEL_KEYS[state])}
        </Badge>
      ) : null}
    </div>
  );
}

/**
 * Columns of `GET /v1/tenant/sales/lines`. Sortable keys match the meta
 * (`sale_date`, `net_amount`, `kind`, `created_at`); kind, item type,
 * branch and staff are server-side filters. `compact` drops the filters
 * and the secondary columns (sub-list of a visit).
 */
export function useSaleLineColumns({
  onOpen,
  onReverse,
  onCorrect,
  branchOptions,
  staffOptions,
  compact = false,
}: {
  onOpen: (line: SaleLine) => void;
  onReverse?: (line: SaleLine) => void;
  onCorrect?: (line: SaleLine) => void;
  branchOptions?: Option[];
  staffOptions?: Option[];
  compact?: boolean;
}) {
  const { t, locale } = useLocale();
  const money = useCashAmount();

  return useMemo(() => {
    const filters = !compact;
    const columns = [
      createColumn<SaleLine>({
        accessorKey: "sale_date",
        labelKey: "sales.columns.sale_date",
        enableSorting: filters,
        gridSecondary: true,
        cell: ({ row }) => date(row.original.sale_date, undefined, locale),
      }),
      createColumn<SaleLine>({
        accessorKey: "description",
        labelKey: "sales.columns.description",
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <div className="min-w-0 space-y-0.5">
            <span className="line-clamp-2 text-sm font-medium">
              {row.original.description || "—"}
              {row.original.quantity > 1 ? ` × ${row.original.quantity}` : ""}
            </span>
            {row.original.reservation ? (
              <span className="text-muted-foreground block text-xs">
                {t("sales.reservation_number", {
                  number: row.original.reservation.number,
                })}
              </span>
            ) : null}
          </div>
        ),
      }),
      createColumn<SaleLine>({
        accessorKey: "item_type",
        labelKey: "sales.columns.item_type",
        enableSorting: false,
        ...(filters
          ? {
              filterVariant: "faceted" as const,
              filterOptions: SALE_ITEM_TYPES.map((value) => ({
                value,
                label: t(ITEM_TYPE_LABEL_KEYS[value]),
              })),
            }
          : {}),
        cell: ({ row }) => t(ITEM_TYPE_LABEL_KEYS[row.original.item_type]),
      }),
      ...(compact
        ? []
        : [
            createColumn<SaleLine>({
              id: "customer",
              accessorFn: (row) => row.customer?.name ?? "",
              labelKey: "sales.columns.customer",
              enableSorting: false,
              cell: ({ row }) =>
                row.original.customer?.name ?? t("sales.walk_in"),
            }),
          ]),
      createColumn<SaleLine>({
        id: "staff",
        accessorFn: (row) => row.staff?.name ?? "",
        labelKey: "sales.columns.staff",
        enableSorting: false,
        ...(filters && staffOptions?.length
          ? { filterVariant: "faceted" as const, filterOptions: staffOptions }
          : {}),
        cell: ({ row }) => row.original.staff?.name ?? "—",
      }),
      ...(compact
        ? []
        : [
            createColumn<SaleLine>({
              id: "branch",
              accessorFn: (row) => row.branch.name,
              labelKey: "sales.columns.branch",
              enableSorting: false,
              defaultHidden: true,
              ...(branchOptions?.length
                ? {
                    filterVariant: "faceted" as const,
                    filterOptions: branchOptions,
                  }
                : {}),
              cell: ({ row }) => row.original.branch.name,
            }),
          ]),
      createColumn<SaleLine>({
        accessorKey: "quantity",
        labelKey: "sales.columns.quantity",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <span className="tabular-nums">{row.original.quantity}</span>
        ),
      }),
      createColumn<SaleLine>({
        accessorKey: "unit_price",
        labelKey: "sales.columns.unit_price",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {money(row.original.unit_price, row.original.currency)}
          </span>
        ),
      }),
      createColumn<SaleLine>({
        accessorKey: "discount_amount",
        labelKey: "sales.columns.discount_amount",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {money(row.original.discount_amount, row.original.currency)}
          </span>
        ),
      }),
      createColumn<SaleLine>({
        accessorKey: "net_amount",
        labelKey: "sales.columns.net_amount",
        enableSorting: filters,
        gridSecondary: true,
        cell: ({ row }) => (
          <span
            className={cn(
              "font-medium tabular-nums",
              row.original.kind === "reversal" &&
                amountTone(row.original.net_amount),
            )}
          >
            {money(row.original.net_amount, row.original.currency)}
          </span>
        ),
      }),
      createColumn<SaleLine>({
        accessorKey: "kind",
        labelKey: "sales.columns.kind",
        enableSorting: filters,
        ...(filters
          ? {
              filterVariant: "faceted" as const,
              filterOptions: SALE_LINE_KINDS.map((value) => ({
                value,
                label: t(LINE_KIND_LABEL_KEYS[value]),
              })),
            }
          : {}),
        cell: ({ row }) => <SaleLineKindBadges line={row.original} />,
      }),
      createColumn<SaleLine>({
        accessorKey: "reason",
        labelKey: "sales.columns.reason",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <span className="line-clamp-2 text-sm">
            {row.original.reason || "—"}
          </span>
        ),
      }),
      createColumn<SaleLine>({
        accessorKey: "created_at",
        labelKey: "sales.columns.created_at",
        enableSorting: filters,
        defaultHidden: true,
        cell: ({ row }) => datetime(row.original.created_at, undefined, locale),
      }),
      createColumn<SaleLine>({
        id: "actions",
        labelKey: "sales.columns.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => (
          <EntityRowActions
            actions={[
              {
                id: "open",
                label: t("sales.actions.open"),
                icon: Eye,
                onSelect: () => onOpen(row.original),
              },
              ...(isLiveSale(row.original) && onCorrect
                ? [
                    {
                      id: "correct",
                      label: t("sales.actions.correct"),
                      icon: PencilLine,
                      permission: permissions.tenant.finance.write,
                      onSelect: () => onCorrect(row.original),
                    },
                  ]
                : []),
              ...(isLiveSale(row.original) && onReverse
                ? [
                    {
                      id: "reverse",
                      label: t("sales.actions.reverse"),
                      icon: Undo2,
                      permission: permissions.tenant.finance.write,
                      variant: "destructive" as const,
                      onSelect: () => onReverse(row.original),
                    },
                  ]
                : []),
            ]}
          />
        ),
      }),
    ];
    return columns as ColumnDef<SaleLine, unknown>[];
  }, [
    branchOptions,
    compact,
    locale,
    money,
    onCorrect,
    onOpen,
    onReverse,
    staffOptions,
    t,
  ]);
}
