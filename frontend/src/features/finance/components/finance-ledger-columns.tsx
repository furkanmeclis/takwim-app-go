"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, Paperclip, Undo2 } from "lucide-react";
import { useMemo } from "react";

import { EntityRowActions } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { amountTone, useCashAmount } from "@/features/cash/lib/format";
import { FINANCE_KINDS, FINANCE_SOURCES } from "@/features/finance/constants";
import {
  branchValueLabel,
  categoryValueLabel,
  kindLabel,
  rowCategoryLabel,
  signedAmount,
  sourceLabel,
} from "@/features/finance/lib/format";
import type { FinanceFacets, FinanceLedgerRow } from "@/features/finance/types";
import { cn } from "@/lib/utils";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** A live record of the business (not a sale line, not reversed). */
export function canReverseRow(row: FinanceLedgerRow) {
  return row.source === "manual" && row.entry_type === "entry" && !row.reversed;
}

/**
 * Columns of `GET /v1/tenant/finance/entries`: the business's records and
 * the sale lines (automatic income). Sortable keys match the meta;
 * source, kind, category, branch and staff are server-side facets.
 */
export function useFinanceLedgerColumns({
  facets,
  onOpen,
  onReverse,
}: {
  facets?: FinanceFacets;
  onOpen: (row: FinanceLedgerRow) => void;
  onReverse: (row: FinanceLedgerRow) => void;
}) {
  const { t, locale } = useLocale();
  const money = useCashAmount();

  return useMemo(() => {
    const columns = [
      createColumn<FinanceLedgerRow>({
        accessorKey: "entry_date",
        labelKey: "finance.columns.entry_date",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) => date(row.original.entry_date, undefined, locale),
      }),
      createColumn<FinanceLedgerRow>({
        accessorKey: "source",
        labelKey: "finance.columns.source",
        enableSorting: false,
        defaultHidden: true,
        filterVariant: "faceted",
        filterOptions: FINANCE_SOURCES.map((source) => ({
          value: source,
          label: sourceLabel(t, source),
        })),
        cell: ({ row }) => sourceLabel(t, row.original.source),
      }),
      createColumn<FinanceLedgerRow>({
        accessorKey: "kind",
        labelKey: "finance.columns.kind",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: FINANCE_KINDS.map((kind) => ({
          value: kind,
          label: kindLabel(t, kind),
        })),
        cell: ({ row }) => (
          <div className="flex flex-wrap items-center gap-1">
            <Badge
              variant={row.original.kind === "income" ? "success" : "danger"}
            >
              {kindLabel(t, row.original.kind)}
            </Badge>
            {row.original.source === "sale" ? (
              <Badge variant="outline">{t("finance.sources.sale")}</Badge>
            ) : null}
            {row.original.entry_type === "reversal" ? (
              <Badge variant="warning">
                {t("finance.entry_types.reversal")}
              </Badge>
            ) : null}
            {row.original.reversed ? (
              <Badge variant="warning">{t("finance.reversed")}</Badge>
            ) : null}
          </div>
        ),
      }),
      createColumn<FinanceLedgerRow>({
        id: "category",
        accessorFn: (row) => rowCategoryLabel(t, row),
        labelKey: "finance.columns.category",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: (facets?.category ?? []).map((facet) => ({
          value: facet.value,
          label: `${categoryValueLabel(t, facet.value, facet.label)} (${facet.count})`,
        })),
        cell: ({ row }) => rowCategoryLabel(t, row.original),
      }),
      createColumn<FinanceLedgerRow>({
        accessorKey: "amount",
        labelKey: "finance.columns.amount",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => {
          const signed = signedAmount(row.original.kind, row.original.amount);
          return (
            <span
              className={cn("font-medium tabular-nums", amountTone(signed))}
            >
              {money(signed, row.original.currency)}
            </span>
          );
        },
      }),
      createColumn<FinanceLedgerRow>({
        id: "branch",
        accessorFn: (row) => row.branch?.name ?? "",
        labelKey: "finance.columns.branch",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: (facets?.branch ?? []).map((facet) => ({
          value: facet.value,
          label: `${branchValueLabel(t, facet.value, facet.label)} (${facet.count})`,
        })),
        cell: ({ row }) =>
          row.original.branch?.name ?? t("finance.head_office"),
      }),
      createColumn<FinanceLedgerRow>({
        id: "staff",
        accessorFn: (row) => row.staff?.name ?? "",
        labelKey: "finance.columns.staff",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: (facets?.staff ?? []).map((facet) => ({
          value: facet.value,
          label: `${facet.label ?? facet.value} (${facet.count})`,
        })),
        cell: ({ row }) => row.original.staff?.name ?? "—",
      }),
      createColumn<FinanceLedgerRow>({
        id: "account",
        accessorFn: (row) => row.account?.name ?? "",
        labelKey: "finance.columns.account",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => row.original.account?.name ?? "—",
      }),
      createColumn<FinanceLedgerRow>({
        accessorKey: "description",
        labelKey: "finance.columns.description",
        enableSorting: false,
        cell: ({ row }) => (
          <div className="flex min-w-0 items-center gap-1.5">
            {row.original.has_document ? (
              <Paperclip
                className="text-muted-foreground size-3.5 shrink-0"
                aria-label={t("finance.document.attached")}
              />
            ) : null}
            <span className="line-clamp-2 text-sm">
              {row.original.description || "—"}
            </span>
          </div>
        ),
      }),
      createColumn<FinanceLedgerRow>({
        accessorKey: "created_at",
        labelKey: "finance.columns.created_at",
        enableSorting: true,
        defaultHidden: true,
        cell: ({ row }) => datetime(row.original.created_at, undefined, locale),
      }),
      createColumn<FinanceLedgerRow>({
        accessorKey: "uuid",
        labelKey: "finance.columns.uuid",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <code className="text-muted-foreground text-xs">
            {row.original.uuid}
          </code>
        ),
      }),
      createColumn<FinanceLedgerRow>({
        id: "actions",
        labelKey: "finance.columns.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) =>
          row.original.source === "manual" ? (
            <EntityRowActions
              actions={[
                {
                  id: "open",
                  label: t("finance.actions.open"),
                  icon: Eye,
                  onSelect: () => onOpen(row.original),
                },
                ...(canReverseRow(row.original)
                  ? [
                      {
                        id: "reverse",
                        label: t("finance.actions.reverse"),
                        icon: Undo2,
                        permission: permissions.tenant.finance.write,
                        variant: "destructive" as const,
                        onSelect: () => onReverse(row.original),
                      },
                    ]
                  : []),
              ]}
            />
          ) : null,
      }),
    ];
    return columns as ColumnDef<FinanceLedgerRow, unknown>[];
  }, [facets, locale, money, onOpen, onReverse, t]);
}
