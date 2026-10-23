"use client";

import { useMemo } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppDatePicker,
  AppForm,
  AppInput,
  AppTextarea,
  AppToggleGroup,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { useCreateCashMovement } from "@/features/cash/hooks/use-cash";
import { useBusinessToday, useCashAmount } from "@/features/cash/lib/format";
import {
  createMovementFormSchema,
  movementFormToRequest,
  type MovementFormValues,
} from "@/features/cash/schemas/cash-forms";
import type { CashAccount, CashDirection } from "@/features/cash/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "cash-movement-form";

/** Manual money in / out on one account. */
export function MovementDialog({
  account,
  direction = "in",
  open,
  onOpenChange,
}: {
  account: CashAccount | null;
  direction?: CashDirection;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const money = useCashAmount();
  const today = useBusinessToday();
  const create = useCreateCashMovement(account?.uuid ?? "");
  const schema = useMemo(() => createMovementFormSchema(t), [t]);

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("cash.movement_dialog.title")}
      description={
        account
          ? t("cash.movement_dialog.description", {
              name: account.name,
              balance: money(account.balance, account.currency),
            })
          : undefined
      }
      className="sm:max-w-lg"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={create.isPending}>
            {t("cash.movement_dialog.submit")}
          </Button>
        </>
      }
    >
      {open && account ? (
        <AppForm<MovementFormValues>
          key={`${account.uuid}-${direction}`}
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={{
            direction,
            amount: "",
            movement_date: today,
            description: "",
          }}
          onSubmit={async (values) => {
            await create.mutateAsync(movementFormToRequest(values));
            onOpenChange(false);
          }}
        >
          <AppToggleGroup
            name="direction"
            label={t("cash.fields.direction")}
            className="sm:col-span-2"
            options={[
              { value: "in", label: t("cash.direction.in") },
              { value: "out", label: t("cash.direction.out") },
            ]}
          />
          <AppInput
            name="amount"
            inputMode="decimal"
            label={t("cash.fields.amount", { currency: account.currency })}
          />
          <AppDatePicker
            name="movement_date"
            label={t("cash.fields.movement_date")}
            description={t("cash.fields.movement_date_hint")}
          />
          <AppTextarea
            name="description"
            label={t("cash.fields.description")}
            description={t("cash.fields.description_hint")}
            className="sm:col-span-2"
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
