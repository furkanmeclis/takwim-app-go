"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import type { BulkIssue, BulkSummary } from "@/features/bulk-engine/types";
import { useLocale } from "@/providers/locale-provider";

/** Reason codes of skipped / failed items (`summary.issues[].error`). */
const ISSUE_KEYS: Record<string, string> = {
  invalid_transition: "reservations.bulk_report.reasons.invalid_transition",
  stale: "reservations.bulk_report.reasons.stale",
  expired: "reservations.bulk_report.reasons.expired",
  not_found: "reservations.bulk_report.reasons.not_found",
};

export type BulkReport = { title: string; summary: BulkSummary };

/**
 * Result of a bulk accept / reject / cancel: how many changed, and the
 * records left as they were with the reason (skipped: the action does not
 * apply, or the record changed since it was read).
 */
export function ReservationBulkReportDialog({
  report,
  onOpenChange,
}: {
  report: BulkReport | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<BulkIssue>({
          accessorKey: "label",
          labelKey: "reservations.bulk_report.columns.number",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium tabular-nums">
              {row.original.label ?? "—"}
            </span>
          ),
        }),
        createColumn<BulkIssue>({
          accessorKey: "error",
          labelKey: "reservations.bulk_report.columns.reason",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => {
            const key = ISSUE_KEYS[row.original.error];
            return key ? t(key) : row.original.error;
          },
        }),
        createColumn<BulkIssue>({
          accessorKey: "skipped",
          labelKey: "reservations.bulk_report.columns.result",
          enableSorting: false,
          cell: ({ row }) => (
            <Badge variant={row.original.skipped ? "warning" : "danger"}>
              {row.original.skipped
                ? t("reservations.bulk_report.skipped")
                : t("reservations.bulk_report.failed")}
            </Badge>
          ),
        }),
      ] as ColumnDef<BulkIssue, unknown>[],
    [t],
  );

  const summary = report?.summary;
  return (
    <AppDialog
      open={Boolean(report)}
      onOpenChange={onOpenChange}
      title={report?.title ?? ""}
      description={
        summary
          ? t("reservations.bulk_report.description", {
              succeeded: summary.succeeded,
              skipped: summary.skipped ?? 0,
              failed: summary.failed,
            })
          : undefined
      }
      className="sm:max-w-2xl"
      footer={
        <Button type="button" onClick={() => onOpenChange(false)}>
          {t("reservations.bulk_report.close")}
        </Button>
      }
    >
      <EntityTable
        columns={columns}
        data={summary?.issues ?? []}
        getRowId={(row) => row.entity_uuid}
        emptyTitle={t("reservations.bulk_report.empty_title")}
        emptyDescription={t("reservations.bulk_report.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          globalFilter: false,
          sorting: false,
          pagination: false,
          persistKey: "tenant-reservations-bulk-report-v1",
        }}
      />
    </AppDialog>
  );
}
