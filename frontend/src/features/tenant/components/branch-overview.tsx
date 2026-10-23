"use client";

import { useQuery } from "@tanstack/react-query";
import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { StatsCard } from "@/components/common/stats-card";
import { StatusChip } from "@/components/common/status-chip";
import { EntitySectionCard, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Skeleton } from "@/components/ui/skeleton";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import {
  tenantKeys,
  tenantService,
  type TenantOverviewBranch,
} from "@/features/tenant/services/tenant.service";
import { useLocale } from "@/providers/locale-provider";

function todayLabel(
  t: (key: string, params?: Record<string, string | number>) => string,
  branch: TenantOverviewBranch,
) {
  const { today } = branch;
  if (today.closed) return t("tenant.overview.closed_today");
  return t("tenant.overview.hours", { open: today.open, close: today.close });
}

/**
 * Central panel (requirements §3): the branches the member sees, in total
 * (stat cards) and one by one (table). The header branch picker narrows both
 * to one branch, like every other list.
 */
export function BranchOverview() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useQuery({
    queryKey: tenantKeys.overview(slug),
    queryFn: () => tenantService.getOverview(slug),
    enabled: Boolean(slug),
  });

  if (query.isError) {
    return (
      <ErrorState
        title={t("tenant.overview.load_failed")}
        onRetry={() => void query.refetch()}
      />
    );
  }

  const data = query.data;
  const totals = data?.totals;
  const branches = data?.branches ?? [];
  const n = (v: number | undefined) =>
    v === undefined ? "" : v.toLocaleString(locale);
  const stat = (value: string) =>
    query.isLoading ? <Skeleton className="h-8 w-16" /> : value;

  const columns = [
    createColumn<TenantOverviewBranch>({
      accessorKey: "name",
      labelKey: "tenant.overview.columns.branch",
      enableSorting: false,
      gridPrimary: true,
      cell: ({ row }) => (
        <div className="min-w-0">
          <p className="truncate font-medium">{row.original.name}</p>
          <p className="text-muted-foreground truncate text-xs">
            {[row.original.code, row.original.city].filter(Boolean).join(" · ")}
          </p>
        </div>
      ),
    }),
    createColumn<TenantOverviewBranch>({
      accessorKey: "open_now",
      labelKey: "tenant.overview.columns.now",
      enableSorting: false,
      gridSecondary: true,
      cell: ({ row }) =>
        !row.original.is_active ? (
          <StatusChip label={t("tenant.overview.inactive")} />
        ) : row.original.open_now ? (
          <StatusChip label={t("tenant.overview.open")} tone="success" />
        ) : (
          <StatusChip label={t("tenant.overview.closed")} tone="warning" />
        ),
    }),
    createColumn<TenantOverviewBranch>({
      id: "today",
      labelKey: "tenant.overview.columns.today",
      enableSorting: false,
      cell: ({ row }) => (
        <span className="text-sm">
          {todayLabel(t, row.original)}
          {row.original.today.exception && row.original.today.note ? (
            <span className="text-muted-foreground block text-xs">
              {row.original.today.note}
            </span>
          ) : null}
        </span>
      ),
    }),
    createColumn<TenantOverviewBranch>({
      accessorKey: "member_count",
      labelKey: "tenant.overview.columns.members",
      enableSorting: false,
      cell: ({ row }) => row.original.member_count.toLocaleString(locale),
    }),
    createColumn<TenantOverviewBranch>({
      accessorKey: "show_online",
      labelKey: "tenant.overview.columns.online",
      enableSorting: false,
      cell: ({ row }) =>
        row.original.show_online && row.original.is_active ? (
          <StatusChip label={t("tenant.overview.online")} tone="success" />
        ) : (
          <StatusChip label={t("tenant.overview.hidden")} />
        ),
    }),
  ];

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatsCard
          title={t("tenant.overview.stats.branches")}
          value={stat(n(totals?.branches))}
          hint={
            totals
              ? t("tenant.overview.stats.active_hint", {
                  count: n(totals.active_branches),
                })
              : undefined
          }
        />
        <StatsCard
          title={t("tenant.overview.stats.open_now")}
          value={stat(n(totals?.open_now))}
        />
        <StatsCard
          title={t("tenant.overview.stats.online")}
          value={stat(n(totals?.online_branches))}
        />
        <StatsCard
          title={t("tenant.overview.stats.members")}
          value={stat(n(totals?.members))}
        />
      </div>
      <EntitySectionCard
        title={t("tenant.overview.branches_title")}
        badge={query.isLoading ? undefined : branches.length}
      >
        <EntityTable
          columns={columns}
          data={branches}
          isLoading={query.isLoading}
          getRowId={(row) => row.uuid}
          onRowClick={(row) =>
            router.push(routes.tenant.branches.detail(slug, row.uuid))
          }
          emptyTitle={t("tenant.overview.no_branches")}
          manual={{ filtering: false, sorting: false, pagination: false }}
          features={{
            persistKey: "tenant-overview-branches-v1",
            rowSelection: false,
            columnFilters: false,
            facetedFilters: false,
            sorting: false,
            pagination: false,
          }}
        />
      </EntitySectionCard>
    </div>
  );
}
