"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { MailQuestion, RefreshCw, UserRound } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { routes } from "@/config/routes";
import { defaultHomeForUser } from "@/lib/auth/types";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

/**
 * `/no-access`: signed in, but no platform permission and no organization
 * yet (typically waiting for an invitation).
 */
export function NoAccessPage() {
  const router = useRouter();
  const { t } = useLocale();
  const { user, hydrateProfile } = useAuth();
  const [checking, setChecking] = useState(false);

  const checkAgain = async () => {
    setChecking(true);
    try {
      const refreshed = await hydrateProfile();
      const home = refreshed ? defaultHomeForUser(refreshed) : null;
      if (home && home !== routes.account.noAccess) {
        router.replace(home);
      }
    } finally {
      setChecking(false);
    }
  };

  return (
    <div className="flex flex-1 items-center justify-center py-12">
      <Card className="w-full max-w-md">
        <CardHeader>
          <div className="bg-muted text-muted-foreground mb-2 flex size-10 items-center justify-center rounded-full">
            <MailQuestion className="size-5" />
          </div>
          <CardTitle>{t("tenant.no_access.title")}</CardTitle>
          <CardDescription>{t("tenant.no_access.body")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          {user?.email ? (
            <p className="text-muted-foreground text-sm">
              {t("tenant.access.signed_in_as", { email: user.email })}
            </p>
          ) : null}
          <Button onClick={checkAgain} disabled={checking}>
            <RefreshCw className={checking ? "animate-spin" : undefined} />
            {t("tenant.no_access.check_again")}
          </Button>
          <Button asChild variant="outline">
            <Link href={routes.account.profile.root}>
              <UserRound />
              {t("auth.profile")}
            </Link>
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
