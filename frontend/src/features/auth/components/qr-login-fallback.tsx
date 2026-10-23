"use client";

import Link from "next/link";
import { Smartphone } from "lucide-react";

import { brand } from "@/config/brand";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";

import { AuthCard } from "./auth-card";
import { AuthShell } from "./auth-shell";

const SESSION_ID = /^[A-Za-z0-9_-]{43}$/;

/**
 * Landing page for `/login/qr/{id}` when the QR was opened in a browser
 * instead of the mobile app (app not installed, or universal links off).
 */
export function QRLoginFallback({ sessionId }: { sessionId: string }) {
  const { t } = useLocale();
  const valid = SESSION_ID.test(sessionId);

  return (
    <AuthShell>
      <AuthCard
        title={t("auth.qr.fallback_title", { app: brand.name })}
        description={t("auth.qr.fallback_description", { app: brand.name })}
      >
        <div className="flex flex-col items-center gap-4 text-center">
          <Smartphone className="text-muted-foreground size-10" aria-hidden />
          <ol className="text-muted-foreground list-decimal space-y-1 pl-5 text-left text-sm">
            <li>{t("auth.qr.step_open", { app: brand.name })}</li>
            <li>{t("auth.qr.step_scan")}</li>
            <li>{t("auth.qr.step_approve")}</li>
          </ol>
          {valid ? (
            <Button asChild className="w-full">
              <a href={`${brand.mobileScheme}://login/qr/${sessionId}`}>
                {t("auth.qr.open_in_app", { app: brand.name })}
              </a>
            </Button>
          ) : null}
          <Button asChild variant="outline" className="w-full">
            <Link href={routes.guest.login}>{t("auth.back_to_login")}</Link>
          </Button>
        </div>
      </AuthCard>
    </AuthShell>
  );
}
