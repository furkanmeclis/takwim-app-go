"use client";

import { EntitySectionCard, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import {
  ADDON_LABEL_KEYS,
  PLAN_LABEL_KEYS,
  USAGE_METRIC_LABEL_KEYS,
} from "@/features/entitlements/constants";
import type {
  EntitlementSummary,
  UsageRow,
} from "@/features/entitlements/types";
import { useLocale } from "@/providers/locale-provider";

/** used / limit with a bar; a null limit reads "unlimited". */
export function LimitMeter({
  label,
  used,
  limit,
}: {
  label: string;
  used: number;
  limit: number | null;
}) {
  const { t, locale } = useLocale();
  const full = limit !== null && used >= limit;
  return (
    <div className="space-y-2">
      <div className="flex items-baseline justify-between gap-2 text-sm">
        <span className="font-medium">{label}</span>
        <span className={full ? "text-destructive" : "text-muted-foreground"}>
          {limit === null
            ? t("entitlements.used_unlimited", {
                used: used.toLocaleString(locale),
              })
            : t("entitlements.used_of", {
                used: used.toLocaleString(locale),
                limit: limit.toLocaleString(locale),
              })}
        </span>
      </div>
      {limit !== null ? (
        <Progress value={Math.min(100, (used / Math.max(limit, 1)) * 100)} />
      ) : null}
    </div>
  );
}

/** Plan, add-ons and the branch / staff limits of one organization. */
export function EntitlementLimitsCard({
  summary,
}: {
  summary: EntitlementSummary;
}) {
  const { t } = useLocale();
  return (
    <EntitySectionCard title={t("entitlements.limits_title")}>
      <div className="space-y-5">
        <div className="flex flex-wrap items-center gap-2">
          <Badge>{t(PLAN_LABEL_KEYS[summary.plan])}</Badge>
          {summary.addons.map((addon) => (
            <Badge key={addon} variant="secondary">
              {t(ADDON_LABEL_KEYS[addon])}
            </Badge>
          ))}
        </div>
        <LimitMeter
          label={t("entitlements.branches")}
          used={summary.used.branches}
          limit={summary.branch_limit}
        />
        <LimitMeter
          label={t("entitlements.staff")}
          used={summary.used.staff}
          limit={summary.staff_limit}
        />
        <p className="text-muted-foreground text-xs">
          {t("entitlements.staff_hint")}
        </p>
      </div>
    </EntitySectionCard>
  );
}

/** This month's communication usage counters. */
export function EntitlementUsageCard({
  summary,
}: {
  summary: EntitlementSummary;
}) {
  const { t, locale } = useLocale();
  const columns = [
    createColumn<UsageRow>({
      accessorKey: "metric",
      labelKey: "entitlements.metric",
      enableSorting: false,
      gridPrimary: true,
      cell: ({ row }) => t(USAGE_METRIC_LABEL_KEYS[row.original.metric]),
    }),
    createColumn<UsageRow>({
      accessorKey: "quantity",
      labelKey: "entitlements.quantity",
      enableSorting: false,
      gridSecondary: true,
      cell: ({ row }) => row.original.quantity.toLocaleString(locale),
    }),
  ];
  return (
    <EntitySectionCard
      title={t("entitlements.usage_title", { period: summary.period })}
    >
      <EntityTable
        columns={columns}
        data={summary.usage}
        getRowId={(row) => row.metric}
        emptyTitle={t("entitlements.usage_empty")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "entitlements-usage-v1",
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          sorting: false,
          pagination: false,
        }}
      />
    </EntitySectionCard>
  );
}
