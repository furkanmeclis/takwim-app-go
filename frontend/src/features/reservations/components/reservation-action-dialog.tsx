"use client";

import { useMemo } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppSelect, AppTextarea } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { ACTION_LABEL_KEYS } from "@/features/reservations/constants";
import { useReservationAction } from "@/features/reservations/hooks/use-reservations";
import { cancelReasonLabel } from "@/features/reservations/lib/labels";
import {
  CANCEL_REASONS,
  createActionSchema,
  type ActionFormValues,
} from "@/features/reservations/schemas/reservation-forms";
import type { ReservationAction } from "@/features/reservations/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "reservation-action-form";

const DESCRIPTION_KEYS: Record<ReservationAction, string> = {
  accept: "reservations.action_dialog.accept",
  reject: "reservations.action_dialog.reject",
  cancel: "reservations.action_dialog.cancel",
  start: "reservations.action_dialog.start",
  complete: "reservations.action_dialog.complete",
  "no-show": "reservations.action_dialog.no_show",
};

export type ActionTarget = {
  uuid: string;
  number: number;
  version: number;
  action: ReservationAction;
};

/**
 * Confirms a status change with an optional note (cancel asks the reason).
 * The version read with the record goes along, so a change made meanwhile
 * by someone else is refused instead of overwritten.
 */
export function ReservationActionDialog({
  target,
  onOpenChange,
  onDone,
}: {
  target: ActionTarget | null;
  onOpenChange: (open: boolean) => void;
  onDone?: () => void;
}) {
  const { t } = useLocale();
  const run = useReservationAction();
  const schema = useMemo(() => createActionSchema(t), [t]);
  const reasonOptions = useMemo(
    () =>
      CANCEL_REASONS.map((value) => ({
        value,
        label: cancelReasonLabel(t, value),
      })),
    [t],
  );
  const action = target?.action;
  const destructive =
    action === "cancel" || action === "reject" || action === "no-show";

  return (
    <AppDialog
      open={Boolean(target)}
      onOpenChange={onOpenChange}
      title={
        action
          ? `${t(ACTION_LABEL_KEYS[action])} · #${target?.number ?? ""}`
          : ""
      }
      description={action ? t(DESCRIPTION_KEYS[action]) : undefined}
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
            variant={destructive ? "destructive" : "default"}
            disabled={run.isPending}
          >
            {action ? t(ACTION_LABEL_KEYS[action]) : ""}
          </Button>
        </>
      }
    >
      {target ? (
        <AppForm<ActionFormValues>
          key={`${target.uuid}-${target.action}`}
          id={FORM_ID}
          className="grid gap-4"
          schema={schema}
          defaultValues={{
            reason: action === "cancel" ? CANCEL_REASONS[0] : "",
            note: "",
          }}
          onSubmit={async (values) => {
            await run.mutateAsync({
              uuid: target.uuid,
              action: target.action,
              body: {
                expected_version: target.version,
                reason: action === "cancel" ? values.reason : undefined,
                note: values.note || undefined,
              },
            });
            onOpenChange(false);
            onDone?.();
          }}
        >
          {action === "cancel" ? (
            <AppSelect
              name="reason"
              label={t("reservations.fields.cancel_reason")}
              options={reasonOptions}
            />
          ) : null}
          <AppTextarea
            name="note"
            label={t("reservations.fields.note")}
            description={t("reservations.fields.note_hint")}
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
