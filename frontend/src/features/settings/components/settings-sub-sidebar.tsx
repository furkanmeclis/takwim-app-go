"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  ChevronDown,
  PanelLeftClose,
  PanelLeftOpen,
  RefreshCw,
  Search,
  Server,
  SlidersHorizontal,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Input } from "@/components/ui/input";
import { ScrollArea, ScrollBar } from "@/components/ui/scroll-area";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { permissions } from "@/config/permissions";
import type { NavGroupDef, NavItemDef } from "@/features/nav-engine";
import { useNavGroupOpen } from "@/features/nav-engine/hooks/use-nav-group-open";
import {
  useSystemInfo,
  useSystemPing,
} from "@/features/settings/hooks/use-system-info";
import { useSubSidebarCollapsed } from "@/features/settings/hooks/use-subsidebar-collapsed";
import {
  formatUptimeShort,
  isSettingsHrefActive,
  visibleSettingsGroups,
} from "@/features/settings/lib/settings-nav";
import type { SystemInfo } from "@/features/settings/types";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type Translate = (key: string) => string;

function statusLabelKey(info: SystemInfo | undefined): string {
  if (!info) return "settings.system.status_unknown";
  if (info.status === "healthy") return "settings.system.status_operational";
  if (info.status === "degraded") return "settings.system.status_degraded";
  return "settings.system.status_down";
}

function StatusDot({ info }: { info: SystemInfo | undefined }) {
  const tone =
    info?.status === "healthy"
      ? ["bg-emerald-400", "bg-emerald-500"]
      : info?.status === "degraded"
        ? ["bg-amber-400", "bg-amber-500"]
        : info?.status === "down"
          ? ["bg-rose-400", "bg-rose-500"]
          : ["bg-muted-foreground/40", "bg-muted-foreground/60"];
  return (
    <span className="relative flex size-2 shrink-0">
      {info ? (
        <span
          className={cn(
            "absolute inline-flex h-full w-full animate-ping rounded-full opacity-75",
            tone[0],
          )}
        />
      ) : null}
      <span
        className={cn("relative inline-flex size-2 rounded-full", tone[1])}
      />
    </span>
  );
}

/** Health pill: status + Go version (only rendered when diagnostics are readable). */
function StatusPill({
  info,
  t,
}: {
  info: SystemInfo | undefined;
  t: Translate;
}) {
  return (
    <div className="border-border/80 bg-muted/40 flex items-center justify-between gap-2 rounded-lg border px-2.5 py-1.5 text-xs">
      <div className="flex min-w-0 items-center gap-2">
        <StatusDot info={info} />
        <span className="text-foreground truncate text-[11px] font-medium">
          {t(statusLabelKey(info))}
        </span>
      </div>
      {info?.runtime.go_version ? (
        <Badge
          variant="outline"
          className="border-border h-4 px-1.5 py-0 font-mono text-[9px]"
        >
          {info.runtime.go_version}
        </Badge>
      ) : null}
    </div>
  );
}

function SearchBox({
  value,
  onChange,
  t,
  className,
}: {
  value: string;
  onChange: (value: string) => void;
  t: Translate;
  className?: string;
}) {
  return (
    <div className="relative mt-2.5">
      <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2" />
      <Input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={t("settings.subnav.search_placeholder")}
        aria-label={t("settings.subnav.search_placeholder")}
        className={cn("h-8 pl-8 text-xs", className)}
      />
    </div>
  );
}

function SubSidebarLink({
  item,
  active,
  t,
}: {
  item: NavItemDef;
  active: boolean;
  t: Translate;
}) {
  const Icon = item.icon;
  return (
    <Link
      href={item.href}
      aria-current={active ? "page" : undefined}
      className={cn(
        "group flex items-center justify-between rounded-lg px-2.5 py-1.5 text-xs font-medium transition-all duration-150",
        active
          ? "bg-primary/10 text-primary ring-primary/20 dark:bg-primary/20 font-semibold ring-1"
          : "text-muted-foreground hover:bg-muted hover:text-foreground",
      )}
    >
      <div className="flex min-w-0 items-center gap-2">
        <Icon
          className={cn(
            "size-3.5 shrink-0 transition-transform group-hover:scale-105",
            active
              ? "text-primary"
              : "text-muted-foreground group-hover:text-foreground",
          )}
        />
        <span className="truncate">{t(item.titleKey)}</span>
      </div>
      {active ? (
        <span className="bg-primary size-1.5 shrink-0 rounded-full" />
      ) : null}
    </Link>
  );
}

