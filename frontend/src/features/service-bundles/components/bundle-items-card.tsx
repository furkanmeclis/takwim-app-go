"use client";

import type { ColumnDef } from "@tanstack/react-table";

import { StatusChip } from "@/components/common/status-chip";
import { EntitySectionCard, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { useResourceKinds } from "@/features/resources/lib/kinds";
import type {
  ServiceBundle,
  ServiceBundleItem,
} from "@/features/service-bundles/types";
import { STAFF_MODE_LABEL_KEYS } from "@/features/services/constants";
import { priceLabel } from "@/features/services/lib/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * The services of the bundle in the order they are served, each with its
 * catalog price and its share of the bundle price. A booking keeps every
 * service as its own line with its own person and resources.
 */
export function BundleItemsCard({ bundle }: { bundle: ServiceBundle }) {
  const { t, locale } = useLocale();
  const kinds = useResourceKinds();
  const money = (amount: string) => priceLabel(amount, bundle.currency, locale);

  const columns = [
    createColumn<ServiceBundleItem>({
      accessorKey: "position",
      labelKey: "service_bundles.items.columns.position",
      enableSorting: false,
      cell: ({ row }) => row.original.position + 1,
    }),
    createColumn<ServiceBundleItem>({
      accessorKey: "service_name",
      labelKey: "service_bundles.items.columns.service",
      enableSorting: false,
      gridPrimary: true,
      cell: ({ row }) => (
        <span className="flex items-center gap-2 font-medium">
          {row.original.service_name}
          {row.original.bookable ? null : (
            <StatusChip
              label={t("service_bundles.items.not_bookable")}
              tone="warning"
            />
          )}
        </span>
      ),
    }),
    createColumn<ServiceBundleItem>({
      accessorKey: "duration_minutes",
      labelKey: "service_bundles.items.columns.duration",
      enableSorting: false,
      cell: ({ row }) =>
        t("services.minutes", {
          minutes: String(row.original.duration_minutes),
        }),
    }),
    createColumn<ServiceBundleItem>({
      accessorKey: "list_price",
      labelKey: "service_bundles.items.columns.list_price",
      enableSorting: false,
      cell: ({ row }) => money(row.original.list_price),
    }),
    createColumn<ServiceBundleItem>({
      accessorKey: "allocated_price",
      labelKey: "service_bundles.items.columns.allocated_price",
      enableSorting: false,
      gridSecondary: true,
      cell: ({ row }) => (
        <span className="font-medium">
          {money(row.original.allocated_price)}
        </span>
      ),
    }),
    createColumn<ServiceBundleItem>({
      accessorKey: "staff_mode",
      labelKey: "service_bundles.items.columns.staff_mode",
      enableSorting: false,
      cell: ({ row }) => t(STAFF_MODE_LABEL_KEYS[row.original.staff_mode]),
    }),
    createColumn<ServiceBundleItem>({
      id: "resource_kinds",
      accessorFn: (row) => row.resource_kinds.join(","),
      labelKey: "service_bundles.items.columns.resources",
      enableSorting: false,
      cell: ({ row }) =>
        row.original.resource_kinds.length > 0 ? (
          <span className="flex flex-wrap gap-1">
            {row.original.resource_kinds.map((kind) => (
              <Badge key={kind} variant="secondary">
                {kinds.label(kind)}
              </Badge>
            ))}
          </span>
        ) : (
          <span className="text-muted-foreground">
            {t("services.no_resources")}
          </span>
        ),
    }),
  ] as ColumnDef<ServiceBundleItem, unknown>[];

  return (
    <EntitySectionCard
      title={t("service_bundles.detail.items")}
      badge={bundle.items.length}
    >
      <p className="text-muted-foreground mb-3 text-sm">
        {t("service_bundles.detail.items_desc")}
      </p>
      <EntityTable
        columns={columns}
        data={bundle.items}
        getRowId={(row) => String(row.position)}
        emptyTitle={t("service_bundles.items.empty")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          globalFilter: false,
          sorting: false,
          pagination: false,
          persistKey: "tenant-service-bundle-items-v1",
        }}
      />
    </EntitySectionCard>
  );
}
