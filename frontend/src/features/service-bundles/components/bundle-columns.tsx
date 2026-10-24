"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { createColumn } from "@/components/tables";
import {
  BundleRowActions,
  type BundleRowActionHandlers,
} from "@/features/service-bundles/components/bundle-row-actions";
import type {
  ServiceBundle,
  ServiceBundleFacets,
} from "@/features/service-bundles/types";
import { priceLabel } from "@/features/services/lib/format";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type BundleColumnsOptions = {
  handlers: BundleRowActionHandlers;
  facets?: ServiceBundleFacets;
};

/**
 * Columns follow `GET /v1/tenant/service-bundles/meta`: sortable keys match
 * `sortable_fields`; `branch`, `is_active` and `show_online` are
 * server-side faceted filters.
 */
export function useBundleColumns({ handlers, facets }: BundleColumnsOptions) {
  const { t, locale } = useLocale();

  return useMemo(() => {
    const yesNo = (yes: string, no: string) => [
      { value: "true", labelKey: yes, label: t(yes) },
      { value: "false", labelKey: no, label: t(no) },
    ];
    return [
      createColumn<ServiceBundle>({
        accessorKey: "name",
        labelKey: "service_bundles.columns.name",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-medium">{row.original.name}</span>
        ),
      }),
      createColumn<ServiceBundle>({
        id: "items",
        accessorFn: (row) => row.items.length,
        labelKey: "service_bundles.columns.items",
        enableSorting: false,
        gridSecondary: true,
        cell: ({ row }) => (
          <span className="line-clamp-1">
            {row.original.items.map((item) => item.service_name).join(" + ")}
          </span>
        ),
      }),
      createColumn<ServiceBundle>({
        accessorKey: "duration_minutes",
        labelKey: "service_bundles.columns.duration",
        enableSorting: false,
        cell: ({ row }) =>
          t("services.minutes", {
            minutes: String(row.original.duration_minutes),
          }),
      }),
      createColumn<ServiceBundle>({
        accessorKey: "price",
        labelKey: "service_bundles.columns.price",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) =>
          priceLabel(row.original.price, row.original.currency, locale),
      }),
      createColumn<ServiceBundle>({
        accessorKey: "list_price",
        labelKey: "service_bundles.columns.list_price",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="text-muted-foreground">
            {priceLabel(row.original.list_price, row.original.currency, locale)}
          </span>
        ),
      }),
      createColumn<ServiceBundle>({
        id: "branch",
        accessorFn: (row) => row.branches.length,
        labelKey: "service_bundles.columns.branches",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: (facets?.branch ?? []).map((facet) => ({
          value: facet.value,
          label: `${facet.label ?? facet.value} (${facet.count})`,
        })),
        cell: ({ row }) =>
          row.original.branches.length === 0 ? (
            <span className="text-muted-foreground">
              {t("service_bundles.no_branches")}
            </span>
          ) : (
            <span className="line-clamp-1">
              {row.original.branches.map((branch) => branch.code).join(", ")}
            </span>
          ),
      }),
      createColumn<ServiceBundle>({
        id: "show_online",
        accessorFn: (row) => String(row.show_online),
        labelKey: "service_bundles.columns.show_online",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: yesNo(
          "service_bundles.online.visible",
          "service_bundles.online.hidden",
        ),
        cell: ({ row }) => (
          <StatusChip
            label={
              row.original.show_online
                ? t("service_bundles.online.visible")
                : t("service_bundles.online.hidden")
            }
            tone={row.original.show_online ? "success" : "default"}
          />
        ),
      }),
      createColumn<ServiceBundle>({
        id: "is_active",
        accessorFn: (row) => String(row.is_active),
        labelKey: "service_bundles.columns.is_active",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: yesNo(
          "service_bundles.status.active",
          "service_bundles.status.inactive",
        ),
        cell: ({ row }) => (
          <StatusChip
            label={
              row.original.is_active
                ? row.original.bookable
                  ? t("service_bundles.status.active")
                  : t("service_bundles.status.not_bookable")
                : t("service_bundles.status.inactive")
            }
            tone={
              row.original.is_active
                ? row.original.bookable
                  ? "success"
                  : "warning"
                : "default"
            }
          />
        ),
      }),
      createColumn<ServiceBundle>({
        accessorKey: "sort_order",
        labelKey: "service_bundles.columns.sort_order",
        enableSorting: true,
        defaultHidden: true,
      }),
      createColumn<ServiceBundle>({
        accessorKey: "created_at",
        labelKey: "service_bundles.columns.created_at",
        enableSorting: true,
        defaultHidden: true,
        cell: ({ row }) => datetime(row.original.created_at, undefined, locale),
      }),
      createColumn<ServiceBundle>({
        accessorKey: "uuid",
        labelKey: "service_bundles.columns.uuid",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <code className="text-muted-foreground text-xs">
            {row.original.uuid}
          </code>
        ),
      }),
      createColumn<ServiceBundle>({
        id: "actions",
        labelKey: "service_bundles.columns.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => (
          <BundleRowActions bundle={row.original} handlers={handlers} />
        ),
      }),
    ] as ColumnDef<ServiceBundle, unknown>[];
  }, [facets?.branch, handlers, locale, t]);
}
