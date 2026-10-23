"use client";

import { Link2Off, LogIn } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { Loading } from "@/components/common/loading";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { AuthCard } from "@/features/auth/components/auth-card";
import { AuthShell } from "@/features/auth/components/auth-shell";
import { useRedeemPanelLink } from "@/features/customer-panel/hooks/use-customer-panel";
import { customerErrorMessage } from "@/features/customer-panel/lib/errors";
import { useFragmentToken } from "@/features/customer-panel/lib/fragment";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

/**
 * `/panel/l#token`: the single-use panel link of the booking SMS. Opening
 * the page does not spend the link (link previews and scanners open it
 * too); the person presses the button. An expired or used link leads to
 * the code sign-in, without any sign-up.
 */
export function CustomerLinkPage() {
  const { t } = useLocale();
  const router = useRouter();
  const token = useFragmentToken();
  const redeem = useRedeemPanelLink();
  const [failed, setFailed] = useState<string | null>(null);

  const invalid =
    token === "" ||
    (redeem.isError &&
      isApiError(redeem.error) &&
      redeem.error.code === "LINK_INVALID");

  const enter = () => {
    if (!token) return;
    setFailed(null);
    redeem.mutate(token, {
      onSuccess: () => router.replace(routes.customer.root),
      onError: (error) => setFailed(customerErrorMessage(t, error)),
    });
  };

  return (
    <AuthShell homeHref={routes.customer.login}>
      {token === undefined ? (
        <Loading label={t("common.loading")} />
      ) : invalid ? (
        <AuthCard
          title={t("customer_panel.link.invalid_title")}
          description={t("customer_panel.link.invalid_description")}
        >
          <div className="grid gap-4">
            <Link2Off className="text-muted-foreground mx-auto size-10" />
            <Button asChild className="w-full">
              <Link href={routes.customer.login}>
                <LogIn className="size-4" />
                {t("customer_panel.link.sign_in_with_code")}
              </Link>
            </Button>
          </div>
        </AuthCard>
      ) : (
        <AuthCard
          title={t("customer_panel.link.title")}
          description={t("customer_panel.link.description")}
          footer={t("customer_panel.link.footer")}
        >
          <div className="grid gap-4">
            {failed ? (
              <Alert variant="destructive">
                <AlertDescription>{failed}</AlertDescription>
              </Alert>
            ) : null}
            <Button
              className="w-full"
              disabled={redeem.isPending}
              onClick={enter}
            >
              <LogIn className="size-4" />
              {t("customer_panel.link.enter")}
            </Button>
          </div>
        </AuthCard>
      )}
    </AuthShell>
  );
}
