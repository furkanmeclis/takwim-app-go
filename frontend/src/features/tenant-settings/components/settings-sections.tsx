"use client";

import { useWatch } from "react-hook-form";

import {
  AppInput,
  AppSelect,
  AppSwitch,
  AppTextarea,
  FormSection,
} from "@/components/forms";
import {
  INVOICE_ITEM_LABEL_KEYS,
  INVOICE_ITEM_TYPES,
  SALES_CHANNELS,
  SALES_CHANNEL_HINT_KEYS,
  SALES_CHANNEL_LABEL_KEYS,
} from "@/features/tenant-settings/constants";
import type { SettingsFormValues } from "@/features/tenant-settings/schemas/settings-form";
import { useLocale } from "@/providers/locale-provider";

type SectionProps = { disabled?: boolean };

/** Firma: legal / billing identity and the public contact details. */
export function CompanySection({ disabled }: SectionProps) {
  const { t } = useLocale();
  return (
    <FormSection
      title={t("tenant_settings.sections.company")}
      description={t("tenant_settings.sections.company_desc")}
      columns={2}
    >
      <AppInput
        name="company.legal_name"
        label={t("tenant_settings.fields.legal_name")}
        className="sm:col-span-2"
        disabled={disabled}
      />
      <AppInput
        name="company.tax_number"
        label={t("tenant_settings.fields.tax_number")}
        description={t("tenant_settings.fields.tax_number_hint")}
        inputMode="numeric"
        maxLength={14}
        disabled={disabled}
      />
      <AppInput
        name="company.tax_office"
        label={t("tenant_settings.fields.tax_office")}
        disabled={disabled}
      />
      <AppTextarea
        name="company.address"
        label={t("tenant_settings.fields.address")}
        className="sm:col-span-2"
        rows={3}
        disabled={disabled}
      />
      <AppInput
        name="company.phone"
        type="tel"
        label={t("tenant_settings.fields.phone")}
        disabled={disabled}
      />
      <AppInput
        name="company.email"
        type="email"
        label={t("tenant_settings.fields.email")}
        disabled={disabled}
      />
      <AppInput
        name="company.website"
        type="url"
        label={t("tenant_settings.fields.website")}
        placeholder="https://"
        className="sm:col-span-2"
        disabled={disabled}
      />
    </FormSection>
  );
}

/**
 * Rezervasyon kuralları & ödeme: the business-wide default booking rules.
 * The review window only applies to staff approval and the payment hold only
 * to online payment, so those fields show when they matter.
 */
export function ReservationSection({ disabled }: SectionProps) {
  const { t } = useLocale();
  const approval = useWatch<SettingsFormValues, "reservation.approval_mode">({
    name: "reservation.approval_mode",
  });
  const payOnline = useWatch<SettingsFormValues, "payment.pay_online">({
    name: "payment.pay_online",
  });
  return (
    <div className="space-y-4">
      <FormSection
        title={t("tenant_settings.sections.reservation")}
        description={t("tenant_settings.sections.reservation_desc")}
        columns={2}
      >
        <AppSelect
          name="reservation.approval_mode"
          label={t("tenant_settings.fields.approval_mode")}
          placeholder={t("tenant_settings.fields.approval_mode_placeholder")}
          options={[
            {
              value: "auto",
              label: t("tenant_settings.approval_modes.auto"),
            },
            {
              value: "staff",
              label: t("tenant_settings.approval_modes.staff"),
            },
          ]}
          disabled={disabled}
        />
        {approval === "staff" ? (
          <AppInput
            name="reservation.staff_review_window_minutes"
            type="number"
            min={1}
            label={t("tenant_settings.fields.staff_review_window_minutes")}
            description={t("tenant_settings.fields.staff_review_window_hint")}
            disabled={disabled}
          />
        ) : (
          <div className="hidden sm:block" />
        )}
        <AppInput
          name="reservation.min_lead_minutes"
          type="number"
          min={0}
          label={t("tenant_settings.fields.min_lead_minutes")}
          description={t("tenant_settings.fields.min_lead_hint")}
          disabled={disabled}
        />
        <AppInput
          name="reservation.max_advance_days"
          type="number"
          min={1}
          label={t("tenant_settings.fields.max_advance_days")}
          description={t("tenant_settings.fields.max_advance_hint")}
          disabled={disabled}
        />
        <AppInput
          name="reservation.cancel_cutoff_minutes"
          type="number"
          min={0}
          label={t("tenant_settings.fields.cancel_cutoff_minutes")}
          description={t("tenant_settings.fields.cutoff_hint")}
          disabled={disabled}
        />
        <AppInput
          name="reservation.change_cutoff_minutes"
          type="number"
          min={0}
          label={t("tenant_settings.fields.change_cutoff_minutes")}
          description={t("tenant_settings.fields.cutoff_hint")}
          disabled={disabled}
        />
      </FormSection>
      <FormSection
        title={t("tenant_settings.sections.payment")}
        description={t("tenant_settings.sections.payment_desc")}
        columns={2}
      >
        <AppSwitch
          name="payment.pay_at_venue"
          label={t("tenant_settings.fields.pay_at_venue")}
          description={t("tenant_settings.fields.pay_at_venue_hint")}
          disabled={disabled}
        />
        <AppSwitch
          name="payment.pay_online"
          label={t("tenant_settings.fields.pay_online")}
          description={t("tenant_settings.fields.pay_online_hint")}
          disabled={disabled}
        />
        {payOnline ? (
          <AppInput
            name="reservation.payment_hold_minutes"
            type="number"
            min={1}
            label={t("tenant_settings.fields.payment_hold_minutes")}
            description={t("tenant_settings.fields.payment_hold_hint")}
            disabled={disabled}
          />
        ) : null}
      </FormSection>
    </div>
  );
}

