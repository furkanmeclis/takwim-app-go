"use client";

import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import {
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { routes } from "@/config/routes";
import { BusinessSelect } from "@/features/customer-panel/components/business-select";
import { useCustomerPackageColumns } from "@/features/customer-panel/components/customer-package-columns";
import { useCustomerPackages } from "@/features/customer-panel/hooks/use-customer-panel";
import type { CustomerPackageParams } from "@/features/customer-panel/types";
import { useLocale } from "@/providers/locale-provider";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

/**
 * `/panel/packages` ("Paketlerim", TEC-984): the session packages the
 * person bought from the businesses that know them, active first. A row
 * opens the package with its usage history.
 */
export function CustomerPackagesPage() {
  const { t } = useLocale();
  const router = useRouter();
  const [business, setBusiness] = useState<string | undefined>();
  const listState = useServerListState({ initialPageSize: 20 });
  const columns = useCustomerPackageColumns();

  const params = useMemo<CustomerPackageParams>(() => {
    const state = filterValues(
      listState.columnFilters.find((filter) => filter.id === "state")?.value,
    );
    return {
      limit: listState.params.limit,
      offset: listState.params.offset,
      business,
      state: state.length ? state.join(",") : undefined,
    };
  }, [business, listState.columnFilters, listState.params]);

  const query = useCustomerPackages(params);
  const total = query.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );

  return (
    <EntityPage
      title={t("customer_panel.packages.title")}
      description={t("customer_panel.packages.description")}
    >
      <Alert>
        <AlertDescription>{t("customer_panel.packages.note")}</AlertDescription>
      </Alert>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) => router.push(routes.customer.package(row.uuid))}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("customer_panel.errors.list_failed")}
        onRetry={() => void query.refetch()}
        emptyTitle={t("customer_panel.packages.empty")}
        emptyDescription={t("customer_panel.packages.empty_description")}
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "customer-panel-packages-v1",
          rowSelection: false,
          globalFilter: false,
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <BusinessSelect value={business} onChange={setBusiness} />
            <EntityToolbar
              onRefresh={() => void query.refetch()}
              refreshDisabled={query.isFetching}
            />
          </>
        }
      />
    </EntityPage>
  );
}
