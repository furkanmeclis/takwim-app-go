"use client";

import type { ColumnDef, RowSelectionState } from "@tanstack/react-table";
import { GitCompare } from "lucide-react";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { VersionDiffDialog } from "@/features/tenant-settings/components/version-diff-dialog";
import { useSettingsVersions } from "@/features/tenant-settings/hooks/use-tenant-settings";
import type { OrganizationSettingsVersionSummary } from "@/features/tenant-settings/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * Version history (DataTable): every save is an immutable version. Pick two
 * rows to compare them, or compare one row with the version before it.
 */
export function SettingsVersionsTab({
  currentVersion,
}: {
  currentVersion: number;
}) {
  const { t, locale } = useLocale();
  const listState = useServerListState({
    initialSort: "-version",
    initialPageSize: 20,
  });
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({});
  const [compare, setCompare] = useState<[number, number] | null>(null);
  const listQuery = useSettingsVersions({
    limit: listState.params.limit,
    offset: listState.params.offset,
    sort: listState.params.sort,
  });

  const selected = Object.keys(rowSelection)
    .filter((key) => rowSelection[key])
    .map(Number)
    .sort((a, b) => a - b);

  const columns = useMemo(
    () =>
      [
        createSelectColumnDef<OrganizationSettingsVersionSummary>(),
        createColumn<OrganizationSettingsVersionSummary>({
          accessorKey: "version",
          labelKey: "tenant_settings.versions.columns.version",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex items-center gap-2 font-medium tabular-nums">
              v{row.original.version}
              {row.original.version === currentVersion ? (
                <Badge variant="secondary">
                  {t("tenant_settings.versions.current")}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<OrganizationSettingsVersionSummary>({
          accessorKey: "created_at",
          labelKey: "tenant_settings.versions.columns.created_at",
          enableSorting: false,
          cell: ({ row }) =>
            datetime(row.original.created_at, undefined, locale),
        }),
        createColumn<OrganizationSettingsVersionSummary>({
          id: "created_by",
          accessorFn: (row) => row.created_by?.email ?? "",
          labelKey: "tenant_settings.versions.columns.created_by",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => {
            const by = row.original.created_by;
            if (!by)
              return (
                <span className="text-muted-foreground">
                  {t("tenant_settings.versions.unknown_author")}
                </span>
              );
            const name = `${by.name} ${by.surname}`.trim();
            return (
              <span className="flex flex-col">
                <span>{name || by.email}</span>
                {name ? (
                  <span className="text-muted-foreground text-xs">
                    {by.email}
                  </span>
                ) : null}
              </span>
            );
          },
        }),
        createColumn<OrganizationSettingsVersionSummary>({
          id: "ready_for_online_sales",
          accessorFn: (row) => String(row.ready_for_online_sales),
          labelKey: "tenant_settings.versions.columns.ready",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={
                row.original.ready_for_online_sales
                  ? t("tenant_settings.readiness.ready_short")
                  : t("tenant_settings.readiness.not_ready_short")
              }
              tone={row.original.ready_for_online_sales ? "success" : "warning"}
            />
          ),
        }),
        createColumn<OrganizationSettingsVersionSummary>({
          id: "actions",
          labelKey: "tenant_settings.versions.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "compare-previous",
                  label: t("tenant_settings.versions.compare_previous"),
                  icon: GitCompare,
                  disabled: row.original.version <= 1,
                  onSelect: () =>
                    setCompare([
                      row.original.version - 1,
                      row.original.version,
                    ]),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<OrganizationSettingsVersionSummary, unknown>[],
    [currentVersion, locale, t],
  );

  const total = listQuery.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );

  return (
    <>
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => String(row.version)}
        onRowClick={(row) => {
          if (row.version > 1) setCompare([row.version - 1, row.version]);
        }}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("tenant_settings.versions.load_failed")}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={t("tenant_settings.versions.empty_title")}
        emptyDescription={t("tenant_settings.versions.empty_description")}
        pageCount={pageCount}
        state={{
          ...listState.tableState,
          rowSelection,
          onRowSelectionChange: setRowSelection,
        }}
        features={{
          persistKey: "tenant-settings-versions-v1",
          rowSelection: true,
        }}
        toolbarExtra={
          <>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={selected.length !== 2}
              onClick={() => setCompare([selected[0], selected[1]])}
            >
              <GitCompare className="size-4" />
              {t("tenant_settings.versions.compare_selected", {
                count: selected.length,
              })}
            </Button>
            <EntityToolbar
              onRefresh={() => void listQuery.refetch()}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />
      <VersionDiffDialog
        versions={compare}
        onOpenChange={(open) => {
          if (!open) setCompare(null);
        }}
      />
    </>
  );
}
