"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useRouter } from "next/navigation";
import { useCallback, useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { routes } from "@/config/routes";
import { useCashAmount } from "@/features/cash/lib/format";
import { ResourceIOToolbar } from "@/features/io";
import {
  CUSTOMER_PACKAGE_STATES,
  CUSTOMER_PACKAGES_RESOURCE,
  STATE_LABEL_KEYS,
  STATE_TONES,
} from "@/features/session-packages/constants";
import {
  useCustomerPackageFacets,
  useCustomerPackagesMeta,
  useCustomerPackages,
} from "@/features/session-packages/hooks/use-session-packages";
import { sessionsLabel } from "@/features/session-packages/lib/format";
import type {
  CustomerPackage,
  CustomerPackageFacets,
  ListCustomerPackagesParams,
} from "@/features/session-packages/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

function joined(values: string[]) {
  return values.length ? values.join(",") : undefined;
}

function useCustomerPackageColumns({
  showCustomer,
  facets,
}: {
  showCustomer: boolean;
  facets?: CustomerPackageFacets;
}) {
  const { t, locale } = useLocale();
  const money = useCashAmount();

  return useMemo(() => {
    const columns = [
      ...(showCustomer
        ? [
            createColumn<CustomerPackage>({
              id: "customer",
              accessorFn: (row) => row.customer.name,
              labelKey: "session_packages.customer.columns.customer",
              enableSorting: false,
              cell: ({ row }) => (
                <div className="min-w-0">
                  <p className="truncate font-medium">
                    {row.original.customer.name}
                  </p>
                  {row.original.customer.phone ? (
                    <p className="text-muted-foreground truncate text-xs">
                      {row.original.customer.phone}
                    </p>
                  ) : null}
                </div>
              ),
            }),
          ]
        : []),
      createColumn<CustomerPackage>({
        accessorKey: "name",
        labelKey: "session_packages.customer.columns.name",
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <div className="min-w-0">
            <p className="truncate font-medium">{row.original.name}</p>
            <p className="text-muted-foreground truncate text-xs">
              {row.original.service_name}
            </p>
          </div>
        ),
      }),
      createColumn<CustomerPackage>({
        id: "package",
        accessorFn: (row) => row.package_uuid ?? "",
        labelKey: "session_packages.customer.columns.package",
        enableSorting: false,
        defaultHidden: true,
        filterVariant: "faceted",
        filterOptions: (facets?.package ?? []).map((f) => ({
          value: f.value,
          label: f.label,
        })),
        cell: ({ row }) => row.original.name,
      }),
      createColumn<CustomerPackage>({
        id: "sessions",
        accessorFn: (row) => row.used,
        labelKey: "session_packages.customer.columns.sessions",
        enableSorting: false,
        cell: ({ row }) => (
          <div className="flex flex-col">
            <span className="tabular-nums">
              {sessionsLabel(row.original.used, row.original.total_sessions)}
            </span>
            <span className="text-muted-foreground text-xs">
              {t("session_packages.customer.sessions_hint", {
                remaining: row.original.remaining,
                held: row.original.held,
              })}
            </span>
          </div>
        ),
      }),
      createColumn<CustomerPackage>({
        accessorKey: "remaining_amount",
        labelKey: "session_packages.customer.columns.remaining_amount",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {money(row.original.remaining_amount, row.original.currency)}
          </span>
        ),
      }),
      createColumn<CustomerPackage>({
        accessorKey: "price",
        labelKey: "session_packages.customer.columns.price",
        cell: ({ row }) => (
          <span className="tabular-nums">
            {money(row.original.price, row.original.currency)}
          </span>
        ),
      }),
      createColumn<CustomerPackage>({
        accessorKey: "sold_on",
        labelKey: "session_packages.customer.columns.sold_on",
        cell: ({ row }) => date(row.original.sold_on, undefined, locale),
      }),
      createColumn<CustomerPackage>({
        accessorKey: "expires_on",
        labelKey: "session_packages.customer.columns.expires_on",
        cell: ({ row }) =>
          row.original.expires_on
            ? date(row.original.expires_on, undefined, locale)
            : t("session_packages.validity_none"),
      }),
      createColumn<CustomerPackage>({
        accessorKey: "state",
        labelKey: "session_packages.customer.columns.state",
        enableSorting: false,
        gridSecondary: true,
        filterVariant: "faceted",
        filterOptions: CUSTOMER_PACKAGE_STATES.map((value) => ({
          value,
          label: t(STATE_LABEL_KEYS[value]),
        })),
        cell: ({ row }) => (
          <StatusChip
            label={t(STATE_LABEL_KEYS[row.original.state])}
            tone={STATE_TONES[row.original.state]}
          />
        ),
      }),
      createColumn<CustomerPackage>({
        id: "branch",
        accessorFn: (row) => row.branch.name,
        labelKey: "session_packages.customer.columns.branch",
        enableSorting: false,
        defaultHidden: true,
        filterVariant: "faceted",
        filterOptions: (facets?.branch ?? []).map((f) => ({
          value: f.value,
          label: f.label,
        })),
        cell: ({ row }) => row.original.branch.name,
      }),
      createColumn<CustomerPackage>({
        accessorKey: "created_at",
        labelKey: "session_packages.customer.columns.created_at",
        defaultHidden: true,
        cell: ({ row }) => datetime(row.original.created_at, undefined, locale),
      }),
    ];
    return columns as ColumnDef<CustomerPackage, unknown>[];
  }, [facets, locale, money, showCustomer, t]);
}

