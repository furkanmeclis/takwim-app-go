"use client";

import { useMemo } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppDatePicker,
  AppForm,
  AppInput,
  AppSelect,
  AppTextarea,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  useCashAccountOptions,
  useCashTransfer,
} from "@/features/cash/hooks/use-cash";
import { kindLabel, useBusinessToday } from "@/features/cash/lib/format";
import {
  createTransferFormSchema,
  transferFormToRequest,
  type TransferFormValues,
} from "@/features/cash/schemas/cash-forms";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "cash-transfer-form";

/**
 * Moves money between two accounts of the business: till → bank deposit,
 * online provider → bank payout, branch till → head office. Both legs are
 * written together and are not revenue.
 */
export function TransferDialog({
  fromAccountUuid,
  open,
  onOpenChange,
}: {
  /** Preselected source account. */
  fromAccountUuid?: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const today = useBusinessToday();
  const transfer = useCashTransfer();
  const options = useCashAccountOptions(open);
  const schema = useMemo(() => createTransferFormSchema(t), [t]);

  const accounts = useMemo(
    () => (options.data?.items ?? []).filter((a) => a.status === "active"),
    [options.data?.items],
  );
  const accountOptions = useMemo(
    () =>
      accounts.map((account) => ({
        value: account.uuid,
        label: [
          account.name,
          kindLabel(t, account.kind),
          account.branch_name || t("cash.head_office"),
        ].join(" · "),
      })),
    [accounts, t],
  );

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("cash.transfer_dialog.title")}
      description={t("cash.transfer_dialog.description")}
      className="sm:max-w-xl"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={transfer.isPending}>
            {t("cash.transfer_dialog.submit")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<TransferFormValues>
          key={fromAccountUuid ?? "any"}
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={{
            from_account_uuid: fromAccountUuid ?? "",
            to_account_uuid: "",
            amount: "",
            movement_date: today,
            description: "",
          }}
          onSubmit={async (values) => {
            await transfer.mutateAsync(transferFormToRequest(values));
            onOpenChange(false);
          }}
        >
          {(form) => {
            const from = form.watch("from_account_uuid");
            const currency = accounts.find((a) => a.uuid === from)?.currency;
            const targets = accountOptions.filter(
              (option) =>
                option.value !== from &&
                (!currency ||
                  accounts.find((a) => a.uuid === option.value)?.currency ===
                    currency),
            );
            return (
              <>
                <AppSelect
                  name="from_account_uuid"
                  label={t("cash.fields.from_account")}
                  placeholder={t("cash.fields.account_placeholder")}
                  options={accountOptions}
                />
                <AppSelect
                  name="to_account_uuid"
                  label={t("cash.fields.to_account")}
                  placeholder={t("cash.fields.account_placeholder")}
                  options={targets}
                />
                <AppInput
                  name="amount"
                  inputMode="decimal"
                  label={t("cash.fields.amount", {
                    currency: currency ?? "",
                  })}
                />
                <AppDatePicker
                  name="movement_date"
                  label={t("cash.fields.movement_date")}
                />
                <AppTextarea
                  name="description"
                  label={t("cash.fields.description")}
                  description={t("cash.transfer_dialog.description_hint")}
                  className="sm:col-span-2"
                />
              </>
            );
          }}
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
