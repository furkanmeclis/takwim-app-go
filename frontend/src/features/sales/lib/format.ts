import type { LedgerEntry, SaleLine } from "@/features/sales/types";

/** Where a line stands in its correction chain (null = a plain live sale). */
export type LineState = "reversed" | "corrected" | "replacement" | null;

export function lineState(line: SaleLine): LineState {
  if (line.kind !== "sale") return null;
  if (line.replaced_by_uuid) return "corrected";
  if (line.reversed_by_uuid) return "reversed";
  if (line.replaces_uuid) return "replacement";
  return null;
}

export const LINE_STATE_LABEL_KEYS: Record<Exclude<LineState, null>, string> = {
  reversed: "sales.line_states.reversed",
  corrected: "sales.line_states.corrected",
  replacement: "sales.line_states.replacement",
};

/** A live sale line (not a reversal, not reversed yet) can be reversed or corrected. */
export function isLiveSale(line: SaleLine) {
  return line.kind === "sale" && !line.reversed_by_uuid;
}

/** Amount of an entry as it moves the customer's balance: debit +, credit −. */
export function signedEntryAmount(
  entry: Pick<LedgerEntry, "amount" | "direction">,
) {
  const value = entry.amount.trim();
  if (entry.direction === "debit") return value;
  return value.startsWith("-") ? value.slice(1) : `-${value}`;
}
