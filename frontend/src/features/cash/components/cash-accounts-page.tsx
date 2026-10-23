"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { ArrowLeftRight, CalendarDays, NotebookText } from "lucide-react";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { PermissionGuard } from "@/components/common/permission-guard";
import {
  EntityActions,
  EntityCreateButton,
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
  useUrlListDefaults,
} from "@/components/entity";
import { createSelectColumnDef } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BulkActionMenu,
  SelectionBanner,
  resolveBulkActionsWithIcons,
  useBulkSelection,
} from "@/features/bulk-engine";
import { useCashAccountColumns } from "@/features/cash/components/cash-account-columns";
import type { CashAccountRowActionHandlers } from "@/features/cash/components/cash-account-row-actions";
import { ClosingDialog } from "@/features/cash/components/closing-dialog";
import { MovementDialog } from "@/features/cash/components/movement-dialog";
import { TransferDialog } from "@/features/cash/components/transfer-dialog";
import { CASH_ACCOUNTS_RESOURCE } from "@/features/cash/constants";
import {
  useCashAccountFacets,
  useCashAccounts,
  useCashAccountsMeta,
  useUpdateCashAccount,
} from "@/features/cash/hooks/use-cash";
import type {
  CashAccount,
  ListCashAccountsParams,
} from "@/features/cash/types";
import { ResourceIOToolbar } from "@/features/io";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";

const FILTER_IDS = ["branch", "kind", "status"] as const;
const SORT_FIELDS = [
  "name",
  "kind",
  "balance",
  "status",
  "sort_order",
  "created_at",
] as const;

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

function joined(values: string[]) {
  return values.length ? values.join(",") : undefined;
}

/**
 * Tenant list standard for cash, bank and online accounts: server-side
 * pagination / sort / search, faceted filters, row actions (money in / out,
 * transfer, closing), export, bulk archive / activate, live refresh.
 */
export function CashAccountsPage() {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const urlDefaults = useUrlListDefaults(FILTER_IDS, SORT_FIELDS);

  const listState = useServerListState({
    initialSort: urlDefaults.initialSort ?? "",
    initialColumnFilters: urlDefaults.initialColumnFilters ?? [
      { id: "status", value: ["active"] },
    ],
    initialPageSize: 20,
    realtime: tenantLiveList(organization, CASH_ACCOUNTS_RESOURCE),
  });

  const [movementTarget, setMovementTarget] = useState<CashAccount | null>(
    null,
  );
  const [transfer, setTransfer] = useState<{
    open: boolean;
    from?: string;
  }>({ open: false });
  const [closingTarget, setClosingTarget] = useState<CashAccount | null>(null);

  const listParams = useMemo<ListCashAccountsParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    return {
      ...listState.params,
      branch: joined(column("branch")),
      kind: joined(column("kind")),
      status: joined(column("status")),
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useCashAccounts(listParams);
  const metaQuery = useCashAccountsMeta();
  const facetsQuery = useCashAccountFacets();
  const updateAccount = useUpdateCashAccount();

  const openDetail = useCallback(
    (account: CashAccount) =>
      router.push(routes.tenant.cash.detail(slug, account.uuid)),
    [router, slug],
  );

  const handlers = useMemo<CashAccountRowActionHandlers>(
    () => ({
      onView: openDetail,
      onMovement: setMovementTarget,
      onTransfer: (account) => setTransfer({ open: true, from: account.uuid }),
      onClose: setClosingTarget,
      onEdit: (account) =>
        router.push(routes.tenant.cash.edit(slug, account.uuid)),
      onToggleArchive: (account) =>
        updateAccount.mutate({
          uuid: account.uuid,
          body: {
            status: account.status === "active" ? "archived" : "active",
          },
        }),
    }),
    [openDetail, router, slug, updateAccount],
  );

  const baseColumns = useCashAccountColumns({
    handlers,
    facets: facetsQuery.data,
  });
  const columns = useMemo(
    () => [createSelectColumnDef<CashAccount>(), ...baseColumns],
    [baseColumns],
  );

  // Same filters for export and the "all matching" bulk scope.
  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      branch: listParams.branch,
      kind: listParams.kind,
      status: listParams.status,
      sort: listParams.sort,
    }),
    [
      listParams.branch,
      listParams.kind,
      listParams.q,
      listParams.sort,
      listParams.status,
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
        CASH_ACCOUNTS_RESOURCE,
        meta?.bulk_actions ?? [],
      ),
    [meta?.bulk_actions],
  );

  const pageCount = useMemo(() => {
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listState.pagination.pageSize, total]);

  const hasFilters =
    Boolean(listParams.q) ||
    Boolean(listParams.branch) ||
    Boolean(listParams.kind) ||
    Boolean(listParams.status);

  const refresh = () => {
    void listQuery.refetch();
    void facetsQuery.refetch();
  };

  return (
    <EntityPage
      title={t("cash.title")}
      description={t("cash.description")}
      permission={permissions.tenant.cash.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("cash.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("cash.title") },
      ]}
      actions={
        <EntityActions>
          <Button asChild size="sm" variant="outline">
            <Link href={routes.tenant.cash.daily(slug)}>
              <CalendarDays className="size-4" />
              {t("cash.daily.title")}
            </Link>
          </Button>
          <Button asChild size="sm" variant="outline">
            <Link href={routes.tenant.cash.movements(slug)}>
              <NotebookText className="size-4" />
              {t("cash.movements.title")}
            </Link>
          </Button>
          <PermissionGuard permission={permissions.tenant.cash.write}>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => setTransfer({ open: true })}
            >
              <ArrowLeftRight className="size-4" />
              {t("cash.actions.transfer")}
            </Button>
          </PermissionGuard>
          <EntityCreateButton
            onClick={() => router.push(routes.tenant.cash.create(slug))}
            label={t("cash.actions.create")}
            permission={permissions.tenant.cash.manage}
          />
        </EntityActions>
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
        errorDescription={t("cash.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters ? t("cash.empty_filtered_title") : t("cash.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("cash.empty_filtered_description")
            : t("cash.empty_description")
        }
        pageCount={pageCount}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: "tenant-cash-accounts-v1",
          rowSelection: true,
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource={CASH_ACCOUNTS_RESOURCE}
              actions={bulkActions}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              onComplete={() => {
                bulkSelection.clearSelection();
                refresh();
              }}
            />
            <ResourceIOToolbar
              resource={CASH_ACCOUNTS_RESOURCE}
              query={engineQuery}
              capabilities={meta?.capabilities}
            />
            <EntityToolbar
              onRefresh={refresh}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />

      <MovementDialog
        account={movementTarget}
        open={Boolean(movementTarget)}
        onOpenChange={(open) => {
          if (!open) setMovementTarget(null);
        }}
      />
      <TransferDialog
        fromAccountUuid={transfer.from}
        open={transfer.open}
        onOpenChange={(open) =>
          setTransfer((prev) => (open ? prev : { open: false }))
        }
      />
      <ClosingDialog
        account={closingTarget}
        open={Boolean(closingTarget)}
        onOpenChange={(open) => {
          if (!open) setClosingTarget(null);
        }}
      />
    </EntityPage>
  );
}
