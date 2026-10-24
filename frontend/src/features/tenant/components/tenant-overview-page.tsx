"use client";

import type { ReactNode } from "react";
import { Building2, KeyRound, UserRound } from "lucide-react";

import { PageHeader } from "@/components/layout";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { ConfirmationAlerts } from "@/features/confirmations/components/confirmation-alerts";
import { BranchOverview } from "@/features/tenant/components/branch-overview";
import { SectorOverviewCard } from "@/features/sectors/components/sector-overview-card";
import { SECTOR_LABEL_KEYS } from "@/features/sectors/constants";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantRoleLabel } from "@/features/tenant/lib/role-label";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function Row({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 py-1.5 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span className="min-w-0 truncate text-right font-medium">{value}</span>
    </div>
  );
}

/**
 * Landing page of the organization shell: the central panel of the branches
 * (in total and one by one), then the organization and your membership.
 */
export function TenantOverviewPage() {
  const { t, locale } = useLocale();
  const tenant = useTenant();
  const org = tenant.organization;
  const roleLabel = tenantRoleLabel(t, tenant.role);

  return (
    <div className="space-y-6">
      <PageHeader
        title={org?.name ?? tenant.slug}
        description={t("tenant.overview.description")}
        icon={<Building2 className="size-6" />}
      />

      <ConfirmationAlerts />

      <BranchOverview />

      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <Building2 className="size-4" />
              {t("tenant.overview.organization_title")}
            </CardTitle>
          </CardHeader>
          <CardContent className="divide-y">
            <Row
              label={t("organizations.name")}
              value={org?.name ?? tenant.slug}
            />
            <Row label={t("organizations.slug")} value={tenant.slug} />
            <Row
              label={t("sectors.label")}
              value={org?.sector ? t(SECTOR_LABEL_KEYS[org.sector]) : "—"}
            />
            <Row
              label={t("organizations.status")}
              value={
                org?.status === "suspended" ? (
                  <Badge variant="warning">
                    {t("tenant.status.suspended")}
                  </Badge>
                ) : (
                  <Badge variant="success">{t("tenant.status.active")}</Badge>
                )
              }
            />
            <Row
              label={t("organizations.access_ends_at")}
              value={
                org?.accessEndsAt
                  ? date(org.accessEndsAt, undefined, locale)
                  : t("tenant.overview.no_end")
              }
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <UserRound className="size-4" />
              {t("tenant.overview.membership_title")}
            </CardTitle>
            <CardDescription>
              {t("tenant.overview.membership_description")}
            </CardDescription>
          </CardHeader>
          <CardContent className="divide-y">
            <Row
              label={t("tenant.overview.role")}
              value={
                roleLabel ? <Badge variant="secondary">{roleLabel}</Badge> : "—"
              }
            />
            <Row
              label={t("tenant.overview.permissions")}
              value={
                <span className="inline-flex items-center gap-1.5">
                  <KeyRound className="text-muted-foreground size-3.5" />
                  {tenant.permissions.length}
                </span>
              }
            />
          </CardContent>
        </Card>
      </div>

      <SectorOverviewCard />
    </div>
  );
}
