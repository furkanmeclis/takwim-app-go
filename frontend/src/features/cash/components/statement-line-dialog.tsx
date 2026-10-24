"use client";

import { useMemo } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppDatePicker,
  AppForm,
  AppInput,
  AppSelect,
  AppTextarea,
  AppToggleGroup,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { CASH_STATEMENT_LINE_TYPES } from "@/features/cash/constants";
import { useAddCashStatementLine } from "@/features/cash/hooks/use-cash";
import { useBusinessToday } from "@/features/cash/lib/format";
import {
  createLineFormSchema,
  lineFormToRequest,
  type LineFormValues,
} from "@/features/cash/schemas/cash-forms";
import type { CashStatement } from "@/features/cash/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "cash-statement-line-form";

/** One statement row typed by hand (a sheet import is the usual way in). */
export function StatementLineDialog({
  statement,
  open,
  onOpenChange,
}: {
  statement: CashStatement;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const today = useBusinessToday();
  const add = useAddCashStatementLine(statement.uuid);
  const schema = useMemo(() => createLineFormSchema(t), [t]);
  const typeOptions = useMemo(
    () =>
      CASH_STATEMENT_LINE_TYPES.map((type) => ({
        value: type,
        label: t(`cash.line_types.${type}`),
      })),
    [t],
  );

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("cash.line_dialog.title")}
      description={t("cash.line_dialog.description", { name: statement.name })}
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
          <Button type="submit" form={FORM_ID} disabled={add.isPending}>
            {t("cash.line_dialog.submit")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<LineFormValues>
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={{
            line_date: statement.period_to ?? today,
            direction: "in",
            amount: "",
            fee: "",
            line_type: statement.source === "provider" ? "payment" : "other",
            description: "",
            reference: "",
            balance_after: "",
          }}
          onSubmit={async (values) => {
            await add.mutateAsync(lineFormToRequest(values));
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
            label={t("cash.fields.amount", { currency: statement.currency })}
          />
          <AppDatePicker name="line_date" label={t("cash.fields.line_date")} />
          <AppSelect
            name="line_type"
            label={t("cash.fields.line_type")}
            options={typeOptions}
          />
          <AppInput
            name="fee"
            inputMode="decimal"
            label={t("cash.fields.fee", { currency: statement.currency })}
            description={t("cash.fields.fee_hint")}
          />
          <AppTextarea
            name="description"
            label={t("cash.fields.description")}
            className="sm:col-span-2"
          />
          <AppInput
            name="reference"
            label={t("cash.fields.reference")}
            description={t("cash.fields.reference_hint")}
          />
          <AppInput
            name="balance_after"
            inputMode="decimal"
            label={t("cash.fields.balance_after", {
              currency: statement.currency,
            })}
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
