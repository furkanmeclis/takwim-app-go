"use client";

import type { ColumnDef } from "@tanstack/react-table";
import {
  BookCheck,
  EyeOff,
  Link2,
  RotateCcw,
  Undo2,
  Unlink2,
} from "lucide-react";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { EntityRowActions, type EntityRowAction } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import {
  CASH_STATEMENT_LINE_STATUSES,
  CASH_STATEMENT_LINE_TYPES,
} from "@/features/cash/constants";
import {
  amountTone,
  movementKindLabel,
  useCashAmount,
} from "@/features/cash/lib/format";
import type {
  CashStatementLine,
  CashStatementLineFacets,
  CashStatementLineStatus,
} from "@/features/cash/types";
import { cn } from "@/lib/utils";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

export const LINE_STATUS_TONE: Record<
  CashStatementLineStatus,
  "default" | "success" | "warning" | "danger"
> = {
  pending: "warning",
  matched: "success",
  difference: "danger",
  posted: "success",
  ignored: "default",
};

export type LineActions = {
  onMatch: (line: CashStatementLine) => void;
  onPost: (line: CashStatementLine) => void;
  onReset: (line: CashStatementLine) => void;
  onIgnore: (line: CashStatementLine) => void;
  onReverse: (line: CashStatementLine) => void;
};

/** Row actions of one line by its reconciliation state. */
export function lineActions(
  line: CashStatementLine,
  t: (key: string) => string,
  handlers: LineActions,
  locked: boolean,
): EntityRowAction[] {
  if (locked) return [];
  const write = permissions.tenant.cash.write;
  switch (line.status) {
    case "pending":
      return [
        {
          id: "match",
          label: t("cash.lines.actions.match"),
          icon: Link2,
          permission: write,
          onSelect: () => handlers.onMatch(line),
        },
        {
          id: "post",
          label: t("cash.lines.actions.post"),
          icon: BookCheck,
          permission: write,
          onSelect: () => handlers.onPost(line),
        },
        {
          id: "ignore",
          label: t("cash.lines.actions.ignore"),
          icon: EyeOff,
          permission: write,
          separatorBefore: true,
          onSelect: () => handlers.onIgnore(line),
        },
      ];
    case "matched":
    case "difference":
      return [
        {
          id: "match",
          label: t("cash.lines.actions.rematch"),
          icon: Link2,
          permission: write,
          onSelect: () => handlers.onMatch(line),
        },
        {
          id: "reset",
          label: t("cash.lines.actions.reset"),
          icon: Unlink2,
          permission: write,
          variant: "destructive",
          separatorBefore: true,
          onSelect: () => handlers.onReset(line),
        },
      ];
    case "ignored":
      return [
        {
          id: "reset",
          label: t("cash.lines.actions.reset"),
          icon: RotateCcw,
          permission: write,
          onSelect: () => handlers.onReset(line),
        },
      ];
    case "posted":
      return [
        {
          id: "reverse",
          label: t("cash.lines.actions.reverse"),
          icon: Undo2,
          permission: write,
          variant: "destructive",
          onSelect: () => handlers.onReverse(line),
        },
      ];
    default:
      return [];
  }
}

/**
 * Columns of a statement's lines (`GET /v1/tenant/cash/statements/{uuid}/lines`).
 * Each row carries its reconciliation state and the movement it is matched
 * to or was posted as.
 */
