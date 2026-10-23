"use client";

import { Clock, LayoutGrid, Users } from "lucide-react";
import { useMemo } from "react";

import {
  AppForm,
  AppInput,
  AppSelect,
  AppSwitch,
  AppTextarea,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { WeeklyHoursEditor } from "@/features/branches/components/weekly-hours-editor";
import { MAX_CAPACITY } from "@/features/resources/constants";
import { useResourceKinds } from "@/features/resources/lib/kinds";
import {
  createResourceFormSchema,
  type ResourceFormValues,
} from "@/features/resources/schemas/resource-form";
import { useSector } from "@/features/sectors/hooks/use-sector";
import { useAccessibleBranches } from "@/features/staff/hooks/use-staff";
import { useLocale } from "@/providers/locale-provider";

type ResourceFormProps = {
  mode: "create" | "edit";
  defaultValues: ResourceFormValues;
  isSubmitting?: boolean;
  onSubmit: (values: ResourceFormValues) => Promise<void>;
  onCancel: () => void;
};

/** Create / edit form: identity, capacity, availability (own hours). */
export function ResourceForm({
  mode,
  defaultValues,
  isSubmitting,
  onSubmit,
  onCancel,
}: ResourceFormProps) {
  const { t } = useLocale();
  const { sector } = useSector();
  const schema = useMemo(() => createResourceFormSchema(t), [t]);
  const branches = useAccessibleBranches();
  const kinds = useResourceKinds();

  const branchOptions = useMemo(
    () =>
      (branches.data?.items ?? []).map((branch) => ({
        value: branch.uuid,
        label: `${branch.name} (${branch.code})`,
      })),
    [branches.data?.items],
  );

  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "identity",
          label: t("resources.form.section_identity"),
          description: t("resources.form.section_identity_desc"),
          icon: LayoutGrid,
        },
        {
          key: "capacity",
          label: t("resources.form.section_capacity"),
          description: t("resources.form.section_capacity_desc"),
          icon: Users,
        },
        {
          key: "hours",
          label: t("resources.form.section_hours"),
          description: t("resources.form.section_hours_desc"),
          icon: Clock,
        },
      ]),
    [t],
  );

  return (
    <AppForm schema={schema} defaultValues={defaultValues} onSubmit={onSubmit}>
      {(form) => {
        const kind = form.watch("kind");
        return (
          <FormLayout navItems={sections.navItems}>
            <FormSection
              id={sections.id("identity")}
              title={t("resources.form.section_identity")}
              description={t("resources.form.section_identity_desc")}
              columns={2}
            >
              <AppSelect
                name="branch_uuid"
                label={t("resources.fields.branch")}
                placeholder={t("resources.fields.branch_placeholder")}
                options={branchOptions}
              />
              <AppSelect
                name="kind"
                label={t("resources.fields.kind")}
                description={t("resources.fields.kind_hint")}
                options={kinds.options}
              />
              <AppInput
                name="name"
                label={t("resources.fields.name")}
                description={t("resources.fields.name_hint")}
              />
              <AppInput
                name="zone"
                label={t("resources.fields.zone")}
                description={t(
                  sector?.capacity.party_size
                    ? "resources.fields.zone_hint_restaurant"
                    : "resources.fields.zone_hint",
                )}
              />
              <AppSwitch
                name="show_online"
                label={t("resources.fields.show_online")}
                description={t("resources.fields.show_online_hint")}
              />
              <AppSwitch
                name="is_active"
                label={t("resources.fields.is_active")}
                description={t("resources.fields.is_active_hint")}
              />
              <AppTextarea
                name="description"
                label={t("resources.fields.description")}
                className="sm:col-span-2"
              />
            </FormSection>

            <FormSection
              id={sections.id("capacity")}
              title={t("resources.form.section_capacity")}
              description={t("resources.form.section_capacity_desc")}
              columns={2}
            >
              <AppInput
                name="min_capacity"
                type="number"
                min={1}
                max={MAX_CAPACITY}
                label={t("resources.fields.min_capacity")}
                description={t("resources.fields.min_capacity_hint")}
              />
              <AppInput
                name="capacity"
                type="number"
                min={1}
                max={MAX_CAPACITY}
                label={t("resources.fields.capacity")}
                description={t("resources.fields.capacity_hint")}
              />
              {kind === "table" ? (
                <AppSwitch
                  name="combinable"
                  label={t("resources.fields.combinable")}
                  description={t("resources.fields.combinable_hint")}
                />
              ) : null}
              <AppInput
                name="sort_order"
                type="number"
                min={0}
                label={t("resources.fields.sort_order")}
                description={t("resources.fields.sort_order_hint")}
              />
            </FormSection>

            <FormSection
              id={sections.id("hours")}
              title={t("resources.form.section_hours")}
              description={t("resources.form.section_hours_desc")}
            >
              <AppSwitch
                name="own_hours"
                label={t("resources.fields.own_hours")}
                description={t("resources.fields.own_hours_hint")}
              />
              {form.watch("own_hours") ? (
                <WeeklyHoursEditor name="working_hours" />
              ) : null}
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
                  ? t("resources.actions.create")
                  : t("resources.actions.save")}
              </Button>
            </FormActions>
          </FormLayout>
        );
      }}
    </AppForm>
  );
}
