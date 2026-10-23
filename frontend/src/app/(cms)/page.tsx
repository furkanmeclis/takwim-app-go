"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

export default function CmsHomePage() {
  const { t } = useLocale();
  const { user } = useAuth();

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          {t("cms.home.title")}
        </h1>
        <p className="text-muted-foreground mt-1 text-sm">
          {t("cms.home.subtitle")}
        </p>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>{t("cms.home.welcome")}</CardTitle>
        </CardHeader>
        <CardContent className="text-muted-foreground text-sm">
          {user?.fullName
            ? t("cms.home.greeting", { name: user.fullName })
            : t("cms.home.placeholder")}
        </CardContent>
      </Card>
    </div>
  );
}
