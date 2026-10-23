import type { AppLayoutVariant } from "@/components/layout/app-layout";
import { cmsNav, platformNav, settingsNav } from "@/config/nav";
import { isNavEntryVisible } from "@/features/nav-engine/lib/access";
import { resolveSearchIcon } from "@/features/search-engine/lib/icons";
import type { PaletteItem } from "@/features/search-engine/types";
import type { NavCatalog } from "@/features/nav-engine/types";
import { createElement } from "react";

type AccessFns = {
  can: (permission: string | string[]) => boolean;
  canAny: (permissions: string[]) => boolean;
};

export function buildNavPageItems(
  variant: AppLayoutVariant,
  access: AccessFns,
  t: (key: string) => string,
): PaletteItem[] {
  // Platform palette also indexes the System Settings sub-pages.
  const catalogs: NavCatalog[] =
    variant === "cms" ? [cmsNav] : [platformNav, settingsNav];
  const items: PaletteItem[] = [];
  const seenHrefs = new Set<string>();

  for (const catalog of catalogs) {
    for (const group of catalog.groups) {
      if (!isNavEntryVisible(group, access)) continue;
      const groupLabel = t(group.labelKey);
      for (const item of group.items) {
        if (!isNavEntryVisible(item, access)) continue;
        if (item.soon || seenHrefs.has(item.href)) continue;
        seenHrefs.add(item.href);
        items.push({
          id: `page:${item.id}`,
          spec: "pages",
          label: t(item.titleKey),
          description: groupLabel,
          href: item.href,
          iconKey: item.searchIcon ?? "pages",
          icon: createElement(resolveSearchIcon(item.searchIcon ?? "pages"), {
            className: "size-4 shrink-0 opacity-80",
          }),
          group: t("search.group_pages"),
        });
      }
    }
  }

  return items;
}
