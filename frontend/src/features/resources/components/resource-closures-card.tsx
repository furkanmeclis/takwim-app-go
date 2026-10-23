"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";

import { PermissionGuard } from "@/components/common/permission-guard";
import {
  EntityDeleteDialog,
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { ClosureDialog } from "@/features/resources/components/closure-dialog";
import {
  useDeleteClosure,
  useResourceClosures,
} from "@/features/resources/hooks/use-resources";
import type { Closure } from "@/features/resources/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const PAGE_SIZE = 20;

/** Periods the resource cannot be booked (maintenance, private event …). */
export function ResourceClosuresCard({
  resourceUuid,
}: {
  resourceUuid: string;
}) {
  const { t, locale } = useLocale();
  const [pagination, setPagination] = useState({
    pageIndex: 0,
    pageSize: PAGE_SIZE,
  });
  const query = useResourceClosures(resourceUuid, {
    limit: pagination.pageSize,
    offset: pagination.pageIndex * pagination.pageSize,
  });
  const remove = useDeleteClosure(resourceUuid);
  const [dialog, setDialog] = useState<{
    open: boolean;
    closure: Closure | null;
  }>({ open: false, closure: null });
  const [deleteTarget, setDeleteTarget] = useState<Closure | null>(null);
  const total = query.data?.total ?? 0;
  const now = new Date().toISOString();
  const write = permissions.tenant.resources.write;

  const columns = [
    createColumn<Closure>({
      accessorKey: "starts_at",
      labelKey: "resources.closures.fields.starts_at",
      enableSorting: false,
      gridPrimary: true,
      cell: ({ row }) => (
        <div className="flex items-center gap-2">
          <span>{datetime(row.original.starts_at, undefined, locale)}</span>
          {row.original.starts_at <= now && now < row.original.ends_at ? (
            <Badge variant="warning">{t("resources.closures.now")}</Badge>
          ) : null}
        </div>
      ),
    }),
    createColumn<Closure>({
      accessorKey: "ends_at",
      labelKey: "resources.closures.fields.ends_at",
      enableSorting: false,
      gridSecondary: true,
      cell: ({ row }) => datetime(row.original.ends_at, undefined, locale),
    }),
    createColumn<Closure>({
      accessorKey: "reason",
      labelKey: "resources.closures.fields.reason",
      enableSorting: false,
      cell: ({ row }) => (
        <span className="text-muted-foreground line-clamp-2 text-sm">
          {row.original.reason || "—"}
        </span>
      ),
    }),
    createColumn<Closure>({
      id: "actions",
      labelKey: "resources.columns.actions",
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) => (
        <EntityRowActions
          actions={[
            {
              id: "edit",
              label: t("resources.actions.edit"),
              icon: Pencil,
              permission: write,
              onSelect: () => setDialog({ open: true, closure: row.original }),
            },
            {
              id: "delete",
              label: t("resources.actions.delete"),
              icon: Trash2,
              variant: "destructive",
              permission: write,
              onSelect: () => setDeleteTarget(row.original),
            },
          ]}
        />
      ),
    }),
  ] as ColumnDef<Closure, unknown>[];

  return (
    <EntitySectionCard
      title={t("resources.detail.closures")}
      badge={query.data ? total : undefined}
      actions={
        <PermissionGuard permission={write}>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => setDialog({ open: true, closure: null })}
          >
            <Plus className="size-4" />
            {t("resources.closures.add")}
          </Button>
        </PermissionGuard>
      }
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("resources.closures.empty_title")}
        emptyDescription={t("resources.closures.empty_description")}
        pageCount={Math.max(1, Math.ceil(total / pagination.pageSize))}
        manual={{ filtering: false, sorting: false, pagination: true }}
        state={{ pagination, onPaginationChange: setPagination }}
        features={{
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          globalFilter: false,
          sorting: false,
          persistKey: "tenant-resource-closures-v1",
        }}
      />
      <ClosureDialog
        resourceUuid={resourceUuid}
        open={dialog.open}
        closure={dialog.closure}
        onOpenChange={(open) =>
          setDialog((prev) => ({ open, closure: open ? prev.closure : null }))
        }
      />
      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        softDelete={false}
        entityLabel={
          deleteTarget
            ? datetime(deleteTarget.starts_at, undefined, locale)
            : ""
        }
        isPending={remove.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return;
          remove.mutate(deleteTarget.uuid, {
            onSettled: () => setDeleteTarget(null),
          });
        }}
      />
    </EntitySectionCard>
  );
}
