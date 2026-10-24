"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Undo2 } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatsCard } from "@/components/common/stats-card";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityActions,
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useCashAmount } from "@/features/cash/lib/format";
import { useNavModules } from "@/features/nav-engine/lib/modules";
import { RefundDialog } from "@/features/session-packages/components/refund-dialog";
import {
  RELEASE_REASON_LABEL_KEYS,
  SESSION_STATUS_LABEL_KEYS,
  SESSION_STATUS_TONES,
  STATE_LABEL_KEYS,
  STATE_TONES,
} from "@/features/session-packages/constants";
import { useCustomerPackage } from "@/features/session-packages/hooks/use-session-packages";
import { sessionsLabel } from "@/features/session-packages/lib/format";
import type { CustomerPackageSession } from "@/features/session-packages/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** One package sold to a customer: balance, sessions with their visits, refund. */
export function CustomerPackageDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const hasModule = useNavModules();
  const router = useRouter();
  const money = useCashAmount();
  const query = useCustomerPackage(uuid);
  const [refunding, setRefunding] = useState(false);
  const pkg = query.data;
  const title = pkg?.name ?? t("session_packages.detail.title");
  const canRefund =
    hasModule("cash") &&
    can(permissions.tenant.payment.refund) &&
    pkg != null &&
    pkg.state !== "refunded";

  const columns = useMemo(
    () =>
      [
        createColumn<CustomerPackageSession>({
          accessorKey: "session_no",
          labelKey: "session_packages.sessions.columns.session_no",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">#{row.original.session_no}</span>
          ),
        }),
        createColumn<CustomerPackageSession>({
          id: "reservation",
          accessorFn: (row) => row.reservation.number,
          labelKey: "session_packages.sessions.columns.reservation",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="min-w-0">
              <p className="font-medium">
                {t("session_packages.sessions.reservation_number", {
                  number: row.original.reservation.number,
                })}
              </p>
              <p className="text-muted-foreground text-xs">
                {datetime(
                  row.original.reservation.starts_at,
                  undefined,
                  locale,
                )}
              </p>
            </div>
          ),
        }),
        createColumn<CustomerPackageSession>({
          id: "service",
          accessorFn: (row) => row.service_name ?? "",
          labelKey: "session_packages.sessions.columns.service",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="min-w-0">
              <p className="truncate">{row.original.service_name ?? "—"}</p>
              {row.original.staff_name ? (
                <p className="text-muted-foreground truncate text-xs">
                  {row.original.staff_name}
                </p>
              ) : null}
            </div>
          ),
        }),
        createColumn<CustomerPackageSession>({
          accessorKey: "amount",
          labelKey: "session_packages.sessions.columns.amount",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {money(row.original.amount, pkg?.currency)}
            </span>
          ),
        }),
        createColumn<CustomerPackageSession>({
          accessorKey: "status",
          labelKey: "session_packages.sessions.columns.status",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => {
            const s = row.original;
            const reason = s.release_reason
              ? t(
                  RELEASE_REASON_LABEL_KEYS[s.release_reason] ??
                    "session_packages.release_reasons.other",
                )
              : "";
            const when = s.consumed_on
              ? date(s.consumed_on, undefined, locale)
              : s.released_at
                ? datetime(s.released_at, undefined, locale)
                : "";
            return (
              <div className="flex flex-col items-start gap-1">
                <StatusChip
                  label={t(SESSION_STATUS_LABEL_KEYS[s.status])}
                  tone={SESSION_STATUS_TONES[s.status]}
                />
                {reason || when ? (
                  <span className="text-muted-foreground text-xs">
                    {[reason, when].filter(Boolean).join(" · ")}
                  </span>
                ) : null}
              </div>
            );
          },
        }),
      ] as ColumnDef<CustomerPackageSession, unknown>[],
    [locale, money, pkg?.currency, t],
  );

  return (
    <EntityPage
      title={title}
      description={t("session_packages.detail.description")}
      permission={permissions.tenant.sessionPackages.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("session_packages.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("session_packages.title"),
          href: routes.tenant.packages.root(slug),
        },
        { label: title },
      ]}
      actions={
        canRefund ? (
          <EntityActions>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="text-destructive hover:text-destructive"
              onClick={() => setRefunding(true)}
            >
              <Undo2 className="size-4" />
              {t("session_packages.actions.refund")}
            </Button>
          </EntityActions>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("session_packages.detail.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {pkg ? (
        <div className="space-y-6">
          <EntityHeader
            title={pkg.name}
            subtitle={[pkg.customer.name, pkg.service_name]
              .filter(Boolean)
              .join(" · ")}
            badges={
              <StatusChip
                label={t(STATE_LABEL_KEYS[pkg.state])}
                tone={STATE_TONES[pkg.state]}
              />
            }
          />

          <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
            <StatsCard
              title={t("session_packages.detail.used")}
              value={sessionsLabel(pkg.used, pkg.total_sessions)}
              hint={money(pkg.used_amount, pkg.currency)}
            />
            <StatsCard
              title={t("session_packages.detail.held")}
              value={String(pkg.held)}
              hint={money(pkg.held_amount, pkg.currency)}
            />
            <StatsCard
              title={t("session_packages.detail.remaining")}
              value={String(pkg.remaining)}
              hint={money(pkg.remaining_amount, pkg.currency)}
            />
            <StatsCard
              title={t("session_packages.detail.session_value")}
              value={money(pkg.session_value, pkg.currency)}
              hint={t("session_packages.detail.price_hint", {
                price: money(pkg.price, pkg.currency),
              })}
            />
          </div>

          <EntitySectionCard title={t("session_packages.detail.terms")}>
            <EntityDetail
              sections={[
                {
                  id: "terms",
                  fields: [
                    {
                      key: "customer",
                      label: t("session_packages.customer.columns.customer"),
                      value: (
                        <Link
                          className="text-primary hover:underline"
                          href={routes.tenant.customers.detail(
                            slug,
                            pkg.customer.uuid,
                          )}
                        >
                          {pkg.customer.name}
                        </Link>
                      ),
                    },
                    {
                      key: "service",
                      label: t("session_packages.columns.service"),
                      value: pkg.service_name,
                    },
                    {
                      key: "branch",
                      label: t("session_packages.detail.sold_branch"),
                      value: pkg.branch.name,
                    },
                    {
                      key: "branches",
                      label: t("session_packages.columns.branches"),
                      value: pkg.all_branches
                        ? t("session_packages.all_branches")
                        : pkg.branches.map((b) => b.name).join(", "),
                    },
                    {
                      key: "sold_on",
                      label: t("session_packages.customer.columns.sold_on"),
                      value: date(pkg.sold_on, undefined, locale),
                    },
                    {
                      key: "expires_on",
                      label: t("session_packages.customer.columns.expires_on"),
                      value: pkg.expires_on
                        ? date(pkg.expires_on, undefined, locale)
                        : t("session_packages.validity_none"),
                    },
                    {
                      key: "staff",
                      label: t("session_packages.detail.staff"),
                      value: pkg.staff?.name ?? "—",
                    },
                    {
                      key: "note",
                      label: t("session_packages.detail.note"),
                      value: pkg.note || "—",
                    },
                    ...(pkg.refund
                      ? [
                          {
                            key: "refund",
                            label: t("session_packages.detail.refund"),
                            value: t("session_packages.detail.refund_value", {
                              amount: money(pkg.refund.amount, pkg.currency),
                              date: date(
                                pkg.refund.refunded_on,
                                undefined,
                                locale,
                              ),
                              reason: pkg.refund.reason,
                            }),
                          },
                        ]
                      : []),
                  ],
                },
              ]}
            />
          </EntitySectionCard>

          <EntitySectionCard
            title={t("session_packages.detail.sessions")}
            badge={pkg.sessions?.length ?? 0}
          >
            <p className="text-muted-foreground mb-3 text-sm">
              {t("session_packages.detail.sessions_hint")}
            </p>
            <EntityTable
              columns={columns}
              data={pkg.sessions ?? []}
              getRowId={(row) => row.uuid}
              onRowClick={(row) =>
                router.push(
                  routes.tenant.reservations.detail(slug, row.reservation.uuid),
                )
              }
              emptyTitle={t("session_packages.detail.sessions_empty")}
              manual={{ filtering: false, sorting: false, pagination: false }}
              features={{
                rowSelection: false,
                columnFilters: false,
                facetedFilters: false,
                globalFilter: false,
                sorting: false,
                pagination: false,
                viewMode: true,
                persistKey: "tenant-customer-package-sessions-v1",
              }}
            />
          </EntitySectionCard>

          {canRefund ? (
            <RefundDialog
              pkg={pkg}
              open={refunding}
              onOpenChange={setRefunding}
            />
          ) : null}
        </div>
      ) : null}
    </EntityPage>
  );
}
