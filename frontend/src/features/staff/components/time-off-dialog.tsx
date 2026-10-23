"use client";

import { useMemo } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppDatePicker,
  AppForm,
  AppSelect,
  AppTextarea,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  TIME_OFF_KINDS,
  TIME_OFF_KIND_LABEL_KEYS,
} from "@/features/staff/constants";
import { useSaveTimeOff } from "@/features/staff/hooks/use-staff";
import {
  createTimeOffSchema,
  timeOffFormDefaults,
  timeOffFormToRequest,
  type TimeOffFormValues,
} from "@/features/staff/schemas/staff-form";
import type { TimeOff } from "@/features/staff/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "staff-time-off-form";

/** Create / edit a time off of a person. */
export function TimeOffDialog({
  staffUuid,
  open,
  timeOff,
  onOpenChange,
}: {
  staffUuid: string;
  open: boolean;
  /** Entry to edit; `null` creates a new one. */
  timeOff: TimeOff | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const save = useSaveTimeOff(staffUuid);
  const schema = useMemo(() => createTimeOffSchema(t), [t]);
  const kindOptions = useMemo(
    () =>
      TIME_OFF_KINDS.map((value) => ({
        value,
        label: t(TIME_OFF_KIND_LABEL_KEYS[value]),
      })),
    [t],
  );

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        timeOff
          ? t("staff.time_off.edit_title")
          : t("staff.time_off.create_title")
      }
      description={t("staff.time_off.dialog_description")}
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
          <Button type="submit" form={FORM_ID} disabled={save.isPending}>
            {timeOff ? t("staff.actions.save") : t("staff.time_off.add")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<TimeOffFormValues>
          key={timeOff?.uuid ?? "new"}
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={timeOffFormDefaults(timeOff ?? undefined)}
          onSubmit={async (values) => {
            await save.mutateAsync({
              timeOffUuid: timeOff?.uuid,
              body: timeOffFormToRequest(values),
            });
            onOpenChange(false);
          }}
        >
          <AppSelect
            name="kind"
            label={t("staff.time_off.fields.kind")}
            options={kindOptions}
            className="sm:col-span-2"
          />
          <AppDatePicker
            name="starts_at"
            withTime
            label={t("staff.time_off.fields.starts_at")}
          />
          <AppDatePicker
            name="ends_at"
            withTime
            label={t("staff.time_off.fields.ends_at")}
          />
          <AppTextarea
            name="note"
            label={t("staff.time_off.fields.note")}
            className="sm:col-span-2"
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
