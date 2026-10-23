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
