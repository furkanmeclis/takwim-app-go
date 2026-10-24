"use client";

import { useMemo } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppCombobox,
  AppDatePicker,
  AppForm,
  AppInput,
  AppSelect,
  AppTextarea,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  useCorrectFinanceEntry,
  useUpdateFinanceEntry,
} from "@/features/finance/hooks/use-finance";
import { useFinanceFormOptions } from "@/features/finance/hooks/use-finance-options";
import {
  NONE,
  classifyStaff,
  correctFormDefaults,
  correctFormToRequest,
  createClassifyFormSchema,
  createCorrectFormSchema,
  type ClassifyFormValues,
  type CorrectFormValues,
} from "@/features/finance/schemas/finance-forms";
import type { FinanceEntry } from "@/features/finance/types";
import { useLocale } from "@/providers/locale-provider";

const CLASSIFY_FORM = "finance-classify-form";
const CORRECT_FORM = "finance-correct-form";

type DialogProps = {
  entry: FinanceEntry;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

/** Changes category (same kind), description and staff; money stays. */
export function ClassifyDialog({ entry, open, onOpenChange }: DialogProps) {
  const { t } = useLocale();
  const update = useUpdateFinanceEntry(entry.uuid);
  const schema = useMemo(() => createClassifyFormSchema(t), [t]);
  const options = useFinanceFormOptions({
    kind: entry.kind,
    enabled: open,
    keep: { category: entry.category, staff: entry.staff },
  });

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("finance.classify_dialog.title")}
      description={t("finance.classify_dialog.description")}
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
          <Button
            type="submit"
            form={CLASSIFY_FORM}
            disabled={update.isPending}
          >
            {t("form.save")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<ClassifyFormValues>
          id={CLASSIFY_FORM}
          className="grid gap-4"
          schema={schema}
          defaultValues={{
            category_uuid: entry.category.uuid,
            staff_uuid: entry.staff?.uuid ?? NONE,
            description: entry.description,
          }}
          onSubmit={async (values) => {
            await update.mutateAsync({
              category_uuid: values.category_uuid,
              staff_uuid: classifyStaff(values.staff_uuid),
              description: values.description.trim(),
            });
            onOpenChange(false);
          }}
        >
          <AppSelect
            name="category_uuid"
            label={t("finance.fields.category")}
            options={options.categoryOptions}
          />
          <AppCombobox
            name="staff_uuid"
            label={t("finance.fields.staff")}
            placeholder={t("finance.fields.staff_none")}
            options={options.staffOptions}
          />
          <AppTextarea
            name="description"
            label={t("finance.fields.description")}
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

/**
 * Corrects amount, date, account, category or staff: the record is
 * reversed today and replaced by a new one in one step. Both stay in the
 * history; the document moves to the replacement.
 */
export function CorrectDialog({ entry, open, onOpenChange }: DialogProps) {
  const { t } = useLocale();
  const correct = useCorrectFinanceEntry(entry.uuid);
  const schema = useMemo(() => createCorrectFormSchema(t), [t]);
  const options = useFinanceFormOptions({
    kind: entry.kind,
    enabled: open,
    keep: { category: entry.category, staff: entry.staff },
  });
  const accountOptions = options.accountOptions.some(
    (o) => o.value === entry.account.uuid,
  )
    ? options.accountOptions
    : [
        { value: entry.account.uuid, label: entry.account.name },
        ...options.accountOptions,
      ];

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("finance.correct_dialog.title")}
      description={t("finance.correct_dialog.description")}
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
          <Button
            type="submit"
            form={CORRECT_FORM}
            variant="destructive"
            disabled={correct.isPending}
          >
            {t("finance.correct_dialog.submit")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<CorrectFormValues>
          id={CORRECT_FORM}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={correctFormDefaults(entry)}
          onSubmit={async (values) => {
            await correct.mutateAsync(correctFormToRequest(entry, values));
            onOpenChange(false);
          }}
        >
          <AppSelect
            name="category_uuid"
            label={t("finance.fields.category")}
            options={options.categoryOptions}
            className="sm:col-span-2"
          />
          <AppSelect
            name="cash_account_uuid"
            label={t(
              entry.kind === "income"
                ? "finance.fields.account_income"
                : "finance.fields.account_expense",
            )}
            options={accountOptions}
            className="sm:col-span-2"
          />
          <AppInput
            name="amount"
            inputMode="decimal"
            label={t("finance.fields.amount")}
          />
          <AppDatePicker
            name="entry_date"
            label={t("finance.fields.entry_date")}
          />
          <AppCombobox
            name="staff_uuid"
            label={t("finance.fields.staff")}
            placeholder={t("finance.fields.staff_none")}
            options={options.staffOptions}
            className="sm:col-span-2"
          />
          <AppTextarea
            name="description"
            label={t("finance.fields.description")}
            className="sm:col-span-2"
          />
          <AppTextarea
            name="reason"
            label={t("finance.fields.reason")}
            className="sm:col-span-2"
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
