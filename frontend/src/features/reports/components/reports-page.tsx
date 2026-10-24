"use client";

import { Download, Info } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatsCard } from "@/components/common/stats-card";
import {
  EntityPage,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useBusinessToday, useCashAmount } from "@/features/cash/lib/format";
import {
  branchId,
  useReportBranchColumns,
  useReportDayColumns,
  useReportStockColumns,
  useReportVisitColumns,
} from "@/features/reports/components/report-columns";
import { ReportFilters } from "@/features/reports/components/report-filters";
import {
  useReport,
  useReportExport,
} from "@/features/reports/hooks/use-reports";
import { percent } from "@/features/reports/lib/figures";
import type {
  Report,
  ReportFilter,
  ReportMoney,
  ReportSection,
} from "@/features/reports/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { date as formatDate } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type Tab = "days" | "branches" | "visits" | "stock";

const EMPTY: string[] = [];

function MoneyCards({
  money: m,
  omitted,
}: {
  money: ReportMoney;
  omitted: string[];
}) {
  const { t } = useLocale();
  const money = useCashAmount();
  const fmt = (value: string) => money(value, m.currency);
  const hidden = (key: string) => omitted.includes(key);
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatsCard
          title={t("reports.cards.revenue")}
          value={fmt(m.revenue.net)}
          hint={t("reports.cards.revenue_hint", {
            service: fmt(m.revenue.service_net),
            product: fmt(m.revenue.product_net),
          })}
        />
        <StatsCard
          title={t("reports.cards.collections")}
          value={hidden("collections") ? "—" : fmt(m.collections.net)}
          hint={
            hidden("collections")
              ? t("reports.cards.not_for_staff")
              : t("reports.cards.collections_hint", {
                  refunded: fmt(m.collections.refunded),
                })
          }
        />
        <StatsCard
          title={t("reports.cards.expenses")}
          value={hidden("expenses") ? "—" : fmt(m.expenses)}
          hint={
            hidden("expenses")
              ? t("reports.cards.not_for_staff")
              : t("reports.cards.expenses_hint", { income: fmt(m.income) })
          }
        />
        <StatsCard
          title={t("reports.cards.operational_result")}
          value={fmt(m.operational_result)}
          hint={t("reports.cards.operational_result_hint")}
        />
      </div>
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatsCard
          title={t("reports.cards.staff_share")}
          value={fmt(m.staff_share)}
          hint={t("reports.cards.staff_share_hint", {
            paid: fmt(m.staff_payments),
          })}
        />
        <StatsCard
          title={t("reports.cards.business_share")}
          value={fmt(m.business_share)}
          hint={t("reports.cards.business_share_hint")}
        />
        <StatsCard
          title={t("reports.cards.product_cost")}
          value={fmt(m.product_cost)}
          hint={t("reports.cards.product_cost_hint")}
        />
        <StatsCard
          title={t("reports.cards.transfers")}
          value={hidden("transfers") ? "—" : fmt(m.transfers)}
          hint={
            hidden("transfers")
              ? t("reports.cards.not_for_staff")
              : t("reports.cards.transfers_hint")
          }
        />
      </div>
    </div>
  );
}

function CountCards({ report }: { report: Report }) {
  const { t } = useLocale();
  const money = useCashAmount();
  const r = report.totals.reservations;
  const c = report.totals.confirmations;
  const s = report.totals.stock;
  const noAnswer = c.cancelled + c.flagged + c.unreachable;
  return (
    <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
      <StatsCard
        title={t("reports.cards.reservations")}
        value={r.total}
        hint={t("reports.cards.reservations_hint", {
          completed: r.completed,
          cancelled: r.cancelled,
          no_show: r.no_show,
        })}
      />
      <StatsCard
        title={t("reports.cards.confirmed")}
        value={c.confirmed}
        hint={t("reports.cards.confirmed_hint", {
          rate: percent(c.confirmed, c.total),
          total: c.total,
        })}
      />
      <StatsCard
        title={t("reports.cards.no_answer")}
        value={noAnswer}
        hint={t("reports.cards.no_answer_hint", {
          declined: c.declined,
          change: c.change_requested,
        })}
      />
      <StatsCard
        title={t("reports.cards.stock_sold")}
        value={s.sold}
        hint={t("reports.cards.stock_hint", {
          low: s.low_items,
          out: s.out_items,
          value: money(s.value, s.currency),
        })}
      />
    </div>
  );
}

