"use client";

import Link from "next/link";

import { AppSidebarLogo } from "@/components/brand";
import type { AppLayoutVariant } from "@/components/layout/app-layout";
import {
  Sidebar,
  SidebarFooter,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from "@/components/ui/sidebar";
import { useShellNav } from "@/components/layout/use-shell-nav";
import { NavEngine } from "@/features/nav-engine";
import { OrganizationAvatar } from "@/features/tenant/components/organization-avatar";
import { useOptionalTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

/** Tenant shell header: the organization's logo and name. */
function OrganizationSidebarLogo() {
  const { t } = useLocale();
  const tenant = useOptionalTenant();
  if (!tenant) return <AppSidebarLogo />;
  return (
    <>
      <OrganizationAvatar logoUrl={tenant.organization?.logoUrl} />
      <span className="grid flex-1 text-left text-sm leading-tight group-data-[collapsible=icon]:hidden">
        <span className="truncate font-medium">
          {tenant.organization?.name ?? tenant.slug}
        </span>
        <span className="truncate text-xs opacity-70">
          {t("layout.section_organization")}
        </span>
      </span>
    </>
  );
}

export function AppSidebar({
  variant = "platform",
}: {
  variant?: AppLayoutVariant;
}) {
  const { catalog, homeHref } = useShellNav(variant);

  return (
    <Sidebar collapsible="icon" variant="inset">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" asChild>
              <Link href={homeHref}>
                {variant === "tenant" ? (
                  <OrganizationSidebarLogo />
                ) : (
                  <AppSidebarLogo />
                )}
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <NavEngine catalog={catalog} homeHref={homeHref} />

      <SidebarFooter />
      <SidebarRail />
    </Sidebar>
  );
}
