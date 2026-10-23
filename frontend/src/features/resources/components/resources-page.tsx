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
import { useResourceColumns } from "@/features/resources/components/resource-columns";
import { ResourceQuickEditDrawer } from "@/features/resources/components/resource-quick-edit-drawer";
import type { ResourceRowActionHandlers } from "@/features/resources/components/resource-row-actions";
import { RESOURCES_RESOURCE } from "@/features/resources/constants";
import {
  useDeleteResource,
  useResourcesFacets,
  useResourcesList,
  useResourcesMeta,
  useUpdateResource,
} from "@/features/resources/hooks/use-resources";
import type { ListResourcesParams, Resource } from "@/features/resources/types";
import { useSector } from "@/features/sectors/hooks/use-sector";
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

function joined(values: string[]) {
  return values.length ? values.join(",") : undefined;
}

/**
 * Tenant list standard: server-side pagination / sort / search, faceted
 * filters, column visibility, row actions, row click → detail, quick-edit
 * drawer, export / import, bulk actions with the selection banner.
 */
export function ResourcesPage() {
  const { t } = useLocale();
  const { term } = useSector();
  const { slug, organization } = useTenant();
  const router = useRouter();

  const listState = useServerListState({
    initialSort: "",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, RESOURCES_RESOURCE),
  });

  const [quickEdit, setQuickEdit] = useState<Resource | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Resource | null>(null);

  const listParams = useMemo<ListResourcesParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    return {
      ...listState.params,
      branch: joined(column("branch")),
      kind: joined(column("kind")),
      zone: joined(column("zone")),
      is_active: oneBoolean(column("is_active")),
      show_online: oneBoolean(column("show_online")),
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useResourcesList(listParams);
  const metaQuery = useResourcesMeta();
  const facetsQuery = useResourcesFacets();
  const updateResource = useUpdateResource();
  const deleteResource = useDeleteResource();

  const openDetail = useCallback(
    (resource: Resource) =>
      router.push(routes.tenant.resources.detail(slug, resource.uuid)),
    [router, slug],
  );

  const handlers = useMemo<ResourceRowActionHandlers>(
    () => ({
      onView: openDetail,
      onQuickEdit: setQuickEdit,
      onEdit: (resource) =>
        router.push(routes.tenant.resources.edit(slug, resource.uuid)),
      onToggleActive: (resource) =>
        updateResource.mutate({
          uuid: resource.uuid,
          body: { is_active: !resource.is_active },
        }),
      onDelete: setDeleteTarget,
    }),
    [openDetail, router, slug, updateResource],
  );

  const baseColumns = useResourceColumns({
    handlers,
    facets: facetsQuery.data,
  });
  const columns = useMemo(
    () => [createSelectColumnDef<Resource>(), ...baseColumns],
    [baseColumns],
  );

  // Same filters for export, import refresh and "all matching" bulk scope.
  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      branch: listParams.branch,
      kind: listParams.kind,
      zone: listParams.zone,
      is_active: listParams.is_active,
      show_online: listParams.show_online,
      sort: listParams.sort,
    }),
    [
      listParams.branch,
      listParams.is_active,
      listParams.kind,
      listParams.q,
      listParams.show_online,
      listParams.sort,
      listParams.zone,
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
      resolveBulkActionsWithIcons(RESOURCES_RESOURCE, meta?.bulk_actions ?? []),
    [meta?.bulk_actions],
  );

  const pageCount = useMemo(() => {
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listState.pagination.pageSize, total]);

  const hasFilters =
    Boolean(listParams.q) ||
    Boolean(listParams.branch) ||
    Boolean(listParams.kind) ||
    Boolean(listParams.zone) ||
    Boolean(listParams.is_active) ||
    Boolean(listParams.show_online);

  const refresh = () => {
    void listQuery.refetch();
    void facetsQuery.refetch();
  };

  return (
    <EntityPage
      title={t("resources.title")}
      description={t("resources.description", { resource: term("resource") })}
      permission={permissions.tenant.resources.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("resources.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("resources.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => router.push(routes.tenant.resources.create(slug))}
          label={t("resources.actions.create")}
          permission={permissions.tenant.resources.write}
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
        errorDescription={t("resources.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("resources.empty_filtered_title")
            : t("resources.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("resources.empty_filtered_description")
            : t("resources.empty_description", { resource: term("resource") })
        }
        pageCount={pageCount}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: `tenant-resources-v1`,
          rowSelection: true,
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource={RESOURCES_RESOURCE}
              actions={bulkActions}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              onComplete={() => {
                bulkSelection.clearSelection();
                refresh();
              }}
            />
            <ResourceIOToolbar
              resource={RESOURCES_RESOURCE}
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

      <ResourceQuickEditDrawer
        resource={quickEdit}
        onOpenChange={(open) => {
          if (!open) setQuickEdit(null);
        }}
      />

      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        entityLabel={deleteTarget?.name ?? ""}
        isPending={deleteResource.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return;
          deleteResource.mutate(deleteTarget.uuid, {
            onSettled: () => setDeleteTarget(null),
          });
        }}
      />
    </EntityPage>
  );
}
