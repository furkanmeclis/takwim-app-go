"use client";

import { useMemo } from "react";

import type { AppLayoutVariant } from "@/components/layout/app-layout";
import { platformNav, settingsNav, tenantNav } from "@/config/nav";
import { routes } from "@/config/routes";
import { useOptionalTenant } from "@/features/tenant/components/tenant-provider";
import type { NavCatalog } from "@/features/nav-engine";

export type ShellNav = {
  /** Sidebar catalog of the shell. */
  catalog: NavCatalog;
  /** Catalogs the command palette indexes as pages. */
  paletteCatalogs: NavCatalog[];
  homeHref: string;
};

/** Sidebar + palette catalogs of the current shell. */
export function useShellNav(variant: AppLayoutVariant): ShellNav {
  const tenantSlug = useOptionalTenant()?.slug;

  return useMemo<ShellNav>(() => {
    if (variant === "tenant" && tenantSlug) {
      const catalog = tenantNav(tenantSlug);
      return {
        catalog,
        paletteCatalogs: [catalog],
        homeHref: routes.tenant.home(tenantSlug),
      };
    }
    // Platform palette also indexes the System Settings sub-pages.
    return {
      catalog: platformNav,
      paletteCatalogs: [platformNav, settingsNav],
      homeHref: routes.platform.home,
    };
  }, [tenantSlug, variant]);
}
