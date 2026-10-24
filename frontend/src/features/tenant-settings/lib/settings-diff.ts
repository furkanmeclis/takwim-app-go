import type { OrganizationSettings } from "@/features/tenant-settings/types";

export type SettingsDiffRow = {
  /** Stable dot path (`company.tax_number`, `sales_channels.web`, …). */
  path: string;
  before: string;
  after: string;
};

/**
 * Flattens a settings document into `path → display value`. Lists are keyed
 * by their natural key (channel code, invoice item type) instead of the
 * array index, so reordering never shows up as a change.
 */
export function flattenSettings(s: OrganizationSettings): Map<string, string> {
  const out = new Map<string, string>();
  const put = (path: string, value: unknown) => {
    if (value === null || value === undefined) out.set(path, "");
    else out.set(path, String(value));
  };
  for (const [section, values] of Object.entries({
    company: s.company,
    reservation: s.reservation,
    payment: s.payment,
    policies: s.policies,
  })) {
    for (const [key, value] of Object.entries(values)) {
      put(`${section}.${key}`, value);
    }
  }
  for (const channel of s.sales_channels) {
    put(`sales_channels.${channel.code}`, channel.is_active);
  }
  // Versions saved before TEC-908 decode with an empty list.
  put("embed.allowed_origins", (s.embed?.allowed_origins ?? []).join(", "));
  put("tax.default_vat_rate", s.tax.default_vat_rate);
  for (const mapping of s.tax.invoice_item_mappings) {
    put(
      `tax.invoice_item_mappings.${mapping.item_type}.account_code`,
      mapping.account_code,
    );
    put(
      `tax.invoice_item_mappings.${mapping.item_type}.vat_rate`,
      mapping.vat_rate,
    );
    put(
      `tax.invoice_item_mappings.${mapping.item_type}.description`,
      mapping.description,
    );
  }
  return out;
}

/** Changed paths between two documents, in document order. */
export function diffSettings(
  before: OrganizationSettings,
  after: OrganizationSettings,
): SettingsDiffRow[] {
  const a = flattenSettings(before);
  const b = flattenSettings(after);
  const paths = [...new Set([...a.keys(), ...b.keys()])];
  return paths
    .map((path) => ({
      path,
      before: a.get(path) ?? "",
      after: b.get(path) ?? "",
    }))
    .filter((row) => row.before !== row.after);
}
