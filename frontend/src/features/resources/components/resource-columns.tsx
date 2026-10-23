"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import {
  ResourceRowActions,
  type ResourceRowActionHandlers,
} from "@/features/resources/components/resource-row-actions";
import {
  capacityLabel,
  useResourceKinds,
} from "@/features/resources/lib/kinds";
import type {
  Resource,
  ResourceFacets,
  ResourceKind,
} from "@/features/resources/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type ResourceColumnsOptions = {
  handlers: ResourceRowActionHandlers;
  facets?: ResourceFacets;
};

/**
 * Columns follow `GET /v1/tenant/resources/meta`: sortable keys match
 * `sortable_fields`; `kind`, `branch`, `zone`, `is_active` and
 * `show_online` are server-side faceted filters.
 */
export function useResourceColumns({
  handlers,
  facets,
}: ResourceColumnsOptions) {
  const { t, locale } = useLocale();
  const kinds = useResourceKinds();

  return useMemo(() => {
    const yesNo = (yes: string, no: string) => [
      { value: "true", labelKey: yes, label: t(yes) },
      { value: "false", labelKey: no, label: t(no) },
    ];
    return [
      createColumn<Resource>({
        accessorKey: "name",
        labelKey: "resources.columns.name",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-medium">{row.original.name}</span>
        ),
      }),
      createColumn<Resource>({
        accessorKey: "kind",
        labelKey: "resources.columns.kind",
        enableSorting: true,
        gridSecondary: true,
        filterVariant: "faceted",
        filterOptions: (facets?.kind ?? []).map((facet) => ({
          value: facet.value,
          label: `${kinds.label(facet.value as ResourceKind)} (${facet.count})`,
        })),
        cell: ({ row }) => (
          <Badge variant="secondary">{kinds.label(row.original.kind)}</Badge>
        ),
      }),
      createColumn<Resource>({
        id: "branch",
        accessorFn: (row) => row.branch.name,
        labelKey: "resources.columns.branch",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: (facets?.branch ?? []).map((facet) => ({
          value: facet.value,
          label: `${facet.label ?? facet.value} (${facet.count})`,
        })),
        cell: ({ row }) => row.original.branch.name,
      }),
      createColumn<Resource>({
        accessorKey: "zone",
        labelKey: "resources.columns.zone",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: (facets?.zone ?? []).map((facet) => ({
          value: facet.value,
          label: `${facet.value} (${facet.count})`,
        })),
        cell: ({ row }) => row.original.zone || "—",
      }),
      createColumn<Resource>({
        accessorKey: "capacity",
        labelKey: "resources.columns.capacity",
        enableSorting: true,
        cell: ({ row }) =>
          t("resources.capacity_people", {
            range: capacityLabel(
              row.original.min_capacity,
              row.original.capacity,
            ),
          }),
      }),
      createColumn<Resource>({
        id: "combinable",
        accessorFn: (row) => String(row.combinable),
        labelKey: "resources.columns.combinable",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) =>
          row.original.combinable
            ? t("resources.combinable.yes")
            : t("resources.combinable.no"),
      }),
      createColumn<Resource>({
        id: "show_online",
        accessorFn: (row) => String(row.show_online),
        labelKey: "resources.columns.show_online",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: yesNo(
          "resources.online.visible",
          "resources.online.hidden",
        ),
        cell: ({ row }) => (
          <StatusChip
            label={
              row.original.show_online
                ? t("resources.online.visible")
                : t("resources.online.hidden")
            }
            tone={row.original.show_online ? "success" : "default"}
          />
        ),
      }),
      createColumn<Resource>({
        id: "is_active",
        accessorFn: (row) => String(row.is_active),
        labelKey: "resources.columns.is_active",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: yesNo(
          "resources.status.active",
          "resources.status.inactive",
        ),
        cell: ({ row }) => (
          <StatusChip
            label={
              row.original.is_active
                ? t("resources.status.active")
                : t("resources.status.inactive")
            }
            tone={row.original.is_active ? "success" : "default"}
          />
        ),
      }),
      createColumn<Resource>({
        accessorKey: "sort_order",
        labelKey: "resources.columns.sort_order",
        enableSorting: true,
        defaultHidden: true,
      }),
      createColumn<Resource>({
        accessorKey: "created_at",
        labelKey: "resources.columns.created_at",
        enableSorting: true,
        defaultHidden: true,
        cell: ({ row }) => datetime(row.original.created_at, undefined, locale),
      }),
      createColumn<Resource>({
        accessorKey: "uuid",
        labelKey: "resources.columns.uuid",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <code className="text-muted-foreground text-xs">
            {row.original.uuid}
          </code>
        ),
      }),
      createColumn<Resource>({
        id: "actions",
        labelKey: "resources.columns.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => (
          <ResourceRowActions resource={row.original} handlers={handlers} />
        ),
      }),
    ] as ColumnDef<Resource, unknown>[];
  }, [facets?.branch, facets?.kind, facets?.zone, handlers, kinds, locale, t]);
}
