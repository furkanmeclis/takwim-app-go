import {
  Armchair,
  BadgePercent,
  BookOpenText,
  HandCoins,
  ArrowLeftRight,
  ArrowDownUp,
  CalendarCheck,
  CalendarClock,
  CalendarDays,
  ChartColumn,
  Boxes,
  ConciergeBell,
  Gauge,
  History,
  ReceiptText,
  PlugZap,
  Activity,
  Bell,
  Code2,
  Building2,
  Contact,
  Cpu,
  Download,
  HardDrive,
  KeyRound,
  LayoutDashboard,
  LogIn,
  Package,
  Palette,
  ScrollText,
  Settings2,
  Share2,
  Shield,
  ShieldCheck,
  SlidersHorizontal,
  Sparkles,
  Store,
  Ticket,
  Upload,
  UserRound,
  Users,
  Wallet,
} from "lucide-react";

import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { defineNav } from "@/features/nav-engine";
import type { NavCatalog } from "@/features/nav-engine";
import { ConfirmationsNavAdornment } from "@/features/confirmations/nav";
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
        {
          id: "organizations",
          titleKey: "layout.nav_organizations",
          href: routes.platform.organizations.root,
          icon: Building2,
          permission: permissions.organizations.read,
        },
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

/**
 * Organization shell (`/t/{slug}`) sidebar. Built per slug because every href
 * carries the organization. Items gate on `tenant.*` permissions, which the
 * tenant shell resolves for the current organization only.
 */
