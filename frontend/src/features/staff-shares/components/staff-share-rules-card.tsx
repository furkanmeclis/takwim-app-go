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
import { useNavModules } from "@/features/nav-engine/lib/modules";
import { RuleDialog } from "@/features/staff-shares/components/rule-dialog";
import { STAFF_SHARES_MODULE } from "@/features/staff-shares/constants";
import {
  useDeleteStaffShareRule,
  useStaffShareRules,
} from "@/features/staff-shares/hooks/use-staff-shares";
import {
  ruleScopeLabel,
  useShareRateLabel,
} from "@/features/staff-shares/lib/format";
import type { StaffShareRule } from "@/features/staff-shares/types";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const PAGE_SIZE = 20;

/**
 * Share rules of a staff member on their detail page: the default rate of
 * services and products and per-service rates, each from a start date.
 * Hidden when the staff shares module is off or the member cannot see
 * staff earnings.
 */
export function StaffShareRulesCard({ staffUuid }: { staffUuid: string }) {
  const { can } = usePermission();
  const hasModule = useNavModules();
  const visible =
    hasModule(STAFF_SHARES_MODULE) && can(permissions.tenant.earnings.read);
  if (!visible) return null;
  return <RulesCard staffUuid={staffUuid} />;
}

function RulesCard({ staffUuid }: { staffUuid: string }) {
  const { t, locale } = useLocale();
  const rateLabel = useShareRateLabel();
  const [pagination, setPagination] = useState({
    pageIndex: 0,
    pageSize: PAGE_SIZE,
  });
  const query = useStaffShareRules({
    staff: staffUuid,
    limit: pagination.pageSize,
    offset: pagination.pageIndex * pagination.pageSize,
  });
  const remove = useDeleteStaffShareRule();
  const [dialog, setDialog] = useState<{
    open: boolean;
    rule: StaffShareRule | null;
  }>({ open: false, rule: null });
  const [deleteTarget, setDeleteTarget] = useState<StaffShareRule | null>(null);
  const total = query.data?.total ?? 0;

  const columns = [
    createColumn<StaffShareRule>({
      id: "scope",
      accessorFn: (row) => row.service?.name ?? row.item_type,
      labelKey: "staff_shares.rules.columns.scope",
      enableSorting: false,
      gridSecondary: true,
      cell: ({ row }) => (
        <div className="flex flex-wrap items-center gap-1">
          <span className="font-medium">{ruleScopeLabel(t, row.original)}</span>
          {row.original.service ? (
            <Badge variant="secondary">
              {t("staff_shares.rules.override")}
            </Badge>
          ) : null}
        </div>
      ),
    }),
    createColumn<StaffShareRule>({
      id: "rate",
      accessorFn: (row) => row.rate ?? row.fixed_amount ?? "",
      labelKey: "staff_shares.rules.columns.rate",
      enableSorting: false,
      gridPrimary: true,
      cell: ({ row }) => (
        <span className="font-medium tabular-nums">
          {rateLabel(row.original)}
        </span>
      ),
    }),
    createColumn<StaffShareRule>({
      accessorKey: "valid_from",
      labelKey: "staff_shares.rules.columns.valid_from",
      enableSorting: false,
      cell: ({ row }) => date(row.original.valid_from, undefined, locale),
    }),
    createColumn<StaffShareRule>({
      accessorKey: "share_count",
      labelKey: "staff_shares.rules.columns.share_count",
      enableSorting: false,
      cell: ({ row }) => (
        <span className="tabular-nums">{row.original.share_count}</span>
      ),
    }),
    createColumn<StaffShareRule>({
      accessorKey: "note",
      labelKey: "staff_shares.rules.columns.note",
      enableSorting: false,
      cell: ({ row }) => (
        <span className="text-muted-foreground line-clamp-2 text-sm">
          {row.original.note || "—"}
        </span>
      ),
    }),
    createColumn<StaffShareRule>({
      id: "actions",
      labelKey: "staff_shares.columns.actions",
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) => (
        <EntityRowActions
          actions={[
            {
              id: "edit",
              label: t("staff_shares.actions.edit"),
              icon: Pencil,
              permission: permissions.tenant.earnings.manage,
              onSelect: () => setDialog({ open: true, rule: row.original }),
            },
            {
              id: "delete",
              label: t("staff_shares.actions.delete"),
              icon: Trash2,
              variant: "destructive",
              permission: permissions.tenant.earnings.manage,
              onSelect: () => setDeleteTarget(row.original),
            },
          ]}
        />
      ),
    }),
  ] as ColumnDef<StaffShareRule, unknown>[];

  return (
    <EntitySectionCard
      title={t("staff_shares.rules.title")}
      badge={query.data ? total : undefined}
      actions={
        <PermissionGuard permission={permissions.tenant.earnings.manage}>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => setDialog({ open: true, rule: null })}
          >
            <Plus className="size-4" />
            {t("staff_shares.rules.add")}
          </Button>
        </PermissionGuard>
      }
    >
      <p className="text-muted-foreground mb-4 text-sm">
        {t("staff_shares.rules.hint")}
      </p>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("staff_shares.rules.empty_title")}
        emptyDescription={t("staff_shares.rules.empty_description")}
        pageCount={Math.max(1, Math.ceil(total / pagination.pageSize))}
        manual={{ filtering: false, sorting: false, pagination: true }}
        state={{ pagination, onPaginationChange: setPagination }}
        features={{
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          globalFilter: false,
          sorting: false,
          persistKey: "tenant-staff-share-rules-v1",
        }}
      />
      <RuleDialog
        staffUuid={staffUuid}
        open={dialog.open}
        rule={dialog.rule}
        onOpenChange={(open) =>
          setDialog((prev) => ({ open, rule: open ? prev.rule : null }))
        }
      />
      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        softDelete={false}
        entityLabel={
          deleteTarget
            ? `${ruleScopeLabel(t, deleteTarget)} · ${rateLabel(deleteTarget)}`
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
