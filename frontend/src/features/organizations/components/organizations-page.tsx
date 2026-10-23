"use client";

import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
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
import {
  useOrganizationsColumns,
  type OrganizationRowActionHandlers,
} from "@/features/organizations/components/organizations-columns";
import { ORGANIZATIONS_RESOURCE } from "@/features/organizations/constants";
import {
  useDeleteOrganization,
  useOrganizationsList,
  useOrganizationsMeta,
  useSetOrganizationStatus,
} from "@/features/organizations/hooks/use-organizations";
import type {
  ListOrganizationsParams,
  OrganizationSummary,
} from "@/features/organizations/types";
import { useLocale } from "@/providers/locale-provider";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

/**
 * Platform organizations (TEC-670): server-side search / sort / faceted
 * filters (status, access state, access window), owner and counts, export,
 * bulk activate / suspend with the selection banner, live updates on
 * `resource:platform.organizations`.
 */
export function OrganizationsPage() {
  const { t } = useLocale();
  const router = useRouter();
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
    realtime: { resource: ORGANIZATIONS_RESOURCE },
  });
  const [deleteTarget, setDeleteTarget] = useState<OrganizationSummary | null>(
    null,
  );
  const [statusTarget, setStatusTarget] = useState<OrganizationSummary | null>(
    null,
  );

  const listParams = useMemo<ListOrganizationsParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    const joined = (id: string) => {
      const values = column(id);
      return values.length ? values.join(",") : undefined;
    };
    return {
      ...listState.params,
      status: joined("status"),
      access: joined("access"),
      window: joined("window"),
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useOrganizationsList(listParams);
  const metaQuery = useOrganizationsMeta();
  const setStatus = useSetOrganizationStatus();
  const remove = useDeleteOrganization();

  const openDetail = useCallback(
    (org: OrganizationSummary) =>
      router.push(routes.platform.organizations.detail(org.uuid)),
    [router],
  );
  const handlers = useMemo<OrganizationRowActionHandlers>(
    () => ({
      onView: openDetail,
      onEdit: (org) =>
        router.push(routes.platform.organizations.edit(org.uuid)),
      onToggleStatus: setStatusTarget,
      onDelete: setDeleteTarget,
    }),
    [openDetail, router],
  );
  const baseColumns = useOrganizationsColumns(handlers);
  const columns = useMemo(
    () => [createSelectColumnDef<OrganizationSummary>(), ...baseColumns],
    [baseColumns],
  );

  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      status: listParams.status,
      access: listParams.access,
      window: listParams.window,
      sort: listParams.sort,
    }),
    [
      listParams.access,
      listParams.q,
      listParams.sort,
      listParams.status,
      listParams.window,
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
        ORGANIZATIONS_RESOURCE,
        meta?.bulk_actions ?? [],
      ),
    [meta?.bulk_actions],
  );
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );
  const hasFilters =
    Boolean(listParams.q) ||
    Boolean(listParams.status) ||
    Boolean(listParams.access) ||
    Boolean(listParams.window);
  const suspending = statusTarget?.status === "active";

  return (
    <EntityPage
      title={t("organizations.title")}
      description={t("organizations.description")}
      permission={permissions.organizations.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("organizations.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("organizations.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => router.push(routes.platform.organizations.create)}
          label={t("organizations.actions.create")}
          permission={permissions.organizations.write}
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
        errorDescription={t("organizations.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() => listQuery.refetch())}
        emptyTitle={
          hasFilters
            ? t("organizations.empty_filtered_title")
            : t("organizations.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("organizations.empty_filtered_description")
            : t("organizations.empty_description")
        }
        pageCount={pageCount}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: "platform-organizations-v1",
          rowSelection: true,
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource={ORGANIZATIONS_RESOURCE}
              actions={bulkActions}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              onComplete={() => {
                bulkSelection.clearSelection();
                void listQuery.refetch();
              }}
            />
            <ResourceIOToolbar
              resource={ORGANIZATIONS_RESOURCE}
              query={engineQuery}
              capabilities={meta?.capabilities}
            />
            <EntityToolbar
              onRefresh={() => void listQuery.refetch()}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />

      <ConfirmDialog
        open={Boolean(statusTarget)}
        title={
          suspending
            ? t("organizations.status_confirm.suspend_title", {
                name: statusTarget?.name ?? "",
              })
            : t("organizations.status_confirm.activate_title", {
                name: statusTarget?.name ?? "",
              })
        }
        description={
          suspending
            ? t("organizations.status_confirm.suspend_description")
            : t("organizations.status_confirm.activate_description")
        }
        confirmLabel={
          suspending
            ? t("organizations.actions.suspend")
            : t("organizations.actions.activate")
        }
        variant={suspending ? "destructive" : "default"}
        isPending={setStatus.isPending}
        onCancel={() => setStatusTarget(null)}
        onConfirm={() => {
          if (!statusTarget) return;
          setStatus.mutate(
            {
              uuid: statusTarget.uuid,
              status: suspending ? "suspended" : "active",
            },
            { onSettled: () => setStatusTarget(null) },
          );
        }}
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
