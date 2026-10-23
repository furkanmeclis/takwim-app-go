"use client";

import { useMemo } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppForm,
  AppInput,
  AppSelect,
  AppSwitch,
  AppTextarea,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { WeeklyHoursEditor } from "@/features/branches/components/weekly-hours-editor";
import {
  SERVICE_POINT_TYPES,
  SERVICE_POINT_TYPE_LABEL_KEYS,
} from "@/features/branches/constants";
import { useSaveServicePoint } from "@/features/branches/hooks/use-branches";
import {
  createServicePointSchema,
  servicePointFormDefaults,
  servicePointFormToRequest,
  type ServicePointFormValues,
} from "@/features/branches/schemas/branch-form";
import type { ServicePoint, WeeklyHours } from "@/features/branches/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "service-point-form";

type ServicePointDialogProps = {
  branchUuid: string;
  branchHours: WeeklyHours;
  open: boolean;
  /** Point to edit; `null` creates a new one. */
  point: ServicePoint | null;
  onOpenChange: (open: boolean) => void;
};

/** Create / edit a service point of a branch. */
export function ServicePointDialog({
  branchUuid,
  branchHours,
  open,
  point,
  onOpenChange,
}: ServicePointDialogProps) {
  const { t } = useLocale();
  const save = useSaveServicePoint(branchUuid);
  const schema = useMemo(() => createServicePointSchema(t), [t]);
  const typeOptions = useMemo(
    () =>
      SERVICE_POINT_TYPES.map((value) => ({
        value,
        label: t(SERVICE_POINT_TYPE_LABEL_KEYS[value]),
      })),
    [t],
  );

  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        point
          ? t("branches.service_points.edit_title")
          : t("branches.service_points.create_title")
      }
      description={t("branches.service_points.dialog_description")}
      className="max-h-[90vh] overflow-y-auto sm:max-w-2xl"
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
            {point
              ? t("branches.actions.save")
              : t("branches.service_points.add")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<ServicePointFormValues>
          key={point?.uuid ?? "new"}
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={servicePointFormDefaults(
            point ?? undefined,
            branchHours,
          )}
          onSubmit={async (values) => {
            await save.mutateAsync({
              pointUuid: point?.uuid,
              body: servicePointFormToRequest(values),
            });
            onOpenChange(false);
          }}
        >
          {(form) => (
            <>
              <AppSelect
                name="type"
                label={t("branches.service_points.fields.type")}
                options={typeOptions}
              />
              <AppInput
                name="name"
                label={t("branches.service_points.fields.name")}
              />
              <AppTextarea
                name="description"
                label={
                  form.watch("type") === "address_zone"
                    ? t("branches.service_points.fields.zone")
                    : t("branches.service_points.fields.address")
                }
                className="sm:col-span-2"
              />
              <AppInput
                name="extra_fee"
                inputMode="decimal"
                label={t("branches.service_points.fields.extra_fee")}
                description={t("branches.service_points.fields.extra_fee_hint")}
              />
              <div className="space-y-3">
                <AppSwitch
                  name="online_selectable"
                  label={t("branches.service_points.fields.online_selectable")}
                />
                <AppSwitch
                  name="is_active"
                  label={t("branches.service_points.fields.is_active")}
                />
              </div>
              <AppSwitch
                name="own_hours"
                className="sm:col-span-2"
                label={t("branches.service_points.fields.own_hours")}
                description={t("branches.service_points.fields.own_hours_hint")}
              />
              {form.watch("own_hours") ? (
                <div className="sm:col-span-2">
                  <WeeklyHoursEditor name="working_hours" />
                </div>
              ) : null}
            </>
          )}
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
