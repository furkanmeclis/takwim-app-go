"use client";

import { useMemo } from "react";

import {
  AppCheckbox,
  AppForm,
  AppInput,
  AppTextarea,
} from "@/components/forms";
import { Input } from "@/components/ui/input";
import {
  contactDefaults,
  createContactSchema,
} from "@/features/business-page/schemas/contact-form";
import type {
  ContactValues,
  PublicBookingField,
} from "@/features/business-page/types";
import { CustomFieldInputs } from "@/features/reservations/components/custom-field-inputs";
import { useLocale } from "@/providers/locale-provider";

export const CONTACT_FORM_ID = "business-page-contact";

/**
 * Contact step: name and phone (e-mail and a note are optional). No account
 * or password; phone verification comes with the customer panel
 * (TEC-909 / TEC-910). The wizard's footer button submits this form.
 */
export function ContactStep({
  value,
  fields,
  onSubmit,
}: {
  value: ContactValues | null;
  /** The business's online questions (TEC-905). */
  fields: PublicBookingField[];
  onSubmit: (values: ContactValues) => void;
}) {
  const { t } = useLocale();
  const schema = useMemo(() => createContactSchema(t, fields), [t, fields]);

  return (
    <AppForm<ContactValues>
      id={CONTACT_FORM_ID}
      schema={schema}
      defaultValues={value ?? contactDefaults(fields)}
      onSubmit={onSubmit}
      className="grid gap-4 sm:grid-cols-2"
    >
      {(form) => (
        <>
          <AppInput
            name="first_name"
            label={t("business_page.contact.first_name")}
            autoComplete="given-name"
          />
          <AppInput
            name="last_name"
            label={t("business_page.contact.last_name")}
            autoComplete="family-name"
          />
          <AppInput
            name="phone"
            type="tel"
            inputMode="tel"
            autoComplete="tel"
            label={t("business_page.contact.phone")}
            description={t("business_page.contact.phone_hint")}
          />
          <AppInput
            name="email"
            type="email"
            inputMode="email"
            autoComplete="email"
            label={t("business_page.contact.email")}
          />
          {fields.length > 0 ? (
            <div className="grid gap-4 sm:col-span-2">
              <CustomFieldInputs fields={fields} />
            </div>
          ) : null}
          <AppTextarea
            name="customer_note"
            className="sm:col-span-2"
            rows={3}
            label={t("business_page.contact.note")}
          />
          <div className="space-y-3 sm:col-span-2">
            <AppCheckbox
              name="sms_consent"
              label={t("business_page.contact.sms_consent")}
            />
            <AppCheckbox
              name="email_consent"
              label={t("business_page.contact.email_consent")}
            />
          </div>
          {/* Honeypot: people never see or reach this field. */}
          <div
            aria-hidden="true"
            className="pointer-events-none absolute -left-[9999px] h-px w-px overflow-hidden opacity-0"
          >
            <Input
              {...form.register("website")}
              tabIndex={-1}
              autoComplete="off"
            />
          </div>
        </>
      )}
    </AppForm>
  );
}
