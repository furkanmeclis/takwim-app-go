"use client";

import type { ReactNode } from "react";

import {
  SettingsMobileNav,
  SettingsSubSidebar,
} from "@/features/settings/components/settings-sub-sidebar";

/** Two-column shell for `/platform/settings/*`: sub-sidebar (desktop) or pills + drawer (mobile). */
export function PlatformSettingsLayout({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-[calc(100svh-8.5rem)] flex-col gap-4 lg:flex-row lg:items-start lg:gap-6">
      <SettingsMobileNav />
      <SettingsSubSidebar className="lg:sticky lg:top-2 lg:max-h-[calc(100svh-8.5rem)]" />
      <div className="min-w-0 flex-1">{children}</div>
    </div>
  );
}
