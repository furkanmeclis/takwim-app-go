"use client";

import { CalendarOff, Clock, MapPin, Store } from "lucide-react";
import { useMemo } from "react";

import {
  AppForm,
  AppInput,
  AppSwitch,
  AppTextarea,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { HolidayExceptionsEditor } from "@/features/branches/components/holiday-exceptions-editor";
import { WeeklyHoursEditor } from "@/features/branches/components/weekly-hours-editor";
import {
  createBranchFormSchema,
  type BranchFormValues,
} from "@/features/branches/schemas/branch-form";
import { useLocale } from "@/providers/locale-provider";

type BranchFormProps = {
  mode: "create" | "edit";
  defaultValues: BranchFormValues;
  isSubmitting?: boolean;
  onSubmit: (values: BranchFormValues) => Promise<void>;
  onCancel: () => void;
};

/** Create / edit form: details, contact & location, weekly hours, holidays. */
export function BranchForm({
  mode,
  defaultValues,
  isSubmitting,
  onSubmit,
  onCancel,
}: BranchFormProps) {
  const { t } = useLocale();
  const schema = useMemo(() => createBranchFormSchema(t), [t]);
  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "details",
          label: t("branches.form.section_details"),
          description: t("branches.form.section_details_desc"),
          icon: Store,
        },
        {
          key: "location",
          label: t("branches.form.section_location"),
          description: t("branches.form.section_location_desc"),
          icon: MapPin,
        },
        {
          key: "hours",
          label: t("branches.form.section_hours"),
          description: t("branches.form.section_hours_desc"),
          icon: Clock,
        },
        {
          key: "holidays",
          label: t("branches.form.section_holidays"),
          description: t("branches.form.section_holidays_desc"),
          icon: CalendarOff,
        },
      ]),
    [t],
  );

  return (
    <AppForm schema={schema} defaultValues={defaultValues} onSubmit={onSubmit}>
      {(form) => (
        <FormLayout navItems={sections.navItems}>
          <FormSection
            id={sections.id("details")}
            title={t("branches.form.section_details")}
            description={t("branches.form.section_details_desc")}
            columns={2}
          >
            <AppInput
              name="code"
              label={t("branches.fields.code")}
              description={t("branches.fields.code_hint")}
              autoCapitalize="characters"
            />
            <AppInput name="name" label={t("branches.fields.name")} />
            <AppInput
              name="prep_minutes"
              type="number"
              min={0}
              max={10080}
              label={t("branches.fields.prep_minutes")}
              description={t("branches.fields.prep_minutes_hint")}
            />
            <AppSwitch
              name="is_active"
              label={t("branches.fields.is_active")}
              description={t("branches.fields.is_active_hint")}
            />
          </FormSection>

          <FormSection
            id={sections.id("location")}
            title={t("branches.form.section_location")}
            description={t("branches.form.section_location_desc")}
            columns={2}
          >
            <AppTextarea
              name="address"
              label={t("branches.fields.address")}
              className="sm:col-span-2"
            />
            <AppInput name="city" label={t("branches.fields.city")} />
            <AppInput name="district" label={t("branches.fields.district")} />
            <AppInput
              name="phone"
              type="tel"
              label={t("branches.fields.phone")}
            />
            <AppInput
              name="email"
              type="email"
              label={t("branches.fields.email")}
            />
            <AppInput
              name="latitude"
              inputMode="decimal"
              label={t("branches.fields.latitude")}
              placeholder="41.008240"
            />
            <AppInput
              name="longitude"
              inputMode="decimal"
              label={t("branches.fields.longitude")}
              placeholder="28.978359"
            />
          </FormSection>

          <FormSection
            id={sections.id("hours")}
            title={t("branches.form.section_hours")}
            description={t("branches.form.section_hours_desc")}
          >
            <WeeklyHoursEditor name="working_hours" />
          </FormSection>

          <FormSection
            id={sections.id("holidays")}
            title={t("branches.form.section_holidays")}
            description={t("branches.form.section_holidays_desc")}
          >
            <HolidayExceptionsEditor />
          </FormSection>

          <FormActions>
            <Button type="button" variant="outline" onClick={onCancel}>
              {t("form.cancel")}
            </Button>
            <Button
              type="submit"
              disabled={isSubmitting || form.formState.isSubmitting}
            >
              {mode === "create"
                ? t("branches.actions.create")
                : t("branches.actions.save")}
            </Button>
          </FormActions>
        </FormLayout>
      )}
    </AppForm>
  );
}
