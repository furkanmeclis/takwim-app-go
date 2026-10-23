"use client";

import { ArrowLeftRight, History } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityCreateButton,
  EntityDeleteDialog,
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createSelectColumnDef } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BulkActionMenu,
  SelectionBanner,
  resolveBulkActionsWithIcons,
  useBulkSelection,
} from "@/features/bulk-engine";
import { ResourceIOToolbar } from "@/features/io";
import { useProductColumns } from "@/features/products/components/product-columns";
import { ProductQuickEditDrawer } from "@/features/products/components/product-quick-edit-drawer";
import type { ProductRowActionHandlers } from "@/features/products/components/product-row-actions";
import { StockMovementDialog } from "@/features/products/components/stock-movement-dialog";
import { PRODUCTS_RESOURCE } from "@/features/products/constants";
import {
  useDeleteProduct,
  useProductsFacets,
  useProductsList,
  useProductsMeta,
  useUpdateProduct,
} from "@/features/products/hooks/use-products";
import type { ListProductsParams, Product } from "@/features/products/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

/** Both values of a yes / no facet selected = no filter. */
function oneBoolean(values: string[]) {
  return values.length === 1 ? (values[0] as "true" | "false") : undefined;
}

function joined(values: string[]) {
  return values.length ? values.join(",") : undefined;
}

/**
 * Tenant list standard: server-side pagination / sort / search, faceted
 * filters (branch, stock status, active, online), column visibility, row
 * actions, row click → detail, quick-edit drawer, stock movement dialog,
 * export / import, bulk actions with the selection banner.
 */
export function ProductsPage() {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const { can } = usePermission();
  const router = useRouter();

  const listState = useServerListState({
    initialSort: "",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, PRODUCTS_RESOURCE),
  });

  const [quickEdit, setQuickEdit] = useState<Product | null>(null);
  const [movement, setMovement] = useState<{
    open: boolean;
    product: Product | null;
  }>({ open: false, product: null });
  const [deleteTarget, setDeleteTarget] = useState<Product | null>(null);

  const listParams = useMemo<ListProductsParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    return {
      ...listState.params,
      branch: joined(column("branch")),
      stock_status: joined(column("stock_status")),
      is_active: oneBoolean(column("is_active")),
      show_online: oneBoolean(column("show_online")),
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useProductsList(listParams);
  const metaQuery = useProductsMeta();
  const facetsQuery = useProductsFacets();
  const updateProduct = useUpdateProduct();
  const deleteProduct = useDeleteProduct();

  const openDetail = useCallback(
    (product: Product) =>
      router.push(routes.tenant.products.detail(slug, product.uuid)),
    [router, slug],
  );

  const handlers = useMemo<ProductRowActionHandlers>(
    () => ({
      onView: openDetail,
      onQuickEdit: setQuickEdit,
      onEdit: (product) =>
        router.push(routes.tenant.products.edit(slug, product.uuid)),
      onMovement: (product) => setMovement({ open: true, product }),
      onToggleActive: (product) =>
        updateProduct.mutate({
          uuid: product.uuid,
          body: { is_active: !product.is_active },
        }),
      onDelete: setDeleteTarget,
    }),
    [openDetail, router, slug, updateProduct],
  );

  const baseColumns = useProductColumns({
    handlers,
    facets: facetsQuery.data,
  });
  const columns = useMemo(
    () => [createSelectColumnDef<Product>(), ...baseColumns],
    [baseColumns],
  );

  // Same filters for export, import refresh and "all matching" bulk scope.
  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      branch: listParams.branch,
      stock_status: listParams.stock_status,
      is_active: listParams.is_active,
      show_online: listParams.show_online,
      sort: listParams.sort,
    }),
    [
      listParams.branch,
      listParams.is_active,
      listParams.q,
      listParams.show_online,
      listParams.sort,
      listParams.stock_status,
    ],
  );

  const total = listQuery.data?.total ?? 0;
  const bulkSelection = useBulkSelection({
    listQueryKey: listParams,
    bulkQuery: engineQuery,
    total,
  });

  const meta = metaQuery.data;
  const bulkActions = useMemo(
    () =>
      resolveBulkActionsWithIcons(PRODUCTS_RESOURCE, meta?.bulk_actions ?? []),
    [meta?.bulk_actions],
  );

  const pageCount = useMemo(() => {
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listState.pagination.pageSize, total]);

  const hasFilters =
    Boolean(listParams.q) ||
    Boolean(listParams.branch) ||
    Boolean(listParams.stock_status) ||
    Boolean(listParams.is_active) ||
    Boolean(listParams.show_online);

  const canMove =
    can(permissions.tenant.products.stock) ||
    can(permissions.tenant.products.sell);

  const refresh = () => {
    void listQuery.refetch();
    void facetsQuery.refetch();
  };

  return (
    <EntityPage
      title={t("products.title")}
      description={t("products.description")}
      permission={permissions.tenant.products.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("products.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("products.title") },
      ]}
      actions={
        <>
          <Button asChild variant="outline">
            <Link href={routes.tenant.stockMovements.root(slug)}>
              <History className="size-4" />
              {t("products.movements.title")}
            </Link>
          </Button>
          {canMove ? (
            <Button
              variant="outline"
              onClick={() => setMovement({ open: true, product: null })}
            >
              <ArrowLeftRight className="size-4" />
              {t("products.actions.record_movement")}
            </Button>
          ) : null}
          <EntityCreateButton
            onClick={() => router.push(routes.tenant.products.create(slug))}
            label={t("products.actions.create")}
            permission={permissions.tenant.products.write}
          />
        </>
      }
    >
      <SelectionBanner
        selectedCount={bulkSelection.selectedCount}
        total={total}
        showSelectAll={bulkSelection.showSelectAllBanner}
        allMatchingSelected={bulkSelection.scope.mode === "all"}
        onSelectAllMatching={bulkSelection.selectAllMatching}
        onClearSelection={bulkSelection.clearSelection}
      />
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={openDetail}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("products.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("products.empty_filtered_title")
            : t("products.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("products.empty_filtered_description")
            : t("products.empty_description")
        }
        pageCount={pageCount}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: `tenant-products-v1`,
          rowSelection: true,
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource={PRODUCTS_RESOURCE}
              actions={bulkActions}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              onComplete={() => {
                bulkSelection.clearSelection();
                refresh();
              }}
            />
            <ResourceIOToolbar
              resource={PRODUCTS_RESOURCE}
              query={engineQuery}
              capabilities={meta?.capabilities}
              onImportComplete={refresh}
            />
            <EntityToolbar
              onRefresh={refresh}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />

      <ProductQuickEditDrawer
        product={quickEdit}
        onOpenChange={(open) => {
          if (!open) setQuickEdit(null);
        }}
      />

      <StockMovementDialog
        open={movement.open}
        defaults={{ product: movement.product }}
        onOpenChange={(open) =>
          setMovement((prev) => ({
            open,
            product: open ? prev.product : null,
          }))
        }
      />

      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        entityLabel={deleteTarget?.name ?? ""}
        isPending={deleteProduct.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return;
          deleteProduct.mutate(deleteTarget.uuid, {
            onSettled: () => setDeleteTarget(null),
          });
        }}
      />
    </EntityPage>
  );
}
