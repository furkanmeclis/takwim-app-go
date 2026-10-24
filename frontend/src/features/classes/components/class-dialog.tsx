"use client";

import { useEffect, useMemo } from "react";
import { useFormContext, useWatch } from "react-hook-form";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppCombobox,
  AppDatePicker,
  AppForm,
  AppInput,
  AppSelect,
  AppSwitch,
  AppTextarea,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { useBusinessToday } from "@/features/cash/lib/format";
import {
  MAX_CAPACITY,
  MAX_NOTE,
  MIN_CAPACITY,
  WEEKDAY_LABEL_KEYS,
  WEEKDAYS,
} from "@/features/classes/constants";
import {
  useClassForm,
  useSaveClass,
} from "@/features/classes/hooks/use-classes";
import type {
  ClassFormOptions,
  ClassSchedule,
  ClassScheduleInput,
  Weekday,
} from "@/features/classes/types";
import type { ResourceKind } from "@/features/resources/types";
import { SECTOR_RESOURCE_LABEL_KEYS } from "@/features/sectors/constants";
import { useAccessibleBranches } from "@/features/staff/hooks/use-staff";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "class-schedule-form";
/** Picks nobody: the first booking of a session chooses (TEC-904). */
const AUTO = "auto";
const CLOCK = /^([01]\d|2[0-3]):[0-5]\d$/;
const WHOLE = /^\d+$/;

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

function createClassSchema(t: Translate) {
  return z
    .object({
      service_uuid: z.string().min(1, t("classes.validation.service")),
      branch_uuid: z.string().min(1, t("classes.validation.branch")),
      weekday: z.enum(WEEKDAYS as [Weekday, ...Weekday[]]),
      start_time: z
        .string()
        .refine((v) => CLOCK.test(v), t("classes.validation.start_time")),
      staff_uuid: z.string(),
      resource_uuid: z.string(),
      capacity: z
        .string()
        .trim()
        .refine(
          (v) =>
            v === "" ||
            (WHOLE.test(v) &&
              Number(v) >= MIN_CAPACITY &&
              Number(v) <= MAX_CAPACITY),
          t("classes.validation.capacity", { max: MAX_CAPACITY }),
        ),
      valid_from: z.string().min(1, t("classes.validation.valid_from")),
      valid_until: z.string(),
      is_active: z.boolean(),
      note: z.string().max(MAX_NOTE, t("classes.validation.note")),
    })
    .superRefine((values, ctx) => {
      if (values.valid_until && values.valid_until < values.valid_from) {
        ctx.addIssue({
          code: "custom",
          path: ["valid_until"],
          message: t("classes.errors.before_from"),
        });
      }
    });
}

type ClassFormValues = z.infer<ReturnType<typeof createClassSchema>>;

