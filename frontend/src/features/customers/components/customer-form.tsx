"use client";

import { MessageSquareText, ShieldCheck, UserRound } from "lucide-react";
import { useMemo } from "react";

import {
  AppDatePicker,
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
import {
  createCustomerFormSchema,
  type CustomerFormValues,
} from "@/features/customers/schemas/customer-form";
import { useLocale } from "@/providers/locale-provider";

type CustomerFormProps = {
  mode: "create" | "edit";
  defaultValues: CustomerFormValues;
  /** Shown above the phone when the record is verified (edit). */
  verified?: boolean;
  isSubmitting?: boolean;
  onSubmit: (values: CustomerFormValues) => Promise<void>;
  onCancel: () => void;
};

/** Create / edit form: contact, notes and tags, consent. */
export function CustomerForm({
  mode,
  defaultValues,
  verified,
  isSubmitting,
  onSubmit,
  onCancel,
}: CustomerFormProps) {
  const { t } = useLocale();
  const schema = useMemo(() => createCustomerFormSchema(t), [t]);

  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "contact",
          label: t("customers.form.section_contact"),
          description: t("customers.form.section_contact_desc"),
          icon: UserRound,
        },
        {
          key: "notes",
          label: t("customers.form.section_notes"),
          description: t("customers.form.section_notes_desc"),
          icon: MessageSquareText,
        },
        {
          key: "consent",
          label: t("customers.form.section_consent"),
          description: t("customers.form.section_consent_desc"),
          icon: ShieldCheck,
        },
      ]),
    [t],
  );

  return (
    <AppForm schema={schema} defaultValues={defaultValues} onSubmit={onSubmit}>
      {(form) => (
        <FormLayout navItems={sections.navItems}>
          <FormSection
            id={sections.id("contact")}
            title={t("customers.form.section_contact")}
            description={t("customers.form.section_contact_desc")}
            columns={2}
          >
            <AppInput
              name="first_name"
              label={t("customers.fields.first_name")}
            />
            <AppInput
              name="last_name"
              label={t("customers.fields.last_name")}
            />
            <AppInput
              name="phone"
              type="tel"
              inputMode="tel"
              label={t("customers.fields.phone")}
              description={
                verified
                  ? t("customers.fields.phone_hint_verified")
                  : t("customers.fields.phone_hint")
              }
            />
            <AppInput
              name="email"
              type="email"
              label={t("customers.fields.email")}
            />
            <AppDatePicker
              name="birth_date"
              label={t("customers.fields.birth_date")}
              captionLayout="dropdown"
            />
          </FormSection>

          <FormSection
            id={sections.id("notes")}
            title={t("customers.form.section_notes")}
            description={t("customers.form.section_notes_desc")}
          >
            <AppInput
              name="tags"
              label={t("customers.fields.tags")}
              description={t("customers.fields.tags_hint")}
            />
            <AppTextarea
              name="notes"
              label={t("customers.fields.notes")}
              description={t("customers.fields.notes_hint")}
            />
          </FormSection>

          <FormSection
            id={sections.id("consent")}
            title={t("customers.form.section_consent")}
            description={t("customers.form.section_consent_desc")}
            columns={2}
          >
            <AppSwitch
              name="sms_consent"
              label={t("customers.fields.sms_consent")}
              description={t("customers.fields.sms_consent_hint")}
            />
            <AppSwitch
              name="email_consent"
              label={t("customers.fields.email_consent")}
              description={t("customers.fields.email_consent_hint")}
            />
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
                ? t("customers.actions.create")
                : t("customers.actions.save")}
            </Button>
          </FormActions>
        </FormLayout>
      )}
    </AppForm>
  );
}
