"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ActivityEventDrawer } from "@/features/tenant-activity/components/activity-event-drawer";
import {
  useTenantActivity,
  useTenantActivityFacets,
} from "@/features/tenant-activity/hooks/use-tenant-activity";
import {
  activityActionLabel,
  activityActorName,
  activityChanges,
  activityResourceLabel,
} from "@/features/tenant-activity/lib/labels";
import type {
  ListTenantActivityParams,
  TenantActivityEvent,
} from "@/features/tenant-activity/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime, relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

/**
 * İşlem geçmişi (requirements §3): who changed what, when and in which
 * branch, newest first. Faceted filters on resource and action; the header
 * branch picker narrows it like every list. A row opens the before / after.
 */
export function TenantActivityPage() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const [selected, setSelected] = useState<TenantActivityEvent | null>(null);

  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
  });

  const listParams = useMemo<ListTenantActivityParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    const resources = column("resource");
    const actions = column("action");
    return {
      ...listState.params,
      resource: resources.length ? resources.join(",") : undefined,
      action: actions.length ? actions.join(",") : undefined,
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useTenantActivity(listParams);
  const facetsQuery = useTenantActivityFacets();
  const facets = facetsQuery.data;

  const columns = useMemo(
    () =>
      [
        createColumn<TenantActivityEvent>({
          accessorKey: "created_at",
          labelKey: "activity.columns.created_at",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <time
              dateTime={row.original.created_at}
              title={relativeDatetime(row.original.created_at, locale)}
              className="text-sm whitespace-nowrap tabular-nums"
            >
              {datetime(row.original.created_at, undefined, locale)}
            </time>
          ),
        }),
        createColumn<TenantActivityEvent>({
          accessorKey: "action",
          labelKey: "activity.columns.action",
          enableSorting: false,
          gridPrimary: true,
          filterVariant: "faceted",
          filterOptions: (facets?.actions ?? []).map((facet) => ({
            value: facet.value,
            label: `${activityActionLabel(t, facet.value)} (${facet.count})`,
          })),
          cell: ({ row }) => {
            const changed = activityChanges(row.original).length;
            return (
              <div className="min-w-0">
                <p className="truncate font-medium">
                  {activityActionLabel(t, row.original.action)}
                </p>
                {changed > 0 ? (
                  <p className="text-muted-foreground text-xs">
                    {t("activity.history.changed_fields", { count: changed })}
                  </p>
                ) : null}
              </div>
            );
          },
        }),
        createColumn<TenantActivityEvent>({
          accessorKey: "resource",
          labelKey: "activity.columns.resource",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: (facets?.resources ?? []).map((facet) => ({
            value: facet.value,
            label: `${activityResourceLabel(t, facet.value)} (${facet.count})`,
          })),
          cell: ({ row }) => activityResourceLabel(t, row.original.resource),
        }),
        createColumn<TenantActivityEvent>({
          id: "branch",
          accessorFn: (row) => row.branch?.name ?? "",
          labelKey: "activity.history.branch",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.branch?.name ?? (
              <span className="text-muted-foreground">
                {t("activity.history.whole_business")}
              </span>
            ),
        }),
        createColumn<TenantActivityEvent>({
          id: "actor",
          accessorFn: (row) => activityActorName(row) ?? "",
          labelKey: "activity.columns.actor",
          enableSorting: false,
          cell: ({ row }) =>
            activityActorName(row.original) ?? (
              <span className="text-muted-foreground">
                {t("activity.history.system")}
              </span>
            ),
        }),
      ] as ColumnDef<TenantActivityEvent, unknown>[],
    [facets?.actions, facets?.resources, locale, t],
  );

  const total = listQuery.data?.total ?? 0;
  const pageCount = useMemo(() => {
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listState.pagination.pageSize, total]);

  const hasFilters =
    Boolean(listParams.q) ||
    Boolean(listParams.resource) ||
    Boolean(listParams.action);

  const refresh = () => {
    void listQuery.refetch();
    void facetsQuery.refetch();
  };

  return (
    <EntityPage
      title={t("activity.history.title")}
      description={t("activity.history.description")}
      permission={permissions.tenant.activity.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("activity.history.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("activity.history.title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={setSelected}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("activity.history.load_failed")}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={
          hasFilters
            ? t("activity.history.empty_filtered_title")
            : t("activity.history.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("activity.history.empty_filtered_description")
            : t("activity.history.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-activity-v1",
          rowSelection: false,
          viewMode: true,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={refresh}
            refreshDisabled={listQuery.isFetching}
          />
        }
      />

      <ActivityEventDrawer
        event={selected}
        onOpenChange={(open) => {
          if (!open) setSelected(null);
        }}
      />
    </EntityPage>
  );
}
