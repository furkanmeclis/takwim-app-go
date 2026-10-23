"use client";

import { useQueryClient } from "@tanstack/react-query";
import { CalendarDays, LogOut, UserRound, Wallet } from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, type ReactNode } from "react";

import { AppSidebarLogo } from "@/components/brand";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { LocaleSwitch } from "@/components/layout/locale-switch";
import { ThemeSwitch } from "@/components/layout/theme-switch";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import {
  customerPanelKeys,
  useCustomerLogout,
  useCustomerProfile,
} from "@/features/customer-panel/hooks/use-customer-panel";
import { loginHref } from "@/features/customer-panel/lib/navigation";
import { setCustomerUnauthorizedHandler } from "@/features/customer-panel/services/customer-panel.service";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

const NAV = [
  {
    href: routes.customer.root,
    labelKey: "customer_panel.nav.reservations",
    icon: CalendarDays,
    match: (path: string) =>
      path === routes.customer.root || path.startsWith("/panel/reservations"),
  },
  {
    href: routes.customer.payments,
    labelKey: "customer_panel.nav.payments",
    icon: Wallet,
    match: (path: string) => path.startsWith(routes.customer.payments),
  },
  {
    href: routes.customer.profile,
    labelKey: "customer_panel.nav.profile",
    icon: UserRound,
    match: (path: string) => path.startsWith(routes.customer.profile),
  },
] as const;

/**
 * Frame of the customer panel (takwim.link/panel): top bar and a three-tab
 * navigation, no staff sidebar. It needs a customer session; without one
 * (or when it ends) the person goes to the panel's own sign-in.
 */
export function CustomerPanelShell({ children }: { children: ReactNode }) {
  const { t } = useLocale();
  const router = useRouter();
  const pathname = usePathname() ?? routes.customer.root;
  const queryClient = useQueryClient();
  const profile = useCustomerProfile();
  const logout = useCustomerLogout();

  useEffect(() => {
    setCustomerUnauthorizedHandler(() => {
      queryClient.removeQueries({ queryKey: customerPanelKeys.all });
      router.replace(loginHref(window.location.pathname));
    });
    return () => setCustomerUnauthorizedHandler(null);
  }, [queryClient, router]);

  const signedOut =
    profile.isError &&
    isApiError(profile.error) &&
    profile.error.status === 401;

  useEffect(() => {
    if (signedOut) router.replace(loginHref(pathname));
  }, [pathname, router, signedOut]);

  let body: ReactNode;
  if (profile.isLoading || signedOut) {
    body = <Loading label={t("common.loading")} />;
  } else if (profile.isError) {
    body = (
      <ErrorState
        title={t("customer_panel.errors.generic")}
        onRetry={() => void profile.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  } else {
    body = children;
  }

  return (
    <div className="flex min-h-svh flex-col">
      <header className="bg-background/80 sticky top-0 z-40 border-b backdrop-blur">
        <div className="mx-auto flex h-14 w-full max-w-5xl items-center gap-2 px-4 md:h-16 md:px-6">
          <Link
            href={routes.customer.root}
            className="flex min-w-0 items-center gap-2"
            aria-label={t("customer_panel.title")}
          >
            <AppSidebarLogo />
          </Link>
          <nav
            className="ms-6 hidden items-center gap-1 md:flex"
            aria-label={t("customer_panel.nav.label")}
          >
            {NAV.map((item) => (
              <Button
                key={item.href}
                asChild
                size="sm"
                variant={item.match(pathname) ? "secondary" : "ghost"}
              >
                <Link href={item.href}>
                  <item.icon className="size-4" />
                  {t(item.labelKey)}
                </Link>
              </Button>
            ))}
          </nav>
          <div className="ms-auto flex items-center gap-1">
            <LocaleSwitch />
            <ThemeSwitch />
            {profile.data ? (
              <Button
                variant="ghost"
                size="icon"
                aria-label={t("customer_panel.profile.logout")}
                title={t("customer_panel.profile.logout")}
                disabled={logout.isPending}
                onClick={() =>
                  logout.mutate(false, {
                    onSettled: () => router.replace(routes.customer.login),
                  })
                }
              >
                <LogOut className="size-4" />
              </Button>
            ) : null}
          </div>
        </div>
      </header>
      <main className="mx-auto flex w-full max-w-5xl min-w-0 flex-1 flex-col gap-4 px-4 pt-4 pb-24 md:px-6 md:pb-10">
        {body}
      </main>
      <nav
        className="bg-background/95 fixed inset-x-0 bottom-0 z-40 grid grid-cols-3 border-t pb-[env(safe-area-inset-bottom)] backdrop-blur md:hidden"
        aria-label={t("customer_panel.nav.label")}
      >
        {NAV.map((item) => {
          const active = item.match(pathname);
          return (
            <Button
              key={item.href}
              asChild
              variant="ghost"
              className={cn(
                "h-16 flex-col gap-1 rounded-none text-xs",
                active ? "text-primary" : "text-muted-foreground",
              )}
            >
              <Link href={item.href} aria-current={active ? "page" : undefined}>
                <item.icon className="size-5" />
                {t(item.labelKey)}
              </Link>
            </Button>
          );
        })}
      </nav>
    </div>
  );
}
