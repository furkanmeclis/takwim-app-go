"use client";

import { KeyRound, Phone } from "lucide-react";
import { useMemo, useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppInput } from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  useStartPhoneChange,
  useVerifyPhoneChange,
} from "@/features/customer-panel/hooks/use-customer-panel";
import { customerErrorMessage } from "@/features/customer-panel/lib/errors";
import type { CustomerSignInStart } from "@/features/customer-panel/types";
import { appToast } from "@/providers/toast-provider";
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

/**
 * Safe number change: a code goes to the new number; once it is entered
 * the profile (and every visit it sees) moves to that number, the old
 * number starts empty for its next holder, and the other devices are
 * signed out.
 */
export function CustomerPhoneChangeDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const start = useStartPhoneChange();
  const verify = useVerifyPhoneChange();
  const [sent, setSent] = useState<{
    phone: string;
    result: CustomerSignInStart;
  } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const phoneForm = useMemo(() => phoneSchema(t), [t]);
  const codeForm = useMemo(() => codeSchema(t), [t]);

  const close = (next: boolean) => {
    if (!next) {
      setSent(null);
      setError(null);
    }
    onOpenChange(next);
  };

  const requestCode = async (phone: string) => {
    setError(null);
    try {
      const result = await start.mutateAsync(phone);
      setSent({ phone, result });
    } catch (err) {
      setError(customerErrorMessage(t, err));
    }
  };

  return (
    <AppDialog
      open={open}
      onOpenChange={close}
      title={t("customer_panel.phone_change.title")}
      description={
        sent
          ? t("customer_panel.phone_change.code_description", {
              phone: sent.result.phone_masked,
            })
          : t("customer_panel.phone_change.description")
      }
    >
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
      {sent ? (
        <AppForm<CodeValues>
          key="code"
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
              appToast.success(t("customer_panel.phone_change.done"));
              close(false);
            } catch (err) {
              setError(customerErrorMessage(t, err));
            }
          }}
        >
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
          <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setSent(null);
                setError(null);
              }}
            >
              {t("customer_panel.login.change_phone")}
            </Button>
            <Button type="submit" disabled={verify.isPending}>
              {t("customer_panel.phone_change.confirm")}
            </Button>
          </div>
        </AppForm>
      ) : (
        <AppForm<PhoneValues>
          key="phone"
          schema={phoneForm}
          defaultValues={{ phone: "" }}
          className="grid gap-4"
          onSubmit={(values) => requestCode(values.phone)}
        >
          <AppInput
            name="phone"
            type="tel"
            label={t("customer_panel.phone_change.new_phone")}
            placeholder={t("customer_panel.login.phone_placeholder")}
            startIcon={Phone}
            inputMode="tel"
            autoComplete="tel"
            autoFocus
          />
          <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
            <Button
              type="button"
              variant="outline"
              onClick={() => close(false)}
            >
              {t("customer_panel.actions.keep")}
            </Button>
            <Button type="submit" disabled={start.isPending}>
              {t("customer_panel.login.send_code")}
            </Button>
          </div>
        </AppForm>
      )}
    </AppDialog>
  );
}
