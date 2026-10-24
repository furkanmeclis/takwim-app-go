"use client";

import { CalendarCheck } from "lucide-react";
import Link from "next/link";
import type { ReactNode } from "react";

import { AppLogo } from "@/components/brand";
import { LocaleSwitch } from "@/components/layout/locale-switch";
import { ThemeSwitch } from "@/components/layout/theme-switch";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { useBookingSurface } from "@/features/business-page/components/booking-surface";
import { BusinessTheme } from "@/features/business-page/components/business-theme";
import { CUSTOMER_PANEL_PATH } from "@/features/business-page/constants";
import type { PublicBusiness } from "@/features/business-page/types";
import { resolveLogoSrc } from "@/lib/branding/logo-url";
import { assertServiceMediaURL } from "@/lib/media/urls";
import { useLocale } from "@/providers/locale-provider";

function initials(name: string) {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]?.toLocaleUpperCase("tr"))
    .join("");
}

/** Logo of the business from its API-owned URL, else its initials. */
export function BusinessAvatar({
  business,
  className,
}: {
  business: Pick<PublicBusiness, "name" | "logo_url">;
  className?: string;
}) {
  const src = resolveLogoSrc(assertServiceMediaURL(business.logo_url));
  return (
    <Avatar className={className}>
      {src ? <AvatarImage src={src} alt={business.name} /> : null}
      <AvatarFallback className="font-semibold">
        {initials(business.name)}
      </AvatarFallback>
    </Avatar>
  );
}

/**
 * "Randevularım": the shared customer panel. Inside the embedded screen it
 * opens in a new tab, because the panel is never framed.
 */
export function CustomerPanelLink({
  className,
  children,
}: {
  className?: string;
  children: ReactNode;
}) {
  const { embed } = useBookingSurface();
  return (
    <Link
      href={CUSTOMER_PANEL_PATH}
      className={className}
      {...(embed ? { target: "_blank", rel: "noopener noreferrer" } : {})}
    >
      {children}
    </Link>
  );
}

/**
 * Frame of the embedded screen (TEC-908). No sticky header and no
 * viewport height, so an inline frame takes exactly its content's height.
 * The compact view is the wizard alone; the full view adds the business's
 * name, language and colour mode.
 */
function EmbedShell({
  business,
  full,
  children,
}: {
  business?: PublicBusiness;
  full: boolean;
  children: ReactNode;
}) {
  const { t } = useLocale();
  return (
    <div className="bg-background flex flex-col">
      {full ? (
        <header className="border-b">
          <div className="mx-auto flex h-14 w-full max-w-3xl items-center justify-between gap-2 px-4">
            <div className="flex min-w-0 items-center gap-2">
              {business ? (
                <BusinessAvatar business={business} className="size-8" />
              ) : null}
              <span className="truncate text-sm font-semibold">
                {business?.name ?? "…"}
              </span>
            </div>
            <div className="flex shrink-0 items-center gap-1">
              <Button asChild variant="ghost" size="sm">
                <CustomerPanelLink>
                  <CalendarCheck className="size-4" />
                  <span className="sr-only sm:not-sr-only">
                    {t("business_page.my_bookings")}
                  </span>
                </CustomerPanelLink>
              </Button>
              <LocaleSwitch />
              <ThemeSwitch />
            </div>
          </div>
        </header>
      ) : null}
      <main className="mx-auto flex w-full max-w-3xl flex-col gap-4 px-4 py-4">
        {children}
      </main>
      <footer className="text-muted-foreground mx-auto flex w-full max-w-3xl items-center justify-between gap-2 px-4 pb-4 text-xs">
        <span className="flex items-center gap-2">
          <span>{t("business_page.powered_by")}</span>
          <AppLogo variant="logo" className="h-4 w-auto" />
        </span>
        {full ? null : <LocaleSwitch />}
      </footer>
    </div>
  );
}

/**
 * Frame of the business page (takwim.link/{slug}): the business's logo and
 * name, language and colour mode, and the way to the customer's own panel.
 * No panel sidebar; mobile first (375 px) and centred on wide screens. Both
 * frames sit inside the business's theme (TEC-966).
 */
export function BusinessPageShell({
  business,
  slug,
  children,
}: {
  business?: PublicBusiness;
  slug: string;
  children: ReactNode;
}) {
  const { t } = useLocale();
  const { embed } = useBookingSurface();
  if (embed) {
    return (
      <BusinessTheme scope={slug} theme={business?.theme}>
        <EmbedShell business={business} full={embed.view === "full"}>
          {children}
        </EmbedShell>
      </BusinessTheme>
    );
  }
  return (
    <BusinessTheme
      scope={slug}
      theme={business?.theme}
      className="bg-muted/40 flex min-h-svh flex-col"
    >
      <header className="bg-background/95 sticky top-0 z-20 border-b backdrop-blur">
        <div className="mx-auto flex h-14 w-full max-w-3xl items-center justify-between gap-2 px-4">
          <Link
            href={`/b/${encodeURIComponent(slug)}`}
            className="flex min-w-0 items-center gap-2"
          >
            {business ? (
              <BusinessAvatar business={business} className="size-8" />
            ) : null}
            <span className="truncate text-sm font-semibold">
              {business?.name ?? "…"}
            </span>
          </Link>
          <div className="flex shrink-0 items-center gap-1">
            <Button asChild variant="ghost" size="sm">
              <Link href={CUSTOMER_PANEL_PATH}>
                <CalendarCheck className="size-4" />
                <span className="sr-only sm:not-sr-only">
                  {t("business_page.my_bookings")}
                </span>
              </Link>
            </Button>
            <LocaleSwitch />
            <ThemeSwitch />
          </div>
        </div>
      </header>
      <main className="mx-auto flex w-full max-w-3xl flex-1 flex-col gap-4 px-4 py-4 sm:py-6">
        {children}
      </main>
      <footer className="text-muted-foreground mx-auto flex w-full max-w-3xl items-center justify-center gap-2 px-4 py-6 text-xs">
        <span>{t("business_page.powered_by")}</span>
        <AppLogo variant="logo" className="h-4 w-auto" />
      </footer>
    </BusinessTheme>
  );
}