function SubSidebarGroup({
  group,
  pathname,
  forceOpen,
  t,
}: {
  group: NavGroupDef;
  pathname: string;
  forceOpen: boolean;
  t: Translate;
}) {
  const [open, setOpen] = useNavGroupOpen(
    "settings",
    group.id,
    group.defaultOpen ?? true,
  );
  const isOpen = open || forceOpen;

  return (
    <Collapsible open={isOpen} onOpenChange={setOpen} className="space-y-1">
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className="group text-muted-foreground/80 hover:text-foreground flex w-full items-center justify-between rounded-md px-2 py-1 text-[10px] font-bold tracking-wider uppercase transition-colors"
        >
          <span>{t(group.labelKey)}</span>
          <ChevronDown
            className={cn(
              "text-muted-foreground/60 group-hover:text-foreground size-3 transition-transform duration-200",
              isOpen ? "rotate-0" : "-rotate-90",
            )}
          />
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-0.5 pt-0.5">
        {group.items.map((item) => (
          <SubSidebarLink
            key={item.id}
            item={item}
            active={isSettingsHrefActive(pathname, item.href)}
            t={t}
          />
        ))}
      </CollapsibleContent>
    </Collapsible>
  );
}

/** Desktop settings sub-sidebar: grouped, searchable, collapsible to icons. */
export function SettingsSubSidebar({ className }: { className?: string }) {
  const { t } = useLocale();
  const pathname = usePathname();
  const { can, canAny } = usePermission();
  const [search, setSearch] = useState("");
  const [collapsed, toggleCollapsed] = useSubSidebarCollapsed();

  const canReadSystem = can(permissions.settings.read);
  const { data: systemInfo, isFetching } = useSystemInfo();
  const pingMutation = useSystemPing();

  const groups = useMemo(
    () => visibleSettingsGroups({ can, canAny }, t, search),
    [can, canAny, t, search],
  );

  return (
    <aside
      className={cn(
        "border-border bg-card text-card-foreground hidden shrink-0 flex-col rounded-2xl border shadow-xs transition-[width] duration-200 ease-in-out lg:flex",
        collapsed ? "w-16" : "w-68",
        className,
      )}
    >
      <div
        className={cn(
          "border-border border-b p-3",
          collapsed && "px-2 py-3 text-center",
        )}
      >
        <div className="flex items-center justify-between gap-1.5">
          {!collapsed ? (
            <div className="flex min-w-0 items-center gap-2.5">
              <div className="bg-primary/10 text-primary border-primary/20 flex size-8 shrink-0 items-center justify-center rounded-lg border">
                <Server className="size-4" />
              </div>
              <div className="min-w-0">
                <h2 className="text-foreground truncate text-xs font-bold tracking-tight">
                  {t("settings.subnav.title")}
                </h2>
                <p className="text-muted-foreground truncate text-[10px]">
                  {t("settings.subnav.subtitle")}
                </p>
              </div>
            </div>
          ) : null}

          <div
            className={cn(
              "flex items-center gap-1",
              collapsed && "mx-auto flex-col",
            )}
          >
            {!collapsed && canReadSystem ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="text-muted-foreground hover:text-foreground size-7"
                    aria-label={t("settings.system.refresh_tooltip")}
                    onClick={() => pingMutation.mutate()}
                    disabled={pingMutation.isPending}
                  >
                    <RefreshCw
                      className={cn(
                        "size-3.5",
                        (pingMutation.isPending || isFetching) &&
                          "animate-spin",
                      )}
                    />
                  </Button>
                </TooltipTrigger>
                <TooltipContent side="right">
                  {t("settings.system.refresh_tooltip")}
                </TooltipContent>
              </Tooltip>
            ) : null}

            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="text-muted-foreground hover:text-foreground size-7"
                  aria-label={
                    collapsed
                      ? t("layout.expand_sidebar")
                      : t("layout.collapse_sidebar")
                  }
                  onClick={toggleCollapsed}
                >
                  {collapsed ? (
                    <PanelLeftOpen className="text-primary size-4" />
                  ) : (
                    <PanelLeftClose className="size-4" />
                  )}
                </Button>
              </TooltipTrigger>
              <TooltipContent side="right">
                {collapsed
                  ? t("layout.expand_sidebar")
                  : t("layout.collapse_sidebar")}
              </TooltipContent>
            </Tooltip>
          </div>
        </div>

        {canReadSystem ? (
          collapsed ? (
            <div className="mt-2 flex justify-center">
              <Tooltip>
                <TooltipTrigger asChild>
                  <div className="bg-muted/60 flex size-6 items-center justify-center rounded-full">
                    <StatusDot info={systemInfo} />
                  </div>
                </TooltipTrigger>
                <TooltipContent side="right">
                  {t(statusLabelKey(systemInfo))}
                </TooltipContent>
              </Tooltip>
            </div>
          ) : (
            <div className="mt-2.5">
              <StatusPill info={systemInfo} t={t} />
            </div>
          )
        ) : null}

        {!collapsed ? (
          <SearchBox
            value={search}
            onChange={setSearch}
            t={t}
            className="bg-background/70 border-border h-7.5"
          />
        ) : null}
      </div>

      <ScrollArea className="flex-1 px-2 py-2">
        <nav aria-label={t("settings.subnav.title")} className="space-y-3">
          {collapsed
            ? groups.map((group) => (
                <div key={group.id} className="space-y-1">
                  {group.items.map((item) => {
                    const Icon = item.icon;
                    const active = isSettingsHrefActive(pathname, item.href);
                    return (
                      <Tooltip key={item.id}>
                        <TooltipTrigger asChild>
                          <Link
                            href={item.href}
                            aria-label={t(item.titleKey)}
                            aria-current={active ? "page" : undefined}
                            className={cn(
                              "mx-auto flex size-9 items-center justify-center rounded-lg transition-colors",
                              active
                                ? "bg-primary text-primary-foreground font-semibold shadow-xs"
                                : "text-muted-foreground hover:bg-muted hover:text-foreground",
                            )}
                          >
                            <Icon className="size-4 shrink-0" />
                          </Link>
                        </TooltipTrigger>
                        <TooltipContent side="right" className="font-medium">
                          {t(item.titleKey)}
                        </TooltipContent>
                      </Tooltip>
                    );
                  })}
                </div>
              ))
            : groups.map((group) => (
                <SubSidebarGroup
                  key={group.id}
                  group={group}
                  pathname={pathname}
                  forceOpen={search.trim().length > 0}
                  t={t}
                />
              ))}

          {!collapsed && groups.length === 0 ? (
            <div className="text-muted-foreground py-6 text-center text-xs">
              {t("common.no_results")}
            </div>
          ) : null}
        </nav>
      </ScrollArea>

      {canReadSystem && systemInfo ? (
        <div
          className={cn(
            "border-border text-muted-foreground border-t p-2.5 text-[10px]",
            collapsed && "text-center",
          )}
        >
          {!collapsed ? (
            <div className="flex items-center justify-between">
              <span className="text-foreground/80 flex items-center gap-1.5 font-semibold">
                <span className="size-1.5 rounded-full bg-emerald-500" />
                {systemInfo.environment.env.toUpperCase()}
              </span>
              <span className="font-mono">
                {t("settings.system.uptime_label")}{" "}
                {formatUptimeShort(systemInfo.runtime.uptime_seconds)}
              </span>
            </div>
          ) : (
            <Tooltip>
              <TooltipTrigger asChild>
                <div className="mx-auto flex size-6 items-center justify-center font-mono text-[9px]">
                  {Math.floor(systemInfo.runtime.uptime_seconds / 3600)}h
                </div>
              </TooltipTrigger>
              <TooltipContent side="right">
                {t("settings.system.uptime_label")}{" "}
                {formatUptimeShort(systemInfo.runtime.uptime_seconds)}
              </TooltipContent>
            </Tooltip>
          )}
        </div>
      ) : null}
    </aside>
  );
}