/**
 * Packages sold to customers with their balance (server list). On the
 * customer page it is narrowed to one customer and leaves the customer
 * column out. A row opens the package with its sessions.
 */
export function CustomerPackagesTable({
  customerUuid,
  persistKey,
}: {
  customerUuid?: string;
  persistKey: string;
}) {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const listState = useServerListState({
    initialSort: "-sold_on",
    initialPageSize: customerUuid ? 10 : 20,
    realtime: tenantLiveList(organization, CUSTOMER_PACKAGES_RESOURCE),
  });
  const facets = useCustomerPackageFacets();

  const params = useMemo<ListCustomerPackagesParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    return {
      ...listState.params,
      customer: customerUuid,
      state: joined(column("state")),
      package: joined(column("package")),
      branch: joined(column("branch")),
    };
  }, [customerUuid, listState.columnFilters, listState.params]);

  const query = useCustomerPackages(params);
  const meta = useCustomerPackagesMeta();
  // The list's filters for the export (TEC-984).
  const exportQuery = useMemo(
    () => ({
      q: params.q,
      sort: params.sort,
      state: params.state,
      package: params.package,
      branch: params.branch,
    }),
    [params.branch, params.package, params.q, params.sort, params.state],
  );
  const columns = useCustomerPackageColumns({
    showCustomer: !customerUuid,
    facets: facets.data,
  });
  const openDetail = useCallback(
    (row: CustomerPackage) =>
      router.push(routes.tenant.packages.detail(slug, row.uuid)),
    [router, slug],
  );
  const total = query.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );
  const hasFilters = Boolean(
    params.q || params.state || params.package || params.branch,
  );

  return (
    <EntityTable
      columns={columns}
      data={query.data?.items ?? []}
      getRowId={(row) => row.uuid}
      onRowClick={openDetail}
      isLoading={query.isLoading}
      isError={query.isError}
      errorDescription={t("session_packages.customer.load_failed")}
      onRetry={() => void query.refetch()}
      realtime={listState.realtimeFor(() =>
        Promise.all([query.refetch(), facets.refetch()]),
      )}
      emptyTitle={
        hasFilters
          ? t("session_packages.empty_filtered_title")
          : t("session_packages.customer.empty_title")
      }
      emptyDescription={
        hasFilters
          ? t("session_packages.empty_filtered_description")
          : t("session_packages.customer.empty_description")
      }
      pageCount={pageCount}
      state={listState.tableState}
      features={{ persistKey, rowSelection: false, viewMode: true }}
      toolbarExtra={
        <>
          {customerUuid ? null : (
            <ResourceIOToolbar
              resource={CUSTOMER_PACKAGES_RESOURCE}
              query={exportQuery}
              capabilities={meta.data?.capabilities}
            />
          )}
          <EntityToolbar
            onRefresh={() => {
              void query.refetch();
              void facets.refetch();
            }}
            refreshDisabled={query.isFetching}
          />
        </>
      }
    />
  );
}
