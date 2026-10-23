"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, Pause, Pencil, Play, Trash2 } from "lucide-react";
import { useMemo } from "react";

import { EntityRowActions, type EntityRowAction } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import {
  ACCESS_LABEL_KEYS,
  ORGANIZATION_ACCESS_STATES,
  ORGANIZATION_STATUSES,
  ORGANIZATION_WINDOWS,
  STATUS_LABEL_KEYS,
  WINDOW_LABEL_KEYS,
} from "@/features/organizations/constants";
import {
  OrganizationAccessChip,
  OrganizationStatusChip,
  ownerLabel,
} from "@/features/organizations/components/organization-cells";
import type { OrganizationSummary } from "@/features/organizations/types";
import { OrganizationAvatar } from "@/features/tenant/components/organization-avatar";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

export type OrganizationRowActionHandlers = {
  onView: (org: OrganizationSummary) => void;
  onEdit: (org: OrganizationSummary) => void;
  onToggleStatus: (org: OrganizationSummary) => void;
  onDelete: (org: OrganizationSummary) => void;
};

function Count({ value }: { value: number }) {
  return <span className="tabular-nums">{value}</span>;
}

/**
 * Columns follow `GET /v1/platform/organizations/meta`: status, access
 * state and access window are server-side faceted filters.
 */
export function useOrganizationsColumns(
  handlers: OrganizationRowActionHandlers,
) {
  const { t, locale } = useLocale();

  return useMemo(
    () =>
      [
        createColumn<OrganizationSummary>({
          accessorKey: "name",
          labelKey: "organizations.columns.name",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex min-w-0 items-center gap-2">
              <OrganizationAvatar
                logoUrl={row.original.logo_url}
                className="size-7 rounded-md"
              />
              <span className="truncate font-medium">{row.original.name}</span>
            </span>
          ),
        }),
        createColumn<OrganizationSummary>({
          accessorKey: "slug",
          labelKey: "organizations.columns.slug",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <code className="text-muted-foreground text-xs">
              {row.original.slug}
            </code>
          ),
        }),
        createColumn<OrganizationSummary>({
          id: "owner",
          accessorFn: (row) => (row.owner ? ownerLabel(row.owner) : ""),
          labelKey: "organizations.columns.owner",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.owner ? (
              <span className="flex min-w-0 flex-col">
                <span className="truncate">
                  {ownerLabel(row.original.owner)}
                </span>
                <span className="text-muted-foreground truncate text-xs">
                  {row.original.owner.email}
                </span>
              </span>
            ) : row.original.pending_invitation_count > 0 ? (
              <Badge variant="warning">
                {t("organizations.owner_invited")}
              </Badge>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<OrganizationSummary>({
          accessorKey: "member_count",
          labelKey: "organizations.columns.member_count",
          enableSorting: true,
          cell: ({ row }) => <Count value={row.original.member_count} />,
        }),
        createColumn<OrganizationSummary>({
          accessorKey: "branch_count",
          labelKey: "organizations.columns.branch_count",
          enableSorting: true,
          cell: ({ row }) => <Count value={row.original.branch_count} />,
        }),
        createColumn<OrganizationSummary>({
          accessorKey: "status",
          labelKey: "organizations.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: ORGANIZATION_STATUSES.map((value) => ({
            value,
            labelKey: STATUS_LABEL_KEYS[value],
            label: t(STATUS_LABEL_KEYS[value]),
          })),
          cell: ({ row }) => (
            <OrganizationStatusChip status={row.original.status} />
          ),
        }),
        createColumn<OrganizationSummary>({
          id: "access",
          accessorFn: (row) => row.access_state,
          labelKey: "organizations.columns.access",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: ORGANIZATION_ACCESS_STATES.map((value) => ({
            value,
            labelKey: ACCESS_LABEL_KEYS[value],
            label: t(ACCESS_LABEL_KEYS[value]),
          })),
          cell: ({ row }) => (
            <OrganizationAccessChip state={row.original.access_state} />
          ),
        }),
        createColumn<OrganizationSummary>({
          id: "window",
          accessorFn: (row) => row.access_ends_at ?? "",
          labelKey: "organizations.columns.access_ends_at",
          enableSorting: false,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: ORGANIZATION_WINDOWS.map((value) => ({
            value,
            labelKey: WINDOW_LABEL_KEYS[value],
            label: t(WINDOW_LABEL_KEYS[value]),
          })),
          cell: ({ row }) =>
            row.original.access_ends_at ? (
              date(row.original.access_ends_at, undefined, locale)
            ) : (
              <span className="text-muted-foreground">
                {t("organizations.window.unlimited")}
              </span>
            ),
        }),
        createColumn<OrganizationSummary>({
          accessorKey: "created_at",
          labelKey: "organizations.columns.created_at",
          enableSorting: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, undefined, locale),
        }),
        createColumn<OrganizationSummary>({
          id: "actions",
          labelKey: "organizations.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const org = row.original;
            const write = permissions.organizations.write;
            const actions: EntityRowAction[] = [
              {
                id: "view",
                label: t("organizations.actions.view"),
                icon: Eye,
                onSelect: () => handlers.onView(org),
              },
              {
                id: "edit",
                label: t("organizations.actions.edit"),
                icon: Pencil,
                permission: write,
                onSelect: () => handlers.onEdit(org),
              },
              {
                id: "toggle-status",
                label:
                  org.status === "active"
                    ? t("organizations.actions.suspend")
                    : t("organizations.actions.activate"),
                icon: org.status === "active" ? Pause : Play,
                permission: write,
                onSelect: () => handlers.onToggleStatus(org),
              },
              {
                id: "delete",
                label: t("organizations.actions.delete"),
                icon: Trash2,
                permission: write,
                variant: "destructive",
                separatorBefore: true,
                onSelect: () => handlers.onDelete(org),
              },
            ];
            return <EntityRowActions actions={actions} />;
          },
        }),
      ] as ColumnDef<OrganizationSummary, unknown>[],
    [handlers, locale, t],
  );
}
