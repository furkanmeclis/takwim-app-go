"use client";

import type { ReactNode } from "react";
import Link from "next/link";

import { AppSidebarLogo } from "@/components/brand";
import { LocaleSwitch } from "@/components/layout/locale-switch";
import { ThemeSwitch } from "@/components/layout/theme-switch";
import { UserMenu } from "@/components/layout/user-menu";
import { routes } from "@/config/routes";
import { ImpersonationBanner } from "@/features/auth/components/impersonation-banner";

/**
 * Shell-independent frame for pages every signed-in user can open (profile,
 * organization picker, "no access yet"): top bar only, no sidebar.
 */
export function AccountLayout({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-svh flex-col">
      <header className="bg-background/80 sticky top-0 z-40 border-b backdrop-blur">
        <div className="mx-auto flex h-16 w-full max-w-5xl items-center gap-3 px-4 md:px-6">
          <Link
            href={routes.account.home}
            className="flex min-w-0 items-center gap-2"
          >
            <AppSidebarLogo />
          </Link>
          <div className="ms-auto flex items-center gap-1">
            <LocaleSwitch />
            <ThemeSwitch />
            <UserMenu />
          </div>
        </div>
      </header>
      <main className="mx-auto flex w-full max-w-5xl min-w-0 flex-1 flex-col gap-4 px-4 pt-4 pb-10 md:px-6">
        <ImpersonationBanner />
        {children}
      </main>
    </div>
  );
}
