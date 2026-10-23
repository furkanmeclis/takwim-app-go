"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import {
  ProductRowActions,
  type ProductRowActionHandlers,
} from "@/features/products/components/product-row-actions";
import { StockStatusChip } from "@/features/products/components/stock-status-chip";
import {
  STOCK_STATUS_LABEL_KEYS,
  STOCK_STATUSES,
  UNIT_LABEL_KEYS,
} from "@/features/products/constants";
import { priceLabel } from "@/features/products/lib/format";
import type { Product, ProductFacets } from "@/features/products/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type ProductColumnsOptions = {
  handlers: ProductRowActionHandlers;
  facets?: ProductFacets;
};

/**
 * Columns follow `GET /v1/tenant/products/meta`: sortable keys match
 * `sortable_fields`; `branch`, `stock_status`, `is_active` and
 * `show_online` are server-side faceted filters.
 */
export function useProductColumns({ handlers, facets }: ProductColumnsOptions) {
  const { t, locale } = useLocale();

  return useMemo(() => {
    const yesNo = (yes: string, no: string) => [
      { value: "true", labelKey: yes, label: t(yes) },
      { value: "false", labelKey: no, label: t(no) },
    ];
    return [
      createColumn<Product>({
        accessorKey: "name",
        labelKey: "products.columns.name",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="flex flex-col">
            <span className="font-medium">{row.original.name}</span>
            {row.original.variants_count > 0 ? (
              <span className="text-muted-foreground text-xs">
                {t("products.variants_count", {
                  count: String(row.original.variants_count),
                })}
              </span>
            ) : null}
          </span>
        ),
      }),
      createColumn<Product>({
        accessorKey: "sku",
        labelKey: "products.columns.sku",
        enableSorting: true,
        cell: ({ row }) => <code className="text-xs">{row.original.sku}</code>,
      }),
      createColumn<Product>({
        accessorKey: "barcode",
        labelKey: "products.columns.barcode",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) =>
          row.original.barcode ? (
            <code className="text-xs">{row.original.barcode}</code>
          ) : (
            <span className="text-muted-foreground">—</span>
          ),
      }),
      createColumn<Product>({
        accessorKey: "price",
        labelKey: "products.columns.price",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) =>
          priceLabel(row.original.price, row.original.currency, locale),
      }),
      createColumn<Product>({
        accessorKey: "cost",
        labelKey: "products.columns.cost",
        enableSorting: true,
        defaultHidden: true,
        cell: ({ row }) =>
          priceLabel(row.original.cost, row.original.currency, locale),
      }),
      createColumn<Product>({
        id: "stock",
        accessorFn: (row) => row.stock.available,
        labelKey: "products.columns.stock",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) => <StockStatusChip stock={row.original.stock} />,
      }),
      createColumn<Product>({
        id: "stock_status",
        accessorFn: (row) => row.stock.status,
        labelKey: "products.columns.stock_status",
        enableSorting: false,
        defaultHidden: true,
        filterVariant: "faceted",
        filterOptions: STOCK_STATUSES.map((status) => ({
          value: status,
          labelKey: STOCK_STATUS_LABEL_KEYS[status],
          label: t(STOCK_STATUS_LABEL_KEYS[status]),
        })),
        cell: ({ row }) => (
          <StockStatusChip stock={row.original.stock} showCount={false} />
        ),
      }),
      createColumn<Product>({
        id: "reserved",
        accessorFn: (row) => row.stock.reserved,
        labelKey: "products.columns.reserved",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => row.original.stock.reserved.toLocaleString(locale),
      }),
      createColumn<Product>({
        accessorKey: "unit",
        labelKey: "products.columns.unit",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => t(UNIT_LABEL_KEYS[row.original.unit]),
      }),
      createColumn<Product>({
        id: "branch",
        accessorFn: (row) => row.offers.length,
        labelKey: "products.columns.branches",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: (facets?.branch ?? []).map((facet) => ({
          value: facet.value,
          label: `${facet.label ?? facet.value} (${facet.count})`,
        })),
        cell: ({ row }) => {
          const offers = row.original.offers;
          if (offers.length === 0) {
            return (
              <span className="text-muted-foreground">
                {t("products.no_branches")}
              </span>
            );
          }
          return (
            <span className="flex flex-wrap gap-1">
              {offers.map((offer) => (
                <Badge
                  key={offer.branch_uuid}
                  variant={offer.is_active ? "secondary" : "outline"}
                >
                  {offer.branch_code}
                </Badge>
              ))}
            </span>
          );
        },
      }),
      createColumn<Product>({
        id: "show_online",
        accessorFn: (row) => String(row.show_online),
        labelKey: "products.columns.show_online",
        enableSorting: false,
        defaultHidden: true,
        filterVariant: "faceted",
        filterOptions: yesNo(
          "products.online.visible",
          "products.online.hidden",
        ),
        cell: ({ row }) => (
          <StatusChip
            label={
              row.original.show_online
                ? t("products.online.visible")
                : t("products.online.hidden")
            }
            tone={row.original.show_online ? "success" : "default"}
          />
        ),
      }),
      createColumn<Product>({
        id: "is_active",
        accessorFn: (row) => String(row.is_active),
        labelKey: "products.columns.is_active",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: yesNo(
          "products.status.active",
          "products.status.inactive",
        ),
        cell: ({ row }) => (
          <StatusChip
            label={
              row.original.is_active
                ? t("products.status.active")
                : t("products.status.inactive")
            }
            tone={row.original.is_active ? "success" : "default"}
          />
        ),
      }),
      createColumn<Product>({
        accessorKey: "created_at",
        labelKey: "products.columns.created_at",
        enableSorting: true,
        defaultHidden: true,
        cell: ({ row }) => datetime(row.original.created_at, undefined, locale),
      }),
      createColumn<Product>({
        accessorKey: "uuid",
        labelKey: "products.columns.uuid",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <code className="text-muted-foreground text-xs">
            {row.original.uuid}
          </code>
        ),
      }),
      createColumn<Product>({
        id: "actions",
        labelKey: "products.columns.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => (
          <ProductRowActions product={row.original} handlers={handlers} />
        ),
      }),
    ] as ColumnDef<Product, unknown>[];
  }, [facets?.branch, handlers, locale, t]);
}
