"use client";

import { useQuery } from "@tanstack/react-query";
import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useExportsColumns } from "@/features/io/components/exports-columns";
import { ioKeys } from "@/features/io/hooks/query-keys";
import { exportsService } from "@/features/io/services/exports.service";
import { useLocale } from "@/providers/locale-provider";
import { useMemo } from "react";

export function ExportsPage() {
  const { t } = useLocale();
  const router = useRouter();
  const listState = useServerListState({ initialPageSize: 20 });
  const columns = useExportsColumns();

  const listQuery = useQuery({
    queryKey: ioKeys.exports.list(listState.params),
    queryFn: () =>
      exportsService.list({
        limit: listState.params.limit,
        offset: listState.params.offset,
      }),
  });

  const pageCount = useMemo(() => {
    const total = listQuery.data?.total ?? 0;
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listQuery.data?.total, listState.pagination.pageSize]);

  return (
    <EntityPage
      title={t("exports.title")}
      description={t("exports.description")}
      permission={permissions.exports.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("exports.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("exports.title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(job) =>
          router.push(routes.platform.exports.detail(job.uuid))
        }
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={t("exports.empty_title")}
        emptyDescription={t("exports.empty_description")}
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "platform-exports-v1",
          rowSelection: false,
          columnFilters: false,
          globalFilter: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void listQuery.refetch()}
            refreshDisabled={listQuery.isFetching}
          />
        }
      />
    </EntityPage>
  );
}
