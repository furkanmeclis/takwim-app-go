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
import { useSector } from "@/features/sectors/hooks/use-sector";
import { StaffQuickEditDrawer } from "@/features/staff/components/staff-quick-edit-drawer";
import type { StaffRowActionHandlers } from "@/features/staff/components/staff-row-actions";
import { useStaffColumns } from "@/features/staff/components/staff-columns";
import { STAFF_RESOURCE } from "@/features/staff/constants";
import {
  useDeleteStaff,
  useStaffFacets,
  useStaffList,
  useStaffMeta,
  useUpdateStaff,
} from "@/features/staff/hooks/use-staff";
import type { ListStaffParams, Staff } from "@/features/staff/types";
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
 * Tenant list standard: server-side pagination / sort / search, faceted
 * filters, column visibility, row actions, row click → detail, quick-edit
 * drawer, export / import, bulk actions with the selection banner.
 */
export function StaffPage() {
  const { t } = useLocale();
  const { term } = useSector();
  const { slug, organization } = useTenant();
  const router = useRouter();

  const listState = useServerListState({
    initialSort: "name",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, STAFF_RESOURCE),
  });

  const [quickEdit, setQuickEdit] = useState<Staff | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Staff | null>(null);

  const listParams = useMemo<ListStaffParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    const branches = column("branch");
    return {
      ...listState.params,
      branch: branches.length ? branches.join(",") : undefined,
      is_active: oneBoolean(column("is_active")),
      show_online: oneBoolean(column("show_online")),
      has_account: oneBoolean(column("has_account")),
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useStaffList(listParams);
  const metaQuery = useStaffMeta();
  const facetsQuery = useStaffFacets();
  const updateStaff = useUpdateStaff();
  const deleteStaff = useDeleteStaff();

  const openDetail = useCallback(
    (staff: Staff) => router.push(routes.tenant.staff.detail(slug, staff.uuid)),
    [router, slug],
  );

  const handlers = useMemo<StaffRowActionHandlers>(
    () => ({
      onView: openDetail,
      onQuickEdit: setQuickEdit,
      onEdit: (staff) =>
        router.push(routes.tenant.staff.edit(slug, staff.uuid)),
      onToggleActive: (staff) =>
        updateStaff.mutate({
          uuid: staff.uuid,
          body: { is_active: !staff.is_active },
        }),
      onDelete: setDeleteTarget,
    }),
    [openDetail, router, slug, updateStaff],
  );

  const baseColumns = useStaffColumns({
    handlers,
    facets: facetsQuery.data,
  });
  const columns = useMemo(
    () => [createSelectColumnDef<Staff>(), ...baseColumns],
    [baseColumns],
  );

  // Same filters for export, import refresh and "all matching" bulk scope.
  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      branch: listParams.branch,
      is_active: listParams.is_active,
      show_online: listParams.show_online,
      has_account: listParams.has_account,
      sort: listParams.sort,
    }),
    [
      listParams.branch,
      listParams.has_account,
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
    () => resolveBulkActionsWithIcons(STAFF_RESOURCE, meta?.bulk_actions ?? []),
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
    Boolean(listParams.show_online) ||
    Boolean(listParams.has_account);

  const refresh = () => {
    void listQuery.refetch();
    void facetsQuery.refetch();
  };

  return (
    <EntityPage
      title={t("staff.title")}
      description={t("staff.description", { staff: term("staff") })}
      permission={permissions.tenant.staff.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("staff.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("staff.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => router.push(routes.tenant.staff.create(slug))}
          label={t("staff.actions.create")}
          permission={permissions.tenant.staff.write}
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
        errorDescription={t("staff.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters ? t("staff.empty_filtered_title") : t("staff.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("staff.empty_filtered_description")
            : t("staff.empty_description")
        }
        pageCount={pageCount}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: `tenant-staff-v1`,
          rowSelection: true,
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource={STAFF_RESOURCE}
              actions={bulkActions}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              onComplete={() => {
                bulkSelection.clearSelection();
                refresh();
              }}
            />
            <ResourceIOToolbar
              resource={STAFF_RESOURCE}
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

      <StaffQuickEditDrawer
        staff={quickEdit}
        onOpenChange={(open) => {
          if (!open) setQuickEdit(null);
        }}
      />

      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        entityLabel={deleteTarget?.name ?? ""}
        isPending={deleteStaff.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return;
          deleteStaff.mutate(deleteTarget.uuid, {
            onSettled: () => setDeleteTarget(null),
          });
        }}
      />
    </EntityPage>
  );
}
