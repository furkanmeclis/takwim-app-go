"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { PhoneOff, ScrollText } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ReassignmentDrawer } from "@/features/customer-phones/components/reassignment-drawer";
import { useCustomerPhoneReleases } from "@/features/customer-phones/hooks/use-customer-phones";
import type { ListCustomerPhoneReleasesParams } from "@/features/customer-phones/services/customer-phones.service";
import {
  RELEASE_REASONS,
  RELEASE_SOURCES,
  type CustomerPhoneRelease,
  type CustomerPhoneReleaseReason,
} from "@/features/customer-phones/types";
import { datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function joined(value: unknown) {
  const values = Array.isArray(value)
    ? value.filter((v): v is string => typeof v === "string" && v !== "")
    : typeof value === "string" && value
      ? [value]
      : [];
  return values.length ? values.join(",") : undefined;
}

function reasonTone(reason: CustomerPhoneReleaseReason) {
  if (reason === "reassigned") return "danger" as const;
  if (reason === "dormant") return "warning" as const;
  return "default" as const;
}

/** Log filter of the customer panel's security warnings. */
const CUSTOMER_AUTH_LOGS = `${routes.platform.settings.logs}?source=customer_auth&level=warn`;

/**
 * `/platform/settings/customer-phones` (TEC-972): customer numbers that
 * changed hands — number changes and reassignment reports of the people,
 * notices processed here and long unproven numbers closed at sign-in —
 * and the form that processes a reassignment notice.
 */
export function CustomerPhonesPage() {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const [reassigning, setReassigning] = useState(false);

  const listState = useServerListState({
    initialSort: "-released_at",
    initialPageSize: 20,
  });

  const params = useMemo<ListCustomerPhoneReleasesParams>(() => {
    const column = (id: string) =>
      joined(listState.columnFilters.find((f) => f.id === id)?.value);
    return {
      ...listState.params,
      reason: column("reason"),
      source: column("source"),
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useCustomerPhoneReleases(params);

  const columns = useMemo<ColumnDef<CustomerPhoneRelease>[]>(
    () => [
      createColumn<CustomerPhoneRelease>({
        accessorKey: "phone_masked",
        labelKey: "customer_phones.columns.phone",
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-mono text-sm tabular-nums">
            {row.original.phone_masked}
          </span>
        ),
      }),
      createColumn<CustomerPhoneRelease>({
        accessorKey: "reason",
        labelKey: "customer_phones.columns.reason",
        enableSorting: false,
        gridSecondary: true,
        filterVariant: "faceted",
        filterOptions: RELEASE_REASONS.map((value) => ({
          value,
          label: t(`customer_phones.reasons.${value}`),
          labelKey: `customer_phones.reasons.${value}`,
        })),
        cell: ({ row }) => (
          <StatusChip
            label={t(`customer_phones.reasons.${row.original.reason}`)}
            tone={reasonTone(row.original.reason)}
          />
        ),
      }),
      createColumn<CustomerPhoneRelease>({
        accessorKey: "source",
        labelKey: "customer_phones.columns.source",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: RELEASE_SOURCES.map((value) => ({
          value,
          label: t(`customer_phones.sources.${value}`),
          labelKey: `customer_phones.sources.${value}`,
        })),
        cell: ({ row }) => (
          <span className="flex flex-col">
            <span>{t(`customer_phones.sources.${row.original.source}`)}</span>
            {row.original.created_by_name ? (
              <span className="text-muted-foreground text-xs">
                {row.original.created_by_name}
              </span>
            ) : null}
          </span>
        ),
      }),
      createColumn<CustomerPhoneRelease>({
        id: "closed",
        labelKey: "customer_phones.columns.closed",
        enableSorting: false,
        cell: ({ row }) =>
          row.original.has_profile
            ? t("customer_phones.closed_summary", {
                businesses: row.original.businesses_hidden,
                sessions: row.original.sessions_revoked,
              })
            : t("customer_phones.no_profile"),
      }),
      createColumn<CustomerPhoneRelease>({
        accessorKey: "note",
        labelKey: "customer_phones.columns.note",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="line-clamp-2 text-sm">
            {row.original.note || "—"}
          </span>
        ),
      }),
      createColumn<CustomerPhoneRelease>({
        accessorKey: "released_at",
        labelKey: "customer_phones.columns.released_at",
        cell: ({ row }) =>
          datetime(row.original.released_at, "dd.MM.yyyy HH:mm", locale),
      }),
    ],
    [locale, t],
  );

  const total = listQuery.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );

  return (
    <EntityPage
      title={t("customer_phones.title")}
      description={t("customer_phones.description")}
      permission={permissions.customerPhones.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("customer_phones.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("customer_phones.title") },
      ]}
      actions={
        <div className="flex flex-wrap gap-2">
          {can(permissions.logs.read) ? (
            <Button variant="outline" asChild>
              <Link href={CUSTOMER_AUTH_LOGS}>
                <ScrollText className="size-4" />
                {t("customer_phones.view_warnings")}
              </Link>
            </Button>
          ) : null}
          {can(permissions.customerPhones.write) ? (
            <Button onClick={() => setReassigning(true)}>
              <PhoneOff className="size-4" />
              {t("customer_phones.reassign")}
            </Button>
          ) : null}
        </div>
      }
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={t("customer_phones.empty_title")}
        emptyDescription={t("customer_phones.empty_description")}
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "platform-customer-phones-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void listQuery.refetch()}
            refreshDisabled={listQuery.isFetching}
          />
        }
      />

      <ReassignmentDrawer open={reassigning} onOpenChange={setReassigning} />
    </EntityPage>
  );
}
