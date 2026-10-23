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
import { BranchQuickEditDrawer } from "@/features/branches/components/branch-quick-edit-drawer";
import type { BranchRowActionHandlers } from "@/features/branches/components/branch-row-actions";
import { useBranchesColumns } from "@/features/branches/components/branches-columns";
import { BRANCHES_RESOURCE } from "@/features/branches/constants";
import {
  useBranchFacets,
  useBranchesList,
  useBranchesMeta,
  useDeleteBranch,
  useUpdateBranch,
} from "@/features/branches/hooks/use-branches";
import type { Branch, ListBranchesParams } from "@/features/branches/types";
import { ResourceIOToolbar } from "@/features/io";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

/**
 * Tenant list standard (TEC-669): server-side pagination / sort / search,
 * faceted filters, column visibility, row actions, row click → detail,
 * quick-edit drawer, export / import, bulk actions with the selection banner.
 */
export function BranchesPage() {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();

  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, BRANCHES_RESOURCE),
  });

  const [quickEdit, setQuickEdit] = useState<Branch | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Branch | null>(null);

  const listParams = useMemo<ListBranchesParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    const cities = column("city");
    const active = column("is_active");
    return {
      ...listState.params,
      city: cities.length ? cities.join(",") : undefined,
      // Both values selected = no filter.
      is_active:
        active.length === 1 ? (active[0] as "true" | "false") : undefined,
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useBranchesList(listParams);
  const metaQuery = useBranchesMeta();
  const facetsQuery = useBranchFacets();
  const updateBranch = useUpdateBranch();
  const deleteBranch = useDeleteBranch();

  const openDetail = useCallback(
    (branch: Branch) =>
      router.push(routes.tenant.branches.detail(slug, branch.uuid)),
    [router, slug],
  );

  const handlers = useMemo<BranchRowActionHandlers>(
    () => ({
      onView: openDetail,
      onQuickEdit: setQuickEdit,
      onEdit: (branch) =>
        router.push(routes.tenant.branches.edit(slug, branch.uuid)),
      onToggleActive: (branch) =>
        updateBranch.mutate({
          uuid: branch.uuid,
          body: { is_active: !branch.is_active },
        }),
      onDelete: setDeleteTarget,
    }),
    [openDetail, router, slug, updateBranch],
  );

  const baseColumns = useBranchesColumns({
    handlers,
    facets: facetsQuery.data,
  });
  const columns = useMemo(
    () => [createSelectColumnDef<Branch>(), ...baseColumns],
    [baseColumns],
  );

  // Same filters for export, import refresh and "all matching" bulk scope.
  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      city: listParams.city,
      is_active: listParams.is_active,
      sort: listParams.sort,
    }),
    [listParams.city, listParams.is_active, listParams.q, listParams.sort],
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
      resolveBulkActionsWithIcons(BRANCHES_RESOURCE, meta?.bulk_actions ?? []),
    [meta?.bulk_actions],
  );

  const pageCount = useMemo(() => {
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listState.pagination.pageSize, total]);

  const hasFilters =
    Boolean(listParams.q) ||
    Boolean(listParams.city) ||
    Boolean(listParams.is_active);

  const refresh = () => {
    void listQuery.refetch();
    void facetsQuery.refetch();
  };

  return (
    <EntityPage
      title={t("branches.title")}
      description={t("branches.description")}
      permission={permissions.tenant.branches.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("branches.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("branches.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => router.push(routes.tenant.branches.create(slug))}
          label={t("branches.actions.create")}
          permission={permissions.tenant.branches.write}
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
        errorDescription={t("branches.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("branches.empty_filtered_title")
            : t("branches.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("branches.empty_filtered_description")
            : t("branches.empty_description")
        }
        pageCount={pageCount}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: `tenant-branches-v1`,
          rowSelection: true,
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource={BRANCHES_RESOURCE}
              actions={bulkActions}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              onComplete={() => {
                bulkSelection.clearSelection();
                refresh();
              }}
            />
            <ResourceIOToolbar
              resource={BRANCHES_RESOURCE}
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

      <BranchQuickEditDrawer
        branch={quickEdit}
        onOpenChange={(open) => {
          if (!open) setQuickEdit(null);
        }}
      />

      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        entityLabel={
          deleteTarget ? `${deleteTarget.code} · ${deleteTarget.name}` : ""
        }
        isPending={deleteBranch.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return;
          deleteBranch.mutate(deleteTarget.uuid, {
            onSettled: () => setDeleteTarget(null),
          });
        }}
      />
    </EntityPage>
  );
}
