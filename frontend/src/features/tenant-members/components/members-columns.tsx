"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import {
  MemberRowActions,
  type MemberRowActionHandlers,
} from "@/features/tenant-members/components/member-row-actions";
import type { TenantMember } from "@/features/tenant-members/types";
import type { TenantRole } from "@/features/tenant-roles/types";
import { tenantRoleLabel } from "@/features/tenant/lib/role-label";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * Account status of the member. Credentials are never shown or managed here:
 * an organization owner cannot touch another account's password or 2FA.
 */
const USER_STATUS_KEYS: Record<string, string> = {
  active: "users.status.active",
  disabled: "users.status.disabled",
  pending: "users.status.pending",
};

type MembersColumnsOptions = {
  handlers: MemberRowActionHandlers;
  roles: TenantRole[];
  /** Permission of the row actions (default `tenant.members.write`). */
  writePermission?: string;
};

/**
 * Columns follow `GET /v1/tenant/members/meta`: `roles` and `branch_scope`
 * are server-side faceted filters; sortable keys match `sortable_fields`.
 */
export function useMembersColumns({
  handlers,
  roles,
  writePermission,
}: MembersColumnsOptions) {
  const { t, locale } = useLocale();

  return useMemo(
    () =>
      [
        createColumn<TenantMember>({
          id: "name",
          accessorFn: (row) => `${row.user.name} ${row.user.surname}`.trim(),
          labelKey: "tenant_members.columns.name",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">
              {`${row.original.user.name} ${row.original.user.surname}`.trim() ||
                "—"}
            </span>
          ),
        }),
        createColumn<TenantMember>({
          id: "email",
          accessorFn: (row) => row.user.email,
          labelKey: "tenant_members.columns.email",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="text-muted-foreground">
              {row.original.user.email}
            </span>
          ),
        }),
        createColumn<TenantMember>({
          id: "status",
          accessorFn: (row) => row.user.status,
          labelKey: "tenant_members.columns.status",
          enableSorting: false,
          cell: ({ row }) => {
            const status = row.original.user.status;
            const tone =
              status === "active"
                ? "success"
                : status === "disabled"
                  ? "danger"
                  : "warning";
            const key = USER_STATUS_KEYS[status];
            return <StatusChip label={key ? t(key) : status} tone={tone} />;
          },
        }),
        createColumn<TenantMember>({
          id: "roles",
          accessorFn: (row) => row.roles.map((role) => role.uuid).join(","),
          labelKey: "tenant_members.columns.roles",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: roles.map((role) => ({
            value: role.uuid,
            label: tenantRoleLabel(t, role),
          })),
          cell: ({ row }) => (
            <div className="flex flex-wrap gap-1">
              {row.original.roles.map((role) => (
                <Badge
                  key={role.uuid}
                  variant={role.is_template ? "secondary" : "outline"}
                >
                  {tenantRoleLabel(t, role)}
                </Badge>
              ))}
            </div>
          ),
        }),
        createColumn<TenantMember>({
          accessorKey: "branch_scope",
          labelKey: "tenant_members.columns.branch_scope",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: [
            {
              value: "all",
              labelKey: "tenant_members.branch_scope.all",
              label: t("tenant_members.branch_scope.all"),
            },
            {
              value: "selected",
              labelKey: "tenant_members.branch_scope.selected",
              label: t("tenant_members.branch_scope.selected"),
            },
          ],
          cell: ({ row }) =>
            row.original.branch_scope === "all" ? (
              <StatusChip
                label={t("tenant_members.branch_scope.all")}
                tone="success"
              />
            ) : (
              <div className="flex flex-wrap gap-1">
                {row.original.branches.length ? (
                  row.original.branches.map((branch) => (
                    <Badge
                      key={branch.uuid}
                      variant="outline"
                      title={branch.name}
                    >
                      {branch.code}
                    </Badge>
                  ))
                ) : (
                  <StatusChip
                    label={t("tenant_members.branch_scope.none")}
                    tone="warning"
                  />
                )}
              </div>
            ),
        }),
        createColumn<TenantMember>({
          accessorKey: "last_login_at",
          labelKey: "tenant_members.columns.last_login_at",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.last_login_at ? (
              datetime(row.original.last_login_at, undefined, locale)
            ) : (
              <span className="text-muted-foreground">
                {t("tenant_members.never_logged_in")}
              </span>
            ),
        }),
        createColumn<TenantMember>({
          accessorKey: "created_at",
          labelKey: "tenant_members.columns.created_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, undefined, locale),
        }),
        createColumn<TenantMember>({
          id: "actions",
          labelKey: "tenant_members.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <MemberRowActions
              member={row.original}
              handlers={handlers}
              writePermission={writePermission}
            />
          ),
        }),
      ] as ColumnDef<TenantMember, unknown>[],
    [handlers, locale, roles, t, writePermission],
  );
}
