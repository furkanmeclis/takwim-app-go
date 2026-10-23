"use client";

import { useRouter } from "next/navigation";
import { useMemo, type ReactNode } from "react";

import {
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { routes } from "@/config/routes";
import { useCustomerReservationColumns } from "@/features/customer-panel/components/customer-reservation-columns";
import { useCustomerList } from "@/features/customer-panel/hooks/use-customer-panel";
import type {
  CustomerListParams,
  CustomerScope,
} from "@/features/customer-panel/types";
import { useLocale } from "@/providers/locale-provider";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

type Props = {
  scope: CustomerScope;
  /** Payments view: `/v1/customer/payments` and the money columns. */
  money?: boolean;
  business?: string;
  emptyTitle: string;
  emptyDescription: string;
  toolbarExtra?: ReactNode;
  persistKey: string;
};

/**
 * The person's visits (or payments) as a server list: pagination, sort by
 * start, status filter; on phones the grid view. A row opens the visit.
 */
export function CustomerReservationTable({
  scope,
  money = false,
  business,
  emptyTitle,
  emptyDescription,
  toolbarExtra,
  persistKey,
}: Props) {
  const { t } = useLocale();
  const router = useRouter();
  const listState = useServerListState({
    initialSort: scope === "upcoming" ? "starts_at" : "-starts_at",
    initialPageSize: 20,
  });

  const params = useMemo<CustomerListParams>(() => {
    const status = filterValues(
      listState.columnFilters.find((filter) => filter.id === "status")?.value,
    );
    return {
      limit: listState.params.limit,
      offset: listState.params.offset,
      sort: listState.params.sort,
      scope,
      business,
      status: status.length ? status.join(",") : undefined,
    };
  }, [business, listState.columnFilters, listState.params, scope]);

  const query = useCustomerList(money, params);
  const columns = useCustomerReservationColumns({ money });

  const total = query.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );

  return (
    <EntityTable
      columns={columns}
      data={query.data?.items ?? []}
      getRowId={(row) => row.uuid}
      onRowClick={(row) => router.push(routes.customer.reservation(row.uuid))}
      isLoading={query.isLoading}
      isError={query.isError}
      errorDescription={t("customer_panel.errors.list_failed")}
      onRetry={() => void query.refetch()}
      emptyTitle={emptyTitle}
      emptyDescription={emptyDescription}
      pageCount={pageCount}
      state={listState.tableState}
      features={{
        persistKey,
        rowSelection: false,
        globalFilter: false,
        viewMode: true,
      }}
      toolbarExtra={
        <>
          {toolbarExtra}
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        </>
      }
    />
  );
}
