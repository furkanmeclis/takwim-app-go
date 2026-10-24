"use client";

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
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BulkActionMenu,
  SelectionBanner,
  resolveBulkActionsWithIcons,
  useBulkSelection,
} from "@/features/bulk-engine";
import { ResourceIOToolbar } from "@/features/io";
import { useBundleColumns } from "@/features/service-bundles/components/bundle-columns";
import type { BundleRowActionHandlers } from "@/features/service-bundles/components/bundle-row-actions";
import { SERVICE_BUNDLES_RESOURCE } from "@/features/service-bundles/constants";
import {
  useDeleteServiceBundle,
  useServiceBundlesFacets,
  useServiceBundlesList,
  useServiceBundlesMeta,
  useUpdateServiceBundle,
} from "@/features/service-bundles/hooks/use-service-bundles";
import type {
  ListServiceBundlesParams,
  ServiceBundle,
} from "@/features/service-bundles/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

/** Both values of a yes / no facet selected = no filter. */
function oneBoolean(values: string[]) {
  return values.length === 1 ? (values[0] as "true" | "false") : undefined;
}

/**
 * Combined service bundles: server-side pagination / sort / search, faceted
 * filters, column visibility, row actions and row click → detail, export /
 * import and bulk activate / deactivate with the selection banner (TEC-985).
 */
export function BundlesPage() {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();

  const listState = useServerListState({
    initialSort: "",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, SERVICE_BUNDLES_RESOURCE),
  });
  const [deleteTarget, setDeleteTarget] = useState<ServiceBundle | null>(null);

  const listParams = useMemo<ListServiceBundlesParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    const branch = column("branch");
    return {
      ...listState.params,
      branch: branch.length ? branch.join(",") : undefined,
      is_active: oneBoolean(column("is_active")),
      show_online: oneBoolean(column("show_online")),
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useServiceBundlesList(listParams);
  const metaQuery = useServiceBundlesMeta();
  const facetsQuery = useServiceBundlesFacets();
  const update = useUpdateServiceBundle();
  const remove = useDeleteServiceBundle();

  const openDetail = useCallback(
    (bundle: ServiceBundle) =>
      router.push(routes.tenant.serviceBundles.detail(slug, bundle.uuid)),
    [router, slug],
  );

  const handlers = useMemo<BundleRowActionHandlers>(
    () => ({
      onView: openDetail,
      onEdit: (bundle) =>
        router.push(routes.tenant.serviceBundles.edit(slug, bundle.uuid)),
      onToggleActive: (bundle) =>
        update.mutate({
          uuid: bundle.uuid,
          body: { is_active: !bundle.is_active },
        }),
      onDelete: setDeleteTarget,
    }),
    [openDetail, router, slug, update],
  );

  const baseColumns = useBundleColumns({
    handlers,
    facets: facetsQuery.data,
  });
  const columns = useMemo(
    () => [createSelectColumnDef<ServiceBundle>(), ...baseColumns],
    [baseColumns],
  );

  // Same filters for export, import refresh and "all matching" bulk scope.
  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      branch: listParams.branch,
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
      resolveBulkActionsWithIcons(
        SERVICE_BUNDLES_RESOURCE,
        meta?.bulk_actions ?? [],
      ),
    [meta?.bulk_actions],
  );
  const pageCount = useMemo(() => {
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listState.pagination.pageSize, total]);

  const hasFilters =
    Boolean(listParams.q) ||
    Boolean(listParams.branch) ||
    Boolean(listParams.is_active) ||
    Boolean(listParams.show_online);

  const refresh = () => {
    void listQuery.refetch();
    void facetsQuery.refetch();
  };

  return (
    <EntityPage
      title={t("service_bundles.title")}
      description={t("service_bundles.description")}
      permission={permissions.tenant.services.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("service_bundles.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("service_bundles.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => router.push(routes.tenant.serviceBundles.create(slug))}
          label={t("service_bundles.actions.create")}
          permission={permissions.tenant.services.write}
        />
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
        errorDescription={t("service_bundles.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("service_bundles.empty_filtered_title")
            : t("service_bundles.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("service_bundles.empty_filtered_description")
            : t("service_bundles.empty_description")
        }
        pageCount={pageCount}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: "tenant-service-bundles-v1",
          rowSelection: true,
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource={SERVICE_BUNDLES_RESOURCE}
              actions={bulkActions}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              onComplete={() => {
                bulkSelection.clearSelection();
                refresh();
              }}
            />
            <ResourceIOToolbar
              resource={SERVICE_BUNDLES_RESOURCE}
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

      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        entityLabel={deleteTarget?.name ?? ""}
        isPending={remove.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return;
          remove.mutate(deleteTarget.uuid, {
            onSettled: () => setDeleteTarget(null),
          });
        }}
      />
    </EntityPage>
  );
}
