"use client";

import { Minus, Plus } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { PermissionGuard } from "@/components/common/permission-guard";
import { EntityActions, EntityPage } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useBusinessToday } from "@/features/cash/lib/format";
import { EntryDialog } from "@/features/finance/components/entry-dialog";
import { FinanceCategories } from "@/features/finance/components/finance-categories";
import { FinanceLedgerTable } from "@/features/finance/components/finance-ledger-table";
import { FinanceSummary } from "@/features/finance/components/finance-summary";
import { PeriodPicker } from "@/features/staff-shares/components/period-picker";
import type { FinanceKind, FinancePeriod } from "@/features/finance/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

type Tab = "summary" | "entries" | "categories";

/**
 * Income and expenses of a period: the figures kept apart (sale revenue,
 * other income, expenses, staff payouts, net), the combined list in which
 * sale lines appear as income, and the categories. Money in or out outside
 * sales is recorded here and posts its cash movement.
 */
export function FinancePage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const today = useBusinessToday();
  const [period, setPeriod] = useState<FinancePeriod>(() => ({
    date_from: `${today.slice(0, 8)}01`,
    date_to: today,
  }));
  const [tab, setTab] = useState<Tab>("summary");
  const [entryKind, setEntryKind] = useState<FinanceKind | null>(null);

  return (
    <EntityPage
      title={t("finance.title")}
      description={t("finance.description")}
      permission={permissions.tenant.finance.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("finance.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("finance.title") },
      ]}
      actions={
        <EntityActions>
          <PermissionGuard permission={permissions.tenant.finance.write}>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => setEntryKind("income")}
            >
              <Plus className="size-4" />
              {t("finance.actions.add_income")}
            </Button>
            <Button
              type="button"
              size="sm"
              onClick={() => setEntryKind("expense")}
            >
              <Minus className="size-4" />
              {t("finance.actions.add_expense")}
            </Button>
          </PermissionGuard>
        </EntityActions>
      }
    >
      <div className="space-y-6">
        <PeriodPicker value={period} onChange={setPeriod} />
        <Tabs
          value={tab}
          onValueChange={(value) => setTab(value as Tab)}
          className="space-y-4"
        >
          <TabsList>
            <TabsTrigger value="summary">
              {t("finance.tabs.summary")}
            </TabsTrigger>
            <TabsTrigger value="entries">
              {t("finance.tabs.entries")}
            </TabsTrigger>
            <TabsTrigger value="categories">
              {t("finance.tabs.categories")}
            </TabsTrigger>
          </TabsList>
          <TabsContent value="summary">
            {tab === "summary" ? <FinanceSummary period={period} /> : null}
          </TabsContent>
          <TabsContent value="entries">
            {tab === "entries" ? <FinanceLedgerTable period={period} /> : null}
          </TabsContent>
          <TabsContent value="categories">
            {tab === "categories" ? <FinanceCategories /> : null}
          </TabsContent>
        </Tabs>
      </div>
      <EntryDialog
        open={entryKind !== null}
        kind={entryKind ?? "expense"}
        onOpenChange={(open) => {
          if (!open) setEntryKind(null);
        }}
      />
    </EntityPage>
  );
}
