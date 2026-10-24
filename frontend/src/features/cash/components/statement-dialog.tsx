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
import {
  useCreateCashStatement,
  useUpdateCashStatement,
} from "@/features/cash/hooks/use-cash";
import {
  createStatementFormSchema,
  statementFormDefaults,
  statementFormToCreate,
  statementFormToUpdate,
  type StatementFormValues,
} from "@/features/cash/schemas/cash-forms";
import type { CashAccount, CashStatement } from "@/features/cash/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "cash-statement-form";

/**
 * Opens a statement (bank statement or provider settlement report) on a bank
 * / online account, or edits the name, period and stated balance of an open
 * one. Lines are imported or added on the statement page.
 */
export function StatementDialog({
  account,
  statement,
  open,
  onOpenChange,
  onCreated,
}: {
  account: CashAccount;
  /** Edit mode when set. */
  statement?: CashStatement | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated?: (statement: CashStatement) => void;
}) {
  const { t } = useLocale();
  const create = useCreateCashStatement();
  const update = useUpdateCashStatement();
  const schema = useMemo(() => createStatementFormSchema(t), [t]);
  const editing = Boolean(statement);
  const pending = create.isPending || update.isPending;

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        editing
          ? t("cash.statement_dialog.edit_title")
          : t("cash.statement_dialog.title")
      }
      description={t("cash.statement_dialog.description", {
        name: account.name,
      })}
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
          <Button type="submit" form={FORM_ID} disabled={pending}>
            {editing
              ? t("cash.actions.save")
              : t("cash.statement_dialog.submit")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<StatementFormValues>
          key={statement?.uuid ?? account.uuid}
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={statementFormDefaults(
            statement ?? null,
            account.kind === "online" ? "provider" : "bank",
          )}
          onSubmit={async (values) => {
            if (statement) {
              await update.mutateAsync({
                uuid: statement.uuid,
                body: statementFormToUpdate(values),
              });
            } else {
              const created = await create.mutateAsync(
                statementFormToCreate(values, account.uuid),
              );
              onCreated?.(created);
            }
            onOpenChange(false);
          }}
        >
          {editing ? null : (
            <AppToggleGroup
              name="source"
              label={t("cash.fields.statement_source")}
              description={t("cash.fields.statement_source_hint")}
              className="sm:col-span-2"
              options={[
                { value: "bank", label: t("cash.statement_sources.bank") },
                {
                  value: "provider",
                  label: t("cash.statement_sources.provider"),
                },
              ]}
            />
          )}
          <AppInput
            name="name"
            label={t("cash.fields.statement_name")}
            description={t("cash.fields.statement_name_hint")}
            className="sm:col-span-2"
          />
          <AppDatePicker
            name="period_from"
            label={t("cash.fields.period_from")}
          />
          <AppDatePicker name="period_to" label={t("cash.fields.period_to")} />
          <AppInput
            name="closing_balance"
            inputMode="decimal"
            label={t("cash.fields.closing_balance", {
              currency: account.currency,
            })}
            description={t("cash.fields.closing_balance_hint")}
            className="sm:col-span-2"
          />
          <AppTextarea
            name="note"
            label={t("cash.fields.note")}
            className="sm:col-span-2"
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
