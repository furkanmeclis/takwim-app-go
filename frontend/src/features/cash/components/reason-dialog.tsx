"use client";

import { useMemo } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppTextarea } from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  createReasonFormSchema,
  type ReasonFormValues,
} from "@/features/cash/schemas/cash-forms";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "cash-reason-form";

/**
 * Confirms a correction that needs a written reason: reversing a movement
 * or reopening a closed day. The reason stays on the record.
 */
export function ReasonDialog({
  open,
  title,
  description,
  confirmLabel,
  isPending,
  onOpenChange,
  onConfirm,
}: {
  open: boolean;
  title: string;
  description: string;
  confirmLabel: string;
  isPending?: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (reason: string) => Promise<unknown>;
}) {
  const { t } = useLocale();
  const schema = useMemo(() => createReasonFormSchema(t), [t]);

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={title}
      description={description}
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
            form={FORM_ID}
            variant="destructive"
            disabled={isPending}
          >
            {confirmLabel}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<ReasonFormValues>
          id={FORM_ID}
          schema={schema}
          defaultValues={{ reason: "" }}
          onSubmit={async (values) => {
            await onConfirm(values.reason.trim());
            onOpenChange(false);
          }}
        >
          <AppTextarea name="reason" label={t("cash.fields.reason")} />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
