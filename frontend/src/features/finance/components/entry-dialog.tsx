"use client";

import { Paperclip, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useFormContext, useWatch } from "react-hook-form";

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
import { Input } from "@/components/ui/input";
import { useBusinessToday } from "@/features/cash/lib/format";
import {
  DOCUMENT_ACCEPT,
  FINANCE_KINDS,
  MAX_DOCUMENT_BYTES,
} from "@/features/finance/constants";
import { useCreateFinanceEntry } from "@/features/finance/hooks/use-finance";
import { useFinanceFormOptions } from "@/features/finance/hooks/use-finance-options";
import { kindLabel } from "@/features/finance/lib/format";
import {
  AUTO,
  NONE,
  createEntryFormSchema,
  entryFormToRequest,
  type EntryFormValues,
} from "@/features/finance/schemas/finance-forms";
import { financeService } from "@/features/finance/services/finance.service";
import type { FinanceKind } from "@/features/finance/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { fileSize } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const FORM_ID = "finance-entry-form";

/** Category / account / branch / staff pickers; the category list follows the kind. */
function EntryFields({ open }: { open: boolean }) {
  const { t } = useLocale();
  const { setValue } = useFormContext<EntryFormValues>();
  const kind = useWatch<EntryFormValues, "kind">({ name: "kind" });
  const options = useFinanceFormOptions({ kind, enabled: open });
  const previousKind = useRef(kind);

  useEffect(() => {
    if (previousKind.current === kind) return;
    previousKind.current = kind;
    setValue("category_uuid", "");
  }, [kind, setValue]);

  const kindOptions = FINANCE_KINDS.map((value) => ({
    value,
    label: kindLabel(t, value),
  }));

  return (
    <>
      <div className="sm:col-span-2">
        <AppSelect
          name="kind"
          label={t("finance.fields.kind")}
          options={kindOptions}
        />
      </div>
      <AppSelect
        name="category_uuid"
        label={t("finance.fields.category")}
        placeholder={t("finance.fields.category_placeholder")}
        options={options.categoryOptions}
        className="sm:col-span-2"
      />
      <AppSelect
        name="cash_account_uuid"
        label={t(
          kind === "income"
            ? "finance.fields.account_income"
            : "finance.fields.account_expense",
        )}
        placeholder={t("finance.fields.account_placeholder")}
        description={
          options.accountsLoaded && options.accountOptions.length === 0
            ? t("finance.fields.account_none")
            : t("finance.fields.account_hint")
        }
        options={options.accountOptions}
        className="sm:col-span-2"
      />
      <AppSelect
        name="branch_uuid"
        label={t("finance.fields.branch")}
        options={options.branchOptions}
      />
      <AppCombobox
        name="staff_uuid"
        label={t("finance.fields.staff")}
        placeholder={t("finance.fields.staff_none")}
        options={options.staffOptions}
      />
      <AppInput
        name="amount"
        inputMode="decimal"
        label={t("finance.fields.amount")}
      />
      <AppDatePicker name="entry_date" label={t("finance.fields.entry_date")} />
      <AppTextarea
        name="description"
        label={t("finance.fields.description")}
        className="sm:col-span-2"
      />
    </>
  );
}

/**
 * Records money in or out outside sales. The amount leaves (or enters) the
 * picked account in the same step; a document can be attached right away.
 * The idempotency key makes a double submit write once.
 */
export function EntryDialog({
  open,
  kind,
  onOpenChange,
}: {
  open: boolean;
  kind: FinanceKind;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const today = useBusinessToday();
  const create = useCreateFinanceEntry();
  const schema = useMemo(() => createEntryFormSchema(t), [t]);
  const fileInput = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [idempotencyKey, setIdempotencyKey] = useState(() =>
    crypto.randomUUID(),
  );

  const close = (next: boolean) => {
    if (!next) {
      setIdempotencyKey(crypto.randomUUID());
      setFile(null);
    }
    onOpenChange(next);
  };

  return (
    <AppDialog
      open={open}
      onOpenChange={close}
      title={t(
        kind === "income"
          ? "finance.entry_dialog.title_income"
          : "finance.entry_dialog.title_expense",
      )}
      description={t("finance.entry_dialog.description")}
      className="sm:max-w-xl"
      footer={
        <>
          <Button type="button" variant="outline" onClick={() => close(false)}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={create.isPending}>
            {t("finance.entry_dialog.submit")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<EntryFormValues>
          key={kind}
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={{
            kind,
            category_uuid: "",
            cash_account_uuid: "",
            branch_uuid: AUTO,
            staff_uuid: NONE,
            amount: "",
            entry_date: today,
            description: "",
          }}
          onSubmit={async (values) => {
            const entry = await create.mutateAsync(
              entryFormToRequest(values, idempotencyKey),
            );
            if (file) {
              try {
                await financeService.uploadDocument(slug, entry.uuid, file);
              } catch {
                appToast.error(
                  t("finance.document.upload_failed_after_create"),
                );
              }
            }
            close(false);
          }}
        >
          <EntryFields open={open} />
          <div className="space-y-2 sm:col-span-2">
            <p className="text-sm font-medium">
              {t("finance.fields.document")}
            </p>
            <Input
              ref={fileInput}
              type="file"
              accept={DOCUMENT_ACCEPT}
              className="hidden"
              aria-label={t("finance.document.attach")}
              onChange={(event) => {
                const picked = event.target.files?.[0];
                event.target.value = "";
                if (!picked) return;
                if (picked.size > MAX_DOCUMENT_BYTES) {
                  appToast.error(t("finance.document.too_large"));
                  return;
                }
                setFile(picked);
              }}
            />
            {file ? (
              <div className="flex items-center gap-2 rounded-md border px-3 py-2 text-sm">
                <Paperclip className="text-muted-foreground size-4 shrink-0" />
                <span className="min-w-0 flex-1 truncate">{file.name}</span>
                <span className="text-muted-foreground text-xs">
                  {fileSize(file.size)}
                </span>
                <Button
                  type="button"
                  size="icon"
                  variant="ghost"
                  aria-label={t("finance.document.remove")}
                  onClick={() => setFile(null)}
                >
                  <X className="size-4" />
                </Button>
              </div>
            ) : (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => fileInput.current?.click()}
              >
                <Paperclip className="size-4" />
                {t("finance.document.attach")}
              </Button>
            )}
            <p className="text-muted-foreground text-xs">
              {t("finance.document.hint")}
            </p>
          </div>
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
