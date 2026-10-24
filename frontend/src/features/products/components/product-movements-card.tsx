"use client";

import { useMemo, useState } from "react";

import {
  EntitySectionCard,
  EntityTable,
  useServerListState,
} from "@/components/entity";
import { permissions } from "@/config/permissions";
import { useMovementColumns } from "@/features/products/components/movement-columns";
import { SaleReturnDialog } from "@/features/products/components/sale-return-dialog";
import {
  StockSaleCollectDialog,
  StockSaleRefundDialog,
} from "@/features/products/components/stock-sale-money-dialogs";
import { useStockMovementsList } from "@/features/products/hooks/use-products";
import type {
  ListStockMovementsParams,
  Product,
  StockMovement,
} from "@/features/products/types";
import { useNavModules } from "@/features/nav-engine/lib/modules";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Ledger rows of one product (all its variants), newest first. */
export function ProductMovementsCard({ product }: { product: Product }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const hasModule = useNavModules();
  const [returning, setReturning] = useState<StockMovement | null>(null);
  const [collecting, setCollecting] = useState<StockMovement | null>(null);
  const [refunding, setRefunding] = useState<StockMovement | null>(null);
  const canReturn =
    can(permissions.tenant.products.stock) &&
    can(permissions.tenant.payment.refund);
  const canCollect =
    hasModule("cash") && can(permissions.tenant.payment.collect);
  const canRefund = hasModule("cash") && can(permissions.tenant.payment.refund);
  const listState = useServerListState({
    initialSort: "-occurred_at",
    initialPageSize: 10,
  });
  const params = useMemo<ListStockMovementsParams>(
    () => ({ ...listState.params, product: product.uuid }),
    [listState.params, product.uuid],
  );
  const query = useStockMovementsList(params);
  const columns = useMovementColumns({
    withProduct: false,
    filters: false,
    onReturn: canReturn ? setReturning : undefined,
    onCollect: canCollect ? setCollecting : undefined,
    onRefund: canRefund ? setRefunding : undefined,
  });
  const total = query.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 10)),
  );

  return (
    <EntitySectionCard
      title={t("products.detail.movements")}
      badge={query.data ? total : undefined}
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("products.movements.load_failed")}
        onRetry={() => void query.refetch()}
        emptyTitle={t("products.movements.empty_title")}
        emptyDescription={t("products.movements.product_empty_description")}
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-product-movements-v1",
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          globalFilter: false,
        }}
      />
      <SaleReturnDialog
        movement={returning}
        onOpenChange={(open) => {
          if (!open) setReturning(null);
        }}
      />
      <StockSaleCollectDialog
        movement={collecting}
        onOpenChange={(open) => {
          if (!open) setCollecting(null);
        }}
      />
      <StockSaleRefundDialog
        movement={refunding}
        onOpenChange={(open) => {
          if (!open) setRefunding(null);
        }}
      />
    </EntitySectionCard>
  );
}
