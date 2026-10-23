import { routes } from "@/config/routes";
import { settingsNav } from "@/config/nav";
import { isNavEntryVisible } from "@/features/nav-engine/lib/access";
import type { NavGroupDef } from "@/features/nav-engine";

type AccessFns = {
  can: (permission: string | string[]) => boolean;
  canAny: (permissions: string[]) => boolean;
};

/** Permission-filtered settings groups, optionally narrowed by a search query. */
export function visibleSettingsGroups(
  access: AccessFns,
  t: (key: string) => string,
  query = "",
): NavGroupDef[] {
  const q = query.trim().toLocaleLowerCase();
  return settingsNav.groups
    .filter((group) => isNavEntryVisible(group, access))
    .map((group) => {
      const groupLabel = t(group.labelKey).toLocaleLowerCase();
      return {
        ...group,
        items: group.items.filter((item) => {
          if (!isNavEntryVisible(item, access)) return false;
          if (!q) return true;
          return (
            t(item.titleKey).toLocaleLowerCase().includes(q) ||
            groupLabel.includes(q)
          );
        }),
      };
    })
    .filter((group) => group.items.length > 0);
}

/** Active-state rule: the overview owns both `/platform/settings` and `/system`. */
export function isSettingsHrefActive(pathname: string, href: string): boolean {
  if (href === routes.platform.settings.root) {
    return (
      pathname === routes.platform.settings.root ||
      pathname === routes.platform.settings.system
    );
  }
  return pathname === href || pathname.startsWith(`${href}/`);
}

export function formatUptimeShort(seconds: number): string {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  return `${h}h ${m}m`;
}