function ClassFields({ options }: { options?: ClassFormOptions }) {
  const { t } = useLocale();
  const { control, getValues, setValue } = useFormContext<ClassFormValues>();
  const serviceUuid = useWatch({ control, name: "service_uuid" });
  const branchUuid = useWatch({ control, name: "branch_uuid" });
  const branches = useAccessibleBranches();
  const service = options?.services.find((s) => s.uuid === serviceUuid);

  const serviceOptions = useMemo(
    () =>
      (options?.services ?? []).map((s) => ({
        value: s.uuid,
        label: s.is_active ? s.name : `${s.name} (${t("classes.inactive")})`,
      })),
    [options?.services, t],
  );
  const branchOptions = useMemo(
    () =>
      (branches.data?.items ?? [])
        .filter((b) => !service || service.branch_uuids.includes(b.uuid))
        .map((b) => ({ value: b.uuid, label: b.name })),
    [branches.data?.items, service],
  );
  const staffOptions = useMemo(
    () => [
      { value: AUTO, label: t("classes.fields.staff_auto") },
      ...(service?.trainers ?? [])
        .filter((p) => !branchUuid || p.branch_uuids.includes(branchUuid))
        .map((p) => ({
          value: p.uuid,
          label: p.is_active ? p.name : `${p.name} (${t("classes.inactive")})`,
        })),
    ],
    [branchUuid, service?.trainers, t],
  );
  const resourceOptions = useMemo(
    () => [
      { value: AUTO, label: t("classes.fields.resource_auto") },
      ...(options?.resources ?? [])
        .filter(
          (r) =>
            r.branch_uuid === branchUuid &&
            (service?.resource_kinds ?? []).includes(r.kind),
        )
        .map((r) => ({
          value: r.uuid,
          label: `${r.name} · ${t(
            SECTOR_RESOURCE_LABEL_KEYS[r.kind as ResourceKind] ?? r.kind,
          )}`,
        })),
    ],
    [branchUuid, options?.resources, service?.resource_kinds, t],
  );

  // A new service or branch drops choices that no longer fit it.
  useEffect(() => {
    if (!options || !service) return;
    const values = getValues();
    if (
      values.staff_uuid !== AUTO &&
      !staffOptions.some((o) => o.value === values.staff_uuid)
    ) {
      setValue("staff_uuid", AUTO);
    }
    if (
      values.resource_uuid !== AUTO &&
      !resourceOptions.some((o) => o.value === values.resource_uuid)
    ) {
      setValue("resource_uuid", AUTO);
    }
  }, [getValues, options, resourceOptions, service, setValue, staffOptions]);

  const usesStaff = service ? service.staff_mode !== "none" : true;
  const usesResource = service ? service.resource_kinds.length > 0 : false;

  return (
    <>
      <AppCombobox
        name="service_uuid"
        label={t("classes.fields.service")}
        description={t("classes.fields.service_hint")}
        placeholder={t("classes.fields.service_placeholder")}
        searchPlaceholder={t("classes.fields.service_search")}
        emptyText={t("classes.fields.service_empty")}
        options={serviceOptions}
        className="sm:col-span-2"
      />
      <AppCombobox
        name="branch_uuid"
        label={t("classes.fields.branch")}
        placeholder={t("classes.fields.branch_placeholder")}
        searchPlaceholder={t("classes.fields.branch_search")}
        emptyText={t("classes.fields.branch_empty")}
        options={branchOptions}
        className="sm:col-span-2"
      />
      <AppSelect
        name="weekday"
        label={t("classes.fields.weekday")}
        options={WEEKDAYS.map((day) => ({
          value: day,
          label: t(WEEKDAY_LABEL_KEYS[day]),
        }))}
      />
      <AppInput
        name="start_time"
        type="time"
        step={60}
        label={t("classes.fields.start_time")}
        description={
          service
            ? t("classes.fields.start_time_hint", {
                minutes: service.duration_minutes,
              })
            : undefined
        }
      />
      {usesStaff ? (
        <AppCombobox
          name="staff_uuid"
          label={t("classes.fields.staff")}
          description={t("classes.fields.staff_hint")}
          searchPlaceholder={t("classes.fields.staff_search")}
          emptyText={t("classes.fields.staff_empty")}
          options={staffOptions}
          className="sm:col-span-2"
        />
      ) : null}
      {usesResource ? (
        <AppCombobox
          name="resource_uuid"
          label={t("classes.fields.resource")}
          description={t("classes.fields.resource_hint")}
          searchPlaceholder={t("classes.fields.resource_search")}
          emptyText={t("classes.fields.resource_empty")}
          options={resourceOptions}
          className="sm:col-span-2"
        />
      ) : null}
      <AppInput
        name="capacity"
        inputMode="numeric"
        label={t("classes.fields.capacity")}
        description={
          service
            ? t("classes.fields.capacity_hint", {
                seats: service.group_capacity,
              })
            : t("classes.fields.capacity_hint_empty")
        }
      />
      <AppSwitch
        name="is_active"
        label={t("classes.fields.is_active")}
        description={t("classes.fields.is_active_hint")}
      />
      <AppDatePicker name="valid_from" label={t("classes.fields.valid_from")} />
      <AppDatePicker
        name="valid_until"
        label={t("classes.fields.valid_until")}
        description={t("classes.fields.valid_until_hint")}
        placeholder={t("classes.fields.valid_until_placeholder")}
      />
      <AppTextarea
        name="note"
        label={t("classes.fields.note")}
        className="sm:col-span-2"
      />
    </>
  );
}

function toRequest(values: ClassFormValues): ClassScheduleInput {
  return {
    service_uuid: values.service_uuid,
    branch_uuid: values.branch_uuid,
    weekday: values.weekday,
    start_time: values.start_time,
    staff_uuid: values.staff_uuid === AUTO ? null : values.staff_uuid,
    resource_uuid: values.resource_uuid === AUTO ? null : values.resource_uuid,
    capacity: values.capacity ? Number(values.capacity) : null,
    valid_from: values.valid_from,
    valid_until: values.valid_until || null,
    is_active: values.is_active,
    note: values.note.trim(),
  };
}

/**
 * Add, change or copy a class of the timetable: a group service on one
 * weekday at one time in a branch, with its trainer, area and seats.
 * With `copy` the dialog starts from another class and creates a new one
 * (e.g. the same class on another day).
 */
export function ClassDialog({
  schedule,
  copy = false,
  open,
  onOpenChange,
}: {
  /** Class to edit (or to copy); `null` starts an empty class. */
  schedule: ClassSchedule | null;
  copy?: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const today = useBusinessToday();
  const save = useSaveClass();
  const form = useClassForm(open);
  const schema = useMemo(() => createClassSchema(t), [t]);
  const editing = Boolean(schedule) && !copy;

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        editing
          ? t("classes.dialog.edit_title")
          : copy
            ? t("classes.dialog.copy_title")
            : t("classes.dialog.create_title")
      }
      description={t("classes.dialog.description")}
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
            form={FORM_ID}
            disabled={save.isPending || form.isLoading}
          >
            {editing ? t("classes.actions.save") : t("classes.actions.create")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<ClassFormValues>
          key={`${schedule?.uuid ?? "new"}-${copy ? "copy" : "edit"}`}
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={{
            service_uuid: schedule?.service.uuid ?? "",
            branch_uuid: schedule?.branch.uuid ?? "",
            weekday: schedule?.weekday ?? "mon",
            start_time: schedule?.start_time ?? "",
            staff_uuid: schedule?.staff?.uuid ?? AUTO,
            resource_uuid: schedule?.resource?.uuid ?? AUTO,
            capacity:
              schedule?.capacity != null ? String(schedule.capacity) : "",
            valid_from: editing ? (schedule?.valid_from ?? today) : today,
            valid_until: schedule?.valid_until ?? "",
            is_active: schedule?.is_active ?? true,
            note: schedule?.note ?? "",
          }}
          onSubmit={async (values) => {
            await save.mutateAsync({
              uuid: editing ? schedule?.uuid : undefined,
              body: toRequest(values),
            });
            onOpenChange(false);
          }}
        >
          <ClassFields options={form.data} />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