/** What the figures cover, stated on the page (requirements §10). */
function ScopeNote({ report }: { report: Report }) {
  const { t, locale } = useLocale();
  const omitted = report.scope.omitted_for_staff;
  const lines = [
    t("reports.scope.period", {
      from: formatDate(report.date_from, undefined, locale),
      to: formatDate(report.date_to, undefined, locale),
      timezone: report.scope.timezone,
    }),
    report.scope.head_office
      ? t("reports.scope.head_office_included")
      : t("reports.scope.head_office_excluded"),
    t("reports.scope.formula"),
    t("reports.scope.separate"),
    omitted.length > 0
      ? t("reports.scope.staff_omitted", {
          fields: omitted.map((f) => t(`reports.omitted.${f}`)).join(", "),
        })
      : null,
    t("reports.scope.stock"),
    t("reports.scope.packages"),
  ].filter(Boolean);
  return (
    <Alert>
      <Info className="size-4" />
      <AlertTitle>{t("reports.scope.title")}</AlertTitle>
      <AlertDescription>
        <ul className="list-disc space-y-1 ps-4">
          {lines.map((line) => (
            <li key={line}>{line}</li>
          ))}
        </ul>
      </AlertDescription>
    </Alert>
  );
}

/**
 * Reports of a period (TEC-925): visits and confirmation results, revenue,
 * collections, transfers, expenses, staff share and payouts, the business
 * share and the operating result, and the stock. Per day, per branch and in
 * total, with CSV downloads. Each figure comes from its own record, so
 * revenue, collections, transfers and staff payouts never mix (A20).
 */
