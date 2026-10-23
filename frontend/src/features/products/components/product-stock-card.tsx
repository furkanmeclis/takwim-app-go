"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { ArrowLeftRight } from "lucide-react";

import {
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { StockStatusChip } from "@/features/products/components/stock-status-chip";
import type { Product, ProductStockLevel } from "@/features/products/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * Stock of the product (or each variant) per branch of the member's
 * scope. Reserved units (TEC-919 holds) are not available for sale.
 */
export function ProductStockCard({
  product,
  canMove,
  onMovement,
}: {
  product: Product;
  canMove: boolean;
  onMovement: (level?: ProductStockLevel) => void;
}) {
  const { t, locale } = useLocale();
  const levels = product.levels ?? [];
  const hasVariants = product.variants_count > 0;
  const count = (value: number) => value.toLocaleString(locale);

  const columns = [
    createColumn<ProductStockLevel>({
      accessorKey: "branch_name",
      labelKey: "products.stock.columns.branch",
      enableSorting: false,
      gridPrimary: true,
      cell: ({ row }) => (
        <span className="font-medium">
          {row.original.branch_name}{" "}
          <span className="text-muted-foreground text-xs">
            {row.original.branch_code}
          </span>
        </span>
      ),
    }),
    ...(hasVariants
      ? [
          createColumn<ProductStockLevel>({
            accessorKey: "variant_name",
            labelKey: "products.stock.columns.variant",
            enableSorting: false,
            cell: ({ row }) => (
              <span className="flex flex-col">
                <span>{row.original.variant_name}</span>
                <code className="text-muted-foreground text-xs">
                  {row.original.variant_sku}
                </code>
              </span>
            ),
          }),
        ]
      : []),
    createColumn<ProductStockLevel>({
      accessorKey: "on_hand",
      labelKey: "products.stock.columns.on_hand",
      enableSorting: false,
      cell: ({ row }) => (
        <span className="tabular-nums">{count(row.original.on_hand)}</span>
      ),
    }),
    createColumn<ProductStockLevel>({
      accessorKey: "reserved",
      labelKey: "products.stock.columns.reserved",
      enableSorting: false,
      cell: ({ row }) => (
        <span className="tabular-nums">{count(row.original.reserved)}</span>
      ),
    }),
    createColumn<ProductStockLevel>({
      id: "available",
      accessorFn: (row) => row.available,
      labelKey: "products.stock.columns.available",
      enableSorting: false,
      gridSecondary: true,
      cell: ({ row }) => <StockStatusChip stock={row.original} />,
    }),
    createColumn<ProductStockLevel>({
      accessorKey: "updated_at",
      labelKey: "products.stock.columns.updated_at",
      enableSorting: false,
      cell: ({ row }) => datetime(row.original.updated_at, undefined, locale),
    }),
    ...(canMove
      ? [
          createColumn<ProductStockLevel>({
            id: "actions",
            labelKey: "products.columns.actions",
            enableSorting: false,
            enableHiding: false,
            cell: ({ row }) => (
              <EntityRowActions
                actions={[
                  {
                    id: "movement",
                    label: t("products.actions.record_movement"),
                    icon: ArrowLeftRight,
                    onSelect: () => onMovement(row.original),
                  },
                ]}
              />
            ),
          }),
        ]
      : []),
  ] as ColumnDef<ProductStockLevel, unknown>[];

  return (
    <EntitySectionCard
      title={t("products.detail.stock")}
      badge={count(product.stock.available)}
      actions={
        canMove ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => onMovement()}
          >
            <ArrowLeftRight className="size-4" />
            {t("products.actions.record_movement")}
          </Button>
        ) : null
      }
    >
      <EntityTable
        columns={columns}
        data={levels}
        getRowId={(row) => `${row.branch_uuid}:${row.variant_uuid ?? ""}`}
        emptyTitle={t("products.stock.empty_title")}
        emptyDescription={t("products.stock.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          globalFilter: false,
          sorting: false,
          pagination: false,
          persistKey: "tenant-product-stock-v1",
        }}
      />
    </EntitySectionCard>
  );
}
