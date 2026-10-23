"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import {
  TenantRoleRowActions,
  type TenantRoleRowActionHandlers,
} from "@/features/tenant-roles/components/tenant-role-row-actions";
import type { TenantRole } from "@/features/tenant-roles/types";
import { tenantRoleLabel } from "@/features/tenant/lib/role-label";
import { useLocale } from "@/providers/locale-provider";

/** Columns follow `GET /v1/tenant/roles/meta` (`is_template` is faceted). */
export function useTenantRolesColumns(handlers: TenantRoleRowActionHandlers) {
  const { t } = useLocale();

  return useMemo(
    () =>
      [
        createColumn<TenantRole>({
          id: "name",
          accessorFn: (row) => row.name,
          labelKey: "tenant_roles.columns.name",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="min-w-0">
              <p className="font-medium">{tenantRoleLabel(t, row.original)}</p>
              {row.original.description ? (
                <p className="text-muted-foreground line-clamp-1 text-xs">
                  {row.original.description}
                </p>
              ) : null}
            </div>
          ),
        }),
        createColumn<TenantRole>({
          accessorKey: "key",
          labelKey: "tenant_roles.columns.key",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <code className="bg-muted rounded px-1.5 py-0.5 text-xs">
              {row.original.key}
            </code>
          ),
        }),
        createColumn<TenantRole>({
          id: "is_template",
          accessorFn: (row) => String(row.is_template),
          labelKey: "tenant_roles.columns.is_template",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: [
            {
              value: "true",
              labelKey: "tenant_roles.template",
              label: t("tenant_roles.template"),
            },
            {
              value: "false",
              labelKey: "tenant_roles.custom",
              label: t("tenant_roles.custom"),
            },
          ],
          cell: ({ row }) => (
            <StatusChip
              label={
                row.original.is_template
                  ? t("tenant_roles.template")
                  : t("tenant_roles.custom")
              }
              tone={row.original.is_template ? "default" : "success"}
            />
          ),
        }),
        createColumn<TenantRole>({
          id: "permissions",
          accessorFn: (row) => row.permissions.length,
          labelKey: "tenant_roles.columns.permissions",
          enableSorting: false,
          cell: ({ row }) => (
            <Badge variant="secondary" className="tabular-nums">
              {t("tenant_roles.permission_count", {
                count: row.original.permissions.length,
              })}
            </Badge>
          ),
        }),
        createColumn<TenantRole>({
          accessorKey: "member_count",
          labelKey: "tenant_roles.columns.member_count",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.member_count}</span>
          ),
        }),
        createColumn<TenantRole>({
          id: "actions",
          labelKey: "tenant_roles.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <TenantRoleRowActions role={row.original} handlers={handlers} />
          ),
        }),
      ] as ColumnDef<TenantRole, unknown>[],
    [handlers, t],
  );
}
