"use client";

import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
  useUrlListDefaults,
} from "@/components/entity";
import { DatePicker } from "@/components/ui/date-picker";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useCashMovementColumns } from "@/features/cash/components/cash-movement-columns";
import { ReasonDialog } from "@/features/cash/components/reason-dialog";
import { CASH_MOVEMENTS_RESOURCE } from "@/features/cash/constants";
import {
  useCashMovementFacets,
  useCashMovements,
  useCashMovementsMeta,
  useReverseCashMovement,
} from "@/features/cash/hooks/use-cash";
import type {
  CashDirection,
  CashMovement,
  ListCashMovementsParams,
} from "@/features/cash/types";
import { ResourceIOToolbar } from "@/features/io";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";

const FILTER_IDS = ["account", "kind", "direction"] as const;
const SORT_FIELDS = ["movement_date", "amount", "kind", "created_at"] as const;

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

function joined(values: string[]) {
  return values.length ? values.join(",") : undefined;
}

/**
 * The cash book: every movement of every account the member can see,
 * filtered by account, kind, direction and date range. Read-only apart from
 * reversals; money is posted from an account or as a transfer.
 */
export function CashMovementsPage() {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const urlDefaults = useUrlListDefaults(FILTER_IDS, SORT_FIELDS);
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");

  const listState = useServerListState({
    initialSort: urlDefaults.initialSort ?? "-movement_date",
    initialColumnFilters: urlDefaults.initialColumnFilters,
    initialPageSize: 50,
    realtime: tenantLiveList(organization, CASH_MOVEMENTS_RESOURCE),
  });

  const listParams = useMemo<ListCashMovementsParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    const direction = column("direction");
    return {
      ...listState.params,
      account: joined(column("account")),
      kind: joined(column("kind")),
      direction:
        direction.length === 1 ? (direction[0] as CashDirection) : undefined,
      date_from: dateFrom || undefined,
      date_to: dateTo || undefined,
    };
  }, [dateFrom, dateTo, listState.columnFilters, listState.params]);

  const listQuery = useCashMovements(listParams);
  const metaQuery = useCashMovementsMeta();
  const facetsQuery = useCashMovementFacets();
  const reverse = useReverseCashMovement();
  const [target, setTarget] = useState<CashMovement | null>(null);
  const onReverse = useCallback((m: CashMovement) => setTarget(m), []);

  const columns = useCashMovementColumns({
    facets: facetsQuery.data,
    showAccount: true,
    showBalance: false,
    filters: true,
    onReverse,
  });

  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      account: listParams.account,
      kind: listParams.kind,
      direction: listParams.direction,
      date_from: listParams.date_from,
      date_to: listParams.date_to,
      sort: listParams.sort,
    }),
    [
      listParams.account,
      listParams.date_from,
      listParams.date_to,
      listParams.direction,
      listParams.kind,
      listParams.q,
      listParams.sort,
    ],
  );

  const total = listQuery.data?.total ?? 0;
  const pageCount = useMemo(() => {
    const size = listState.pagination.pageSize || 50;
    return Math.max(1, Math.ceil(total / size));
  }, [listState.pagination.pageSize, total]);

  const hasFilters = Object.entries(engineQuery).some(
    ([key, value]) => key !== "sort" && Boolean(value),
  );

  const refresh = () => {
    void listQuery.refetch();
    void facetsQuery.refetch();
  };

  return (
    <EntityPage
      title={t("cash.movements.title")}
      description={t("cash.movements.description")}
      permission={permissions.tenant.cash.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("cash.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("cash.title"), href: routes.tenant.cash.root(slug) },
        { label: t("cash.movements.title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("cash.movements.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("cash.empty_filtered_title")
            : t("cash.movements.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("cash.empty_filtered_description")
            : t("cash.movements.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-cash-movements-v1",
          rowSelection: false,
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <div className="flex flex-wrap items-center gap-2">
              <DatePicker
                className="w-full sm:w-40"
                value={dateFrom}
                onChange={setDateFrom}
                placeholder={t("cash.movements.date_from")}
              />
              <DatePicker
                className="w-full sm:w-40"
                value={dateTo}
                onChange={setDateTo}
                placeholder={t("cash.movements.date_to")}
              />
            </div>
            <ResourceIOToolbar
              resource={CASH_MOVEMENTS_RESOURCE}
              query={engineQuery}
              capabilities={metaQuery.data?.capabilities}
            />
            <EntityToolbar
              onRefresh={refresh}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />
      <ReasonDialog
        open={Boolean(target)}
        title={t("cash.reverse_dialog.title")}
        description={t(
          target?.transfer_group
            ? "cash.reverse_dialog.description_transfer"
            : "cash.reverse_dialog.description",
        )}
        confirmLabel={t("cash.actions.reverse")}
        isPending={reverse.isPending}
        onOpenChange={(open) => {
          if (!open) setTarget(null);
        }}
        onConfirm={(reason) =>
          reverse.mutateAsync({ uuid: target!.uuid, reason })
        }
      />
    </EntityPage>
  );
}
