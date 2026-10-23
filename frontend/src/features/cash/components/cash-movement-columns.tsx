"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Undo2 } from "lucide-react";
import { useMemo } from "react";

import { EntityRowActions } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import {
  CASH_MOVEMENT_KINDS,
  REVERSIBLE_KINDS,
} from "@/features/cash/constants";
import {
  amountTone,
  movementKindLabel,
  useCashAmount,
} from "@/features/cash/lib/format";
import type { CashMovement, CashMovementFacets } from "@/features/cash/types";
import { cn } from "@/lib/utils";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** The panel reverses manual movements and transfers that are not reversed yet. */
export function canReverse(movement: CashMovement) {
  return REVERSIBLE_KINDS.includes(movement.kind) && !movement.reversed_by_uuid;
}

/**
 * Movement columns of the cash book (`GET /v1/tenant/cash/movements`) and of
 * an account's own book (adds the running balance). Movements are never
 * edited; the only row action is the reversal.
 */
export function useCashMovementColumns({
  facets,
  showAccount,
  showBalance,
  filters,
  onReverse,
}: {
  facets?: CashMovementFacets;
  showAccount: boolean;
  showBalance: boolean;
  /** Server-side faceted filters (cash book only). */
  filters: boolean;
  onReverse: (movement: CashMovement) => void;
}) {
  const { t, locale } = useLocale();
  const money = useCashAmount();

  return useMemo(() => {
    const columns = [
      createColumn<CashMovement>({
        accessorKey: "movement_date",
        labelKey: "cash.movements.columns.movement_date",
        enableSorting: filters,
        gridSecondary: true,
        cell: ({ row }) => date(row.original.movement_date, undefined, locale),
      }),
      ...(showAccount
        ? [
            createColumn<CashMovement>({
              id: "account",
              accessorFn: (row) => row.account.name,
              labelKey: "cash.movements.columns.account",
              enableSorting: false,
              filterVariant: filters ? "faceted" : undefined,
              filterOptions: (facets?.account ?? []).map((facet) => ({
                value: facet.value,
                label: `${facet.label ?? facet.value} (${facet.count})`,
              })),
              cell: ({ row }) => row.original.account.name,
            }),
          ]
        : []),
      createColumn<CashMovement>({
        accessorKey: "kind",
        labelKey: "cash.movements.columns.kind",
        enableSorting: filters,
        filterVariant: filters ? "faceted" : undefined,
        filterOptions: filters
          ? facets?.kind?.length
            ? facets.kind.map((facet) => ({
                value: facet.value,
                label: `${movementKindLabel(t, facet.value as CashMovement["kind"])} (${facet.count})`,
              }))
            : CASH_MOVEMENT_KINDS.map((kind) => ({
                value: kind,
                label: movementKindLabel(t, kind),
              }))
          : undefined,
        cell: ({ row }) => (
          <div className="flex flex-wrap items-center gap-1">
            <Badge variant="secondary">
              {movementKindLabel(t, row.original.kind)}
            </Badge>
            {row.original.reversed_by_uuid ? (
              <Badge variant="warning">{t("cash.movements.reversed")}</Badge>
            ) : null}
          </div>
        ),
      }),
      createColumn<CashMovement>({
        id: "direction",
        accessorFn: (row) => row.direction,
        labelKey: "cash.movements.columns.direction",
        enableSorting: false,
        defaultHidden: true,
        filterVariant: filters ? "faceted" : undefined,
        filterOptions: [
          { value: "in", label: t("cash.direction.in") },
          { value: "out", label: t("cash.direction.out") },
        ],
        cell: ({ row }) => t(`cash.direction.${row.original.direction}`),
      }),
      createColumn<CashMovement>({
        accessorKey: "amount",
        labelKey: "cash.movements.columns.amount",
        enableSorting: filters,
        gridPrimary: true,
        cell: ({ row }) => {
          const signed =
            row.original.direction === "out"
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
      ...(showBalance
        ? [
            createColumn<CashMovement>({
              id: "balance_after",
              accessorFn: (row) => row.balance_after ?? "",
              labelKey: "cash.movements.columns.balance_after",
              enableSorting: false,
              cell: ({ row }) => (
                <span className="tabular-nums">
                  {money(row.original.balance_after, row.original.currency)}
                </span>
              ),
            }),
          ]
        : []),
      createColumn<CashMovement>({
        accessorKey: "description",
        labelKey: "cash.movements.columns.description",
        enableSorting: false,
        cell: ({ row }) => {
          const counter = row.original.counter_account?.name;
          return (
            <div className="min-w-0 space-y-0.5">
              <span className="line-clamp-2 text-sm">
                {row.original.description || "—"}
              </span>
              {counter ? (
                <span className="text-muted-foreground block text-xs">
                  {t(
                    row.original.direction === "out"
                      ? "cash.movements.to_account"
                      : "cash.movements.from_account",
                    { name: counter },
                  )}
                </span>
              ) : null}
            </div>
          );
        },
      }),
      createColumn<CashMovement>({
        id: "created_by",
        accessorFn: (row) =>
          row.created_by
            ? `${row.created_by.name} ${row.created_by.surname}`.trim()
            : "",
        labelKey: "cash.movements.columns.created_by",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) =>
          row.original.created_by
            ? `${row.original.created_by.name} ${row.original.created_by.surname}`.trim()
            : t("cash.movements.system"),
      }),
      createColumn<CashMovement>({
        accessorKey: "created_at",
        labelKey: "cash.movements.columns.created_at",
        enableSorting: filters,
        defaultHidden: true,
        cell: ({ row }) => datetime(row.original.created_at, undefined, locale),
      }),
      createColumn<CashMovement>({
        accessorKey: "uuid",
        labelKey: "cash.movements.columns.uuid",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <code className="text-muted-foreground text-xs">
            {row.original.uuid}
          </code>
        ),
      }),
      createColumn<CashMovement>({
        id: "actions",
        labelKey: "cash.columns.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) =>
          canReverse(row.original) ? (
            <EntityRowActions
              actions={[
                {
                  id: "reverse",
                  label: t("cash.actions.reverse"),
                  icon: Undo2,
                  permission: permissions.tenant.cash.write,
                  variant: "destructive",
                  onSelect: () => onReverse(row.original),
                },
              ]}
            />
          ) : null,
      }),
    ];
    return columns as ColumnDef<CashMovement, unknown>[];
  }, [facets, filters, locale, money, onReverse, showAccount, showBalance, t]);
}
