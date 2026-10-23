"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { ArrowLeftRight, Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityDeleteDialog,
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { ProductVariantDialog } from "@/features/products/components/product-variant-dialog";
import { StockStatusChip } from "@/features/products/components/stock-status-chip";
import {
  useCreateProductVariant,
  useDeleteProductVariant,
  useUpdateProductVariant,
} from "@/features/products/hooks/use-products";
import {
  localizeProductError,
  priceLabel,
} from "@/features/products/lib/format";
import {
  variantFormToRequest,
  type VariantFormValues,
} from "@/features/products/schemas/product-form";
import type { Product, ProductVariant } from "@/features/products/types";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/**
 * Variants of the product (size, color …), each with its own stock code,
 * optional price and stock. A product with variants keeps its stock on
 * the variants; the first variant needs the product's own stock at zero.
 */
export function ProductVariantsCard({
  product,
  onMovement,
}: {
  product: Product;
  onMovement: (variant: ProductVariant) => void;
}) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.products.write);
  const canMove =
    can(permissions.tenant.products.stock) ||
    can(permissions.tenant.products.sell);
  const create = useCreateProductVariant(product.uuid);
  const update = useUpdateProductVariant(product.uuid);
  const remove = useDeleteProductVariant(product.uuid);
  const [dialog, setDialog] = useState<{
    open: boolean;
    variant: ProductVariant | null;
  }>({ open: false, variant: null });
  const [removeTarget, setRemoveTarget] = useState<ProductVariant | null>(null);
  const variants = product.variants ?? [];

  const submit = async (values: VariantFormValues) => {
    const body = variantFormToRequest(values);
    try {
      if (dialog.variant) {
        await update.mutateAsync({ variantUuid: dialog.variant.uuid, body });
      } else {
        await create.mutateAsync(body);
      }
    } catch (error) {
      localizeProductError(error, t);
      throw error;
    }
  };

  const columns = [
    createColumn<ProductVariant>({
      accessorKey: "name",
      labelKey: "products.variants.fields.name",
      enableSorting: false,
      gridPrimary: true,
      cell: ({ row }) => (
        <span className="font-medium">{row.original.name}</span>
      ),
    }),
    createColumn<ProductVariant>({
      accessorKey: "sku",
      labelKey: "products.columns.sku",
      enableSorting: false,
      cell: ({ row }) => (
        <span className="flex flex-col">
          <code className="text-xs">{row.original.sku}</code>
          {row.original.barcode ? (
            <code className="text-muted-foreground text-xs">
              {row.original.barcode}
            </code>
          ) : null}
        </span>
      ),
    }),
    createColumn<ProductVariant>({
      accessorKey: "effective_price",
      labelKey: "products.columns.price",
      enableSorting: false,
      gridSecondary: true,
      cell: ({ row }) => (
        <span className="flex items-center gap-2">
          {priceLabel(row.original.effective_price, product.currency, locale)}
          {row.original.price != null ? (
            <Badge variant="secondary">{t("products.offers.override")}</Badge>
          ) : null}
        </span>
      ),
    }),
    createColumn<ProductVariant>({
      id: "stock",
      accessorFn: (row) => row.stock.available,
      labelKey: "products.columns.stock",
      enableSorting: false,
      gridSecondary: true,
      cell: ({ row }) => <StockStatusChip stock={row.original.stock} />,
    }),
    createColumn<ProductVariant>({
      id: "is_active",
      accessorFn: (row) => String(row.is_active),
      labelKey: "products.columns.is_active",
      enableSorting: false,
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
    createColumn<ProductVariant>({
      id: "actions",
      labelKey: "products.columns.actions",
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) => {
        const actions: EntityRowAction[] = [];
        if (canMove) {
          actions.push({
            id: "movement",
            label: t("products.actions.record_movement"),
            icon: ArrowLeftRight,
            onSelect: () => onMovement(row.original),
          });
        }
        actions.push(
          {
            id: "edit",
            label: t("products.actions.edit"),
            icon: Pencil,
            permission: permissions.tenant.products.write,
            onSelect: () => setDialog({ open: true, variant: row.original }),
          },
          {
            id: "remove",
            label: t("products.actions.delete"),
            icon: Trash2,
            variant: "destructive",
            permission: permissions.tenant.products.write,
            onSelect: () => setRemoveTarget(row.original),
          },
        );
        return <EntityRowActions actions={actions} />;
      },
    }),
  ] as ColumnDef<ProductVariant, unknown>[];

  return (
    <EntitySectionCard
      title={t("products.detail.variants")}
      badge={variants.length}
      actions={
        canWrite ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => setDialog({ open: true, variant: null })}
          >
            <Plus className="size-4" />
            {t("products.variants.add")}
          </Button>
        ) : null
      }
    >
      <p className="text-muted-foreground mb-3 text-sm">
        {t("products.variants.description")}
      </p>
      <EntityTable
        columns={columns}
        data={variants}
        getRowId={(row) => row.uuid}
        emptyTitle={t("products.variants.empty_title")}
        emptyDescription={t("products.variants.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          globalFilter: false,
          sorting: false,
          pagination: false,
          persistKey: "tenant-product-variants-v1",
        }}
      />
      <ProductVariantDialog
        open={dialog.open}
        product={product}
        variant={dialog.variant}
        isPending={create.isPending || update.isPending}
        onOpenChange={(open) =>
          setDialog((prev) => ({ open, variant: open ? prev.variant : null }))
        }
        onSubmit={submit}
      />
      <EntityDeleteDialog
        open={Boolean(removeTarget)}
        entityLabel={removeTarget?.name ?? ""}
        isPending={remove.isPending}
        onCancel={() => setRemoveTarget(null)}
        onConfirm={() => {
          if (!removeTarget) return;
          remove.mutate(removeTarget.uuid, {
            onSettled: () => setRemoveTarget(null),
          });
        }}
      />
    </EntitySectionCard>
  );
}