export function tenantNav(slug: string): NavCatalog {
  return defineNav({
    id: "tenant",
    groups: [
      {
        id: "general",
        labelKey: "layout.section_organization",
        icon: Building2,
        defaultOpen: true,
        items: [
          {
            id: "home",
            titleKey: "layout.home",
            href: routes.tenant.home(slug),
            icon: LayoutDashboard,
          },
          {
            id: "activity",
            titleKey: "layout.nav_tenant_activity",
            href: routes.tenant.activity(slug),
            icon: History,
            permission: permissions.tenant.activity.read,
          },
        ],
      },
      {
        id: "operations",
        labelKey: "layout.section_operations",
        icon: Store,
        defaultOpen: true,
        items: [
          {
            id: "calendar",
            titleKey: "layout.nav_calendar",
            href: routes.tenant.calendar(slug),
            icon: CalendarDays,
            anyPermission: [
              permissions.tenant.reservations.read,
              permissions.tenant.reservations.own,
            ],
            module: "calendar",
          },
          {
            id: "my-agenda",
            titleKey: "layout.nav_my_agenda",
            href: routes.tenant.myAgenda(slug),
            icon: CalendarClock,
            anyPermission: [
              permissions.tenant.reservations.read,
              permissions.tenant.reservations.own,
            ],
            module: "reservations",
          },
          {
            id: "my-shares",
            titleKey: "layout.nav_my_shares",
            href: routes.tenant.myShares(slug),
            icon: HandCoins,
            anyPermission: [
              permissions.tenant.earnings.read,
              permissions.tenant.earnings.own,
            ],
            module: "staff_shares",
          },
          {
            id: "reservations",
            titleKey: "layout.nav_reservations",
            href: routes.tenant.reservations.root(slug),
            icon: CalendarCheck,
            anyPermission: [
              permissions.tenant.reservations.read,
              permissions.tenant.reservations.own,
            ],
            module: "reservations",
          },
          {
            id: "confirmations",
            titleKey: "layout.nav_confirmations",
            href: routes.tenant.reservations.confirmations(slug),
            icon: ShieldCheck,
            permission: permissions.tenant.reservations.read,
            module: "confirmation",
            Adornment: ConfirmationsNavAdornment,
          },
          {
            id: "payment-follow-ups",
            titleKey: "layout.nav_payment_follow_ups",
            href: routes.tenant.reservations.followUps(slug),
            icon: ReceiptText,
            anyPermission: [
              permissions.tenant.payment.refund,
              permissions.tenant.payment.collect,
              permissions.tenant.finance.read,
            ],
            module: "reservations",
          },
          {
            id: "branches",
            titleKey: "layout.nav_branches",
            href: routes.tenant.branches.root(slug),
            icon: Store,
            permission: permissions.tenant.branches.read,
          },
          {
            id: "staff",
            titleKey: "layout.nav_staff",
            href: routes.tenant.staff.root(slug),
            icon: Contact,
            permission: permissions.tenant.staff.read,
            module: "staff",
          },
          {
            id: "resources",
            titleKey: "layout.nav_resources",
            href: routes.tenant.resources.root(slug),
            icon: Armchair,
            permission: permissions.tenant.resources.read,
            module: "resources",
          },
          {
            id: "services",
            titleKey: "layout.nav_services",
            href: routes.tenant.services.root(slug),
            icon: ConciergeBell,
            permission: permissions.tenant.services.read,
            module: "services",
          },
          {
            id: "service-bundles",
            titleKey: "layout.nav_service_bundles",
            href: routes.tenant.serviceBundles.root(slug),
            icon: Boxes,
            permission: permissions.tenant.services.read,
            module: "services",
          },
          {
            id: "products",
            titleKey: "layout.nav_products",
            href: routes.tenant.products.root(slug),
            icon: Package,
            permission: permissions.tenant.products.read,
            module: "products",
          },
          {
            id: "stock-movements",
            titleKey: "layout.nav_stock_movements",
            href: routes.tenant.stockMovements.root(slug),
            icon: ArrowLeftRight,
            permission: permissions.tenant.products.read,
            module: "products",
          },
          {
            id: "customers",
            titleKey: "layout.nav_customers",
            href: routes.tenant.customers.root(slug),
            icon: UserRound,
            permission: permissions.tenant.customers.read,
            module: "customers",
          },
          {
            id: "packages",
            titleKey: "layout.nav_packages",
            href: routes.tenant.packages.root(slug),
            icon: Ticket,
            permission: permissions.tenant.sessionPackages.read,
            module: "packages",
          },
          {
            id: "cash",
            titleKey: "layout.nav_cash",
            href: routes.tenant.cash.root(slug),
            icon: Wallet,
            permission: permissions.tenant.cash.read,
            module: "cash",
          },
          {
            id: "finance",
            titleKey: "layout.nav_finance",
            href: routes.tenant.finance.root(slug),
            icon: ArrowDownUp,
            permission: permissions.tenant.finance.read,
            module: "finance",
          },
          {
            id: "sales-ledger",
            titleKey: "layout.nav_sales_ledger",
            href: routes.tenant.salesLedger(slug),
            icon: BookOpenText,
            permission: permissions.tenant.finance.read,
            module: "finance",
          },
          {
            id: "staff-shares",
            titleKey: "layout.nav_staff_shares",
            href: routes.tenant.staffShares(slug),
            icon: BadgePercent,
            permission: permissions.tenant.earnings.read,
            module: "staff_shares",
          },
          {
            id: "reports",
            titleKey: "layout.nav_reports",
            href: routes.tenant.reports(slug),
            icon: ChartColumn,
            permission: permissions.tenant.finance.read,
            module: "reports",
          },
        ],
      },
      {
        id: "settings",
        labelKey: "layout.section_tenant_settings",
        icon: Settings2,
        defaultOpen: false,
        items: [
          {
            id: "settings-general",
            titleKey: "layout.nav_tenant_settings_general",
            href: routes.tenant.settings.root(slug),
            icon: SlidersHorizontal,
            permission: permissions.tenant.settings.read,
          },
          {
            id: "settings-members",
            titleKey: "layout.nav_tenant_settings_members",
            href: routes.tenant.settings.members(slug),
            icon: Users,
            permission: permissions.tenant.members.read,
          },
          {
            id: "settings-roles",
            titleKey: "layout.nav_tenant_settings_roles",
            href: routes.tenant.settings.roles.root(slug),
            icon: Shield,
            permission: permissions.tenant.roles.read,
          },
          {
            id: "settings-integrations",
            titleKey: "layout.nav_tenant_settings_integrations",
            href: routes.tenant.settings.integrations(slug),
            icon: PlugZap,
            permission: permissions.tenant.integrations.read,
          },
          {
            id: "settings-notifications",
            titleKey: "layout.nav_tenant_settings_notifications",
            href: routes.tenant.settings.notifications(slug),
            icon: Bell,
            permission: permissions.tenant.settings.read,
          },
          {
            id: "settings-confirmation",
            titleKey: "layout.nav_tenant_settings_confirmation",
            href: routes.tenant.settings.confirmation(slug),
            icon: CalendarCheck,
            permission: permissions.tenant.settings.read,
            module: "confirmation",
          },
          {
            id: "settings-embed",
            titleKey: "layout.nav_tenant_settings_embed",
            href: routes.tenant.settings.embed(slug),
            icon: Code2,
            permission: permissions.tenant.settings.read,
            module: "booking_page",
          },
          {
            id: "settings-plan",
            titleKey: "layout.nav_tenant_settings_plan",
            href: routes.tenant.settings.plan(slug),
            icon: Gauge,
            permission: permissions.tenant.settings.read,
          },
        ],
      },
    ],
  });
}
