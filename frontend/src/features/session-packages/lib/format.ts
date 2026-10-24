import { normalizeAmount } from "@/features/cash/lib/format";
import { fromMinor, toMinor } from "@/lib/utils/format";

/**
 * Value of one session as the API computes it (price / total, rounded
 * down to the cent; the last session takes the remainder). Shown while a
 * definition is typed; `null` while the inputs are not valid yet.
 */
export function sessionValue(price: string, total: string): string | null {
  const minor = toMinor(normalizeAmount(price ?? ""));
  const count = Number((total ?? "").trim());
  if (minor == null || !Number.isInteger(count) || count < 1) return null;
  return fromMinor(Math.floor(minor / count));
}

/** "3 / 5" used sessions of a package. */
export function sessionsLabel(used: number, total: number) {
  return `${used} / ${total}`;
}
