"use client";

import { useQuery } from "@tanstack/react-query";
import { CalendarCheck, CalendarDays, ClipboardCheck } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatsCard } from "@/components/common/stats-card";
import { EntitySectionCard, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useBusinessToday, useCashAmount } from "@/features/cash/lib/format";
import { ReservationStatusChip } from "@/features/reservations/components/reservation-status-chips";
import { RESERVATIONS_RESOURCE } from "@/features/reservations/constants";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantResourceChannel } from "@/features/tenant/lib/live";
import {
  tenantKeys,
  tenantService,
  type DailySummaryBranch,
  type DailySummaryMoney,
  type DailySummaryVisit,
} from "@/features/tenant/services/tenant.service";
import { useChannel } from "@/hooks/use-realtime";
import { date as formatDate, datetimeInZone } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Any of these opens the daily summary (the API returns what each allows). */
const DAILY_READ = [
  permissions.tenant.reservations.read,
  permissions.tenant.reservations.own,
  permissions.tenant.finance.read,
  permissions.tenant.earnings.read,
  permissions.tenant.earnings.own,
];

function pick<T extends { currency: string }>(
  list: T[] | null | undefined,
  currency: string,
): T | undefined {
  if (!list?.length) return undefined;
  return list.find((m) => m.currency === currency) ?? list[0];
}

/**
 * Daily summary of the home page (TEC-926, requirements §11): the
 * member's own visits and earnings when it serves customers (an owner can
 * be on the calendar too), the visits and money of every branch it sees
 * (the header branch picker narrows it to one branch), one row per branch,
 * and what waits for the business. Each part follows the member's
 * permissions; a member without any of them does not see the section.
 */
