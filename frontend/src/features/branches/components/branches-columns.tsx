"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { Badge } from "@/components/ui/badge";
import { createColumn } from "@/components/tables";
import {
  BranchRowActions,
  type BranchRowActionHandlers,
} from "@/features/branches/components/branch-row-actions";
import type { Branch, BranchFacets } from "@/features/branches/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type BranchesColumnsOptions = {
  handlers: BranchRowActionHandlers;
  facets?: BranchFacets;
};

/**
 * Columns follow `GET /v1/tenant/branches/meta`: sortable keys match
 * `sortable_fields`; `city` and `is_active` are server-side faceted filters.
 */
export function useBranchesColumns({
  handlers,
  facets,
}: BranchesColumnsOptions) {
  const { t, locale } = useLocale();

  return useMemo(
    () =>
      [
        createColumn<Branch>({
          accessorKey: "code",
          labelKey: "branches.columns.code",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <code className="bg-muted rounded px-1.5 py-0.5 text-xs font-semibold">
              {row.original.code}
            </code>
          ),
        }),
        createColumn<Branch>({
          accessorKey: "name",
          labelKey: "branches.columns.name",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.name}</span>
          ),
        }),
        createColumn<Branch>({
          accessorKey: "city",
          labelKey: "branches.columns.city",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: (facets?.city ?? []).map((facet) => ({
            value: facet.value,
            label: `${facet.value} (${facet.count})`,
          })),
          cell: ({ row }) => row.original.city || "—",
        }),
        createColumn<Branch>({
          accessorKey: "district",
          labelKey: "branches.columns.district",
          enableSorting: false,
          cell: ({ row }) => row.original.district || "—",
        }),
        createColumn<Branch>({
          accessorKey: "phone",
          labelKey: "branches.columns.phone",
          enableSorting: false,
          cell: ({ row }) => row.original.phone || "—",
        }),
        createColumn<Branch>({
          accessorKey: "email",
          labelKey: "branches.columns.email",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.email || "—",
        }),
        createColumn<Branch>({
          accessorKey: "prep_minutes",
          labelKey: "branches.columns.prep_minutes",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) =>
            t("branches.minutes", { count: row.original.prep_minutes }),
        }),
        createColumn<Branch>({
          accessorKey: "service_points_count",
          labelKey: "branches.columns.service_points_count",
          enableSorting: false,
          cell: ({ row }) => (
            <Badge variant="secondary" className="tabular-nums">
              {row.original.service_points_count}
            </Badge>
          ),
        }),
        createColumn<Branch>({
          id: "is_active",
          accessorFn: (row) => String(row.is_active),
          labelKey: "branches.columns.is_active",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: [
            {
              value: "true",
              labelKey: "branches.status.active",
              label: t("branches.status.active"),
            },
            {
              value: "false",
              labelKey: "branches.status.inactive",
              label: t("branches.status.inactive"),
            },
          ],
          cell: ({ row }) => (
            <StatusChip
              label={
                row.original.is_active
                  ? t("branches.status.active")
                  : t("branches.status.inactive")
              }
              tone={row.original.is_active ? "success" : "default"}
            />
          ),
        }),
        createColumn<Branch>({
          accessorKey: "created_at",
          labelKey: "branches.columns.created_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, undefined, locale),
        }),
        createColumn<Branch>({
          accessorKey: "uuid",
          labelKey: "branches.columns.uuid",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <code className="text-muted-foreground text-xs">
              {row.original.uuid}
            </code>
          ),
        }),
        createColumn<Branch>({
          id: "actions",
          labelKey: "branches.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <BranchRowActions branch={row.original} handlers={handlers} />
          ),
        }),
      ] as ColumnDef<Branch, unknown>[],
    [facets?.city, handlers, locale, t],
  );
}
