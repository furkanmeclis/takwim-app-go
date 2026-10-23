"use client";

import { useMemo } from "react";

import {
  EntitySectionCard,
  EntityTable,
  useServerListState,
} from "@/components/entity";
import { useMovementColumns } from "@/features/products/components/movement-columns";
import { useStockMovementsList } from "@/features/products/hooks/use-products";
import type {
  ListStockMovementsParams,
  Product,
} from "@/features/products/types";
import { useLocale } from "@/providers/locale-provider";

/** Ledger rows of one product (all its variants), newest first. */
export function ProductMovementsCard({ product }: { product: Product }) {
  const { t } = useLocale();
  const listState = useServerListState({
    initialSort: "-occurred_at",
    initialPageSize: 10,
  });
  const params = useMemo<ListStockMovementsParams>(
    () => ({ ...listState.params, product: product.uuid }),
    [listState.params, product.uuid],
  );
  const query = useStockMovementsList(params);
  const columns = useMovementColumns({ withProduct: false, filters: false });
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
    </EntitySectionCard>
  );
}
