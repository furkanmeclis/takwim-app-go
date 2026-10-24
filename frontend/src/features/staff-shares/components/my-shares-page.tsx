"use client";

import { useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatsCard } from "@/components/common/stats-card";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useBusinessToday, useCashAmount } from "@/features/cash/lib/format";
import { PeriodPicker } from "@/features/staff-shares/components/period-picker";
import {
  StaffShareLinesTable,
  StaffSharePayoutsTable,
} from "@/features/staff-shares/components/staff-share-lists";
import { useMyStaffShares } from "@/features/staff-shares/hooks/use-staff-shares";
import type { StaffSharePeriod } from "@/features/staff-shares/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { ApiError } from "@/lib/api/errors";
import { date as formatDate } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * The member's own shares: today, the picked period and what is still to
 * be paid, then their own calculated shares and payouts. Works with only
 * `tenant.earnings.own`; the API answers with the member's staff record.
 */
export function MySharesPage() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const money = useCashAmount();
  const today = useBusinessToday();
  const [period, setPeriod] = useState<Required<StaffSharePeriod>>(() => ({
    date_from: `${today.slice(0, 8)}01`,
    date_to: today,
  }));
  const query = useMyStaffShares(period);
  const data = query.data;
  const listPeriod = useMemo(() => period, [period]);
  const noStaff = query.error instanceof ApiError && query.error.status === 404;

  return (
    <EntityPage
      title={t("staff_shares.mine.title")}
      description={t("staff_shares.mine.description")}
      permission={[
        permissions.tenant.earnings.read,
        permissions.tenant.earnings.own,
      ]}
      permissionMode="any"
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("staff_shares.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("staff_shares.mine.title") },
      ]}
      actions={<PeriodPicker value={period} onChange={setPeriod} />}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {noStaff ? (
        <EmptyState
          title={t("staff_shares.mine.no_staff_title")}
          description={t("staff_shares.mine.no_staff_description")}
        />
      ) : query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("staff_shares.summary.load_failed")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {data ? (
        <div className="space-y-6">
          {data.period.length === 0 ? (
            <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
              <StatsCard
                title={t("staff_shares.mine.today")}
                value={money("0")}
              />
              <StatsCard
                title={t("staff_shares.mine.period")}
                value={money("0")}
              />
            </div>
          ) : null}
          {data.period.map((row) => {
            const daily = data.daily.find((d) => d.currency === row.currency);
            return (
              <div
                key={row.currency}
                className="grid grid-cols-2 gap-4 lg:grid-cols-4"
              >
                <StatsCard
                  title={t("staff_shares.mine.today")}
                  value={money(daily?.share ?? "0", row.currency)}
                  hint={formatDate(data.today, undefined, locale)}
                />
                <StatsCard
                  title={t("staff_shares.mine.period")}
                  value={money(row.share, row.currency)}
                  hint={t("staff_shares.mine.period_hint", {
                    sales: money(row.sales, row.currency),
                    count: row.sale_count,
                  })}
                />
                <StatsCard
                  title={t("staff_shares.summary.columns.payable_now")}
                  value={money(row.balance.payable_now, row.currency)}
                  hint={t("staff_shares.summary.pending_hint", {
                    amount: money(row.pending, row.currency),
                  })}
                />
                <StatsCard
                  title={t("staff_shares.summary.columns.paid_total")}
                  value={money(row.balance.paid, row.currency)}
                  hint={t("staff_shares.mine.remaining_hint", {
                    amount: money(row.balance.remaining, row.currency),
                  })}
                />
              </div>
            );
          })}

          <EntitySectionCard title={t("staff_shares.tabs.lines")}>
            <StaffShareLinesTable
              period={listPeriod}
              showStaff={false}
              live={false}
              persistKey="tenant-my-share-lines-v1"
            />
          </EntitySectionCard>
          <EntitySectionCard title={t("staff_shares.tabs.payouts")}>
            <StaffSharePayoutsTable
              period={listPeriod}
              showStaff={false}
              live={false}
              canReverse={false}
              persistKey="tenant-my-share-payouts-v1"
            />
          </EntitySectionCard>
        </div>
      ) : null}
    </EntityPage>
  );
}
