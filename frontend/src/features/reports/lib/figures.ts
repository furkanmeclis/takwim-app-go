import type {
  ReportFigures,
  ReportMoney,
  ReportPackageBalance,
} from "@/features/reports/types";

/** The open package rights of the business currency (always listed first). */
export function mainBalance(
  balance: ReportPackageBalance[],
  currency: string,
): ReportPackageBalance | undefined {
  return balance.find((b) => b.currency === currency) ?? balance[0];
}

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
