"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { useCashAmount } from "@/features/cash/lib/format";
import {
  DIRECTION_LABEL_KEYS,
  ENTRY_KIND_LABEL_KEYS,
  LEDGER_ENTRY_KINDS,
  METHOD_LABEL_KEYS,
} from "@/features/sales/constants";
import { signedEntryAmount } from "@/features/sales/lib/format";
import type { LedgerEntry } from "@/features/sales/types";
import { cn } from "@/lib/utils";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type Option = { value: string; label: string };

/** Money the customer paid (a credit) reads as positive. */
const CREDIT_TONE = "text-emerald-700 dark:text-emerald-300";

/** Kind of an entry; a reversal names the kind it undoes. */
export function LedgerEntryKindBadges({ entry }: { entry: LedgerEntry }) {
  const { t } = useLocale();
  const tone =
    entry.kind === "collection"
      ? "success"
      : entry.kind === "reversal"
        ? "warning"
        : entry.kind === "refund"
          ? "danger"
          : "secondary";
  return (
    <div className="flex flex-wrap items-center gap-1">
      <Badge variant={tone}>{t(ENTRY_KIND_LABEL_KEYS[entry.kind])}</Badge>
      {entry.kind === "reversal" ? (
        <Badge variant="outline">
          {t(ENTRY_KIND_LABEL_KEYS[entry.effective_kind])}
        </Badge>
      ) : null}
      {entry.reversed_by_uuid ? (
        <Badge variant="warning">{t("sales.line_states.reversed")}</Badge>
      ) : null}
    </div>
  );
}

/**
 * Columns of `GET /v1/tenant/sales/entries`. The amount is signed the way
 * it moves the customer's balance (debit +, credit −).
 */
export function useLedgerEntryColumns({
  branchOptions,
}: {
  branchOptions?: Option[];
}) {
  const { t, locale } = useLocale();
  const money = useCashAmount();

  return useMemo(() => {
    const columns = [
      createColumn<LedgerEntry>({
        accessorKey: "entry_date",
        labelKey: "sales.entries.columns.entry_date",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) => date(row.original.entry_date, undefined, locale),
      }),
      createColumn<LedgerEntry>({
        accessorKey: "kind",
        labelKey: "sales.entries.columns.kind",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: LEDGER_ENTRY_KINDS.map((value) => ({
          value,
          label: t(ENTRY_KIND_LABEL_KEYS[value]),
        })),
        cell: ({ row }) => <LedgerEntryKindBadges entry={row.original} />,
      }),
      createColumn<LedgerEntry>({
        id: "customer",
        accessorFn: (row) => row.customer?.name ?? "",
        labelKey: "sales.entries.columns.customer",
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <div className="min-w-0 space-y-0.5">
            <span className="font-medium">
              {row.original.customer?.name ?? t("sales.walk_in")}
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
      createColumn<LedgerEntry>({
        accessorKey: "direction",
        labelKey: "sales.entries.columns.direction",
        enableSorting: false,
        cell: ({ row }) => t(DIRECTION_LABEL_KEYS[row.original.direction]),
      }),
      createColumn<LedgerEntry>({
        accessorKey: "amount",
        labelKey: "sales.entries.columns.amount",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) => (
          <span
            className={cn(
              "font-medium tabular-nums",
              row.original.direction === "credit" && CREDIT_TONE,
            )}
          >
            {money(signedEntryAmount(row.original), row.original.currency)}
          </span>
        ),
      }),
      createColumn<LedgerEntry>({
        accessorKey: "method",
        labelKey: "sales.entries.columns.method",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) =>
          row.original.method
            ? t(METHOD_LABEL_KEYS[row.original.method] ?? row.original.method)
            : "—",
      }),
      createColumn<LedgerEntry>({
        id: "account",
        accessorFn: (row) => row.cash_account?.name ?? "",
        labelKey: "sales.entries.columns.account",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => row.original.cash_account?.name ?? "—",
      }),
      createColumn<LedgerEntry>({
        accessorKey: "description",
        labelKey: "sales.entries.columns.description",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="line-clamp-2 text-sm">
            {row.original.description || "—"}
          </span>
        ),
      }),
      createColumn<LedgerEntry>({
        id: "branch",
        accessorFn: (row) => row.branch.name,
        labelKey: "sales.entries.columns.branch",
        enableSorting: false,
        defaultHidden: true,
        ...(branchOptions?.length
          ? { filterVariant: "faceted" as const, filterOptions: branchOptions }
          : {}),
        cell: ({ row }) => row.original.branch.name,
      }),
      createColumn<LedgerEntry>({
        accessorKey: "created_at",
        labelKey: "sales.entries.columns.created_at",
        enableSorting: true,
        defaultHidden: true,
        cell: ({ row }) => datetime(row.original.created_at, undefined, locale),
      }),
    ];
    return columns as ColumnDef<LedgerEntry, unknown>[];
  }, [branchOptions, locale, money, t]);
}
