import type {
  InvoiceItemType,
  SalesChannelCode,
} from "@/features/tenant-settings/types";

export const SALES_CHANNELS: SalesChannelCode[] = ["web", "widget", "api"];

export const INVOICE_ITEM_TYPES: InvoiceItemType[] = [
  "service",
  "product",
  "package",
];

/** Static label keys (no template keys, see check-i18n). */
export const SALES_CHANNEL_LABEL_KEYS: Record<SalesChannelCode, string> = {
  web: "tenant_settings.channels.web",
  widget: "tenant_settings.channels.widget",
  api: "tenant_settings.channels.api",
};

export const SALES_CHANNEL_HINT_KEYS: Record<SalesChannelCode, string> = {
  web: "tenant_settings.channels.web_hint",
  widget: "tenant_settings.channels.widget_hint",
  api: "tenant_settings.channels.api_hint",
};

export const INVOICE_ITEM_LABEL_KEYS: Record<InvoiceItemType, string> = {
  service: "tenant_settings.invoice_items.service",
  product: "tenant_settings.invoice_items.product",
  package: "tenant_settings.invoice_items.package",
};

/** Form field label keys by settings dot path (readiness list + diff view). */
export const FIELD_LABEL_KEYS: Record<string, string> = {
  "company.legal_name": "tenant_settings.fields.legal_name",
  "company.tax_number": "tenant_settings.fields.tax_number",
  "company.tax_office": "tenant_settings.fields.tax_office",
  "company.address": "tenant_settings.fields.address",
  "company.phone": "tenant_settings.fields.phone",
  "company.email": "tenant_settings.fields.email",
  "company.website": "tenant_settings.fields.website",
  "reservation.approval_mode": "tenant_settings.fields.approval_mode",
  "reservation.staff_review_window_minutes":
    "tenant_settings.fields.staff_review_window_minutes",
  "reservation.payment_hold_minutes":
    "tenant_settings.fields.payment_hold_minutes",
  "reservation.min_lead_minutes": "tenant_settings.fields.min_lead_minutes",
  "reservation.max_advance_days": "tenant_settings.fields.max_advance_days",
  "reservation.cancel_cutoff_minutes":
    "tenant_settings.fields.cancel_cutoff_minutes",
  "reservation.change_cutoff_minutes":
    "tenant_settings.fields.change_cutoff_minutes",
  "payment.pay_at_venue": "tenant_settings.fields.pay_at_venue",
  "payment.pay_online": "tenant_settings.fields.pay_online",
  "policies.cancellation": "tenant_settings.fields.cancellation",
  "policies.no_show": "tenant_settings.fields.no_show",
  "policies.change": "tenant_settings.fields.change",
  "tax.default_vat_rate": "tenant_settings.fields.default_vat_rate",
  "embed.allowed_origins": "tenant_settings.fields.embed_origins",
};

/** Which tab holds a settings dot path (error badges, jump to first error). */
export type SettingsTab =
  "company" | "reservation" | "policies" | "channels" | "tax";

export function tabOfPath(path: string): SettingsTab {
  if (path.startsWith("company")) return "company";
  if (path.startsWith("reservation") || path.startsWith("payment"))
    return "reservation";
  if (path.startsWith("policies")) return "policies";
  if (path.startsWith("sales_channels") || path.startsWith("embed"))
    return "channels";
  return "tax";
}
