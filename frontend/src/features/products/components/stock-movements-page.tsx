"use client";

import { addDays, isValid, parseISO } from "date-fns";
import { ArrowLeftRight } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useMovementColumns } from "@/features/products/components/movement-columns";
import { SaleReturnDialog } from "@/features/products/components/sale-return-dialog";
import { StockMovementDialog } from "@/features/products/components/stock-movement-dialog";
import {
  StockSaleCollectDialog,
  StockSaleRefundDialog,
} from "@/features/products/components/stock-sale-money-dialogs";
import { ResourceIOToolbar } from "@/features/io";
import {
  PRODUCTS_RESOURCE,
  STOCK_MOVEMENTS_RESOURCE,
} from "@/features/products/constants";
import {
  useStockMovementsFacets,
  useStockMovementsList,
  useStockMovementsMeta,
} from "@/features/products/hooks/use-products";
import type {
  ListStockMovementsParams,
  StockMovement,
} from "@/features/products/types";
import { useNavModules } from "@/features/nav-engine/lib/modules";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

function joined(values: string[]) {
  return values.length ? values.join(",") : undefined;
}

/**
 * A picked day (`yyyy-MM-dd`) as the RFC 3339 instant of its local
 * midnight, `days` later: `from` is the start of the first day, `to` the
 * start of the day after the last (the API range is half-open).
 */
function dayStart(day: string, days = 0): string | undefined {
  if (!day) return undefined;
  const parsed = parseISO(day);
  return isValid(parsed) ? addDays(parsed, days).toISOString() : undefined;
}

/**
 * The append-only stock ledger of the member's branches: every receipt,
 * sale, return, count and transfer with the balance right after it.
 * Rows are never edited; a correction is a new count or movement.
 */
export function StockMovementsPage() {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const { can } = usePermission();
  const hasModule = useNavModules();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [returning, setReturning] = useState<StockMovement | null>(null);
  const [collecting, setCollecting] = useState<StockMovement | null>(null);
  const [refunding, setRefunding] = useState<StockMovement | null>(null);
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");

  // Movements publish on the products channel (stock changed).
  const listState = useServerListState({
    initialSort: "-occurred_at",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, PRODUCTS_RESOURCE),
  });

  const listParams = useMemo<ListStockMovementsParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    return {
      ...listState.params,
      kind: joined(column("kind")),
      branch: joined(column("branch")),
      staff: joined(column("staff")),
      from: dayStart(dateFrom),
      to: dayStart(dateTo, 1),
    };
  }, [dateFrom, dateTo, listState.columnFilters, listState.params]);

  const listQuery = useStockMovementsList(listParams);
  const facetsQuery = useStockMovementsFacets();
  const metaQuery = useStockMovementsMeta();
  // A return needs both the stock and the refund permission (API rule).
  const canReturn =
    can(permissions.tenant.products.stock) &&
    can(permissions.tenant.payment.refund);
  // Money of a sale (TEC-992): the money permissions and the cash module.
  const canCollect =
    hasModule("cash") && can(permissions.tenant.payment.collect);
  const canRefund = hasModule("cash") && can(permissions.tenant.payment.refund);
  const columns = useMovementColumns({
    facets: facetsQuery.data,
    onReturn: canReturn ? setReturning : undefined,
    onCollect: canCollect ? setCollecting : undefined,
    onRefund: canRefund ? setRefunding : undefined,
  });

  const total = listQuery.data?.total ?? 0;
  const pageCount = useMemo(() => {
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listState.pagination.pageSize, total]);

  // Exports honour the same filters as the list.
  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      kind: listParams.kind,
      branch: listParams.branch,
      product: listParams.product,
      staff: listParams.staff,
      from: listParams.from,
      to: listParams.to,
      sort: listParams.sort,
    }),
    [
      listParams.branch,
      listParams.from,
      listParams.kind,
      listParams.product,
      listParams.q,
      listParams.sort,
      listParams.staff,
      listParams.to,
    ],
  );

  const hasFilters = Object.entries(engineQuery).some(
    ([key, value]) => key !== "sort" && Boolean(value),
  );

  const canMove =
    can(permissions.tenant.products.stock) ||
    can(permissions.tenant.products.sell);

  const refresh = () => {
    void listQuery.refetch();
    void facetsQuery.refetch();
  };

  return (
    <EntityPage
      title={t("products.movements.title")}
      description={t("products.movements.description")}
      permission={permissions.tenant.products.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("products.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("products.title"),
          href: routes.tenant.products.root(slug),
        },
        { label: t("products.movements.title") },
      ]}
      actions={
        canMove ? (
          <Button onClick={() => setDialogOpen(true)}>
            <ArrowLeftRight className="size-4" />
            {t("products.actions.record_movement")}
          </Button>
        ) : null
      }
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("products.movements.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("products.movements.empty_filtered_title")
            : t("products.movements.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("products.movements.empty_filtered_description")
            : t("products.movements.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-stock-movements-v1",
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
                placeholder={t("products.movements.date_from")}
              />
              <DatePicker
                className="w-full sm:w-40"
                value={dateTo}
                onChange={setDateTo}
                placeholder={t("products.movements.date_to")}
              />
            </div>
            <ResourceIOToolbar
              resource={STOCK_MOVEMENTS_RESOURCE}
              query={engineQuery}
              capabilities={metaQuery.data?.capabilities}
              onImportComplete={refresh}
            />
            <EntityToolbar
              onRefresh={refresh}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />

      <StockMovementDialog open={dialogOpen} onOpenChange={setDialogOpen} />
      <SaleReturnDialog
        movement={returning}
        onOpenChange={(open) => {
          if (!open) setReturning(null);
        }}
      />
      <StockSaleCollectDialog
        movement={collecting}
        onOpenChange={(open) => {
          if (!open) setCollecting(null);
        }}
      />
      <StockSaleRefundDialog
        movement={refunding}
        onOpenChange={(open) => {
          if (!open) setRefunding(null);
        }}
      />
    </EntityPage>
  );
}
