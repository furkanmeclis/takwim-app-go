"use client";

import { Clock, Layers, Store, Wallet } from "lucide-react";
import { useMemo } from "react";

import {
  AppForm,
  AppInput,
  AppMultiCombobox,
  AppSelect,
  AppSwitch,
  AppTextarea,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { useResourceKinds } from "@/features/resources/lib/kinds";
import {
  MAX_BUFFER,
  MAX_DURATION,
  MIN_DURATION,
  PAYMENT_MODE_LABEL_KEYS,
  PAYMENT_MODES,
  STAFF_MODE_LABEL_KEYS,
  STAFF_MODES,
  UNCATEGORIZED,
} from "@/features/services/constants";
import { useServiceCategories } from "@/features/services/hooks/use-services";
import {
  createServiceFormSchema,
  type ServiceFormValues,
} from "@/features/services/schemas/service-form";
import { useAccessibleBranches } from "@/features/staff/hooks/use-staff";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

type ServiceFormProps = {
  mode: "create" | "edit";
  defaultValues: ServiceFormValues;
  isSubmitting?: boolean;
  onSubmit: (values: ServiceFormValues) => Promise<void>;
  onCancel: () => void;
};

/**
 * Create / edit form: identity, time (duration and buffers), price and
 * payment, what a booking holds (staff, resource kinds). On create the
 * branches offering the service are picked too; later they change on the
 * detail page (branch offers).
 */
export function ServiceForm({
  mode,
  defaultValues,
  isSubmitting,
  onSubmit,
  onCancel,
}: ServiceFormProps) {
  const { t } = useLocale();
  const { organization } = useTenant();
  const schema = useMemo(() => createServiceFormSchema(t), [t]);
  const categories = useServiceCategories();
  const branches = useAccessibleBranches();
  const kinds = useResourceKinds();

  const categoryOptions = useMemo(
    () => [
      { value: UNCATEGORIZED, label: t("services.uncategorized") },
      ...(categories.data?.items ?? []).map((category) => ({
        value: category.uuid,
        label: category.name,
      })),
    ],
    [categories.data?.items, t],
  );
  const branchOptions = useMemo(
    () =>
      (branches.data?.items ?? []).map((branch) => ({
        value: branch.uuid,
        label: `${branch.name} (${branch.code})`,
      })),
    [branches.data?.items],
  );
  const paymentOptions = PAYMENT_MODES.map((mode) => ({
    value: mode,
    label: t(PAYMENT_MODE_LABEL_KEYS[mode]),
  }));
  const staffOptions = STAFF_MODES.map((mode) => ({
    value: mode,
    label: t(STAFF_MODE_LABEL_KEYS[mode]),
  }));

  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "identity",
          label: t("services.form.section_identity"),
          description: t("services.form.section_identity_desc"),
          icon: Layers,
        },
        {
          key: "time",
          label: t("services.form.section_time"),
          description: t("services.form.section_time_desc"),
          icon: Clock,
        },
        {
          key: "price",
          label: t("services.form.section_price"),
          description: t("services.form.section_price_desc"),
          icon: Wallet,
        },
        {
          key: "booking",
          label: t("services.form.section_booking"),
          description: t("services.form.section_booking_desc"),
          icon: Store,
        },
      ]),
    [t],
  );

  return (
    <AppForm schema={schema} defaultValues={defaultValues} onSubmit={onSubmit}>
      {(form) => (
        <FormLayout navItems={sections.navItems}>
          <FormSection
            id={sections.id("identity")}
            title={t("services.form.section_identity")}
            description={t("services.form.section_identity_desc")}
            columns={2}
          >
            <AppInput
              name="name"
              label={t("services.fields.name")}
              description={t("services.fields.name_hint")}
            />
            <AppSelect
              name="category_uuid"
              label={t("services.fields.category")}
              description={t("services.fields.category_hint")}
              options={categoryOptions}
            />
            <AppSwitch
              name="show_online"
              label={t("services.fields.show_online")}
              description={t("services.fields.show_online_hint")}
            />
            <AppSwitch
              name="is_active"
              label={t("services.fields.is_active")}
              description={t("services.fields.is_active_hint")}
            />
            <AppInput
              name="sort_order"
              type="number"
              min={0}
              label={t("services.fields.sort_order")}
              description={t("services.fields.sort_order_hint")}
            />
            <AppTextarea
              name="description"
              label={t("services.fields.description")}
              description={t("services.fields.description_hint")}
              className="sm:col-span-2"
            />
          </FormSection>

          <FormSection
            id={sections.id("time")}
            title={t("services.form.section_time")}
            description={t("services.form.section_time_desc")}
            columns={2}
          >
            <AppInput
              name="duration_minutes"
              type="number"
              min={MIN_DURATION}
              max={MAX_DURATION}
              label={t("services.fields.duration")}
              description={t("services.fields.duration_hint")}
            />
            <AppInput
              name="buffer_before_minutes"
              type="number"
              min={0}
              max={MAX_BUFFER}
              label={t("services.fields.buffer_before")}
              description={t("services.fields.buffer_before_hint")}
            />
            <AppInput
              name="buffer_after_minutes"
              type="number"
              min={0}
              max={MAX_BUFFER}
              label={t("services.fields.buffer_after")}
              description={t("services.fields.buffer_after_hint")}
            />
          </FormSection>

          <FormSection
            id={sections.id("price")}
            title={t("services.form.section_price")}
            description={t("services.form.section_price_desc")}
            columns={2}
          >
            <AppInput
              name="price"
              inputMode="decimal"
              label={t("services.fields.price", {
                currency: organization?.currency ?? "TRY",
              })}
              description={t("services.fields.price_hint")}
            />
            <AppSelect
              name="payment_mode"
              label={t("services.fields.payment_mode")}
              description={t("services.fields.payment_mode_hint")}
              options={paymentOptions}
            />
          </FormSection>

          <FormSection
            id={sections.id("booking")}
            title={t("services.form.section_booking")}
            description={t("services.form.section_booking_desc")}
            columns={2}
          >
            <AppSelect
              name="staff_mode"
              label={t("services.fields.staff_mode")}
              description={t("services.fields.staff_mode_hint")}
              options={staffOptions}
            />
            <AppMultiCombobox
              name="resource_kinds"
              label={t("services.fields.resource_kinds")}
              description={t("services.fields.resource_kinds_hint")}
              options={kinds.options}
              placeholder={t("services.fields.resource_kinds_placeholder")}
              emptyText={t("services.fields.resource_kinds_empty")}
            />
            {mode === "create" ? (
              <AppMultiCombobox
                name="branch_uuids"
                label={t("services.fields.branches")}
                description={t("services.fields.branches_hint")}
                options={branchOptions}
                placeholder={t("services.fields.branches_placeholder")}
                emptyText={t("services.fields.branches_empty")}
                className="sm:col-span-2"
              />
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
                ? t("services.actions.create")
                : t("services.actions.save")}
            </Button>
          </FormActions>
        </FormLayout>
      )}
    </AppForm>
  );
}
