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
import { TimeOffDialog } from "@/features/staff/components/time-off-dialog";
import { TIME_OFF_KIND_LABEL_KEYS } from "@/features/staff/constants";
import {
  useDeleteTimeOff,
  useStaffTimeOff,
} from "@/features/staff/hooks/use-staff";
import type { TimeOff } from "@/features/staff/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const PAGE_SIZE = 20;

/** Leave, sick days and other time off of a person (every branch). */
export function StaffTimeOffCard({ staffUuid }: { staffUuid: string }) {
  const { t, locale } = useLocale();
  const [pagination, setPagination] = useState({
    pageIndex: 0,
    pageSize: PAGE_SIZE,
  });
  const query = useStaffTimeOff(staffUuid, {
    limit: pagination.pageSize,
    offset: pagination.pageIndex * pagination.pageSize,
  });
  const remove = useDeleteTimeOff(staffUuid);
  const [dialog, setDialog] = useState<{
    open: boolean;
    timeOff: TimeOff | null;
  }>({ open: false, timeOff: null });
  const [deleteTarget, setDeleteTarget] = useState<TimeOff | null>(null);
  const total = query.data?.total ?? 0;
  const now = new Date().toISOString();

  const columns = [
    createColumn<TimeOff>({
      accessorKey: "kind",
      labelKey: "staff.time_off.fields.kind",
      enableSorting: false,
      gridPrimary: true,
      cell: ({ row }) => (
        <div className="flex items-center gap-2">
          <Badge variant="secondary">
            {t(TIME_OFF_KIND_LABEL_KEYS[row.original.kind])}
          </Badge>
          {row.original.starts_at <= now && now < row.original.ends_at ? (
            <Badge variant="warning">{t("staff.time_off.now")}</Badge>
          ) : null}
        </div>
      ),
    }),
    createColumn<TimeOff>({
      accessorKey: "starts_at",
      labelKey: "staff.time_off.fields.starts_at",
      enableSorting: false,
      gridSecondary: true,
      cell: ({ row }) => datetime(row.original.starts_at, undefined, locale),
    }),
    createColumn<TimeOff>({
      accessorKey: "ends_at",
      labelKey: "staff.time_off.fields.ends_at",
      enableSorting: false,
      cell: ({ row }) => datetime(row.original.ends_at, undefined, locale),
    }),
    createColumn<TimeOff>({
      accessorKey: "note",
      labelKey: "staff.time_off.fields.note",
      enableSorting: false,
      cell: ({ row }) => (
        <span className="text-muted-foreground line-clamp-2 text-sm">
          {row.original.note || "—"}
        </span>
      ),
    }),
    createColumn<TimeOff>({
      id: "actions",
      labelKey: "staff.columns.actions",
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) => (
        <EntityRowActions
          actions={[
            {
              id: "edit",
              label: t("staff.actions.edit"),
              icon: Pencil,
              permission: permissions.tenant.schedules.write,
              onSelect: () => setDialog({ open: true, timeOff: row.original }),
            },
            {
              id: "delete",
              label: t("staff.actions.delete"),
              icon: Trash2,
              variant: "destructive",
              permission: permissions.tenant.schedules.write,
              onSelect: () => setDeleteTarget(row.original),
            },
          ]}
        />
      ),
    }),
  ] as ColumnDef<TimeOff, unknown>[];

  return (
    <EntitySectionCard
      title={t("staff.detail.time_off")}
      badge={query.data ? total : undefined}
      actions={
        <PermissionGuard permission={permissions.tenant.schedules.write}>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => setDialog({ open: true, timeOff: null })}
          >
            <Plus className="size-4" />
            {t("staff.time_off.add")}
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
        emptyTitle={t("staff.time_off.empty_title")}
        emptyDescription={t("staff.time_off.empty_description")}
        pageCount={Math.max(1, Math.ceil(total / pagination.pageSize))}
        manual={{ filtering: false, sorting: false, pagination: true }}
        state={{ pagination, onPaginationChange: setPagination }}
        features={{
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          globalFilter: false,
          sorting: false,
          persistKey: "tenant-staff-time-off-v1",
        }}
      />
      <TimeOffDialog
        staffUuid={staffUuid}
        open={dialog.open}
        timeOff={dialog.timeOff}
        onOpenChange={(open) =>
          setDialog((prev) => ({ open, timeOff: open ? prev.timeOff : null }))
        }
      />
      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        softDelete={false}
        entityLabel={
          deleteTarget
            ? `${t(TIME_OFF_KIND_LABEL_KEYS[deleteTarget.kind])} · ${datetime(
                deleteTarget.starts_at,
                undefined,
                locale,
              )}`
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
