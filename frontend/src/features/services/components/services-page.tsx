"use client";

import { FolderTree } from "lucide-react";
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
import { ServiceCategoriesDialog } from "@/features/services/components/service-categories-dialog";
import { useServiceColumns } from "@/features/services/components/service-columns";
import { ServiceQuickEditDrawer } from "@/features/services/components/service-quick-edit-drawer";
import type { ServiceRowActionHandlers } from "@/features/services/components/service-row-actions";
import { SERVICES_RESOURCE } from "@/features/services/constants";
import {
  useDeleteService,
  useServicesFacets,
  useServicesList,
  useServicesMeta,
  useUpdateService,
} from "@/features/services/hooks/use-services";
import type { ListServicesParams, Service } from "@/features/services/types";
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
export function ServicesPage() {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();

  const listState = useServerListState({
    initialSort: "",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, SERVICES_RESOURCE),
  });

  const [quickEdit, setQuickEdit] = useState<Service | null>(null);
  const [categoriesOpen, setCategoriesOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Service | null>(null);

  const listParams = useMemo<ListServicesParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    return {
      ...listState.params,
      category: joined(column("category")),
      branch: joined(column("branch")),
      payment_mode: joined(column("payment_mode")),
      is_active: oneBoolean(column("is_active")),
      show_online: oneBoolean(column("show_online")),
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useServicesList(listParams);
  const metaQuery = useServicesMeta();
  const facetsQuery = useServicesFacets();
  const updateService = useUpdateService();
  const deleteService = useDeleteService();

  const openDetail = useCallback(
    (service: Service) =>
      router.push(routes.tenant.services.detail(slug, service.uuid)),
    [router, slug],
  );

  const handlers = useMemo<ServiceRowActionHandlers>(
    () => ({
      onView: openDetail,
      onQuickEdit: setQuickEdit,
      onEdit: (service) =>
        router.push(routes.tenant.services.edit(slug, service.uuid)),
      onToggleActive: (service) =>
        updateService.mutate({
          uuid: service.uuid,
          body: { is_active: !service.is_active },
        }),
      onDelete: setDeleteTarget,
    }),
    [openDetail, router, slug, updateService],
  );

  const baseColumns = useServiceColumns({
    handlers,
    facets: facetsQuery.data,
  });
  const columns = useMemo(
    () => [createSelectColumnDef<Service>(), ...baseColumns],
    [baseColumns],
  );

  // Same filters for export, import refresh and "all matching" bulk scope.
  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      category: listParams.category,
      branch: listParams.branch,
      payment_mode: listParams.payment_mode,
      is_active: listParams.is_active,
      show_online: listParams.show_online,
      sort: listParams.sort,
    }),
    [
      listParams.branch,
      listParams.category,
      listParams.is_active,
      listParams.payment_mode,
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
      resolveBulkActionsWithIcons(SERVICES_RESOURCE, meta?.bulk_actions ?? []),
    [meta?.bulk_actions],
  );

  const pageCount = useMemo(() => {
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listState.pagination.pageSize, total]);

  const hasFilters =
    Boolean(listParams.q) ||
    Boolean(listParams.category) ||
    Boolean(listParams.branch) ||
    Boolean(listParams.payment_mode) ||
    Boolean(listParams.is_active) ||
    Boolean(listParams.show_online);

  const refresh = () => {
    void listQuery.refetch();
    void facetsQuery.refetch();
  };

  return (
    <EntityPage
      title={t("services.title")}
      description={t("services.description")}
      permission={permissions.tenant.services.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("services.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("services.title") },
      ]}
      actions={
        <>
          <Button variant="outline" onClick={() => setCategoriesOpen(true)}>
            <FolderTree className="size-4" />
            {t("services.categories.manage")}
          </Button>
          <EntityCreateButton
            onClick={() => router.push(routes.tenant.services.create(slug))}
            label={t("services.actions.create")}
            permission={permissions.tenant.services.write}
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
        errorDescription={t("services.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("services.empty_filtered_title")
            : t("services.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("services.empty_filtered_description")
            : t("services.empty_description")
        }
        pageCount={pageCount}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: `tenant-services-v1`,
          rowSelection: true,
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource={SERVICES_RESOURCE}
              actions={bulkActions}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              onComplete={() => {
                bulkSelection.clearSelection();
                refresh();
              }}
            />
            <ResourceIOToolbar
              resource={SERVICES_RESOURCE}
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

      <ServiceQuickEditDrawer
        service={quickEdit}
        onOpenChange={(open) => {
          if (!open) setQuickEdit(null);
        }}
      />

      <ServiceCategoriesDialog
        open={categoriesOpen}
        onOpenChange={setCategoriesOpen}
      />

      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        entityLabel={deleteTarget?.name ?? ""}
        isPending={deleteService.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return;
          deleteService.mutate(deleteTarget.uuid, {
            onSettled: () => setDeleteTarget(null),
          });
        }}
      />
    </EntityPage>
  );
}
