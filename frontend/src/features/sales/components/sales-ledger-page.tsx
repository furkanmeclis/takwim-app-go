"use client";

import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useBusinessToday } from "@/features/cash/lib/format";
import { LedgerEntriesTable } from "@/features/sales/components/ledger-entries-table";
import { SaleLinesTable } from "@/features/sales/components/sale-lines-table";
import { SalesSummary } from "@/features/sales/components/sales-summary";
import type { SalesPeriod } from "@/features/sales/types";
import { PeriodPicker } from "@/features/staff-shares/components/period-picker";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

type Tab = "lines" | "entries";

/**
 * The sales ledger (TEC-973): revenue, collections, transfers and staff
 * payouts of the period kept apart (A20), then the sale lines and the
 * customer ledger. Both lists are append-only; a wrong sale line is
 * reversed or corrected from its drawer. Default period: today.
 */
export function SalesLedgerPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const today = useBusinessToday();
  const [period, setPeriod] = useState<SalesPeriod>(() => ({
    date_from: today,
    date_to: today,
  }));
  const [tab, setTab] = useState<Tab>("lines");

  return (
    <EntityPage
      title={t("sales.page_title")}
      description={t("sales.page_description")}
      permission={permissions.tenant.finance.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("sales.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("sales.page_title") },
      ]}
    >
      <div className="space-y-6">
        <PeriodPicker value={period} onChange={setPeriod} />
        <SalesSummary period={period} />
        <Tabs
          value={tab}
          onValueChange={(value) => setTab(value as Tab)}
          className="space-y-4"
        >
          <TabsList>
            <TabsTrigger value="lines">{t("sales.tabs.lines")}</TabsTrigger>
            <TabsTrigger value="entries">{t("sales.tabs.entries")}</TabsTrigger>
          </TabsList>
          <TabsContent value="lines">
            {tab === "lines" ? <SaleLinesTable period={period} /> : null}
          </TabsContent>
          <TabsContent value="entries">
            {tab === "entries" ? <LedgerEntriesTable period={period} /> : null}
          </TabsContent>
        </Tabs>
      </div>
    </EntityPage>
  );
}
