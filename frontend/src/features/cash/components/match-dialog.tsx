"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Check } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  useCashLineCandidates,
  useMatchCashLine,
} from "@/features/cash/hooks/use-cash";
import { movementKindLabel, useCashAmount } from "@/features/cash/lib/format";
import type {
  CashMatchCandidate,
  CashStatementLine,
} from "@/features/cash/types";
import { date, fromMinor, toMinor } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function useDebounced(value: string, ms: number) {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), ms);
    return () => window.clearTimeout(timer);
  }, [ms, value]);
  return debounced;
}

/**
 * Manual match of one statement line with a book movement: the API lists the
 * movements of the same account and direction around the line's date,
 * closest amount first. A different amount matches as a difference, which
 * stays visible until the book is corrected.
 */
export function MatchDialog({
  line,
  open,
  onOpenChange,
}: {
  line: CashStatementLine | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const money = useCashAmount();
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<CashMatchCandidate | null>(null);
  const [note, setNote] = useState("");
  const q = useDebounced(search, 300);
  const query = useCashLineCandidates(open ? (line?.uuid ?? null) : null, q);
  const match = useMatchCashLine();

  // The picker state belongs to one line: clear it whenever the dialog closes.
  const close = () => {
    setSearch("");
    setSelected(null);
    setNote("");
    onOpenChange(false);
  };

  const lineMinor = line ? (toMinor(line.amount) ?? 0) : 0;
  const difference = selected
    ? fromMinor(lineMinor - (toMinor(selected.amount) ?? 0))
    : null;

  const columns = useMemo(
    () =>
      [
        createColumn<CashMatchCandidate>({
          accessorKey: "movement_date",
          labelKey: "cash.lines.candidates.movement_date",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <div className="space-y-0.5 text-sm">
              <span>{date(row.original.movement_date, undefined, locale)}</span>
              {row.original.day_delta ? (
                <span className="text-muted-foreground block text-xs">
                  {t("cash.lines.candidates.day_delta", {
                    days: Math.abs(row.original.day_delta),
                  })}
                </span>
              ) : null}
            </div>
          ),
        }),
        createColumn<CashMatchCandidate>({
          accessorKey: "amount",
          labelKey: "cash.lines.candidates.amount",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="space-y-0.5">
              <span className="font-medium tabular-nums">
                {money(row.original.amount, row.original.currency)}
              </span>
              {row.original.exact ? (
                <Badge variant="success">
                  {t("cash.lines.candidates.exact")}
                </Badge>
              ) : (
                <span className="text-muted-foreground block text-xs tabular-nums">
                  {t("cash.lines.candidates.amount_delta", {
                    amount: money(
                      row.original.amount_delta,
                      row.original.currency,
                    ),
                  })}
                </span>
              )}
            </div>
          ),
        }),
        createColumn<CashMatchCandidate>({
          accessorKey: "kind",
          labelKey: "cash.lines.candidates.kind",
          enableSorting: false,
          cell: ({ row }) => (
            <Badge variant="secondary">
              {movementKindLabel(t, row.original.kind)}
            </Badge>
          ),
        }),
        createColumn<CashMatchCandidate>({
          accessorKey: "description",
          labelKey: "cash.lines.candidates.description",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="min-w-0 space-y-0.5 text-sm">
              <span className="line-clamp-2">
                {row.original.description || "—"}
              </span>
              {row.original.counter_account ? (
                <span className="text-muted-foreground block text-xs">
                  {row.original.counter_account}
                </span>
              ) : null}
            </div>
          ),
        }),
        createColumn<CashMatchCandidate>({
          id: "pick",
          labelKey: "cash.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const active = selected?.uuid === row.original.uuid;
            return (
              <Button
                type="button"
                size="sm"
                variant={active ? "default" : "outline"}
                onClick={() => setSelected(active ? null : row.original)}
              >
                {active ? <Check className="size-4" /> : null}
                {active
                  ? t("cash.lines.candidates.selected")
                  : t("cash.lines.candidates.select")}
              </Button>
            );
          },
        }),
      ] as ColumnDef<CashMatchCandidate, unknown>[],
    [locale, money, selected?.uuid, t],
  );

  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) close();
      }}
      title={t("cash.match_dialog.title")}
      description={
        line
          ? t("cash.match_dialog.description", {
              date: date(line.line_date, undefined, locale),
              amount: money(
                line.direction === "out" ? `-${line.amount}` : line.amount,
                line.currency,
              ),
              description: line.description || line.reference || "—",
            })
          : undefined
      }
      className="sm:max-w-3xl"
      footer={
        <>
          <Button type="button" variant="outline" onClick={close}>
            {t("form.cancel")}
          </Button>
          <Button
            type="button"
            disabled={!selected || !line || match.isPending}
            onClick={async () => {
              if (!selected || !line) return;
              await match.mutateAsync({
                lineUuid: line.uuid,
                body: { movement_uuid: selected.uuid, note: note.trim() },
              });
              close();
            }}
          >
            {difference && toMinor(difference)
              ? t("cash.match_dialog.submit_difference")
              : t("cash.match_dialog.submit")}
          </Button>
        </>
      }
    >
      {open && line ? (
        <div className="space-y-4">
          <div className="grid gap-2">
            <Label htmlFor="cash-match-search">
              {t("cash.match_dialog.search")}
            </Label>
            <Input
              id="cash-match-search"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder={t("cash.match_dialog.search_placeholder")}
            />
          </div>
          <EntityTable
            columns={columns}
            data={query.data?.items ?? []}
            getRowId={(row) => row.uuid}
            isLoading={query.isLoading}
            isError={query.isError}
            onRetry={() => void query.refetch()}
            emptyTitle={t("cash.match_dialog.empty_title")}
            emptyDescription={t("cash.match_dialog.empty_description")}
            onRowClick={(row) =>
              setSelected((current) =>
                current?.uuid === row.uuid ? null : row,
              )
            }
            features={{
              rowSelection: false,
              columnFilters: false,
              facetedFilters: false,
              globalFilter: false,
              sorting: false,
              pagination: false,
              columnVisibility: false,
              density: false,
              viewMode: false,
              persistKey: "tenant-cash-match-candidates-v1",
            }}
          />
          {selected && difference && toMinor(difference) ? (
            <Alert variant="destructive">
              <AlertDescription>
                {t("cash.match_dialog.difference_warning", {
                  amount: money(difference, line.currency),
                })}
              </AlertDescription>
            </Alert>
          ) : null}
          <div className="grid gap-2">
            <Label htmlFor="cash-match-note">{t("cash.fields.note")}</Label>
            <Textarea
              id="cash-match-note"
              value={note}
              onChange={(event) => setNote(event.target.value)}
              placeholder={t("cash.match_dialog.note_placeholder")}
              rows={2}
            />
          </div>
        </div>
      ) : null}
    </AppDialog>
  );
}
