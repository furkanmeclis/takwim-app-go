"use client";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import {
  EntitlementLimitsCard,
  EntitlementUsageCard,
} from "@/features/entitlements/components/entitlement-overview";
import { useTenantEntitlements } from "@/features/entitlements/hooks/use-entitlements";
import { useLocale } from "@/providers/locale-provider";

/**
 * Settings → Plan ve kullanım (TEC-883): the organization's plan, add-ons,
 * branch / staff seats against their limits and this month's communication
 * usage. Read-only; the platform operator changes the plan.
 */
export function PlanUsagePage() {
  const { t } = useLocale();
  const query = useTenantEntitlements();
  return (
    <EntityPage
      title={t("entitlements.title")}
      description={t("entitlements.page_description")}
      permission={permissions.tenant.settings.read}
      forbiddenFallback={<ErrorState title={t("common.error_forbidden")} />}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {query.data ? (
        <div className="grid gap-6 lg:grid-cols-2">
          <EntitlementLimitsCard summary={query.data} />
          <EntitlementUsageCard summary={query.data} />
        </div>
      ) : null}
    </EntityPage>
  );
}
