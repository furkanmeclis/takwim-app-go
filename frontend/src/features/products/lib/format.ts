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
