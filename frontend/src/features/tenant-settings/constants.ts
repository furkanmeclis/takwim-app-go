import type {
  InvoiceItemType,
  SalesChannelCode,
} from "@/features/tenant-settings/types";

export const SALES_CHANNELS: SalesChannelCode[] = [
  "web",
  "widget",
  "api",
  "branch",
  "yolcu360",
  "obilet",
];

export const INVOICE_ITEM_TYPES: InvoiceItemType[] = [
  "rental",
  "extra_service",
  "insurance",
  "delivery",
  "fuel",
  "mileage",
  "late_return",
  "damage",
  "deposit",
];

/** Static label keys (no template keys, see check-i18n). */
export const SALES_CHANNEL_LABEL_KEYS: Record<SalesChannelCode, string> = {
  web: "tenant_settings.channels.web",
  widget: "tenant_settings.channels.widget",
  api: "tenant_settings.channels.api",
  branch: "tenant_settings.channels.branch",
  yolcu360: "tenant_settings.channels.yolcu360",
  obilet: "tenant_settings.channels.obilet",
};

export const SALES_CHANNEL_HINT_KEYS: Record<SalesChannelCode, string> = {
  web: "tenant_settings.channels.web_hint",
  widget: "tenant_settings.channels.widget_hint",
  api: "tenant_settings.channels.api_hint",
  branch: "tenant_settings.channels.branch_hint",
  yolcu360: "tenant_settings.channels.yolcu360_hint",
  obilet: "tenant_settings.channels.obilet_hint",
};

export const INVOICE_ITEM_LABEL_KEYS: Record<InvoiceItemType, string> = {
  rental: "tenant_settings.invoice_items.rental",
  extra_service: "tenant_settings.invoice_items.extra_service",
  insurance: "tenant_settings.invoice_items.insurance",
  delivery: "tenant_settings.invoice_items.delivery",
  fuel: "tenant_settings.invoice_items.fuel",
  mileage: "tenant_settings.invoice_items.mileage",
  late_return: "tenant_settings.invoice_items.late_return",
  damage: "tenant_settings.invoice_items.damage",
  deposit: "tenant_settings.invoice_items.deposit",
};

/** Form field label keys by settings dot path (readiness list + diff view). */
export const FIELD_LABEL_KEYS: Record<string, string> = {
  "company.legal_name": "tenant_settings.fields.legal_name",
  "company.tax_number": "tenant_settings.fields.tax_number",
  "company.tax_office": "tenant_settings.fields.tax_office",
  "company.address": "tenant_settings.fields.address",
  "company.phone": "tenant_settings.fields.phone",
  "company.email": "tenant_settings.fields.email",
  "reservation.approval_mode": "tenant_settings.fields.approval_mode",
  "reservation.staff_review_window_minutes":
    "tenant_settings.fields.staff_review_window_minutes",
  "reservation.payment_hold_minutes":
    "tenant_settings.fields.payment_hold_minutes",
  "reservation.quote_validity_minutes":
    "tenant_settings.fields.quote_validity_minutes",
  "payment.prepayment_rule": "tenant_settings.fields.prepayment_rule",
  "payment.prepayment_value": "tenant_settings.fields.prepayment_value",
  "payment.full_payment_allowed": "tenant_settings.fields.full_payment_allowed",
  "policies.cancellation": "tenant_settings.fields.cancellation",
  "policies.no_show": "tenant_settings.fields.no_show",
  "policies.early_return": "tenant_settings.fields.early_return",
  "policies.extension": "tenant_settings.fields.extension",
  "policies.late_return_grace_minutes":
    "tenant_settings.fields.late_return_grace_minutes",
  "policies.extra_hour_fee": "tenant_settings.fields.extra_hour_fee",
  "policies.fuel": "tenant_settings.fields.fuel",
  "policies.mileage": "tenant_settings.fields.mileage",
  "tax.default_vat_rate": "tenant_settings.fields.default_vat_rate",
};

/** Which tab holds a settings dot path (error badges, jump to first error). */
export type SettingsTab =
  "company" | "reservation" | "policies" | "channels" | "tax";

export function tabOfPath(path: string): SettingsTab {
  if (path.startsWith("company")) return "company";
  if (path.startsWith("reservation") || path.startsWith("payment"))
    return "reservation";
  if (path.startsWith("policies")) return "policies";
  if (path.startsWith("sales_channels")) return "channels";
  return "tax";
}
