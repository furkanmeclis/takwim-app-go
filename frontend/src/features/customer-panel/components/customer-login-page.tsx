"use client";

import { KeyRound, Phone } from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import { z } from "zod";

import { AppForm, AppInput } from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { AuthCard } from "@/features/auth/components/auth-card";
import { AuthShell } from "@/features/auth/components/auth-shell";
import {
  useCustomerProfile,
  useStartSignIn,
  useVerifySignIn,
} from "@/features/customer-panel/hooks/use-customer-panel";
import { customerErrorMessage } from "@/features/customer-panel/lib/errors";
import { safeNext } from "@/features/customer-panel/lib/navigation";
import type { CustomerSignInStart } from "@/features/customer-panel/types";
import { useLocale } from "@/providers/locale-provider";

type Translate = (key: string) => string;

function phoneSchema(t: Translate) {
  return z.object({
    phone: z
      .string()
      .trim()
      .min(7, t("customer_panel.login.invalid_phone"))
      .max(32, t("customer_panel.login.invalid_phone")),
  });
}

function codeSchema(t: Translate) {
  return z.object({
    code: z
      .string()
      .trim()
      .regex(/^\d{6}$/, t("customer_panel.login.code_format")),
  });
}

type PhoneValues = z.infer<ReturnType<typeof phoneSchema>>;
type CodeValues = z.infer<ReturnType<typeof codeSchema>>;

/** Seconds left until `at` (0 when passed), ticking every second. */
function useCountdown(at: number | null) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!at) return;
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [at]);
  return at ? Math.max(0, Math.ceil((at - now) / 1000)) : 0;
}

/**
 * `/panel/login`: phone, then the 6 digit code sent by SMS. The API answers
 * every valid phone the same way (it does not tell whether a business knows
 * the number); a code reaches only numbers a business has on record.
 */
export function CustomerLoginPage() {
  const { t } = useLocale();
  const router = useRouter();
  const searchParams = useSearchParams();
  const next = safeNext(searchParams.get("next"));
  const profile = useCustomerProfile();
  const start = useStartSignIn();
  const verify = useVerifySignIn();
  const [sent, setSent] = useState<{
    phone: string;
    result: CustomerSignInStart;
  } | null>(null);
  const [resendAt, setResendAt] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const wait = useCountdown(resendAt);
  const phoneForm = useMemo(() => phoneSchema(t), [t]);
  const codeForm = useMemo(() => codeSchema(t), [t]);

  // Already signed in: straight to the panel.
  useEffect(() => {
    if (profile.data) router.replace(next);
  }, [next, profile.data, router]);

  const requestCode = async (phone: string) => {
    setError(null);
    try {
      const result = await start.mutateAsync(phone);
      setSent({ phone, result });
      setResendAt(Date.now() + result.resend_after_seconds * 1000);
    } catch (err) {
      setError(customerErrorMessage(t, err));
    }
  };

  return (
    <AuthShell homeHref={routes.customer.root}>
      {sent ? (
        <AuthCard
          title={t("customer_panel.login.code_title")}
          description={t("customer_panel.login.code_description", {
            phone: sent.result.phone_masked,
          })}
        >
          <AppForm<CodeValues>
            schema={codeForm}
            defaultValues={{ code: "" }}
            className="grid gap-4"
            onSubmit={async (values) => {
              setError(null);
              try {
                await verify.mutateAsync({
                  phone: sent.phone,
                  code: values.code,
                });
                router.replace(next);
              } catch (err) {
                setError(customerErrorMessage(t, err));
              }
            }}
          >
            {error ? (
              <Alert variant="destructive">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            ) : null}
            <AppInput
              name="code"
              label={t("customer_panel.login.code")}
              startIcon={KeyRound}
              inputMode="numeric"
              autoComplete="one-time-code"
              maxLength={6}
              autoFocus
              className="tracking-widest"
            />
            <Button
              type="submit"
              className="w-full"
              disabled={verify.isPending}
            >
              {t("customer_panel.login.verify")}
            </Button>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <Button
                type="button"
                variant="link"
                className="px-0"
                onClick={() => {
                  setSent(null);
                  setError(null);
                }}
              >
                {t("customer_panel.login.change_phone")}
              </Button>
              <Button
                type="button"
                variant="link"
                className="px-0"
                disabled={wait > 0 || start.isPending}
                onClick={() => void requestCode(sent.phone)}
              >
                {wait > 0
                  ? t("customer_panel.login.resend_in", {
                      seconds: wait,
                    })
                  : t("customer_panel.login.resend")}
              </Button>
            </div>
          </AppForm>
        </AuthCard>
      ) : (
        <AuthCard
          title={t("customer_panel.login.title")}
          description={t("customer_panel.login.description")}
          footer={t("customer_panel.login.footer")}
        >
          <AppForm<PhoneValues>
            schema={phoneForm}
            defaultValues={{ phone: "" }}
            className="grid gap-4"
            onSubmit={(values) => requestCode(values.phone)}
          >
            {error ? (
              <Alert variant="destructive">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            ) : null}
            <AppInput
              name="phone"
              type="tel"
              label={t("customer_panel.login.phone")}
              placeholder={t("customer_panel.login.phone_placeholder")}
              startIcon={Phone}
              inputMode="tel"
              autoComplete="tel"
              autoFocus
            />
            <Button type="submit" className="w-full" disabled={start.isPending}>
              {t("customer_panel.login.send_code")}
            </Button>
          </AppForm>
        </AuthCard>
      )}
    </AuthShell>
  );
}
