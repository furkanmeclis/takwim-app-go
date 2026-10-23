"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";

import { defaultHomeForUser } from "@/lib/auth/types";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

/**
 * `/` has no content of its own: once the profile is fresh it sends the user
 * to their home (platform, their organization, the organization picker or
 * the "no access yet" page).
 */
export function HomeRedirect() {
  const router = useRouter();
  const { t } = useLocale();
  const { bootstrapped, user } = useAuth();

  useEffect(() => {
    // Wait for the verified profile: the cached one may list stale
    // organizations.
    if (!bootstrapped || !user) return;
    router.replace(defaultHomeForUser(user));
  }, [bootstrapped, router, user]);

  return (
    <div className="text-muted-foreground flex flex-1 items-center justify-center py-24 text-sm">
      {t("common.loading")}
    </div>
  );
}
