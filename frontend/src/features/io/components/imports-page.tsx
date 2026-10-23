"use client";

import { useMemo } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
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
import { useImportsColumns } from "@/features/io/components/imports-columns";
import { ioKeys } from "@/features/io/hooks/query-keys";
import { importsService } from "@/features/io/services/imports.service";
import { useAppMutation } from "@/lib/query/mutation";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export function ImportsPage() {
  const { t } = useLocale();
  const router = useRouter();
  const queryClient = useQueryClient();
  const listState = useServerListState({ initialPageSize: 20 });

  const listQuery = useQuery({
    queryKey: ioKeys.imports.list(listState.params),
    queryFn: () =>
      importsService.list({
        limit: listState.params.limit,
        offset: listState.params.offset,
      }),
  });

  const rollback = useAppMutation({
    mutationFn: (uuid: string) => importsService.rollback(uuid),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ioKeys.imports.lists() });
      appToast.success(t("imports.toast.rollback_success"));
    },
  });

  const columns = useImportsColumns({
    onRollback: (uuid) => rollback.mutate(uuid),
    rollbackPending: rollback.isPending,
  });

  const pageCount = useMemo(() => {
    const total = listQuery.data?.total ?? 0;
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listQuery.data?.total, listState.pagination.pageSize]);

  return (
    <EntityPage
      title={t("imports.title")}
      description={t("imports.description")}
      permission={permissions.imports.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("imports.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("imports.title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(job) =>
          router.push(routes.platform.imports.detail(job.uuid))
        }
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={t("imports.empty_title")}
        emptyDescription={t("imports.empty_description")}
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "platform-imports-v1",
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
