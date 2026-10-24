"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityDeleteDialog,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { permissions } from "@/config/permissions";
import { useCashAmount } from "@/features/cash/lib/format";
import { PackageDialog } from "@/features/session-packages/components/package-dialog";
import { SESSION_PACKAGES_RESOURCE } from "@/features/session-packages/constants";
import {
  useDeleteSessionPackage,
  useSessionPackages,
} from "@/features/session-packages/hooks/use-session-packages";
import type {
  ListSessionPackagesParams,
  SessionPackage,
} from "@/features/session-packages/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

function usePackageColumns({
  onEdit,
  onDelete,
}: {
  onEdit?: (pkg: SessionPackage) => void;
  onDelete?: (pkg: SessionPackage) => void;
}) {
  const { t, locale } = useLocale();
  const money = useCashAmount();

  return useMemo(() => {
    const columns = [
      createColumn<SessionPackage>({
        accessorKey: "name",
        labelKey: "session_packages.columns.name",
        gridPrimary: true,
        cell: ({ row }) => (
          <div className="min-w-0">
            <p className="truncate font-medium">{row.original.name}</p>
            {row.original.description ? (
              <p className="text-muted-foreground line-clamp-1 text-xs">
                {row.original.description}
              </p>
            ) : null}
          </div>
        ),
      }),
      createColumn<SessionPackage>({
        id: "service",
        accessorFn: (row) => row.service.name,
        labelKey: "session_packages.columns.service",
        enableSorting: false,
        cell: ({ row }) => row.original.service.name,
      }),
      createColumn<SessionPackage>({
        accessorKey: "total_sessions",
        labelKey: "session_packages.columns.total_sessions",
        cell: ({ row }) => (
          <span className="tabular-nums">{row.original.total_sessions}</span>
        ),
      }),
      createColumn<SessionPackage>({
        accessorKey: "price",
        labelKey: "session_packages.columns.price",
        gridSecondary: true,
        cell: ({ row }) => (
          <span className="font-medium tabular-nums">
            {money(row.original.price, row.original.currency)}
          </span>
        ),
      }),
      createColumn<SessionPackage>({
        accessorKey: "session_value",
        labelKey: "session_packages.columns.session_value",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {money(row.original.session_value, row.original.currency)}
          </span>
        ),
      }),
      createColumn<SessionPackage>({
        accessorKey: "validity_days",
        labelKey: "session_packages.columns.validity_days",
        enableSorting: false,
        cell: ({ row }) =>
          row.original.validity_days != null
            ? t("session_packages.validity_value", {
                days: row.original.validity_days,
              })
            : t("session_packages.validity_none"),
      }),
      createColumn<SessionPackage>({
        id: "branches",
        accessorFn: (row) => row.branches.map((b) => b.name).join(", "),
        labelKey: "session_packages.columns.branches",
        enableSorting: false,
        cell: ({ row }) =>
          row.original.all_branches ? (
            t("session_packages.all_branches")
          ) : (
            <span className="line-clamp-2 text-sm">
              {row.original.branches.map((b) => b.name).join(", ")}
            </span>
          ),
      }),
      createColumn<SessionPackage>({
        accessorKey: "is_active",
        labelKey: "session_packages.columns.is_active",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: [
          { value: "true", label: t("session_packages.active.yes") },
          { value: "false", label: t("session_packages.active.no") },
        ],
        cell: ({ row }) => (
          <StatusChip
            label={
              row.original.is_active
                ? t("session_packages.active.yes")
                : t("session_packages.active.no")
            }
            tone={row.original.is_active ? "success" : "default"}
          />
        ),
      }),
      createColumn<SessionPackage>({
        accessorKey: "sold_count",
        labelKey: "session_packages.columns.sold_count",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <span className="tabular-nums">{row.original.sold_count}</span>
        ),
      }),
      createColumn<SessionPackage>({
        accessorKey: "created_at",
        labelKey: "session_packages.columns.created_at",
        defaultHidden: true,
        cell: ({ row }) => datetime(row.original.created_at, undefined, locale),
      }),
      ...(onEdit && onDelete
        ? [
            createColumn<SessionPackage>({
              id: "actions",
              labelKey: "session_packages.columns.actions",
              enableSorting: false,
              enableHiding: false,
              enableResizing: false,
              cell: ({ row }) => (
                <EntityRowActions
                  actions={[
                    {
                      id: "edit",
                      label: t("session_packages.actions.edit"),
                      icon: Pencil,
                      permission: permissions.tenant.sessionPackages.write,
                      onSelect: () => onEdit(row.original),
                    },
                    {
                      id: "delete",
                      label: t("session_packages.actions.delete"),
                      icon: Trash2,
                      permission: permissions.tenant.sessionPackages.write,
                      variant: "destructive",
                      onSelect: () => onDelete(row.original),
                    },
                  ]}
                />
              ),
            }),
          ]
        : []),
    ];
    return columns as ColumnDef<SessionPackage, unknown>[];
  }, [locale, money, onDelete, onEdit, t]);
}

/**
 * The package definitions of the business (server list: search, sort,
 * active filter). Managing them needs `tenant.session_packages.write`.
 */
export function PackageDefinitionsTable({
  canWrite,
  creating,
  onCreatingChange,
}: {
  canWrite: boolean;
  creating: boolean;
  onCreatingChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { organization } = useTenant();
  const listState = useServerListState({
    initialSort: "name",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, SESSION_PACKAGES_RESOURCE),
  });
  const [editing, setEditing] = useState<SessionPackage | null>(null);
  const [deleting, setDeleting] = useState<SessionPackage | null>(null);
  const remove = useDeleteSessionPackage();

  const params = useMemo<ListSessionPackagesParams>(() => {
    const active = filterValues(
      listState.columnFilters.find((filter) => filter.id === "is_active")
        ?.value,
    );
    return {
      ...listState.params,
      is_active:
        active.length === 1 ? (active[0] as "true" | "false") : undefined,
    };
  }, [listState.columnFilters, listState.params]);

  const query = useSessionPackages(params);
  const columns = usePackageColumns(
    canWrite ? { onEdit: setEditing, onDelete: setDeleting } : {},
  );
  const total = query.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );
  const hasFilters = Boolean(params.q || params.is_active);

  return (
    <>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={canWrite ? setEditing : undefined}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("session_packages.load_failed")}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={
          hasFilters
            ? t("session_packages.empty_filtered_title")
            : t("session_packages.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("session_packages.empty_filtered_description")
            : t("session_packages.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-session-packages-v1",
          rowSelection: false,
          viewMode: true,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
      <PackageDialog
        pkg={editing}
        open={creating || Boolean(editing)}
        onOpenChange={(open) => {
          if (open) return;
          setEditing(null);
          onCreatingChange(false);
        }}
      />
      <EntityDeleteDialog
        open={Boolean(deleting)}
        entityLabel={deleting?.name ?? ""}
        description={t("session_packages.delete_description")}
        isPending={remove.isPending}
        onCancel={() => setDeleting(null)}
        onConfirm={() => {
          if (!deleting) return;
          remove.mutate(deleting.uuid, { onSettled: () => setDeleting(null) });
        }}
      />
    </>
  );
}
