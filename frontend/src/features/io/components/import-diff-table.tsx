"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import type {
  ImportPreviewRow,
  ImportPreviewSummary,
} from "@/features/io/types";
import { useLocale } from "@/providers/locale-provider";

/**
 * Dry-run diff of an import (adapters implementing RowPreviewer): create /
 * update counts and, per row, the fields it would change.
 */
export function ImportDiffTable({
  preview,
  fieldLabel,
}: {
  preview: ImportPreviewSummary;
  /** Label of an import field key (falls back to the key). */
  fieldLabel: (key: string) => string;
}) {
  const { t } = useLocale();
  const rows = useMemo(
    () => (preview.rows ?? []).filter((row) => Boolean(row.op)),
    [preview.rows],
  );
  const columns = useMemo(
    () =>
      [
        createColumn<ImportPreviewRow>({
          accessorKey: "index",
          labelKey: "imports.diff.columns.row",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.index}</span>
          ),
        }),
        createColumn<ImportPreviewRow>({
          id: "op",
          accessorFn: (row) => row.op ?? "",
          labelKey: "imports.diff.columns.op",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.op === "update" ? (
              <Badge variant="secondary">{t("imports.diff.update")}</Badge>
            ) : (
              <Badge variant="outline">{t("imports.diff.create")}</Badge>
            ),
        }),
        createColumn<ImportPreviewRow>({
          id: "changes",
          labelKey: "imports.diff.columns.changes",
          enableSorting: false,
          cell: ({ row }) => {
            const changes = row.original.changes ?? [];
            if (!changes.length) {
              return (
                <span className="text-muted-foreground text-xs">
                  {t("imports.diff.no_changes")}
                </span>
              );
            }
            return (
              <ul className="space-y-0.5 text-xs">
                {changes.map((change) => (
                  <li key={change.field}>
                    <span className="font-medium">
                      {fieldLabel(change.field)}:
                    </span>{" "}
                    {change.from ? (
                      <span className="text-muted-foreground line-through">
                        {change.from}
                      </span>
                    ) : null}
                    {change.from ? " → " : ""}
                    <span>{change.to || "—"}</span>
                  </li>
                ))}
              </ul>
            );
          },
        }),
      ] as ColumnDef<ImportPreviewRow, unknown>[],
    [fieldLabel, t],
  );

  if (!rows.length) return null;

  return (
    <div className="space-y-2">
      <p className="text-muted-foreground text-xs">
        {t("imports.diff.summary", {
          creates: preview.creates ?? 0,
          updates: preview.updates ?? 0,
        })}
      </p>
      <EntityTable
        columns={columns}
        data={rows}
        getRowId={(row) => String(row.index)}
        emptyTitle={t("imports.diff.empty")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{ persistKey: "import-diff-v1", rowSelection: false }}
      />
    </div>
  );
}
