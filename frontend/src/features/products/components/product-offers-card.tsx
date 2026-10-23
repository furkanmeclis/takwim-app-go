"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityDeleteDialog,
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { ProductOfferDialog } from "@/features/products/components/product-offer-dialog";
import { useSetProductOffers } from "@/features/products/hooks/use-products";
import { priceLabel } from "@/features/products/lib/format";
import {
  normalizePrice,
  type OfferFormValues,
} from "@/features/products/schemas/product-form";
import type {
  Product,
  ProductOffer,
  ProductOffersRequest,
} from "@/features/products/types";
import { useAccessibleBranches } from "@/features/staff/hooks/use-staff";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type OfferInput = ProductOffersRequest["offers"][number];

function toInput(offer: ProductOffer): OfferInput {
  return {
    branch_uuid: offer.branch_uuid,
    price: offer.price,
    is_active: offer.is_active,
  };
}

/**
 * Branches selling the product, with their own price. A member edits the
 * branches of their scope only (`tenant.products.offer`); the API keeps
 * the other branches' offers as they are. A sale needs an open offer.
 */
export function ProductOffersCard({ product }: { product: Product }) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canOffer = can(permissions.tenant.products.offer);
  const branches = useAccessibleBranches();
  const save = useSetProductOffers(product.uuid);
  const [dialog, setDialog] = useState<{
    open: boolean;
    offer: ProductOffer | null;
  }>({ open: false, offer: null });
  const [removeTarget, setRemoveTarget] = useState<ProductOffer | null>(null);

  const editable = product.offers.filter((offer) => offer.editable);
  const freeBranches = useMemo(() => {
    const taken = new Set(product.offers.map((offer) => offer.branch_uuid));
    return (branches.data?.items ?? [])
      .filter((branch) => !taken.has(branch.uuid))
      .map((branch) => ({
        value: branch.uuid,
        label: `${branch.name} (${branch.code})`,
      }));
  }, [branches.data?.items, product.offers]);

  const put = (offers: OfferInput[]) => save.mutateAsync({ offers });

  const submit = async (values: OfferFormValues) => {
    const next: OfferInput = {
      branch_uuid: values.branch_uuid,
      price: normalizePrice(values.price),
      is_active: values.is_active,
    };
    const rest = editable
      .filter((offer) => offer.branch_uuid !== values.branch_uuid)
      .map(toInput);
    await put([...rest, next]);
  };

  const columns = [
    createColumn<ProductOffer>({
      accessorKey: "branch_name",
      labelKey: "products.offers.fields.branch",
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
    createColumn<ProductOffer>({
      accessorKey: "effective_price",
      labelKey: "products.offers.fields.price",
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
    createColumn<ProductOffer>({
      id: "is_active",
      accessorFn: (row) => String(row.is_active),
      labelKey: "products.offers.fields.is_active",
      enableSorting: false,
      cell: ({ row }) => (
        <StatusChip
          label={
            row.original.is_active
              ? t("products.offers.open")
              : t("products.offers.paused")
          }
          tone={row.original.is_active ? "success" : "default"}
        />
      ),
    }),
    createColumn<ProductOffer>({
      id: "actions",
      labelKey: "products.columns.actions",
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) =>
        row.original.editable ? (
          <EntityRowActions
            actions={[
              {
                id: "edit",
                label: t("products.actions.edit"),
                icon: Pencil,
                permission: permissions.tenant.products.offer,
                onSelect: () => setDialog({ open: true, offer: row.original }),
              },
              {
                id: "remove",
                label: t("products.offers.remove"),
                icon: Trash2,
                variant: "destructive",
                permission: permissions.tenant.products.offer,
                onSelect: () => setRemoveTarget(row.original),
              },
            ]}
          />
        ) : null,
    }),
  ] as ColumnDef<ProductOffer, unknown>[];

  return (
    <EntitySectionCard
      title={t("products.detail.offers")}
      badge={product.offers.length}
      actions={
        canOffer ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={freeBranches.length === 0}
            onClick={() => setDialog({ open: true, offer: null })}
          >
            <Plus className="size-4" />
            {t("products.offers.add")}
          </Button>
        ) : null
      }
    >
      <EntityTable
        columns={columns}
        data={product.offers}
        getRowId={(row) => row.branch_uuid}
        emptyTitle={t("products.offers.empty_title")}
        emptyDescription={t("products.offers.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          globalFilter: false,
          sorting: false,
          pagination: false,
          persistKey: "tenant-product-offers-v1",
        }}
      />
      <ProductOfferDialog
        open={dialog.open}
        offer={dialog.offer}
        branchOptions={freeBranches}
        catalogPrice={product.price}
        isPending={save.isPending}
        onOpenChange={(open) =>
          setDialog((prev) => ({ open, offer: open ? prev.offer : null }))
        }
        onSubmit={submit}
      />
      <EntityDeleteDialog
        open={Boolean(removeTarget)}
        softDelete={false}
        entityLabel={removeTarget?.branch_name ?? ""}
        isPending={save.isPending}
        onCancel={() => setRemoveTarget(null)}
        onConfirm={() => {
          if (!removeTarget) return;
          void put(
            editable
              .filter((offer) => offer.branch_uuid !== removeTarget.branch_uuid)
              .map(toInput),
          ).finally(() => setRemoveTarget(null));
        }}
      />
    </EntitySectionCard>
  );
}
