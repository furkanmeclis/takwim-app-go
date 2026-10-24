"use client";

import { useMemo, useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppDatePicker,
  AppForm,
  AppInput,
  AppSelect,
  AppTextarea,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { useCashAccountOptions } from "@/features/cash/hooks/use-cash";
import {
  normalizeAmount,
  useBusinessToday,
  useCashAmount,
} from "@/features/cash/lib/format";
import { MAX_NOTE } from "@/features/staff-shares/constants";
import { usePayStaffShare } from "@/features/staff-shares/hooks/use-staff-shares";
import type { StaffShareSummaryRow } from "@/features/staff-shares/types";
import { toMinor } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "staff-share-payout-form";
const AMOUNT = /^\d+([.,]\d{1,2})?$/;

type Translate = (key: string) => string;

function createPayoutSchema(t: Translate, maxMinor: number) {
  return z.object({
    cash_account_uuid: z
      .string()
      .min(1, t("staff_shares.validation.account_required")),
    amount: z
      .string()
      .trim()
      .refine((v) => AMOUNT.test(v), t("staff_shares.validation.amount"))
      .refine(
        (v) => Number(v.replace(",", ".")) > 0,
        t("staff_shares.validation.amount_positive"),
      )
      .refine(
        (v) => (toMinor(normalizeAmount(v)) ?? 0) <= maxMinor,
        t("staff_shares.validation.amount_too_large"),
      ),
    date: z.string().min(1, t("staff_shares.validation.date_required")),
    note: z.string().max(MAX_NOTE, t("staff_shares.validation.note_max")),
  });
}

type PayoutFormValues = z.infer<ReturnType<typeof createPayoutSchema>>;

/**
 * Pays a staff member from a cash or bank account. The API posts a staff
 * payment on the account (not a second expense) and refuses more than the
 * payable balance; the idempotency key makes a double submit post once.
 */
export function PayoutDialog({
  row,
  open,
  onOpenChange,
}: {
  row: StaffShareSummaryRow | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const money = useCashAmount();
  const today = useBusinessToday();
  const pay = usePayStaffShare();
  const accounts = useCashAccountOptions(open);
  const [idempotencyKey, setIdempotencyKey] = useState(() =>
    crypto.randomUUID(),
  );
  const payable = row?.balance.payable_now ?? "0";
  const schema = useMemo(
    () => createPayoutSchema(t, toMinor(payable) ?? 0),
    [payable, t],
  );

  const options = (accounts.data?.items ?? [])
    .filter(
      (account) =>
        account.status === "active" &&
        (!row || account.currency === row.currency),
    )
    .map((account) => ({
      value: account.uuid,
      label: account.branch_name
        ? `${account.name} · ${account.branch_name}`
        : account.name,
    }));

  const close = (next: boolean) => {
    if (!next) setIdempotencyKey(crypto.randomUUID());
    onOpenChange(next);
  };

  return (
    <AppDialog
      open={open}
      onOpenChange={close}
      title={t("staff_shares.payout_dialog.title")}
      description={
        row
          ? t("staff_shares.payout_dialog.description", {
              name: row.staff.name,
              amount: money(payable, row.currency),
            })
          : undefined
      }
      className="sm:max-w-lg"
      footer={
        <>
          <Button type="button" variant="outline" onClick={() => close(false)}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={pay.isPending}>
            {t("staff_shares.payout_dialog.submit")}
          </Button>
        </>
      }
    >
      {open && row ? (
        <AppForm<PayoutFormValues>
          key={`${row.staff.uuid}-${accounts.isSuccess}`}
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={{
            cash_account_uuid: options.length === 1 ? options[0].value : "",
            amount: (toMinor(payable) ?? 0) > 0 ? payable : "",
            date: today,
            note: "",
          }}
          onSubmit={async (values) => {
            await pay.mutateAsync({
              staff_uuid: row.staff.uuid,
              cash_account_uuid: values.cash_account_uuid,
              amount: normalizeAmount(values.amount),
              date: values.date,
              note: values.note.trim() || undefined,
              idempotency_key: idempotencyKey,
            });
            close(false);
          }}
        >
          <AppSelect
            name="cash_account_uuid"
            label={t("staff_shares.fields.cash_account")}
            placeholder={t("staff_shares.fields.cash_account_placeholder")}
            description={
              accounts.isSuccess && options.length === 0
                ? t("staff_shares.fields.cash_account_none")
                : undefined
            }
            options={options}
            className="sm:col-span-2"
          />
          <AppInput
            name="amount"
            inputMode="decimal"
            label={t("staff_shares.fields.amount", { currency: row.currency })}
          />
          <AppDatePicker name="date" label={t("staff_shares.fields.date")} />
          <AppTextarea
            name="note"
            label={t("staff_shares.fields.note")}
            className="sm:col-span-2"
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