export function DailySummary() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const { canAny, can } = usePermission();
  const money = useCashAmount();
  const today = useBusinessToday();
  const [date, setDate] = useState(today);
  const enabled = canAny(DAILY_READ);

  const query = useQuery({
    queryKey: tenantKeys.daily(slug, date),
    queryFn: () => tenantService.getDailySummary(slug, date),
    enabled: enabled && Boolean(slug),
    placeholderData: (previous) => previous,
  });
  useChannel(
    enabled &&
      organization?.uuid &&
      canAny([
        permissions.tenant.reservations.read,
        permissions.tenant.reservations.own,
      ])
      ? tenantResourceChannel(organization.uuid, RESERVATIONS_RESOURCE)
      : null,
    () => void query.refetch(),
  );

  const data = query.data;
  const zone = data?.timezone ?? organization?.timezone ?? "Europe/Istanbul";
  const currency = data?.currency ?? organization?.currency ?? "TRY";
  const fmt = (m: { currency: string } | undefined, value?: string) =>
    m && value !== undefined ? money(value, m.currency) : money("0", currency);
  const time = (value: string) =>
    datetimeInZone(value, zone, locale, { timeStyle: "short" });

  const branchColumns = useMemo(
    () => [
      createColumn<DailySummaryBranch>({
        id: "branch",
        labelKey: "tenant.daily.columns.branch",
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) =>
          row.original.branch ? (
            <span className="font-medium">{row.original.branch.name}</span>
          ) : (
            <span className="text-muted-foreground">
              {t("tenant.daily.head_office")}
            </span>
          ),
      }),
      createColumn<DailySummaryBranch>({
        id: "reservations",
        labelKey: "tenant.daily.columns.reservations",
        enableSorting: false,
        cell: ({ row }) =>
          row.original.reservations
            ? row.original.reservations.total.toLocaleString(locale)
            : "—",
      }),
      createColumn<DailySummaryBranch>({
        id: "revenue",
        labelKey: "tenant.daily.columns.revenue",
        enableSorting: false,
        gridSecondary: true,
        cell: ({ row }) => {
          const m = pick<DailySummaryMoney>(row.original.money, currency);
          return <span className="tabular-nums">{fmt(m, m?.revenue)}</span>;
        },
      }),
      createColumn<DailySummaryBranch>({
        id: "collections",
        labelKey: "tenant.daily.columns.collections",
        enableSorting: false,
        cell: ({ row }) => {
          const m = pick<DailySummaryMoney>(row.original.money, currency);
          return <span className="tabular-nums">{fmt(m, m?.collections)}</span>;
        },
      }),
      createColumn<DailySummaryBranch>({
        id: "staff_share",
        labelKey: "tenant.daily.columns.staff_share",
        enableSorting: false,
        cell: ({ row }) => {
          const m = pick<DailySummaryMoney>(row.original.money, currency);
          return <span className="tabular-nums">{fmt(m, m?.staff_share)}</span>;
        },
      }),
      createColumn<DailySummaryBranch>({
        id: "business_share",
        labelKey: "tenant.daily.columns.business_share",
        enableSorting: false,
        cell: ({ row }) => {
          const m = pick<DailySummaryMoney>(row.original.money, currency);
          return (
            <span className="font-medium tabular-nums">
              {fmt(m, m?.business_share)}
            </span>
          );
        },
      }),
    ],
    // fmt and pick read currency / money only.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [t, locale, currency, money],
  );

  const visitColumns = useMemo(
    () => [
      createColumn<DailySummaryVisit>({
        accessorKey: "starts_at",
        labelKey: "tenant.daily.columns.time",
        enableSorting: false,
        gridSecondary: true,
        cell: ({ row }) => (
          <span className="font-medium tabular-nums">
            {time(row.original.starts_at)}–{time(row.original.ends_at)}
          </span>
        ),
      }),
      createColumn<DailySummaryVisit>({
        accessorKey: "customer_name",
        labelKey: "tenant.daily.columns.customer",
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-medium">
            {row.original.customer_name} · #{row.original.number}
          </span>
        ),
      }),
      createColumn<DailySummaryVisit>({
        accessorKey: "service_name",
        labelKey: "tenant.daily.columns.service",
        enableSorting: false,
      }),
      createColumn<DailySummaryVisit>({
        accessorKey: "branch_name",
        labelKey: "tenant.daily.columns.branch",
        enableSorting: false,
      }),
      createColumn<DailySummaryVisit>({
        accessorKey: "status",
        labelKey: "tenant.daily.columns.status",
        enableSorting: false,
        cell: ({ row }) => (
          <ReservationStatusChip status={row.original.status} />
        ),
      }),
    ],
    // time reads zone / locale only.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [zone, locale],
  );

  if (!enabled) return null;
  if (query.isError) {
    return (
      <ErrorState
        title={t("tenant.daily.load_failed")}
        onRetry={() => void query.refetch()}
      />
    );
  }

  const loading = query.isLoading;
  const stat = (value: string | number | undefined) =>
    loading || value === undefined ? <Skeleton className="h-8 w-20" /> : value;
  const business = data?.business ?? null;
  const own = data?.own ?? null;
  const pending = data?.pending ?? null;
  const bizMoney = pick<DailySummaryMoney>(business?.money, currency);
  const ownEarning = pick(own?.earnings, currency);
  const branches = business?.branches ?? [];
  const isToday = date === today;
  const n = (v: number) => v.toLocaleString(locale);

  const countCards = [
    business?.reservations ? (
      <StatsCard
        key="business"
        title={t("tenant.daily.cards.business_reservations")}
        value={stat(n(business.reservations.total))}
        hint={t("tenant.daily.cards.business_reservations_hint", {
          completed: n(business.reservations.completed),
          open: n(
            business.reservations.active +
              business.reservations.in_service +
              business.reservations.requested,
          ),
          cancelled: n(business.reservations.cancelled),
        })}
      />
    ) : null,
    own ? (
      <StatsCard
        key="own"
        title={t("tenant.daily.cards.own_reservations")}
        value={stat(n(own.reservations.total))}
        hint={t("tenant.daily.cards.own_reservations_hint", {
          completed: n(own.reservations.completed),
          cancelled: n(own.reservations.cancelled),
        })}
      />
    ) : null,
    pending ? (
      <StatsCard
        key="approvals"
        title={t("tenant.daily.cards.approvals")}
        value={stat(n(pending.approvals))}
        hint={t("tenant.daily.cards.approvals_hint")}
      />
    ) : null,
    pending ? (
      <StatsCard
        key="confirmations"
        title={t("tenant.daily.cards.confirmations")}
        value={stat(n(pending.confirmations_waiting))}
        hint={t("tenant.daily.cards.confirmations_hint", {
          attention: n(pending.confirmations_attention),
        })}
      />
    ) : null,
  ].filter(Boolean);

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0">
          <h2 className="flex items-center gap-2 text-lg font-semibold">
            <CalendarDays className="size-5" />
            {isToday ? t("tenant.daily.title_today") : t("tenant.daily.title")}
          </h2>
          <p className="text-muted-foreground text-sm">
            {t("tenant.daily.description", {
              date: formatDate(date, undefined, locale),
            })}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <DatePicker
            className="w-full sm:w-44"
            value={date}
            onChange={(next) => next && setDate(next)}
          />
          {!isToday ? (
            <Button variant="outline" onClick={() => setDate(today)}>
              {t("tenant.daily.today")}
            </Button>
          ) : null}
        </div>
      </div>

      {loading || bizMoney ? (
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <StatsCard
            title={t("tenant.daily.cards.revenue")}
            value={stat(
              bizMoney ? money(bizMoney.revenue, bizMoney.currency) : undefined,
            )}
            hint={
              bizMoney
                ? t("tenant.daily.cards.revenue_hint", {
                    count: n(bizMoney.sale_count),
                  })
                : undefined
            }
          />
          <StatsCard
            title={t("tenant.daily.cards.collections")}
            value={stat(
              bizMoney
                ? money(bizMoney.collections, bizMoney.currency)
                : undefined,
            )}
            hint={t("tenant.daily.cards.collections_hint")}
          />
          <StatsCard
            title={t("tenant.daily.cards.staff_share")}
            value={stat(
              bizMoney
                ? money(bizMoney.staff_share, bizMoney.currency)
                : undefined,
            )}
            hint={t("tenant.daily.cards.staff_share_hint")}
          />
          <StatsCard
            title={t("tenant.daily.cards.business_share")}
            value={stat(
              bizMoney
                ? money(bizMoney.business_share, bizMoney.currency)
                : undefined,
            )}
            hint={t("tenant.daily.cards.business_share_hint")}
          />
        </div>
      ) : null}

      {countCards.length > 0 ? (
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          {countCards}
        </div>
      ) : null}

      {pending ? (
        <div className="flex flex-wrap gap-2">
          {pending.approvals > 0 ? (
            <Button asChild size="sm" variant="outline">
              <Link
                href={`${routes.tenant.reservations.root(slug)}?approval_status=pending`}
              >
                <ClipboardCheck className="size-4" />
                {t("tenant.daily.show_approvals")}
              </Link>
            </Button>
          ) : null}
          {pending.confirmations_waiting + pending.confirmations_attention >
          0 ? (
            <Button asChild size="sm" variant="outline">
              <Link href={routes.tenant.reservations.confirmations(slug)}>
                <CalendarCheck className="size-4" />
                {t("tenant.daily.show_confirmations")}
              </Link>
            </Button>
          ) : null}
        </div>
      ) : null}

      {business?.branches && branches.length > 1 ? (
        <EntitySectionCard
          title={t("tenant.daily.branches_title")}
          badge={branches.length}
        >
          <EntityTable
            columns={branchColumns}
            data={branches}
            isLoading={loading}
            getRowId={(row) => row.branch?.uuid ?? "head_office"}
            emptyTitle={t("tenant.daily.no_branches")}
            manual={{ filtering: false, sorting: false, pagination: false }}
            features={{
              persistKey: "tenant-daily-branches-v1",
              rowSelection: false,
              columnFilters: false,
              facetedFilters: false,
              sorting: false,
              pagination: false,
            }}
          />
        </EntitySectionCard>
      ) : null}

      {own ? (
        <EntitySectionCard
          title={t("tenant.daily.own_title", {
            name: data?.staff?.name ?? "",
          })}
          badge={own.visits.length}
          actions={
            <Button asChild size="sm" variant="outline">
              <Link href={routes.tenant.myAgenda(slug)}>
                {t("tenant.daily.open_agenda")}
              </Link>
            </Button>
          }
        >
          <div className="space-y-4">
            {own.earnings ? (
              <div className="grid grid-cols-2 gap-4">
                <StatsCard
                  title={t("tenant.daily.cards.own_sales")}
                  value={stat(
                    ownEarning
                      ? money(ownEarning.sales, ownEarning.currency)
                      : money("0", currency),
                  )}
                  hint={t("tenant.daily.cards.own_sales_hint", {
                    count: n(ownEarning?.sale_count ?? 0),
                  })}
                />
                <StatsCard
                  title={t("tenant.daily.cards.own_share")}
                  value={stat(
                    ownEarning
                      ? money(ownEarning.share, ownEarning.currency)
                      : money("0", currency),
                  )}
                  hint={t("tenant.daily.cards.own_share_hint")}
                />
              </div>
            ) : null}
            <EntityTable
              columns={visitColumns}
              data={own.visits}
              isLoading={loading}
              getRowId={(row) => `${row.reservation_uuid}:${row.line_index}`}
              onRowClick={(row) =>
                router.push(
                  routes.tenant.reservations.detail(slug, row.reservation_uuid),
                )
              }
              emptyTitle={t("tenant.daily.no_visits")}
              manual={{ filtering: false, sorting: false, pagination: false }}
              features={{
                persistKey: "tenant-daily-visits-v1",
                rowSelection: false,
                columnFilters: false,
                facetedFilters: false,
                sorting: false,
                pagination: false,
              }}
            />
          </div>
        </EntitySectionCard>
      ) : null}

      {!loading &&
      data &&
      !business &&
      !own &&
      can(permissions.tenant.reservations.own) ? (
        <p className="text-muted-foreground text-sm">
          {t("tenant.daily.no_staff_record")}
        </p>
      ) : null}
    </div>
  );
}
