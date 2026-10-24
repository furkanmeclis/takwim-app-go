"use client";

import { Plus, ShoppingBag } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatsCard } from "@/components/common/stats-card";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useBusinessToday, useCashAmount } from "@/features/cash/lib/format";
import { useNavModules } from "@/features/nav-engine/lib/modules";
import { CustomerPackagesTable } from "@/features/session-packages/components/customer-packages-table";
import { PackageDefinitionsTable } from "@/features/session-packages/components/package-definitions-table";
import { SellPackageDialog } from "@/features/session-packages/components/sell-package-dialog";
import { usePackageSummary } from "@/features/session-packages/hooks/use-session-packages";
import type { PackagePeriod } from "@/features/session-packages/types";
import { PeriodPicker } from "@/features/staff-shares/components/period-picker";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type Tab = "sold" | "definitions";

/**
 * Multi-session packages (TEC-920): the package report of a period (money
 * taken for packages, sessions used, refunds; open, held and expired
 * rights today), the packages sold to customers and the definitions.
 * Package sales are a cash figure here; the sessions' value reaches the
 * sales report through the visits, so nothing is counted twice.
 */
export function PackagesPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const hasModule = useNavModules();
  const money = useCashAmount();
  const today = useBusinessToday();
  const [period, setPeriod] = useState<Required<PackagePeriod>>(() => ({
    date_from: `${today.slice(0, 8)}01`,
    date_to: today,
  }));
  const [tab, setTab] = useState<Tab>("sold");
  const [selling, setSelling] = useState(false);
  const [creating, setCreating] = useState(false);
  const summary = usePackageSummary(period);
  const canWrite = can(permissions.tenant.sessionPackages.write);
  const canSell =
    hasModule("cash") &&
    can(permissions.tenant.sessionPackages.sell) &&
    can(permissions.tenant.payment.collect);

  return (
    <EntityPage
      title={t("session_packages.title")}
      description={t("session_packages.description")}
      permission={permissions.tenant.sessionPackages.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("session_packages.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("session_packages.title") },
      ]}
      actions={
        <>
          <PeriodPicker value={period} onChange={setPeriod} />
          {canSell ? (
            <Button onClick={() => setSelling(true)}>
              <ShoppingBag className="size-4" />
              {t("session_packages.actions.sell")}
            </Button>
          ) : null}
        </>
      }
    >
      <div className="space-y-6">
        {summary.isError ? (
          <ErrorState
            title={t("common.error_generic")}
            description={t("session_packages.summary.load_failed")}
            onRetry={() => void summary.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : null}

        {(summary.data?.totals ?? []).map((f) => (
          <div key={f.currency} className="space-y-2">
            {(summary.data?.totals.length ?? 0) > 1 ? (
              <h3 className="text-muted-foreground text-sm font-medium">
                {f.currency}
              </h3>
            ) : null}
            <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
              <StatsCard
                title={t("session_packages.summary.sold")}
                value={money(f.sold.amount, f.currency)}
                hint={t("session_packages.summary.sold_hint", {
                  count: f.sold.count,
                })}
              />
              <StatsCard
                title={t("session_packages.summary.used")}
                value={money(f.used.amount, f.currency)}
                hint={t("session_packages.summary.used_hint", {
                  sessions: f.used.sessions,
                })}
              />
              <StatsCard
                title={t("session_packages.summary.open")}
                value={money(f.open.amount, f.currency)}
                hint={t("session_packages.summary.open_hint", {
                  sessions: f.open.sessions,
                  held: f.held.sessions,
                })}
              />
              <StatsCard
                title={t("session_packages.summary.refunded")}
                value={money(f.refunded.amount, f.currency)}
                hint={t("session_packages.summary.refunded_hint", {
                  count: f.refunded.count,
                  expired: f.expired.sessions,
                  amount: money(f.expired.amount, f.currency),
                })}
              />
            </div>
          </div>
        ))}
        <p className="text-muted-foreground text-xs">
          {t("session_packages.summary.note")}
        </p>

        <Tabs
          value={tab}
          onValueChange={(value) => setTab(value as Tab)}
          className="space-y-4"
        >
          <TabsList>
            <TabsTrigger value="sold">
              {t("session_packages.tabs.sold")}
            </TabsTrigger>
            <TabsTrigger value="definitions">
              {t("session_packages.tabs.definitions")}
            </TabsTrigger>
          </TabsList>

          <TabsContent value="sold">
            {tab === "sold" ? (
              <CustomerPackagesTable persistKey="tenant-customer-packages-v1" />
            ) : null}
          </TabsContent>

          <TabsContent value="definitions">
            {tab === "definitions" ? (
              <EntitySectionCard
                title={t("session_packages.tabs.definitions")}
                actions={
                  canWrite ? (
                    <Button
                      type="button"
                      size="sm"
                      onClick={() => setCreating(true)}
                    >
                      <Plus className="size-4" />
                      {t("session_packages.actions.create")}
                    </Button>
                  ) : null
                }
              >
                <p className="text-muted-foreground mb-4 text-sm">
                  {t("session_packages.definitions_hint")}
                </p>
                <PackageDefinitionsTable
                  canWrite={canWrite}
                  creating={creating}
                  onCreatingChange={setCreating}
                />
              </EntitySectionCard>
            ) : null}
          </TabsContent>
        </Tabs>
      </div>
      {canSell ? (
        <SellPackageDialog open={selling} onOpenChange={setSelling} />
      ) : null}
    </EntityPage>
  );
}
