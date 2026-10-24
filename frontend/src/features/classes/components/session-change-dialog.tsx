"use client";

import { useMemo } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppInput, AppTextarea } from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  MAX_CAPACITY,
  MAX_REASON,
  MIN_CAPACITY,
} from "@/features/classes/constants";
import { useChangeClassSession } from "@/features/classes/hooks/use-classes";
import type { ClassSession } from "@/features/classes/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "class-session-form";
const WHOLE = /^\d+$/;

/** `cancel` takes the session off the timetable; `seats` sets its own seats. */
export type SessionChangeMode = "cancel" | "seats";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

function createSchema(t: Translate, mode: SessionChangeMode, booked: number) {
  return z.object({
    capacity: z
      .string()
      .trim()
      .refine(
        (v) =>
          mode === "cancel" ||
          v === "" ||
          (WHOLE.test(v) &&
            Number(v) >= MIN_CAPACITY &&
            Number(v) <= MAX_CAPACITY),
        t("classes.validation.capacity", { max: MAX_CAPACITY }),
      )
      .refine(
        (v) => mode === "cancel" || v === "" || Number(v) >= booked,
        t("classes.errors.below_booked"),
      ),
    reason: z.string().max(MAX_REASON, t("classes.validation.reason")),
  });
}

type SessionFormValues = z.infer<ReturnType<typeof createSchema>>;

/**
 * Cancels one session or changes its seats. Only that day changes; the
 * class keeps its timetable. A booked session is not cancelled and its
 * seats never go below the seats booked (the API checks both under the
 * booking lock).
 */
export function SessionChangeDialog({
  session,
  mode,
  open,
  onOpenChange,
}: {
  session: ClassSession;
  mode: SessionChangeMode;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const change = useChangeClassSession();
  const schema = useMemo(
    () => createSchema(t, mode, session.booked),
    [mode, session.booked, t],
  );
  const cancel = mode === "cancel";

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        cancel
          ? t("classes.session_dialog.cancel_title")
          : t("classes.session_dialog.seats_title")
      }
      description={
        cancel
          ? t("classes.session_dialog.cancel_description")
          : t("classes.session_dialog.seats_description", {
              booked: session.booked,
            })
      }
      className="sm:max-w-md"
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
            variant={cancel ? "destructive" : "default"}
            disabled={change.isPending}
          >
            {cancel
              ? t("classes.actions.cancel_session")
              : t("classes.actions.save")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<SessionFormValues>
          key={`${session.schedule_uuid}-${session.date}-${mode}`}
          id={FORM_ID}
          className="grid gap-4"
          schema={schema}
          defaultValues={{
            capacity: session.capacity_changed ? String(session.capacity) : "",
            reason: session.reason ?? "",
          }}
          onSubmit={async (values) => {
            const own = session.capacity_changed ? session.capacity : null;
            await change.mutateAsync({
              uuid: session.schedule_uuid,
              date: session.date,
              body: cancel
                ? { cancelled: true, capacity: own, reason: values.reason }
                : {
                    cancelled: session.cancelled,
                    capacity: values.capacity ? Number(values.capacity) : null,
                    reason: values.reason,
                  },
            });
            onOpenChange(false);
          }}
        >
          {!cancel ? (
            <AppInput
              name="capacity"
              inputMode="numeric"
              label={t("classes.fields.session_capacity")}
              description={t("classes.fields.session_capacity_hint")}
            />
          ) : null}
          <AppTextarea
            name="reason"
            label={t("classes.fields.reason")}
            placeholder={t("classes.fields.reason_placeholder")}
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
