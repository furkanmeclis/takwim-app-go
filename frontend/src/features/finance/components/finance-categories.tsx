"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Archive, ArchiveRestore, Pencil, Plus, Trash2 } from "lucide-react";
import { useCallback, useMemo, useState } from "react";

import { PermissionGuard } from "@/components/common/permission-guard";
import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { CategoryDialog } from "@/features/finance/components/category-dialog";
import { FINANCE_KINDS } from "@/features/finance/constants";
import {
  useDeleteFinanceCategory,
  useFinanceCategories,
  useSaveFinanceCategory,
} from "@/features/finance/hooks/use-finance";
import { kindLabel } from "@/features/finance/lib/format";
import type { FinanceCategory, FinanceKind } from "@/features/finance/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const QUERY = { limit: 200, offset: 0 };

type Handlers = {
  onEdit: (category: FinanceCategory) => void;
  onToggle: (category: FinanceCategory) => void;
  onDelete: (category: FinanceCategory) => void;
};

function useCategoryColumns(handlers: Handlers, canManage: boolean) {
  const { t } = useLocale();
  return useMemo(
    () =>
      [
        createColumn<FinanceCategory>({
          accessorKey: "name",
          labelKey: "finance.categories.columns.name",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="font-medium">{row.original.name}</span>
              {row.original.is_system ? (
                <Badge variant="outline">
                  {t("finance.categories.system")}
                </Badge>
              ) : null}
            </div>
          ),
        }),
        createColumn<FinanceCategory>({
          accessorKey: "kind",
          labelKey: "finance.categories.columns.kind",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <Badge
              variant={row.original.kind === "income" ? "success" : "danger"}
            >
              {kindLabel(t, row.original.kind)}
            </Badge>
          ),
        }),
        createColumn<FinanceCategory>({
          accessorKey: "status",
          labelKey: "finance.categories.columns.status",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(`finance.category_status.${row.original.status}`)}
              tone={row.original.status === "active" ? "success" : "default"}
            />
          ),
        }),
        createColumn<FinanceCategory>({
          accessorKey: "entry_count",
          labelKey: "finance.categories.columns.entry_count",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.entry_count}</span>
          ),
        }),
        createColumn<FinanceCategory>({
          accessorKey: "sort_order",
          labelKey: "finance.categories.columns.sort_order",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.sort_order}</span>
          ),
        }),
        createColumn<FinanceCategory>({
          id: "actions",
          labelKey: "finance.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) =>
            canManage ? (
              <EntityRowActions
                actions={[
                  {
                    id: "edit",
                    label: t("finance.actions.edit"),
                    icon: Pencil,
                    permission: permissions.tenant.finance.write,
                    onSelect: () => handlers.onEdit(row.original),
                  },
                  {
                    id: "toggle",
                    label:
                      row.original.status === "active"
                        ? t("finance.actions.archive")
                        : t("finance.actions.activate"),
                    icon:
                      row.original.status === "active"
                        ? Archive
                        : ArchiveRestore,
                    permission: permissions.tenant.finance.write,
                    onSelect: () => handlers.onToggle(row.original),
                  },
                  ...(!row.original.is_system && row.original.entry_count === 0
                    ? [
                        {
                          id: "delete",
                          label: t("finance.actions.delete"),
                          icon: Trash2,
                          permission: permissions.tenant.finance.write,
                          variant: "destructive" as const,
                          onSelect: () => handlers.onDelete(row.original),
                        },
                      ]
                    : []),
                ]}
              />
            ) : null,
        }),
      ] as ColumnDef<FinanceCategory, unknown>[],
    [canManage, handlers, t],
  );
}

/**
 * Income and expense categories: the defaults of every business and its
 * own. A used or default category is archived instead of deleted, so the
 * records keep it. Changes need every branch (business-wide setting).
 */
export function FinanceCategories() {
  const { t } = useLocale();
  const { branchScope } = useTenant();
  const query = useFinanceCategories(QUERY);
  const save = useSaveFinanceCategory();
  const remove = useDeleteFinanceCategory();
  const [editing, setEditing] = useState<FinanceCategory | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [defaultKind, setDefaultKind] = useState<FinanceKind>("expense");
  const [deleting, setDeleting] = useState<FinanceCategory | null>(null);
  const canManage = branchScope === "all";

  const onEdit = useCallback((category: FinanceCategory) => {
    setEditing(category);
    setDialogOpen(true);
  }, []);
  const onToggle = useCallback(
    (category: FinanceCategory) =>
      save.mutate({
        uuid: category.uuid,
        patch: {
          status: category.status === "active" ? "archived" : "active",
        },
      }),
    [save],
  );
  const onDelete = useCallback(
    (category: FinanceCategory) => setDeleting(category),
    [],
  );
  const handlers = useMemo(
    () => ({ onEdit, onToggle, onDelete }),
    [onDelete, onEdit, onToggle],
  );
  const columns = useCategoryColumns(handlers, canManage);
  const items = query.data?.items ?? [];

  return (
    <EntitySectionCard
      title={t("finance.categories.title")}
      badge={query.data ? items.length : undefined}
      actions={
        canManage ? (
          <PermissionGuard permission={permissions.tenant.finance.write}>
            {FINANCE_KINDS.map((kind) => (
              <Button
                key={kind}
                type="button"
                size="sm"
                variant="outline"
                onClick={() => {
                  setEditing(null);
                  setDefaultKind(kind);
                  setDialogOpen(true);
                }}
              >
                <Plus className="size-4" />
                {t(`finance.categories.add_${kind}`)}
              </Button>
            ))}
          </PermissionGuard>
        ) : null
      }
    >
      <p className="text-muted-foreground mb-4 text-sm">
        {t("finance.categories.description")}
      </p>
      <EntityTable
        columns={columns}
        data={items}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("finance.categories.empty_title")}
        emptyDescription={t("finance.categories.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-finance-categories-v1",
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          globalFilter: true,
          sorting: false,
          pagination: false,
          viewMode: true,
        }}
      />
      <CategoryDialog
        open={dialogOpen}
        category={editing}
        defaultKind={defaultKind}
        onOpenChange={(open) => {
          setDialogOpen(open);
          if (!open) setEditing(null);
        }}
      />
      <ConfirmDialog
        open={Boolean(deleting)}
        title={t("finance.categories.delete_title")}
        description={t("finance.categories.delete_description", {
          name: deleting?.name ?? "",
        })}
        confirmLabel={t("finance.actions.delete")}
        variant="destructive"
        isPending={remove.isPending}
        onCancel={() => setDeleting(null)}
        onConfirm={() =>
          remove.mutate(deleting!.uuid, {
            onSettled: () => setDeleting(null),
          })
        }
      />
    </EntitySectionCard>
  );
}