export function ReportsPage() {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const { can } = usePermission();
  const today = useBusinessToday();
  const [filter, setFilter] = useState<ReportFilter>(() => ({
    date_from: `${today.slice(0, 8)}01`,
    date_to: today,
  }));
  const [tab, setTab] = useState<Tab>("days");
  const query = useReport(filter);
  const exportCsv = useReportExport();
  const data = query.data;
  const currency = data?.scope.currency ?? organization?.currency ?? "TRY";
  const omitted = data?.scope.omitted_for_staff ?? EMPTY;

  const dayColumns = useReportDayColumns(currency, omitted);
  const visitColumns = useReportVisitColumns();
  const branchColumns = useReportBranchColumns(currency, omitted);
  const stockColumns = useReportStockColumns();

  const download = (section: ReportSection) =>
    exportCsv.mutate({ filter, section });

  const tableFeatures = (key: string) => ({
    persistKey: `tenant-reports-${key}-v1`,
    rowSelection: false,
    columnFilters: false,
    facetedFilters: false,
    globalFilter: false,
    sorting: false,
    pagination: false,
    viewMode: true,
  });
  const manual = { filtering: false, sorting: false, pagination: false };

  return (
    <EntityPage
      title={t("reports.title")}
      description={t("reports.description")}
      permission={permissions.tenant.finance.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("reports.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("reports.title") },
      ]}
      actions={
        can(permissions.tenant.report.export) ? (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                type="button"
                variant="outline"
                disabled={exportCsv.isPending}
              >
                <Download className="size-4" />
                {t("reports.export.button")}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onSelect={() => download("days")}>
                {t("reports.export.days")}
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => download("branches")}>
                {t("reports.export.branches")}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        ) : null
      }
    >
      <div className="space-y-6">
        <EntitySectionCard title={t("reports.filters.title")}>
          <ReportFilters value={filter} onChange={setFilter} />
        </EntitySectionCard>

        {query.isError ? (
          <ErrorState
            title={t("common.error_generic")}
            description={t("reports.load_failed")}
            onRetry={() => void query.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : null}

        {data ? (
          <>
            <ScopeNote report={data} />
            {data.totals.money.map((m) => (
              <div key={m.currency} className="space-y-2">
                {data.totals.money.length > 1 ? (
                  <h3 className="text-muted-foreground text-sm font-medium">
                    {m.currency}
                  </h3>
                ) : null}
                <MoneyCards money={m} omitted={omitted} />
              </div>
            ))}
            <CountCards report={data} />
          </>
        ) : null}

        <Tabs
          value={tab}
          onValueChange={(value) => setTab(value as Tab)}
          className="space-y-4"
        >
          <TabsList className="flex h-auto flex-wrap">
            <TabsTrigger value="days">{t("reports.tabs.days")}</TabsTrigger>
            <TabsTrigger value="branches">
              {t("reports.tabs.branches")}
            </TabsTrigger>
            <TabsTrigger value="visits">{t("reports.tabs.visits")}</TabsTrigger>
            <TabsTrigger value="stock">{t("reports.tabs.stock")}</TabsTrigger>
          </TabsList>

          <TabsContent value="days">
            <EntitySectionCard
              title={t("reports.days.title")}
              badge={data ? data.days.length : undefined}
            >
              <EntityTable
                columns={dayColumns}
                data={data?.days ?? []}
                getRowId={(row) => row.date}
                isLoading={query.isLoading}
                isError={query.isError}
                onRetry={() => void query.refetch()}
                emptyTitle={t("reports.empty_title")}
                emptyDescription={t("reports.empty_description")}
                manual={manual}
                features={tableFeatures("days")}
              />
            </EntitySectionCard>
          </TabsContent>

          <TabsContent value="branches">
            <EntitySectionCard
              title={t("reports.branches.title")}
              badge={data ? data.branches.length : undefined}
            >
              <p className="text-muted-foreground mb-4 text-sm">
                {t("reports.branches.description")}
              </p>
              <EntityTable
                columns={branchColumns}
                data={data?.branches ?? []}
                getRowId={branchId}
                isLoading={query.isLoading}
                isError={query.isError}
                onRetry={() => void query.refetch()}
                emptyTitle={t("reports.empty_title")}
                emptyDescription={t("reports.empty_description")}
                manual={manual}
                features={tableFeatures("branches")}
              />
            </EntitySectionCard>
          </TabsContent>

          <TabsContent value="visits">
            <EntitySectionCard
              title={t("reports.visits.title")}
              badge={data ? data.days.length : undefined}
            >
              <p className="text-muted-foreground mb-4 text-sm">
                {t("reports.visits.description")}
              </p>
              <EntityTable
                columns={visitColumns}
                data={data?.days ?? []}
                getRowId={(row) => row.date}
                isLoading={query.isLoading}
                isError={query.isError}
                onRetry={() => void query.refetch()}
                emptyTitle={t("reports.empty_title")}
                emptyDescription={t("reports.empty_description")}
                manual={manual}
                features={tableFeatures("visits")}
              />
            </EntitySectionCard>
          </TabsContent>

          <TabsContent value="stock">
            <EntitySectionCard
              title={t("reports.stock.title")}
              badge={data ? data.branches.length : undefined}
            >
              <p className="text-muted-foreground mb-4 text-sm">
                {t("reports.stock.description")}
              </p>
              <EntityTable
                columns={stockColumns}
                data={(data?.branches ?? []).filter((b) => b.branch !== null)}
                getRowId={branchId}
                isLoading={query.isLoading}
                isError={query.isError}
                onRetry={() => void query.refetch()}
                emptyTitle={t("reports.empty_title")}
                emptyDescription={t("reports.stock.empty_description")}
                manual={manual}
                features={tableFeatures("stock")}
              />
            </EntitySectionCard>
          </TabsContent>
        </Tabs>
      </div>
    </EntityPage>
  );
}
