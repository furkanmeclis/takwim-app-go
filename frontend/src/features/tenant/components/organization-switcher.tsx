"use client";

import Link from "next/link";
import { Check, ChevronsUpDown, LayoutDashboard } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { routes } from "@/config/routes";
import { OrganizationAvatar } from "@/features/tenant/components/organization-avatar";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantRoleLabel } from "@/features/tenant/lib/role-label";
import {
  isOrganizationAccessExpired,
  isPlatformUser,
  userOrganizations,
} from "@/lib/auth/types";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

/**
 * Header switcher of the tenant shell: lists the user's organizations
 * (`/me` `organizations[]`). Switching is plain navigation to another
 * `/t/{slug}`; nothing is stored in the session.
 */
export function OrganizationSwitcher() {
  const { t } = useLocale();
  const { user } = useAuth();
  const tenant = useTenant();
  const organizations = userOrganizations(user);
  const currentName = tenant.organization?.name ?? tenant.slug;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          className="h-9 max-w-64 min-w-0 gap-2 px-2"
          aria-label={t("tenant.switcher.label")}
        >
          <OrganizationAvatar
            logoUrl={tenant.organization?.logoUrl}
            className="size-6 rounded-md"
          />
          <span className="truncate text-sm font-medium">{currentName}</span>
          <ChevronsUpDown className="text-muted-foreground size-3.5 shrink-0" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-72">
        <DropdownMenuLabel className="text-muted-foreground text-xs font-normal">
          {t("tenant.switcher.title")}
        </DropdownMenuLabel>
        <DropdownMenuGroup>
          {organizations.length === 0 ? (
            <DropdownMenuItem disabled>
              {t("tenant.switcher.empty")}
            </DropdownMenuItem>
          ) : (
            organizations.map((org) => {
              const current = org.slug === tenant.slug;
              const unavailable =
                org.status === "suspended" || isOrganizationAccessExpired(org);
              return (
                <DropdownMenuItem key={org.uuid} asChild>
                  <Link
                    href={routes.tenant.home(org.slug)}
                    aria-current={current ? "page" : undefined}
                    className="flex items-center gap-2"
                  >
                    <OrganizationAvatar
                      logoUrl={org.logoUrl}
                      className="size-6 rounded-md"
                    />
                    <span className="flex min-w-0 flex-1 flex-col">
                      <span className="truncate">{org.name}</span>
                      <span className="text-muted-foreground truncate text-xs">
                        {tenantRoleLabel(t, org.role)}
                      </span>
                    </span>
                    {unavailable ? (
                      <Badge variant="warning" className="shrink-0">
                        {org.status === "suspended"
                          ? t("tenant.status.suspended")
                          : t("tenant.status.expired")}
                      </Badge>
                    ) : null}
                    {current ? <Check className="size-4 shrink-0" /> : null}
                  </Link>
                </DropdownMenuItem>
              );
            })
          )}
        </DropdownMenuGroup>
        {isPlatformUser(user) ? (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild>
              <Link href={routes.platform.home}>
                <LayoutDashboard />
                {t("tenant.switcher.platform")}
              </Link>
            </DropdownMenuItem>
          </>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
