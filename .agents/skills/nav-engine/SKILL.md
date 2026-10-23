---
name: nav-engine
description: >-
  Add or change platform/CMS sidebar items via the nav-engine catalog:
  accordion groups, count/label/dot/custom badges, and info popovers.
  Use when editing the sidebar, AppSidebar, config/nav.ts, nav items,
  nav groups, badges, or sidebar info.
---

**Persona:** Sidebar is a catalog, not JSX rows.

# Nav engine

Do **not** add menu rows in `frontend/src/components/layout/app-sidebar.tsx`.
That file only mounts `NavEngine`.

| Piece | Path |
|-------|------|
| Catalog | `frontend/src/config/nav.ts` (`platformNav`, `settingsNav`, `cmsNav`) |
| Engine | `frontend/src/features/nav-engine` |
| Canonical example | `frontend/src/features/users/nav/` |

Public API: `defineNav`, `defineNavItem`, `defineNavGroup`, `createNavAdornment` from `@/features/nav-engine`.

## Add an item

1. Route in `frontend/src/config/routes.ts`, permission in `frontend/src/config/permissions.ts`.
2. i18n: `titleKey` / group `labelKey` as `layout.*` in `frontend/src/locales/{en,tr}/layout.json`.
3. Put the item in the right group in `config/nav.ts` (inline object **or** `defineNavItem` exported from the feature).
4. Gate with `permission` (AND) or `anyPermission` (OR). Omit both → always visible if the group is visible.

```ts
{
  id: "roles",
  titleKey: "layout.nav_roles",
  href: routes.platform.roles.root,
  icon: Shield,
  permission: permissions.roles.read,
}
```

Static extras on the item: `soon: true`, `badges`, `info`. Live data → `Adornment` (next section).

## Live badges / info

Keep the hook in the feature. Wrap with `createNavAdornment` so hooks stay legal.

```ts
import { createNavAdornment, defineNavItem } from "@/features/nav-engine";

function useOrdersNavAdornment(): NavAdornment {
  const { t } = useLocale();
  const { data } = useOrdersNavStats();
  return {
    badges: [{ kind: "count", value: data?.open ?? 0, variant: "secondary" }],
    info: {
      title: t("orders.nav.info_title"),
      rows: [{ label: t("orders.status.open"), value: data?.open ?? 0, tone: "warning" }],
    },
  };
}

export const OrdersNavAdornment = createNavAdornment(useOrdersNavAdornment);

export const ordersNavItem = defineNavItem({
  id: "orders",
  titleKey: "layout.nav_orders",
  href: routes.platform.orders.root,
  icon: ShoppingBag,
  permission: permissions.orders.read,
  Adornment: OrdersNavAdornment,
});
```

Then import `ordersNavItem` into `platformNav` (same pattern as `usersNavItem`).

## System Settings sub-sidebar

Platform admin/config pages (roles, logs, storage, access, auth, integrations, imports/exports, branding, system diagnostics) live under `/platform/settings/*` and are listed in `settingsNav`, not `platformNav`. `features/settings` renders it as a grouped, searchable sub-sidebar; `platformNav` has a single "System Settings" item gated by `anyPermission: settingsNavPermissions` (derived from `settingsNav`). Add a new settings page by: route under `routes.platform.settings`, page in `src/app/(platform)/platform/settings/…`, item in the right `settingsNav` group. The command palette indexes `settingsNav` too.

Copy `frontend/src/features/users/nav/` for a full example: count + pending label + custom pulse + status info popover.

## Badge kinds

| `kind` | Fields | Notes |
|--------|--------|-------|
| `count` | `value`, `variant?`, `max?` (default 99), `hiddenWhenZero?` (default true) | Number pill |
| `label` | `text`, `variant?` | Text pill (`soon` uses this) |
| `dot` | `variant?` | 8px status dot |
| `custom` | `id`, `render: () => ReactNode` | Anything; keep tiny |

`variant`: `default` \| `secondary` \| `success` \| `warning` \| `danger` \| `outline`.

Merge order: live `Adornment` badges → static `item.badges` → `soon` label.

## Info

```ts
info: {
  title?: string;
  description?: string;
  rows?: { label: string; value: ReactNode; tone?: NavBadgeVariant }[];
  render?: () => ReactNode; // replaces the default panel
}
```

Expanded sidebar: info icon → popover. Icon-collapsed flyout: compact summary under the row.

## Groups

```ts
{
  id: "manage",
  labelKey: "layout.section_manage",
  icon: Shield,          // required for icon-mode flyout trigger
  defaultOpen: true,
  collapsible: true,     // false pins the section open
  items: [/* ... */],
}
```

- Expanded: accordion. Open state in `localStorage` (`app.nav.groups.v1:<catalogId>`).
- Icon-collapsed: 2+ items → flyout with group `icon`; 1 item → direct link.
- Empty after permission filter → group is hidden.
- New shell: add a catalog with a unique `id` and pass it to `<NavEngine catalog={…} homeHref={…} />`.

Do not reimplement accordion, flyout, badges, or info in layout components.
