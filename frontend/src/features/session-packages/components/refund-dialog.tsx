"use client";

import { useMemo, useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppForm,
  AppInput,
  AppSelect,
  AppTextarea,
  AppToggleGroup,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { useCashAccountOptions } from "@/features/cash/hooks/use-cash";
import { normalizeAmount, useCashAmount } from "@/features/cash/lib/format";
import {
  MAX_NOTE,
  METHOD_LABEL_KEYS,
  PACKAGE_METHODS,
} from "@/features/session-packages/constants";
import { useRefundPackage } from "@/features/session-packages/hooks/use-session-packages";
import type { CustomerPackage } from "@/features/session-packages/types";
import { toMinor } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "refund-package-form";
const AMOUNT = /^\d+([.,]\d{1,2})?$/;

type Translate = (key: string) => string;

function createRefundSchema(t: Translate, maxMinor: number) {
  return z
    .object({
      amount: z
        .string()
        .trim()
        .refine((v) => AMOUNT.test(v), t("session_packages.validation.amount"))
        .refine(
          (v) => (toMinor(normalizeAmount(v)) ?? 0) <= maxMinor,
          t("session_packages.validation.refund_too_large"),
        ),
      method: z.enum(PACKAGE_METHODS as ["cash", "card", "transfer"]),
      cash_account_uuid: z.string(),
      reason: z
        .string()
        .trim()
        .min(3, t("session_packages.validation.reason_min"))
        .max(MAX_NOTE, t("session_packages.validation.note_max")),
    })
    .superRefine((values, ctx) => {
      const minor = toMinor(normalizeAmount(values.amount)) ?? 0;
      if (minor > 0 && !values.cash_account_uuid) {
        ctx.addIssue({
          code: "custom",
          path: ["cash_account_uuid"],
          message: t("session_packages.validation.account_required"),
        });
      }
    });
}

type RefundFormValues = z.infer<ReturnType<typeof createRefundSchema>>;

/**
 * Closes a package and pays part of its price back. The suggested amount
 * is the value of the sessions neither used nor held; the kept part stays
 * the customer's charge. Held sessions must be released first (API 409).
 */
export function RefundDialog({
  pkg,
  open,
  onOpenChange,
}: {
  pkg: CustomerPackage;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const money = useCashAmount();
  const refund = useRefundPackage(pkg.uuid);
  const accounts = useCashAccountOptions(open);
  const [idempotencyKey, setIdempotencyKey] = useState(() =>
    crypto.randomUUID(),
  );
  const schema = useMemo(
    () => createRefundSchema(t, toMinor(pkg.price) ?? 0),
    [pkg.price, t],
  );
  const options = (accounts.data?.items ?? [])
    .filter((a) => a.status === "active" && a.currency === pkg.currency)
    .map((a) => ({
      value: a.uuid,
      label: a.branch_name ? `${a.name} · ${a.branch_name}` : a.name,
    }));

  const close = (next: boolean) => {
    if (!next) setIdempotencyKey(crypto.randomUUID());
    onOpenChange(next);
  };

  return (
    <AppDialog
      open={open}
      onOpenChange={close}
      title={t("session_packages.refund.title")}
      description={t("session_packages.refund.description", {
        name: pkg.name,
        remaining: pkg.remaining,
        amount: money(pkg.remaining_amount, pkg.currency),
      })}
      className="sm:max-w-lg"
      footer={
        <>
          <Button type="button" variant="outline" onClick={() => close(false)}>
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={FORM_ID}
            variant="destructive"
            disabled={refund.isPending}
          >
            {t("session_packages.refund.submit")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<RefundFormValues>
          key={`${pkg.uuid}-${accounts.isSuccess}`}
          id={FORM_ID}
          className="grid gap-4"
          schema={schema}
          defaultValues={{
            amount: pkg.remaining_amount,
            method: "cash",
            cash_account_uuid: options.length === 1 ? options[0].value : "",
            reason: "",
          }}
          onSubmit={async (values) => {
            await refund.mutateAsync({
              amount: normalizeAmount(values.amount),
              method: values.method,
              cash_account_uuid: values.cash_account_uuid || null,
              reason: values.reason.trim(),
              idempotency_key: idempotencyKey,
            });
            close(false);
          }}
        >
          <AppInput
            name="amount"
            inputMode="decimal"
            label={t("session_packages.refund.fields.amount", {
              currency: pkg.currency,
            })}
            description={t("session_packages.refund.fields.amount_hint", {
              price: money(pkg.price, pkg.currency),
            })}
          />
          <AppToggleGroup
            name="method"
            label={t("session_packages.refund.fields.method")}
            options={PACKAGE_METHODS.map((value) => ({
              value,
              label: t(METHOD_LABEL_KEYS[value]),
            }))}
          />
          <AppSelect
            name="cash_account_uuid"
            label={t("session_packages.refund.fields.cash_account")}
            placeholder={t(
              "session_packages.sell.fields.cash_account_placeholder",
            )}
            options={options}
          />
          <AppTextarea
            name="reason"
            label={t("session_packages.refund.fields.reason")}
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
