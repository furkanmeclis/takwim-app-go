"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";

import { PermissionGuard } from "@/components/common/permission-guard";
import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  EntityDeleteDialog,
  EntityRowActions,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { ServiceCategoryDialog } from "@/features/services/components/service-category-dialog";
import {
  useDeleteServiceCategory,
  useServiceCategories,
} from "@/features/services/hooks/use-services";
import type { ServiceCategory } from "@/features/services/types";
import { useLocale } from "@/providers/locale-provider";

/**
 * Category list of the catalog. Deleting a category keeps its services;
 * they become uncategorized.
 */
export function ServiceCategoriesDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const query = useServiceCategories();
  const remove = useDeleteServiceCategory();
  const write = permissions.tenant.services.write;
  const [editor, setEditor] = useState<{
    open: boolean;
    category: ServiceCategory | null;
  }>({ open: false, category: null });
  const [deleteTarget, setDeleteTarget] = useState<ServiceCategory | null>(
    null,
  );

  const columns = [
    createColumn<ServiceCategory>({
      accessorKey: "name",
      labelKey: "services.categories.fields.name",
      enableSorting: false,
      gridPrimary: true,
      cell: ({ row }) => (
        <span className="font-medium">{row.original.name}</span>
      ),
    }),
    createColumn<ServiceCategory>({
      accessorKey: "services_count",
      labelKey: "services.categories.fields.services_count",
      enableSorting: false,
      gridSecondary: true,
    }),
    createColumn<ServiceCategory>({
      accessorKey: "sort_order",
      labelKey: "services.columns.sort_order",
      enableSorting: false,
    }),
    createColumn<ServiceCategory>({
      id: "actions",
      labelKey: "services.columns.actions",
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) => (
        <EntityRowActions
          actions={[
            {
              id: "edit",
              label: t("services.actions.edit"),
              icon: Pencil,
              permission: write,
              onSelect: () => setEditor({ open: true, category: row.original }),
            },
            {
              id: "delete",
              label: t("services.actions.delete"),
              icon: Trash2,
              variant: "destructive",
              permission: write,
              onSelect: () => setDeleteTarget(row.original),
            },
          ]}
        />
      ),
    }),
  ] as ColumnDef<ServiceCategory, unknown>[];

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("services.categories.title")}
      description={t("services.categories.description")}
      className="sm:max-w-2xl"
      footer={
        <>
          <PermissionGuard permission={write}>
            <Button
              type="button"
              variant="outline"
              onClick={() => setEditor({ open: true, category: null })}
            >
              <Plus className="size-4" />
              {t("services.categories.add")}
            </Button>
          </PermissionGuard>
          <Button type="button" onClick={() => onOpenChange(false)}>
            {t("services.categories.done")}
          </Button>
        </>
      }
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("services.categories.empty_title")}
        emptyDescription={t("services.categories.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          globalFilter: false,
          sorting: false,
          pagination: false,
          persistKey: "tenant-service-categories-v1",
        }}
      />
      <ServiceCategoryDialog
        open={editor.open}
        category={editor.category}
        onOpenChange={(next) =>
          setEditor((prev) => ({
            open: next,
            category: next ? prev.category : null,
          }))
        }
      />
      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        entityLabel={deleteTarget?.name ?? ""}
        description={t("services.categories.delete_description")}
        isPending={remove.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return;
          remove.mutate(deleteTarget.uuid, {
            onSettled: () => setDeleteTarget(null),
          });
        }}
      />
    </AppDialog>
  );
}