/** Mobile settings navigation: current page header, drawer, and quick pills. */
export function SettingsMobileNav({ className }: { className?: string }) {
  const { t } = useLocale();
  const pathname = usePathname();
  const { can, canAny } = usePermission();
  const [sheetOpen, setSheetOpen] = useState(false);
  const [search, setSearch] = useState("");

  const canReadSystem = can(permissions.settings.read);
  const { data: systemInfo } = useSystemInfo();

  const allGroups = useMemo(
    () => visibleSettingsGroups({ can, canAny }, t),
    [can, canAny, t],
  );
  const filteredGroups = useMemo(
    () => visibleSettingsGroups({ can, canAny }, t, search),
    [can, canAny, t, search],
  );
  const allItems = useMemo(
    () => allGroups.flatMap((group) => group.items),
    [allGroups],
  );

  let active: { group: NavGroupDef; item: NavItemDef } | null = null;
  for (const group of allGroups) {
    const item = group.items.find((entry) =>
      isSettingsHrefActive(pathname, entry.href),
    );
    if (item) {
      active = { group, item };
      break;
    }
  }

  const ActiveIcon = active?.item.icon ?? Server;

  return (
    <div
      className={cn(
        "border-border bg-card flex flex-col gap-2 rounded-xl border p-2.5 shadow-xs lg:hidden",
        className,
      )}
    >
      <div className="flex items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <div className="bg-primary/10 text-primary flex size-7 shrink-0 items-center justify-center rounded-md">
            <ActiveIcon className="size-3.5" />
          </div>
          <div className="min-w-0">
            <p className="text-muted-foreground truncate text-[10px] font-semibold tracking-wider uppercase">
              {active ? t(active.group.labelKey) : t("settings.subnav.title")}
            </p>
            {active ? (
              <h3 className="text-foreground truncate text-xs font-bold">
                {t(active.item.titleKey)}
              </h3>
            ) : null}
          </div>
        </div>

        <Sheet open={sheetOpen} onOpenChange={setSheetOpen}>
          <SheetTrigger asChild>
            <Button
              variant="outline"
              size="sm"
              className="h-7.5 shrink-0 gap-1.5 px-2.5 text-xs font-medium"
            >
              <SlidersHorizontal className="text-primary size-3.5" />
              <span>{t("settings.subnav.title")}</span>
            </Button>
          </SheetTrigger>

          <SheetContent
            side="left"
            className="flex w-[85vw] max-w-sm flex-col p-0"
          >
            <SheetHeader className="border-border border-b p-4 text-left">
              <div className="flex items-center gap-2.5">
                <div className="bg-primary/10 text-primary border-primary/20 flex size-8 items-center justify-center rounded-lg border">
                  <Server className="size-4" />
                </div>
                <div>
                  <SheetTitle className="text-sm font-bold">
                    {t("settings.subnav.title")}
                  </SheetTitle>
                  <SheetDescription className="text-muted-foreground text-xs">
                    {t("settings.subnav.subtitle")}
                  </SheetDescription>
                </div>
              </div>

              {canReadSystem ? (
                <div className="mt-3">
                  <StatusPill info={systemInfo} t={t} />
                </div>
              ) : null}

              <SearchBox
                value={search}
                onChange={setSearch}
                t={t}
                className="bg-background"
              />
            </SheetHeader>

            <ScrollArea className="flex-1 px-3 py-3">
              <nav
                aria-label={t("settings.subnav.title")}
                className="space-y-4"
              >
                {filteredGroups.map((group) => (
                  <div key={group.id} className="space-y-1">
                    <h4 className="text-muted-foreground/80 px-2 text-[10px] font-bold tracking-wider uppercase">
                      {t(group.labelKey)}
                    </h4>
                    <div className="space-y-0.5">
                      {group.items.map((item) => {
                        const Icon = item.icon;
                        const isActive = isSettingsHrefActive(
                          pathname,
                          item.href,
                        );
                        return (
                          <Link
                            key={item.id}
                            href={item.href}
                            aria-current={isActive ? "page" : undefined}
                            onClick={() => setSheetOpen(false)}
                            className={cn(
                              "flex items-center justify-between rounded-lg px-2.5 py-2 text-xs font-medium transition-colors",
                              isActive
                                ? "bg-primary text-primary-foreground font-semibold shadow-xs"
                                : "text-muted-foreground hover:bg-muted hover:text-foreground",
                            )}
                          >
                            <div className="flex min-w-0 items-center gap-2.5">
                              <Icon className="size-4 shrink-0" />
                              <span className="truncate">
                                {t(item.titleKey)}
                              </span>
                            </div>
                            {isActive ? (
                              <span className="bg-primary-foreground size-1.5 shrink-0 rounded-full" />
                            ) : null}
                          </Link>
                        );
                      })}
                    </div>
                  </div>
                ))}

                {filteredGroups.length === 0 ? (
                  <div className="text-muted-foreground py-6 text-center text-xs">
                    {t("common.no_results")}
                  </div>
                ) : null}
              </nav>
            </ScrollArea>
          </SheetContent>
        </Sheet>
      </div>

      <div className="relative -mx-1">
        <ScrollArea className="w-full whitespace-nowrap">
          <div className="flex w-max space-x-1.5 px-1 py-0.5">
            {allItems.map((item) => {
              const Icon = item.icon;
              const isActive = isSettingsHrefActive(pathname, item.href);
              return (
                <Link
                  key={item.id}
                  href={item.href}
                  aria-current={isActive ? "page" : undefined}
                  className={cn(
                    "inline-flex shrink-0 items-center gap-1.5 rounded-lg px-2.5 py-1 text-xs font-medium transition-colors",
                    isActive
                      ? "bg-primary text-primary-foreground font-semibold shadow-xs"
                      : "bg-muted/60 text-muted-foreground hover:bg-muted hover:text-foreground",
                  )}
                >
                  <Icon className="size-3.5 shrink-0" />
                  <span>{t(item.titleKey)}</span>
                </Link>
              );
            })}
          </div>
          <ScrollBar orientation="horizontal" className="invisible" />
        </ScrollArea>
      </div>
    </div>
  );
}
