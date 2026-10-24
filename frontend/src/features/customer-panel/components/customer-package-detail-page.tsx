"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useRouter } from "next/navigation";
import { useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatsCard } from "@/components/common/stats-card";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { routes } from "@/config/routes";
import { useCustomerPackage } from "@/features/customer-panel/hooks/use-customer-panel";
import type {
  CustomerPackage,
  CustomerPackageSession,
} from "@/features/customer-panel/types";
import { statusLabel } from "@/features/reservations/lib/labels";
import {
  SESSION_STATUS_LABEL_KEYS,
  SESSION_STATUS_TONES,
  STATE_LABEL_KEYS,
  STATE_TONES,
} from "@/features/session-packages/constants";
import { isApiError } from "@/lib/api";
import { amount, date, datetimeInZone } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * `/panel/packages/{uuid}`: one package as the person sees it (TEC-984):
 * sessions used, booked and left, the last day, where it can be used and
 * the visits that used it. Sessions given back to the package are not
 * listed.
 */
export function CustomerPackageDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const query = useCustomerPackage(uuid);
  const pkg = query.data;
  const notFound =
    query.isError && isApiError(query.error) && query.error.status === 404;

  return (
    <EntityPage
      title={pkg ? pkg.name : t("customer_panel.packages.detail_title")}
      breadcrumbs={[
        {
          label: t("customer_panel.packages.title"),
          href: routes.customer.packages,
        },
        { label: pkg ? pkg.name : "…" },
      ]}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={
            notFound
              ? t("customer_panel.packages.not_found")
              : t("customer_panel.errors.generic")
          }
          onRetry={notFound ? undefined : () => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {pkg ? (
        <div className="space-y-6">
          <EntityHeader
            title={pkg.name}
            subtitle={`${pkg.business.name} · ${pkg.service_name}`}
            badges={
              <StatusChip
                label={t(STATE_LABEL_KEYS[pkg.state])}
                tone={STATE_TONES[pkg.state]}
              />
            }
          />
          {pkg.state === "refunded" ? (
            <Alert>
              <AlertDescription>
                {t("customer_panel.packages.refunded_note", {
                  date: pkg.refunded_on
                    ? date(pkg.refunded_on, undefined, locale)
                    : "—",
                })}
              </AlertDescription>
            </Alert>
          ) : null}
          <div className="grid gap-3 sm:grid-cols-3">
            <StatsCard
              title={t("customer_panel.packages.remaining")}
              value={t("customer_panel.packages.remaining_of", {
                remaining: pkg.remaining,
                total: pkg.total_sessions,
              })}
              hint={
                pkg.expires_on
                  ? t("customer_panel.packages.until", {
                      date: date(pkg.expires_on, undefined, locale),
                    })
                  : t("session_packages.validity_none")
              }
            />
            <StatsCard
              title={t("customer_panel.packages.used")}
              value={pkg.used}
            />
            <StatsCard
              title={t("customer_panel.packages.held")}
              value={pkg.held}
              hint={t("customer_panel.packages.held_hint")}
            />
          </div>
          <PackageTerms pkg={pkg} />
          <PackageSessions sessions={pkg.sessions ?? []} />
        </div>
      ) : null}
    </EntityPage>
  );
}

function PackageTerms({ pkg }: { pkg: CustomerPackage }) {
  const { t, locale } = useLocale();
  return (
    <EntitySectionCard title={t("customer_panel.packages.terms")}>
      <EntityDetail
        sections={[
          {
            id: "terms",
            fields: [
              {
                key: "business",
                label: t("customer_panel.columns.business"),
                value: pkg.business.name,
              },
              {
                key: "service",
                label: t("customer_panel.packages.columns.service"),
                value: pkg.service_name,
              },
              {
                key: "sold_on",
                label: t("customer_panel.packages.columns.sold_on"),
                value: `${date(pkg.sold_on, undefined, locale)} · ${pkg.branch.name}`,
              },
              {
                key: "expires_on",
                label: t("customer_panel.packages.columns.expires_on"),
                value: pkg.expires_on
                  ? date(pkg.expires_on, undefined, locale)
                  : t("session_packages.validity_none"),
              },
              {
                key: "branches",
                label: t("customer_panel.packages.branches"),
                value: pkg.all_branches
                  ? t("session_packages.all_branches")
                  : pkg.branches.map((branch) => branch.name).join(", ") || "—",
              },
              {
                key: "price",
                label: t("customer_panel.packages.columns.price"),
                value: amount(pkg.price, pkg.currency, locale),
              },
            ],
          },
        ]}
      />
    </EntitySectionCard>
  );
}

function PackageSessions({ sessions }: { sessions: CustomerPackageSession[] }) {
  const { t, locale } = useLocale();
  const router = useRouter();
  const columns = useMemo(
    () =>
      [
        createColumn<CustomerPackageSession>({
          accessorKey: "session_no",
          labelKey: "customer_panel.packages.sessions.session_no",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">#{row.original.session_no}</span>
          ),
        }),
        createColumn<CustomerPackageSession>({
          accessorKey: "starts_at",
          labelKey: "customer_panel.columns.starts_at",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {datetimeInZone(
                row.original.starts_at,
                row.original.timezone,
                locale,
                { dateStyle: "medium", timeStyle: "short" },
              )}
            </span>
          ),
        }),
        createColumn<CustomerPackageSession>({
          id: "visit",
          accessorFn: (row) => row.service_name ?? "",
          labelKey: "customer_panel.packages.sessions.visit",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <div className="flex flex-col">
              <span>
                {[row.original.service_name, row.original.staff_name]
                  .filter(Boolean)
                  .join(" · ") || "—"}
              </span>
              <span className="text-muted-foreground text-xs">
                {row.original.branch_name} · #{row.original.reservation_number}{" "}
                · {statusLabel(t, row.original.reservation_status)}
              </span>
            </div>
          ),
        }),
        createColumn<CustomerPackageSession>({
          accessorKey: "status",
          labelKey: "customer_panel.packages.sessions.status",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(SESSION_STATUS_LABEL_KEYS[row.original.status])}
              tone={SESSION_STATUS_TONES[row.original.status]}
            />
          ),
        }),
        createColumn<CustomerPackageSession>({
          accessorKey: "consumed_on",
          labelKey: "customer_panel.packages.sessions.consumed_on",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.consumed_on
              ? date(row.original.consumed_on, undefined, locale)
              : "—",
        }),
      ] as ColumnDef<CustomerPackageSession, unknown>[],
    [locale, t],
  );

  return (
    <EntitySectionCard
      title={t("customer_panel.packages.history")}
      badge={sessions.length}
    >
      <p className="text-muted-foreground mb-3 text-sm">
        {t("customer_panel.packages.history_hint")}
      </p>
      <EntityTable
        columns={columns}
        data={sessions}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.customer.reservation(row.reservation_uuid))
        }
        emptyTitle={t("customer_panel.packages.history_empty")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          globalFilter: false,
          sorting: false,
          pagination: false,
          viewMode: true,
          persistKey: "customer-panel-package-sessions-v1",
        }}
      />
    </EntitySectionCard>
  );
}
