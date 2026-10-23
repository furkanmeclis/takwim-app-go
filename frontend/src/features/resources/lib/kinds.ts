"use client";

import { useMemo } from "react";

import { RESOURCE_KINDS } from "@/features/resources/constants";
import type { ResourceKind } from "@/features/resources/types";
import { SECTOR_RESOURCE_LABEL_KEYS } from "@/features/sectors/constants";
import { useSector } from "@/features/sectors/hooks/use-sector";
import { useLocale } from "@/providers/locale-provider";

/**
 * Kinds in the order the business needs them: the sector's own kinds first
 * (a restaurant books tables and areas), then the others. The API accepts
 * every kind, so a business can still add a device or a room.
 */
export function useResourceKinds() {
  const { sector } = useSector();
  const { t } = useLocale();

  return useMemo(() => {
    const own = (sector?.capacity.resources ?? []) as ResourceKind[];
    const ordered = [
      ...own,
      ...RESOURCE_KINDS.filter((kind) => !own.includes(kind)),
    ];
    const label = (kind: ResourceKind) => t(SECTOR_RESOURCE_LABEL_KEYS[kind]);
    return {
      /** Default kind of a new resource. */
      primary: ordered[0] ?? "chair",
      label,
      options: ordered.map((kind) => ({ value: kind, label: label(kind) })),
    };
  }, [sector?.capacity.resources, t]);
}

/** "2–4" or "4" for a capacity range. */
export function capacityLabel(min: number, max: number) {
  return min === max ? String(max) : `${min}–${max}`;
}
