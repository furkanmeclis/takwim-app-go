"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { Badge } from "@/components/ui/badge";
import { createColumn } from "@/components/tables";
import { StaffAvatar } from "@/features/staff/components/staff-avatar";
import {
  StaffRowActions,
  type StaffRowActionHandlers,
} from "@/features/staff/components/staff-row-actions";
import type { Staff, StaffFacets } from "@/features/staff/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type StaffColumnsOptions = {
  handlers: StaffRowActionHandlers;
  facets?: StaffFacets;
};

/**
 * Columns follow `GET /v1/tenant/staff/meta`: sortable keys match
 * `sortable_fields`; `branches`, `is_active`, `show_online` and `account`
 * are server-side faceted filters.
 */
export function useStaffColumns({ handlers, facets }: StaffColumnsOptions) {
  const { t, locale } = useLocale();

  return useMemo(() => {
    const yesNo = (yes: string, no: string) => [
      { value: "true", labelKey: yes, label: t(yes) },
      { value: "false", labelKey: no, label: t(no) },
    ];
    return [
      createColumn<Staff>({
        accessorKey: "name",
        labelKey: "staff.columns.name",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <StaffAvatar name={row.original.name} color={row.original.color} />
            <span className="font-medium">{row.original.name}</span>
          </div>
        ),
      }),
      createColumn<Staff>({
        accessorKey: "title",
        labelKey: "staff.columns.title",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) => row.original.title || "—",
      }),
      createColumn<Staff>({
        id: "branch",
        accessorFn: (row) => row.branches.map((b) => b.name).join(", "),
        labelKey: "staff.columns.branches",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: (facets?.branch ?? []).map((facet) => ({
          value: facet.value,
          label: `${facet.label} (${facet.count})`,
        })),
        cell: ({ row }) =>
          row.original.branches.length ? (
            <div className="flex flex-wrap gap-1">
              {row.original.branches.map((branch) => (
                <Badge key={branch.uuid} variant="secondary">
                  {branch.name}
                </Badge>
              ))}
            </div>
          ) : (
            "—"
          ),
      }),
      createColumn<Staff>({
        accessorKey: "phone",
        labelKey: "staff.columns.phone",
        enableSorting: false,
        cell: ({ row }) => row.original.phone || "—",
      }),
      createColumn<Staff>({
        accessorKey: "email",
        labelKey: "staff.columns.email",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => row.original.email || "—",
      }),
      createColumn<Staff>({
        id: "has_account",
        accessorFn: (row) => String(Boolean(row.account)),
        labelKey: "staff.columns.account",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: yesNo("staff.account.linked", "staff.account.none"),
        cell: ({ row }) =>
          row.original.account ? (
            <span className="text-sm">{row.original.account.email}</span>
          ) : (
            <span className="text-muted-foreground text-sm">
              {t("staff.account.none")}
            </span>
          ),
      }),
      createColumn<Staff>({
        id: "show_online",
        accessorFn: (row) => String(row.show_online),
        labelKey: "staff.columns.show_online",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: yesNo("staff.online.visible", "staff.online.hidden"),
        cell: ({ row }) => (
          <StatusChip
            label={
              row.original.show_online
                ? t("staff.online.visible")
                : t("staff.online.hidden")
            }
            tone={row.original.show_online ? "success" : "default"}
          />
        ),
      }),
      createColumn<Staff>({
        id: "is_active",
        accessorFn: (row) => String(row.is_active),
        labelKey: "staff.columns.is_active",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: yesNo("staff.status.active", "staff.status.inactive"),
        cell: ({ row }) => (
          <StatusChip
            label={
              row.original.is_active
                ? t("staff.status.active")
                : t("staff.status.inactive")
            }
            tone={row.original.is_active ? "success" : "default"}
          />
        ),
      }),
      createColumn<Staff>({
        accessorKey: "created_at",
        labelKey: "staff.columns.created_at",
        enableSorting: true,
        defaultHidden: true,
        cell: ({ row }) => datetime(row.original.created_at, undefined, locale),
      }),
      createColumn<Staff>({
        accessorKey: "uuid",
        labelKey: "staff.columns.uuid",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <code className="text-muted-foreground text-xs">
            {row.original.uuid}
          </code>
        ),
      }),
      createColumn<Staff>({
        id: "actions",
        labelKey: "staff.columns.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => (
          <StaffRowActions staff={row.original} handlers={handlers} />
        ),
      }),
    ] as ColumnDef<Staff, unknown>[];
  }, [facets?.branch, handlers, locale, t]);
}
