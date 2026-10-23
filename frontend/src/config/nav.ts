import {
  Activity,
  Bell,
  Cpu,
  Download,
  HardDrive,
  KeyRound,
  LayoutDashboard,
  LogIn,
  Palette,
  ScrollText,
  Settings2,
  Share2,
  Shield,
  Sparkles,
  Upload,
} from "lucide-react";

import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { defineNav } from "@/features/nav-engine";
import type { NavCatalog } from "@/features/nav-engine";
import { usersNavItem } from "@/features/users/nav";

const integrationReadPermissions = [
  permissions.integrations.google.read,
  permissions.integrations.github.read,
  permissions.integrations.facebook.read,
  permissions.integrations.apple.read,
];

/**
 * System Settings sub-sidebar (`/platform/settings/*`). Rendered by
 * `features/settings` (not by `NavEngine`), but uses the same catalog shape
 * and `permission` / `anyPermission` gating.
 */
export const settingsNav = defineNav({
  id: "settings",
  groups: [
    {
      id: "system",
      labelKey: "settings.subnav.group_system",
      icon: Activity,
      defaultOpen: true,
      items: [
        {
          id: "system-overview",
          titleKey: "settings.subnav.system_overview",
          href: routes.platform.settings.root,
          icon: Activity,
          permission: permissions.settings.read,
        },
        {
          id: "system-environment",
          titleKey: "settings.subnav.system_environment",
          href: routes.platform.settings.environment,
          icon: Cpu,
          permission: permissions.settings.read,
        },
        {
          id: "logs",
          titleKey: "layout.nav_logs",
          href: routes.platform.settings.logs,
          icon: ScrollText,
          permission: permissions.logs.read,
        },
        {
          id: "activity",
          titleKey: "layout.nav_activity",
          href: routes.platform.settings.activity,
          icon: Activity,
          permission: permissions.activity.read,
        },
      ],
    },
    {
      id: "general",
      labelKey: "settings.subnav.group_general",
      icon: Settings2,
      defaultOpen: true,
      items: [
        {
          id: "general-branding",
          titleKey: "settings.subnav.general_branding",
          href: routes.platform.settings.general,
          icon: Palette,
          permission: permissions.settings.read,
        },
      ],
    },
    {
      id: "security",
      labelKey: "settings.subnav.group_security",
      icon: Shield,
      defaultOpen: true,
      items: [
        {
          id: "roles",
          titleKey: "layout.nav_roles",
          href: routes.platform.settings.roles,
          icon: Shield,
          permission: permissions.roles.read,
        },
        {
          id: "auth-settings",
          titleKey: "settings.subnav.auth_methods",
          href: routes.platform.settings.auth,
          icon: LogIn,
          permission: permissions.authSettings.read,
        },
        {
          id: "access-policy",
          titleKey: "settings.subnav.access_policy",
          href: routes.platform.settings.access,
          icon: KeyRound,
          permission: permissions.access.read,
        },
      ],
    },
    {
      id: "integrations",
      labelKey: "settings.subnav.group_integrations",
      icon: Share2,
      defaultOpen: true,
      items: [
        {
          id: "oauth-integrations",
          titleKey: "settings.subnav.oauth_providers",
          href: routes.platform.settings.integrations,
          icon: Share2,
          anyPermission: integrationReadPermissions,
        },
        {
          id: "storage",
          titleKey: "settings.subnav.storage",
          href: routes.platform.settings.storage,
          icon: HardDrive,
          permission: permissions.storage.read,
        },
      ],
    },
    {
      id: "io",
      labelKey: "settings.subnav.group_maintenance",
      icon: Upload,
      defaultOpen: true,
      items: [
        {
          id: "imports",
          titleKey: "layout.nav_imports",
          href: routes.platform.settings.imports,
          icon: Upload,
          permission: permissions.imports.read,
        },
        {
          id: "exports",
          titleKey: "layout.nav_exports",
          href: routes.platform.settings.exports,
          icon: Download,
          permission: permissions.exports.read,
        },
        {
          id: "purge-rules",
          titleKey: "settings.subnav.purge_rules",
          href: routes.platform.settings.maintenance,
          icon: Sparkles,
          permission: permissions.logs.read,
        },
      ],
    },
  ],
});

/** Every slug that unlocks at least one settings subpage (OR-gate for the shell). */
function catalogPermissions(catalog: NavCatalog): string[] {
  const slugs = new Set<string>();
  for (const group of catalog.groups) {
    for (const item of group.items) {
      if (typeof item.permission === "string") slugs.add(item.permission);
      else if (item.permission) item.permission.forEach((p) => slugs.add(p));
      item.anyPermission?.forEach((p) => slugs.add(p));
    }
  }
  return [...slugs].sort();
}

export const settingsNavPermissions = catalogPermissions(settingsNav);

export const platformNav = defineNav({
  id: "platform",
  groups: [
    {
      id: "general",
      labelKey: "layout.nav_platform",
      icon: LayoutDashboard,
      defaultOpen: true,
      items: [
        {
          id: "home",
          titleKey: "layout.home",
          href: routes.platform.home,
          icon: LayoutDashboard,
        },
      ],
    },
    {
      id: "manage",
      labelKey: "layout.section_manage",
      icon: Shield,
      defaultOpen: true,
      items: [
        usersNavItem,
        {
          id: "notifications",
          titleKey: "layout.nav_notifications",
          href: routes.platform.notifications.root,
          icon: Bell,
          permission: permissions.notifications.platformRead,
        },
        {
          id: "settings",
          titleKey: "layout.nav_settings",
          href: routes.platform.settings.root,
          icon: Settings2,
          anyPermission: settingsNavPermissions,
        },
      ],
    },
  ],
});

export const cmsNav = defineNav({
  id: "cms",
  groups: [
    {
      id: "cms",
      labelKey: "layout.section_cms",
      icon: LayoutDashboard,
      defaultOpen: true,
      items: [
        {
          id: "home",
          titleKey: "layout.home",
          href: routes.cms.home,
          icon: LayoutDashboard,
        },
      ],
    },
  ],
});
