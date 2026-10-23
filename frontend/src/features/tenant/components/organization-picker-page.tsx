"use client";

import Link from "next/link";
import { Building2, ChevronRight, LayoutDashboard } from "lucide-react";

import { PageHeader } from "@/components/layout";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { routes } from "@/config/routes";
import { tenantRoleLabel } from "@/features/tenant/lib/role-label";
import {
  isOrganizationAccessExpired,
  isPlatformUser,
  userOrganizations,
} from "@/lib/auth/types";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

/** `/organizations`: choose which organization to open. */
export function OrganizationPickerPage() {
  const { t } = useLocale();
  const { user } = useAuth();
  const organizations = userOrganizations(user);

  return (
    <div className="mx-auto w-full max-w-2xl space-y-6">
      <PageHeader
        title={t("tenant.picker.title")}
        description={t("tenant.picker.description")}
        icon={<Building2 className="size-6" />}
      />

      {organizations.length === 0 ? (
        <Card>
          <CardContent className="flex flex-col items-start gap-3 py-6">
            <p className="text-muted-foreground text-sm">
              {t("tenant.switcher.empty")}
            </p>
            <Button asChild variant="outline">
              <Link href={routes.account.noAccess}>
                {t("tenant.picker.no_access_link")}
              </Link>
            </Button>
          </CardContent>
        </Card>
      ) : (
        <ul className="flex flex-col gap-3">
          {organizations.map((org) => {
            const unavailable =
              org.status === "suspended" || isOrganizationAccessExpired(org);
            return (
              <li key={org.uuid}>
                <Link
                  href={routes.tenant.home(org.slug)}
                  className="bg-card hover:bg-accent focus-visible:ring-ring flex items-center gap-3 rounded-xl border p-4 transition-colors focus-visible:ring-2 focus-visible:outline-none"
                >
                  <span className="bg-muted text-muted-foreground flex size-10 shrink-0 items-center justify-center rounded-lg">
                    <Building2 className="size-5" />
                  </span>
                  <span className="flex min-w-0 flex-1 flex-col">
                    <span className="truncate font-medium">{org.name}</span>
                    <span className="text-muted-foreground truncate text-sm">
                      {tenantRoleLabel(t, org.role)} · {org.slug}
                    </span>
                  </span>
                  {unavailable ? (
                    <Badge variant="warning" className="shrink-0">
                      {org.status === "suspended"
                        ? t("tenant.status.suspended")
                        : t("tenant.status.expired")}
                    </Badge>
                  ) : null}
                  <ChevronRight className="text-muted-foreground size-4 shrink-0" />
                </Link>
              </li>
            );
          })}
        </ul>
      )}

      {isPlatformUser(user) ? (
        <Button asChild variant="outline">
          <Link href={routes.platform.home}>
            <LayoutDashboard />
            {t("tenant.switcher.platform")}
          </Link>
        </Button>
      ) : null}
    </div>
  );
}
