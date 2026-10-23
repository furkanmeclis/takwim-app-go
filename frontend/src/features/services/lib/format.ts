import type { AppLocale } from "@/config/i18n";
import { currency } from "@/lib/utils/format";

/** "₺450,00" for an API amount ("450.00") in the organization's currency. */
export function priceLabel(amount: string, code: string, locale: AppLocale) {
  return currency(Number(amount), code || "TRY", locale);
}
