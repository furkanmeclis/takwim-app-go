"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { Pencil, Power, PowerOff, Trash2 } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PermissionGuard } from "@/components/common/permission-guard";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityActions,
  EntityDeleteDialog,
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
} from "@/components/entity";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ProductActivityCard } from "@/features/products/components/product-activity-card";
import { ProductImageCard } from "@/features/products/components/product-image-card";
import { ProductMovementsCard } from "@/features/products/components/product-movements-card";
import { ProductOffersCard } from "@/features/products/components/product-offers-card";
import { ProductStockCard } from "@/features/products/components/product-stock-card";
import { ProductVariantsCard } from "@/features/products/components/product-variants-card";
import { StockMovementDialog } from "@/features/products/components/stock-movement-dialog";
import { StockStatusChip } from "@/features/products/components/stock-status-chip";
import { UNIT_LABEL_KEYS } from "@/features/products/constants";
import {
  useDeleteProduct,
  useProduct,
  useUpdateProduct,
} from "@/features/products/hooks/use-products";
import { priceLabel } from "@/features/products/lib/format";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type MovementDefaults = { variantUuid?: string; branchUuid?: string };

/**
 * Show page: catalog values, variants, branch offers, stock per branch,
 * the product's ledger, photo and activity.
 */
export function ProductDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const router = useRouter();
  const query = useProduct(uuid);
  const update = useUpdateProduct();
  const remove = useDeleteProduct();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [movement, setMovement] = useState<{
    open: boolean;
    defaults: MovementDefaults;
  }>({ open: false, defaults: {} });
  const product = query.data;
  const title = product?.name ?? t("products.detail_title");
  const canMove =
    can(permissions.tenant.products.stock) ||
    can(permissions.tenant.products.sell);
  const openMovement = (defaults: MovementDefaults = {}) =>
    setMovement({ open: true, defaults });

  return (
    <EntityPage
      title={title}
      description={t("products.detail_description")}
      permission={permissions.tenant.products.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("products.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("products.title"),
          href: routes.tenant.products.root(slug),
        },
        { label: title },
      ]}
      actions={
        product ? (
          <PermissionGuard permission={permissions.tenant.products.write}>
            <EntityActions>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={update.isPending}
                onClick={() =>
                  update.mutate({
                    uuid,
                    body: { is_active: !product.is_active },
                  })
                }
              >
                {product.is_active ? (
                  <PowerOff className="size-4" />
                ) : (
                  <Power className="size-4" />
                )}
                {product.is_active
                  ? t("products.actions.deactivate")
                  : t("products.actions.activate")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="text-destructive hover:text-destructive"
                onClick={() => setConfirmDelete(true)}
              >
                <Trash2 className="size-4" />
                {t("products.actions.delete")}
              </Button>
              <Button asChild size="sm">
                <Link href={routes.tenant.products.edit(slug, uuid)}>
                  <Pencil className="size-4" />
                  {t("products.actions.edit")}
                </Link>
              </Button>
            </EntityActions>
          </PermissionGuard>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("products.detail_not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {product ? (
        <div className="space-y-6">
          <EntityHeader
            title={product.name}
            subtitle={product.sku}
            badges={
              <>
                <StatusChip
                  label={
                    product.is_active
                      ? t("products.status.active")
                      : t("products.status.inactive")
                  }
                  tone={product.is_active ? "success" : "default"}
                />
                <StatusChip
                  label={
                    product.show_online
                      ? t("products.online.visible")
                      : t("products.online.hidden")
                  }
                  tone={product.show_online ? "success" : "default"}
                />
                <StockStatusChip stock={product.stock} />
              </>
            }
            description={product.description || undefined}
          />

          <EntitySectionCard title={t("products.detail.overview")}>
            <EntityDetail
              sections={[
                {
                  id: "overview",
                  fields: [
                    {
                      key: "sku",
                      label: t("products.fields.sku"),
                      value: <code className="text-xs">{product.sku}</code>,
                    },
                    {
                      key: "barcode",
                      label: t("products.fields.barcode"),
                      value: product.barcode || "—",
                    },
                    {
                      key: "unit",
                      label: t("products.fields.unit"),
                      value: t(UNIT_LABEL_KEYS[product.unit]),
                    },
                    {
                      key: "price",
                      label: t("products.fields.price_short"),
                      value: priceLabel(
                        product.price,
                        product.currency,
                        locale,
                      ),
                    },
                    {
                      key: "cost",
                      label: t("products.fields.cost_short"),
                      value: priceLabel(product.cost, product.currency, locale),
                    },
                    {
                      key: "low_stock_threshold",
                      label: t("products.fields.low_stock_threshold"),
                      value: String(product.low_stock_threshold),
                    },
                    {
                      key: "stock",
                      label: t("products.fields.stock_summary"),
                      value: t("products.stock_summary", {
                        on_hand: product.stock.on_hand.toLocaleString(locale),
                        reserved: product.stock.reserved.toLocaleString(locale),
                        available:
                          product.stock.available.toLocaleString(locale),
                      }),
                    },
                    {
                      key: "created_at",
                      label: t("products.fields.created_at"),
                      value: datetime(product.created_at, undefined, locale),
                    },
                    {
                      key: "updated_at",
                      label: t("products.fields.updated_at"),
                      value: datetime(product.updated_at, undefined, locale),
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>

          <ProductStockCard
            product={product}
            canMove={canMove}
            onMovement={(level) =>
              openMovement({
                branchUuid: level?.branch_uuid,
                variantUuid: level?.variant_uuid ?? undefined,
              })
            }
          />
          <ProductVariantsCard
            product={product}
            onMovement={(variant) =>
              openMovement({ variantUuid: variant.uuid })
            }
          />
          <ProductOffersCard product={product} />
          <ProductMovementsCard product={product} />
          <ProductImageCard product={product} />
          <ProductActivityCard productUuid={uuid} />

          <StockMovementDialog
            open={movement.open}
            defaults={{ product, ...movement.defaults }}
            onOpenChange={(open) =>
              setMovement((prev) => ({
                open,
                defaults: open ? prev.defaults : {},
              }))
            }
          />

          <EntityDeleteDialog
            open={confirmDelete}
            entityLabel={product.name}
            isPending={remove.isPending}
            onCancel={() => setConfirmDelete(false)}
            onConfirm={() =>
              remove.mutate(uuid, {
                onSuccess: () => router.push(routes.tenant.products.root(slug)),
                onSettled: () => setConfirmDelete(false),
              })
            }
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
