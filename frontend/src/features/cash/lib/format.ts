"use client";

import { useCallback, useMemo } from "react";

import type {
  CashAccountKind,
  CashMovement,
  CashMovementKind,
} from "@/features/cash/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { amount, dateKeyInZone, toMinor } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

export const kindLabel = (t: Translate, kind: CashAccountKind) =>
  t(`cash.kinds.${kind}`);

export const movementKindLabel = (t: Translate, kind: CashMovementKind) =>
  t(`cash.movement_kinds.${kind}`);

/** "1.250,00 ₺" in the active locale; `signed` prefixes money out with "−". */
export function useCashAmount() {
  const { locale } = useLocale();
  const { organization } = useTenant();
  const fallback = organization?.currency ?? "TRY";
  return useCallback(
    (value: string | null | undefined, currency?: string) =>
      amount(value, currency || fallback, locale),
    [fallback, locale],
  );
}

/** Signed amount of a movement as minor units (in = +, out = −). */
export function signedMinor(
  movement: Pick<CashMovement, "amount" | "direction">,
) {
  const minor = toMinor(movement.amount) ?? 0;
  return movement.direction === "out" ? -minor : minor;
}

/** Tone of a balance / difference: negative = danger, positive = success. */
export function amountTone(value: string | null | undefined) {
  const minor = value == null ? 0 : (toMinor(value) ?? 0);
  if (minor < 0) return "text-destructive";
  if (minor > 0) return "text-emerald-700 dark:text-emerald-300";
  return "text-muted-foreground";
}

/** Business date ("2026-06-01") in the organization's time zone. */
export function useBusinessToday() {
  const { organization } = useTenant();
  const zone = organization?.timezone ?? undefined;
  return useMemo(() => dateKeyInZone(new Date(), zone), [zone]);
}

/** "450,5" → "450.50"; anything else is returned trimmed for the API to refuse. */
export function normalizeAmount(value: string): string {
  const v = value.trim().replace(",", ".");
  const n = Number(v);
  return v && Number.isFinite(n) ? n.toFixed(2) : v;
}
