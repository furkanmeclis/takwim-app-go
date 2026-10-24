"use client";

import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatsCard } from "@/components/common/stats-card";
import {
  EntityPage,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useBusinessToday, useCashAmount } from "@/features/cash/lib/format";
import { PayoutDialog } from "@/features/staff-shares/components/payout-dialog";
import { PeriodPicker } from "@/features/staff-shares/components/period-picker";
import {
  StaffShareLinesTable,
  StaffSharePayoutsTable,
} from "@/features/staff-shares/components/staff-share-lists";
import {
  useStaffShareBranchColumns,
  useStaffShareSummaryColumns,
} from "@/features/staff-shares/components/staff-share-summary-columns";
import { useStaffSharesSummary } from "@/features/staff-shares/hooks/use-staff-shares";
import type {
  StaffSharePeriod,
  StaffShareSummaryRow,
} from "@/features/staff-shares/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type Tab = "summary" | "lines" | "payouts";

/**
 * Staff shares of a period: totals (revenue, staff share, business share),
 * one row per staff member with the payable balance and a payout, the
 * calculated shares and the payouts. The branch picked in the header
 * narrows the period figures; balances run over every branch and day.
 */
export function StaffSharesPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const money = useCashAmount();
  const today = useBusinessToday();
  const [period, setPeriod] = useState<Required<StaffSharePeriod>>(() => ({
    date_from: `${today.slice(0, 8)}01`,
    date_to: today,
  }));
  const [tab, setTab] = useState<Tab>("summary");
  const [payRow, setPayRow] = useState<StaffShareSummaryRow | null>(null);
  const onPay = useCallback((row: StaffShareSummaryRow) => setPayRow(row), []);

  const query = useStaffSharesSummary(period);
  const data = query.data;
  const canPay = can(permissions.tenant.earnings.pay);
  const staffColumns = useStaffShareSummaryColumns({
    onPay: canPay ? onPay : undefined,
  });
  const branchColumns = useStaffShareBranchColumns();
  const listPeriod = useMemo(() => period, [period]);

  return (
    <EntityPage
      title={t("staff_shares.title")}
      description={t("staff_shares.description")}
      permission={permissions.tenant.earnings.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("staff_shares.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("staff_shares.title") },
      ]}
      actions={<PeriodPicker value={period} onChange={setPeriod} />}
    >
      <div className="space-y-6">
        {query.isError ? (
          <ErrorState
            title={t("common.error_generic")}
            description={t("staff_shares.summary.load_failed")}
            onRetry={() => void query.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : null}

        {(data?.totals ?? []).map((totals) => (
          <div key={totals.currency} className="space-y-2">
            {(data?.totals.length ?? 0) > 1 ? (
              <h3 className="text-muted-foreground text-sm font-medium">
                {totals.currency}
              </h3>
            ) : null}
            <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
              <StatsCard
                title={t("staff_shares.summary.columns.revenue")}
                value={money(totals.revenue, totals.currency)}
              />
              <StatsCard
                title={t("staff_shares.summary.columns.share")}
                value={money(totals.share, totals.currency)}
                hint={t("staff_shares.summary.pending_hint", {
                  amount: money(totals.pending, totals.currency),
                })}
              />
              <StatsCard
                title={t("staff_shares.summary.columns.business_share")}
                value={money(totals.business_share, totals.currency)}
              />
              <StatsCard
                title={t("staff_shares.summary.columns.period_paid")}
                value={money(totals.period_paid, totals.currency)}
              />
            </div>
          </div>
        ))}

        <Tabs
          value={tab}
          onValueChange={(value) => setTab(value as Tab)}
          className="space-y-4"
        >
          <TabsList>
            <TabsTrigger value="summary">
              {t("staff_shares.tabs.summary")}
            </TabsTrigger>
            <TabsTrigger value="lines">
              {t("staff_shares.tabs.lines")}
            </TabsTrigger>
            <TabsTrigger value="payouts">
              {t("staff_shares.tabs.payouts")}
            </TabsTrigger>
          </TabsList>

          <TabsContent value="summary" className="space-y-6">
            <EntitySectionCard
              title={t("staff_shares.summary.staff_title")}
              badge={data ? data.staff.length : undefined}
            >
              <p className="text-muted-foreground mb-4 text-sm">
                {t("staff_shares.summary.staff_description")}
              </p>
              <EntityTable
                columns={staffColumns}
                data={data?.staff ?? []}
                getRowId={(row) => `${row.staff.uuid}-${row.currency}`}
                isLoading={query.isLoading}
                isError={query.isError}
                onRetry={() => void query.refetch()}
                emptyTitle={t("staff_shares.summary.empty_title")}
                emptyDescription={t("staff_shares.summary.empty_description")}
                manual={{ filtering: false, sorting: false, pagination: false }}
                features={{
                  persistKey: "tenant-staff-shares-summary-v1",
                  rowSelection: false,
                  columnFilters: false,
                  facetedFilters: false,
                  globalFilter: false,
                  sorting: false,
                  pagination: false,
                  viewMode: true,
                }}
              />
            </EntitySectionCard>
            <EntitySectionCard
              title={t("staff_shares.summary.branches_title")}
              badge={data ? data.branches.length : undefined}
            >
              <EntityTable
                columns={branchColumns}
                data={data?.branches ?? []}
                getRowId={(row) => `${row.branch.uuid}-${row.currency}`}
                isLoading={query.isLoading}
                isError={query.isError}
                onRetry={() => void query.refetch()}
                emptyTitle={t("staff_shares.summary.empty_title")}
                emptyDescription={t("staff_shares.summary.empty_description")}
                manual={{ filtering: false, sorting: false, pagination: false }}
                features={{
                  persistKey: "tenant-staff-shares-branches-v1",
                  rowSelection: false,
                  columnFilters: false,
                  facetedFilters: false,
                  globalFilter: false,
                  sorting: false,
                  pagination: false,
                  viewMode: true,
                }}
              />
            </EntitySectionCard>
          </TabsContent>

          <TabsContent value="lines">
            {tab === "lines" ? (
              <StaffShareLinesTable
                period={listPeriod}
                showStaff
                live
                persistKey="tenant-staff-share-lines-v1"
              />
            ) : null}
          </TabsContent>

          <TabsContent value="payouts">
            {tab === "payouts" ? (
              <StaffSharePayoutsTable
                period={listPeriod}
                showStaff
                live
                canReverse={canPay}
                persistKey="tenant-staff-share-payouts-v1"
              />
            ) : null}
          </TabsContent>
        </Tabs>
      </div>
      <PayoutDialog
        row={payRow}
        open={Boolean(payRow)}
        onOpenChange={(open) => {
          if (!open) setPayRow(null);
        }}
      />
    </EntityPage>
  );
}
