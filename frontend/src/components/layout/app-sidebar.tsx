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
import { cmsNav, platformNav } from "@/config/nav";
import { routes } from "@/config/routes";
import { NavEngine } from "@/features/nav-engine";

export function AppSidebar({
  variant = "platform",
}: {
  variant?: AppLayoutVariant;
}) {
  const catalog = variant === "cms" ? cmsNav : platformNav;
  const homeHref =
    variant === "platform" ? routes.platform.home : routes.cms.home;

  return (
    <Sidebar collapsible="icon" variant="inset">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" asChild>
              <Link href={homeHref}>
                <AppSidebarLogo />
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
