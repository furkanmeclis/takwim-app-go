"use client";

import { useCallback } from "react";

import { useCashAmount } from "@/features/cash/lib/format";
import type {
  StaffShareBasis,
  StaffShareRule,
} from "@/features/staff-shares/types";
import { useLocale } from "@/providers/locale-provider";

type RateFields = {
  basis: StaffShareBasis;
  rate?: string | null;
  fixed_amount?: string | null;
};

/**
 * The agreed rate as people read it: "%40" (locale percent) or
 * "50,00 ₺ / adet" for a fixed amount per unit.
 */
export function useShareRateLabel() {
  const { t, locale } = useLocale();
  const money = useCashAmount();
  return useCallback(
    (value: RateFields, currency?: string) => {
      if (value.basis === "percent") {
        const n = Number(value.rate ?? "");
        if (!Number.isFinite(n)) return "—";
        return new Intl.NumberFormat(locale, {
          style: "percent",
          maximumFractionDigits: 2,
        }).format(n / 100);
      }
      return t("staff_shares.rate.fixed_per_unit", {
        amount: money(value.fixed_amount, currency),
      });
    },
    [locale, money, t],
  );
}

/** "Hizmetler (varsayılan)" or the service name of an override. */
export function ruleScopeLabel(
  t: (key: string, params?: Record<string, string | number>) => string,
  rule: Pick<StaffShareRule, "item_type" | "service">,
) {
  if (rule.service) return rule.service.name;
  return rule.item_type === "product"
    ? t("staff_shares.rules.scope_default_product")
    : t("staff_shares.rules.scope_default_service");
}
