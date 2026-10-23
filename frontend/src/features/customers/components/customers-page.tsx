"use client";

import { useRouter } from "next/navigation";
import { useCallback, useMemo, useRef, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityCreateButton,
  EntityDeleteDialog,
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createSelectColumnDef } from "@/components/tables";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BulkActionMenu,
  SelectionBanner,
  resolveBulkActionsWithIcons,
  useBulkSelection,
} from "@/features/bulk-engine";
import type { BulkActionDef } from "@/features/bulk-engine/types";
import { useCustomerColumns } from "@/features/customers/components/customer-columns";
import { CustomerQuickEditDrawer } from "@/features/customers/components/customer-quick-edit-drawer";
import type { CustomerRowActionHandlers } from "@/features/customers/components/customer-row-actions";
import { CustomerTagDialog } from "@/features/customers/components/customer-tag-dialog";
import { CUSTOMERS_RESOURCE } from "@/features/customers/constants";
import {
  useCustomersFacets,
  useCustomersList,
  useCustomersMeta,
  useDeleteCustomer,
} from "@/features/customers/hooks/use-customers";
import type { Customer, ListCustomersParams } from "@/features/customers/types";
import { ResourceIOToolbar } from "@/features/io";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

/** Both values of a yes / no facet selected = no filter. */
function oneBoolean(values: string[]) {
  return values.length === 1 ? (values[0] as "true" | "false") : undefined;
}

function joined(values: string[]) {
  return values.length ? values.join(",") : undefined;
}

type TagPrompt = {
  mode: "add" | "remove";
  resolve: (params: Record<string, string> | null) => void;
};

/**
 * Tenant list standard: server-side pagination / sort / search, faceted
 * filters, column visibility, row actions, row click → detail, quick-edit
 * drawer, export / import, bulk actions (tag / untag ask the tag first).
 */
export function CustomersPage() {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();

  const listState = useServerListState({
    initialSort: "",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, CUSTOMERS_RESOURCE),
  });

  const [quickEdit, setQuickEdit] = useState<Customer | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Customer | null>(null);
  const [tagPrompt, setTagPrompt] = useState<TagPrompt | null>(null);
  const tagPromptRef = useRef<TagPrompt | null>(null);

  const listParams = useMemo<ListCustomersParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    return {
      ...listState.params,
      tag: joined(column("tags")),
      source: joined(column("source")),
      verified: oneBoolean(column("verified")),
      sms_consent: oneBoolean(column("sms_consent")),
      email_consent: oneBoolean(column("email_consent")),
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useCustomersList(listParams);
  const metaQuery = useCustomersMeta();
  const facetsQuery = useCustomersFacets();
  const deleteCustomer = useDeleteCustomer();

  const openDetail = useCallback(
    (customer: Customer) =>
      router.push(routes.tenant.customers.detail(slug, customer.uuid)),
    [router, slug],
  );

  const handlers = useMemo<CustomerRowActionHandlers>(
    () => ({
      onView: openDetail,
      onQuickEdit: setQuickEdit,
      onEdit: (customer) =>
        router.push(routes.tenant.customers.edit(slug, customer.uuid)),
      onDelete: setDeleteTarget,
    }),
    [openDetail, router, slug],
  );

  const baseColumns = useCustomerColumns({
    handlers,
    facets: facetsQuery.data,
  });
  const columns = useMemo(
    () => [createSelectColumnDef<Customer>(), ...baseColumns],
    [baseColumns],
  );

  // Same filters for export, import refresh and "all matching" bulk scope.
  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      tag: listParams.tag,
      source: listParams.source,
      verified: listParams.verified,
      sms_consent: listParams.sms_consent,
      email_consent: listParams.email_consent,
      sort: listParams.sort,
    }),
    [
      listParams.email_consent,
      listParams.q,
      listParams.sms_consent,
      listParams.sort,
      listParams.source,
      listParams.tag,
      listParams.verified,
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
      resolveBulkActionsWithIcons(CUSTOMERS_RESOURCE, meta?.bulk_actions ?? []),
    [meta?.bulk_actions],
  );

  const resolveParams = useCallback((action: BulkActionDef) => {
    if (action.id !== "add_tag" && action.id !== "remove_tag") {
      return Promise.resolve(undefined);
    }
    return new Promise<Record<string, string> | null>((resolve) => {
      const prompt: TagPrompt = {
        mode: action.id === "add_tag" ? "add" : "remove",
        resolve,
      };
      tagPromptRef.current = prompt;
      setTagPrompt(prompt);
    });
  }, []);

  const closeTagPrompt = (params: Record<string, string> | null) => {
    tagPromptRef.current?.resolve(params);
    tagPromptRef.current = null;
    setTagPrompt(null);
  };

  const pageCount = useMemo(() => {
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listState.pagination.pageSize, total]);

  const hasFilters =
    Boolean(listParams.q) ||
    Boolean(listParams.tag) ||
    Boolean(listParams.source) ||
    Boolean(listParams.verified) ||
    Boolean(listParams.sms_consent) ||
    Boolean(listParams.email_consent);

  const refresh = () => {
    void listQuery.refetch();
    void facetsQuery.refetch();
  };

  return (
    <EntityPage
      title={t("customers.title")}
      description={t("customers.description")}
      permission={permissions.tenant.customers.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("customers.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("customers.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => router.push(routes.tenant.customers.create(slug))}
          label={t("customers.actions.create")}
          permission={permissions.tenant.customers.write}
        />
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
        errorDescription={t("customers.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("customers.empty_filtered_title")
            : t("customers.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("customers.empty_filtered_description")
            : t("customers.empty_description")
        }
        pageCount={pageCount}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: `tenant-customers-v1`,
          rowSelection: true,
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource={CUSTOMERS_RESOURCE}
              actions={bulkActions}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              resolveParams={resolveParams}
              onComplete={() => {
                bulkSelection.clearSelection();
                refresh();
              }}
            />
            <ResourceIOToolbar
              resource={CUSTOMERS_RESOURCE}
              query={engineQuery}
              capabilities={meta?.capabilities}
              onImportComplete={refresh}
            />
            <EntityToolbar
              onRefresh={refresh}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />

      <CustomerQuickEditDrawer
        customer={quickEdit}
        onOpenChange={(open) => {
          if (!open) setQuickEdit(null);
        }}
      />

      <CustomerTagDialog
        open={Boolean(tagPrompt)}
        mode={tagPrompt?.mode ?? "add"}
        suggestions={(facetsQuery.data?.tag ?? []).map((facet) => facet.value)}
        onSubmit={(tag) => closeTagPrompt({ tag })}
        onCancel={() => closeTagPrompt(null)}
      />

      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        entityLabel={deleteTarget?.full_name ?? ""}
        isPending={deleteCustomer.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return;
          deleteCustomer.mutate(deleteTarget.uuid, {
            onSettled: () => setDeleteTarget(null),
          });
        }}
      />
    </EntityPage>
  );
}