export function useStatementLineColumns({
  facets,
  locked,
  handlers,
}: {
  facets?: CashStatementLineFacets;
  /** Closed statement: no row actions. */
  locked: boolean;
  handlers: LineActions;
}) {
  const { t, locale } = useLocale();
  const money = useCashAmount();

  return useMemo(() => {
    const statusLabel = (status: string) =>
      t(`cash.line_status.${status as CashStatementLineStatus}`);
    const columns = [
      createColumn<CashStatementLine>({
        accessorKey: "line_no",
        labelKey: "cash.lines.columns.line_no",
        enableSorting: true,
        defaultHidden: true,
        cell: ({ row }) => (
          <span className="text-muted-foreground tabular-nums">
            {row.original.line_no}
          </span>
        ),
      }),
      createColumn<CashStatementLine>({
        accessorKey: "line_date",
        labelKey: "cash.lines.columns.line_date",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) => date(row.original.line_date, undefined, locale),
      }),
      createColumn<CashStatementLine>({
        accessorKey: "description",
        labelKey: "cash.lines.columns.description",
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <div className="min-w-0 space-y-0.5">
            <span className="line-clamp-2 text-sm">
              {row.original.description || "—"}
            </span>
            {row.original.reference ? (
              <code className="text-muted-foreground block text-xs">
                {row.original.reference}
              </code>
            ) : null}
          </div>
        ),
      }),
      createColumn<CashStatementLine>({
        accessorKey: "reference",
        labelKey: "cash.lines.columns.reference",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => row.original.reference || "—",
      }),
      createColumn<CashStatementLine>({
        id: "direction",
        accessorFn: (row) => row.direction,
        labelKey: "cash.lines.columns.direction",
        enableSorting: false,
        defaultHidden: true,
        filterVariant: "faceted",
        filterOptions: [
          { value: "in", label: t("cash.direction.in") },
          { value: "out", label: t("cash.direction.out") },
        ],
        cell: ({ row }) => t(`cash.direction.${row.original.direction}`),
      }),
      createColumn<CashStatementLine>({
        accessorKey: "amount",
        labelKey: "cash.lines.columns.amount",
        enableSorting: true,
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
      createColumn<CashStatementLine>({
        accessorKey: "fee",
        labelKey: "cash.lines.columns.fee",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) =>
          row.original.fee && row.original.fee !== "0.00" ? (
            <span className="tabular-nums">
              {money(row.original.fee, row.original.currency)}
            </span>
          ) : (
            "—"
          ),
      }),
      createColumn<CashStatementLine>({
        accessorKey: "line_type",
        labelKey: "cash.lines.columns.line_type",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: CASH_STATEMENT_LINE_TYPES.map((type) => ({
          value: type,
          label: t(`cash.line_types.${type}`),
        })),
        cell: ({ row }) => (
          <Badge variant="secondary">
            {t(`cash.line_types.${row.original.line_type}`)}
          </Badge>
        ),
      }),
      createColumn<CashStatementLine>({
        accessorKey: "status",
        labelKey: "cash.lines.columns.status",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: facets?.status?.length
          ? facets.status.map((facet) => ({
              value: facet.value,
              label: `${statusLabel(facet.value)} (${facet.count})`,
            }))
          : CASH_STATEMENT_LINE_STATUSES.map((status) => ({
              value: status,
              label: statusLabel(status),
            })),
        cell: ({ row }) => (
          <div className="flex flex-wrap items-center gap-1">
            <StatusChip
              label={statusLabel(row.original.status)}
              tone={LINE_STATUS_TONE[row.original.status]}
            />
            {row.original.status === "difference" ? (
              <span
                className={cn(
                  "text-xs tabular-nums",
                  amountTone(row.original.difference_amount),
                )}
              >
                {money(row.original.difference_amount, row.original.currency)}
              </span>
            ) : null}
            {row.original.match_method === "auto" ? (
              <Badge variant="outline">{t("cash.lines.auto")}</Badge>
            ) : null}
          </div>
        ),
      }),
      createColumn<CashStatementLine>({
        id: "movement",
        accessorFn: (row) =>
          row.posted_movement?.uuid ?? row.matched_movement?.uuid ?? "",
        labelKey: "cash.lines.columns.matched_movement",
        enableSorting: false,
        cell: ({ row }) => {
          const movement =
            row.original.posted_movement ?? row.original.matched_movement;
          if (!movement) return "—";
          return (
            <div className="min-w-0 space-y-0.5 text-sm">
              <span className="flex flex-wrap items-center gap-1">
                <Badge variant="secondary">
                  {movementKindLabel(t, movement.kind)}
                </Badge>
                {movement.movement_date ? (
                  <span className="text-muted-foreground text-xs">
                    {date(movement.movement_date, undefined, locale)}
                  </span>
                ) : null}
              </span>
              <span className="text-muted-foreground line-clamp-1 text-xs">
                {movement.amount
                  ? money(movement.amount, row.original.currency)
                  : null}
                {movement.description ? ` · ${movement.description}` : ""}
              </span>
            </div>
          );
        },
      }),
      createColumn<CashStatementLine>({
        accessorKey: "balance_after",
        labelKey: "cash.lines.columns.balance_after",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) =>
          row.original.balance_after ? (
            <span className="tabular-nums">
              {money(row.original.balance_after, row.original.currency)}
            </span>
          ) : (
            "—"
          ),
      }),
      createColumn<CashStatementLine>({
        accessorKey: "note",
        labelKey: "cash.lines.columns.note",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <span className="text-muted-foreground line-clamp-2 text-sm">
            {row.original.note || "—"}
          </span>
        ),
      }),
      createColumn<CashStatementLine>({
        id: "resolved_by",
        accessorFn: (row) =>
          row.resolved_by
            ? `${row.resolved_by.name} ${row.resolved_by.surname}`.trim()
            : "",
        labelKey: "cash.lines.columns.resolved_by",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) =>
          row.original.resolved_by ? (
            <div className="space-y-0.5 text-sm">
              <span>
                {`${row.original.resolved_by.name} ${row.original.resolved_by.surname}`.trim()}
              </span>
              {row.original.resolved_at ? (
                <span className="text-muted-foreground block text-xs">
                  {datetime(row.original.resolved_at, undefined, locale)}
                </span>
              ) : null}
            </div>
          ) : (
            "—"
          ),
      }),
      createColumn<CashStatementLine>({
        id: "actions",
        labelKey: "cash.columns.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => {
          const actions = lineActions(row.original, t, handlers, locked);
          return actions.length ? <EntityRowActions actions={actions} /> : null;
        },
      }),
    ];
    return columns as ColumnDef<CashStatementLine, unknown>[];
  }, [facets, handlers, locale, locked, money, t]);
}
