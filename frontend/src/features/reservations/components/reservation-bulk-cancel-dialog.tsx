"use client";

import { useMemo } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppSelect } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { cancelReasonLabel } from "@/features/reservations/lib/labels";
import {
  CANCEL_REASONS,
  createActionSchema,
  type ActionFormValues,
} from "@/features/reservations/schemas/reservation-forms";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "reservation-bulk-cancel";

/**
 * Asks the cancel reason of the bulk `cancel` action (the same reasons as a
 * single cancel). The confirmation follows in the bulk menu.
 */
export function ReservationBulkCancelDialog({
  open,
  count,
  onSubmit,
  onCancel,
}: {
  open: boolean;
  count: number;
  onSubmit: (reason: string) => void;
  onCancel: () => void;
}) {
  const { t } = useLocale();
  const schema = useMemo(() => createActionSchema(t), [t]);
  const reasonOptions = useMemo(
    () =>
      CANCEL_REASONS.map((value) => ({
        value,
        label: cancelReasonLabel(t, value),
      })),
    [t],
  );

  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onCancel();
      }}
      title={t("reservations.bulk_cancel.title", { count })}
      description={t("reservations.bulk_cancel.description")}
      footer={
        <>
          <Button type="button" variant="outline" onClick={onCancel}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID}>
            {t("reservations.bulk_cancel.continue")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<ActionFormValues>
          id={FORM_ID}
          className="grid gap-4"
          schema={schema}
          defaultValues={{ reason: CANCEL_REASONS[0], note: "" }}
          onSubmit={(values) => onSubmit(values.reason)}
        >
          <AppSelect
            name="reason"
            label={t("reservations.fields.cancel_reason")}
            options={reasonOptions}
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
