import type { AppLocale } from "@/config/i18n";
import { ApiError } from "@/lib/api/errors";
import { currency } from "@/lib/utils/format";

/** "₺450,00" for an API amount ("450.00") in the organization's currency. */
export function priceLabel(amount: string, code: string, locale: AppLocale) {
  return currency(Number(amount), code || "TRY", locale);
}

/** "+5" / "-2" for a signed ledger quantity. */
export function signedQuantity(quantity: number, locale: AppLocale) {
  const value = Math.abs(quantity).toLocaleString(locale);
  if (quantity > 0) return `+${value}`;
  if (quantity < 0) return `-${value}`;
  return value;
}

/**
 * Field details of a stock or catalog API error carry English messages
 * and a code (`insufficient_stock`, `not_offered`, …). Show the localized
 * text for codes `products.errors.*` knows; `AppForm` puts them under the
 * fields.
 */
export function localizeProductError(
  error: unknown,
  t: (key: string) => string,
) {
  if (!(error instanceof ApiError)) return;
  for (const detail of error.details) {
    if (!detail.code) continue;
    const key = `products.errors.${detail.code}`;
    const text = t(key);
    if (text !== key) detail.message = text;
  }
}

/**
 * Select options of the accounts a counter sale uses (TEC-992); a branch
 * account is marked so a business-wide one of the same name stands apart.
 */
export function accountOptions(
  accounts: { uuid: string; name: string; branch: boolean }[],
  t: (key: string, params?: Record<string, string | number>) => string,
) {
  return accounts.map((account) => ({
    value: account.uuid,
    label: account.branch
      ? t("products.movements.collection.account_branch", {
          name: account.name,
        })
      : account.name,
  }));
}

/**
 * The money state of a sale row (TEC-992): what the sale is worth
 * (units × unit price), what was collected and paid back, and the chip to
 * show. Null for other kinds.
 */
export function salePayment(movement: {
  kind: string;
  quantity: number;
  unit_price: string | null;
  collected: string | null;
  refunded: string | null;
}) {
  if (movement.kind !== "sale" || movement.collected == null) return null;
  const total = Math.abs(movement.quantity) * Number(movement.unit_price ?? 0);
  const collected = Number(movement.collected);
  const refunded = Number(movement.refunded ?? 0);
  const paid = collected - refunded;
  const remaining = Math.max(0, Math.round((total - paid) * 100) / 100);
  const refundable = Math.max(0, Math.round(paid * 100) / 100);
  let state: "unpaid" | "partial" | "paid" | "refunded";
  let tone: "default" | "success" | "warning" | "danger";
  if (refunded > 0 && refundable === 0) {
    state = "refunded";
    tone = "default";
  } else if (collected === 0 && total > 0) {
    state = "unpaid";
    tone = "warning";
  } else if (remaining > 0) {
    state = "partial";
    tone = "warning";
  } else {
    state = "paid";
    tone = "success";
  }
  return {
    total,
    collected: movement.collected,
    refunded,
    remaining,
    refundable,
    state,
    tone,
  } as const;
}
