"use client";

import { useCallback } from "react";

import { useOptionalTenant } from "@/features/tenant/components/tenant-provider";
import { SECTOR_TERM_KEYS } from "@/features/sectors/constants";
import type { Sector, SectorModule } from "@/features/sectors/types";
import { useLocale } from "@/providers/locale-provider";

type TermNoun = keyof Sector["terms"];

/**
 * The current organization's sector (TEC-882): its definition, module
 * visibility (narrowed by the plan's add-ons, TEC-883) and the words the
 * panel uses ("Masa", "Oda / cihaz", "Uzman"). Outside the tenant shell, or
 * before the scope loads, `sector` is null, `hasModule` answers false and
 * `term` falls back to the generic word.
 */
export function useSector() {
  const tenant = useOptionalTenant();
  const sector = tenant?.sector ?? null;
  // Plan add-ons narrow the sector's modules (TEC-883).
  const modules = tenant?.entitlements?.modules ?? sector?.modules;
  const { t } = useLocale();

  const hasModule = useCallback(
    (module: SectorModule) => Boolean(modules?.includes(module)),
    [modules],
  );

  const term = useCallback(
    (noun: TermNoun) => {
      const code = sector?.terms[noun];
      const key = (code && SECTOR_TERM_KEYS[code]) ?? `sectors.generic.${noun}`;
      return t(key);
    },
    [sector, t],
  );

  return { sector, hasModule, term };
}
