import type { ReportFigures, ReportMoney } from "@/features/reports/types";

/**
 * The money of the business currency (the API lists it first and always
 * includes it). Tables show this one; other currencies appear in the cards.
 */
export function mainMoney(
  figures: Pick<ReportFigures, "money">,
  currency: string,
): ReportMoney | undefined {
  return figures.money.find((m) => m.currency === currency) ?? figures.money[0];
}

/** Share of a part in a total as a whole percent ("—" without a total). */
export function percent(part: number, total: number): string {
  if (!total) return "—";
  return `%${Math.round((part / total) * 100)}`;
}
