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
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import type { TenantRoleRowActionHandlers } from "@/features/tenant-roles/components/tenant-role-row-actions";
import { useTenantRolesColumns } from "@/features/tenant-roles/components/tenant-roles-columns";
import {
  useDeleteTenantRole,
  useTenantRolesList,
} from "@/features/tenant-roles/hooks/use-tenant-roles";
import {
  TENANT_ROLES_RESOURCE,
  type ListTenantRolesParams,
  type TenantRole,
} from "@/features/tenant-roles/types";
import { tenantRoleLabel } from "@/features/tenant/lib/role-label";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

/**
 * Tenant roles: the shared templates (read-only) and the organization's own
 * roles, each with a permission matrix.
 */
export function TenantRolesPage() {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const listState = useServerListState({
    initialSort: "",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, TENANT_ROLES_RESOURCE),
  });
  const [deleteTarget, setDeleteTarget] = useState<TenantRole | null>(null);

  const listParams = useMemo<ListTenantRolesParams>(() => {
    const template = filterValues(
      listState.columnFilters.find((filter) => filter.id === "is_template")
        ?.value,
    );
    const { limit, offset, q } = listState.params;
    return {
      limit,
      offset,
      q,
      is_template:
        template.length === 1 ? (template[0] as "true" | "false") : undefined,
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useTenantRolesList(listParams);
  const remove = useDeleteTenantRole();

  const openRole = useCallback(
    (role: TenantRole) =>
      router.push(routes.tenant.settings.roles.detail(slug, role.uuid)),
    [router, slug],
  );
  const handlers = useMemo<TenantRoleRowActionHandlers>(
    () => ({ onOpen: openRole, onDelete: setDeleteTarget }),
    [openRole],
  );
  const columns = useTenantRolesColumns(handlers);

  const total = listQuery.data?.total ?? 0;
  const pageCount = useMemo(() => {
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listState.pagination.pageSize, total]);
  const hasFilters = Boolean(listParams.q) || Boolean(listParams.is_template);

  return (
    <EntityPage
      title={t("tenant_roles.title")}
      description={t("tenant_roles.description")}
      permission={permissions.tenant.roles.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("tenant_roles.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("layout.section_tenant_settings"),
          href: routes.tenant.settings.root(slug),
        },
        { label: t("tenant_roles.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => router.push(routes.tenant.settings.roles.create(slug))}
          label={t("tenant_roles.actions.create")}
          permission={permissions.tenant.roles.write}
        />
      }
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={openRole}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("tenant_roles.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() => listQuery.refetch())}
        emptyTitle={
          hasFilters
            ? t("tenant_roles.empty_filtered_title")
            : t("tenant_roles.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("tenant_roles.empty_filtered_description")
            : t("tenant_roles.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{ persistKey: "tenant-roles-v1", viewMode: true }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void listQuery.refetch()}
            refreshDisabled={listQuery.isFetching}
          />
        }
      />

      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        softDelete={false}
        entityLabel={deleteTarget ? tenantRoleLabel(t, deleteTarget) : ""}
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