/** Politikalar: customer-facing policy texts. */
export function PoliciesSection({ disabled }: SectionProps) {
  const { t } = useLocale();
  return (
    <FormSection
      title={t("tenant_settings.sections.policies")}
      description={t("tenant_settings.sections.policies_desc")}
      columns={2}
    >
      <AppTextarea
        name="policies.cancellation"
        label={t("tenant_settings.fields.cancellation")}
        rows={4}
        disabled={disabled}
      />
      <AppTextarea
        name="policies.no_show"
        label={t("tenant_settings.fields.no_show")}
        rows={4}
        disabled={disabled}
      />
      <AppTextarea
        name="policies.change"
        label={t("tenant_settings.fields.change")}
        rows={4}
        className="sm:col-span-2"
        disabled={disabled}
      />
    </FormSection>
  );
}

/** Satış kanalları: one switch per channel (form order = API order). */
export function ChannelsSection({ disabled }: SectionProps) {
  const { t } = useLocale();
  return (
    <FormSection
      title={t("tenant_settings.sections.channels")}
      description={t("tenant_settings.sections.channels_desc")}
      columns={2}
    >
      {SALES_CHANNELS.map((code, index) => (
        <AppSwitch
          key={code}
          name={`sales_channels.${index}.is_active`}
          label={t(SALES_CHANNEL_LABEL_KEYS[code])}
          description={t(SALES_CHANNEL_HINT_KEYS[code])}
          disabled={disabled}
        />
      ))}
    </FormSection>
  );
}

/** Vergi: default VAT (no built-in legal rate) and invoice item mapping. */
export function TaxSection({ disabled }: SectionProps) {
  const { t } = useLocale();
  return (
    <div className="space-y-4">
      <FormSection
        title={t("tenant_settings.sections.tax")}
        description={t("tenant_settings.sections.tax_desc")}
        columns={2}
      >
        <AppInput
          name="tax.default_vat_rate"
          inputMode="decimal"
          label={t("tenant_settings.fields.default_vat_rate")}
          description={t("tenant_settings.fields.default_vat_rate_hint")}
          disabled={disabled}
        />
      </FormSection>
      <FormSection
        title={t("tenant_settings.sections.invoice_items")}
        description={t("tenant_settings.sections.invoice_items_desc")}
      >
        <div className="divide-border divide-y">
          {INVOICE_ITEM_TYPES.map((type, index) => (
            <div
              key={type}
              className="grid gap-3 py-3 first:pt-0 last:pb-0 sm:grid-cols-[minmax(0,10rem)_1fr_8rem_1.5fr] sm:items-start"
            >
              <p className="pt-2 text-sm font-medium">
                {t(INVOICE_ITEM_LABEL_KEYS[type])}
              </p>
              <AppInput
                name={`tax.invoice_item_mappings.${index}.account_code`}
                placeholder={t("tenant_settings.fields.account_code")}
                aria-label={t("tenant_settings.fields.account_code")}
                disabled={disabled}
              />
              <AppInput
                name={`tax.invoice_item_mappings.${index}.vat_rate`}
                inputMode="decimal"
                placeholder={t("tenant_settings.fields.vat_rate")}
                aria-label={t("tenant_settings.fields.vat_rate")}
                disabled={disabled}
              />
              <AppInput
                name={`tax.invoice_item_mappings.${index}.description`}
                placeholder={t("tenant_settings.fields.mapping_description")}
                aria-label={t("tenant_settings.fields.mapping_description")}
                disabled={disabled}
              />
            </div>
          ))}
        </div>
      </FormSection>
    </div>
  );
}
